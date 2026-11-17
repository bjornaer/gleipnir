#!/bin/sh
# Serve a model for gleipnir clients without installing the coding agent:
#   curl -fsSL https://github.com/bjornaer/gleipnir/releases/latest/download/fenrir.sh | sh -s -- --auth --listen 0.0.0.0:47100
# Flags: --auth (require API keys), --listen ADDR (default 127.0.0.1:47100), --model TAG (default: best fit)
set -eu
curl -fsSL https://github.com/bjornaer/gleipnir/releases/latest/download/install.sh | GLEIPNIR_NO_OPENCODE=1 sh
exec "$HOME/.opencode/bin/gleip" fenrir "$@"
