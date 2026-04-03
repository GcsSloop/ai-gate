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

assert_contains "name: Import Apple signing certificate"
assert_contains 'APPLE_CERTIFICATE_P12: ${{ secrets.APPLE_CERTIFICATE_P12 }}'
assert_contains 'APPLE_CERTIFICATE_PASSWORD: ${{ secrets.APPLE_CERTIFICATE_PASSWORD }}'
assert_contains 'APPLE_KEYCHAIN_PASSWORD='
assert_contains 'security create-keychain -p "$APPLE_KEYCHAIN_PASSWORD"'
assert_contains 'security import "$apple_cert_path"'
assert_contains 'security unlock-keychain -p "$APPLE_KEYCHAIN_PASSWORD"'
assert_contains 'security set-key-partition-list -S apple-tool:,apple: -s -k "$APPLE_KEYCHAIN_PASSWORD"'
assert_contains 'security list-keychains -d user -s'
assert_contains "name: Notarize macOS bundle"

echo "PASS: release_apple_signing_test"
