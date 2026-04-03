#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TARGET_DIR="${TARGET_DIR:-$ROOT_DIR/desktop/src-tauri/target}"
NOTARY_TARGET_PATH=""
DMG_BACKGROUND_SOURCE="$ROOT_DIR/assets/dmg-bg.png"
DMG_VOLUME_NAME="AI Gate Installer"
DMG_WINDOW_WIDTH=800
DMG_WINDOW_HEIGHT=560
DMG_WINDOW_POS_X=120
DMG_WINDOW_POS_Y=120
DMG_ICON_SIZE=128
DMG_TEXT_SIZE=16
DMG_APP_ICON_X=170
DMG_APP_ICON_Y=275
DMG_APPLICATIONS_ICON_X=630
DMG_APPLICATIONS_ICON_Y=275
APPDMG_BIN="$ROOT_DIR/desktop/node_modules/.bin/appdmg"

resolve_bundle_dir() {
  local universal_dir="$TARGET_DIR/universal-apple-darwin/release/bundle"
  local native_dir="$TARGET_DIR/release/bundle"

  if [[ -d "$universal_dir" ]]; then
    printf '%s\n' "$universal_dir"
    return 0
  fi
  if [[ -d "$native_dir" ]]; then
    printf '%s\n' "$native_dir"
    return 0
  fi

  return 1
}

BUNDLE_DIR="$(resolve_bundle_dir || true)"
APP_PATH=""
DMG_PATH=""
if [[ -n "$BUNDLE_DIR" ]]; then
  APP_PATH="$(find "$BUNDLE_DIR/macos" -maxdepth 1 -name "*.app" -type d | head -n1 || true)"
  DMG_PATH="$(find "$BUNDLE_DIR/dmg" -maxdepth 1 -name "*.dmg" -type f | head -n1 || true)"
  if [[ -z "$DMG_PATH" && -n "$APP_PATH" ]]; then
    mkdir -p "$BUNDLE_DIR/dmg"
    DMG_PATH="$BUNDLE_DIR/dmg/$(basename "${APP_PATH%.app}").dmg"
  fi
fi

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

prepare_dmg_background() {
  local source_path="$1"
  local output_path="$2"

  cp "$source_path" "$output_path"

  if command -v sips >/dev/null 2>&1; then
    sips --resampleHeight "$DMG_WINDOW_HEIGHT" "$output_path" >/dev/null
    sips --cropToHeightWidth "$DMG_WINDOW_HEIGHT" "$DMG_WINDOW_WIDTH" "$output_path" >/dev/null
  fi
}

rebuild_dmg_from_signed_app() {
  local app_path="$1"
  local dmg_path="$2"
  local stage_dir
  local background_path
  local app_name
  local spec_path
  app_name="$(basename "$app_path")"

  rm -f "$dmg_path"

  if [[ -x "$APPDMG_BIN" && -f "$DMG_BACKGROUND_SOURCE" ]]; then
    stage_dir="$(mktemp -d)"
    mkdir -p "$stage_dir/.background"
    background_path="$stage_dir/.background/dmg-bg.png"
    spec_path="$stage_dir/appdmg.json"
    prepare_dmg_background "$DMG_BACKGROUND_SOURCE" "$background_path"
    cp -R "$app_path" "$stage_dir/$app_name"

    cat >"$spec_path" <<EOF
{
  "title": "$DMG_VOLUME_NAME",
  "background": ".background/dmg-bg.png",
  "icon-size": $DMG_ICON_SIZE,
  "window": {
    "position": { "x": $DMG_WINDOW_POS_X, "y": $DMG_WINDOW_POS_Y },
    "size": { "width": $DMG_WINDOW_WIDTH, "height": $DMG_WINDOW_HEIGHT }
  },
  "format": "UDZO",
  "contents": [
    { "x": $DMG_APP_ICON_X, "y": $DMG_APP_ICON_Y, "type": "file", "path": "$app_name" },
    { "x": $DMG_APPLICATIONS_ICON_X, "y": $DMG_APPLICATIONS_ICON_Y, "type": "link", "path": "/Applications" }
  ]
}
EOF

    "$APPDMG_BIN" "$spec_path" "$dmg_path"

    rm -rf "$stage_dir"
    return
  fi

  echo "appdmg unavailable or background missing; falling back to simple DMG layout" >&2
  hdiutil create \
    -volname "$DMG_VOLUME_NAME" \
    -srcfolder "$app_path" \
    -ov \
    -format UDZO \
    -o "$dmg_path"
}

sign_dmg() {
  local path="$1"

  codesign --force --timestamp \
    --sign "$APPLE_SIGNING_IDENTITY" \
    "$path"
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
  if [[ -n "${APPLE_SIGNING_IDENTITY:-}" ]]; then
    sign_dmg "$DMG_PATH"
  fi
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
  spctl -a -t open --context context:primary-signature -vv "$DMG_PATH"
else
  xcrun stapler staple "$APP_PATH"
  spctl -a -t exec -vv "$APP_PATH"
fi
