# Account Reorder And Form Cleanup Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make account sorting pointer-driven and live on all clients, shrink source icon presentation, and remove the redundant `/responses` toggle from account forms.

**Architecture:** Replace the hand-rolled pointer reorder controller with a `@dnd-kit` sortable list that uses a floating drag overlay plus a stable in-list placeholder. Persist priority only on drag end. Simplify create/edit payload construction so `/responses` support is implicit, and tighten the related UI styling in the card list and source icon selector.

**Tech Stack:** React 19, Testing Library, Ant Design, CSS, existing account API layer.

---

### Task 1: Lock overlay drag behavior with tests

**Files:**
- Modify: `frontend/src/features/accounts/AccountsPage.test.tsx`

**Step 1: Write failing tests**

Cover:
- third-party create flow no longer renders `原生 /responses`
- edit flow no longer renders `原生 /responses`
- dragging renders a floating overlay and leaves a placeholder in the list
- dragging reorders cards before pointer release and persists priority after release

**Step 2: Run targeted test**

Run: `npm --prefix frontend run test -- AccountsPage.test.tsx`

### Task 2: Implement overlay-based live reorder

**Files:**
- Modify: `frontend/src/features/accounts/AccountsPage.tsx`
- Modify: `frontend/src/styles.css`
- Modify: `frontend/package.json`

**Step 1: Replace custom drag logic**

Use `@dnd-kit/core`, `@dnd-kit/sortable`, and `@dnd-kit/utilities` for sortable account cards with a dedicated drag handle.

**Step 2: Add overlay + placeholder**

Render the dragged card through `DragOverlay`, keep the source card in-place as a placeholder, and reorder the in-memory list during drag-over.

**Step 3: Persist on release**

Persist priority once after drag end, and revert on failure.

### Task 3: Simplify source icon + form controls

**Files:**
- Modify: `frontend/src/features/accounts/AccountsPage.tsx`
- Modify: `frontend/src/styles.css`

**Step 1: Remove `/responses` switch from add/edit forms**

Always submit `supports_responses: true` for compatible accounts.

**Step 2: Shrink source icon presentation**

Adjust select option avatar size and card icon size to reduce visual weight.

### Task 4: Verify

**Files:**
- Verify: `frontend/src/features/accounts/AccountsPage.test.tsx`

**Step 1: Run account page tests**

Run: `npm --prefix frontend run test -- AccountsPage.test.tsx`

**Step 2: Inspect final diff**

Run: `git diff -- frontend/src/features/accounts/AccountsPage.tsx frontend/src/features/accounts/AccountsPage.test.tsx frontend/src/styles.css`
