#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=scripts/desktop/release_path_helpers.sh
source "$ROOT_DIR/scripts/desktop/release_path_helpers.sh"
VERSION="${RELEASE_VERSION:-${GITHUB_REF_NAME:-${CI_COMMIT_TAG:-local}}}"
PLATFORM="${RELEASE_PLATFORM:-auto}"
TARGET_DIR="${TARGET_DIR:-$ROOT_DIR/desktop/src-tauri/target}"
OUT_DIR="${RELEASE_ASSET_DIR:-$ROOT_DIR/release-assets}"
SIDECAR_BIN_DIR="${SIDECAR_BIN_DIR:-$ROOT_DIR/desktop/src-tauri/bin}"
APP_EXECUTABLE_NAME="${APP_EXECUTABLE_NAME:-aigate-desktop.exe}"

mkdir -p "$OUT_DIR"

checksum_file() {
  local file="$1"
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
  elif command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
  else
    openssl dgst -sha256 "$file" | awk '{print $NF}'
  fi
}

write_checksums() {
  local output_file="$OUT_DIR/aigate-${VERSION}-${PLATFORM}-SHA256SUMS.txt"
  : >"$output_file"
  while IFS= read -r file; do
    local name
    name="$(basename "$file")"
    printf '%s  %s\n' "$(checksum_file "$file")" "$name" >>"$output_file"
  done < <(find "$OUT_DIR" -maxdepth 1 -type f ! -name 'aigate-*-SHA256SUMS.txt' | sort)
}

zip_dir() {
  local source_dir="$1"
  local dest_zip="$2"
  rm -f "$dest_zip"
  if command -v ditto >/dev/null 2>&1; then
    ditto -c -k --sequesterRsrc --keepParent "$source_dir" "$dest_zip"
  elif command -v zip >/dev/null 2>&1; then
    local parent_dir
    parent_dir="$(dirname "$source_dir")"
    local dir_name
    dir_name="$(basename "$source_dir")"
    (
      cd "$parent_dir"
      zip -rq "$dest_zip" "$dir_name"
    )
  elif command -v powershell.exe >/dev/null 2>&1; then
    local source_windows
    local dest_windows
    source_windows="$(to_windows_path "$source_dir")"
    dest_windows="$(to_windows_path "$dest_zip")"
    powershell.exe -NoLogo -NoProfile -Command \
      "Compress-Archive -Path '${source_windows}\\*' -DestinationPath '${dest_windows}' -Force" \
      >/dev/null
  else
    echo "No zip implementation available" >&2
    exit 1
  fi
}

collect_macos_assets() {
  local src_dir="$TARGET_DIR/universal-apple-darwin/release/bundle"
  local app_path
  local dmg_path
  app_path="$(find "$src_dir/macos" -maxdepth 1 -name "*.app" -type d | head -n1 || true)"
  dmg_path="$(find "$src_dir/dmg" -maxdepth 1 -name "*.dmg" -type f | head -n1 || true)"

  [[ -n "$app_path" ]] || {
    echo "No macOS app bundle found under $src_dir" >&2
    exit 1
  }

  zip_dir "$app_path" "$OUT_DIR/aigate-${VERSION}-macOS.zip"

  if [[ -n "$dmg_path" ]]; then
    cp "$dmg_path" "$OUT_DIR/aigate-${VERSION}-macOS.dmg"
  fi
}

collect_windows_assets() {
  local src_dir="$TARGET_DIR/x86_64-pc-windows-msvc/release"
  local msi_path
  local app_path="$src_dir/$APP_EXECUTABLE_NAME"
  local sidecar_path="$SIDECAR_BIN_DIR/routerd-x86_64-pc-windows-msvc.exe"
  local stage_dir
  msi_path="$(find "$src_dir/bundle/msi" -maxdepth 1 -name "*.msi" -type f | head -n1 || true)"

  [[ -n "$msi_path" ]] || {
    echo "No Windows MSI found under $src_dir/bundle/msi" >&2
    exit 1
  }
  [[ -f "$app_path" ]] || {
    echo "No Windows desktop executable found at $app_path" >&2
    exit 1
  }
  [[ -f "$sidecar_path" ]] || {
    echo "No Windows sidecar found at $sidecar_path" >&2
    exit 1
  }

  cp "$msi_path" "$OUT_DIR/aigate-${VERSION}-windows.msi"

  stage_dir="$(mktemp -d)"
  mkdir -p "$stage_dir/aigate-${VERSION}-windows/bin"
  cp "$app_path" "$stage_dir/aigate-${VERSION}-windows/$APP_EXECUTABLE_NAME"
  cp "$sidecar_path" "$stage_dir/aigate-${VERSION}-windows/bin/"
  zip_dir "$stage_dir/aigate-${VERSION}-windows" "$OUT_DIR/aigate-${VERSION}-windows.zip"
  rm -rf "$stage_dir"
}

if [[ "$PLATFORM" == "auto" ]]; then
  if [[ -d "$TARGET_DIR/universal-apple-darwin/release/bundle" ]]; then
    PLATFORM="macos"
  elif [[ -d "$TARGET_DIR/x86_64-pc-windows-msvc/release/bundle" ]]; then
    PLATFORM="windows"
  else
    echo "Unable to infer release platform from $TARGET_DIR" >&2
    exit 1
  fi
fi

case "$PLATFORM" in
  macos)
    collect_macos_assets
    ;;
  windows)
    collect_windows_assets
    ;;
  *)
    echo "Unsupported release platform: $PLATFORM" >&2
    exit 1
    ;;
esac

write_checksums

ls -la "$OUT_DIR"
