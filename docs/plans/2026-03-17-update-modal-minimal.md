# Update Modal Minimal Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Simplify the home update modal so it matches the surrounding modal style while leaving the about-page update card unchanged.

**Architecture:** Introduce a dedicated modal-only update component that reuses the existing update service contract and state rendering rules but presents them with lighter layout chrome. The modal in `App.tsx` will switch to that new component, while `SettingsPage.tsx` keeps the original `UpdateCard`.

**Tech Stack:** React, TypeScript, Ant Design, Vitest, existing app styles.

---

### Task 1: Lock the modal/about split with failing tests

**Files:**
- Modify: `frontend/src/App.test.tsx`
- Verify: `frontend/src/features/settings/SettingsPage.test.tsx`

**Step 1: Write the failing test**
- Extend the update-modal test so it asserts the dialog does not render the inner `应用更新` heading, the GitHub Release description, or the `检查更新` button.
- Assert the modal still shows retained content such as current version, target version, publish time, or release notes.

**Step 2: Run test to verify it fails**
Run: `npm --prefix frontend test -- --runInBand src/App.test.tsx src/features/settings/SettingsPage.test.tsx`
Expected: FAIL because the modal still uses the shared `UpdateCard`.

**Step 3: Write minimal implementation**
- Add a dedicated modal update component under `frontend/src/features/updates/`.
- Point `frontend/src/App.tsx` at the new component.
- Add any minimal CSS required for padding and border removal while keeping the about-page card untouched.

**Step 4: Run tests to verify they pass**
Run: `npm --prefix frontend test -- --runInBand src/App.test.tsx src/features/settings/SettingsPage.test.tsx`
Expected: PASS.

### Task 2: Verify the update feature slice stays green

**Files:**
- Verify: `frontend/src/features/updates/UpdateCard.test.tsx`

**Step 1: Run focused update tests**
Run: `npm --prefix frontend test -- --runInBand src/features/updates/UpdateCard.test.tsx`
Expected: PASS.

**Step 2: Run the full frontend suite**
Run: `npm --prefix frontend test`
Expected: PASS.
