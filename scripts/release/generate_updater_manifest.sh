#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

tag=""
repo=""
assets_root=""
output=""
notes=""

usage() {
  cat <<'EOF'
Usage:
  bash scripts/release/generate_updater_manifest.sh \
    --tag v1.2.3 \
    --repo GcsSloop/ai-gate \
    --assets-root dist \
    --output dist/latest.json \
    [--notes "Release notes"]
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --tag)
      tag="$2"
      shift 2
      ;;
    --repo)
      repo="$2"
      shift 2
      ;;
    --assets-root)
      assets_root="$2"
      shift 2
      ;;
    --output)
      output="$2"
      shift 2
      ;;
    --notes)
      notes="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

[[ -n "$tag" ]] || {
  echo "--tag is required" >&2
  exit 1
}
[[ -n "$repo" ]] || {
  echo "--repo is required" >&2
  exit 1
}
[[ -n "$assets_root" ]] || {
  echo "--assets-root is required" >&2
  exit 1
}
[[ -n "$output" ]] || {
  echo "--output is required" >&2
  exit 1
}

version="${tag#v}"
mac_asset="aigate-${tag}-darwin-universal.app.tar.gz"
mac_sig_asset="${mac_asset}.sig"
windows_asset="aigate-${tag}-windows.msi"
windows_sig_asset="${windows_asset}.sig"

find_asset() {
  local file_name="$1"
  find "$assets_root" -type f -name "$file_name" | head -n1 || true
}

read_trimmed_file() {
  local path="$1"
  tr -d '\r\n' <"$path"
}

mac_path="$(find_asset "$mac_asset")"
mac_sig_path="$(find_asset "$mac_sig_asset")"
windows_path="$(find_asset "$windows_asset")"
windows_sig_path="$(find_asset "$windows_sig_asset")"

[[ -f "$mac_path" ]] || {
  echo "Missing macOS updater asset: $mac_asset" >&2
  exit 1
}
[[ -f "$mac_sig_path" ]] || {
  echo "Missing macOS updater signature: $mac_sig_asset" >&2
  exit 1
}
[[ -f "$windows_path" ]] || {
  echo "Missing Windows updater asset: $windows_asset" >&2
  exit 1
}
[[ -f "$windows_sig_path" ]] || {
  echo "Missing Windows updater signature: $windows_sig_asset" >&2
  exit 1
}

mac_signature="$(read_trimmed_file "$mac_sig_path")"
windows_signature="$(read_trimmed_file "$windows_sig_path")"
pub_date="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
mkdir -p "$(dirname "$output")"

VERSION="$version" \
NOTES="$notes" \
PUB_DATE="$pub_date" \
REPO="$repo" \
TAG="$tag" \
MAC_ASSET="$mac_asset" \
MAC_SIGNATURE="$mac_signature" \
WINDOWS_ASSET="$windows_asset" \
WINDOWS_SIGNATURE="$windows_signature" \
OUTPUT_FILE="$output" \
node <<'EOF'
const fs = require("fs");

const output = process.env.OUTPUT_FILE;
const repo = process.env.REPO;
const tag = process.env.TAG;
const baseUrl = `https://github.com/${repo}/releases/download/${tag}`;

const manifest = {
  version: process.env.VERSION,
  notes: process.env.NOTES || "",
  pub_date: process.env.PUB_DATE,
  platforms: {
    "darwin-aarch64": {
      signature: process.env.MAC_SIGNATURE,
      url: `${baseUrl}/${process.env.MAC_ASSET}`,
    },
    "darwin-x86_64": {
      signature: process.env.MAC_SIGNATURE,
      url: `${baseUrl}/${process.env.MAC_ASSET}`,
    },
    "windows-x86_64": {
      signature: process.env.WINDOWS_SIGNATURE,
      url: `${baseUrl}/${process.env.WINDOWS_ASSET}`,
    },
  },
};

fs.writeFileSync(output, JSON.stringify(manifest, null, 2) + "\n");
EOF

printf 'Generated updater manifest at %s\n' "$output"
