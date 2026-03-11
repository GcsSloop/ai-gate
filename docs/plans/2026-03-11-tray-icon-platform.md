# Tray Icon Platform Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Load platform-appropriate tray icons so macOS uses a template tray icon and Windows uses a color tray icon.

**Architecture:** Keep the existing tray setup flow in `main.rs`, but extract tray icon selection into small helper functions that choose embedded bytes by platform. On macOS, also set the builder's template/icon-as-template behavior so the white source asset is rendered correctly in both light and dark menu bars.

**Tech Stack:** Rust, Tauri 2, desktop assets, Rust unit tests

---

### Task 1: Add failing unit tests for platform tray icon selection

**Files:**
- Modify: `desktop/src-tauri/src/main.rs`
- Test: `desktop/src-tauri/src/main.rs`

**Step 1: Write the failing test**
Add unit tests for helper functions that assert macOS selects the tray template asset path/bytes and Windows selects the color tray asset path/bytes.

**Step 2: Run test to verify it fails**
Run: `cargo test tray_icon_ -q`
Expected: FAIL because the helper functions do not exist yet.

**Step 3: Write minimal implementation**
Extract helper functions for tray icon bytes/config, wire them into `setup_tray`, and enable template behavior only on macOS.

**Step 4: Run test to verify it passes**
Run: `cargo test tray_icon_ -q`
Expected: PASS.

### Task 2: Add dedicated tray icon assets

**Files:**
- Create/Update: `desktop/src-tauri/icons/tray-icon-template.png`
- Create/Update: `desktop/src-tauri/icons/tray-icon-color.png`
- Source: `assets/aigate-128x128-white.png`
- Source: `assets/aigate-128x128-color.png`

**Step 1: Copy platform-specific tray icon source files into the desktop icon directory**
Preserve source files in `assets/` and create tray-specific copies in Tauri's icon directory.

**Step 2: Verify files exist**
Run: `ls desktop/src-tauri/icons/tray-icon-template.png desktop/src-tauri/icons/tray-icon-color.png`
Expected: Both files present.

### Task 3: Run focused verification

**Files:**
- Modify: `desktop/src-tauri/src/main.rs`
- Test: `desktop/src-tauri/src/main.rs`

**Step 1: Run Rust tray tests**
Run: `cargo test tray_icon_ -q`
Expected: PASS.

**Step 2: Run full desktop Rust tests**
Run: `cargo test -q`
Expected: PASS.

**Step 3: Commit**
```bash
git add docs/plans/2026-03-11-tray-icon-platform-design.md docs/plans/2026-03-11-tray-icon-platform.md desktop/src-tauri/src/main.rs desktop/src-tauri/icons/tray-icon-template.png desktop/src-tauri/icons/tray-icon-color.png
git commit -m "fix(desktop): split tray icons by platform"
```
