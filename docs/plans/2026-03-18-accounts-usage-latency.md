# Accounts Usage Latency Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make `GET /ai-router/api/accounts/usage` return quickly by reading cached snapshots only, and move online refresh to an explicit refresh path.

**Architecture:** Remove synchronous official usage refresh from the read endpoint. Keep the existing snapshot repository as the source of truth for list reads, and add a dedicated refresh action that reuses the driver-based usage refresh flow with a bounded request context.

**Tech Stack:** Go, net/http, SQLite repository layer, existing usage refresh orchestrator, handler tests.

---

### Task 1: Lock in non-blocking list behavior

**Files:**
- Modify: `backend/internal/api/accounts_handler_test.go`

**Step 1: Write the failing test**

Add a handler test proving `GET /accounts/usage` returns cached snapshot data without making any outbound usage refresh request.

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/api -run TestAccountsHandlerListUsageReadsCachedSnapshotsOnly -count=1`
Expected: FAIL because the handler currently refreshes official usage inline.

**Step 3: Write minimal implementation**

Remove the inline refresh call from `listAccountsUsage`.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/api -run TestAccountsHandlerListUsageReadsCachedSnapshotsOnly -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/api/accounts_handler.go backend/internal/api/accounts_handler_test.go
git commit -m "fix(accounts): avoid synchronous refresh in usage list"
```

### Task 2: Add explicit refresh endpoint

**Files:**
- Modify: `backend/internal/api/accounts_handler.go`
- Modify: `backend/internal/api/accounts_handler_test.go`

**Step 1: Write the failing test**

Add a handler test for `POST /accounts/usage/refresh` that refreshes snapshots and returns quickly with updated usage data.

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/api -run TestAccountsHandlerRefreshUsage -count=1`
Expected: FAIL because the route does not exist.

**Step 3: Write minimal implementation**

Add a refresh endpoint that:
- lists accounts
- refreshes snapshots through a shared refresh helper
- uses `context.WithTimeout`
- returns `204 No Content`

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/api -run TestAccountsHandlerRefreshUsage -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/api/accounts_handler.go backend/internal/api/accounts_handler_test.go
git commit -m "feat(accounts): add explicit usage refresh endpoint"
```

### Task 3: Final verification

**Files:**
- No code changes required unless verification finds regressions

**Step 1: Run backend API tests**

Run: `cd backend && go test ./internal/api -count=1`
Expected: PASS

**Step 2: Run full backend tests**

Run: `cd backend && go test ./... -count=1`
Expected: PASS

**Step 3: Review worktree**

Run: `git status --short --branch`
Expected: only intended files changed
