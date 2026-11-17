package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPickModel(t *testing.T) {
	db, err := openDB()
	if err != nil {
		t.Fatal(err)
	}
	for budget, want := range map[float64]string{
		400: "qwen3-coder:480b",
		32:  "qwen3.8:27b",  // 21.2 fits; gpt-oss:120b (78.5) does not
		20:  "devstral:24b", // qwen3.8 needs 21.2
		9.6: "mimo-v2.6:9b", // 7.9 fits, qwen3:14b (11.2) doesn't
		3:   "qwen3:4b",
		1.5: "lfm3:1.7b",
	} {
		if got, err := pickModel(db, budget); err != nil || got != want {
			t.Errorf("budget %v: got %q, %v; want %q", budget, got, err, want)
		}
	}
	if _, err := pickModel(db, 1); err == nil {
		t.Error("budget 1: want error, nothing fits")
	}
}

func TestWriteConfigMerges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.json")
	userPerms := `[{"action":"*","effect":"allow","resource":"*"}]`
	os.WriteFile(path, []byte(`{"theme":"x","permissions":`+userPerms+`}`), 0o600)

	if err := writeConfig(path, "gleipnir/m", map[string]any{"name": "Gleipnir"}); err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	b, _ := os.ReadFile(path)
	json.Unmarshal(b, &cfg)

	if cfg["theme"] != "x" || cfg["model"] != "gleipnir/m" || cfg["providers"].(map[string]any)["gleipnir"] == nil {
		t.Errorf("merge lost or missed keys: %s", b)
	}
	if p, _ := json.Marshal(cfg["permissions"]); string(p) != userPerms {
		t.Errorf("user permissions overwritten: %s", p)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Error("no backup written")
	}

	fresh := filepath.Join(t.TempDir(), "opencode.json")
	writeConfig(fresh, "", nil)
	b, _ = os.ReadFile(fresh)
	json.Unmarshal(b, &cfg)
	if len(cfg["permissions"].([]any)) != len(safePermissions) {
		t.Errorf("fresh config missing safe permissions: %s", b)
	}
}
