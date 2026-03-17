# Active Account First Routing Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make all request routing prefer the current active account first without changing persisted manual account order.

**Architecture:** Backend routing will keep user-managed `priority` values unchanged and build request-time candidate order as active account first plus the remaining accounts in manual order. Both `/v1/responses` and `/v1/chat/completions` will use the same helper so successful failover updates the active account, and subsequent requests keep preferring that account until it becomes unusable.

**Tech Stack:** Go, SQLite, `net/http`, Go tests.

---

### Task 1: Lock active-account-first behavior with failing tests

**Files:**
- Modify: `backend/internal/api/responses_handler_test.go`
- Modify: `backend/internal/api/gateway_handler_test.go`

**Step 1: Write the failing tests**
- Add a thin responses test where a lower-priority active account succeeds and a higher-priority account must not be called.
- Add a gateway test with the same shape for `/v1/chat/completions`.

**Step 2: Run tests to verify they fail**
Run: `go test ./backend/internal/api -run 'TestResponsesHandlerThinModePrefersActiveAccountWhenAvailable|TestGatewayHandlerPrefersActiveAccountWhenAutoFailoverEnabled' -count=1`
Expected: FAIL because current auto-failover ordering still starts from persisted priority.

**Step 3: Write minimal implementation**
- Update shared routing helpers in `backend/internal/api/account_routing_state.go`.
- Switch `backend/internal/api/responses_handler.go` and `backend/internal/api/gateway_handler.go` to use the new helper.

**Step 4: Run tests to verify they pass**
Run: `go test ./backend/internal/api -run 'TestResponsesHandlerThinModePrefersActiveAccountWhenAvailable|TestGatewayHandlerPrefersActiveAccountWhenAutoFailoverEnabled' -count=1`
Expected: PASS.

### Task 2: Verify existing failover semantics still hold

**Files:**
- Verify: `backend/internal/api/responses_handler_test.go`
- Verify: `backend/internal/api/gateway_handler_test.go`

**Step 1: Run targeted regression coverage**
Run: `go test ./backend/internal/api -run 'TestResponsesHandlerThinMode|TestGatewayHandler' -count=1`
Expected: PASS.

**Step 2: Check workspace status**
Run: `git status --short`
Expected: only the intended backend test, code, and docs changes.
