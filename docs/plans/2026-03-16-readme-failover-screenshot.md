# README Failover And Screenshot Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Refresh the Chinese and English README copy so product advantages mention automatic failover and the screenshots section includes the statistics page.

**Architecture:** This is a documentation-only update. The root Chinese README and the English README gain a short product-advantages section plus a new statistics screenshot entry, while the Chinese redirect page points readers to the expanded content.

**Tech Stack:** Markdown

---

### Task 1: Update the root Chinese README

**Files:**
- Modify: `README.md`

**Step 1: Add product-advantage copy**

Insert a concise `产品优势` section that explains local-first access, low-friction multi-account switching, and automatic failover when one account runs out of quota or becomes unavailable.

**Step 2: Add the statistics screenshot**

Add `assets/screenshot-statistics.png` under the screenshot section with a short label.

### Task 2: Update the English README

**Files:**
- Modify: `docs/README.en.md`

**Step 1: Add matching advantage copy**

Insert a `Product Advantages` section with equivalent meaning in English, including automatic failover to another available account when the active one is exhausted or unhealthy.

**Step 2: Add the statistics screenshot**

Add `../assets/screenshot-statistics.png` to the screenshots section.

### Task 3: Update the Chinese redirect page

**Files:**
- Modify: `docs/README.zh-CN.md`

**Step 1: Sync the redirect text**

Mention that the root README now includes the automatic failover explanation and the statistics page screenshot, so readers know where to find the expanded content.
