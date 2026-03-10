# Settings Localization And Theme Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add persisted runtime language switching and runtime theme switching to AI Gate, both controlled from Settings and applied immediately without manual save.

**Architecture:** Extend the existing `app_settings` persistence model with `language` and `theme_mode`, then drive the renderer through app-level state. Keep manual save for the existing settings, but use dedicated auto-save handlers for language and theme controls.

**Tech Stack:** Go, SQLite, React 19, Ant Design 6, Vitest, Testing Library

---

### Task 1: Commit the existing drag-sort work

**Files:**
- Modify: none

**Step 1: Stage the prepared files**

Run: `git add frontend/package.json frontend/package-lock.json frontend/src/features/accounts/AccountsPage.tsx frontend/src/features/accounts/AccountsPage.test.tsx frontend/src/styles.css docs/plans/2026-03-10-account-reorder-and-form-cleanup-design.md docs/plans/2026-03-10-account-reorder-and-form-cleanup.md`

**Step 2: Commit them**

Run: `git commit -m "feat: improve account drag sorting ux"`

### Task 2: Add failing tests for persisted language settings

**Files:**
- Modify: `frontend/src/features/settings/SettingsPage.test.tsx`
- Modify: `frontend/src/App.test.tsx`
- Modify: `backend/internal/settings/repository_test.go`
- Modify: `backend/internal/api/settings_handler_test.go`

**Step 1: Add a frontend settings test for auto-saving language changes**

Assert that changing the language selector sends `PUT /settings/app` immediately and calls `onSettingsChanged` with the persisted value.

**Step 2: Add a frontend app test for English copy**

Assert that bootstrapping `language: "en-US"` renders English top-level labels.

**Step 3: Add backend repository and handler expectations**

Assert that `language` defaults to `zh-CN` and round-trips through storage and the HTTP settings endpoint.

**Step 4: Run the focused tests and confirm they fail for the expected missing-field reason**

Run:
- `npm --prefix frontend run test -- SettingsPage.test.tsx App.test.tsx`
- `go test ./backend/internal/settings ./backend/internal/api`

### Task 3: Implement persisted language switching

**Files:**
- Add: `frontend/src/lib/i18n.ts`
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/features/accounts/AccountsPage.tsx`
- Modify: `frontend/src/features/settings/SettingsPage.tsx`
- Modify: `frontend/src/lib/api.ts`
- Modify: `frontend/src/lib/desktop-shell.ts`
- Modify: `backend/internal/settings/repository.go`
- Modify: `backend/internal/store/sqlite/store.go`
- Modify: `backend/internal/store/sqlite/migrations.go`
- Modify: `backend/internal/api/settings_handler.go`

**Step 1: Add `language` to the settings model and sanitize it**

**Step 2: Add additive SQLite migration support**

**Step 3: Add a small typed translator helper**

**Step 4: Wire app-level language state and translate active UI copy**

**Step 5: Add an immediate-save language selector in Settings**

**Step 6: Re-run focused tests until green**

### Task 4: Commit language switching

**Files:**
- Modify: files from Task 2 and Task 3

**Step 1: Stage the language-related files**

**Step 2: Commit with message**

Run: `git commit -m "feat: add runtime language switching"`

### Task 5: Add failing tests for theme switching

**Files:**
- Modify: `frontend/src/features/settings/SettingsPage.test.tsx`
- Modify: `frontend/src/App.test.tsx`
- Modify: `backend/internal/settings/repository_test.go`
- Modify: `backend/internal/api/settings_handler_test.go`

**Step 1: Add a settings test for auto-saving theme changes**

**Step 2: Add an app test for applying dark/system theme immediately**

**Step 3: Extend backend tests for `theme_mode` defaults and persistence**

**Step 4: Run focused tests and confirm they fail before implementation**

### Task 6: Implement persisted theme switching

**Files:**
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/features/settings/SettingsPage.tsx`
- Modify: `frontend/src/lib/api.ts`
- Modify: `backend/internal/settings/repository.go`
- Modify: `backend/internal/store/sqlite/store.go`
- Modify: `backend/internal/store/sqlite/migrations.go`
- Modify: `backend/internal/api/settings_handler.go`

**Step 1: Add `theme_mode` to the settings model and sanitize it**

**Step 2: Add additive SQLite migration support**

**Step 3: Drive Ant Design theme from saved mode plus `prefers-color-scheme`**

**Step 4: Add an immediate-save theme selector in Settings**

**Step 5: Re-run focused tests until green**

### Task 7: Commit theme switching

**Files:**
- Modify: files from Task 5 and Task 6

**Step 1: Stage the theme-related files**

**Step 2: Commit with message**

Run: `git commit -m "feat: add runtime theme switching"`

### Task 8: Verify, push, and tag

**Files:**
- Modify: none

**Step 1: Run verification**

Run:
- `npm --prefix frontend run test`
- `go test ./backend/internal/settings ./backend/internal/api ./backend/internal/store/sqlite`

**Step 2: Push `main`**

Run: `git push origin main`

**Step 3: Create and push the next release tag**

Run:
- `git tag <next-version>`
- `git push origin <next-version>`
