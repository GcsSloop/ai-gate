#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DB_NAME="${1:-aigate-skill-metrics-db}"
KV_BINDING="${2:-SKILL_METRICS_CACHE}"

echo "[1/4] creating D1 database: $DB_NAME"
npx wrangler d1 create "$DB_NAME"

echo "[2/4] creating KV namespace: $KV_BINDING"
npx wrangler kv namespace create "$KV_BINDING"

echo "[3/4] apply schema.sql to D1 (update DB name if needed)"
npx wrangler d1 execute "$DB_NAME" --remote --file="$ROOT_DIR/sql/schema.sql"

cat <<'EOF'
[4/4] done
Next:
- copy generated database_id / kv id into wrangler.jsonc
- deploy: npx wrangler deploy
EOF
