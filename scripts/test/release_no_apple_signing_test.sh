#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WORKFLOW_DIR="$ROOT_DIR/.github/workflows"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_workflows_not_contains() {
  local pattern="$1"
  if grep -R -Fq "$pattern" "$WORKFLOW_DIR"; then
    fail "expected GitHub Actions workflows to not contain: $pattern"
  fi
}

assert_workflows_not_contains "name: Prepare notarization key"
assert_workflows_not_contains "name: Import Apple signing certificate"
assert_workflows_not_contains "name: Notarize macOS bundle"
assert_workflows_not_contains "APPLE_API_KEY"
assert_workflows_not_contains "APPLE_API_KEY_ID"
assert_workflows_not_contains "APPLE_API_ISSUER"
assert_workflows_not_contains "APPLE_CERTIFICATE_P12"
assert_workflows_not_contains "APPLE_CERTIFICATE_PASSWORD"
assert_workflows_not_contains "APPLE_SIGNING_IDENTITY"
assert_workflows_not_contains "APPLE_KEYCHAIN"
assert_workflows_not_contains "scripts/desktop/notarize_macos.sh"
assert_workflows_not_contains "scripts/test/notarize_macos_test.sh"
assert_workflows_not_contains "security create-keychain"
assert_workflows_not_contains "security import"
assert_workflows_not_contains "security unlock-keychain"
assert_workflows_not_contains "security set-key-partition-list"
assert_workflows_not_contains "dist/**/*.dmg"

echo "PASS: release_no_apple_signing_test"
