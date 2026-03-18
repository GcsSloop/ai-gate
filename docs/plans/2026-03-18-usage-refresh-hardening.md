# Usage Refresh Hardening Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make account usage refresh start automatically on backend startup and keep running even if the sidecar or a background job exits unexpectedly.

**Architecture:** Run one refresh pass immediately when the backend boots, then continue on the existing interval inside a protected scheduler loop that cannot die silently on panic. In the desktop app, add a sidecar exit watcher that restarts `routerd` when it exits unexpectedly, while preserving the existing on-demand request recovery path.

**Tech Stack:** Go backend, Rust/Tauri desktop runtime, existing HTTP/API integration tests.

---

### Task 1: Immediate Backend Refresh

**Files:**
- Modify: `backend/internal/bootstrap/bootstrap.go`
- Test: `backend/internal/bootstrap/bootstrap_test.go`

**Step 1: Write the failing test**

Add a test that creates an account with a long scheduler interval, then verifies `/ai-router/api/accounts/usage` is populated without waiting for the first ticker cycle.

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/bootstrap -run TestNewAppRefreshesUsageImmediatelyOnStartup -count=1`

Expected: FAIL because refresh only runs after the first scheduler tick.

**Step 3: Write minimal implementation**

Refactor the background scheduler startup so it performs one immediate run of recovery/refresh/compaction/backup before entering the ticker loop.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/bootstrap -run TestNewAppRefreshesUsageImmediatelyOnStartup -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/bootstrap/bootstrap.go backend/internal/bootstrap/bootstrap_test.go
git commit -m "fix: refresh usage on backend startup"
```

### Task 2: Protect Scheduler Loop

**Files:**
- Modify: `backend/internal/bootstrap/bootstrap.go`
- Test: `backend/internal/bootstrap/bootstrap_test.go`

**Step 1: Write the failing test**

Add a focused unit test around the scheduler runner helper that simulates a panicking job and verifies subsequent iterations still execute.

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/bootstrap -run TestBackgroundLoopRecoversFromPanics -count=1`

Expected: FAIL because a panic currently kills the goroutine.

**Step 3: Write minimal implementation**

Wrap each background cycle with `defer recover()` and log the panic so the loop keeps running.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/bootstrap -run TestBackgroundLoopRecoversFromPanics -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/bootstrap/bootstrap.go backend/internal/bootstrap/bootstrap_test.go
git commit -m "fix: keep background scheduler alive after panic"
```

### Task 3: Auto-Restart Desktop Sidecar

**Files:**
- Modify: `desktop/src-tauri/src/main.rs`
- Test: `desktop/src-tauri/src/main.rs`

**Step 1: Write the failing test**

Add unit coverage for the restart policy helper so unexpected exits trigger recovery while intentional shutdown/restart paths do not.

**Step 2: Run test to verify it fails**

Run: `cd desktop/src-tauri && cargo test sidecar_exit -- --nocapture`

Expected: FAIL because there is no exit watcher/restart policy yet.

**Step 3: Write minimal implementation**

Track intentional sidecar shutdown reasons, spawn a watcher thread after launching the child, and restart the sidecar when it exits unexpectedly.

**Step 4: Run test to verify it passes**

Run: `cd desktop/src-tauri && cargo test sidecar_exit -- --nocapture`

Expected: PASS

**Step 5: Commit**

```bash
git add desktop/src-tauri/src/main.rs
git commit -m "fix: restart sidecar after unexpected exit"
```

### Task 4: Verify End-to-End Behavior

**Files:**
- Modify: none

**Step 1: Run backend tests**

Run: `cd backend && go test ./internal/bootstrap -count=1`

Expected: PASS

**Step 2: Run desktop tests**

Run: `cd desktop/src-tauri && cargo test sidecar -- --nocapture`

Expected: PASS

**Step 3: Run diff hygiene**

Run: `git diff --check`

Expected: no output
