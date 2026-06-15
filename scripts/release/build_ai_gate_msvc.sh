#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

PROJECT_NAME="ai-gate"
PLUGIN_PKG="io.github.gcssloop.ai-gate"
PLUGIN_NAME="ai-gate"
PLUGIN_DESC="AI Gate server mode gateway"
GO_BUILD_TARGET="./cmd/routerd"
BINARY_NAME="ai-gate"
SERVICE_PORT="6789"
OUTPUT_DIR="${REPO_ROOT}/dist/msvc"
VERSION=""
KEEP_STAGE="0"
SELECTED_TARGET="linux/amd64"

usage() {
  cat <<USAGE
Usage: $0 [options]

Options:
  --version <semver>         Package version (default: latest git tag or dev build)
  --port <port>              Service port in daemon.json (default: ${SERVICE_PORT})
  --output <dir>             Output directory (default: ./dist/msvc)
  --target <os/arch>         Build target; only linux/amd64 is supported
  --keep-stage               Keep temp stage directory
  -h, --help                 Show help
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      VERSION="${2:-}"
      shift 2
      ;;
    --port)
      SERVICE_PORT="${2:-}"
      shift 2
      ;;
    --output)
      OUTPUT_DIR="${2:-}"
      shift 2
      ;;
    --target)
      SELECTED_TARGET="${2:-}"
      shift 2
      ;;
    --keep-stage)
      KEEP_STAGE="1"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if [[ "${SELECTED_TARGET}" != "linux/amd64" ]]; then
  echo "Only linux/amd64 is supported for ai-gate msvc packaging" >&2
  exit 1
fi

if [[ -z "${VERSION}" ]]; then
  latest_tag="$(git -C "${REPO_ROOT}" describe --tags --abbrev=0 2>/dev/null || true)"
  if [[ -n "${latest_tag}" ]]; then
    VERSION="${latest_tag}"
  else
    git_sha="$(git -C "${REPO_ROOT}" rev-parse --short HEAD 2>/dev/null || true)"
    VERSION="0.0.0-${git_sha:-dev}"
  fi
fi

WEBUI_STATIC="${REPO_ROOT}/backend/internal/webui/static"
WEBUI_BACKUP="$(mktemp -d)"
cleanup() {
  rm -rf "${WEBUI_STATIC}"
  mkdir -p "${WEBUI_STATIC}"
  cp -R "${WEBUI_BACKUP}/." "${WEBUI_STATIC}/"
  rm -rf "${WEBUI_BACKUP}"
}
trap cleanup EXIT

mkdir -p "${WEBUI_STATIC}" "${OUTPUT_DIR}"
cp -R "${WEBUI_STATIC}/." "${WEBUI_BACKUP}/"

echo "[frontend] build server webui"
npm --prefix "${REPO_ROOT}/frontend" run build -- --mode server
rm -rf "${WEBUI_STATIC}"
mkdir -p "${WEBUI_STATIC}"
cp -R "${REPO_ROOT}/frontend/dist/." "${WEBUI_STATIC}/"

os="linux"
arch="amd64"
stage_dir="${OUTPUT_DIR}/.stage-${PROJECT_NAME}-${os}-${arch}"
gocache_dir="${OUTPUT_DIR}/.gocache-${os}-${arch}"
base_name="${PROJECT_NAME}_${VERSION}_${os}_${arch}"
zip_path="${OUTPUT_DIR}/${base_name}.zip"
msvc_path="${OUTPUT_DIR}/${base_name}.msvc"

echo "[${os}/${arch}] build binary"
rm -rf "${stage_dir}"
mkdir -p "${stage_dir}/backend" "${stage_dir}/data" "${stage_dir}/logs" "${gocache_dir}"
cp "${REPO_ROOT}/assets/aigate_1024_1024.png" "${stage_dir}/icon.png"
(
  cd "${REPO_ROOT}/backend"
  GOCACHE="${gocache_dir}" CGO_ENABLED=0 GOOS="${os}" GOARCH="${arch}" \
    go build -o "${stage_dir}/backend/${BINARY_NAME}" "${GO_BUILD_TARGET}"
)
chmod +x "${stage_dir}/backend/${BINARY_NAME}"

echo "[${os}/${arch}] generate daemon.json"
cat > "${stage_dir}/daemon.json" <<JSON
{
  "id": "",
  "interpreter": "",
  "interpreter_self": "",
  "interpreter_args": [],
  "path": "./backend/${BINARY_NAME}",
  "pkg": "${PLUGIN_PKG}",
  "version": "${VERSION}",
  "log_file": "./logs/ai-gate.log",
  "pid_file": "",
  "icon": "./icon.png",
  "name": "${PLUGIN_NAME}",
  "description": "${PLUGIN_DESC}",
  "args": ["--server"],
  "auto_start": true,
  "auto_restart": true,
  "restart_max": 3,
  "restart_delay": 1000,
  "cwd": "./",
  "env_vars": [
    "AI_GATE_MODE=server",
    "AI_GATE_SERVER_PASSWORD=change-me",
    "CODEX_ROUTER_LISTEN_ADDR=0.0.0.0:${SERVICE_PORT}",
    "CODEX_ROUTER_DATABASE_PATH=./data/aigate.sqlite"
  ],
  "port": ${SERVICE_PORT},
  "port_type": "HTTP",
  "os": "${os}",
  "arch": "${arch}",
  "is_match": true,
  "stop_timeout": 10000,
  "doc": "https://github.com/GcsSloop/ai-gate",
  "api_doc": "{{currentServer}}/ai-gate/api/",
  "homepage": "{{currentServer}}/ai-gate/webui/",
  "gateway": [
    {
      "enable": true,
      "gateWayType": "HTTP",
      "apiPath": "http://127.0.0.1:${SERVICE_PORT}/ai-gate",
      "apiPrefix": "/ai-gate",
      "removePrefix": false
    }
  ],
  "datas": [
    "./data",
    "./logs"
  ]
}
JSON

echo "[${os}/${arch}] pack msvc"
rm -f "${zip_path}" "${msvc_path}"
(
  cd "${stage_dir}"
  zip -qr "${zip_path}" .
)
mv "${zip_path}" "${msvc_path}"
rm -rf "${gocache_dir}"
if [[ "${KEEP_STAGE}" != "1" ]]; then
  rm -rf "${stage_dir}"
fi
echo "Built ${msvc_path}"
