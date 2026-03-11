# Updater Signing Key CI Decode Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix release builds by decoding the updater signing key secret before Tauri consumes it.

**Architecture:** Add a shell test that checks for a decode step in the release workflow, then update the workflow to decode the base64 secret into `GITHUB_ENV` and reuse that decoded value in the Tauri build step. Keep all other release behavior unchanged.

**Tech Stack:** GitHub Actions YAML, bash.

---

### Task 1: Add a failing workflow regression test

**Files:**
- Create: `scripts/test/release_updater_signing_key_test.sh`
- Modify: `.github/workflows/release.yml`

**Step 1: Write the failing test**
Assert that the release workflow contains a dedicated updater key decode step and that the build step reads `TAURI_SIGNING_PRIVATE_KEY` from the decoded workflow env instead of directly from the secret.

**Step 2: Run test to verify it fails**
Run: `bash scripts/test/release_updater_signing_key_test.sh`
Expected: FAIL because the workflow currently passes the secret directly.

**Step 3: Implement the minimal workflow change**
Add a decode step before `tauri build`, export the decoded key via `GITHUB_ENV`, and keep the password handling unchanged.

**Step 4: Run test to verify it passes**
Run: `bash scripts/test/release_updater_signing_key_test.sh`
Expected: PASS.

### Task 2: Fold the new test into existing CI checks

**Files:**
- Modify: `.github/workflows/dev-ci.yml`
- Modify: `.github/workflows/release.yml`

**Step 1: Add the new test to release script test lists**
Update both workflows so future changes validate the decode logic.

**Step 2: Run local verification**
Run: `bash scripts/test/release_updater_signing_key_test.sh && ruby -e 'require "yaml"; YAML.load_file(".github/workflows/release.yml")' && git diff --check`
Expected: PASS.
