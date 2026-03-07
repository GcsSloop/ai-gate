# Codex Router Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a modular-monolith Codex router with a web console, official-account authorization, third-party API account management, usage-aware routing, cooldown auto-recovery, and an OpenAI-compatible gateway.

**Architecture:** Use a Go backend with clean internal modules for accounts, routing, sessions, scheduling, and provider adapters, plus a React/Vite frontend for management. Store state in SQLite, encrypt secrets at rest, and expose OpenAI-compatible inbound APIs while keeping management APIs separate from gateway traffic.

**Tech Stack:** Go, SQLite, React, Vite, TypeScript, HTTP streaming, table-driven Go tests, frontend component tests, end-to-end smoke tests

---

### Task 1: Bootstrap repository structure

**Files:**
- Create: `backend/go.mod`
- Create: `backend/cmd/routerd/main.go`
- Create: `backend/internal/`
- Create: `frontend/package.json`
- Create: `frontend/src/main.tsx`
- Create: `Makefile`
- Create: `.gitignore`
- Create: `README.md`

**Step 1: Write the failing test**

Create `backend/internal/bootstrap/bootstrap_test.go` with a test that expects a config load function and a router service constructor to exist.

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/bootstrap -run TestNewApp -count=1`
Expected: FAIL because the package and constructor do not exist yet.

**Step 3: Write minimal implementation**

- initialize `go.mod`
- add `main.go` with a stub server startup
- add a minimal `bootstrap` package with `NewApp`
- scaffold frontend with a minimal Vite entrypoint

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/bootstrap -run TestNewApp -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add .gitignore Makefile README.md backend frontend
git commit -m "chore: bootstrap codex router workspace"
```

### Task 2: Define backend configuration model

**Files:**
- Create: `backend/internal/config/config.go`
- Create: `backend/internal/config/config_test.go`
- Modify: `backend/cmd/routerd/main.go`

**Step 1: Write the failing test**

Add tests for:
- default listen address
- SQLite path resolution
- encryption key validation
- scheduler interval parsing

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/config -count=1`
Expected: FAIL with missing config types or validation logic.

**Step 3: Write minimal implementation**

Implement a config loader that reads environment variables and returns validated runtime configuration.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/config -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/config backend/cmd/routerd/main.go
git commit -m "feat: add backend configuration loader"
```

### Task 3: Add database bootstrap and migrations

**Files:**
- Create: `backend/internal/store/sqlite/store.go`
- Create: `backend/internal/store/sqlite/migrations.go`
- Create: `backend/internal/store/sqlite/store_test.go`
- Create: `backend/internal/store/sqlite/testdata/`

**Step 1: Write the failing test**

Add tests that create a temporary SQLite database and verify migrations create tables for:
- providers
- accounts
- usage snapshots
- routing policies
- conversations
- messages
- runs

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/store/sqlite -count=1`
Expected: FAIL because migrations and schema bootstrap are missing.

**Step 3: Write minimal implementation**

Implement store initialization and schema creation with idempotent migrations.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/store/sqlite -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/store/sqlite
git commit -m "feat: add sqlite schema bootstrap"
```

### Task 4: Implement encrypted credential storage

**Files:**
- Create: `backend/internal/secrets/crypto.go`
- Create: `backend/internal/secrets/crypto_test.go`
- Modify: `backend/internal/store/sqlite/store.go`

**Step 1: Write the failing test**

Add tests for:
- encrypt/decrypt roundtrip
- invalid key rejection
- ciphertext differs for same plaintext across different nonces

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/secrets -count=1`
Expected: FAIL because the crypto helper does not exist.

**Step 3: Write minimal implementation**

Implement credential encryption helpers and wire encrypted blobs into account persistence.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/secrets -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/secrets backend/internal/store/sqlite/store.go
git commit -m "feat: add encrypted credential storage"
```

### Task 5: Model provider and account domain types

**Files:**
- Create: `backend/internal/accounts/types.go`
- Create: `backend/internal/accounts/repository.go`
- Create: `backend/internal/accounts/repository_test.go`

**Step 1: Write the failing test**

Add tests covering:
- creation of OAuth and API-key accounts
- status transitions between `active`, `cooldown`, `degraded`, `invalid`, `disabled`
- persisted `cooldown_until`

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/accounts -count=1`
Expected: FAIL because domain types and repository methods are missing.

**Step 3: Write minimal implementation**

Define domain structs, repository interfaces, and SQLite-backed implementations needed for account lifecycle management.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/accounts -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/accounts
git commit -m "feat: add account domain model"
```

