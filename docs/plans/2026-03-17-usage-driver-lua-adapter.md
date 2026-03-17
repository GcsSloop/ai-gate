# Usage Driver And Lua Adapter Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Introduce a plugin-style account driver and usage driver architecture with built-in drivers for official and adapted providers, plus strict-schema Lua adapters for new third-party usage APIs.

**Architecture:** Keep request forwarding in Go, split credential resolution from usage lookup, and run snapshot refresh in the background through a registry-driven orchestrator. Reuse `usage.Snapshot` as the routing model, extend it for metadata, and validate every Lua result before it reaches normalization or persistence.

**Tech Stack:** Go, SQLite, existing backend scheduler, Lua embedding runtime, repository tests, handler tests.

---

### Task 1: Add account configuration fields for driver selection

**Files:**
- Modify: `backend/internal/accounts/types.go`
- Modify: `backend/internal/accounts/repository.go`
- Modify: `backend/internal/store/sqlite/migrations.go`
- Modify: `backend/internal/store/sqlite/store.go`
- Test: `backend/internal/accounts/repository_test.go`
- Test: `backend/internal/store/sqlite/store_test.go`

**Step 1: Write the failing tests**

Add tests that create and reload an account with:

- `usage_driver`
- `usage_config_json`
- `account_driver`

Verify the values round-trip through SQLite.

**Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/accounts ./internal/store/sqlite -count=1`

Expected: FAIL because the schema and repository do not expose the new fields yet.

**Step 3: Write the minimal implementation**

- extend `accounts.Account`
- add new columns in migrations and `addColumnIfMissing`
- update create/list/get/update SQL in `backend/internal/accounts/repository.go`

**Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/accounts ./internal/store/sqlite -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/accounts/types.go backend/internal/accounts/repository.go backend/internal/store/sqlite/migrations.go backend/internal/store/sqlite/store.go backend/internal/accounts/repository_test.go backend/internal/store/sqlite/store_test.go
git commit -m "feat(accounts): persist usage driver configuration"
```

### Task 2: Extend usage snapshot metadata for heterogeneous sources

**Files:**
- Modify: `backend/internal/usage/types.go`
- Modify: `backend/internal/usage/repository.go`
- Modify: `backend/internal/store/sqlite/migrations.go`
- Modify: `backend/internal/store/sqlite/store.go`
- Test: `backend/internal/usage/repository_test.go`

**Step 1: Write the failing tests**

Add repository tests that save and reload snapshot metadata:

- `source`
- `confidence`
- `provider_snapshot_json`
- `stale`
- `last_error`

**Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/usage ./internal/store/sqlite -count=1`

Expected: FAIL because the columns and repository mapping do not exist.

**Step 3: Write the minimal implementation**

- extend `usage.Snapshot`
- add columns and repository scan/insert logic

**Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/usage ./internal/store/sqlite -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/usage/types.go backend/internal/usage/repository.go backend/internal/store/sqlite/migrations.go backend/internal/store/sqlite/store.go backend/internal/usage/repository_test.go
git commit -m "feat(usage): add snapshot source metadata"
```

### Task 3: Introduce driver interfaces and registry

**Files:**
- Create: `backend/internal/accountdrv/types.go`
- Create: `backend/internal/usagedrv/types.go`
- Create: `backend/internal/usagedrv/registry/registry.go`
- Create: `backend/internal/usagedrv/registry/registry_test.go`

**Step 1: Write the failing tests**

Add registry tests for:

- built-in official driver selection
- built-in adapted provider selection
- Lua driver selection when `usage_driver == "lua"`
- clear error when no driver matches

**Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/usagedrv/registry -count=1`

Expected: FAIL because the registry package does not exist.

**Step 3: Write the minimal implementation**

Define:

- `ResolvedCredential`
- `RawUsageResult`
- `UsageLimits`
- `AccountDriver`
- `UsageDriver`
- registry struct and lookup rules

**Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/usagedrv/registry -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/accountdrv/types.go backend/internal/usagedrv/types.go backend/internal/usagedrv/registry/registry.go backend/internal/usagedrv/registry/registry_test.go
git commit -m "feat(usage): add driver registry"
```

### Task 4: Build account drivers for API key and official session flows

