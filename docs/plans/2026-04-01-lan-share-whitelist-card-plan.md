# LAN Share Whitelist Card Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the LAN share IP whitelist textarea with a dedicated whitelist card that has its own enable switch and modal-based CRUD flows.

**Architecture:** Add a separate persisted boolean flag to distinguish disabled whitelist mode from enabled-but-empty mode. Keep the stored whitelist as newline-delimited text in backend settings, and let the frontend translate between text storage and list editing UI.

**Tech Stack:** Go, SQLite, React, TypeScript, Ant Design, Vitest.

---

### Task 1: Add failing backend tests

**Files:**
- Modify: `backend/internal/bootstrap/lan_share_access_test.go`
- Modify: `backend/internal/settings/repository_test.go`
- Modify: `backend/internal/api/settings_handler_test.go`

**Step 1: Write failing tests**
- Add coverage for `LANShareWhitelistEnabled` persistence.
- Add coverage for `whitelist enabled + empty list => forbidden`.
- Add coverage for `whitelist disabled + empty list => allowed`.

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/bootstrap ./internal/settings ./internal/api`
Expected: build/test failure because `LANShareWhitelistEnabled` does not exist yet.

### Task 2: Implement backend support

**Files:**
- Modify: `backend/internal/settings/repository.go`
- Modify: `backend/internal/store/sqlite/migrations.go`
- Modify: `backend/internal/store/sqlite/store.go`
- Modify: `backend/internal/bootstrap/bootstrap.go`

**Step 1: Add persisted setting field**
- Add `LANShareWhitelistEnabled` to `AppSettings`.
- Add DB column and migration compatibility.
- Read/write it through repository.

**Step 2: Implement access-control behavior**
- If LAN share is off: bypass whitelist.
- If whitelist is off: allow remote LAN clients.
- If whitelist is on and empty: reject non-loopback.
- If whitelist is on and populated: enforce entries.

**Step 3: Run tests**

Run: `cd backend && go test ./internal/bootstrap ./internal/settings ./internal/api`
Expected: PASS.

### Task 3: Add failing frontend tests

**Files:**
- Modify: `frontend/src/features/settings/SettingsPage.test.tsx`

**Step 1: Write failing tests**
- Assert whitelist card only appears after LAN share is enabled.
- Assert save payload includes `lan_share_whitelist_enabled`.
- Assert whitelist items can be created, edited, and deleted through modals.

**Step 2: Run test to verify it fails**

Run: `cd frontend && npm test -- --run SettingsPage`
Expected: FAIL because the new card and modal actions do not exist yet.

### Task 4: Implement frontend card and modal CRUD

**Files:**
- Modify: `frontend/src/features/settings/SettingsPage.tsx`
- Modify: `frontend/src/lib/api.ts`
- Modify: `frontend/src/styles.css`

**Step 1: Add settings field support**
- Add `lan_share_whitelist_enabled` to `AppSettings` typing.
- Default it to `false` in settings page state.

**Step 2: Replace textarea UI**
- Remove textarea-based whitelist editing from proxy card.
- Show new whitelist card only when LAN share is enabled.
- Add switch, list rows, and empty state.

**Step 3: Add modal CRUD**
- Add create/edit modal with one IP input.
- Add delete confirm modal.
- Serialize list back to newline-delimited text.

**Step 4: Run tests**

Run: `cd frontend && npm test -- --run SettingsPage`
Expected: PASS.

### Task 5: Run focused verification

**Files:**
- No code changes required

**Step 1: Run backend verification**

Run: `cd backend && go test ./internal/bootstrap ./internal/settings ./internal/api`
Expected: PASS.

**Step 2: Run frontend verification**

Run: `cd frontend && npm test -- --run SettingsPage`
Expected: PASS.