### Task 6: Add usage snapshot and health persistence

**Files:**
- Create: `backend/internal/usage/types.go`
- Create: `backend/internal/usage/repository.go`
- Create: `backend/internal/usage/repository_test.go`
- Modify: `backend/internal/store/sqlite/migrations.go`

**Step 1: Write the failing test**

Add tests for writing and loading usage snapshots, including latest health score and cooldown metadata.

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/usage -count=1`
Expected: FAIL because usage persistence is not implemented.

**Step 3: Write minimal implementation**

Implement snapshot persistence and latest-state retrieval methods for routing decisions.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/usage -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/usage backend/internal/store/sqlite/migrations.go
git commit -m "feat: persist account usage snapshots"
```

### Task 7: Build routing feasibility checks

**Files:**
- Create: `backend/internal/routing/feasibility.go`
- Create: `backend/internal/routing/feasibility_test.go`

**Step 1: Write the failing test**

Add table-driven tests for:
- enough quota to satisfy projected turn
- insufficient balance
- insufficient RPM / TPM
- safety multiplier preventing risky selection

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/routing -run TestFeasibility -count=1`
Expected: FAIL because feasibility logic is missing.

**Step 3: Write minimal implementation**

Implement token budget estimation input structures and feasibility decision logic.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/routing -run TestFeasibility -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/routing
git commit -m "feat: add routing feasibility checks"
```

### Task 8: Implement account scoring and candidate selection

**Files:**
- Create: `backend/internal/routing/scoring.go`
- Create: `backend/internal/routing/scoring_test.go`
- Modify: `backend/internal/accounts/types.go`
- Modify: `backend/internal/usage/types.go`

**Step 1: Write the failing test**

Add tests that verify account ordering using:
- manual priority
- health score
- latency
- throttling penalties
- degraded state penalties

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/routing -run TestScoreCandidates -count=1`
Expected: FAIL because scoring and ordering are missing.

**Step 3: Write minimal implementation**

Implement weighted scoring and sorted candidate selection with explicit exclusion reasons.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/routing -run TestScoreCandidates -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/routing backend/internal/accounts/types.go backend/internal/usage/types.go
git commit -m "feat: add account scoring logic"
```

### Task 9: Implement cooldown lifecycle and auto-recovery rules

**Files:**
- Create: `backend/internal/routing/cooldown.go`
- Create: `backend/internal/routing/cooldown_test.go`
- Modify: `backend/internal/accounts/repository.go`

**Step 1: Write the failing test**

Add tests for:
- explicit cooldown timestamps
- inferred rolling-window cooldown
- probe eligibility after `cooldown_until`
- `disabled` and `invalid` accounts never auto-reactivating

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/routing -run TestCooldownLifecycle -count=1`
Expected: FAIL because cooldown logic is missing.

**Step 3: Write minimal implementation**

Implement cooldown transition helpers and reactivation eligibility rules.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/routing -run TestCooldownLifecycle -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/routing backend/internal/accounts/repository.go
git commit -m "feat: add cooldown lifecycle rules"
```

### Task 10: Add provider adapter interfaces

**Files:**
- Create: `backend/internal/providers/provider.go`
- Create: `backend/internal/providers/openai/adapter.go`
- Create: `backend/internal/providers/provider_test.go`

**Step 1: Write the failing test**

Add tests that assert a provider adapter can:
- build outbound requests
- expose capability flags
- classify errors into routing categories

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/providers/... -count=1`
Expected: FAIL because provider interfaces are missing.

**Step 3: Write minimal implementation**

Define adapter contracts and add a first OpenAI-compatible adapter stub with error classification helpers.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/providers/... -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/providers
git commit -m "feat: add provider adapter interfaces"
```

### Task 11: Implement official-account auth connector interfaces

**Files:**
- Create: `backend/internal/auth/oauth.go`
- Create: `backend/internal/auth/oauth_test.go`
- Create: `backend/internal/auth/session_state.go`

**Step 1: Write the failing test**

