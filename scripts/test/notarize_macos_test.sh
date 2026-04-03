#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SOURCE_SCRIPT="$ROOT_DIR/scripts/desktop/notarize_macos.sh"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_contains() {
  local file="$1"
  local pattern="$2"
  if ! grep -Fq -- "$pattern" "$file"; then
    fail "expected $file to contain: $pattern"
  fi
}

assert_not_contains() {
  local file="$1"
  local pattern="$2"
  if grep -Fq -- "$pattern" "$file"; then
    fail "expected $file to not contain: $pattern"
  fi
}

make_repo() {
  local repo_dir="$1"
  mkdir -p "$repo_dir/scripts/desktop" \
    "$repo_dir/desktop/src-tauri/target/universal-apple-darwin/release/bundle/macos" \
    "$repo_dir/desktop/src-tauri/target/universal-apple-darwin/release/bundle/dmg" \
    "$repo_dir/assets"
  cp "$SOURCE_SCRIPT" "$repo_dir/scripts/desktop/notarize_macos.sh"
  chmod +x "$repo_dir/scripts/desktop/notarize_macos.sh"
  mkdir -p "$repo_dir/desktop/src-tauri/target/universal-apple-darwin/release/bundle/macos/AI Gate.app/Contents/MacOS"
  mkdir -p "$repo_dir/desktop/src-tauri/target/universal-apple-darwin/release/bundle/macos/AI Gate.app/Contents/Resources/bin"
  printf '#!/usr/bin/env bash\nexit 0\n' >"$repo_dir/desktop/src-tauri/target/universal-apple-darwin/release/bundle/macos/AI Gate.app/Contents/MacOS/aigate-desktop"
  printf '#!/usr/bin/env bash\nexit 0\n' >"$repo_dir/desktop/src-tauri/target/universal-apple-darwin/release/bundle/macos/AI Gate.app/Contents/Resources/bin/routerd-universal-apple-darwin"
  chmod +x \
    "$repo_dir/desktop/src-tauri/target/universal-apple-darwin/release/bundle/macos/AI Gate.app/Contents/MacOS/aigate-desktop" \
    "$repo_dir/desktop/src-tauri/target/universal-apple-darwin/release/bundle/macos/AI Gate.app/Contents/Resources/bin/routerd-universal-apple-darwin"
  : > "$repo_dir/desktop/src-tauri/target/universal-apple-darwin/release/bundle/dmg/AI Gate.dmg"
  printf '%s' 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+XWZ0AAAAASUVORK5CYII=' | base64 --decode >"$repo_dir/assets/dmg-bg.png"
}

make_fake_bin() {
  local bin_dir="$1"
  mkdir -p "$bin_dir"

  cat >"$bin_dir/codesign" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'codesign:%s\n' "$*" >>"$CALL_LOG"
EOF
  chmod +x "$bin_dir/codesign"

  cat >"$bin_dir/spctl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'spctl:%s\n' "$*" >>"$CALL_LOG"
EOF
  chmod +x "$bin_dir/spctl"

  cat >"$bin_dir/ditto" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'ditto:%s\n' "$*" >>"$CALL_LOG"
EOF
  chmod +x "$bin_dir/ditto"

  cat >"$bin_dir/sips" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'sips:%s\n' "$*" >>"$CALL_LOG"
EOF
  chmod +x "$bin_dir/sips"

  cat >"$bin_dir/hdiutil" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'hdiutil:%s\n' "$*" >>"$CALL_LOG"
out=""
prev=""
for arg in "$@"; do
  if [[ "$prev" == "-o" ]]; then
    out="$arg"
    break
  fi
  prev="$arg"
done
if [[ -n "$out" ]]; then
  : >"$out"
fi
EOF
  chmod +x "$bin_dir/hdiutil"

  cat >"$bin_dir/xcrun" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'xcrun:%s\n' "$*" >>"$CALL_LOG"

if [[ "$1" == "notarytool" && "$2" == "submit" ]]; then
  case "${NOTARY_STATUS:-Accepted}" in
    Accepted)
      printf '{"id":"submission-123","status":"Accepted"}\n'
      ;;
    Invalid)
      printf '{"id":"submission-123","status":"Invalid"}\n'
      ;;
    *)
      printf '{"id":"submission-123","status":"%s"}\n' "${NOTARY_STATUS}"
      ;;
  esac
  exit 0
fi

if [[ "$1" == "notarytool" && "$2" == "log" ]]; then
  printf 'notary-log:%s\n' "$*" >>"$CALL_LOG"
  printf '{"issues":[{"message":"mock invalid"}]}\n'
  exit 0
fi

if [[ "$1" == "stapler" && "$2" == "staple" ]]; then
  printf 'stapler:%s\n' "$3" >>"$CALL_LOG"
  exit 0
