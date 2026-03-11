#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT_PATH="$ROOT_DIR/scripts/release/generate_updater_manifest.sh"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_contains() {
  local file="$1"
  local pattern="$2"
  if ! grep -Fq "$pattern" "$file"; then
    fail "expected $file to contain $pattern"
  fi
}

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

mkdir -p "$tmp_dir/release-assets-macos" "$tmp_dir/release-assets-windows"
printf 'mac updater\n' >"$tmp_dir/release-assets-macos/aigate-v1.2.3-darwin-universal.app.tar.gz"
printf 'mac sig line\n' >"$tmp_dir/release-assets-macos/aigate-v1.2.3-darwin-universal.app.tar.gz.sig"
printf 'windows installer\n' >"$tmp_dir/release-assets-windows/aigate-v1.2.3-windows.msi"
printf 'windows sig line\n' >"$tmp_dir/release-assets-windows/aigate-v1.2.3-windows.msi.sig"

bash "$SCRIPT_PATH" \
  --tag v1.2.3 \
  --repo GcsSloop/ai-gate \
  --assets-root "$tmp_dir" \
  --output "$tmp_dir/latest.json" \
  --notes "hello release"

assert_contains "$tmp_dir/latest.json" '"version": "1.2.3"'
assert_contains "$tmp_dir/latest.json" '"notes": "hello release"'
assert_contains "$tmp_dir/latest.json" '"darwin-aarch64"'
assert_contains "$tmp_dir/latest.json" '"darwin-x86_64"'
assert_contains "$tmp_dir/latest.json" '"windows-x86_64"'
assert_contains "$tmp_dir/latest.json" 'https://github.com/GcsSloop/ai-gate/releases/download/v1.2.3/aigate-v1.2.3-darwin-universal.app.tar.gz'
assert_contains "$tmp_dir/latest.json" 'https://github.com/GcsSloop/ai-gate/releases/download/v1.2.3/aigate-v1.2.3-windows.msi'
assert_contains "$tmp_dir/latest.json" 'mac sig line'
assert_contains "$tmp_dir/latest.json" 'windows sig line'

echo "PASS: release_updater_manifest_test"
