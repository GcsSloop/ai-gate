#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
OUT_DIR="${REPO_ROOT}/dist/msvc-test"
ARTIFACT="${OUT_DIR}/ai-gate_0.0.0-dev_linux_amd64.msvc"

rm -rf "${OUT_DIR}"
cd "${REPO_ROOT}"
bash "${REPO_ROOT}/scripts/release/build_ai_gate_msvc.sh" \
  --target linux/amd64 \
  --version 0.0.0-dev \
  --output "dist/msvc-test"

test -f "${ARTIFACT}"
LIST_FILE="${OUT_DIR}/artifact-list.txt"
unzip -l "${ARTIFACT}" >"${LIST_FILE}"
grep -q "daemon.json" "${LIST_FILE}"
unzip -p "${ARTIFACT}" daemon.json | grep -q '"name": "ai-gate"'
unzip -p "${ARTIFACT}" daemon.json | grep -q '"icon": "./icon.png"'
unzip -p "${ARTIFACT}" daemon.json | grep -q '"apiPrefix": "/ai-gate"'
unzip -p "${ARTIFACT}" daemon.json | grep -q '"./data"'
grep -q "backend/ai-gate" "${LIST_FILE}"
grep -q "icon.png" "${LIST_FILE}"

echo "build_ai_gate_msvc_test passed"
