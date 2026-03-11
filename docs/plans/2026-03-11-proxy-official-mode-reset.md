# Proxy Official Mode Reset Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Remove the top-level `model_provider` key when proxy disable returns Codex to official default mode, while preserving strict third-party provider restoration.

**Architecture:** Keep the existing proxy session model. Only change the final config rewrite step during proxy disable so official-mode sessions delete `model_provider`, while third-party sessions still write the previous provider name back.

**Tech Stack:** Go HTTP handler tests, Go config string rewriting helpers, Markdown docs.

---

### Task 1: Update tests first

**Files:**
- Modify: `/Users/gcssloop/WorkSpace/AIGC/codex-router/backend/internal/api/settings_handler_test.go`

**Step 1: Write the failing test**

- Change official-mode proxy-disable assertions from `assertFileContains(..., 'model_provider = "openai"')` to `assertFileNotContains(..., 'model_provider = "openai"')`.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api`
Expected: FAIL in official-mode proxy disable tests because the code still writes `model_provider = "openai"`.

### Task 2: Implement minimal config rewrite

**Files:**
- Modify: `/Users/gcssloop/WorkSpace/AIGC/codex-router/backend/internal/api/settings_handler.go`

**Step 1: Add helper to remove top-level `model_provider`**

- Remove only the root-level assignment.
- Do not touch `[model_providers.openai]` or any nested config.

**Step 2: Update disable logic**

- In `detachProxyConfig`, if the previous provider is empty or `openai`, delete the top-level `model_provider` key instead of writing `openai`.
- Keep the existing third-party restore behavior unchanged.

### Task 3: Update docs

**Files:**
- Modify: `/Users/gcssloop/WorkSpace/AIGC/codex-router/README.md`
- Modify: `/Users/gcssloop/WorkSpace/AIGC/codex-router/docs/README.zh-CN.md`

**Step 1: Update proxy behavior wording**

- State that official-mode disable removes the top-level `model_provider` and deletes temporary `aigate` config.
- State that third-party mode still restores the original provider name and `base_url`.

### Task 4: Verify

**Files:**
- Test: `/Users/gcssloop/WorkSpace/AIGC/codex-router/backend/internal/api/settings_handler_test.go`

**Step 1: Run focused tests**

Run: `go test ./internal/api`
Expected: PASS
