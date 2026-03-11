# Local Package Version From Reachable Tag Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make local desktop packaging resolve the version from the nearest reachable git tag, sync app metadata to that tag, and produce release assets with the same version.

**Architecture:** Add one shared shell helper for version resolution, reuse it from the asset collector, and introduce a single local packaging entrypoint that orchestrates metadata sync, Tauri build, and asset collection. Keep CI tag-based release flow unchanged.

**Tech Stack:** Bash, git, npm, Tauri, existing release helper scripts.

---

### Task 1: Add failing script tests for reachable-tag resolution

**Files:**
- Create: `scripts/test/release_version_helpers_test.sh`
- Modify: `scripts/test/collect_release_assets_test.sh`
- Create: `scripts/desktop/release_version_helpers.sh`

**Step 1: Write the failing test**
Add a shell test that creates a temporary git repository, tags one commit, advances one commit, and asserts the helper returns the tagged version. Extend the asset collector test to verify fallback resolution uses the reachable tag when `RELEASE_VERSION` is unset.

**Step 2: Run test to verify it fails**
Run: `bash scripts/test/release_version_helpers_test.sh && bash scripts/test/collect_release_assets_test.sh`
Expected: FAIL because the helper does not exist and the asset collector still falls back to `local`.

**Step 3: Write minimal implementation**
Create a helper script that resolves version from `RELEASE_VERSION` or `git describe --tags --abbrev=0`. Source it from the asset collector.

**Step 4: Run test to verify it passes**
Run: `bash scripts/test/release_version_helpers_test.sh && bash scripts/test/collect_release_assets_test.sh`
Expected: PASS.

**Step 5: Commit**
```bash
git add scripts/desktop/release_version_helpers.sh scripts/test/release_version_helpers_test.sh scripts/test/collect_release_assets_test.sh scripts/desktop/collect_release_assets.sh
git commit -m "test(release): cover local tag version resolution"
```

### Task 2: Add the local packaging entrypoint with TDD

**Files:**
- Create: `scripts/desktop/package_local_release.sh`
- Create: `scripts/test/package_local_release_test.sh`
- Modify: `Makefile`

**Step 1: Write the failing test**
Add a shell test that stubs `npm` and the existing release scripts, runs the new entrypoint inside a temporary git repository with a reachable tag, and asserts the commands receive the resolved tag. Add a failure case where no tag exists.

**Step 2: Run test to verify it fails**
Run: `bash scripts/test/package_local_release_test.sh`
Expected: FAIL because the entrypoint does not exist.

**Step 3: Write minimal implementation**
Implement the entrypoint and add a `package-desktop` make target that calls it.

**Step 4: Run test to verify it passes**
Run: `bash scripts/test/package_local_release_test.sh`
Expected: PASS.

**Step 5: Commit**
```bash
git add scripts/desktop/package_local_release.sh scripts/test/package_local_release_test.sh Makefile
git commit -m "feat(release): add local desktop packaging entrypoint"
```

### Task 3: Verify end-to-end local packaging behavior

**Files:**
- Modify: `scripts/desktop/package_local_release.sh` if needed
- Modify: `scripts/release/sync_release_metadata.sh` only if verification exposes a real gap

**Step 1: Run focused script tests**
Run: `bash scripts/test/release_version_helpers_test.sh && bash scripts/test/collect_release_assets_test.sh && bash scripts/test/package_local_release_test.sh && bash scripts/test/sync_release_metadata_test.sh`
Expected: PASS.

**Step 2: Run local packaging command**
Run: `bash scripts/desktop/package_local_release.sh`
Expected: Tauri build completes and `release-assets/` contains versioned artifacts using the nearest reachable tag.

**Step 3: Inspect changed metadata**
Run: `git diff -- frontend/package.json frontend/package-lock.json desktop/package.json desktop/package-lock.json desktop/src-tauri/Cargo.toml desktop/src-tauri/Cargo.lock desktop/src-tauri/tauri.conf.json`
Expected: Versions match the resolved tag.

**Step 4: Commit**
```bash
git add Makefile scripts/desktop scripts/test frontend/package.json frontend/package-lock.json desktop/package.json desktop/package-lock.json desktop/src-tauri/Cargo.toml desktop/src-tauri/Cargo.lock desktop/src-tauri/tauri.conf.json
git commit -m "chore(release): derive local package version from git tag"
```
