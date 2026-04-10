# README Showcase Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Rebuild the Chinese and English README pages into a stronger open-source project showcase with live GitHub badges, richer screenshots, and clear Skill/MCP positioning.

**Architecture:** The implementation is documentation-only. `README.md` becomes the Chinese flagship entry, `docs/README.en.md` mirrors the same structure in English, and `docs/README.zh-CN.md` remains a compatibility landing page that points readers to the richer entry points.

**Tech Stack:** Markdown, Mermaid, Shields.io badges

---

### Task 1: Rebuild the Chinese flagship README

**Files:**
- Modify: `README.md`

**Step 1: Rewrite the hero and value framing**

Add a concise product-style intro, live badges, and a stronger summary of what AI Gate solves.

**Step 2: Add highlights and product tour**

Insert grouped highlights for multi-account routing, failover, local-first security, Skill/MCP management, and desktop operations. Expand the screenshot section to cover MCP and Skill pages.

**Step 3: Restructure practical guidance**

Keep quick start, architecture, CLI integration, and packaging, but move them after the showcase sections.

### Task 2: Mirror the structure in the English README

**Files:**
- Modify: `docs/README.en.md`

**Step 1: Match the same information architecture**

Rewrite the English README to mirror the Chinese structure: hero, badges, highlights, product tour, quick start, architecture, Skill/MCP workflow, and developer sections.

**Step 2: Keep claims precise**

Translate the same product claims without adding new promises or machine-style phrasing.

### Task 3: Update the Chinese compatibility landing page

**Files:**
- Modify: `docs/README.zh-CN.md`

**Step 1: Refresh the pointer page**

Update the landing page so it clearly points to the redesigned Chinese and English README entries and mentions the expanded product tour, badges, and Skill/MCP sections.

### Task 4: Verify documentation output

**Files:**
- Review: `README.md`
- Review: `docs/README.en.md`
- Review: `docs/README.zh-CN.md`

**Step 1: Review the diff**

Run a focused `git diff` on the three README files plus the new plan docs.

**Step 2: Check screenshot and badge references**

Confirm all screenshot filenames exist and badge URLs point at `GcsSloop/ai-gate` and the `release.yml` workflow.
