#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-https://aigate-skill-metrics.gcssloop.workers.dev}"
TOKEN="${TRACKED_REPOS_ADMIN_TOKEN:-}"
TMP_HTML="${TMPDIR:-/tmp}/skills_page_$$.html"
TMP_LIST="${TMPDIR:-/tmp}/skills_sources_$$.txt"
TMP_JSON="${TMPDIR:-/tmp}/tracked_repos_$$.json"

if [[ -z "$TOKEN" ]]; then
  echo "TRACKED_REPOS_ADMIN_TOKEN is required" >&2
  exit 1
fi

pages=(
  "https://skills.sh/"
  "https://skills.sh/trending"
  "https://skills.sh/hot"
  "https://skills.sh/official"
)

cleanup() {
  rm -f "$TMP_HTML" "$TMP_LIST" "$TMP_JSON"
}
trap cleanup EXIT

: > "$TMP_LIST"
for page in "${pages[@]}"; do
  echo "fetch: $page"
  curl -L --max-time 60 -fsS "$page" > "$TMP_HTML"
  rg -o 'source\\":\\"[^"\\]+' "$TMP_HTML" | sed 's/source\\":\\"//' >> "$TMP_LIST" || true
done

sort -u "$TMP_LIST" > "${TMP_LIST}.uniq"
mv "${TMP_LIST}.uniq" "$TMP_LIST"

python3 - "$TMP_LIST" "$TMP_JSON" <<'PY'
import json
import sys
from pathlib import Path

src = Path(sys.argv[1]).read_text().splitlines()
out = Path(sys.argv[2])
items = []
seen = set()

for line in src:
    value = line.strip()
    if not value or "/" not in value:
        continue
    owner, name = value.split("/", 1)
    key = (owner.lower(), name.lower())
    if key in seen:
        continue
    seen.add(key)
    items.append({
        "platform": "github",
        "owner": owner,
        "name": name,
        "branch": "main",
        "enabled": True,
        "sort_order": len(items),
    })

out.write_text(json.dumps({"items": items}, separators=(",", ":")))
print(f"repos={len(items)}")
PY

curl -fsS -X PUT "$BASE_URL/tracked-repos" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  --data-binary @"$TMP_JSON" | jq '{count:(.items|length)}'
