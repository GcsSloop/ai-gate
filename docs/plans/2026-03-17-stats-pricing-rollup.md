# Stats Pricing Rollup Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Upgrade stats windows, token formatting, pricing configuration, and historical usage sparsification without regressing dashboard responsiveness.

**Architecture:** The backend will resolve calendar-aligned ranges, effective pricing, and rollup-aware aggregation. The frontend will keep the stats page mounted while filters change and will animate chart data updates instead of rebuilding the page.

**Tech Stack:** Go, SQLite, React, TypeScript, Ant Design, ECharts

---

### Task 1: Add pricing settings types and persistence

**Files:**
- Modify: `backend/internal/settings/types.go`
- Modify: `backend/internal/settings/repository.go`
- Modify: `frontend/src/lib/api.ts`
- Test: `backend/internal/settings/repository_test.go`

**Step 1: Write the failing test**

Add a repository test that saves and reloads provider-level and account-level pricing rules.

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/settings -run Pricing -count=1`
Expected: FAIL because pricing fields are not persisted yet.

**Step 3: Write minimal implementation**

Add pricing rule structs to settings types and wire them into settings persistence and API payload types.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/settings -run Pricing -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/settings/types.go backend/internal/settings/repository.go backend/internal/settings/repository_test.go frontend/src/lib/api.ts
git commit -m "feat(settings): persist pricing rules"
```

### Task 2: Add rollup tables and compaction support

**Files:**
- Modify: `backend/internal/store/sqlite/migrations.go`
- Modify: `backend/internal/usage/types.go`
- Modify: `backend/internal/usage/repository.go`
- Test: `backend/internal/usage/repository_test.go`

**Step 1: Write the failing test**

Add tests covering:
- saving rollups
- aggregating rolled-up rows
- compaction deleting superseded raw rows

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/usage -run Rollup -count=1`
Expected: FAIL because rollup storage does not exist.

**Step 3: Write minimal implementation**

Add hourly and daily rollup tables, repository methods, and compaction helpers.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/usage -run Rollup -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/store/sqlite/migrations.go backend/internal/usage/types.go backend/internal/usage/repository.go backend/internal/usage/repository_test.go
git commit -m "feat(usage): add rollup storage and compaction"
```

### Task 3: Rework dashboard time windows and pricing resolution

**Files:**
- Modify: `backend/internal/api/dashboard_handler.go`
- Modify: `backend/internal/api/usage_events.go`
- Modify: `backend/internal/usage/repository.go`
- Test: `backend/internal/api/dashboard_handler_test.go`
- Test: `backend/internal/usage/repository_test.go`

**Step 1: Write the failing test**

Add tests for:
- `24h` returning 24 buckets for today
- `7d` and `30d` returning calendar-aligned daily buckets
- zero-filled gaps
- account-over-provider pricing precedence

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/api ./internal/usage -run 'Dashboard|Pricing' -count=1`
Expected: FAIL because the dashboard still uses rolling hours and static pricing.

**Step 3: Write minimal implementation**

Replace `hours` filtering with range keys, bucket synthesis, rollup-aware aggregation, and dynamic cost calculation.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/api ./internal/usage -run 'Dashboard|Pricing' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/api/dashboard_handler.go backend/internal/api/usage_events.go backend/internal/usage/repository.go backend/internal/api/dashboard_handler_test.go backend/internal/usage/repository_test.go
git commit -m "feat(stats): add calendar windows and pricing resolution"
```

### Task 4: Add pricing settings UI

**Files:**
- Modify: `frontend/src/features/settings/SettingsPage.tsx`
- Modify: `frontend/src/lib/i18n.ts`
- Modify: `frontend/src/styles.css`
- Test: `frontend/src/features/settings/SettingsPage.test.tsx`

**Step 1: Write the failing test**

Add a settings-page test covering provider pricing inputs, account pricing overrides, and save payload behavior.

**Step 2: Run test to verify it fails**

Run: `npm --prefix frontend test -- src/features/settings/SettingsPage.test.tsx`
Expected: FAIL because the pricing section is missing.

**Step 3: Write minimal implementation**

Add a compact pricing settings section aligned to the current settings design language.

**Step 4: Run test to verify it passes**

Run: `npm --prefix frontend test -- src/features/settings/SettingsPage.test.tsx`
Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/features/settings/SettingsPage.tsx frontend/src/features/settings/SettingsPage.test.tsx frontend/src/lib/i18n.ts frontend/src/styles.css
git commit -m "feat(settings): add pricing configuration UI"
```

### Task 5: Refresh stats page rendering and chart transitions

**Files:**
- Modify: `frontend/src/features/stats/StatsPage.tsx`
- Modify: `frontend/src/features/stats/StatsCharts.tsx`
- Modify: `frontend/src/styles.css`
- Test: `frontend/src/features/stats/StatsPage.test.tsx`

**Step 1: Write the failing test**

Add tests for:
- `K` and `M` token formatting
- no full-page loading state on range switch
- range requests using new range keys

**Step 2: Run test to verify it fails**

Run: `npm --prefix frontend test -- src/features/stats/StatsPage.test.tsx`
Expected: FAIL because the page still resets on each filter change and uses the old formatter.

**Step 3: Write minimal implementation**

Keep previous dashboard data mounted while fetching, update charts via animated series transitions, and unify compact token formatting.

**Step 4: Run test to verify it passes**

Run: `npm --prefix frontend test -- src/features/stats/StatsPage.test.tsx`
Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/features/stats/StatsPage.tsx frontend/src/features/stats/StatsCharts.tsx frontend/src/features/stats/StatsPage.test.tsx frontend/src/styles.css
git commit -m "feat(stats): smooth range switching and compact token units"
```

### Task 6: Run final verification

**Files:**
- Modify: none

**Step 1: Run backend verification**

Run: `cd backend && go test ./...`
Expected: PASS

**Step 2: Run frontend verification**

Run: `npm --prefix frontend test -- src/features/stats/StatsPage.test.tsx src/features/settings/SettingsPage.test.tsx`
Expected: PASS

**Step 3: Review final diff**

Run: `git status --short && git diff --stat`
Expected: only intended files changed.

**Step 4: Commit**

```bash
git add docs/plans/2026-03-17-stats-pricing-rollup-design.md docs/plans/2026-03-17-stats-pricing-rollup.md
git commit -m "docs: add stats pricing rollup plan"
```
