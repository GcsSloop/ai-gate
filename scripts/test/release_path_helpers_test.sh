#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=scripts/desktop/release_path_helpers.sh
source "$ROOT_DIR/scripts/desktop/release_path_helpers.sh"

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

assert_eq "$(to_windows_path "/d/a/ai-gate/release-assets/out.zip")" \
  'D:\a\ai-gate\release-assets\out.zip' \
  "git-bash windows path conversion"

assert_eq "$(to_windows_path "relative/path/file.zip")" \
  'relative\path\file.zip' \
  "relative path conversion"

echo "PASS: release_path_helpers_test"
