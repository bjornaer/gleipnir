#!/bin/sh
# curl -fsSL https://github.com/bjornaer/gleipnir/releases/latest/download/install.sh | sh
set -eu

OPENCODE_VERSION=2.3.0
REPO=https://github.com/bjornaer/gleipnir/releases/latest/download
BIN="$HOME/.opencode/bin" # shared with OpenCode, whose installer puts it on PATH

case "$(uname -s)" in Darwin) os=darwin ;; Linux) os=linux ;; *) echo "unsupported OS; on Windows use install.ps1" >&2; exit 1 ;; esac
case "$(uname -m)" in x86_64|amd64) arch=amd64 ;; arm64|aarch64) arch=arm64 ;; *) echo "unsupported arch: $(uname -m)" >&2; exit 1 ;; esac
asset="gleip-$os-$arch"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
curl -fsSL "$REPO/$asset" -o "$tmp/$asset"
curl -fsSL "$REPO/checksums.txt" -o "$tmp/checksums.txt"
sha="sha256sum"; command -v sha256sum >/dev/null || sha="shasum -a 256"
(cd "$tmp" && grep " $asset\$" checksums.txt | $sha -c -) >/dev/null 2>&1 \
  || { echo "checksum mismatch for $asset, aborting" >&2; exit 1; }

[ -n "${GLEIPNIR_NO_OPENCODE:-}" ] || curl -fsSL https://opencode.ai/v2/install | bash -s -- --version "$OPENCODE_VERSION"

mkdir -p "$BIN"
install -m 755 "$tmp/$asset" "$BIN/gleip"
"$BIN/gleip" --help
echo "gleip installed to $BIN. Open a new shell, then run: gleip setup"
echo "Run gleip --help anytime to see this again."
