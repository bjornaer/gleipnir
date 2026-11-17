// gleip fenrir: serve a local model to gleipnir clients, sized so the machine keeps room for everything else.
//
// Ollama stays on 127.0.0.1:11434. A small reverse proxy in front of it exposes only the
// OpenAI-compatible /v1/* routes (never Ollama's admin API: pull, delete, ...) and, with --auth,
// requires a key from ~/.config/gleipnir/keys.
package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const ollamaAddr = "127.0.0.1:11434"

func fenrir(args []string) error {
	fs := flag.NewFlagSet("fenrir", flag.ExitOnError)
	auth := fs.Bool("auth", false, "require an API key (keys live in ~/.config/gleipnir/keys, one per line)")
	listen := fs.String("listen", "127.0.0.1:47100", "address to serve on; use 0.0.0.0:47100 to reach it from other machines")
	model := fs.String("model", "", "Ollama model to serve (default: best fit for this machine)")
	fs.Parse(args)

	// Fail fast on a taken port, before downloading gigabytes.
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		return fmt.Errorf("can't listen on %s (port taken? choose another with --listen): %w", *listen, err)
	}
	defer ln.Close()

	if *model == "" {
		db, err := openDB()
		if err != nil {
			return err
		}
		mem := memoryBudgetGB()
		if *model, err = pickModel(db, mem, diskFreeGB(modelDir())); err != nil {
			return err
		}
		fmt.Printf("~%.0f GB memory budget, serving %s\n", mem, *model)
	}
	if err := ensureOllama(); err != nil {
		return err
	}
	capOllama()
	if run("ollama", "pull", *model) != 0 {
		return errors.New("ollama pull failed")
	}

	keys := ""
	if *auth {
		if keys, err = ensureKeys(); err != nil {
			return err
		}
	}

	host, port, _ := net.SplitHostPort(*listen)
	exposed := !net.ParseIP(host).IsLoopback()
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "<this-machine's-address>"
	}
	fmt.Printf("\nFenrir is up on %s, serving %s.\n", *listen, *model)
	fmt.Printf("On each client: gleip setup -> 3 -> URL http://%s:%s/v1 -> model %s\n", host, port, *model)
	if *auth {
		fmt.Println("and export GLEIPNIR_API_KEY=<one of the keys in " + keys + ">")
	} else {
		fmt.Println("and export GLEIPNIR_API_KEY=none (no auth; any value works)")
	}
	if exposed && !*auth {
		fmt.Println("WARNING: reachable from the network with --auth off: anyone who finds this port can use your model.")
	}
	if exposed {
		fmt.Println("NOTE: plain HTTP. Put it behind your existing nginx/Caddy for HTTPS, or keys travel in clear text.")
	}

	srv := &http.Server{Handler: proxy(ollamaAddr, keys), ReadHeaderTimeout: 10 * time.Second}
	return srv.Serve(ln)
}

// proxy forwards /v1/* to Ollama, checking the bearer key against keysFile when it's set.
func proxy(target, keysFile string) http.Handler {
	rp := httputil.NewSingleHostReverseProxy(&url.URL{Scheme: "http", Host: target})
	rp.FlushInterval = -1 // stream tokens as they arrive
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean(r.URL.Path) // so /v1/../api/delete can't slip through
		if !strings.HasPrefix(p, "/v1/") {
			http.NotFound(w, r)
			return
		}
		if keysFile != "" && !validKey(keysFile, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")) {
			http.Error(w, "invalid API key", http.StatusUnauthorized)
			return
		}
		r.URL.Path, r.URL.RawPath = p, ""
		r.Host = target // Ollama 403s any non-local Host header (DNS-rebinding guard)
		r.Header.Del("Authorization")
		r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
		rp.ServeHTTP(w, r)
	})
}

