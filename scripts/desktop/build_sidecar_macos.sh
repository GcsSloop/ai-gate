#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT_DIR="$ROOT_DIR/desktop/src-tauri/bin"
mkdir -p "$OUT_DIR"

ARM64_BIN="$OUT_DIR/routerd-darwin-arm64"
AMD64_BIN="$OUT_DIR/routerd-darwin-amd64"
UNIVERSAL_BIN="$OUT_DIR/routerd-universal-apple-darwin"

echo "Building Go sidecar for macOS arm64..."
(
  cd "$ROOT_DIR/backend"
  CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o "$ARM64_BIN" ./cmd/routerd
)

echo "Building Go sidecar for macOS amd64..."
(
  cd "$ROOT_DIR/backend"
  CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o "$AMD64_BIN" ./cmd/routerd
)

echo "Creating universal sidecar binary..."
lipo -create -output "$UNIVERSAL_BIN" "$ARM64_BIN" "$AMD64_BIN"
chmod +x "$UNIVERSAL_BIN"

echo "Sidecar ready: $UNIVERSAL_BIN"
file "$UNIVERSAL_BIN"
