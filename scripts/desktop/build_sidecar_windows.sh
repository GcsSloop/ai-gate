#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT_DIR="$ROOT_DIR/desktop/src-tauri/bin"
mkdir -p "$OUT_DIR"

resolve_app_version() {
  local raw="${RELEASE_VERSION:-}"
  local normalized
  if [[ -n "$raw" ]]; then
    normalized="${raw#v}"
    normalized="${normalized#V}"
    printf '%s\n' "$normalized"
    return 0
  fi

  local tauri_conf="$ROOT_DIR/desktop/src-tauri/tauri.conf.json"
  if [[ -f "$tauri_conf" ]]; then
    normalized="$(node -e 'const fs = require("fs"); const f = process.argv[1]; const value = JSON.parse(fs.readFileSync(f, "utf8")).version || ""; process.stdout.write(String(value));' "$tauri_conf" 2>/dev/null || true)"
    normalized="$(printf '%s' "$normalized" | tr -d '\r\n[:space:]')"
    if [[ -n "$normalized" ]]; then
      printf '%s\n' "$normalized"
      return 0
    fi
  fi

  local desktop_pkg="$ROOT_DIR/desktop/package.json"
  if [[ -f "$desktop_pkg" ]]; then
    normalized="$(node -e 'const fs = require("fs"); const f = process.argv[1]; const value = JSON.parse(fs.readFileSync(f, "utf8")).version || ""; process.stdout.write(String(value));' "$desktop_pkg" 2>/dev/null || true)"
    normalized="$(printf '%s' "$normalized" | tr -d '\r\n[:space:]')"
    if [[ -n "$normalized" ]]; then
      printf '%s\n' "$normalized"
      return 0
    fi
  fi

  return 1
}

if ! APP_VERSION="$(resolve_app_version)"; then
  echo "Failed to resolve AppVersion for Windows sidecar build. Set RELEASE_VERSION or ensure desktop metadata version exists." >&2
  exit 1
fi
LDFLAGS="-X github.com/gcssloop/codex-router/backend/internal/buildinfo.AppVersion=${APP_VERSION}"

WINDOWS_BIN="$OUT_DIR/routerd-x86_64-pc-windows-msvc.exe"
MACOS_PLACEHOLDER="$OUT_DIR/routerd-universal-apple-darwin"

echo "Building Go sidecar for Windows amd64..."
(
  cd "$ROOT_DIR/backend"
  CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "$LDFLAGS" -o "$WINDOWS_BIN" ./cmd/routerd
)

if [[ ! -f "$MACOS_PLACEHOLDER" ]]; then
  : >"$MACOS_PLACEHOLDER"
fi

echo "Sidecar ready: $WINDOWS_BIN"
