# About GitHub Link Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add the GitHub repository link to the About page and carry the change through the repository release flow.

**Architecture:** Keep the About page structure intact and add one new metadata row for GitHub. Use TDD by adding a focused SettingsPage test first, then implement the minimal JSX and style needed, verify locally, commit, and then run the existing release-rebase-pr-tag-loop workflow.

**Tech Stack:** React, Vitest, Ant Design, GitHub Actions, existing release workflow

---

### Task 1: Add the failing About-page test

**Files:**
- Modify: `frontend/src/features/settings/SettingsPage.test.tsx`
- Test: `frontend/src/features/settings/SettingsPage.test.tsx`

**Step 1: Write the failing test**

Add a test that opens the `关于` tab and asserts:
- text `GitHub` exists
- a link with name `GcsSloop/ai-gate` exists
- `href` equals `https://github.com/GcsSloop/ai-gate`

**Step 2: Run test to verify it fails**

Run: `cd frontend && npm test -- --run src/features/settings/SettingsPage.test.tsx`
Expected: FAIL because the About page does not render the GitHub link yet.

**Step 3: Write minimal implementation**

Add one `about-meta-row` in `frontend/src/features/settings/SettingsPage.tsx` containing the GitHub label and anchor.

**Step 4: Run test to verify it passes**

Run: `cd frontend && npm test -- --run src/features/settings/SettingsPage.test.tsx`
Expected: PASS.

### Task 2: Verify touched frontend scope

**Files:**
- Modify: `frontend/src/features/settings/SettingsPage.tsx`
- Modify: `frontend/src/features/settings/SettingsPage.test.tsx`
- Create: `docs/plans/2026-03-14-about-github-link-design.md`
- Create: `docs/plans/2026-03-14-about-github-link.md`

**Step 1: Run broader frontend verification**

Run: `cd frontend && npm test -- --run src/App.test.tsx src/features/settings/SettingsPage.test.tsx src/features/updates/UpdateCard.test.tsx src/features/updates/updateService.test.ts`
Expected: PASS.

**Step 2: Run diff hygiene check**

Run: `git diff --check`
Expected: no output.

**Step 3: Commit**

```bash
git add frontend/src/features/settings/SettingsPage.tsx frontend/src/features/settings/SettingsPage.test.tsx docs/plans/2026-03-14-about-github-link-design.md docs/plans/2026-03-14-about-github-link.md
git commit -m "feat: add github link to about page"
```

### Task 3: Run the software release flow

**Files:**
- No code changes required if CI passes

**Step 1: Rebase branch onto remote main**

Run the repository release loop skill so the branch is rebased onto `origin/main` before integration.

**Step 2: Push and monitor CI**

Ensure branch CI succeeds before PR merge.

**Step 3: Create PR and merge with rebase**

Use the repository's required rebase merge path.

**Step 4: Tag and monitor release workflow**

Create the next version tag and monitor the release workflow until terminal success or the first concrete failure.
