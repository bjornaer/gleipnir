package main

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
		if got, err := pickModel(db, budget, math.Inf(1)); err != nil || got != want {
			t.Errorf("budget %v: got %q, %v; want %q", budget, got, err, want)
		}
	}
	if _, err := pickModel(db, 1, math.Inf(1)); err == nil {
		t.Error("budget 1: want error, nothing fits")
	}
	// Plenty of RAM but 40 GB of disk: qwen3.8:27b needs 17.7*1.1+10 = 29.5, gpt-oss:120b needs 81.9.
	if got, _ := pickModel(db, 400, 40); got != "qwen3.8:27b" {
		t.Errorf("disk-limited: got %q, want qwen3.8:27b", got)
	}
}

func TestProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("key leaked upstream")
		}
		if h, _, _ := strings.Cut(r.Host, ":"); h != "127.0.0.1" {
			t.Errorf("upstream got Host %q; Ollama rejects non-local hosts", r.Host)
		}
		io.WriteString(w, r.URL.Path)
	}))
	defer upstream.Close()
	keys := filepath.Join(t.TempDir(), "keys")
	os.WriteFile(keys, []byte("# comment\ngood alice\n"), 0o600)
	u, _ := url.Parse(upstream.URL)
	srv := httptest.NewServer(proxy(u.Host, keys))
	defer srv.Close()

	for _, c := range []struct {
		path, key string
		want      int
	}{
		{"/v1/models", "good", 200},
		{"/v1/models", "", 401},
		{"/v1/models", "bad", 401},
		{"/v1/models", "#", 401},     // comment marker isn't a key
		{"/v1/models", "alice", 401}, // label isn't a key
		{"/api/delete", "good", 404}, // Ollama admin API never exposed
		{"/v1/../api/delete", "good", 404},
		{"/v1/%2e%2e/api/pull", "good", 404},
	} {
		req, _ := http.NewRequest("GET", srv.URL+c.path, nil)
		req.URL.Opaque = c.path     // send the path raw, unnormalized by the client
		req.Host = "fenrir.example" // as a remote client would
		if c.key != "" {
			req.Header.Set("Authorization", "Bearer "+c.key)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != c.want {
			t.Errorf("%s key=%q: got %d, want %d", c.path, c.key, res.StatusCode, c.want)
		}
	}

	open := httptest.NewServer(proxy(u.Host, "")) // --auth off
	defer open.Close()
	if res, _ := http.Get(open.URL + "/v1/models"); res.StatusCode != 200 {
		t.Errorf("auth off: got %d, want 200", res.StatusCode)
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
