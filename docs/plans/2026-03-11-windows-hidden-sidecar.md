# Windows Hidden Sidecar Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Hide the Windows backend sidecar console window while preserving the existing macOS-style tray and shutdown semantics.

**Architecture:** Keep the current Tauri sidecar lifecycle. Add a Windows-only process-spawn helper in the desktop shell so the backend launches without a console window, while retaining the current close-to-tray and explicit-exit paths.

**Tech Stack:** Rust, Tauri 2, std::process, Windows process creation flags

---

### Task 1: Add Windows-only sidecar spawn configuration

**Files:**
- Modify: `desktop/src-tauri/src/main.rs`
- Test: `desktop/src-tauri/src/main.rs`

**Step 1: Write the failing test**

Add a Windows-only unit test that creates a `Command`, applies the helper, and asserts the command carries the `CREATE_NO_WINDOW` flag.

**Step 2: Run test to verify it fails**

Run: `cargo test hidden_sidecar`
Expected: FAIL because the helper does not exist yet.

**Step 3: Write minimal implementation**

Add:

- a Windows-only helper that imports `std::os::windows::process::CommandExt`
- a `const CREATE_NO_WINDOW: u32 = 0x0800_0000`
- a small `configure_sidecar_command(&mut Command)` helper
- a no-op non-Windows variant

Call the helper inside `spawn_sidecar()` before `spawn()`.

**Step 4: Run test to verify it passes**

Run: `cargo test hidden_sidecar`
Expected: PASS

**Step 5: Commit**

```bash
git add desktop/src-tauri/src/main.rs
git commit -m "feat: hide windows sidecar console"
```

### Task 2: Verify lifecycle semantics stay intact

**Files:**
- Modify: `desktop/src-tauri/src/main.rs`
- Test: `desktop/src-tauri/src/main.rs`

**Step 1: Write/adjust regression tests**

Keep the existing tests covering:

- `window_close_action(true) == MinimizeWindow`
- `window_close_action(false) == ExitApp`

Add a focused test for the spawn helper on non-Windows so the helper remains a no-op on macOS/Linux builds.

**Step 2: Run tests to verify expected behavior**

Run: `cargo test window_close`
Expected: PASS for existing lifecycle behavior

Run: `cargo test sidecar`
Expected: PASS for helper behavior

**Step 3: Keep implementation minimal**

Do not change `shutdown_sidecar()` or tray exit handling unless tests prove a regression.

**Step 4: Run tests to verify full green**

Run: `cargo test`
Expected: PASS

**Step 5: Commit**

```bash
git add desktop/src-tauri/src/main.rs
git commit -m "test: cover windows hidden sidecar lifecycle"
```

### Task 3: Verify the desktop app still builds

**Files:**
- Modify: none unless build failures require a small fix

**Step 1: Build desktop bundle**

Run: `npm --prefix desktop run tauri build`
Expected: successful desktop build

**Step 2: Verify outcome**

Confirm the app bundle is produced and no lifecycle code was broken by the Windows-specific helper.

**Step 3: Commit final verification state**

If no further code changes were required, no extra commit is necessary.
