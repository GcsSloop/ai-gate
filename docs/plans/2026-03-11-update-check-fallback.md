# Update Check Fallback And Feedback Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Show a clearer check animation and display the latest GitHub release version even when automatic desktop updating is unavailable.

**Architecture:** Extend the frontend update service with a read-only GitHub `latest.json` fallback and adjust the update card to render fallback metadata while keeping install actions desktop-only. Add a small status animation in the checking state so manual checks have visible feedback.

**Tech Stack:** React, TypeScript, Vitest, Ant Design, CSS.

---

### Task 1: Cover fallback update lookup in service tests

**Files:**
- Modify: `frontend/src/features/updates/updateService.test.ts`
- Modify: `frontend/src/features/updates/updateService.ts`

**Step 1: Write the failing test**
Add a test proving `check("2.3.4")` returns `supported: false` and a latest-version payload when the desktop adapter is unsupported but `latest.json` fetch succeeds.

**Step 2: Run test to verify it fails**
Run: `npm --prefix frontend run test -- updateService`
Expected: FAIL because the service does not fetch fallback metadata.

**Step 3: Write minimal implementation**
Add a small fetch-based manifest reader and thread `currentVersion` into the result.

**Step 4: Run test to verify it passes**
Run: `npm --prefix frontend run test -- updateService`
Expected: PASS.

### Task 2: Cover unsupported latest-version UI and check animation

**Files:**
- Modify: `frontend/src/features/updates/UpdateCard.test.tsx`
- Modify: `frontend/src/features/updates/UpdateCard.tsx`
- Modify: `frontend/src/styles.css`
- Modify: `frontend/src/lib/i18n.ts`

**Step 1: Write the failing test**
Add tests proving the card shows a checking-state animation hook and still renders the fetched latest version when the environment is unsupported.

**Step 2: Run test to verify it fails**
Run: `npm --prefix frontend run test -- UpdateCard`
Expected: FAIL because the current card hides latest version in unsupported mode and exposes no dedicated checking animation state.

**Step 3: Write minimal implementation**
Render latest version whenever `state.update` exists, add unsupported explanatory copy, and add a CSS-driven checking animation class.

**Step 4: Run test to verify it passes**
Run: `npm --prefix frontend run test -- UpdateCard`
Expected: PASS.

### Task 3: Run focused verification

**Files:**
- Modify: `frontend/src/features/settings/SettingsPage.test.tsx` only if snapshots/text expectations need updates.

**Step 1: Run related frontend tests**
Run: `npm --prefix frontend run test -- updateService && npm --prefix frontend run test -- UpdateCard && npm --prefix frontend run test -- SettingsPage`
Expected: PASS.

**Step 2: Run formatting/sanity checks**
Run: `git diff --check`
Expected: PASS.
