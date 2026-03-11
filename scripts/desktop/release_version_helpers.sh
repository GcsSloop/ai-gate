#!/usr/bin/env bash
set -euo pipefail

resolve_release_tag() {
  if [[ -n "${RELEASE_VERSION:-}" ]]; then
    printf '%s\n' "$RELEASE_VERSION"
    return 0
  fi

  local tag
  if ! tag="$(git describe --tags --abbrev=0 2>/dev/null)"; then
    echo "No reachable git tag found. Create a tag or set RELEASE_VERSION explicitly." >&2
    return 1
  fi

  printf '%s\n' "$tag"
}

resolve_release_version() {
  resolve_release_tag
}
