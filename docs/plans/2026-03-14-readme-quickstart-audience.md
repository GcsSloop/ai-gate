# README Quick Start Audience Split Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Separate end-user and developer onboarding in the README, and add a stable latest-release download link for ordinary users.

**Architecture:** Keep the current README structure intact, but replace the mixed quick-start block with two audience-specific sections. End users get the latest release link plus simple account/proxy guidance. Developers keep the existing local source workflow.

**Tech Stack:** Markdown, Git

---

### Task 1: Update the Chinese README quick start

**Files:**
- Modify: `README.md`

**Step 1: Add the end-user entry**

Insert a `普通用户` subsection with the GitHub latest release link and concise usage guidance for official and third-party accounts.

**Step 2: Add the developer entry**

Rename the existing source-based setup path into a `开发者` subsection.

**Step 3: Keep the section clean**

Make sure ordinary users are not told to edit environment variables unless they are actually building from source.

### Task 2: Update the English README quick start

**Files:**
- Modify: `docs/README.en.md`

**Step 1: Add the end-user entry**

Insert a `For End Users` subsection with the stable latest release link and simple client setup steps.

**Step 2: Add the developer entry**

Rename the existing source-based setup path into a `For Developers` subsection.

**Step 3: Keep semantic parity**

Make sure the English content matches the Chinese intent without becoming verbose.

### Task 3: Validate the diff

**Files:**
- Modify: `README.md`
- Modify: `docs/README.en.md`

**Step 1: Inspect the diff**

Run: `git diff -- README.md docs/README.en.md docs/plans/2026-03-14-readme-quickstart-audience-design.md docs/plans/2026-03-14-readme-quickstart-audience.md`
Expected: README-only wording changes plus the new design/plan docs.
