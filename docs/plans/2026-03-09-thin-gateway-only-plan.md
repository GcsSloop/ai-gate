# Thin Gateway Only Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Remove thick-gateway compatibility paths so the router only exposes and executes thin-gateway behavior.

**Architecture:** Collapse `ResponsesHandler` to a thin proxy surface: `POST /responses` and `GET /models` only. Delete local fallback, synthetic response APIs, and account-level fallback configuration so the backend, frontend, tests, and docs all describe one behavior. Preserve audit logging and account management, but remove any protocol semantics owned by the gateway.

**Tech Stack:** Go HTTP handlers, SQLite-backed account repository, React/Vitest frontend, Go tests.

---

### Task 1: Remove account fallback configuration

**Files:**
- Modify: `backend/internal/accounts/types.go`
- Modify: `backend/internal/accounts/repository.go`
- Modify: `backend/internal/store/sqlite/migrations.go`
- Modify: `backend/internal/store/sqlite/store.go`
- Modify: `backend/internal/api/accounts_handler.go`
- Modify: `backend/internal/api/accounts_handler_test.go`
- Modify: `frontend/src/lib/api.ts`
- Modify: `frontend/src/features/accounts/AccountsPage.tsx`
- Modify: `frontend/src/features/accounts/AccountsPage.test.tsx`

**Step 1: Write the failing tests**
- Add backend assertions that account APIs no longer expose `allow_chat_fallback`.
- Add frontend assertions that no fallback control is rendered or submitted.

**Step 2: Run test to verify it fails**
- Run: `cd backend && go test ./internal/api -run 'TestAccountsHandler' -count=1`
- Run: `npm --prefix frontend test -- AccountsPage`
- Expected: FAIL because fallback fields still exist.

**Step 3: Write minimal implementation**
- Remove `AllowChatFallback` from account types, storage, API payloads, and frontend forms.
- Keep `/responses` support toggle only.

**Step 4: Run test to verify it passes**
- Run the same commands.
- Expected: PASS.

**Step 5: Commit**
- `git add backend/internal/accounts backend/internal/api/accounts_handler.go backend/internal/api/accounts_handler_test.go backend/internal/store/sqlite frontend/src/lib/api.ts frontend/src/features/accounts/AccountsPage.tsx frontend/src/features/accounts/AccountsPage.test.tsx`
- `git commit -m "refactor: remove fallback account settings"`

### Task 2: Remove thick `/responses` code paths

**Files:**
- Modify: `backend/internal/api/responses_handler.go`
- Modify: `backend/internal/api/responses_handler_test.go`
- Modify: `backend/internal/api/responses_compat_test.go`
- Modify: `backend/internal/api/responses_compact_test.go`
- Modify: `backend/internal/api/contracts/thin_gateway_contract_test.go`

**Step 1: Write the failing tests**
- Add or update tests asserting unsupported endpoints return `404` or thin-mode unsupported behavior consistently.
- Add tests asserting `/responses` never falls back to `/chat/completions` and unsupported accounts fail directly.

**Step 2: Run test to verify it fails**
- Run: `cd backend && go test ./internal/api -run 'TestResponsesHandlerThinModeDisablesSyntheticEndpoints|TestResponsesHandlerThinModeNoChatFallback' -count=1`
- Expected: FAIL while thick code paths still exist.

**Step 3: Write minimal implementation**
- Remove local response detail/input_items/cancel/delete/compact/input_tokens handlers.
- Remove fallback execution branches and synthetic response assembly.
- Leave only thin `/responses` passthrough and `/models` passthrough.

**Step 4: Run test to verify it passes**
- Run the same command plus `cd backend && go test ./internal/api/contracts -count=1`.
- Expected: PASS.

**Step 5: Commit**
- `git add backend/internal/api/responses_handler.go backend/internal/api/responses_handler_test.go backend/internal/api/responses_compat_test.go backend/internal/api/responses_compact_test.go backend/internal/api/contracts/thin_gateway_contract_test.go`
- `git commit -m "refactor: remove thick responses compatibility paths"`

### Task 3: Trim docs and final verification

**Files:**
- Modify: `README.md`
- Modify: `docs/thin-gateway-mode.md`
- Modify: `docs/plans/2026-03-09-thin-gateway-third-party-responses.md` if needed for consistency notes

**Step 1: Update docs**
- Remove fallback and synthetic endpoint references.
- State that only thin gateway behavior remains.

**Step 2: Run verification**
- Run: `cd backend && go test ./... -count=1`
- Run: `npm --prefix frontend test -- AccountsPage`
- Expected: PASS.

**Step 3: Final status check**
- Run: `git status --short`
- Expected: clean worktree.

**Step 4: Commit**
- `git add README.md docs/thin-gateway-mode.md docs/plans/2026-03-09-thin-gateway-third-party-responses.md`
- `git commit -m "docs: align project with thin gateway only mode"`
