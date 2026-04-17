#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT_DIR="$ROOT_DIR/desktop/src-tauri/bin"
mkdir -p "$OUT_DIR"

APP_VERSION="$(node -p "require('$ROOT_DIR/desktop/package.json').version" 2>/dev/null || true)"
if [[ -z "${APP_VERSION}" ]]; then
  APP_VERSION="dev"
fi
LDFLAGS="-X github.com/gcssloop/codex-router/backend/internal/buildinfo.AppVersion=${APP_VERSION}"

ARM64_BIN="$OUT_DIR/routerd-darwin-arm64"
AMD64_BIN="$OUT_DIR/routerd-darwin-amd64"
UNIVERSAL_BIN="$OUT_DIR/routerd-universal-apple-darwin"
WINDOWS_PLACEHOLDER="$OUT_DIR/routerd-x86_64-pc-windows-msvc.exe"

echo "Building Go sidecar for macOS arm64..."
(
  cd "$ROOT_DIR/backend"
  CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags "$LDFLAGS" -o "$ARM64_BIN" ./cmd/routerd
)

echo "Building Go sidecar for macOS amd64..."
(
  cd "$ROOT_DIR/backend"
  CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags "$LDFLAGS" -o "$AMD64_BIN" ./cmd/routerd
)

echo "Creating universal sidecar binary..."
lipo -create -output "$UNIVERSAL_BIN" "$ARM64_BIN" "$AMD64_BIN"
chmod +x "$UNIVERSAL_BIN"

if [[ ! -f "$WINDOWS_PLACEHOLDER" ]]; then
  : >"$WINDOWS_PLACEHOLDER"
fi

echo "Sidecar ready: $UNIVERSAL_BIN"
file "$UNIVERSAL_BIN"
