// gleip: runs OpenCode, and `gleip setup` points it at an account, a local model or a gleipnir endpoint.
package main

import (
	"bufio"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed models.sql
var seed string

var safePermissions = []map[string]string{
	{"action": "shell", "resource": "*", "effect": "ask"},
	{"action": "shell", "resource": "git status *", "effect": "allow"},
	{"action": "shell", "resource": "git diff *", "effect": "allow"},
	{"action": "shell", "resource": "git log *", "effect": "allow"},
	{"action": "shell", "resource": "sudo *", "effect": "deny"},
	{"action": "external_directory", "resource": "*", "effect": "ask"},
	{"action": "read", "resource": "*.env", "effect": "deny"},
	{"action": "read", "resource": "*.env.*", "effect": "deny"},
	{"action": "read", "resource": "*.env.example", "effect": "allow"},
}

var in = bufio.NewReader(os.Stdin)

const banner = `
  ____  _      _____  ___  ____   _   _  ___  ____
 / ___|| |    | ____||_ _||  _ \ | \ | ||_ _||  _ \
| |  _ | |    |  _|   | | | |_) ||  \| | | | | |_) |
| |_| || |___ | |___  | | |  __/ | |\  | | | |  _ <
 \____||_____||_____||___||_|    |_| \_||___||_| \_\
  o-O-o-O-o-O-o-O-o-O-o-O-o-O-o-O-o-O-o-O-o-O-o-O-o
   forged from a cat's footfall and a mountain's roots
`

const help = `
A coding agent on your own model. OpenCode inside, gleipnir around it.

Usage:
  gleip                  start coding in the current directory
  gleip setup            choose the model: your subscription/API key, a local model, or a gleipnir endpoint
  gleip fenrir [flags]   serve a model from this machine to gleipnir clients
      --auth             require an API key (keys in ~/.config/gleipnir/keys)
      --listen ADDR      where to serve (default 127.0.0.1:47100; 0.0.0.0:47100 to expose)
      --model TAG        Ollama model to serve (default: best fit for this machine)
  gleip --update         update gleip and OpenCode
  gleip --help           show this help

Anything else goes straight to OpenCode (see: opencode --help), e.g.
  gleip run "explain this repo"

Docs: https://github.com/bjornaer/gleipnir
`

func main() {
	if fi, _ := os.Stdout.Stat(); len(os.Args) == 1 && fi.Mode()&os.ModeCharDevice != 0 {
		fmt.Print(banner) // only bare `gleip` in a terminal; stays in scrollback behind the TUI
	}
	if len(os.Args) > 1 && os.Args[1] == "setup" {
		fmt.Print(banner)
		if err := setup(); err != nil {
			fmt.Fprintln(os.Stderr, "gleip:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "fenrir" {
		if err := fenrir(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "gleip fenrir:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-h" || os.Args[1] == "help") {
		fmt.Print(banner + help)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--update" {
		// Re-run the installer: same checksum checks, and it bumps the pinned OpenCode too.
		const rel = "https://github.com/bjornaer/gleipnir/releases/latest/download/"
		if runtime.GOOS == "windows" {
			os.Exit(run("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", "irm "+rel+"install.ps1 | iex"))
		}
		os.Exit(run("sh", "-c", "curl -fsSL "+rel+"install.sh | sh"))
	}
	os.Exit(run(opencode(), os.Args[1:]...))
}

func setup() error {
	fmt.Print("\nPoint gleipnir at:\n  1) your subscription / API key (ChatGPT, Anthropic, OpenAI, OpenRouter, ...)\n  2) a local model (Ollama)\n  3) a gleipnir endpoint\n> ")
	var model string
	var provider map[string]any
	switch ask("") {
	case "1":
		if run(opencode(), "auth", "login") != 0 {
			return errors.New("opencode auth login failed")
		}
	case "2":
		db, err := openDB()
		if err != nil {
			return err
		}
		budget := memoryBudgetGB()
		home, _ := os.UserHomeDir()
		pick, err := pickModel(db, budget, diskFreeGB(home))
		if err != nil {
			return fmt.Errorf("%w; use option 1 or 3", err)
		}
		fmt.Printf("~%.0f GB usable for models; best fit: %s\n", budget, pick)
		model = ask("Model [" + pick + "]: ")
		if model == "" {
			model = pick
		}
		if _, err := exec.LookPath("ollama"); err != nil {
			return errors.New("ollama not found; install it first:\n  " + ollamaInstall())
		}
		if run("ollama", "pull", model) != 0 {
			return errors.New("ollama pull failed")
		}
		model = "ollama/" + model
	case "3":
		url := ask("Endpoint base URL (ends in /v1): ")
		name := ask("Model name on that endpoint: ")
		if url == "" || name == "" {
			return errors.New("URL and model are required")
		}
		provider = map[string]any{
			"name":     "Gleipnir",
			"env":      []string{"GLEIPNIR_API_KEY"},
			"package":  "@opencode/ai/providers/openai-compatible",
			"settings": map[string]any{"baseURL": url},
			"models":   map[string]any{name: map[string]any{}},
		}
		model = "gleipnir/" + name
		fmt.Println("Set GLEIPNIR_API_KEY in your environment to the key you were given.")
	default:
		return errors.New("pick 1, 2 or 3")
	}
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := writeConfig(path, model, provider); err != nil {
		return err
	}
	fmt.Println("Wrote", path, "- run `gleip` to start.")
	return nil
}

func openDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // each :memory: connection is its own DB
	_, err = db.Exec(seed)
	return db, err
}

// pickModel returns the best-ranked model whose weights plus ~20% context headroom fit in memory,
// and whose download leaves 10 GB of disk for everything else.
func pickModel(db *sql.DB, memGB, diskGB float64) (string, error) {
	var id string
	err := db.QueryRow(`SELECT id FROM models WHERE size_gb * 1.2 <= ? AND size_gb * 1.1 + 10 <= ? ORDER BY rank LIMIT 1`, memGB, diskGB).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("no local model fits in %.1f GB memory / %.1f GB free disk", memGB, diskGB)
	}
	return id, err
}

// memoryBudgetGB: NVIDIA VRAM if present, else the RAM budget (Apple unified memory lands here too).
// ponytail: no AMD/Intel GPU detection; add when someone hits it.
func memoryBudgetGB() float64 {
	if out, err := exec.Command("nvidia-smi", "--query-gpu=memory.total", "--format=csv,noheader,nounits").Output(); err == nil {
		mib, _ := strconv.ParseFloat(strings.TrimSpace(strings.Split(string(out), "\n")[0]), 64)
		if mib > 0 {
			return mib / 1024 * 0.9
		}
	}
	return ramBudgetGB()
}

// ramBudgetGB leaves room for everything else: 60% of total RAM, and on Linux
// no more than 80% of what other running services leave available.
// ponytail: macOS/Windows use total RAM only.
func ramBudgetGB() float64 {
	var out []byte
	switch runtime.GOOS {
	case "darwin":
		out, _ = exec.Command("sysctl", "-n", "hw.memsize").Output()
	case "windows":
		out, _ = exec.Command("powershell", "-NoProfile", "-Command", "(Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory").Output()
	default:
		b, _ := os.ReadFile("/proc/meminfo") // "MemTotal:  16303404 kB"
		kb := map[string]float64{}
		for _, line := range strings.Split(string(b), "\n") {
			if f := strings.Fields(line); len(f) > 1 {
				kb[f[0]], _ = strconv.ParseFloat(f[1], 64)
			}
		}
		return min(kb["MemTotal:"]*0.6, kb["MemAvailable:"]*0.8) / 1e6
	}
	n, _ := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	return n / 1e9 * 0.6
}

// diskFreeGB returns free space on the filesystem holding dir; +Inf if unknown (Windows), so it never blocks.
func diskFreeGB(dir string) float64 {
	out, err := exec.Command("df", "-Pk", dir).Output()
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if f := strings.Fields(lines[len(lines)-1]); err == nil && len(lines) > 1 && len(f) >= 4 { // fs blocks used avail ...
		if kb, err := strconv.ParseFloat(f[3], 64); err == nil {
			return kb / 1e6
		}
	}
	return math.Inf(1)
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".config", "opencode")
	if _, err := os.Stat(filepath.Join(dir, "opencode.jsonc")); err == nil {
		return "", errors.New(dir + "/opencode.jsonc exists; gleip can't merge comments. Rename it to opencode.json (removing comments) and rerun")
	}
	return filepath.Join(dir, "opencode.json"), os.MkdirAll(dir, 0o755)
}

// writeConfig merges gleipnir's settings into an existing OpenCode config, keeping a .bak.
// Existing permissions are left alone; safe defaults only go in when there are none.
func writeConfig(path, model string, provider map[string]any) error {
	cfg := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &cfg); err != nil {
			return fmt.Errorf("%s is not valid JSON: %w", path, err)
		}
		if err := os.WriteFile(path+".bak", b, 0o600); err != nil {
			return err
		}
	}
	if _, ok := cfg["$schema"]; !ok {
		cfg["$schema"] = "https://opencode.ai/config.json"
	}
	if model != "" {
		cfg["model"] = model
	}
	if provider != nil {
		providers, _ := cfg["providers"].(map[string]any)
		if providers == nil {
			providers = map[string]any{}
		}
		providers["gleipnir"] = provider
		cfg["providers"] = providers
	}
	if _, ok := cfg["permissions"]; !ok {
		cfg["permissions"] = safePermissions
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

func opencode() string {
	if p, err := exec.LookPath("opencode"); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	exe := "opencode"
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	return filepath.Join(home, ".opencode", "bin", exe)
}

func ollamaInstall() string {
	if runtime.GOOS == "windows" {
		return "irm https://ollama.com/install.ps1 | iex"
	}
	return "curl -fsSL https://ollama.com/install.sh | sh"
}

func ask(prompt string) string {
	fmt.Print(prompt)
	s, _ := in.ReadString('\n')
	return strings.TrimSpace(s)
}

func run(name string, args ...string) int {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		fmt.Fprintln(os.Stderr, "gleip:", err)
		return 1
	}
	return 0
}
