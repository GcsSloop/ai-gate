#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
VERSION="${CI_COMMIT_TAG:-local}"
SRC_DIR="$ROOT_DIR/desktop/src-tauri/target/universal-apple-darwin/release/bundle"
OUT_DIR="$ROOT_DIR/release-assets"

mkdir -p "$OUT_DIR"

APP_PATH="$(find "$SRC_DIR/macos" -maxdepth 1 -name "*.app" -type d | head -n1 || true)"
DMG_PATH="$(find "$SRC_DIR/dmg" -maxdepth 1 -name "*.dmg" -type f | head -n1 || true)"

if [[ -n "$APP_PATH" ]]; then
  APP_ZIP="$OUT_DIR/ccc-gateway-${VERSION}-macOS.zip"
  ditto -c -k --sequesterRsrc --keepParent "$APP_PATH" "$APP_ZIP"
fi

if [[ -n "$DMG_PATH" ]]; then
  cp "$DMG_PATH" "$OUT_DIR/ccc-gateway-${VERSION}-macOS.dmg"
fi

(
  cd "$OUT_DIR"
  shasum -a 256 * > SHA256SUMS
)

ls -la "$OUT_DIR"