fi

fail() {
  echo "unexpected xcrun invocation: $*" >&2
  exit 1
}

fail "$@"
EOF
  chmod +x "$bin_dir/xcrun"
}

make_fake_bundle_dmg_script() {
  local repo_dir="$1"
  local bundle_dir="${2:-$repo_dir/desktop/src-tauri/target/universal-apple-darwin/release/bundle/dmg}"
  mkdir -p "$bundle_dir"
cat >"$bundle_dir/bundle_dmg.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'bundle_dmg:%s\n' "$*" >>"$CALL_LOG"
argc=$#
eval "out=\${$((argc-1))}"
: >"$out"
EOF
  chmod +x "$bundle_dir/bundle_dmg.sh"
}

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

repo_invalid="$tmp_dir/repo-invalid"
bin_invalid="$tmp_dir/bin-invalid"
log_invalid="$tmp_dir/invalid.log"
make_repo "$repo_invalid"
make_fake_bin "$bin_invalid"

if (
  cd "$repo_invalid"
  CALL_LOG="$log_invalid" \
  PATH="$bin_invalid:$PATH" \
  APPLE_SIGNING_IDENTITY="Developer ID Application: Example Developer (TEAMID1234)" \
  APPLE_API_KEY_PATH="$tmp_dir/AuthKey.p8" \
  APPLE_API_KEY_ID="ABC123DEFG" \
  APPLE_API_ISSUER="00000000-0000-0000-0000-000000000000" \
  NOTARY_STATUS="Invalid" \
  bash scripts/desktop/notarize_macos.sh
); then
  fail "notarize_macos.sh should fail when notary status is Invalid"
fi

assert_contains "$log_invalid" "xcrun:notarytool submit"
assert_contains "$log_invalid" "xcrun:notarytool log submission-123"
assert_not_contains "$log_invalid" "stapler:"

repo_accept="$tmp_dir/repo-accept"
bin_accept="$tmp_dir/bin-accept"
log_accept="$tmp_dir/accept.log"
make_repo "$repo_accept"
make_fake_bin "$bin_accept"

(
  cd "$repo_accept"
  CALL_LOG="$log_accept" \
  PATH="$bin_accept:$PATH" \
  APPLE_SIGNING_IDENTITY="Developer ID Application: Example Developer (TEAMID1234)" \
  APPLE_API_KEY_PATH="$tmp_dir/AuthKey.p8" \
  APPLE_API_KEY_ID="ABC123DEFG" \
  APPLE_API_ISSUER="00000000-0000-0000-0000-000000000000" \
  NOTARY_STATUS="Accepted" \
  bash scripts/desktop/notarize_macos.sh
)

assert_contains "$log_accept" "xcrun:notarytool submit"
assert_contains "$log_accept" "hdiutil:create -volname AI Gate Installer -srcfolder $repo_accept/desktop/src-tauri/target/universal-apple-darwin/release/bundle/macos/AI Gate.app -ov -format UDZO -o $repo_accept/desktop/src-tauri/target/universal-apple-darwin/release/bundle/dmg/AI Gate.dmg"
assert_contains "$log_accept" "codesign:--force --timestamp --sign Developer ID Application: Example Developer (TEAMID1234) $repo_accept/desktop/src-tauri/target/universal-apple-darwin/release/bundle/dmg/AI Gate.dmg"
assert_contains "$log_accept" "stapler:$repo_accept/desktop/src-tauri/target/universal-apple-darwin/release/bundle/dmg/AI Gate.dmg"
assert_not_contains "$log_accept" "stapler:$repo_accept/desktop/src-tauri/target/universal-apple-darwin/release/bundle/macos/AI Gate.app"
assert_contains "$log_accept" "spctl:-a -t open --context context:primary-signature -vv $repo_accept/desktop/src-tauri/target/universal-apple-darwin/release/bundle/dmg/AI Gate.dmg"
assert_contains "$log_accept" "codesign:--force --options runtime --timestamp --sign Developer ID Application: Example Developer (TEAMID1234) $repo_accept/desktop/src-tauri/target/universal-apple-darwin/release/bundle/macos/AI Gate.app/Contents/MacOS/aigate-desktop"
assert_contains "$log_accept" "codesign:--force --options runtime --timestamp --sign Developer ID Application: Example Developer (TEAMID1234) $repo_accept/desktop/src-tauri/target/universal-apple-darwin/release/bundle/macos/AI Gate.app/Contents/Resources/bin/routerd-universal-apple-darwin"
assert_contains "$log_accept" "codesign:--force --options runtime --timestamp --sign Developer ID Application: Example Developer (TEAMID1234) $repo_accept/desktop/src-tauri/target/universal-apple-darwin/release/bundle/macos/AI Gate.app"
assert_not_contains "$log_accept" "codesign:--force --deep --options runtime --timestamp"

