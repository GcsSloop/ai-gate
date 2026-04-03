#!/usr/bin/env bash
set -euo pipefail

max_attempts="${GO_TEST_MAX_ATTEMPTS:-3}"
retry_delay_seconds="${GO_TEST_RETRY_DELAY_SECONDS:-5}"

attempt=1
while true; do
  if go test ./...; then
    exit 0
  fi

  if [[ "$attempt" -ge "$max_attempts" ]]; then
    exit 1
  fi

  echo "go test failed on attempt $attempt/$max_attempts, retrying in ${retry_delay_seconds}s..." >&2
  sleep "$retry_delay_seconds"
  attempt=$((attempt + 1))
done
