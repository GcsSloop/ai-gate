# Account Usage Mini Meter Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a minimal account-side usage meter with hover-to-actions behavior, and make the status refresh interval configurable from 5 seconds to 1 hour with a default of 60 seconds.

**Architecture:** Extend the persisted app settings with a new refresh interval field, let the app shell drive periodic account/proxy refresh from that setting, and render a fixed right-side slot on account cards that shows two micro progress bars for official-account rolling limits until hover/focus reveals the action menu.

**Tech Stack:** Go, SQLite, React, TypeScript, Ant Design, Tauri desktop shell.

---

### Task 1: Persist refresh interval in settings

**Files:**
- Modify: `backend/internal/settings/repository.go`
- Modify: `backend/internal/store/sqlite/migrations.go`
- Modify: `backend/internal/store/sqlite/store.go`
- Test: `backend/internal/settings/repository_test.go`
- Test: `backend/internal/api/settings_handler_test.go`

**Step 1: Write failing tests**
- Add repository assertions for default `status_refresh_interval_seconds = 60` and persisted/clamped values.
- Add settings handler assertions that invalid payload values are normalized into the supported range.

**Step 2: Run targeted backend tests to verify failure**
- Run: `cd backend && go test ./internal/settings ./internal/api -run 'TestRepository|TestSettingsHandler' -count=1`

**Step 3: Implement minimal backend support**
- Add `status_refresh_interval_seconds` to `AppSettings`.
- Store it in SQLite schema + additive migration path.
- Clamp values to `5..3600` and default to `60`.

**Step 4: Re-run targeted backend tests**
- Run the same command and confirm green.

### Task 2: Use refresh interval in the app shell

**Files:**
- Modify: `frontend/src/lib/api.ts`
- Modify: `frontend/src/App.tsx`
- Test: `frontend/src/App.test.tsx`

**Step 1: Write failing frontend test**
- Add a test that boots the app with `status_refresh_interval_seconds = 5` and verifies the app increments account refresh state on that cadence.

**Step 2: Run targeted frontend test to verify failure**
- Run: `npm --prefix frontend run test -- src/App.test.tsx`

**Step 3: Implement minimal frontend polling**
- Extend `AppSettings` typing.
- Replace the fixed account/proxy polling cadence with the persisted interval.
- Keep update-check polling unchanged.

**Step 4: Re-run targeted frontend test**
- Run the same command and confirm green.

### Task 3: Render account-side micro usage meters

**Files:**
- Modify: `frontend/src/features/accounts/AccountsPage.tsx`
- Modify: `frontend/src/styles.css`
- Modify: `frontend/src/lib/i18n.ts`
- Test: `frontend/src/features/accounts/AccountsPage.test.tsx`

**Step 1: Write failing account-card test**
- Add a test that feeds official account usage snapshots and expects the right-side `5H` / `7D` micro meter labels to render.

**Step 2: Run targeted account test to verify failure**
- Run: `npm --prefix frontend run test -- src/features/accounts/AccountsPage.test.tsx`

**Step 3: Implement minimal UI**
- Add a fixed `account-side-slot` container.
- Show micro meters by default for official accounts when rolling-limit usage data is present.
- Hide meters and reveal action buttons on hover/focus.
- Collapse back to actions-only on narrow layouts.
- Add a settings control for refresh interval using `InputNumber`.

**Step 4: Re-run targeted account test**
- Run the same command and confirm green.

### Task 4: Final verification

**Files:**
- Verify only

**Step 1: Run focused backend and frontend verification**
- Run: `cd backend && go test ./internal/settings ./internal/api -count=1`
- Run: `npm --prefix frontend run test -- src/App.test.tsx src/features/accounts/AccountsPage.test.tsx`

**Step 2: Run broader smoke verification**
- Run: `bash scripts/ci/run_frontend_unit_tests.sh`

