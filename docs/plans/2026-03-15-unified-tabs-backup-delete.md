# Unified Tabs And Backup Delete Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Unify the shell and settings tab design, fix header/content scrolling behavior, and add backup deletion support.

**Architecture:** Keep the existing top-level shell in `App.tsx`, restyle the settings sub-tabs to reuse the same pill-tab language, and add one backend settings endpoint to delete database backups. Ship the work in small TDD batches so backup deletion and layout changes can be verified independently.

**Tech Stack:** React 19, Ant Design 6, Vitest, Go HTTP handlers, SQLite-backed settings/backup storage.

---

### Task 1: Document The Approved Design

**Files:**
- Create: `docs/plans/2026-03-15-unified-tabs-backup-delete-design.md`
- Create: `docs/plans/2026-03-15-unified-tabs-backup-delete.md`

**Step 1: Write the design and task plan**

Capture the approved layout, backup-delete scope, and validation commands.

**Step 2: Commit**

Run:
```bash
git add docs/plans/2026-03-15-unified-tabs-backup-delete-design.md docs/plans/2026-03-15-unified-tabs-backup-delete.md
git commit -m "docs: plan unified tabs and backup delete"
```

### Task 2: Add Backup Delete Backend Capability

**Files:**
- Modify: `backend/internal/api/settings_handler.go`
- Modify: `backend/internal/api/settings_handler_test.go`
- Check: `backend/internal/settings/db_backup.go`

**Step 1: Write the failing tests**

Add handler coverage for deleting an existing backup and for deleting a missing backup.

**Step 2: Run test to verify it fails**

Run:
```bash
cd backend && go test ./internal/api -run 'TestSettingsHandler(DeleteDatabaseBackup|DeleteDatabaseBackupNotFound)' -count=1
```

**Step 3: Write minimal implementation**

Add a delete route and handler method that removes the target backup file, returns the correct status, and keeps error messages specific.

**Step 4: Run test to verify it passes**

Run the same command and confirm both tests pass.

**Step 5: Commit**

Run:
```bash
git add backend/internal/api/settings_handler.go backend/internal/api/settings_handler_test.go
git commit -m "feat: add database backup deletion"
```

### Task 3: Wire Frontend Backup Delete Actions

**Files:**
- Modify: `frontend/src/lib/api.ts`
- Modify: `frontend/src/features/settings/SettingsPage.tsx`
- Modify: `frontend/src/features/settings/SettingsPage.test.tsx`

**Step 1: Write the failing tests**

Add a settings-page test that exercises delete backup confirmation and refresh behavior.

**Step 2: Run test to verify it fails**

Run:
```bash
cd frontend && npm test -- --run src/features/settings/SettingsPage.test.tsx
```

**Step 3: Write minimal implementation**

Expose a delete-backup API helper, add the delete button, confirmation flow, busy state, and refresh after delete.

**Step 4: Run test to verify it passes**

Run the same command and confirm it passes.

**Step 5: Commit**

Run:
```bash
git add frontend/src/lib/api.ts frontend/src/features/settings/SettingsPage.tsx frontend/src/features/settings/SettingsPage.test.tsx
git commit -m "feat: add backup deletion controls"
```

### Task 4: Unify Header And Tab Layout

**Files:**
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/App.test.tsx`
- Modify: `frontend/src/features/settings/SettingsPage.tsx`
- Modify: `frontend/src/features/settings/SettingsPage.test.tsx`
- Modify: `frontend/src/styles.css`

**Step 1: Write the failing tests**

Add coverage for the global text tabs, settings tab toolbar/save-button row, and fixed-header content structure.

**Step 2: Run test to verify it fails**

Run:
```bash
cd frontend && npm test -- --run src/App.test.tsx src/features/settings/SettingsPage.test.tsx
```

**Step 3: Write minimal implementation**

- Use text tabs for `账户 / 统计 / 设置` in the top header.
- Keep the title in the fixed header and move view content into one scroll container.
- Move `界面偏好` above `窗口行为`.
- Restyle settings tabs to match the home tabs.
- Put the settings save button on the same row as the settings tabs.
- Move `立即备份` into the backup card header actions.

**Step 4: Run test to verify it passes**

Run the same command and confirm it passes.

**Step 5: Commit**

Run:
```bash
git add frontend/src/App.tsx frontend/src/App.test.tsx frontend/src/features/settings/SettingsPage.tsx frontend/src/features/settings/SettingsPage.test.tsx frontend/src/styles.css
git commit -m "feat: unify shell and settings tabs"
```

### Task 5: Full Verification And Release

**Files:**
- Check: repository working tree

**Step 1: Run backend regression**

Run:
```bash
cd backend && go test ./... -count=1
```

**Step 2: Run frontend regression**

Run:
```bash
cd frontend && npm test -- --run && npm run build
```

**Step 3: Run diff sanity check**

Run:
```bash
git diff --check
```

**Step 4: Execute the repository release loop**

- Rebase the branch onto `origin/main`
- Push and monitor branch CI
- Create and merge the PR
- Tag the next version
- Push the tag and monitor release CI

**Step 5: Report final release result**

Include merged PR, tag, and release URLs.
