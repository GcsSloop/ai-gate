#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
WEBUI_STATIC="${REPO_ROOT}/backend/internal/webui/static"
WEBUI_BACKUP="$(mktemp -d)"

cleanup() {
  rm -rf "${WEBUI_STATIC}"
  mkdir -p "${WEBUI_STATIC}"
  cp -R "${WEBUI_BACKUP}/." "${WEBUI_STATIC}/"
  rm -rf "${WEBUI_BACKUP}"
}
trap cleanup EXIT

mkdir -p "${WEBUI_STATIC}"
cp -R "${WEBUI_STATIC}/." "${WEBUI_BACKUP}/"

npm --prefix "${REPO_ROOT}/frontend" run build -- --mode server
rm -rf "${WEBUI_STATIC}"
mkdir -p "${WEBUI_STATIC}"
cp -R "${REPO_ROOT}/frontend/dist/." "${WEBUI_STATIC}/"

mkdir -p "${REPO_ROOT}/backend/data/server-dev"
cd "${REPO_ROOT}/backend"
AI_GATE_MODE=server \
AI_GATE_SERVER_PASSWORD="${AI_GATE_SERVER_PASSWORD:-dev-password}" \
CODEX_ROUTER_DATABASE_PATH="${CODEX_ROUTER_DATABASE_PATH:-${REPO_ROOT}/backend/data/server-dev/aigate.sqlite}" \
go run ./cmd/routerd --server