**Files:**
- Create: `backend/internal/accountdrv/apikey_driver.go`
- Create: `backend/internal/accountdrv/official_driver.go`
- Create: `backend/internal/accountdrv/accountdrv_test.go`
- Modify: `backend/internal/auth/local_import.go` (only if needed for reuse)

**Step 1: Write the failing tests**

Add tests that:

- resolve API key accounts into bearer credentials
- resolve local-import official accounts into access token plus metadata
- classify missing/expired data as auth errors

**Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/accountdrv -count=1`

Expected: FAIL because the package does not exist.

**Step 3: Write the minimal implementation**

- implement API key resolution
- wrap existing official auth/session preparation without changing request-path behavior

**Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/accountdrv -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/accountdrv/apikey_driver.go backend/internal/accountdrv/official_driver.go backend/internal/accountdrv/accountdrv_test.go backend/internal/auth/local_import.go
git commit -m "feat(accountdrv): add built-in credential drivers"
```

### Task 5: Move official and adapted usage lookup into built-in drivers

**Files:**
- Create: `backend/internal/usagedrv/builtin/openai_official.go`
- Create: `backend/internal/usagedrv/builtin/ppchat.go`
- Create: `backend/internal/usagedrv/builtin/builtin_test.go`
- Modify: `backend/internal/api/accounts_handler.go`

**Step 1: Write the failing tests**

Add tests for:

- official usage response parsing into raw limits
- adapted provider response parsing into raw limits
- error classification for auth, quota, and upstream failures

**Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/usagedrv/builtin -count=1`

Expected: FAIL because the drivers do not exist.

**Step 3: Write the minimal implementation**

- extract current built-in usage parsing from `backend/internal/api/accounts_handler.go`
- return `RawUsageResult` instead of writing snapshots directly

**Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/usagedrv/builtin -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/usagedrv/builtin/openai_official.go backend/internal/usagedrv/builtin/ppchat.go backend/internal/usagedrv/builtin/builtin_test.go backend/internal/api/accounts_handler.go
git commit -m "refactor(usage): extract built-in usage drivers"
```

### Task 6: Add raw-result normalization into `usage.Snapshot`

**Files:**
- Create: `backend/internal/usage/normalize/normalize.go`
- Create: `backend/internal/usage/normalize/normalize_test.go`
- Modify: `backend/internal/routing/feasibility.go`
- Modify: `backend/internal/api/responses_handler.go`
- Modify: `backend/internal/api/gateway_handler.go`

**Step 1: Write the failing tests**

Add normalization tests covering:

- official window limits
- third-party quota/rate limits
- balance-only providers
- stale/low-confidence snapshots

Add routing feasibility tests to ensure official and third-party limit families both remain valid.

**Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/usage/normalize ./internal/routing -count=1`

Expected: FAIL because the normalizer does not exist yet.

**Step 3: Write the minimal implementation**

- normalize `RawUsageResult` into snapshot fields
- update routing checks only where snapshot metadata affects feasibility

**Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/usage/normalize ./internal/routing -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/usage/normalize/normalize.go backend/internal/usage/normalize/normalize_test.go backend/internal/routing/feasibility.go backend/internal/api/responses_handler.go backend/internal/api/gateway_handler.go
git commit -m "feat(usage): normalize official and third-party limits"
```

### Task 7: Add strict-schema Lua runtime

**Files:**
- Create: `backend/internal/usagedrv/lua/runtime.go`
- Create: `backend/internal/usagedrv/lua/schema.go`
- Create: `backend/internal/usagedrv/lua/runtime_test.go`
- Create: `backend/internal/usagedrv/lua/testdata/minimal_ok.lua`
- Create: `backend/internal/usagedrv/lua/testdata/invalid_shape.lua`

**Step 1: Write the failing tests**

Add tests that verify:

- only host APIs are exposed
- `fetch_usage(ctx)` is required
- valid scripts return normalized `RawUsageResult`
- invalid scripts are rejected by strict schema validation
- timeouts are surfaced as timeout errors

**Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/usagedrv/lua -count=1`

Expected: FAIL because the runtime package does not exist.

**Step 3: Write the minimal implementation**

- embed Lua runtime
- expose only controlled host functions
- decode Lua tables into Go
- validate exact schema before returning

**Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/usagedrv/lua -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/usagedrv/lua/runtime.go backend/internal/usagedrv/lua/schema.go backend/internal/usagedrv/lua/runtime_test.go backend/internal/usagedrv/lua/testdata/minimal_ok.lua backend/internal/usagedrv/lua/testdata/invalid_shape.lua
git commit -m "feat(usage): add strict Lua usage runtime"
```

### Task 8: Add one Lua-backed usage driver implementation path

**Files:**
- Create: `backend/internal/usagedrv/lua/driver.go`
- Create: `backend/internal/usagedrv/lua/driver_test.go`
- Create: `backend/internal/usagedrv/lua/testdata/vendor_x.lua`

**Step 1: Write the failing tests**

Add tests that:

- select the Lua driver from account config
- pass account/config/credential context into the script
- reject malformed `usage_config_json`

**Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/usagedrv/lua -run TestLuaDriver -count=1`

Expected: FAIL because the driver integration path is missing.

**Step 3: Write the minimal implementation**

- implement account-config-driven Lua driver
- load the configured script
- pass a read-only context object

**Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/usagedrv/lua -run TestLuaDriver -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/usagedrv/lua/driver.go backend/internal/usagedrv/lua/driver_test.go backend/internal/usagedrv/lua/testdata/vendor_x.lua
git commit -m "feat(usage): wire Lua usage driver"
```

### Task 9: Orchestrate background snapshot refresh through drivers

**Files:**
- Create: `backend/internal/usage/refresh/orchestrator.go`
- Create: `backend/internal/usage/refresh/orchestrator_test.go`
- Modify: `backend/internal/bootstrap/bootstrap.go`
- Modify: `backend/internal/scheduler/usage_compaction_job.go` (only if coordination is needed)

**Step 1: Write the failing tests**

Add orchestrator tests for:

- successful built-in refresh
- successful Lua refresh
- one-account failure does not stop the batch
- stale metadata preserved on failure

Add bootstrap test coverage that the orchestrator is registered and runs in the background loop.

**Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/usage/refresh ./internal/bootstrap -count=1`

Expected: FAIL because the orchestrator does not exist.

**Step 3: Write the minimal implementation**

- iterate accounts
- resolve credentials
- fetch raw usage
- normalize
- save snapshots
- record stale/error metadata on failure

**Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/usage/refresh ./internal/bootstrap -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/usage/refresh/orchestrator.go backend/internal/usage/refresh/orchestrator_test.go backend/internal/bootstrap/bootstrap.go backend/internal/scheduler/usage_compaction_job.go
git commit -m "feat(usage): refresh snapshots through driver orchestrator"
```

### Task 10: Expose driver configuration through the account API

**Files:**
- Modify: `backend/internal/api/accounts_handler.go`
- Modify: `backend/internal/api/accounts_handler_test.go`
- Modify: `frontend/src/lib/api.ts`

**Step 1: Write the failing tests**

Add handler tests that create and update accounts with:

- `usage_driver`
- `usage_config_json`
- `account_driver`

Verify round-trip payload behavior and defaulting for built-in providers.

**Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/api -run Accounts -count=1`

Expected: FAIL because the API contract does not include the new fields.

**Step 3: Write the minimal implementation**

- extend request/response DTOs
- pass configuration through to the repository
- update frontend API types only, not the full UI editor yet

**Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/api -run Accounts -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/api/accounts_handler.go backend/internal/api/accounts_handler_test.go frontend/src/lib/api.ts
git commit -m "feat(accounts): expose usage driver configuration"
```

### Task 11: Run full verification

**Files:**
- No code changes required unless verification uncovers regressions

**Step 1: Run backend tests**

Run: `cd backend && go test ./... -count=1`

Expected: PASS

**Step 2: Run frontend tests**

Run: `cd frontend && npm test`

Expected: PASS

**Step 3: Run frontend build**

Run: `cd frontend && npm run build`

Expected: PASS

**Step 4: Review git diff**

Run: `git status --short --branch`

Expected: only intended files are changed

**Step 5: Commit final cleanup if needed**

```bash
git add -A
git commit -m "chore: finalize usage driver refactor"
```
