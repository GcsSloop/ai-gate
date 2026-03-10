#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT_PATH="$ROOT_DIR/scripts/release/sync_release_metadata.sh"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_eq() {
  local got="$1"
  local want="$2"
  local msg="$3"
  if [[ "$got" != "$want" ]]; then
    fail "$msg (got=$got want=$want)"
  fi
}

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

mkdir -p \
  "$tmp_dir/frontend" \
  "$tmp_dir/desktop/src-tauri/icons" \
  "$tmp_dir/assets"

cat >"$tmp_dir/frontend/package.json" <<'JSON'
{
  "name": "aigate-frontend",
  "version": "1.0.1"
}
JSON

cat >"$tmp_dir/frontend/package-lock.json" <<'JSON'
{
  "name": "aigate-frontend",
  "version": "1.0.1",
  "packages": {
    "": {
      "name": "aigate-frontend",
      "version": "1.0.1"
    }
  }
}
JSON

cat >"$tmp_dir/desktop/package.json" <<'JSON'
{
  "name": "aigate-desktop",
  "version": "1.0.1"
}
JSON

cat >"$tmp_dir/desktop/package-lock.json" <<'JSON'
{
  "name": "aigate-desktop",
  "version": "1.0.1",
  "packages": {
    "": {
      "name": "aigate-desktop",
      "version": "1.0.1"
    }
  }
}
JSON

cat >"$tmp_dir/desktop/src-tauri/tauri.conf.json" <<'JSON'
{
  "version": "1.0.1",
  "bundle": {
    "icon": ["icons/icon.png", "icons/icon.icns"]
  }
}
JSON

cat >"$tmp_dir/desktop/src-tauri/Cargo.toml" <<'TOML'
[package]
name = "aigate-desktop"
version = "1.0.1"
edition = "2021"
TOML

cat >"$tmp_dir/desktop/src-tauri/Cargo.lock" <<'LOCK'
[[package]]
name = "aigate-desktop"
version = "1.0.1"
LOCK

printf 'ico-bytes' >"$tmp_dir/assets/aigate_1024_1024.ico"

bash "$SCRIPT_PATH" --root "$tmp_dir" --tag v1.0.2

assert_eq "$(node -p "require('$tmp_dir/frontend/package.json').version")" "1.0.2" "frontend package version"
assert_eq "$(node -p "require('$tmp_dir/frontend/package-lock.json').version")" "1.0.2" "frontend lock root version"
assert_eq "$(node -p "require('$tmp_dir/frontend/package-lock.json').packages[''].version")" "1.0.2" "frontend lock package version"
assert_eq "$(node -p "require('$tmp_dir/desktop/package.json').version")" "1.0.2" "desktop package version"
assert_eq "$(node -p "require('$tmp_dir/desktop/package-lock.json').version")" "1.0.2" "desktop lock root version"
assert_eq "$(node -p "require('$tmp_dir/desktop/package-lock.json').packages[''].version")" "1.0.2" "desktop lock package version"
assert_eq "$(node -p "require('$tmp_dir/desktop/src-tauri/tauri.conf.json').version")" "1.0.2" "tauri config version"
assert_eq "$(sed -n '3p' "$tmp_dir/desktop/src-tauri/Cargo.toml" | awk -F'"' '{print $2}')" "1.0.2" "cargo toml version"
assert_eq "$(grep -A1 'name = "aigate-desktop"' "$tmp_dir/desktop/src-tauri/Cargo.lock" | tail -n1 | awk -F'"' '{print $2}')" "1.0.2" "cargo lock version"

cmp "$tmp_dir/assets/aigate_1024_1024.ico" "$tmp_dir/desktop/src-tauri/icons/icon.ico" >/dev/null || \
  fail "icon.ico should match source asset"

echo "PASS: sync_release_metadata_test"
