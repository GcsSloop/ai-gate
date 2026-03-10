#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WORK_ROOT="$ROOT_DIR"
TAG=""
VERSION=""

usage() {
  cat <<'EOF'
Usage:
  bash scripts/release/sync_release_metadata.sh --tag v1.0.2
  bash scripts/release/sync_release_metadata.sh --version 1.0.2

Options:
  --root PATH      Override repository root for tests.
  --tag TAG        Release tag, e.g. v1.0.2.
  --version VER    Normalized version, e.g. 1.0.2.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --root)
      WORK_ROOT="$2"
      shift 2
      ;;
    --tag)
      TAG="$2"
      shift 2
      ;;
    --version)
      VERSION="$2"
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

if [[ -z "$VERSION" && -n "$TAG" ]]; then
  VERSION="${TAG#v}"
fi

if [[ -z "$VERSION" ]]; then
  echo "Either --tag or --version is required" >&2
  exit 1
fi

if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
  echo "Unsupported version format: $VERSION" >&2
  exit 1
fi

update_json_version() {
  local file="$1"
  VERSION="$VERSION" node -e '
    const fs = require("fs");
    const file = process.argv[1];
    const version = process.env.VERSION;
    const data = JSON.parse(fs.readFileSync(file, "utf8"));
    data.version = version;
    if (data.packages && data.packages[""]) {
      data.packages[""].version = version;
    }
    fs.writeFileSync(file, JSON.stringify(data, null, 2) + "\n");
  ' "$file"
}

replace_first_match() {
  local file="$1"
  local search="$2"
  local replace="$3"
  node -e '
    const fs = require("fs");
    const file = process.argv[1];
    const search = process.argv[2];
    const replace = process.argv[3];
    const raw = fs.readFileSync(file, "utf8");
    const next = raw.replace(new RegExp(search, "m"), replace);
    if (!new RegExp(search, "m").test(raw)) {
      console.error(`No replacement made for ${file}`);
      process.exit(1);
    }
    if (next !== raw) {
      fs.writeFileSync(file, next);
    }
  ' "$file" "$search" "$replace"
}

update_cargo_lock_version() {
  local file="$1"
  VERSION="$VERSION" node -e '
    const fs = require("fs");
    const file = process.argv[1];
    const version = process.env.VERSION;
    const raw = fs.readFileSync(file, "utf8");
    const next = raw.replace(
      /name = "aigate-desktop"\nversion = "[^"]+"/m,
      `name = "aigate-desktop"\nversion = "${version}"`
    );
    if (!/name = "aigate-desktop"\nversion = "[^"]+"/m.test(raw)) {
      console.error(`No replacement made for ${file}`);
      process.exit(1);
    }
    if (next !== raw) {
      fs.writeFileSync(file, next);
    }
  ' "$file"
}

frontend_pkg="$WORK_ROOT/frontend/package.json"
frontend_lock="$WORK_ROOT/frontend/package-lock.json"
desktop_pkg="$WORK_ROOT/desktop/package.json"
desktop_lock="$WORK_ROOT/desktop/package-lock.json"
tauri_conf="$WORK_ROOT/desktop/src-tauri/tauri.conf.json"
cargo_toml="$WORK_ROOT/desktop/src-tauri/Cargo.toml"
cargo_lock="$WORK_ROOT/desktop/src-tauri/Cargo.lock"
source_ico="$WORK_ROOT/assets/aigate_1024_1024.ico"
target_ico="$WORK_ROOT/desktop/src-tauri/icons/icon.ico"

for file in \
  "$frontend_pkg" \
  "$frontend_lock" \
  "$desktop_pkg" \
  "$desktop_lock" \
  "$tauri_conf" \
  "$cargo_toml" \
  "$cargo_lock" \
  "$source_ico"
do
  [[ -f "$file" ]] || {
    echo "Required file not found: $file" >&2
    exit 1
  }
done

update_json_version "$frontend_pkg"
update_json_version "$frontend_lock"
update_json_version "$desktop_pkg"
update_json_version "$desktop_lock"
update_json_version "$tauri_conf"

replace_first_match "$cargo_toml" 'version = "[^"]+"' "version = \"$VERSION\""
update_cargo_lock_version "$cargo_lock"

mkdir -p "$(dirname "$target_ico")"
cp "$source_ico" "$target_ico"

echo "Synced release metadata to version $VERSION"
