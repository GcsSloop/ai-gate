#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=scripts/desktop/release_version_helpers.sh
source "$ROOT_DIR/scripts/desktop/release_version_helpers.sh"

TAG="$(resolve_release_tag)"
TARGET_PLATFORM="${RELEASE_PLATFORM:-}"
ASSET_DIR="${RELEASE_ASSET_DIR:-$ROOT_DIR/release-assets}"

sync_args=()
if [[ "$TAG" =~ ^v ]]; then
  sync_args=(--tag "$TAG")
else
  sync_args=(--version "$TAG")
fi

bash "$ROOT_DIR/scripts/release/sync_release_metadata.sh" "${sync_args[@]}"
npm --prefix desktop run tauri build

collect_env=(RELEASE_VERSION="$TAG" RELEASE_ASSET_DIR="$ASSET_DIR")
if [[ -n "$TARGET_PLATFORM" ]]; then
  collect_env+=(RELEASE_PLATFORM="$TARGET_PLATFORM")
fi

env "${collect_env[@]}" bash "$ROOT_DIR/scripts/desktop/collect_release_assets.sh"

printf 'Packaged desktop release for %s into %s\n' "$TAG" "$ASSET_DIR"
