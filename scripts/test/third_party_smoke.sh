#!/usr/bin/env bash
set -euo pipefail

ROUTER_URL="${ROUTER_URL:-http://127.0.0.1:8080}"
THIRD_PARTY_BASE_URL="${THIRD_PARTY_BASE_URL:-https://code.ppchat.vip/v1}"
THIRD_PARTY_API_KEY="${THIRD_PARTY_API_KEY:-}"

if [[ -z "${THIRD_PARTY_API_KEY}" ]]; then
  echo "THIRD_PARTY_API_KEY is required" >&2
  exit 1
fi

echo "Creating low-volume third-party account via router..."
curl -fsS "${ROUTER_URL}/accounts" \
  -H "Content-Type: application/json" \
  -X POST \
  -d "{
    \"provider_type\":\"openai-compatible\",
    \"account_name\":\"ppchat-smoke\",
    \"auth_mode\":\"api_key\",
    \"base_url\":\"${THIRD_PARTY_BASE_URL}\",
    \"credential_ref\":\"${THIRD_PARTY_API_KEY}\"
  }" >/dev/null

echo "Running one tiny completion through router..."
curl -fsS "${ROUTER_URL}/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -X POST \
  -d '{
    "model":"gpt-5.2-codex",
    "stream":false,
    "messages":[{"role":"user","content":"reply with one word: pong"}]
  }'
