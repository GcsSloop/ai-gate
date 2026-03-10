# GitHub Release Pipeline Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a GitHub Actions release pipeline that builds and publishes macOS and Windows desktop packages on tag push, while keeping the structure ready for future platforms.

**Architecture:** Use one tag-triggered workflow with separate test, build, and publish stages. Platform-specific packaging stays in scripts under `scripts/desktop/`, while the desktop app resolves sidecar binaries by OS-specific resource name instead of a macOS-only hardcoded path.

**Tech Stack:** GitHub Actions, Tauri 2, Go 1.23, Node 20, bash scripts, Rust unit tests, shell integration tests.

---

### Task 1: Make desktop sidecar resolution platform-aware

**Files:**
- Modify: `desktop/src-tauri/src/main.rs`
- Test: `desktop/src-tauri/src/main.rs`

**Step 1: Write the failing tests**

Add tests that require:
- macOS sidecar resource name to remain `routerd-universal-apple-darwin`
- Windows sidecar resource name to become `routerd-x86_64-pc-windows-msvc.exe`
- candidate path generation to include the correct resource name per OS

**Step 2: Run test to verify it fails**

Run: `cd desktop/src-tauri && cargo test sidecar_ -q`

**Step 3: Write minimal implementation**

Refactor sidecar path resolution into small helpers:
- OS-specific sidecar resource name
- candidate path generation
- existing `resolve_sidecar_path()` calling those helpers

**Step 4: Run test to verify it passes**

Run: `cd desktop/src-tauri && cargo test sidecar_ -q`

### Task 2: Add Windows sidecar build support

**Files:**
- Create: `scripts/desktop/build_sidecar_windows.sh`

**Step 1: Write the failing test**

Add a shell test that expects the Windows release collection flow to find `bin/routerd-x86_64-pc-windows-msvc.exe`.

**Step 2: Run test to verify it fails**

Run: `bash scripts/test/collect_release_assets_test.sh`

**Step 3: Write minimal implementation**

Create a Windows sidecar build script that:
- builds `backend/cmd/routerd` for `GOOS=windows GOARCH=amd64`
- writes `desktop/src-tauri/bin/routerd-x86_64-pc-windows-msvc.exe`

**Step 4: Run test to verify it passes**

Run: `bash scripts/test/collect_release_assets_test.sh`

### Task 3: Make release asset collection cross-platform

**Files:**
- Modify: `scripts/desktop/collect_release_assets.sh`
- Test: `scripts/test/collect_release_assets_test.sh`

**Step 1: Write the failing tests**

Cover:
- macOS bundle collection still emits `dmg + zip`
- Windows bundle collection emits `msi + zip`
- output names include the tag version

**Step 2: Run test to verify it fails**

Run: `bash scripts/test/collect_release_assets_test.sh`

**Step 3: Write minimal implementation**

Refactor collection script to accept platform inputs and:
- collect macOS app/dmg from `target/universal-apple-darwin/...`
- collect Windows msi from `target/x86_64-pc-windows-msvc/...`
- create a portable Windows zip with the desktop exe plus `bin/` sidecar

**Step 4: Run test to verify it passes**

Run: `bash scripts/test/collect_release_assets_test.sh`

### Task 4: Update Tauri bundle config for cross-platform resources

**Files:**
- Modify: `desktop/src-tauri/tauri.conf.json`

**Step 1: Implement minimal config change**

Ensure bundle resources include both:
- `bin/routerd-universal-apple-darwin`
- `bin/routerd-x86_64-pc-windows-msvc.exe`

Do not add unsupported platform targets directly to shared config if workflow arguments can scope them more safely.

**Step 2: Verify config shape**

Run: `sed -n '1,240p' desktop/src-tauri/tauri.conf.json`

### Task 5: Add GitHub Actions release workflow

**Files:**
- Create: `.github/workflows/release.yml`

**Step 1: Write the workflow**

Use:
- tag trigger: `v*`
- `contents: write`
- test job
- matrix build job for `macos-latest` and `windows-latest`
- artifact upload per platform
- release publish job attaching all assets to the GitHub Release

**Step 2: Build job responsibilities**

- checkout
- setup Go 1.23
- setup Node 20
- install frontend and desktop dependencies
- run platform sidecar build script
- run Tauri build with platform-specific bundle args
- run asset collection script
- upload artifact

**Step 3: Publish job responsibilities**

- download all build artifacts
- publish or update the tag release
- attach all files from the downloaded artifact folders

### Task 6: Verify the release pipeline statically

**Files:**
- Verify: `.github/workflows/release.yml`
- Verify: `scripts/desktop/*.sh`
- Verify: `desktop/src-tauri/src/main.rs`

**Step 1: Run focused tests**

Run: `bash scripts/test/collect_release_assets_test.sh`

**Step 2: Run Rust tests**

Run: `cd desktop/src-tauri && cargo test sidecar_ -q`

**Step 3: Inspect workflow and diff**

Run: `git diff -- .github/workflows/release.yml scripts/desktop desktop/src-tauri/src/main.rs scripts/test/collect_release_assets_test.sh`

