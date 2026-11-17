# gleipnir

```
  ____  _      _____  ___  ____   _   _  ___  ____
 / ___|| |    | ____||_ _||  _ \ | \ | ||_ _||  _ \
| |  _ | |    |  _|   | | | |_) ||  \| | | | | |_) |
| |_| || |___ | |___  | | |  __/ | |\  | | | |  _ <
 \____||_____||_____||___||_|    |_| \_||___||_| \_\
```

A coding agent you point at your own model: your ChatGPT plan, an API key, a model on your own machine, or a model a friend serves with `gleip fenrir`. Under the hood it's [OpenCode](https://opencode.ai) with a one-line installer, a hardware-aware model picker, and cautious permission defaults.

## Install

macOS / Linux:
```sh
curl -fsSL https://github.com/bjornaer/gleipnir/releases/latest/download/install.sh | sh
```

Windows (PowerShell):
```powershell
irm https://github.com/bjornaer/gleipnir/releases/latest/download/install.ps1 | iex
```

The installer checks the download against the release checksums, installs `gleip` and a pinned OpenCode into `~/.opencode/bin`, and prints the help. Open a new terminal afterwards so `gleip` is on your PATH.

## Quick start

```sh
gleip setup   # choose which model to use (once)
gleip         # start coding in the current directory
gleip --help  # all commands
```

## Choosing a model: `gleip setup`

`gleip setup` offers three options:

**1. Your subscription or API key.** This opens OpenCode's login.
- **ChatGPT Plus/Pro:** choose OpenAI → Sign in with ChatGPT. Usage counts against your plan.
- **API keys:** Anthropic, OpenAI, OpenRouter, DeepSeek and others.
- **Claude Pro/Max can't be used.** Anthropic's terms forbid subscriptions in third-party tools and accounts get banned for it. Use an Anthropic API key, or Claude Code.

**2. A local model.** gleip reads your RAM (or NVIDIA VRAM) and free disk, picks the best model from [`models.sql`](models.sql) that fits, and pulls it with Ollama. You can type a different model at the prompt. Ollama must already be installed: `curl -fsSL https://ollama.com/install.sh | sh`.

**3. A gleipnir endpoint.** Use this for a model someone serves with `gleip fenrir`. You'll enter:
- the URL, for example `https://llm.example.com/v1`
- the model name
- then set the key in your shell profile: `export GLEIPNIR_API_KEY=glp_...`. If the server has no auth, set it to any value.

Settings are written to `~/.config/opencode/opencode.json`. An existing config is merged, not replaced, and backed up to `opencode.json.bak` first.

## Serving a model: `gleip fenrir`

`fenrir` turns a machine (your laptop, a Hetzner box) into a model server for gleipnir clients.

```sh
gleip fenrir                                   # this machine only, no auth
gleip fenrir --auth --listen 0.0.0.0:47100     # reachable from the network, keys required
gleip fenrir --model qwen3:8b                  # choose the model yourself
```

Without gleip installed (servers):
```sh
curl -fsSL https://github.com/bjornaer/gleipnir/releases/latest/download/fenrir.sh | sh -s -- --auth
```
`fenrir.sh` installs only `gleip`, not the coding agent.

What it does:
1. **Port check.** Checks the listen port is free before downloading anything. The default is `127.0.0.1:47100`; pick another with `--listen` if something else uses it.
2. **Model choice.** It leaves room for your other services:
   - at most 60% of RAM
   - on Linux, at most 80% of the RAM other services leave free
   - 10 GB of disk left spare
3. **Ollama.** Installs it if it's missing. It stays on `127.0.0.1:11434` and is never exposed directly.
4. **Resource caps (Linux with systemd).** Writes `/etc/systemd/system/ollama.service.d/gleipnir.conf`:
   - half your CPUs
   - the memory budget
   - one model loaded at a time, 2 requests in parallel

   Edit that file and run `sudo systemctl restart ollama` to retune.
5. **Proxy.** Serves only the OpenAI-compatible `/v1/*` API. Ollama's admin API (pulling, deleting models) is never reachable from outside.

It runs in the foreground. Use `tmux`, or a systemd unit, to keep it running after you log out.

### API keys (`--auth`)

The first `--auth` run generates a key into `~/.config/gleipnir/keys` and prints it. The file holds one key per line, with an optional label:

```
glp_3f9a...  max
glp_81c2...  alice
```

To add a friend, append a line (`echo "glp_$(openssl rand -hex 32) bob" >> ~/.config/gleipnir/keys`) and send them their key. To remove a friend, delete their line. Changes apply immediately, with no restart.

### Exposing it to the internet (HTTPS)

fenrir speaks plain HTTP, so don't expose it raw: keys would cross the internet in clear text. Keep it on `127.0.0.1` and put your existing web server in front.

Caddy (gets certificates automatically):
```
llm.example.com {
    reverse_proxy 127.0.0.1:47100
}
```

nginx:
```nginx
location / {
    proxy_pass http://127.0.0.1:47100;
    proxy_buffering off;        # stream tokens as they arrive
    proxy_read_timeout 600s;    # slow CPU models take a while
}
```

Clients then use `https://llm.example.com/v1` in `gleip setup` → 3.

## Updating

```sh
gleip --update
```
This re-runs the installer, which updates gleip and OpenCode, with the same checksum checks as a fresh install.

## Safety defaults

When your OpenCode config has no permissions of its own, gleip adds these:
- **Shell commands:** OpenCode asks before running any, except `git status`, `git diff` and `git log`.
- **`sudo`:** always denied.
- **Files outside the project:** OpenCode asks before touching them.
- **`.env` files:** reading them is denied; `.env.example` is allowed.

If you already have permission rules, gleip leaves them alone.

## Uninstall

```sh
rm ~/.opencode/bin/gleip && opencode uninstall
# if you ran fenrir:
sudo rm /etc/systemd/system/ollama.service.d/gleipnir.conf && sudo systemctl daemon-reload && sudo systemctl restart ollama
rm -r ~/.config/gleipnir
```

## Development

```sh
go test ./...                       # model picker, config merge, proxy auth/path filtering
git tag v0.x.y && git push --tags   # CI builds all platforms and publishes the release
```

- **Model table:** local models live in [`models.sql`](models.sql), with sizes and licenses from Ollama's registry. A lower `rank` means preferred.
- **Pinned OpenCode version:** set by `OPENCODE_VERSION` in `install.sh` and `install.ps1`.
