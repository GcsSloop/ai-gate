#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-https://aigate-skill-metrics.gcssloop.workers.dev}"
TOKEN="${TRACKED_REPOS_ADMIN_TOKEN:-}"

usage() {
  cat <<'EOF'
Usage:
  tracked-repos.sh list
  tracked-repos.sh add <platform> <owner> <name> [branch] [sort_order]
  tracked-repos.sh delete <platform> <owner> <name>
  tracked-repos.sh replace <json_file>

Env:
  BASE_URL                     Metrics service base url.
  TRACKED_REPOS_ADMIN_TOKEN    Required for list/add/delete/replace operations.
EOF
}

auth_args=()
if [[ -n "$TOKEN" ]]; then
  auth_args=(-H "Authorization: Bearer $TOKEN")
fi

cmd="${1:-}"
case "$cmd" in
  list)
    curl -fsS "$BASE_URL/tracked-repos"
    ;;
  add)
    platform="${2:-}"; owner="${3:-}"; name="${4:-}"; branch="${5:-main}"; order="${6:-0}"
    [[ -n "$platform" && -n "$owner" && -n "$name" ]] || { usage; exit 1; }
    curl -fsS -X POST "$BASE_URL/tracked-repos" \
      "${auth_args[@]}" \
      -H "Content-Type: application/json" \
      -d "{\"platform\":\"$platform\",\"owner\":\"$owner\",\"name\":\"$name\",\"branch\":\"$branch\",\"enabled\":true,\"sort_order\":$order}"
    ;;
  delete)
    platform="${2:-}"; owner="${3:-}"; name="${4:-}"
    [[ -n "$platform" && -n "$owner" && -n "$name" ]] || { usage; exit 1; }
    curl -fsS -X DELETE "$BASE_URL/tracked-repos?platform=$platform&owner=$owner&name=$name" \
      "${auth_args[@]}"
    ;;
  replace)
    json_file="${2:-}"
    [[ -n "$json_file" && -f "$json_file" ]] || { usage; exit 1; }
    curl -fsS -X PUT "$BASE_URL/tracked-repos" \
      "${auth_args[@]}" \
      -H "Content-Type: application/json" \
      --data-binary @"$json_file"
    ;;
  *)
    usage
    exit 1
    ;;
esac
