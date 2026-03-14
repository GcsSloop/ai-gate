# Lightweight Usage Stats Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Remove audit-detail persistence, replace it with lightweight usage-event storage, and add a token and cost statistics page.

**Architecture:** Add a new `usage_events` repository for request facts, switch request completion paths to write those facts instead of `conversations/messages/runs`, and replace dashboard plus settings UI with stats-first interfaces. Run one startup cleanup to clear old audit rows and vacuum the database.

**Tech Stack:** Go, SQLite, net/http, React, TypeScript, Ant Design, Vitest

---

### Task 1: Replace the design baseline

**Files:**
- Create: `docs/plans/2026-03-15-lightweight-usage-stats-design.md`
- Create: `docs/plans/2026-03-15-lightweight-usage-stats.md`

**Step 1: Save the approved design**

Write the design and this implementation plan.

**Step 2: Commit**

Run:

```bash
git add docs/plans/2026-03-15-lightweight-usage-stats-design.md docs/plans/2026-03-15-lightweight-usage-stats.md
git commit -m "docs: plan lightweight usage stats"
```

### Task 2: Add `usage_events` schema and repository

**Files:**
- Modify: `backend/internal/store/sqlite/migrations.go`
- Modify: `backend/internal/store/sqlite/store_test.go`
- Modify: `backend/internal/usage/types.go`
- Modify: `backend/internal/usage/repository.go`
- Modify: `backend/internal/usage/repository_test.go`

**Step 1: Write the failing tests**

Add tests for:

- schema contains `usage_events`
- saving one usage event
- listing recent usage events
- summary aggregation by time range

**Step 2: Run the targeted tests and verify RED**

Run:

```bash
cd backend && go test ./internal/usage ./internal/store/sqlite -count=1
```

Expected: FAIL because `usage_events` and related repository APIs do not exist yet.

**Step 3: Write minimal implementation**

- Add the new table.
- Extend `usage.Repository` with event methods.
- Implement save and query methods with small, explicit SQL.

**Step 4: Run the targeted tests and verify GREEN**

Run:

```bash
cd backend && go test ./internal/usage ./internal/store/sqlite -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add backend/internal/store/sqlite/migrations.go backend/internal/store/sqlite/store_test.go backend/internal/usage/types.go backend/internal/usage/repository.go backend/internal/usage/repository_test.go
git commit -m "feat: add lightweight usage event storage"
```

### Task 3: Stop writing audit rows in request paths

**Files:**
- Modify: `backend/internal/api/gateway_handler.go`
- Modify: `backend/internal/api/responses_handler.go`
- Modify: `backend/internal/routing/executor.go`
- Modify: `backend/internal/streaming/proxy.go`
- Modify: request-path tests under `backend/internal/api`

**Step 1: Write the failing tests**

Add tests that assert:

- successful request completion writes one usage event
- failure paths write status and latency
- request handlers no longer require conversation or run persistence for success

**Step 2: Run the targeted tests and verify RED**

Run:

```bash
cd backend && go test ./internal/api ./internal/routing ./internal/streaming -count=1
```

Expected: FAIL because handlers still depend on audit repositories.

**Step 3: Write minimal implementation**

- Introduce lightweight event recording helpers.
- Remove new writes to `conversations`, `messages`, and `runs`.
- Preserve current proxy behavior and response passthrough.

**Step 4: Run the targeted tests and verify GREEN**

Run:

```bash
cd backend && go test ./internal/api ./internal/routing ./internal/streaming -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add backend/internal/api/gateway_handler.go backend/internal/api/responses_handler.go backend/internal/routing/executor.go backend/internal/streaming/proxy.go backend/internal/api
git commit -m "refactor: record usage events instead of audit rows"
```

### Task 4: Replace dashboard APIs with usage-stat APIs

**Files:**
- Modify: `backend/internal/api/dashboard_handler.go`
- Modify: `backend/internal/api/dashboard_handler_test.go`
- Modify: `backend/internal/bootstrap/bootstrap.go`

**Step 1: Write the failing tests**

Add handler tests for:

- summary cards
- trend payload
- recent event list
- account and model filtering

**Step 2: Run the targeted tests and verify RED**

Run:

```bash
cd backend && go test ./internal/api ./internal/bootstrap -count=1
```

Expected: FAIL because current dashboard endpoints are conversation-based.

**Step 3: Write minimal implementation**

- Change the dashboard handler to depend on usage aggregates.
- Expose endpoints needed by the frontend stats page.
- Rewire bootstrap to inject the usage repository instead of the conversation repository.

**Step 4: Run the targeted tests and verify GREEN**

