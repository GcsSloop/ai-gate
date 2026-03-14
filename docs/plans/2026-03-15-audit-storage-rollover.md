# Audit Storage Rollover Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add configurable audit-message rollover so new and historical thin-audit records are compacted into summaries, reducing database and backup size without breaking dashboard or usage flows.

**Architecture:** Extend `messages` storage with summary columns and app settings for per-type raw-retention ceilings. Route all message inserts through a compacting repository path, then add a startup/manual optimizer that rewrites oversized historical rows into summaries and vacuums the database after changes.

**Tech Stack:** Go, SQLite, Tauri-backed settings API, React/Ant Design settings UI, Go/Vitest tests.

---

### Task 1: Expand settings and schema for audit rollover

**Files:**
- Modify: `backend/internal/store/sqlite/migrations.go`
- Modify: `backend/internal/store/sqlite/store.go`
- Modify: `backend/internal/settings/repository.go`
- Modify: `backend/internal/api/settings_handler.go`
- Modify: `frontend/src/lib/api.ts`
- Test: `backend/internal/api/settings_handler_test.go`

**Step 1: Write the failing test**
- Add a settings handler test that round-trips the new audit retention fields and expects sane defaults.

**Step 2: Run test to verify it fails**
- Run: `cd backend && go test ./internal/api -run TestSettingsHandlerGetAndPutAppSettings`

**Step 3: Write minimal implementation**
- Add new app settings fields and defaults.
- Persist and sanitize them in the settings repository.
- Expose them through the API contract.
- Add `messages` summary columns through migrations.

**Step 4: Run test to verify it passes**
- Run the same Go test command.

**Step 5: Commit**
- Commit message: `feat: add audit storage settings`

### Task 2: Add message compaction and rollover logic

**Files:**
- Create: `backend/internal/conversations/compaction.go`
- Modify: `backend/internal/conversations/types.go`
- Modify: `backend/internal/conversations/repository.go`
- Modify: `backend/internal/api/responses_handler.go`
- Test: `backend/internal/conversations/repository_test.go`

**Step 1: Write the failing test**
- Add repository tests that seed many messages of one type and expect older ones to be rewritten into summary mode once the per-type limit is exceeded.

**Step 2: Run test to verify it fails**
- Run: `cd backend && go test ./internal/conversations -run TestSQLiteRepositoryCompactsOlderAuditMessages`

**Step 3: Write minimal implementation**
- Add summary metadata fields to `Message`.
- Build type-aware compactors for `message`, `function_call`, `custom_tool_call`, `function_call_output`, `custom_tool_call_output`, and `reasoning`.
- Change `AppendMessage` to accept compaction policy and rewrite oldest rows beyond threshold.

**Step 4: Run test to verify it passes**
- Run the same Go test command.

**Step 5: Commit**
- Commit message: `feat: compact audit message storage`

### Task 3: Add silent historical optimization and manual trigger

**Files:**
- Create: `backend/internal/conversations/optimizer.go`
- Modify: `backend/internal/bootstrap/bootstrap.go`
- Modify: `backend/internal/api/settings_handler.go`
- Test: `backend/internal/conversations/repository_test.go`
- Test: `backend/internal/api/settings_handler_test.go`

**Step 1: Write the failing test**
- Add a test that seeds oversized historical rows, runs the optimizer, and expects summary mode plus reduced blob fields.

**Step 2: Run test to verify it fails**
- Run: `cd backend && go test ./internal/conversations -run TestOptimizeAuditStorageCompactsHistoricalRows`

**Step 3: Write minimal implementation**
- Add a background optimizer entry point used on app startup.
- Add a settings API endpoint to trigger optimization manually.
- Run `VACUUM` after a successful compaction pass.

**Step 4: Run test to verify it passes**
- Run the same Go test command and the settings handler test for the new endpoint.

**Step 5: Commit**
- Commit message: `feat: add audit storage optimizer`

### Task 4: Surface controls in the settings UI

**Files:**
- Modify: `frontend/src/features/settings/SettingsPage.tsx`
- Modify: `frontend/src/features/settings/SettingsPage.test.tsx`
- Modify: `frontend/src/lib/api.ts`

**Step 1: Write the failing test**
- Extend settings page tests to expect the new audit storage card, fields, and manual optimize action.

**Step 2: Run test to verify it fails**
- Run: `cd frontend && npm test -- --run src/features/settings/SettingsPage.test.tsx`

**Step 3: Write minimal implementation**
- Add UI copy and controls for per-type limits.
- Wire save behavior and manual optimize action.
- Keep layout consistent with the existing settings page.

**Step 4: Run test to verify it passes**
- Run the same Vitest command.

**Step 5: Commit**
- Commit message: `feat: add audit storage controls`

### Task 5: Full verification

**Files:**
- No code changes expected

**Step 1: Run focused backend tests**
- Run: `cd backend && go test ./internal/conversations ./internal/api ./internal/bootstrap`

**Step 2: Run focused frontend tests**
- Run: `cd frontend && npm test -- --run src/features/settings/SettingsPage.test.tsx src/App.test.tsx`

**Step 3: Run diff sanity**
- Run: `git diff --check`

**Step 4: Commit final cleanup if needed**
- Commit message: `chore: finalize audit storage rollover`
