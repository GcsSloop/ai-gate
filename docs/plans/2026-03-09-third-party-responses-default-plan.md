# Third-Party Responses Default Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Default new third-party accounts to `supports_responses=true` while keeping an explicit edit toggle and preserving opt-out behavior.

**Architecture:** Adjust account creation request handling so omitted `supports_responses` values default to `true` for third-party providers and remain overridable. Keep repository/runtime behavior unchanged for existing records. Update the frontend account form to surface the toggle in edit mode and align tests with the new default.

**Tech Stack:** Go HTTP handlers, SQLite-backed account repository, React + Vitest frontend tests.

---

### Task 1: Backend create-account default

**Files:**
- Modify: `backend/internal/api/accounts_handler.go`
- Test: `backend/internal/api/accounts_handler_test.go`

**Step 1: Write the failing test**
- Add a handler test asserting `POST /accounts` with a third-party payload that omits `supports_responses` persists `supports_responses=true`.
- Add a handler test asserting explicit `"supports_responses": false` still persists `false`.

**Step 2: Run test to verify it fails**
- Run: `cd backend && go test ./internal/api -run 'TestAccountsHandler(CreateThirdPartyDefaultsResponsesSupport|CreateThirdPartyRespectsExplicitResponsesOptOut)' -count=1`
- Expected: FAIL because omitted values currently default to `false`.

**Step 3: Write minimal implementation**
- Change create-account request handling to distinguish omitted vs explicit `false`.
- Keep official accounts forced to `true`.

**Step 4: Run test to verify it passes**
- Run the same command.
- Expected: PASS.

**Step 5: Commit**
- `git add backend/internal/api/accounts_handler.go backend/internal/api/accounts_handler_test.go`
- `git commit -m "feat: default third-party accounts to responses support"`

### Task 2: Frontend edit toggle coverage

**Files:**
- Modify: `frontend/src/features/accounts/AccountsPage.tsx`
- Test: `frontend/src/features/accounts/AccountsPage.test.tsx`
- Optionally modify: `frontend/src/lib/api.ts`

**Step 1: Write the failing test**
- Add or update a test asserting the edit UI shows a `supports_responses` toggle for third-party accounts and that the submitted payload preserves user choice.

**Step 2: Run test to verify it fails**
- Run: `npm --prefix frontend test -- AccountsPage`
- Expected: FAIL if the toggle is missing or submission does not include the chosen value.

**Step 3: Write minimal implementation**
- Ensure the account edit form renders the toggle clearly and submits its value.
- Do not add unrelated UI changes.

**Step 4: Run test to verify it passes**
- Run the same frontend command.
- Expected: PASS.

**Step 5: Commit**
- `git add frontend/src/features/accounts/AccountsPage.tsx frontend/src/features/accounts/AccountsPage.test.tsx frontend/src/lib/api.ts`
- `git commit -m "feat: expose responses support toggle in account editor"`

### Task 3: Verification

**Files:**
- No code changes expected.

**Step 1: Run backend verification**
- Run: `cd backend && go test ./internal/api ./internal/accounts -count=1`
- Expected: PASS.

**Step 2: Run frontend verification**
- Run: `npm --prefix frontend test -- AccountsPage`
- Expected: PASS.

**Step 3: Run full backend suite**
- Run: `cd backend && go test ./... -count=1`
- Expected: PASS.

**Step 4: Final status check**
- Run: `git status --short`
- Expected: clean worktree.
