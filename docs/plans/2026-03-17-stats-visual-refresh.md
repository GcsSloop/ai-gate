# Stats Visual Refresh Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Modernize the statistics page with split input/output token summaries, an ECharts dual-line trend chart, and a full-window model-distribution donut chart.

**Architecture:** Add one backend dashboard aggregation endpoint for model distribution, then update the stats page to consume summary, trends, recent events, and model-share data in parallel. The frontend will keep the existing page structure but replace list-based visuals with dedicated ECharts wrappers so the charts remain isolated and testable.

**Tech Stack:** Go, SQLite, React, TypeScript, Ant Design, ECharts, Vitest.

---

### Task 1: Add failing backend tests for model distribution

**Files:**
- Modify: `backend/internal/usage/repository_test.go`
- Modify: `backend/internal/api/dashboard_handler_test.go`

**Step 1: Write the failing tests**
- Add a usage repository test proving the filtered window aggregates request counts by model across all matching events.
- Add a dashboard handler test proving `GET /dashboard/model-distribution` returns the aggregated model rows.

**Step 2: Run tests to verify they fail**
Run: `cd backend && go test ./internal/usage ./internal/api -run 'TestSQLiteRepositoryModelDistribution|TestDashboardHandlerModelDistribution' -count=1`
Expected: FAIL because the repository and handler do not expose model distribution yet.

**Step 3: Write minimal implementation**
- Extend `backend/internal/usage/types.go`, `backend/internal/usage/repository.go`, `backend/internal/api/dashboard_handler.go`, and `backend/internal/bootstrap/bootstrap.go`.

**Step 4: Run tests to verify they pass**
Run: `cd backend && go test ./internal/usage ./internal/api -run 'TestSQLiteRepositoryModelDistribution|TestDashboardHandlerModelDistribution' -count=1`
Expected: PASS.

### Task 2: Add failing frontend tests for the refreshed stats page

**Files:**
- Modify: `frontend/src/features/stats/StatsPage.test.tsx`
- Modify: `frontend/src/lib/api.ts`

**Step 1: Write the failing test**
- Assert the page renders `输入 Token` and `输出 Token`, no longer renders `总 Token` or `额度变化`, shows `模型分布`, and renders explicit chart containers for the line chart and donut chart.

**Step 2: Run test to verify it fails**
Run: `npm --prefix frontend test -- src/features/stats/StatsPage.test.tsx`
Expected: FAIL because the old cards and non-chart panels are still rendered.

**Step 3: Write minimal implementation**
- Add the frontend API call for model distribution.
- Add ECharts dependency and chart wrapper components.
- Update `frontend/src/features/stats/StatsPage.tsx` and `frontend/src/styles.css`.

**Step 4: Run test to verify it passes**
Run: `npm --prefix frontend test -- src/features/stats/StatsPage.test.tsx`
Expected: PASS.

### Task 3: Verify the full slices stay green

**Files:**
- Verify: `backend/internal/usage/repository_test.go`
- Verify: `backend/internal/api/dashboard_handler_test.go`
- Verify: `frontend/src/features/stats/StatsPage.test.tsx`

**Step 1: Run backend coverage**
Run: `cd backend && go test ./internal/usage ./internal/api -count=1`
Expected: PASS.

**Step 2: Run frontend coverage**
Run: `npm --prefix frontend test`
Expected: PASS.
