#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
HELPER_PATH="$ROOT_DIR/scripts/desktop/release_version_helpers.sh"

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

repo_dir="$tmp_dir/repo"
mkdir -p "$repo_dir"
(
  cd "$repo_dir"
  git init >/dev/null
  git config user.name "Codex"
  git config user.email "codex@example.com"
  printf 'one\n' >README.md
  git add README.md
  git commit -m "init" >/dev/null
  git tag v1.2.3
  printf 'two\n' >>README.md
  git add README.md
  git commit -m "next" >/dev/null

  resolved_tag="$(bash -lc 'source "'"$HELPER_PATH"'"; resolve_release_tag')"
  resolved_version="$(bash -lc 'source "'"$HELPER_PATH"'"; resolve_release_version')"
  explicit_version="$(RELEASE_VERSION=v8.8.8 bash -lc 'source "'"$HELPER_PATH"'"; resolve_release_version')"

  assert_eq "$resolved_tag" "v1.2.3" "reachable tag"
  assert_eq "$resolved_version" "v1.2.3" "resolved version"
  assert_eq "$explicit_version" "v8.8.8" "explicit version wins"
)

no_tag_dir="$tmp_dir/no-tag"
mkdir -p "$no_tag_dir"
(
  cd "$no_tag_dir"
  git init >/dev/null
  git config user.name "Codex"
  git config user.email "codex@example.com"
  printf 'seed\n' >README.md
  git add README.md
  git commit -m "init" >/dev/null

  if bash -lc 'source "'"$HELPER_PATH"'"; resolve_release_tag' >/tmp/release-version-helper.out 2>/tmp/release-version-helper.err; then
    fail "resolve_release_tag should fail without reachable tags"
  fi

  grep -Fq "No reachable git tag found" /tmp/release-version-helper.err || \
    fail "expected missing tag error message"
)

echo "PASS: release_version_helpers_test"
