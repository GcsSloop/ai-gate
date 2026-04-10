# Tooling Minimal Management Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Rebuild the skills and MCP management pages into a minimal Codex-only management experience with per-item enable, disable, and delete actions.

**Architecture:** Keep the existing `/tooling` route and `ToolingPage` entry point, but replace the current table-heavy rendering with compact, card-based skills and MCP views. Extend the tooling backend with per-skill sync and delete endpoints so the UI can immediately toggle Codex enablement and remove managed assets from both AI Gate and Codex.

**Tech Stack:** React, Ant Design, Vitest, Go HTTP handlers, filesystem-based tooling state.

---

### Task 1: Lock backend skill behavior with failing tests

**Files:**
- Modify: `backend/internal/api/tooling_handler_test.go`
- Modify: `backend/internal/api/tooling_handler.go`

**Step 1: Write the failing tests**

Add backend tests covering:
- enable a single managed skill collection into Codex
- disable a single managed skill collection from Codex while keeping AI Gate managed copy
- delete a managed skill collection from both AI Gate and Codex

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/api -run TestToolingHandler -count=1`

Expected: FAIL because the new skill endpoints and deletion behavior do not exist yet.

**Step 3: Write minimal implementation**

Add skill-specific apply/delete endpoints and supporting helpers in the tooling handler.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/api -run TestToolingHandler -count=1`

Expected: PASS

### Task 2: Add frontend API support for per-item skill operations

**Files:**
- Modify: `frontend/src/lib/api.ts`

**Step 1: Write the failing test**

Cover the new UI flows in `frontend/src/features/tooling/ToolingPage.test.tsx` so they require single-item skill enable/disable/delete API calls.

**Step 2: Run test to verify it fails**

Run: `cd frontend && npm test -- --run src/features/tooling/ToolingPage.test.tsx`

Expected: FAIL because the new API helpers and UI actions are missing.

**Step 3: Write minimal implementation**

Expose frontend API helpers for:
- per-skill apply with target apps
- per-skill delete
- MCP delete/apply reuse

**Step 4: Run test to verify it passes**

Run: `cd frontend && npm test -- --run src/features/tooling/ToolingPage.test.tsx`

Expected: PASS

### Task 3: Rebuild the skills page to the new minimal card layout

**Files:**
- Modify: `frontend/src/features/tooling/ToolingPage.tsx`
- Modify: `frontend/src/features/tooling/ToolingPage.test.tsx`

**Step 1: Write the failing tests**

Cover:
- top action bar with `导入已有` and `发现技能`
- Codex-only source stats
- installed skill cards with name, description, and Codex enable toggle
- hover delete affordance with confirmation
- no search, filter, or table layout

**Step 2: Run test to verify it fails**

Run: `cd frontend && npm test -- --run src/features/tooling/ToolingPage.test.tsx`

Expected: FAIL because the old table UI is still rendered.

**Step 3: Write minimal implementation**

Replace the old skills view with:
- compact action header
- modal-based discovery section
- card-based installed skill list
- immediate toggle/delete behavior

**Step 4: Run test to verify it passes**

Run: `cd frontend && npm test -- --run src/features/tooling/ToolingPage.test.tsx`

Expected: PASS

### Task 4: Rebuild the MCP page with the same minimal structure

**Files:**
- Modify: `frontend/src/features/tooling/ToolingPage.tsx`
- Modify: `frontend/src/features/tooling/ToolingPage.test.tsx`

**Step 1: Write the failing tests**

Cover:
- top action bar with `导入已有` and `发现服务`
- compact source stats
- installed MCP cards with Codex toggle/delete actions
- modal-based discovery/template entry

**Step 2: Run test to verify it fails**

Run: `cd frontend && npm test -- --run src/features/tooling/ToolingPage.test.tsx`

Expected: FAIL because the old split-column MCP layout is still rendered.

**Step 3: Write minimal implementation**

Apply the same card language and action model as skills while preserving existing MCP template install behavior in a discovery modal.

**Step 4: Run test to verify it passes**

Run: `cd frontend && npm test -- --run src/features/tooling/ToolingPage.test.tsx`

Expected: PASS

### Task 5: Final verification

**Files:**
- Verify only

**Step 1: Run backend verification**

Run: `cd backend && go test ./internal/api -run TestToolingHandler -count=1`

Expected: PASS

**Step 2: Run frontend verification**

Run: `cd frontend && npm test -- --run src/features/tooling/ToolingPage.test.tsx`

Expected: PASS

**Step 3: Inspect diff**

Run: `git diff -- backend/internal/api/tooling_handler.go backend/internal/api/tooling_handler_test.go frontend/src/features/tooling/ToolingPage.tsx frontend/src/features/tooling/ToolingPage.test.tsx frontend/src/lib/api.ts docs/plans/2026-04-08-tooling-minimal-management-plan.md`

Expected: only the planned tooling changes appear.