Add tests for:
- authorization URL generation
- callback state validation
- token refresh decision
- invalid refresh token handling

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/auth -count=1`
Expected: FAIL because auth connector code does not exist.

**Step 3: Write minimal implementation**

Implement generic OAuth connector interfaces and a provider-specific configuration layer for official accounts.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/auth -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/auth
git commit -m "feat: add official account auth connector"
```

### Task 12: Implement account service APIs

**Files:**
- Create: `backend/internal/api/accounts_handler.go`
- Create: `backend/internal/api/accounts_handler_test.go`
- Modify: `backend/internal/accounts/repository.go`

**Step 1: Write the failing test**

Add handler tests for:
- creating a third-party account
- creating an auth session for official account linking
- listing accounts with cooldown countdowns
- manually disabling an account

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/api -run TestAccountsHandler -count=1`
Expected: FAIL because account handlers are missing.

**Step 3: Write minimal implementation**

Implement REST handlers and input validation for account management.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/api -run TestAccountsHandler -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/api backend/internal/accounts/repository.go
git commit -m "feat: add account management api"
```

### Task 13: Implement routing policy service APIs

**Files:**
- Create: `backend/internal/policy/types.go`
- Create: `backend/internal/policy/repository.go`
- Create: `backend/internal/api/policy_handler.go`
- Create: `backend/internal/api/policy_handler_test.go`

**Step 1: Write the failing test**

Add tests for:
- saving candidate order
- saving thresholds and safety multipliers
- assigning model pools
- validating invalid policy payloads

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/api -run TestPolicyHandler -count=1`
Expected: FAIL because policy service and handlers are missing.

**Step 3: Write minimal implementation**

Implement policy domain types, persistence, and HTTP handlers.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/api -run TestPolicyHandler -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/policy backend/internal/api
git commit -m "feat: add routing policy api"
```

### Task 14: Add conversation and run persistence

**Files:**
- Create: `backend/internal/conversations/types.go`
- Create: `backend/internal/conversations/repository.go`
- Create: `backend/internal/conversations/repository_test.go`

**Step 1: Write the failing test**

Add tests for:
- creating a conversation
- appending messages
- recording chained runs
- persisting partial stream output offsets

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/conversations -count=1`
Expected: FAIL because conversation persistence is missing.

**Step 3: Write minimal implementation**

Implement repositories for conversation history, messages, and runs.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/conversations -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/conversations
git commit -m "feat: add conversation and run persistence"
```

### Task 15: Implement gateway request normalization

**Files:**
- Create: `backend/internal/gateway/openai/request.go`
- Create: `backend/internal/gateway/openai/request_test.go`
- Create: `backend/internal/api/gateway_handler.go`

**Step 1: Write the failing test**

Add tests for:
- parsing OpenAI-compatible chat completion payloads
- preserving streaming flag
- extracting model and message history
- rejecting malformed requests

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/gateway/openai -count=1`
Expected: FAIL because request normalization is missing.

**Step 3: Write minimal implementation**

Implement normalized request structs and a gateway handler entrypoint.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/gateway/openai -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/gateway backend/internal/api/gateway_handler.go
git commit -m "feat: add openai-compatible gateway request parser"
```

### Task 16: Implement non-stream routing execution

**Files:**
- Create: `backend/internal/routing/executor.go`
- Create: `backend/internal/routing/executor_test.go`
- Modify: `backend/internal/providers/openai/adapter.go`
- Modify: `backend/internal/api/gateway_handler.go`

**Step 1: Write the failing test**

Add tests for:
- selecting the best candidate
- retrying once on soft failure
- failing over on capacity failure
- persisting run records and exclusion reasons

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/routing -run TestExecuteNonStream -count=1`
Expected: FAIL because the executor does not exist.

**Step 3: Write minimal implementation**

Implement non-stream execution, failure classification, and repository updates.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/routing -run TestExecuteNonStream -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/routing backend/internal/providers/openai/adapter.go backend/internal/api/gateway_handler.go
git commit -m "feat: add non-stream routing executor"
```

### Task 17: Implement stream proxy and continuation chain

**Files:**
- Create: `backend/internal/streaming/proxy.go`
- Create: `backend/internal/streaming/proxy_test.go`
- Modify: `backend/internal/routing/executor.go`
- Modify: `backend/internal/conversations/repository.go`

**Step 1: Write the failing test**

