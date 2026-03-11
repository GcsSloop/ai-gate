#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WORKFLOW_PATH="$ROOT_DIR/.github/workflows/release.yml"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_contains() {
  local pattern="$1"
  if ! grep -Fq "$pattern" "$WORKFLOW_PATH"; then
    fail "expected release workflow to contain: $pattern"
  fi
}

assert_not_contains() {
  local pattern="$1"
  if grep -Fq "$pattern" "$WORKFLOW_PATH"; then
    fail "expected release workflow to not contain: $pattern"
  fi
}

assert_not_contains "name: Prepare updater signing key"
assert_not_contains 'TAURI_SIGNING_PRIVATE_KEY_B64: ${{ secrets.TAURI_SIGNING_PRIVATE_KEY }}'
assert_not_contains 'printf "%s" "$TAURI_SIGNING_PRIVATE_KEY_B64" | base64 --decode'
assert_contains 'TAURI_SIGNING_PRIVATE_KEY: ${{ secrets.TAURI_SIGNING_PRIVATE_KEY }}'
assert_contains 'TAURI_SIGNING_PRIVATE_KEY_PASSWORD: ${{ secrets.TAURI_SIGNING_PRIVATE_KEY_PASSWORD }}'

echo "PASS: release_updater_signing_key_test"
