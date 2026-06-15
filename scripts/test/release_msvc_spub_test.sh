#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RELEASE_WORKFLOW="$ROOT_DIR/.github/workflows/release.yml"
DEV_CI_WORKFLOW="$ROOT_DIR/.github/workflows/dev-ci.yml"

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

assert_contains "$DEV_CI_WORKFLOW" "bash scripts/test/build_ai_gate_msvc_test.sh"
assert_contains "$RELEASE_WORKFLOW" "build-server-plugin:"
assert_contains "$RELEASE_WORKFLOW" "bash scripts/release/build_ai_gate_msvc.sh"
assert_contains "$RELEASE_WORKFLOW" "--target linux/amd64"
assert_contains "$RELEASE_WORKFLOW" "release-assets-msvc"
assert_contains "$RELEASE_WORKFLOW" "npm install -g spub"
assert_contains "$RELEASE_WORKFLOW" "SANSI_CLOUD_PLUGIN_TOKEN: \${{ vars.SANSI_CLOUD_PLUGIN_TOKEN || secrets.SANSI_CLOUD_PLUGIN_TOKEN }}"
assert_contains "$RELEASE_WORKFLOW" "spub -f"
assert_contains "$RELEASE_WORKFLOW" "-t \"\$SANSI_CLOUD_PLUGIN_TOKEN\""
assert_contains "$RELEASE_WORKFLOW" "--open"
assert_contains "$RELEASE_WORKFLOW" "dist/**/*.msvc"

echo "PASS: release_msvc_spub_test"