Add tests for:
- forwarding incremental stream chunks
- capturing partial output on failure
- issuing a continuation run with `fallback_from_run_id`
- preventing duplicate replay markers

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/streaming -count=1`
Expected: FAIL because the stream proxy and continuation logic are missing.

**Step 3: Write minimal implementation**

Implement buffered stream forwarding, partial output capture, and chained continuation requests.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/streaming -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/streaming backend/internal/routing/executor.go backend/internal/conversations/repository.go
git commit -m "feat: add stream continuation failover"
```

### Task 18: Add scheduler for refresh, cooldown recovery, and probes

**Files:**
- Create: `backend/internal/scheduler/scheduler.go`
- Create: `backend/internal/scheduler/scheduler_test.go`
- Create: `backend/internal/scheduler/jobs.go`
- Modify: `backend/cmd/routerd/main.go`

**Step 1: Write the failing test**

Add tests for:
- due job scheduling
- cooldown expiry detection
- successful probe restoring `active`
- failed probe extending cooldown
- skipping `disabled` and `invalid`

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/scheduler -count=1`
Expected: FAIL because the scheduler package is missing.

**Step 3: Write minimal implementation**

Implement a periodic scheduler with jobs for usage refresh, auth refresh, and cooldown recovery probes.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/scheduler -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/scheduler backend/cmd/routerd/main.go
git commit -m "feat: add background scheduler"
```

### Task 19: Expose monitoring and conversation inspection APIs

**Files:**
- Create: `backend/internal/api/monitoring_handler.go`
- Create: `backend/internal/api/monitoring_handler_test.go`
- Create: `backend/internal/api/conversations_handler.go`
- Create: `backend/internal/api/conversations_handler_test.go`

**Step 1: Write the failing test**

Add tests for:
- usage dashboard aggregation
- account status counts
- conversation list pagination
- run-chain inspection for failover history

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/api -run 'Test(Monitoring|Conversations)' -count=1`
Expected: FAIL because monitoring handlers are missing.

**Step 3: Write minimal implementation**

Implement read APIs for dashboards and conversation inspection.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/api -run 'Test(Monitoring|Conversations)' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/api
git commit -m "feat: add monitoring and conversation apis"
```

### Task 20: Bootstrap frontend application shell

**Files:**
- Create: `frontend/src/App.tsx`
- Create: `frontend/src/routes.tsx`
- Create: `frontend/src/styles.css`
- Create: `frontend/src/lib/api.ts`
- Create: `frontend/src/components/layout/`

**Step 1: Write the failing test**

Add frontend tests for:
- route rendering
- navigation shell visibility
- API client base URL handling

**Step 2: Run test to verify it fails**

Run: `cd frontend && npm test -- --runInBand`
Expected: FAIL because the frontend app shell does not exist.

**Step 3: Write minimal implementation**

Create the React app shell, route definitions, and a typed API client wrapper.

**Step 4: Run test to verify it passes**

Run: `cd frontend && npm test -- --runInBand`
Expected: PASS

**Step 5: Commit**

```bash
git add frontend
git commit -m "feat: add frontend app shell"
```

### Task 21: Build account management UI

**Files:**
- Create: `frontend/src/features/accounts/AccountsPage.tsx`
- Create: `frontend/src/features/accounts/AccountForm.tsx`
- Create: `frontend/src/features/accounts/AccountsPage.test.tsx`

**Step 1: Write the failing test**

Add tests for:
- listing accounts
- showing cooldown countdowns
- submitting a third-party account form
- launching official authorization flow

**Step 2: Run test to verify it fails**

Run: `cd frontend && npm test -- AccountsPage --runInBand`
Expected: FAIL because the account management UI is missing.

**Step 3: Write minimal implementation**

Build the account page and form flows backed by the control API.

**Step 4: Run test to verify it passes**

Run: `cd frontend && npm test -- AccountsPage --runInBand`
Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/features/accounts
git commit -m "feat: add account management ui"
```

### Task 22: Build routing policy UI

**Files:**
- Create: `frontend/src/features/policy/PolicyPage.tsx`
- Create: `frontend/src/features/policy/PolicyPage.test.tsx`

**Step 1: Write the failing test**

Add tests for:
- editing thresholds
- ordering account candidates
- saving model pool bindings

**Step 2: Run test to verify it fails**

Run: `cd frontend && npm test -- PolicyPage --runInBand`
Expected: FAIL because the policy page is missing.

**Step 3: Write minimal implementation**

Implement a policy editor with validation and save actions.

**Step 4: Run test to verify it passes**

Run: `cd frontend && npm test -- PolicyPage --runInBand`
Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/features/policy
git commit -m "feat: add routing policy ui"
```

