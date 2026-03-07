# Active Account Routing and Logging Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add detailed backend gateway logs, manual active-account selection, and immediate frontend reorder behavior while decoupling active selection from priority order.

**Architecture:** Persist a global active account flag on accounts, expose it in account APIs, and make routing always attempt active account first. Keep failover candidate pool and existing feasibility checks for fallback. Frontend uses optimistic updates for reorder and active toggle with visual highlight.

**Tech Stack:** Go, SQLite, React, Ant Design, Vitest, Go test

---

### Task 1: Add failing backend tests for active account preference

**Files:**
- Modify: `backend/internal/api/gateway_handler_test.go`
- Modify: `backend/internal/api/responses_handler_test.go`

### Task 2: Add failing backend tests for active account persistence/API output

**Files:**
- Modify: `backend/internal/accounts/repository_test.go`
- Modify: `backend/internal/api/accounts_handler_test.go`

### Task 3: Implement backend model/repository/API/logging changes

**Files:**
- Modify: `backend/internal/accounts/types.go`
- Modify: `backend/internal/accounts/repository.go`
- Modify: `backend/internal/store/sqlite/migrations.go`
- Modify: `backend/internal/store/sqlite/store.go`
- Modify: `backend/internal/api/accounts_handler.go`
- Modify: `backend/internal/api/gateway_handler.go`
- Modify: `backend/internal/api/responses_handler.go`

### Task 4: Add failing frontend tests for active account UI and reorder behavior

**Files:**
- Modify: `frontend/src/features/accounts/AccountsPage.test.tsx`

### Task 5: Implement frontend API/UI changes

**Files:**
- Modify: `frontend/src/lib/api.ts`
- Modify: `frontend/src/features/accounts/AccountsPage.tsx`
- Modify: `frontend/src/styles.css`

### Task 6: Verification

**Commands:**
- `cd backend && go test ./internal/accounts ./internal/api ./internal/routing`
- `cd frontend && npm test -- --run AccountsPage.test.tsx`
