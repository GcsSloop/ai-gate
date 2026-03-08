#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BUNDLE_DIR="$ROOT_DIR/desktop/src-tauri/target/universal-apple-darwin/release/bundle"
APP_PATH="$(find "$BUNDLE_DIR/macos" -maxdepth 1 -name "*.app" -type d | head -n1 || true)"
DMG_PATH="$(find "$BUNDLE_DIR/dmg" -maxdepth 1 -name "*.dmg" -type f | head -n1 || true)"

if [[ -z "$APP_PATH" ]]; then
  echo "No macOS app bundle found, skip notarization"
  exit 0
fi

if [[ -n "${APPLE_SIGNING_IDENTITY:-}" ]]; then
  echo "Code signing app with identity: $APPLE_SIGNING_IDENTITY"
  codesign --force --deep --options runtime --timestamp \
    --sign "$APPLE_SIGNING_IDENTITY" \
    "$APP_PATH"
else
  echo "APPLE_SIGNING_IDENTITY not set, skip explicit codesign"
fi

if [[ -z "${APPLE_API_KEY_PATH:-}" || -z "${APPLE_API_KEY_ID:-}" || -z "${APPLE_API_ISSUER:-}" ]]; then
  echo "Notarization credentials are incomplete, skip notarization"
  exit 0
fi

if [[ -z "$DMG_PATH" ]]; then
  echo "No dmg found, create zip for notarization"
  ZIP_PATH="$ROOT_DIR/desktop/src-tauri/target/ccc-gateway-macos.zip"
  ditto -c -k --sequesterRsrc --keepParent "$APP_PATH" "$ZIP_PATH"
  xcrun notarytool submit "$ZIP_PATH" \
    --key "$APPLE_API_KEY_PATH" \
    --key-id "$APPLE_API_KEY_ID" \
    --issuer "$APPLE_API_ISSUER" \
    --wait
else
  xcrun notarytool submit "$DMG_PATH" \
    --key "$APPLE_API_KEY_PATH" \
    --key-id "$APPLE_API_KEY_ID" \
    --issuer "$APPLE_API_ISSUER" \
    --wait
fi

xcrun stapler staple "$APP_PATH"
if [[ -n "$DMG_PATH" ]]; then
  xcrun stapler staple "$DMG_PATH"
fi

spctl -a -t exec -vv "$APP_PATH"