// validKey reads the keys file on every request, so adding or removing a friend needs no restart.
// Format: one "<key> [label]" per line; lines starting with # are ignored.
func validKey(file, got string) bool {
	b, err := os.ReadFile(file)
	if err != nil || got == "" {
		return false
	}
	ok := false
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) > 0 && !strings.HasPrefix(f[0], "#") && subtle.ConstantTimeCompare([]byte(f[0]), []byte(got)) == 1 {
			ok = true
		}
	}
	return ok
}

func ensureKeys() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	file := filepath.Join(home, ".config", "gleipnir", "keys")
	if b, err := os.ReadFile(file); err == nil && len(strings.TrimSpace(string(b))) > 0 {
		return file, nil
	}
	buf := make([]byte, 32)
	rand.Read(buf)
	key := "glp_" + hex.EncodeToString(buf)
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(file, []byte(key+" first-key\n"), 0o600); err != nil {
		return "", err
	}
	fmt.Printf("Generated API key (saved in %s; add one line per friend):\n  %s\n", file, key)
	return file, nil
}

func ollamaUp() bool {
	c := http.Client{Timeout: 2 * time.Second}
	r, err := c.Get("http://" + ollamaAddr + "/api/version")
	if err != nil {
		return false
	}
	r.Body.Close()
	return r.StatusCode == http.StatusOK
}

func waitOllama() bool {
	for i := 0; i < 30 && !ollamaUp(); i++ {
		time.Sleep(time.Second)
	}
	return ollamaUp()
}

func ensureOllama() error {
	if ollamaUp() {
		return nil
	}
	if c, err := net.DialTimeout("tcp", ollamaAddr, time.Second); err == nil {
		c.Close()
		return errors.New(ollamaAddr + " is taken by something that isn't Ollama")
	}
	if _, err := exec.LookPath("ollama"); err != nil {
		if runtime.GOOS == "windows" {
			return errors.New("install Ollama first: " + ollamaInstall())
		}
		fmt.Println("Installing Ollama with its official installer (may ask for sudo)...")
		if run("sh", "-c", ollamaInstall()) != 0 {
			return errors.New("ollama install failed")
		}
	}
	if !waitOllama() {
		return errors.New("ollama is installed but not running; start it (`ollama serve`, or open the Ollama app) and rerun")
	}
	return nil
}

// capOllama limits the systemd Ollama service so the model can't starve other services.
// ponytail: Linux+systemd only; elsewhere Ollama runs uncapped but still loads one model at a time.
// The knobs (50% of CPUs, RAM budget, 2 parallel requests) live in the drop-in; edit it to retune.
func capOllama() {
	if runtime.GOOS != "linux" || exec.Command("systemctl", "cat", "ollama").Run() != nil {
		return
	}
	conf := fmt.Sprintf(`[Service]
Environment=OLLAMA_MAX_LOADED_MODELS=1
Environment=OLLAMA_NUM_PARALLEL=2
Environment=OLLAMA_KEEP_ALIVE=10m
MemoryMax=%dG
CPUQuota=%d%%
Nice=10
`, int(math.Ceil(ramBudgetGB())), runtime.NumCPU()*50)
	sudo := "sudo "
	if os.Geteuid() == 0 {
		sudo = ""
	}
	dir := "/etc/systemd/system/ollama.service.d"
	cmd := exec.Command("sh", "-c", sudo+"mkdir -p "+dir+" && "+sudo+"tee "+dir+"/gleipnir.conf >/dev/null && "+
		sudo+"systemctl daemon-reload && "+sudo+"systemctl restart ollama")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = strings.NewReader(conf), os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil || !waitOllama() {
		fmt.Println("warning: couldn't cap Ollama's CPU/memory; it runs unlimited")
		return
	}
	fmt.Println("Capped Ollama via " + dir + "/gleipnir.conf")
}

// modelDir is where Ollama will store weights, for the disk check.
func modelDir() string {
	if d := os.Getenv("OLLAMA_MODELS"); d != "" {
		return d
	}
	if runtime.GOOS == "linux" {
		return "/" // the systemd service keeps models under /usr/share/ollama
	}
	home, _ := os.UserHomeDir()
	return home
}
