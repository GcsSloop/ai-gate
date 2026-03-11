#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT_PATH="$ROOT_DIR/scripts/desktop/collect_release_assets.sh"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_file() {
  local path="$1"
  [[ -f "$path" ]] || fail "expected file: $path"
}

assert_contains() {
  local file="$1"
  local pattern="$2"
  if ! grep -Fq "$pattern" "$file"; then
    fail "expected $file to contain $pattern"
  fi
}

create_git_repo_with_tag() {
  local repo_dir="$1"
  mkdir -p "$repo_dir"
  (
    cd "$repo_dir"
    git init >/dev/null
    git config user.name "Codex"
    git config user.email "codex@example.com"
    printf 'seed\n' >README.md
    git add README.md
    git commit -m "init" >/dev/null
    git tag v9.8.7
  )
}

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

target_dir="$tmp_dir/target"
sidecar_dir="$tmp_dir/bin"
mkdir -p "$target_dir/universal-apple-darwin/release/bundle/macos/AI Gate.app/Contents/MacOS"
mkdir -p "$target_dir/universal-apple-darwin/release/bundle/dmg"
mkdir -p "$target_dir/x86_64-pc-windows-msvc/release/bundle/msi"
mkdir -p "$target_dir/x86_64-pc-windows-msvc/release"
mkdir -p "$sidecar_dir"

printf 'mac app\n' >"$target_dir/universal-apple-darwin/release/bundle/macos/AI Gate.app/Contents/MacOS/aigate-desktop"
printf 'dmg\n' >"$target_dir/universal-apple-darwin/release/bundle/dmg/AI Gate_1.2.3_universal.dmg"
printf 'msi\n' >"$target_dir/x86_64-pc-windows-msvc/release/bundle/msi/AI Gate_1.2.3_x64_en-US.msi"
printf 'windows exe\n' >"$target_dir/x86_64-pc-windows-msvc/release/aigate-desktop.exe"
printf 'sidecar exe\n' >"$sidecar_dir/routerd-x86_64-pc-windows-msvc.exe"

mac_out="$tmp_dir/release-macos"
RELEASE_PLATFORM=macos \
RELEASE_VERSION=v1.2.3 \
TARGET_DIR="$target_dir" \
RELEASE_ASSET_DIR="$mac_out" \
SIDECAR_BIN_DIR="$sidecar_dir" \
bash "$SCRIPT_PATH" >/dev/null

assert_file "$mac_out/aigate-v1.2.3-macOS.zip"
assert_file "$mac_out/aigate-v1.2.3-macOS.dmg"
assert_file "$mac_out/aigate-v1.2.3-macos-SHA256SUMS.txt"
assert_contains "$mac_out/aigate-v1.2.3-macos-SHA256SUMS.txt" "aigate-v1.2.3-macOS.zip"
assert_contains "$mac_out/aigate-v1.2.3-macos-SHA256SUMS.txt" "aigate-v1.2.3-macOS.dmg"

windows_out="$tmp_dir/release-windows"
RELEASE_PLATFORM=windows \
RELEASE_VERSION=v1.2.3 \
TARGET_DIR="$target_dir" \
RELEASE_ASSET_DIR="$windows_out" \
SIDECAR_BIN_DIR="$sidecar_dir" \
APP_EXECUTABLE_NAME=aigate-desktop.exe \
bash "$SCRIPT_PATH" >/dev/null

assert_file "$windows_out/aigate-v1.2.3-windows.msi"
assert_file "$windows_out/aigate-v1.2.3-windows.zip"
assert_file "$windows_out/aigate-v1.2.3-windows-SHA256SUMS.txt"
assert_contains "$windows_out/aigate-v1.2.3-windows-SHA256SUMS.txt" "aigate-v1.2.3-windows.msi"
assert_contains "$windows_out/aigate-v1.2.3-windows-SHA256SUMS.txt" "aigate-v1.2.3-windows.zip"

if command -v unzip >/dev/null 2>&1; then
  zip_listing="$(unzip -l "$windows_out/aigate-v1.2.3-windows.zip")"
  [[ "$zip_listing" == *"aigate-v1.2.3-windows/aigate-desktop.exe"* ]] || \
    fail "portable zip should include desktop executable"
  [[ "$zip_listing" == *"aigate-v1.2.3-windows/bin/routerd-x86_64-pc-windows-msvc.exe"* ]] || \
    fail "portable zip should include sidecar binary"
fi

tag_repo="$tmp_dir/tagged-repo"
create_git_repo_with_tag "$tag_repo"

fallback_out="$tmp_dir/release-fallback"
(
  cd "$tag_repo"
  RELEASE_PLATFORM=macos \
  TARGET_DIR="$target_dir" \
  RELEASE_ASSET_DIR="$fallback_out" \
  SIDECAR_BIN_DIR="$sidecar_dir" \
  bash "$SCRIPT_PATH" >/dev/null
)

assert_file "$fallback_out/aigate-v9.8.7-macOS.zip"
assert_file "$fallback_out/aigate-v9.8.7-macOS.dmg"
assert_file "$fallback_out/aigate-v9.8.7-macos-SHA256SUMS.txt"

echo "PASS: collect_release_assets_test"
