# Release Metadata Sync Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make release version metadata follow the pushed tag automatically and ensure the Windows `.ico` asset is copied into the Tauri icon location before packaging.

**Architecture:** Add one repository script that accepts a release tag, normalizes it to an app version, updates the desktop/frontend version declarations, and copies the checked-in `.ico` asset into the Tauri icon path. Run that script in GitHub Actions before tests/builds and cover it with a shell test so future tag changes do not drift from app metadata.

**Tech Stack:** bash, node, jq-free JSON editing, GitHub Actions, Tauri 2, shell tests.

---

### Task 1: Lock release metadata behavior with a failing shell test

**Files:**
- Create: `scripts/test/sync_release_metadata_test.sh`
- Test: `scripts/release/sync_release_metadata.sh`

**Step 1: Write the failing test**

Cover:
- `v1.0.2` becomes `1.0.2`
- desktop/frontend JSON version fields update
- Cargo/Tauri version strings update
- `assets/aigate_1024_1024.ico` is copied to `desktop/src-tauri/icons/icon.ico`

**Step 2: Run test to verify it fails**

Run: `bash scripts/test/sync_release_metadata_test.sh`

**Step 3: Write minimal implementation**

Create `scripts/release/sync_release_metadata.sh` with:
- `--tag` or `--version` input
- optional `--root` for test fixtures
- JSON updates via `node`
- TOML/lock updates via targeted string replacement
- icon copy from `assets/aigate_1024_1024.ico`

**Step 4: Run test to verify it passes**

Run: `bash scripts/test/sync_release_metadata_test.sh`

### Task 2: Wire release metadata sync into build configuration

**Files:**
- Modify: `.github/workflows/release.yml`
- Modify: `desktop/src-tauri/tauri.conf.json`

**Step 1: Update workflow**

Run the sync script in both `test` and `build` jobs using `${{ github.ref_name }}` before tests/builds.

**Step 2: Make icon config explicit**

Add `icons/icon.ico` to the Tauri bundle icon list.

**Step 3: Verify statically**

Run: `sed -n '1,220p' .github/workflows/release.yml`

### Task 3: Verify end-to-end locally

**Files:**
- Verify: `scripts/release/sync_release_metadata.sh`
- Verify: `scripts/test/sync_release_metadata_test.sh`

**Step 1: Run metadata sync test**

Run: `bash scripts/test/sync_release_metadata_test.sh`

**Step 2: Run frontend desktop build**

Run: `npm --prefix frontend run build:desktop`

**Step 3: Inspect diff**

Run: `git diff -- .github/workflows/release.yml desktop/src-tauri/tauri.conf.json frontend/package.json frontend/vite.config.ts scripts/release/sync_release_metadata.sh scripts/test/sync_release_metadata_test.sh`
