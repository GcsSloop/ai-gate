# Skill Discovery Index Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a full-screen skill discovery flow with index-only caching, silent background refresh, explicit no-cache refresh, repository-page viewing, one-click install, and repository management CRUD for public GitHub/GitLab repositories.

**Architecture:** Extend the tooling backend with a separate discovery index/cache layer and repo platform-aware CRUD/search APIs, then rebuild the frontend skill discovery modal around that API. Keep installed skill management on the existing managed-skills path, but annotate discovered items with install status and install via explicit backend action.

**Tech Stack:** Go HTTP handlers and tests, React 19 + Ant Design + Vitest, existing release/version scripts.

---

### Task 1: Add failing backend tests for discovery cache and refresh

**Files:**
- Modify: `backend/internal/api/tooling_handler_test.go`

**Step 1: Write the failing test**

Add tests covering:
- cached discover response
- forced refresh bypassing cache
- sorted discovered skills
- cache file excluding raw skill body content

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api -run 'TestToolingSkillsDiscover|TestToolingSkillDiscoveryCache'`
Expected: FAIL because discovery endpoints/cache do not exist.

**Step 3: Write minimal implementation**

Add backend discovery cache structs, load/save helpers, discover endpoints, and scan/index builders in `backend/internal/api/tooling_handler.go`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/api -run 'TestToolingSkillsDiscover|TestToolingSkillDiscoveryCache'`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/api/tooling_handler.go backend/internal/api/tooling_handler_test.go
git commit -m "feat(tooling): add skill discovery cache api"
```

### Task 2: Add failing backend tests for GitHub/GitLab repo CRUD and search

**Files:**
- Modify: `backend/internal/api/tooling_handler_test.go`

**Step 1: Write the failing test**

Add tests for:
- adding repo with platform
- updating repo branch/platform metadata
- deleting repo by platform/owner/name
- GitHub and GitLab search plumbing

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api -run 'TestToolingSkillRepos'`
Expected: FAIL because repo model and routes are GitHub-only and lack update flow.

**Step 3: Write minimal implementation**

Extend repo record/config/API routing and search helpers in `backend/internal/api/tooling_handler.go`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/api -run 'TestToolingSkillRepos'`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/api/tooling_handler.go backend/internal/api/tooling_handler_test.go
git commit -m "feat(tooling): add multi-platform skill repo management"
```

### Task 3: Add failing frontend API and modal tests for discovery list behavior

**Files:**
- Modify: `frontend/src/lib/api.ts`
- Modify: `frontend/src/features/tooling/ToolingPage.test.tsx`

**Step 1: Write the failing test**

Add tests for:
- full-screen discovery modal defaulting to skill list
- cached list rendering
- silent replacement after background refresh
- manual refresh bypassing cache

**Step 2: Run test to verify it fails**

Run: `npm test -- --run frontend/src/features/tooling/ToolingPage.test.tsx`
Expected: FAIL because discovery modal and APIs do not support this behavior yet.

**Step 3: Write minimal implementation**

Add discovery API types and rebuild `ToolingPage.tsx` modal state flow to consume them.

**Step 4: Run test to verify it passes**

Run: `npm test -- --run frontend/src/features/tooling/ToolingPage.test.tsx`
Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/lib/api.ts frontend/src/features/tooling/ToolingPage.tsx frontend/src/features/tooling/ToolingPage.test.tsx frontend/src/styles.css
git commit -m "feat(tooling): rebuild skill discovery modal"
```

### Task 4: Add failing frontend tests for view, install, and repo CRUD

**Files:**
- Modify: `frontend/src/features/tooling/ToolingPage.test.tsx`

**Step 1: Write the failing test**

Add tests for:
- opening repository page from a skill card
- one-click install from discovered skill card
- opening repo management secondary modal
- creating, editing, deleting repos from repo management

**Step 2: Run test to verify it fails**

Run: `npm test -- --run frontend/src/features/tooling/ToolingPage.test.tsx`
Expected: FAIL because those actions and secondary modal do not exist yet.

**Step 3: Write minimal implementation**

Add install action, repo link action, secondary repo modal, and CRUD UI.

**Step 4: Run test to verify it passes**

Run: `npm test -- --run frontend/src/features/tooling/ToolingPage.test.tsx`
Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/features/tooling/ToolingPage.tsx frontend/src/features/tooling/ToolingPage.test.tsx frontend/src/styles.css
git commit -m "feat(tooling): add skill discovery actions and repo modal"
```

### Task 5: Run full verification for backend and frontend

**Files:**
- Modify as needed based on failures

**Step 1: Run backend tests**

Run: `go test ./...`
Expected: PASS

**Step 2: Run frontend tests**

Run: `npm test -- --run`
Expected: PASS

**Step 3: Run targeted build checks**

Run: `npm run build`
Expected: PASS

**Step 4: Fix any regressions**

Patch the smallest failing areas and rerun the affected commands until green.

**Step 5: Commit**

```bash
git add backend frontend
git commit -m "test: verify skill discovery flow"
```

### Task 6: Release version bump and publish flow

**Files:**
- Modify: `frontend/package.json`
- Modify: `frontend/package-lock.json`
- Modify: `desktop/package.json`
- Modify: `desktop/package-lock.json`
- Modify: `desktop/src-tauri/tauri.conf.json`
- Modify: `desktop/src-tauri/Cargo.toml`
- Modify: `desktop/src-tauri/Cargo.lock`

**Step 1: Determine next version**

Use latest tag `v1.2.1` and choose the next normalized version.

**Step 2: Sync release metadata**

Run: `bash scripts/release/sync_release_metadata.sh --version <next-version>`
Expected: version files updated consistently

**Step 3: Re-run verification**

Run: `go test ./... && npm test -- --run && npm run build`
Expected: PASS

**Step 4: Commit and tag**

```bash
git add .
git commit -m "release: cut v<next-version>"
git tag "v<next-version>"
```

**Step 5: Publish**

Follow the repo release flow to push branch, push tag, and create/merge the release PR if required.

```bash
git push origin <branch>
git push origin "v<next-version>"
```
