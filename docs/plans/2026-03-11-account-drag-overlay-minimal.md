# Account Drag Overlay Minimal Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Remove rotated and width-constrained drag overlay styling so account drag sorting matches the product's minimal visual language.

**Architecture:** Keep the existing dnd-kit drag overlay and placeholder behavior, but simplify the overlay CSS to inherit container width and only use shadow for elevation. Add a narrow regression test that reads the stylesheet and guards against reintroducing rotation or width caps.

**Tech Stack:** React, TypeScript, Vitest, CSS

---

### Task 1: Add regression test for drag overlay styling

**Files:**
- Create: `frontend/src/styles.drag-overlay.test.ts`
- Test: `frontend/src/styles.drag-overlay.test.ts`

**Step 1: Write the failing test**
Create a Vitest test that reads `frontend/src/styles.css` and asserts the `.account-drag-overlay` rule does not include `rotate(` and does not include `width: min(100%, 960px)`.

**Step 2: Run test to verify it fails**
Run: `npm --prefix frontend run test -- src/styles.drag-overlay.test.ts`
Expected: FAIL because the stylesheet still contains the old drag overlay transform and width constraint.

**Step 3: Write minimal implementation**
Update `.account-drag-overlay` in `frontend/src/styles.css` to remove the width rule and replace the transform rule with no transform override.

**Step 4: Run test to verify it passes**
Run: `npm --prefix frontend run test -- src/styles.drag-overlay.test.ts`
Expected: PASS.

**Step 5: Run focused UI regression tests**
Run: `npm --prefix frontend run test -- src/features/accounts/AccountsPage.test.tsx`
Expected: PASS.

**Step 6: Commit**
```bash
git add docs/plans/2026-03-11-account-drag-overlay-minimal-design.md docs/plans/2026-03-11-account-drag-overlay-minimal.md frontend/src/styles.drag-overlay.test.ts frontend/src/styles.css
git commit -m "style: simplify account drag overlay"
```
