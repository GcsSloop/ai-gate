# Remove Demo Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Remove the frontend demo page and all of its entry points while keeping the rest of the product unchanged.

**Architecture:** This is a frontend-only deletion. The app shell stops advertising the demo view, the dedicated demo component and its assets are removed, and tests become the guardrail proving the UI no longer exposes the feature.

**Tech Stack:** React, TypeScript, Vitest, CSS

---

### Task 1: Lock the new behavior with a failing app-shell test

**Files:**
- Modify: `frontend/src/App.test.tsx`

**Step 1: Write the failing test**

Add assertions that the top navigation no longer contains the `演示` tab and remove the now-invalid test that switches into the demo page.

**Step 2: Run test to verify it fails**

Run: `npm --prefix frontend run test -- App.test.tsx`
Expected: FAIL because the current app shell still renders the `演示` tab.

**Step 3: Write minimal implementation**

Remove the `demo` tab, `demo` view union member, and `ProductDashboardDemo` render branch from `frontend/src/App.tsx`.

**Step 4: Run test to verify it passes**

Run: `npm --prefix frontend run test -- App.test.tsx`
Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/App.tsx frontend/src/App.test.tsx
git commit -m "refactor: remove demo navigation"
```

### Task 2: Delete demo-only assets and copy

**Files:**
- Modify: `frontend/src/lib/i18n.ts`
- Modify: `frontend/src/styles.css`
- Delete: `frontend/src/features/demo/ProductDashboardDemo.tsx`
- Delete: `frontend/src/features/demo/ProductDashboardDemo.test.tsx`

**Step 1: Write the failing test**

Task 1 already establishes the required visible behavior. No additional runtime behavior should remain after the component is deleted.

**Step 2: Run test to verify it fails**

Keep using `npm --prefix frontend run test -- App.test.tsx` as the guardrail if any deletion accidentally reintroduces the tab.

**Step 3: Write minimal implementation**

Remove the `演示` translation entry, delete the `.demo-*` CSS block, and delete the demo feature files.

**Step 4: Run test to verify it passes**

Run: `npm --prefix frontend run test -- App.test.tsx`
Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/lib/i18n.ts frontend/src/styles.css frontend/src/features/demo
git commit -m "refactor: remove demo assets"
```

### Task 3: Remove demo-only planning artifacts and verify the frontend

**Files:**
- Delete: `docs/plans/2026-03-16-product-dashboard-demo-design.md`
- Delete: `docs/plans/2026-03-16-product-dashboard-demo.md`
- Delete: `docs/plans/2026-03-16-product-dashboard-topology-design.md`
- Delete: `docs/plans/2026-03-16-product-dashboard-topology.md`

**Step 1: Clean up docs**

Delete the four demo-only plan files so the repository no longer advertises a removed feature.

**Step 2: Run focused verification**

Run: `npm --prefix frontend run test -- App.test.tsx`
Expected: PASS

**Step 3: Run full frontend verification**

Run: `npm --prefix frontend run test`
Expected: PASS

Run: `npm --prefix frontend run build`
Expected: PASS

**Step 4: Commit**

```bash
git add docs/plans frontend/src/App.tsx frontend/src/App.test.tsx frontend/src/lib/i18n.ts frontend/src/styles.css
git commit -m "refactor: remove product demo"
```
