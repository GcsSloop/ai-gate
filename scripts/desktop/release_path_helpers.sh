#!/usr/bin/env bash

to_windows_path() {
  local path="$1"

  if command -v cygpath >/dev/null 2>&1; then
    cygpath -w "$path"
    return
  fi

  if [[ "$path" =~ ^/([a-zA-Z])/(.*)$ ]]; then
    local drive="${BASH_REMATCH[1]}"
    local rest="${BASH_REMATCH[2]//\//\\}"
    drive="$(printf '%s' "$drive" | tr '[:lower:]' '[:upper:]')"
    printf '%s\n' "${drive}:\\${rest}"
    return
  fi

  printf '%s\n' "${path//\//\\}"
}
