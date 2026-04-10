#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${1:-https://aigate-skill-metrics.gcssloop.workers.dev}"
DAY="${2:-$(date -u +%F)}"

echo "[1/3] health"
curl -fsS "$BASE_URL/health" | sed 's/.*/  &/'

echo "[2/3] ingest demo event"
curl -fsS -X POST "$BASE_URL/events/install" \
  -H "Content-Type: application/json" \
  -d "{\"user_hash\":\"smoke_$(date +%s)\",\"skill_name\":\"frontend-design\",\"source_repo\":\"anthropics/skills\",\"client_version\":\"smoke\"}" \
  | sed 's/.*/  &/'

echo "[3/3] query ranking"
curl -fsS "$BASE_URL/rankings/skills?day=$DAY&limit=50" | sed 's/.*/  &/'
