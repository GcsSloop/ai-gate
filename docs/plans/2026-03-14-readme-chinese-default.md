# README Chinese-First Entry Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Switch the repository landing page to a Chinese-first README while preserving a stable English document path and reducing duplicated maintenance.

**Architecture:** Promote the existing Chinese documentation structure to the repository root, move the current English README into `docs/README.en.md`, and convert `docs/README.zh-CN.md` into a short compatibility page that redirects readers to the canonical Chinese root README. Keep all existing diagrams, screenshots, and product boundaries intact.

**Tech Stack:** Markdown, Mermaid, Git

---

### Task 1: Create the target document layout

**Files:**
- Modify: `README.md`
- Create: `docs/README.en.md`
- Modify: `docs/README.zh-CN.md`

**Step 1: Define canonical ownership**

Set root `README.md` as canonical Chinese content and `docs/README.en.md` as canonical English content.

**Step 2: Decide compatibility behavior**

Make `docs/README.zh-CN.md` a short bridge document with links to the root README, English README, and key technical docs.

**Step 3: Preserve current substance**

Carry over the architecture and safety sections from the latest README edits instead of rewriting them from scratch.

### Task 2: Promote Chinese content to the root README

**Files:**
- Modify: `README.md`

**Step 1: Replace English-first body with Chinese-first body**

Use the existing Chinese README content as the source, adjusted for root-relative asset and doc links.

**Step 2: Keep language switch explicit**

Add a clear language switch near the top pointing to `docs/README.en.md`.

**Step 3: Verify links**

Update asset and doc paths so they work from the repository root.

### Task 3: Create the English document

**Files:**
- Create: `docs/README.en.md`

**Step 1: Move the current English README content**

Copy the current English README structure into `docs/README.en.md`.

**Step 2: Fix relative links**

Adjust images, docs links, and local references for the `docs/` directory context.

**Step 3: Add a language switch back to root**

Point English readers back to `../README.md` for the Chinese main entry.

### Task 4: Convert the old Chinese docs README into a bridge page

**Files:**
- Modify: `docs/README.zh-CN.md`

**Step 1: Replace duplicated full content**

Turn the file into a short pointer page.

**Step 2: Keep it useful**

Include direct links to the root README, English README, and the most important technical docs.

**Step 3: Keep expectations clear**

State that the root README is now the canonical Chinese entry.

### Task 5: Validate and finalize

**Files:**
- Modify: `README.md`
- Create: `docs/README.en.md`
- Modify: `docs/README.zh-CN.md`

**Step 1: Inspect the diff**

Run: `git diff -- README.md docs/README.en.md docs/README.zh-CN.md docs/plans/2026-03-14-readme-chinese-default-design.md docs/plans/2026-03-14-readme-chinese-default.md`
Expected: Only documentation and path adjustments.

**Step 2: Check file status**

Run: `git status --short`
Expected: modified/new doc files only.

**Step 3: Commit**

```bash
git add README.md docs/README.en.md docs/README.zh-CN.md docs/plans/2026-03-14-readme-chinese-default-design.md docs/plans/2026-03-14-readme-chinese-default.md
git commit -m "docs: make README chinese-first"
```
