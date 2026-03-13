# README Architecture Refresh Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add clearer architecture and request-flow documentation to both public README entry points without changing product behavior.

**Architecture:** Update the English and Chinese READMEs in parallel so they present the same system model: local client, desktop shell, Go router, upstream official or compatible providers, and local persistence. Add one static component diagram and one runtime request-flow diagram, then explain responsibilities and safety boundaries in short prose.

**Tech Stack:** Markdown, Mermaid, Git

---

### Task 1: Capture the documentation structure

**Files:**
- Modify: `README.md`
- Modify: `docs/README.zh-CN.md`

**Step 1: Identify insertion points**

Locate the current architecture sections and the sections immediately following them so the new material can replace the sparse diagram without disturbing screenshots and quick start.

**Step 2: Define the shared structure**

Use the same section order in both files:
- system architecture diagram
- request flow diagram
- component responsibilities
- data and safety boundary

**Step 3: Keep claims bounded**

Ensure every new sentence matches current project behavior already described elsewhere in the repo.

### Task 2: Update the English README

**Files:**
- Modify: `README.md`

**Step 1: Replace the current architecture block**

Insert a richer system diagram and add a request-flow Mermaid diagram that shows official account and third-party API routing through the local gateway.

**Step 2: Add short explanatory sections**

Add concise prose for component responsibilities and data/safety boundaries.

**Step 3: Verify readability**

Read the updated section end-to-end and remove duplication with existing sections such as `What It Does` and `What It Explicitly Does Not Do`.

### Task 3: Update the Chinese README

**Files:**
- Modify: `docs/README.zh-CN.md`

**Step 1: Mirror the architecture structure**

Insert the same logical sections as the English README, rewritten in concise Chinese for the target audience.

**Step 2: Emphasize practical boundaries**

Make the Chinese copy explicitly answer where data is stored, what stays local, and how official accounts vs third-party APIs move through the system.

**Step 3: Verify semantic alignment**

Confirm the Chinese document matches the English one in architecture intent and product boundary.

### Task 4: Validate and finalize

**Files:**
- Modify: `README.md`
- Modify: `docs/README.zh-CN.md`

**Step 1: Diff the files**

Run: `git diff -- README.md docs/README.zh-CN.md docs/plans/2026-03-14-readme-architecture-design.md docs/plans/2026-03-14-readme-architecture.md`
Expected: Only documentation changes related to architecture refresh.

**Step 2: Sanity-check section anchors and links**

Make sure all internal doc links still resolve and the Mermaid blocks remain fenced correctly.

**Step 3: Commit**

```bash
git add README.md docs/README.zh-CN.md docs/plans/2026-03-14-readme-architecture-design.md docs/plans/2026-03-14-readme-architecture.md
git commit -m "docs: expand README architecture overview"
```
