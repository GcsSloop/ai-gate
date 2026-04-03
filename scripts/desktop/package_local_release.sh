#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=scripts/desktop/release_version_helpers.sh
source "$ROOT_DIR/scripts/desktop/release_version_helpers.sh"

TAG="$(resolve_release_tag)"
TARGET_PLATFORM="${RELEASE_PLATFORM:-}"
ASSET_DIR="${RELEASE_ASSET_DIR:-$ROOT_DIR/release-assets}"

resolve_local_platform() {
  if [[ -n "$TARGET_PLATFORM" ]]; then
    printf '%s\n' "$TARGET_PLATFORM"
    return 0
  fi

  case "$(uname -s)" in
    Darwin)
      printf 'macos\n'
      ;;
    MINGW*|MSYS*|CYGWIN*)
      printf 'windows\n'
      ;;
    *)
      echo "Unsupported local release platform: $(uname -s). Set RELEASE_PLATFORM explicitly." >&2
      return 1
      ;;
  esac
}

sync_args=()
if [[ "$TAG" =~ ^v ]]; then
  sync_args=(--tag "$TAG")
else
  sync_args=(--version "$TAG")
fi

TARGET_PLATFORM="$(resolve_local_platform)"

bash "$ROOT_DIR/scripts/release/sync_release_metadata.sh" "${sync_args[@]}"
case "$TARGET_PLATFORM" in
  macos)
    bash "$ROOT_DIR/scripts/desktop/build_sidecar_macos.sh"
    ;;
  windows)
    bash "$ROOT_DIR/scripts/desktop/build_sidecar_windows.sh"
    ;;
  *)
    echo "Unsupported release platform: $TARGET_PLATFORM" >&2
    exit 1
    ;;
esac

npm --prefix desktop run tauri build
bash "$ROOT_DIR/scripts/desktop/notarize_macos.sh"

collect_env=(RELEASE_VERSION="$TAG" RELEASE_ASSET_DIR="$ASSET_DIR")
if [[ -n "$TARGET_PLATFORM" ]]; then
  collect_env+=(RELEASE_PLATFORM="$TARGET_PLATFORM")
fi

env "${collect_env[@]}" bash "$ROOT_DIR/scripts/desktop/collect_release_assets.sh"

printf 'Packaged desktop release for %s into %s\n' "$TAG" "$ASSET_DIR"