### Task 23: Build monitoring and conversation UI

**Files:**
- Create: `frontend/src/features/monitoring/MonitoringPage.tsx`
- Create: `frontend/src/features/conversations/ConversationsPage.tsx`
- Create: `frontend/src/features/monitoring/MonitoringPage.test.tsx`
- Create: `frontend/src/features/conversations/ConversationsPage.test.tsx`

**Step 1: Write the failing test**

Add tests for:
- rendering usage charts/cards
- rendering account state badges
- listing conversations and run chains

**Step 2: Run test to verify it fails**

Run: `cd frontend && npm test -- MonitoringPage ConversationsPage --runInBand`
Expected: FAIL because monitoring pages are missing.

**Step 3: Write minimal implementation**

Implement the dashboards and conversation inspection views.

**Step 4: Run test to verify it passes**

Run: `cd frontend && npm test -- MonitoringPage ConversationsPage --runInBand`
Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/features/monitoring frontend/src/features/conversations
git commit -m "feat: add monitoring console ui"
```

### Task 24: Wire backend and frontend into a runnable local product

**Files:**
- Modify: `backend/cmd/routerd/main.go`
- Modify: `frontend/package.json`
- Modify: `Makefile`
- Modify: `README.md`

**Step 1: Write the failing test**

Add a backend smoke test and a frontend integration check that expect:
- backend serves control and gateway routes
- frontend dev server proxies API requests

**Step 2: Run test to verify it fails**

Run: `make test`
Expected: FAIL because local wiring and smoke coverage are incomplete.

**Step 3: Write minimal implementation**

Wire HTTP routing, static asset serving, frontend proxy config, and developer commands.

**Step 4: Run test to verify it passes**

Run: `make test`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/cmd/routerd/main.go frontend/package.json Makefile README.md
git commit -m "feat: wire local codex router runtime"
```

### Task 25: Add end-to-end flow verification

**Files:**
- Create: `scripts/test/e2e_smoke.sh`
- Create: `backend/testdata/fake_provider/`
- Create: `docs/testing.md`

**Step 1: Write the failing test**

Design the smoke test to cover:
- add API-key account
- simulate cooldown
- verify automatic recovery probe
- send routed request
- verify monitoring output

**Step 2: Run test to verify it fails**

Run: `bash scripts/test/e2e_smoke.sh`
Expected: FAIL until fake provider fixtures and smoke wiring exist.

**Step 3: Write minimal implementation**

Add fake provider fixtures, write the smoke script, and document local verification flow.

**Step 4: Run test to verify it passes**

Run: `bash scripts/test/e2e_smoke.sh`
Expected: PASS

**Step 5: Commit**

```bash
git add scripts/test/e2e_smoke.sh backend/testdata/fake_provider docs/testing.md
git commit -m "test: add codex router e2e smoke coverage"
```

### Task 26: Final verification and release checklist

**Files:**
- Modify: `README.md`
- Modify: `docs/testing.md`
- Create: `docs/release-checklist.md`

**Step 1: Write the failing test**

List the full verification commands and expected outcomes. Treat missing docs or stale commands as the failing condition.

**Step 2: Run test to verify it fails**

Run: `make test && bash scripts/test/e2e_smoke.sh`
Expected: FAIL if any documented command or release precondition is not yet valid.

**Step 3: Write minimal implementation**

Update docs so setup, verification, configuration, and operational constraints all match the finished product.

**Step 4: Run test to verify it passes**

Run: `make test && bash scripts/test/e2e_smoke.sh`
Expected: PASS

**Step 5: Commit**

```bash
git add README.md docs/testing.md docs/release-checklist.md
git commit -m "docs: finalize codex router release guidance"
```

## Notes for the Implementer

- Keep v1 inbound compatibility focused on OpenAI-style APIs.
- Do not promise byte-perfect stream continuity across account failover.
- Treat `cooldown` as recoverable and `disabled` / `invalid` as non-recoverable without explicit action.
- Add observability hooks early; they are required for debugging routing and recovery behavior.
- Prefer table-driven Go tests for routing decisions and fake-provider integration tests for adapters.
