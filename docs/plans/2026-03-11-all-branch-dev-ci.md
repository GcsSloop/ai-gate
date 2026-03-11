# All-Branch Dev CI Trigger Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Run the existing dev CI test/build jobs on push for every branch.

**Architecture:** Keep the current `dev-ci` workflow jobs unchanged and only widen the `push` branch filter. Leave pull request and release workflows untouched so CI scope changes stay isolated.

**Tech Stack:** GitHub Actions YAML.

---

### Task 1: Widen the dev CI push trigger

**Files:**
- Modify: `.github/workflows/dev-ci.yml`

**Step 1: Update the push branch filter**
Change `on.push.branches` from `dev` to a wildcard that matches all branches.

**Step 2: Preserve PR and manual triggers**
Leave `pull_request.branches: [dev]` and `workflow_dispatch` unchanged.

### Task 2: Verify workflow syntax and scope

**Files:**
- Modify: `.github/workflows/dev-ci.yml`

**Step 1: Parse the workflow YAML**
Run a local YAML parse check against `.github/workflows/dev-ci.yml`.

**Step 2: Review the diff**
Confirm the only behavioral change is the widened `push` scope.
