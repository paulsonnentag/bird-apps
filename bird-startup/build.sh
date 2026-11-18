#!/usr/bin/env bash
# Builds dist/BirdStartup.exe — a single self-installing Windows executable.
# The xeno-canto API key is read from $XC_API_KEY or ./xenocanto.key (gitignored)
# and baked into the binary (XOR-obfuscated, not secret against a determined reader).
set -euo pipefail
cd "$(dirname "$0")"
KEY="${XC_API_KEY:-$(tr -d '[:space:]' < xenocanto.key 2>/dev/null || true)}"
if [[ -z "$KEY" ]]; then
  echo "error: set XC_API_KEY or create bird-startup/xenocanto.key" >&2
  exit 1
fi
# Obfuscate: XOR the key with a fresh random pad each build.
read -r KEYDATA KEYPAD < <(python3 -c '
import os, sys
k = sys.argv[1].encode(); p = os.urandom(len(k))
print(bytes(a ^ b for a, b in zip(k, p)).hex(), p.hex())' "$KEY")
mkdir -p dist
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -H=windowsgui -X main.xcKeyData=$KEYDATA -X main.xcKeyPad=$KEYPAD" \
  -o dist/BirdStartup.exe .
echo "built $(pwd)/dist/BirdStartup.exe ($(du -h dist/BirdStartup.exe | cut -f1))"
