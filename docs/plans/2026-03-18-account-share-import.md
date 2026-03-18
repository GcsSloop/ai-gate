# Account Share Import Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a backend-owned account share/export endpoint and a strict import endpoint, then wire minimal frontend share and import flows that never expose credentials in the UI.

**Architecture:** The backend defines a versioned portable share-package schema, exports it from an existing account record, and validates it on import before creating a fresh account. The frontend only confirms share intent, copies the backend-issued payload to the clipboard, and lets users paste that payload into an import modal.

**Tech Stack:** Go, net/http, React, TypeScript, Ant Design, Vitest

---

### Task 1: Define and test the backend share package contract

**Files:**
- Modify: `backend/internal/api/accounts_handler_test.go`
- Modify: `backend/internal/api/accounts_handler.go`

**Step 1: Write the failing backend tests**

Add tests that prove:

- `POST /accounts/{id}/share` returns a JSON payload string
- the payload has `kind = "aigate-account-share"` and `schema_version = 1`
- the exported account contains the credential and portable fields, but not runtime-only fields

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/api -run TestAccountsHandlerShare -count=1`
Expected: FAIL because the endpoint and share schema do not exist.

**Step 3: Implement the minimal backend export logic**

Add share-package structs and a `shareAccount` handler that:

- parses the account id from the route
- loads the account from the repository
- applies built-in defaults
- serializes the standardized payload string
- returns `200 OK` with `{"payload":"..."}` 

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/api -run TestAccountsHandlerShare -count=1`
Expected: PASS.

**Step 5: Commit**

```bash
git add backend/internal/api/accounts_handler.go backend/internal/api/accounts_handler_test.go
git commit -m "feat(accounts): add share payload export"
```

### Task 2: Add strict shared-import validation and creation

**Files:**
- Modify: `backend/internal/api/accounts_handler_test.go`
- Modify: `backend/internal/api/accounts_handler.go`

**Step 1: Write the failing backend import tests**

Add tests that prove:

- `POST /accounts/import-shared` accepts a valid payload and creates a new account
- imported accounts reset `status`, `priority`, and `is_active`
- invalid `kind`, invalid `schema_version`, invalid `base_url`, and malformed `usage_config_json` return `400`

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/api -run 'TestAccountsHandler(ImportShared|Share)' -count=1`
Expected: FAIL because the import endpoint and validation do not exist.

**Step 3: Implement the minimal backend import path**

Add:

- request structs for `payload`
- validation helpers for provider, auth mode, base URL, and `usage_config_json`
- import handler that creates a fresh active account without mutating existing records

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/api -run 'TestAccountsHandler(ImportShared|Share)' -count=1`
Expected: PASS.

**Step 5: Commit**

```bash
git add backend/internal/api/accounts_handler.go backend/internal/api/accounts_handler_test.go
git commit -m "feat(accounts): add shared account import"
```

### Task 3: Add frontend API client helpers and page tests for share/import

**Files:**
- Modify: `frontend/src/lib/api.ts`
- Modify: `frontend/src/features/accounts/AccountsPage.test.tsx`
- Modify: `frontend/src/features/accounts/AccountsPage.tsx`

**Step 1: Write the failing frontend tests**

Add tests that prove:

- canceling the share modal does not call the backend or clipboard
- confirming share calls the backend share endpoint and copies the returned payload
- importing a pasted payload calls the import endpoint
- failed import surfaces the backend error without closing the modal

**Step 2: Run test to verify it fails**

Run: `cd frontend && npm test -- --run src/features/accounts/AccountsPage.test.tsx`
Expected: FAIL because the page does not yet expose share/import flows.

**Step 3: Implement the minimal frontend flow**

Add API helpers:

- `shareAccount(id)`
- `importSharedAccount(payload)`

Update the accounts page to:

- render a share action button
- show a minimal confirmation modal
- call `navigator.clipboard.writeText` only after confirm
- add an import entry in the add-account menu
- show a paste modal and submit its payload to the backend

Keep the UI free of secret previews.

**Step 4: Run test to verify it passes**

Run: `cd frontend && npm test -- --run src/features/accounts/AccountsPage.test.tsx`
Expected: PASS.

**Step 5: Commit**

```bash
git add frontend/src/lib/api.ts frontend/src/features/accounts/AccountsPage.tsx frontend/src/features/accounts/AccountsPage.test.tsx
git commit -m "feat(accounts): add share and import flow"
```

### Task 4: Verify the touched scope

**Files:**
- Modify: `backend/internal/api/accounts_handler.go`
- Modify: `backend/internal/api/accounts_handler_test.go`
- Modify: `frontend/src/lib/api.ts`
- Modify: `frontend/src/features/accounts/AccountsPage.tsx`
- Modify: `frontend/src/features/accounts/AccountsPage.test.tsx`
- Create: `docs/plans/2026-03-18-account-share-import-design.md`
- Create: `docs/plans/2026-03-18-account-share-import.md`

**Step 1: Run backend verification**

Run: `cd backend && go test ./internal/api -count=1`
Expected: PASS.

**Step 2: Run frontend verification**

Run: `cd frontend && npm test -- --run src/features/accounts/AccountsPage.test.tsx`
Expected: PASS.

**Step 3: Run diff hygiene**

Run: `git diff --check`
Expected: no output.

**Step 4: Commit docs if still unstaged**

```bash
git add docs/plans/2026-03-18-account-share-import-design.md docs/plans/2026-03-18-account-share-import.md
git commit -m "docs: add account share and import plan"
```
