#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SOURCE_SCRIPT_PATH="$ROOT_DIR/scripts/desktop/package_local_release.sh"
HELPER_PATH="$ROOT_DIR/scripts/desktop/release_version_helpers.sh"

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

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

repo_dir="$tmp_dir/repo"
mkdir -p "$repo_dir/scripts/release" "$repo_dir/scripts/desktop" "$repo_dir/bin" "$repo_dir/desktop"
cp "$SOURCE_SCRIPT_PATH" "$repo_dir/scripts/desktop/package_local_release.sh"
cp "$HELPER_PATH" "$repo_dir/scripts/desktop/release_version_helpers.sh"

cat >"$repo_dir/scripts/release/sync_release_metadata.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'sync:%s\n' "$*" >>"$CALL_LOG"
EOF
chmod +x "$repo_dir/scripts/release/sync_release_metadata.sh"

cat >"$repo_dir/scripts/desktop/collect_release_assets.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'collect:%s\n' "${RELEASE_VERSION:-missing}" >>"$CALL_LOG"
mkdir -p "${RELEASE_ASSET_DIR:-$PWD/release-assets}"
printf 'artifact\n' >"${RELEASE_ASSET_DIR:-$PWD/release-assets}/artifact.txt"
EOF
chmod +x "$repo_dir/scripts/desktop/collect_release_assets.sh"

cat >"$repo_dir/bin/npm" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'npm:%s\n' "$*" >>"$CALL_LOG"
EOF
chmod +x "$repo_dir/bin/npm"

(
  cd "$repo_dir"
  git init >/dev/null
  git config user.name "Codex"
  git config user.email "codex@example.com"
  printf 'seed\n' >README.md
  git add README.md
  git commit -m "init" >/dev/null
  git tag v2.3.4
  printf 'next\n' >>README.md
  git add README.md
  git commit -m "next" >/dev/null

  CALL_LOG="$tmp_dir/calls.log" \
  PATH="$repo_dir/bin:$PATH" \
  RELEASE_ASSET_DIR="$tmp_dir/release-assets" \
  bash "$repo_dir/scripts/desktop/package_local_release.sh" >/dev/null
)

assert_file "$tmp_dir/release-assets/artifact.txt"
assert_contains "$tmp_dir/calls.log" "sync:--tag v2.3.4"
assert_contains "$tmp_dir/calls.log" "npm:--prefix desktop run tauri build"
assert_contains "$tmp_dir/calls.log" "collect:v2.3.4"

no_tag_dir="$tmp_dir/no-tag"
mkdir -p "$no_tag_dir/scripts/release" "$no_tag_dir/scripts/desktop" "$no_tag_dir/bin" "$no_tag_dir/desktop"
cp "$repo_dir/scripts/release/sync_release_metadata.sh" "$no_tag_dir/scripts/release/sync_release_metadata.sh"
cp "$repo_dir/scripts/desktop/collect_release_assets.sh" "$no_tag_dir/scripts/desktop/collect_release_assets.sh"
cp "$repo_dir/scripts/desktop/release_version_helpers.sh" "$no_tag_dir/scripts/desktop/release_version_helpers.sh"
cp "$repo_dir/scripts/desktop/package_local_release.sh" "$no_tag_dir/scripts/desktop/package_local_release.sh"
cp "$repo_dir/bin/npm" "$no_tag_dir/bin/npm"

(
  cd "$no_tag_dir"
  git init >/dev/null
  git config user.name "Codex"
  git config user.email "codex@example.com"
  printf 'seed\n' >README.md
  git add README.md
  git commit -m "init" >/dev/null

  if CALL_LOG="$tmp_dir/no-tag-calls.log" PATH="$no_tag_dir/bin:$PATH" bash "$no_tag_dir/scripts/desktop/package_local_release.sh" >/tmp/package-local-release.out 2>/tmp/package-local-release.err; then
    fail "package_local_release should fail without reachable tags"
  fi

  grep -Fq "No reachable git tag found" /tmp/package-local-release.err || \
    fail "expected missing tag error message"
)

echo "PASS: package_local_release_test"
