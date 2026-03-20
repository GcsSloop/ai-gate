# Usage Timeout Settings Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a configurable usage refresh timeout in settings and apply it immediately without restarting the backend.

**Architecture:** Persist `usage_request_timeout_seconds` in `app_settings`, expose it through `/settings/app`, and have both the manual refresh handler and the background usage refresh orchestrator read the latest value from the settings repository on each run. Surface the setting in the existing Settings page account-status card.

**Tech Stack:** Go backend, SQLite settings repository, React + Ant Design frontend, Vitest, Go test.

---

### Task 1: Persist the new app setting
- Modify `backend/internal/settings/repository.go`
- Modify `backend/internal/store/sqlite/migrations.go`
- Modify `backend/internal/store/sqlite/store.go`
- Test `backend/internal/settings/repository_test.go`

### Task 2: Apply the timeout dynamically in refresh flows
- Modify `backend/internal/usage/refresh/orchestrator.go`
- Modify `backend/internal/api/accounts_handler.go`
- Modify `backend/internal/bootstrap/bootstrap.go`
- Test `backend/internal/usage/refresh/orchestrator_test.go`
- Test `backend/internal/api/settings_handler_test.go`
- Test `backend/internal/api/accounts_handler_internal_test.go`

### Task 3: Expose and edit the setting in the frontend
- Modify `frontend/src/lib/api.ts`
- Modify `frontend/src/features/settings/SettingsPage.tsx`
- Test `frontend/src/features/settings/SettingsPage.test.tsx`

### Task 4: Verify end-to-end behavior
- Run `go test ./...`
- Run `npm test -- --run src/features/settings/SettingsPage.test.tsx`
- Run `npm test`
