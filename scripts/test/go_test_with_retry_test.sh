#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT_PATH="$ROOT_DIR/scripts/ci/go_test_with_retry.sh"

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

cat >"$tmp_dir/go" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'go:%s\n' "$*" >>"$CALL_LOG"
attempt_file="$TMP_ATTEMPT_FILE"
attempt=0
if [[ -f "$attempt_file" ]]; then
  attempt="$(cat "$attempt_file")"
fi
attempt=$((attempt + 1))
printf '%s' "$attempt" >"$attempt_file"
if [[ "$attempt" -lt 2 ]]; then
  echo "net/http: TLS handshake timeout" >&2
  exit 1
fi
EOF
chmod +x "$tmp_dir/go"

CALL_LOG="$tmp_dir/calls.log" \
TMP_ATTEMPT_FILE="$tmp_dir/attempt.txt" \
PATH="$tmp_dir:$PATH" \
GO_TEST_MAX_ATTEMPTS=3 \
GO_TEST_RETRY_DELAY_SECONDS=0 \
bash "$SCRIPT_PATH" >/tmp/go-test-retry.out 2>/tmp/go-test-retry.err

assert_contains "$tmp_dir/calls.log" "go:test ./..."
assert_contains /tmp/go-test-retry.err "go test failed on attempt 1/3"

cat >"$tmp_dir/go" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
exit 1
EOF
chmod +x "$tmp_dir/go"

if CALL_LOG="$tmp_dir/calls.log" \
  TMP_ATTEMPT_FILE="$tmp_dir/attempt.txt" \
  PATH="$tmp_dir:$PATH" \
  GO_TEST_MAX_ATTEMPTS=2 \
  GO_TEST_RETRY_DELAY_SECONDS=0 \
  bash "$SCRIPT_PATH" >/tmp/go-test-retry-fail.out 2>/tmp/go-test-retry-fail.err; then
  fail "expected go_test_with_retry.sh to fail after exhausting retries"
fi

assert_contains /tmp/go-test-retry-fail.err "go test failed on attempt 1/2"

echo "PASS: go_test_with_retry_test"
