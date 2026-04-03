#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BUNDLE_DIR="$ROOT_DIR/desktop/src-tauri/target/universal-apple-darwin/release/bundle"
APP_PATH="$(find "$BUNDLE_DIR/macos" -maxdepth 1 -name "*.app" -type d | head -n1 || true)"
DMG_PATH="$(find "$BUNDLE_DIR/dmg" -maxdepth 1 -name "*.dmg" -type f | head -n1 || true)"
NOTARY_TARGET_PATH=""

extract_notary_field() {
  local field="$1"
  local json="$2"

  python3 - "$field" "$json" <<'PY'
import json
import sys

field = sys.argv[1]
payload = sys.argv[2]
data = json.loads(payload)
value = data.get(field, "")
if isinstance(value, str):
    print(value)
PY
}

sign_binary() {
  local path="$1"

  codesign --force --options runtime --timestamp \
    --sign "$APPLE_SIGNING_IDENTITY" \
    "$path"
}

rebuild_dmg_from_signed_app() {
  local app_path="$1"
  local dmg_path="$2"
  local volume_name

  volume_name="$(basename "$app_path" .app)"
  rm -f "$dmg_path"
  hdiutil create \
    -volname "$volume_name" \
    -srcfolder "$app_path" \
    -ov \
    -format UDZO \
    -o "$dmg_path"
}

if [[ -z "$APP_PATH" ]]; then
  echo "No macOS app bundle found, skip notarization"
  exit 0
fi

if [[ -n "${APPLE_SIGNING_IDENTITY:-}" ]]; then
  echo "Code signing app with identity: $APPLE_SIGNING_IDENTITY"
  while IFS= read -r executable_path; do
    sign_binary "$executable_path"
  done < <(
    find "$APP_PATH/Contents/MacOS" "$APP_PATH/Contents/Resources/bin" \
      -type f -perm -111 2>/dev/null | sort
  )

  sign_binary "$APP_PATH"
else
  echo "APPLE_SIGNING_IDENTITY not set, skip explicit codesign"
fi

if [[ -n "$DMG_PATH" ]]; then
  rebuild_dmg_from_signed_app "$APP_PATH" "$DMG_PATH"
fi

if [[ -z "${APPLE_API_KEY_PATH:-}" || -z "${APPLE_API_KEY_ID:-}" || -z "${APPLE_API_ISSUER:-}" ]]; then
  echo "Notarization credentials are incomplete, skip notarization"
  exit 0
fi

if [[ -z "$DMG_PATH" ]]; then
  echo "No dmg found, create zip for notarization"
  NOTARY_TARGET_PATH="$ROOT_DIR/desktop/src-tauri/target/aigate-macos.zip"
  ditto -c -k --sequesterRsrc --keepParent "$APP_PATH" "$NOTARY_TARGET_PATH"
else
  NOTARY_TARGET_PATH="$DMG_PATH"
fi

submit_output="$(xcrun notarytool submit "$NOTARY_TARGET_PATH" \
  --key "$APPLE_API_KEY_PATH" \
  --key-id "$APPLE_API_KEY_ID" \
  --issuer "$APPLE_API_ISSUER" \
  --wait \
  --output-format json)"

notary_id="$(extract_notary_field id "$submit_output")"
notary_status="$(extract_notary_field status "$submit_output")"

if [[ -z "$notary_id" || -z "$notary_status" ]]; then
  echo "Unexpected notarytool response:"
  echo "$submit_output"
  exit 1
fi

if [[ "$notary_status" != "Accepted" ]]; then
  echo "Notarization failed with status: $notary_status"
  xcrun notarytool log "$notary_id" \
    --key "$APPLE_API_KEY_PATH" \
    --key-id "$APPLE_API_KEY_ID" \
    --issuer "$APPLE_API_ISSUER"
  exit 1
fi

if [[ -n "$DMG_PATH" ]]; then
  xcrun stapler staple "$DMG_PATH"
  spctl -a -t open -vv "$DMG_PATH"
else
  xcrun stapler staple "$APP_PATH"
  spctl -a -t exec -vv "$APP_PATH"
fi
