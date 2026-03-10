#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT_DIR="$ROOT_DIR/desktop/src-tauri/bin"
mkdir -p "$OUT_DIR"

WINDOWS_BIN="$OUT_DIR/routerd-x86_64-pc-windows-msvc.exe"
MACOS_PLACEHOLDER="$OUT_DIR/routerd-universal-apple-darwin"

echo "Building Go sidecar for Windows amd64..."
(
  cd "$ROOT_DIR/backend"
  CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o "$WINDOWS_BIN" ./cmd/routerd
)

if [[ ! -f "$MACOS_PLACEHOLDER" ]]; then
  : >"$MACOS_PLACEHOLDER"
fi

echo "Sidecar ready: $WINDOWS_BIN"
