# Routing Order And Active Account Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Unify account ordering and active-account semantics so the homepage drag order becomes the real routing order, the current in-use account is unambiguous in the UI, and auto failover can be toggled on/off cleanly.

**Architecture:** Backend routing will derive candidate order from account priority only. When auto failover is enabled, handlers iterate candidates in that order and persist the account actually selected for routing as the current active account; when disabled, handlers use only the currently active account and pass upstream errors through unchanged. Frontend account cards will render strictly by priority order and settings will expose only one failover switch under proxy controls.

**Tech Stack:** Go, SQLite, React, TypeScript, Ant Design, Vitest.

---

### Task 1: Lock backend routing behavior with failing tests

**Files:**
- Modify: `backend/internal/api/responses_handler_test.go`
- Modify: `backend/internal/api/gateway_handler_test.go`

**Step 1: Write the failing tests**
- Add a thin responses test proving that with auto failover disabled, only the currently active account is used and a 429/usage-limit error is returned directly without switching.
- Add a thin responses test proving that with auto failover enabled, candidate selection follows `priority DESC` instead of the legacy failover queue.
- Add a gateway test proving that successful automatic failover updates the active account to the account that actually served the request.

**Step 2: Run tests to verify they fail**
Run: `go test ./internal/api -run 'TestResponsesHandlerThinMode|TestGateway' -count=1`
Expected: FAIL because routing still uses the explicit failover queue and does not synchronize the active account after failover.

**Step 3: Write minimal implementation**
- Update routing helpers in `backend/internal/api/responses_handler.go` and `backend/internal/api/gateway_handler.go`.
- Remove runtime dependence on `failover_queue_items` for candidate order.
- Add a small helper that persists the routed account as active when a request is successfully served by a different account.

**Step 4: Run tests to verify they pass**
Run: `go test ./internal/api -run 'TestResponsesHandlerThinMode|TestGateway' -count=1`
Expected: PASS.

### Task 2: Lock frontend ordering and settings UX with failing tests

**Files:**
- Modify: `frontend/src/features/accounts/AccountsPage.test.tsx`
- Modify: `frontend/src/features/settings/SettingsPage.test.tsx`

**Step 1: Write the failing tests**
- Add an accounts page test proving fetched cards render in strict priority order and drag-save persists the new priorities immediately.
- Add a settings page test proving the proxy tab shows the auto failover switch under the proxy switch area and no longer renders the standalone automatic failover queue card.

**Step 2: Run tests to verify they fail**
Run: `bash scripts/ci/run_frontend_unit_tests.sh`
Expected: FAIL because account ordering is not normalized by priority and settings still render the old failover section.

**Step 3: Write minimal implementation**
- Normalize account order in `frontend/src/features/accounts/AccountsPage.tsx`.
- Simplify `frontend/src/features/settings/SettingsPage.tsx` to keep only the switch under proxy controls.

**Step 4: Run tests to verify they pass**
Run: `bash scripts/ci/run_frontend_unit_tests.sh`
Expected: PASS.

### Task 3: Make settings defaults and API semantics consistent

**Files:**
- Modify: `backend/internal/settings/repository.go`
- Modify: `backend/internal/api/settings_handler_test.go`
- Modify: `frontend/src/App.test.tsx`

**Step 1: Write the failing tests**
- Add coverage proving default app settings return `auto_failover_enabled=true`.
- Add a settings API test proving legacy failover queue persistence is ignored by runtime routing order.

**Step 2: Run tests to verify they fail**
Run: `go test ./internal/settings ./internal/api -run 'Test.*Failover|TestSettings' -count=1`
Expected: FAIL because the default remains disabled and routing still depends on the explicit queue.

**Step 3: Write minimal implementation**
- Change the default app settings in `backend/internal/settings/repository.go`.
- Keep queue APIs compatible if needed, but remove their effect from runtime ordering.
- Update frontend tests/fixtures that assume the old default.

**Step 4: Run tests to verify they pass**
Run: `go test ./internal/settings ./internal/api -run 'Test.*Failover|TestSettings' -count=1 && bash scripts/ci/run_frontend_unit_tests.sh`
Expected: PASS.