Run:

```bash
cd backend && go test ./internal/api ./internal/bootstrap -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add backend/internal/api/dashboard_handler.go backend/internal/api/dashboard_handler_test.go backend/internal/bootstrap/bootstrap.go
git commit -m "feat: expose usage stats dashboard apis"
```

### Task 5: Remove audit settings and add cleanup migration

**Files:**
- Modify: `backend/internal/settings/repository.go`
- Modify: `backend/internal/settings/repository_test.go`
- Modify: `backend/internal/api/settings_handler.go`
- Modify: `backend/internal/api/settings_handler_test.go`
- Modify: `backend/internal/bootstrap/bootstrap.go`

**Step 1: Write the failing tests**

Add tests that assert:

- app settings no longer include audit-limit fields
- audit optimize endpoint is gone
- startup cleanup clears old audit tables only once

**Step 2: Run the targeted tests and verify RED**

Run:

```bash
cd backend && go test ./internal/settings ./internal/api ./internal/bootstrap -count=1
```

Expected: FAIL because audit settings and optimizer are still present.

**Step 3: Write minimal implementation**

- Remove audit-limit fields from settings persistence and API payloads.
- Remove silent optimize hooks and manual optimize endpoint.
- Add a cleanup marker in settings or a small internal metadata table.
- Clear old audit tables and vacuum once on startup.

**Step 4: Run the targeted tests and verify GREEN**

Run:

```bash
cd backend && go test ./internal/settings ./internal/api ./internal/bootstrap -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add backend/internal/settings/repository.go backend/internal/settings/repository_test.go backend/internal/api/settings_handler.go backend/internal/api/settings_handler_test.go backend/internal/bootstrap/bootstrap.go
git commit -m "refactor: remove audit storage settings"
```

### Task 6: Add the frontend stats page

**Files:**
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/App.test.tsx`
- Modify: `frontend/src/lib/api.ts`
- Create: `frontend/src/features/stats/StatsPage.tsx`
- Create: `frontend/src/features/stats/StatsPage.test.tsx`
- Modify: `frontend/src/styles.css`

**Step 1: Write the failing tests**

Add tests for:

- navigating to the stats page
- loading summary cards and charts
- rendering recent usage rows
- showing estimated cost and balance or quota deltas separately

**Step 2: Run the targeted tests and verify RED**

Run:

```bash
cd frontend && npm test -- --run App StatsPage
```

Expected: FAIL because the stats page and APIs do not exist yet.

**Step 3: Write minimal implementation**

- Add a top-level stats view.
- Add frontend API types for summary, trends, and recent events.
- Build a lightweight page with cards, simple charts, and filters.

**Step 4: Run the targeted tests and verify GREEN**

Run:

```bash
cd frontend && npm test -- --run App StatsPage
```

Expected: PASS.

**Step 5: Commit**

```bash
git add frontend/src/App.tsx frontend/src/App.test.tsx frontend/src/lib/api.ts frontend/src/features/stats frontend/src/styles.css
git commit -m "feat: add token and cost stats page"
```

### Task 7: Remove audit controls from settings UI

**Files:**
- Modify: `frontend/src/features/settings/SettingsPage.tsx`
- Modify: `frontend/src/features/settings/SettingsPage.test.tsx`
- Modify: `frontend/src/lib/api.ts`

**Step 1: Write the failing tests**

Add tests that assert:

- audit storage controls are no longer rendered
- settings save only sends live fields

**Step 2: Run the targeted tests and verify RED**

Run:

```bash
cd frontend && npm test -- --run SettingsPage
```

Expected: FAIL because the UI still exposes audit settings.

**Step 3: Write minimal implementation**

- Remove optimize action and audit limit inputs.
- Keep layout stable.

**Step 4: Run the targeted tests and verify GREEN**

Run:

```bash
cd frontend && npm test -- --run SettingsPage
```

Expected: PASS.

**Step 5: Commit**

```bash
git add frontend/src/features/settings/SettingsPage.tsx frontend/src/features/settings/SettingsPage.test.tsx frontend/src/lib/api.ts
git commit -m "refactor: remove audit storage controls"
```

### Task 8: Final verification

**Files:**
- Verify the whole touched surface

**Step 1: Run backend verification**

```bash
cd backend && go test ./... -count=1
```

**Step 2: Run frontend verification**

```bash
cd frontend && npm test -- --run
```

**Step 3: Check formatting and patch hygiene**

```bash
git diff --check
```

**Step 4: Commit final cleanup**

```bash
git add .
git commit -m "chore: finalize lightweight usage stats rollout"
```
