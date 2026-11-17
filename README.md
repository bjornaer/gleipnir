# gleipnir

A coding agent you point at your own model: your ChatGPT plan or API key, a local model, or a gleipnir endpoint. It's [OpenCode](https://opencode.ai) with an installer, a model picker, and cautious permission defaults.

```sh
curl -fsSL https://github.com/bjornaer/gleipnir/releases/latest/download/install.sh | sh    # macOS / Linux
irm https://github.com/bjornaer/gleipnir/releases/latest/download/install.ps1 | iex         # Windows
```

```sh
gleip setup   # choose: subscription/API key, local model (Ollama), or gleipnir endpoint
gleip         # start coding
```

- **Subscriptions:** ChatGPT Plus/Pro works through Sign in with ChatGPT. Claude Pro/Max can't: Anthropic's terms don't allow subscriptions in third-party tools, so use an Anthropic API key.
- **Local models:** `gleip setup` reads your RAM/VRAM and picks the best fit from `models.sql`.
- **Permissions:** shell commands ask first, `sudo` and `.env` reads are denied. Only applied when your OpenCode config has no permissions of its own.
