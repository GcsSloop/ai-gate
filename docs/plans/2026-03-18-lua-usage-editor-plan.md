# Lua Usage Editor Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add per-account Lua usage configuration to the account editor, including reusable script storage, AI skill copy with account context, and a real Lua test workflow.

**Architecture:** Keep account records storing `usage_driver` plus JSON config, but move managed Lua source into dedicated backend-managed script storage. Add backend APIs for script CRUD and test execution, then wire the account editor to edit script references, copy a generated AI skill prompt with current account context, and run real test requests through the existing Lua runtime.

**Tech Stack:** Go, SQLite, React, Ant Design, Vitest, Go `net/http/httptest`.

---

### Task 1: Define managed script storage contract

**Files:**
- Create: `backend/internal/usagedrv/lua/scripts.go`
- Modify: `backend/internal/usagedrv/lua/driver.go`
- Test: `backend/internal/usagedrv/lua/driver_test.go`

**Step 1: Write failing tests**
- Add tests that expect `usage_config_json` to support a managed script key and resolve it to stored source.
- Add tests for invalid script key handling.

**Step 2: Run targeted Go tests and confirm failure**
Run: `go test ./backend/internal/usagedrv/lua ./backend/internal/api -run 'Lua|UsageScript'`
Expected: failure because managed script storage is not implemented.

**Step 3: Implement script storage and resolution**
- Add a small script manager abstraction.
- Resolve `script` config entries like `managed:<key>` into stored source before Lua execution.

**Step 4: Re-run Go tests**
Run: `go test ./backend/internal/usagedrv/lua ./backend/internal/api -run 'Lua|UsageScript'`
Expected: tests pass.

**Step 5: Commit**
`git add backend/internal/usagedrv/lua docs/plans/2026-03-18-lua-usage-editor-plan.md && git commit -m "feat: add managed lua usage scripts"`

### Task 2: Add backend APIs for script CRUD and Lua testing

**Files:**
- Modify: `backend/internal/api/accounts_handler.go`
- Modify: `backend/internal/api/accounts_handler_test.go`
- Modify: `backend/internal/bootstrap/bootstrap.go`
- Modify: `backend/internal/bootstrap/bootstrap_test.go`

**Step 1: Write failing tests**
- Add API tests for loading/saving a managed script.
- Add API tests for Lua test execution using account credentials and returning output.

**Step 2: Run targeted Go tests and confirm failure**
Run: `go test ./backend/internal/api ./backend/internal/bootstrap -run 'Lua|UsageScript'`
Expected: route not found / handler missing failures.

**Step 3: Implement APIs**
- Add endpoints for managed usage script read/write.
- Add an account-scoped Lua test endpoint that executes real runtime logic and returns structured output.
- Register dependencies from bootstrap.

**Step 4: Re-run Go tests**
Run: `go test ./backend/internal/api ./backend/internal/bootstrap -run 'Lua|UsageScript'`
Expected: pass.

**Step 5: Commit**
`git add backend/internal/api backend/internal/bootstrap && git commit -m "feat: add lua usage config APIs"`

### Task 3: Add frontend account editor UI and skill copy flow

**Files:**
- Modify: `frontend/src/features/accounts/AccountsPage.tsx`
- Modify: `frontend/src/features/accounts/AccountsPage.test.tsx`
- Modify: `frontend/src/lib/api.ts`

**Step 1: Write failing frontend tests**
- Add tests for showing Lua config fields in edit modal.
- Add tests for copying the generated skill prompt with account context.
- Add tests for test execution output rendering.

**Step 2: Run targeted frontend tests and confirm failure**
Run: `npm test -- --run frontend/src/features/accounts/AccountsPage.test.tsx`
Expected: failures because the UI and APIs do not exist yet.

**Step 3: Implement UI**
- Add a Lua usage section in the edit modal.
- Include managed script key, Lua textarea, config JSON, copy-skill action, test action, and output panel.
- Load and save managed scripts via the new backend APIs.

**Step 4: Re-run frontend tests**
Run: `npm test -- --run frontend/src/features/accounts/AccountsPage.test.tsx`
Expected: pass.

**Step 5: Commit**
`git add frontend/src/features/accounts/AccountsPage.tsx frontend/src/features/accounts/AccountsPage.test.tsx frontend/src/lib/api.ts && git commit -m "feat: add lua usage editor UI"`

### Task 4: Verify end-to-end behavior

**Files:**
- Modify as needed from previous tasks.

**Step 1: Run focused verification**
Run: `go test ./backend/internal/usagedrv/lua ./backend/internal/api ./backend/internal/bootstrap`
Run: `npm test -- --run frontend/src/features/accounts/AccountsPage.test.tsx`

**Step 2: Run broader regression checks if focused tests pass**
Run: `go test ./...`
Run: `npm test -- --run`

**Step 3: Fix any regressions**
- Keep changes minimal and scoped.

**Step 4: Final review and summary**
- Confirm API shapes, managed script persistence, and UI copy/test workflow.
