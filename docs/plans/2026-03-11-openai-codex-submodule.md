# OpenAI Codex Submodule Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Standardize `references/openai-codex` as a proper Git submodule without changing its path or remote.

**Architecture:** Add `.gitmodules` metadata for the existing gitlink and use `git submodule absorbgitdirs` to move the nested repository metadata under the parent repository's `.git/modules` directory. Preserve the currently checked out nested repository commit so the parent repository records a clean submodule state.

**Tech Stack:** Git submodule metadata, repository layout verification, commit history rewrite-safe push.

---

### Task 1: Add standard submodule metadata

**Files:**
- Create: `/Users/gcssloop/WorkSpace/AIGC/codex-router/.gitmodules`

**Step 1: Add the submodule entry**

- Name the submodule `references/openai-codex`
- Set path to `references/openai-codex`
- Set URL to `https://github.com/openai/codex.git`

### Task 2: Normalize the nested repository layout

**Files:**
- Modify: `/Users/gcssloop/WorkSpace/AIGC/codex-router/references/openai-codex`

**Step 1: Absorb the nested git directory**

Run: `git submodule absorbgitdirs references/openai-codex`
Expected: submodule metadata moves under `.git/modules/references/openai-codex`

**Step 2: Stage the intended submodule commit**

- Stage `.gitmodules`
- Stage `references/openai-codex` so the parent repository records the current nested repository commit

### Task 3: Verify

**Files:**
- Verify: `/Users/gcssloop/WorkSpace/AIGC/codex-router/.gitmodules`
- Verify: `/Users/gcssloop/WorkSpace/AIGC/codex-router/references/openai-codex`

**Step 1: Check standard submodule state**

Run: `git submodule status`
Expected: `references/openai-codex` appears with a clean recorded commit

**Step 2: Check working tree**

Run: `git status --short`
Expected: only `.gitmodules` and the submodule gitlink are staged for commit
