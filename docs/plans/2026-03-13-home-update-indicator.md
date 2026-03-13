# Home Update Indicator Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a persisted home-page update indicator that periodically checks GitHub releases and shows a themed icon beside the settings button when a new version is available.

**Architecture:** Extend the existing app settings model with `show_home_update_indicator`, persist it through the current backend settings pipeline, and let `App.tsx` own a lightweight periodic update-check state using the existing desktop update service. Render a compact header indicator that only appears when an update is available and the setting is enabled.

**Tech Stack:** Go, SQLite, React, TypeScript, Ant Design, Tauri updater plugin.

---

### Task 1: Persist the new settings field end to end

**Files:**
- Modify: `backend/internal/store/sqlite/migrations.go`
- Modify: `backend/internal/store/sqlite/store.go`
- Modify: `backend/internal/settings/repository.go`
- Modify: `backend/internal/api/settings_handler_test.go`
- Modify: `frontend/src/lib/api.ts`

**Step 1: Write the failing backend tests**

Add assertions that default app settings and saved app settings include `show_home_update_indicator` with the expected value.

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/api ./internal/settings`
Expected: FAIL because the field is missing from the schema or payload.

**Step 3: Write minimal backend implementation**

- Add SQLite column defaulting to `1`
- Thread the field through `AppSettings`, select, sanitize, and upsert logic
- Extend frontend `AppSettings` type

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/api ./internal/settings`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/store/sqlite/migrations.go backend/internal/store/sqlite/store.go backend/internal/settings/repository.go backend/internal/api/settings_handler_test.go frontend/src/lib/api.ts
git commit -m "feat: persist home update indicator setting"
```

### Task 2: Add the failing frontend tests for the home indicator

**Files:**
- Modify: `frontend/src/App.test.tsx`
- Modify: `frontend/src/features/settings/SettingsPage.test.tsx`

**Step 1: Write the failing tests**

Add tests covering:
- Home header shows the update icon when update check finds a newer version and the setting is enabled
- Home header does not show the icon when the setting is disabled
- Clicking the update icon opens settings
- Settings page renders and toggles `show_home_update_indicator`

**Step 2: Run test to verify it fails**

Run: `cd frontend && npm test -- --run src/App.test.tsx src/features/settings/SettingsPage.test.tsx`
Expected: FAIL because the UI and setting do not exist yet.

**Step 3: Write minimal implementation**

Do not overbuild shared state. Only add enough code to satisfy the new tests.

**Step 4: Run test to verify it passes**

Run: `cd frontend && npm test -- --run src/App.test.tsx src/features/settings/SettingsPage.test.tsx`
Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/App.test.tsx frontend/src/features/settings/SettingsPage.test.tsx
git commit -m "test: cover home update indicator"
```

### Task 3: Implement the home update polling state in the app shell

**Files:**
- Modify: `frontend/src/App.tsx`
- Create or Modify: `frontend/src/features/updates/HomeUpdateIndicator.tsx`
- Modify: `frontend/src/features/updates/updateService.ts`

**Step 1: Write the failing test for polling-friendly behavior if needed**

If `App.test.tsx` needs explicit polling control, add a focused test around interval scheduling with fake timers.

**Step 2: Run test to verify it fails**

Run: `cd frontend && npm test -- --run src/App.test.tsx`
Expected: FAIL for missing polling or indicator state.

**Step 3: Write minimal implementation**

- Create a small header indicator component
- In `App.tsx`, start an immediate silent check plus a 6-hour interval when `show_home_update_indicator` is enabled
- Suppress toast noise for automatic checks
- Navigate to settings when the indicator is clicked

**Step 4: Run test to verify it passes**

Run: `cd frontend && npm test -- --run src/App.test.tsx`
Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/App.tsx frontend/src/features/updates/HomeUpdateIndicator.tsx frontend/src/features/updates/updateService.ts
git commit -m "feat: add home update indicator"
```

### Task 4: Wire the settings toggle and polish styling

**Files:**
- Modify: `frontend/src/features/settings/SettingsPage.tsx`
- Modify: `frontend/src/styles.css`
- Modify: `frontend/src/lib/i18n.ts`

**Step 1: Write the failing test if missing**

Add a focused settings-page assertion for the new switch label/behavior if Task 2 did not already cover it.

**Step 2: Run test to verify it fails**

Run: `cd frontend && npm test -- --run src/features/settings/SettingsPage.test.tsx`
Expected: FAIL until the setting appears in the page.

**Step 3: Write minimal implementation**

- Add the new toggle to settings
- Add i18n strings
- Add themed, minimal top-bar styles for the header update icon beside the settings button

**Step 4: Run test to verify it passes**

Run: `cd frontend && npm test -- --run src/features/settings/SettingsPage.test.tsx`
Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/features/settings/SettingsPage.tsx frontend/src/styles.css frontend/src/lib/i18n.ts
git commit -m "feat: add home update indicator toggle"
```

### Task 5: Final verification

**Files:**
- Modify: `docs/plans/2026-03-13-home-update-indicator-design.md`
- Modify: `docs/plans/2026-03-13-home-update-indicator.md`

**Step 1: Run backend verification**

Run: `cd backend && go test ./internal/api ./internal/settings`
Expected: PASS

**Step 2: Run frontend verification**

Run: `cd frontend && npm test -- --run src/App.test.tsx src/features/settings/SettingsPage.test.tsx src/features/updates/UpdateCard.test.tsx src/features/updates/updateService.test.ts`
Expected: PASS

**Step 3: Run formatting / hygiene checks**

Run: `git diff --check`
Expected: no output

**Step 4: Commit docs and remaining code**

```bash
git add docs/plans/2026-03-13-home-update-indicator-design.md docs/plans/2026-03-13-home-update-indicator.md
git commit -m "docs: record home update indicator plan"
```