repo_bundle="$tmp_dir/repo-bundle"
bin_bundle="$tmp_dir/bin-bundle"
log_bundle="$tmp_dir/bundle.log"
make_repo "$repo_bundle"
make_fake_bin "$bin_bundle"
make_fake_bundle_dmg_script "$repo_bundle"

(
  cd "$repo_bundle"
  CALL_LOG="$log_bundle" \
  PATH="$bin_bundle:$PATH" \
  APPLE_SIGNING_IDENTITY="Developer ID Application: Example Developer (TEAMID1234)" \
  APPLE_API_KEY_PATH="$tmp_dir/AuthKey.p8" \
  APPLE_API_KEY_ID="ABC123DEFG" \
  APPLE_API_ISSUER="00000000-0000-0000-0000-000000000000" \
  NOTARY_STATUS="Accepted" \
  bash scripts/desktop/notarize_macos.sh
)

assert_contains "$log_bundle" "bundle_dmg:--volname AI Gate Installer --background"
assert_contains "$log_bundle" "--window-pos 120 120 --window-size 800 560"
assert_contains "$log_bundle" "--icon-size 128 --text-size 16"
assert_contains "$log_bundle" "--icon AI Gate.app 170 275"
assert_contains "$log_bundle" "--app-drop-link 630 275"
assert_not_contains "$log_bundle" "hdiutil:create"

repo_native="$tmp_dir/repo-native"
bin_native="$tmp_dir/bin-native"
log_native="$tmp_dir/native.log"
make_repo "$repo_native"
make_fake_bin "$bin_native"

rm -rf "$repo_native/desktop/src-tauri/target/universal-apple-darwin"
mkdir -p "$repo_native/desktop/src-tauri/target/release/bundle/macos/AI Gate.app/Contents/MacOS"
mkdir -p "$repo_native/desktop/src-tauri/target/release/bundle/macos/AI Gate.app/Contents/Resources/bin"
mkdir -p "$repo_native/desktop/src-tauri/target/release/bundle/dmg"
make_fake_bundle_dmg_script "$repo_native" "$repo_native/desktop/src-tauri/target/release/bundle/dmg"
printf '#!/usr/bin/env bash\nexit 0\n' >"$repo_native/desktop/src-tauri/target/release/bundle/macos/AI Gate.app/Contents/MacOS/aigate-desktop"
printf '#!/usr/bin/env bash\nexit 0\n' >"$repo_native/desktop/src-tauri/target/release/bundle/macos/AI Gate.app/Contents/Resources/bin/routerd-universal-apple-darwin"
chmod +x \
  "$repo_native/desktop/src-tauri/target/release/bundle/macos/AI Gate.app/Contents/MacOS/aigate-desktop" \
  "$repo_native/desktop/src-tauri/target/release/bundle/macos/AI Gate.app/Contents/Resources/bin/routerd-universal-apple-darwin"
: > "$repo_native/desktop/src-tauri/target/release/bundle/dmg/AI Gate.dmg"

(
  cd "$repo_native"
  CALL_LOG="$log_native" \
  PATH="$bin_native:$PATH" \
  bash scripts/desktop/notarize_macos.sh
)

assert_contains "$log_native" "bundle_dmg:--volname AI Gate Installer --background"
assert_contains "$log_native" "--icon AI Gate.app 170 275"
assert_contains "$log_native" "--app-drop-link 630 275"
assert_not_contains "$log_native" "No macOS app bundle found"

repo_no_dmg="$tmp_dir/repo-no-dmg"
bin_no_dmg="$tmp_dir/bin-no-dmg"
log_no_dmg="$tmp_dir/no-dmg.log"
make_repo "$repo_no_dmg"
make_fake_bin "$bin_no_dmg"
make_fake_bundle_dmg_script "$repo_no_dmg"
rm -f "$repo_no_dmg/desktop/src-tauri/target/universal-apple-darwin/release/bundle/dmg/AI Gate.dmg"

(
  cd "$repo_no_dmg"
  CALL_LOG="$log_no_dmg" \
  PATH="$bin_no_dmg:$PATH" \
  bash scripts/desktop/notarize_macos.sh
)

assert_contains "$log_no_dmg" "bundle_dmg:--volname AI Gate Installer --background"
assert_contains "$log_no_dmg" "$repo_no_dmg/desktop/src-tauri/target/universal-apple-darwin/release/bundle/dmg/AI Gate.dmg"
assert_not_contains "$log_no_dmg" "No dmg found, create zip for notarization"
echo "PASS: notarize_macos_test"
