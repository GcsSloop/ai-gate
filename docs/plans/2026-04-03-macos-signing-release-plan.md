# macOS Signing Release Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make the release workflow import the Apple signing certificate into the macOS runner keychain so GitHub Actions can codesign and notarize release builds, then run the repository release loop.

**Architecture:** Add a regression test that asserts the workflow contains a dedicated Apple certificate import step and that the notarization step reads the imported keychain context. Then implement the minimal workflow changes in `.github/workflows/release.yml` using temporary files and a temporary keychain on macOS runners. Keep existing notarization script behavior intact.

**Tech Stack:** GitHub Actions YAML, bash, macOS `security` tooling, GitHub CLI.

---

### Task 1: Isolate non-code cleanup

**Files:**
- Delete: `skills/release-rebase-pr-tag-loop/SKILL.md`

**Step 1: Commit the project-local skill removal**

Run:
```bash
git add skills/release-rebase-pr-tag-loop/SKILL.md
git commit -m "chore: remove project-local release skill copy"
```

Expected: a single isolated cleanup commit.

### Task 2: Add a failing regression test for Apple certificate import

**Files:**
- Create: `scripts/test/release_apple_signing_test.sh`
- Modify: `.github/workflows/release.yml`

**Step 1: Write the failing test**
Assert that the release workflow has a step named for importing the Apple signing certificate, that it reads `APPLE_CERTIFICATE_P12` and `APPLE_CERTIFICATE_PASSWORD`, and that the macOS notarization path runs after this setup.

**Step 2: Run the test to verify it fails**

Run:
```bash
bash scripts/test/release_apple_signing_test.sh
```

Expected: FAIL because the workflow does not yet import the Apple certificate.

### Task 3: Implement the minimal workflow change

**Files:**
- Modify: `.github/workflows/release.yml`

**Step 1: Add a macOS-only import step**
Create a temporary keychain, decode the base64 `.p12`, import it with the password, unlock the keychain, set key partition access, and register it for the job.

**Step 2: Keep existing notarization flow unchanged except for consuming the new environment**
Do not change release packaging or non-macOS behavior.

**Step 3: Run the new regression test**

Run:
```bash
bash scripts/test/release_apple_signing_test.sh
```

Expected: PASS.

### Task 4: Verify workflow syntax and local release checks

**Files:**
- Modify: `.github/workflows/release.yml`

**Step 1: Validate YAML and script references**

Run:
```bash
ruby -e 'require "yaml"; YAML.load_file(".github/workflows/release.yml")'
```

Expected: PASS.

**Step 2: Run relevant release script checks**

Run:
```bash
bash scripts/test/release_updater_signing_key_test.sh && bash scripts/test/release_apple_signing_test.sh
```

Expected: PASS.

### Task 5: Publish through the repository release loop

**Files:**
- Modify: none beyond verified workflow files

**Step 1: Commit the CI fix**
Create a separate commit for the workflow signing change.

**Step 2: Push the branch and monitor branch CI**
If branch CI fails for code or workflow reasons, fix the minimal cause and repeat from rebase.

**Step 3: Create and merge the PR, then create and push the release tag**
If release fails because a GitHub Secret is missing or malformed, stop and report the exact secret name and why it is invalid.
