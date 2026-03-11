# GitHub Update Check And Install Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add GitHub-based update checking, download, installation, and relaunch support to the desktop app using Tauri's official updater plugin.

**Architecture:** Wire Tauri updater/process plugins into the desktop runtime, expose an update status flow to the frontend, and extend the release workflow to generate signed updater artifacts plus `latest.json` on GitHub Releases. Keep manual installers as secondary release assets.

**Tech Stack:** Tauri 2 updater/process plugins, React, TypeScript, Rust, GitHub Actions.

---

### Task 1: Add failing frontend tests for the update UX state machine

**Files:**
- Create: `frontend/src/features/updates/updateService.ts`
- Create: `frontend/src/features/updates/updateService.test.ts`
- Modify: `frontend/package.json`

**Step 1: Write the failing test**
Create a focused test covering: no update available, update available, download progress mapping, install complete, and error state transitions.

**Step 2: Run test to verify it fails**
Run: `npm --prefix frontend run test -- updateService`
Expected: FAIL because the service does not exist.

**Step 3: Write minimal implementation**
Implement a small typed update service wrapper around Tauri updater/process APIs with injectable adapters for tests.

**Step 4: Run test to verify it passes**
Run: `npm --prefix frontend run test -- updateService`
Expected: PASS.

**Step 5: Commit**
```bash
git add frontend/src/features/updates frontend/package.json
git commit -m "test(updates): cover updater state transitions"
```

### Task 2: Add failing UI tests for the update card/dialog

**Files:**
- Modify: `frontend/src/...` locate settings/about page component
- Create: `frontend/src/features/updates/UpdateCard.tsx`
- Create: `frontend/src/features/updates/UpdateCard.test.tsx`

**Step 1: Write the failing test**
Add tests for the visible states: current version, checking, update available with notes, downloading with progress, install-ready with restart, and error toast/message.

**Step 2: Run test to verify it fails**
Run: `npm --prefix frontend run test -- UpdateCard`
Expected: FAIL because the component does not exist.

**Step 3: Write minimal implementation**
Render the update card and wire it to the service state.

**Step 4: Run test to verify it passes**
Run: `npm --prefix frontend run test -- UpdateCard`
Expected: PASS.

**Step 5: Commit**
```bash
git add frontend/src/features/updates
git commit -m "feat(updates): add desktop update card"
```

### Task 3: Add failing desktop runtime/config tests for updater integration

**Files:**
- Modify: `desktop/src-tauri/src/main.rs`
- Modify: `desktop/src-tauri/Cargo.toml`
- Modify: `desktop/src-tauri/tauri.conf.json`
- Locate and modify: `desktop/src-tauri/capabilities/*.json`

**Step 1: Write the failing test**
Add Rust tests for any new helper structs and config assertions where practical; add a script or JSON validation test that checks updater config, endpoint, and capability permissions are present.

**Step 2: Run test to verify it fails**
Run: `cargo test -q` in `desktop/src-tauri` and the config validation script.
Expected: FAIL because updater integration is missing.

**Step 3: Write minimal implementation**
Register updater/process plugins, add updater config and permissions, and keep app metadata/version exposure intact.

**Step 4: Run test to verify it passes**
Run: `cargo test -q` and the config validation script again.
Expected: PASS.

**Step 5: Commit**
```bash
git add desktop/src-tauri desktop/package.json desktop/package-lock.json
git commit -m "feat(updates): wire tauri updater runtime"
```

### Task 4: Add failing release workflow tests for updater artifacts

**Files:**
- Create: `scripts/test/release_updater_assets_test.sh`
- Modify: `scripts/desktop/collect_release_assets.sh` or add a dedicated updater asset collector if separation is cleaner
- Modify: `.github/workflows/dev-ci.yml`
- Modify: `.github/workflows/release.yml`

**Step 1: Write the failing test**
Create a script test that simulates release output and verifies updater bundles, `.sig` files, and `latest.json` are collected and named correctly for GitHub Release publishing.

**Step 2: Run test to verify it fails**
Run: `bash scripts/test/release_updater_assets_test.sh`
Expected: FAIL because updater asset handling is missing.

**Step 3: Write minimal implementation**
Extend release scripts/workflows to build signed updater artifacts and upload them with the release.

**Step 4: Run test to verify it passes**
Run: `bash scripts/test/release_updater_assets_test.sh`
Expected: PASS.

**Step 5: Commit**
```bash
git add scripts/test .github/workflows scripts/desktop
git commit -m "ci(release): publish updater artifacts"
```

### Task 5: Verify end-to-end behavior and document operational prerequisites

**Files:**
- Modify: `README.md`
- Modify: `docs/README.zh-CN.md`
- Modify: `docs/plans/2026-03-11-github-update-check-design.md` if implementation changes assumptions

**Step 1: Run focused frontend tests**
Run: `npm --prefix frontend run test -- updateService && npm --prefix frontend run test -- UpdateCard`
Expected: PASS.

**Step 2: Run desktop tests**
Run: `cargo test -q` in `desktop/src-tauri`
Expected: PASS.

**Step 3: Run script tests**
Run: `bash scripts/test/release_version_helpers_test.sh && bash scripts/test/sync_release_metadata_test.sh && bash scripts/test/collect_release_assets_test.sh && bash scripts/test/package_local_release_test.sh && bash scripts/test/release_updater_assets_test.sh`
Expected: PASS.

**Step 4: Perform local build verification**
Run the desktop release build with signing env vars available and verify updater artifacts plus `latest.json` are produced.
Expected: Signed updater bundles and metadata present.

**Step 5: Commit**
```bash
git add README.md docs/README.zh-CN.md docs/plans/2026-03-11-github-update-check-design.md
git commit -m "docs(updates): document GitHub updater setup"
```
