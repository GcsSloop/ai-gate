# macOS Sidecar Wake Recovery Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make the desktop app automatically recover the local sidecar after macOS sleep/wake causes the backend process to disappear.

**Architecture:** Keep the existing sidecar heartbeat model, but add self-healing in the desktop shell. When a backend request fails with a sidecar-unavailable signal, the desktop app should restart the sidecar and retry once. On macOS reopen events, proactively run a lightweight backend health check to recover before the user manually toggles anything.

**Tech Stack:** Rust, Tauri 2, local TCP HTTP requests, existing sidecar lifecycle helpers in `desktop/src-tauri/src/main.rs`

---

### Task 1: Add failing tests for backend self-healing

**Files:**
- Modify: `desktop/src-tauri/src/main.rs`
- Test: `desktop/src-tauri/src/main.rs`

**Step 1: Write the failing tests**
- Add tests for a helper that treats backend connection/read timeout and connection refusal as restart-worthy sidecar failures.
- Add a test for a helper that limits automatic retry to one attempt.

**Step 2: Run test to verify it fails**
Run: `cd desktop/src-tauri && cargo test sidecar_recovery_ -q`
Expected: FAIL because the helper functions do not exist yet.

**Step 3: Write minimal implementation**
- Add pure helper functions for restart-worthy backend failures and retry gating.

**Step 4: Run test to verify it passes**
Run: `cd desktop/src-tauri && cargo test sidecar_recovery_ -q`
Expected: PASS.

**Step 5: Commit**
```bash
git add desktop/src-tauri/src/main.rs
 git commit -m "fix(desktop): detect restart-worthy sidecar failures"
```

### Task 2: Add failing tests for request auto-recovery

**Files:**
- Modify: `desktop/src-tauri/src/main.rs`
- Test: `desktop/src-tauri/src/main.rs`

**Step 1: Write the failing test**
- Add a test for a request wrapper that retries once after invoking a restart callback when the first backend request fails with a restart-worthy error.

**Step 2: Run test to verify it fails**
Run: `cd desktop/src-tauri && cargo test sidecar_request_ -q`
Expected: FAIL because the wrapper does not exist yet.

**Step 3: Write minimal implementation**
- Add a small retry wrapper around backend requests.
- Reuse existing `restart_sidecar()` in production, but keep the wrapper callback-based for testability.

**Step 4: Run test to verify it passes**
Run: `cd desktop/src-tauri && cargo test sidecar_request_ -q`
Expected: PASS.

**Step 5: Commit**
```bash
git add desktop/src-tauri/src/main.rs
 git commit -m "fix(desktop): retry backend requests after sidecar restart"
```

### Task 3: Wire self-healing into desktop flows

**Files:**
- Modify: `desktop/src-tauri/src/main.rs`

**Step 1: Apply the wrapper to backend request entrypoints**
- Route normal backend calls through the self-healing request wrapper.
- Keep retry count at one.

**Step 2: Add macOS reopen health recovery**
- On `RunEvent::Reopen`, run a lightweight backend check before showing the window.

**Step 3: Run targeted tests**
Run: `cd desktop/src-tauri && cargo test sidecar_ -q`
Expected: PASS.

**Step 4: Run full desktop tests**
Run: `cd desktop/src-tauri && cargo test -q`
Expected: PASS.

**Step 5: Verify diff hygiene**
Run: `git diff --check`
Expected: no output.

**Step 6: Commit**
```bash
git add desktop/src-tauri/src/main.rs docs/plans/2026-03-12-macos-sidecar-wake-recovery.md
 git commit -m "fix(desktop): recover sidecar after macOS wake"
```
