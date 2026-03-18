# Update Download Backend Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Move desktop update download ownership into the Tauri host so frontend refreshes preserve the active update task view, duplicate downloads are prevented, and users can cancel a running download.

**Architecture:** Add a Rust-side update manager that owns the active updater task and exposes a serialized state snapshot through Tauri commands. Refactor the frontend update service and both update views to read and control that shared host-managed state instead of holding download progress in local React memory.

**Tech Stack:** Rust, Tauri 2 updater plugin, React, TypeScript, Vitest

---

### Task 1: Define the host-managed update contract

**Files:**
- Modify: `desktop/src-tauri/src/main.rs`
- Modify: `frontend/src/features/updates/updateService.ts`
- Test: `frontend/src/features/updates/updateService.test.ts`

**Step 1: Write the failing frontend contract tests**

Add tests that describe a host-managed service contract:

- `getState()` returns an existing `downloading` snapshot on first load
- `downloadAndInstall()` delegates to a host command instead of a frontend-held closure
- `cancelDownload()` exists and delegates to the host

**Step 2: Run test to verify it fails**

Run: `cd frontend && npm test -- --run src/features/updates/updateService.test.ts`
Expected: FAIL because the service does not yet expose host-managed state methods.

**Step 3: Add the minimal TypeScript contract**

Introduce:

- a serializable `DesktopUpdateState`
- command-backed adapter methods for `getState`, `check`, `downloadAndInstall`, `cancelDownload`, `relaunch`

Keep the old direct-updater implementation only as an internal desktop host detail on the Rust side, not in the frontend.

**Step 4: Run test to verify it passes**

Run: `cd frontend && npm test -- --run src/features/updates/updateService.test.ts`
Expected: PASS.

### Task 2: Add Rust-side update manager state helpers

**Files:**
- Modify: `desktop/src-tauri/src/main.rs`
- Test: `desktop/src-tauri/src/main.rs`

**Step 1: Write the failing Rust unit tests**

Add focused tests for:

- initial idle state
- progress updates producing expected percent/transferred values
- duplicate active version requests reusing current task state
- cancellation moving the task to `cancelled`

**Step 2: Run test to verify it fails**

Run: `cd desktop/src-tauri && cargo test update_manager_ -q`
Expected: FAIL because the manager helpers do not exist.

**Step 3: Implement the state types and helpers**

Add:

- serializable update metadata payload
- serializable progress payload
- serializable task state enum/string form
- `UpdateManagerState` with helper methods to mutate/check task state

Keep the implementation pure where possible so tests stay fast.

**Step 4: Run test to verify it passes**

Run: `cd desktop/src-tauri && cargo test update_manager_ -q`
Expected: PASS.

### Task 3: Wire Tauri commands and background task execution

**Files:**
- Modify: `desktop/src-tauri/src/main.rs`
- Test: `desktop/src-tauri/src/main.rs`

**Step 1: Write the failing Rust command-level tests**

Add tests or command-adjacent helper tests that prove:

- checking for updates stores latest metadata into shared state
- starting a second download for the same version does not create a new task
- cancelling an active task flips shared state

**Step 2: Run test to verify it fails**

Run: `cd desktop/src-tauri && cargo test update_command_ -q`
Expected: FAIL because commands/background orchestration are missing.

**Step 3: Implement the commands**

Add Tauri commands:

- `get_update_state`
- `check_for_app_update`
- `start_update_download`
- `cancel_update_download`
- `relaunch_after_update`

Register them in `generate_handler!`. Spawn the background download task from the host and update the shared state during progress events.

**Step 4: Run test to verify it passes**

Run: `cd desktop/src-tauri && cargo test update_ -q`
Expected: PASS.

### Task 4: Refactor update views to hydrate from shared host state

**Files:**
- Modify: `frontend/src/features/updates/UpdateCard.tsx`
- Modify: `frontend/src/features/updates/HomeUpdatePanel.tsx`
- Modify: `frontend/src/features/updates/UpdateCard.test.tsx`
- Modify: `frontend/src/App.test.tsx`

**Step 1: Write the failing component tests**

Add tests that prove:

- a pre-existing `downloading` state renders on first paint
- the UI shows `取消下载` while downloading
- remounting the component preserves the rendered host-managed progress instead of resetting to zero

**Step 2: Run test to verify it fails**

Run: `cd frontend && npm test -- --run src/features/updates/UpdateCard.test.tsx src/App.test.tsx`
Expected: FAIL because the components still own local download state.

**Step 3: Implement the minimal component changes**

Update both views to:

- hydrate via `getState()`
- poll while active
- call `cancelDownload()`
- stop holding a private frontend-only download closure

Use one shared mapping from host state to UI text.

**Step 4: Run test to verify it passes**

Run: `cd frontend && npm test -- --run src/features/updates/UpdateCard.test.tsx src/App.test.tsx`
Expected: PASS.

### Task 5: Verify end-to-end touched scope

**Files:**
- Modify: `desktop/src-tauri/src/main.rs`
- Modify: `frontend/src/features/updates/updateService.ts`
- Modify: `frontend/src/features/updates/UpdateCard.tsx`
- Modify: `frontend/src/features/updates/HomeUpdatePanel.tsx`
- Modify: tests touched above
- Create: `docs/plans/2026-03-18-update-download-backend-design.md`
- Create: `docs/plans/2026-03-18-update-download-backend.md`

**Step 1: Run targeted frontend verification**

Run: `cd frontend && npm test -- --run src/features/updates/updateService.test.ts src/features/updates/UpdateCard.test.tsx src/App.test.tsx`
Expected: PASS.

**Step 2: Run targeted desktop verification**

Run: `cd desktop/src-tauri && cargo test update_ -q`
Expected: PASS.

**Step 3: Run repository diff hygiene**

Run: `git diff --check`
Expected: no output.

**Step 4: Commit**

```bash
git add docs/plans/2026-03-18-update-download-backend-design.md docs/plans/2026-03-18-update-download-backend.md desktop/src-tauri/src/main.rs frontend/src/features/updates/updateService.ts frontend/src/features/updates/UpdateCard.tsx frontend/src/features/updates/HomeUpdatePanel.tsx frontend/src/features/updates/updateService.test.ts frontend/src/features/updates/UpdateCard.test.tsx frontend/src/App.test.tsx
git commit -m "feat(update): move download state into desktop host"
```
