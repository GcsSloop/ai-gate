# Usage Driver And Lua Adapter Design

**Goal:** Build a plugin-style account driver and usage driver architecture that supports official quota windows, built-in third-party integrations, and strict-schema Lua adapters for new third-party usage APIs.

**Status:** Approved for implementation.

---

## Context

The current codebase mixes three concerns:

1. account credential handling
2. request forwarding
3. usage snapshot collection

This works for the existing official and partially adapted providers, but it does not scale to many heterogeneous usage APIs. Some providers expose usage by API token, some require session login first, and some only expose partial quota data. The system needs a stable core model while allowing provider-specific usage lookup logic to evolve independently.

The request forwarding path should remain Go-native. Only usage collection for newly introduced third-party providers should become scriptable.

## Design Summary

Adopt a split architecture:

1. **Account driver**
   Resolves usable credentials or sessions for an account.

2. **Usage driver**
   Performs one usage lookup and returns a standard raw result.

3. **Snapshot normalizer**
   Converts raw provider limits into the existing `usage.Snapshot` model plus a small set of metadata fields.

4. **Refresh orchestrator**
   Runs in the background on a schedule, writes snapshots, and keeps routing decoupled from live quota queries.

Use two driver families:

- **Built-in drivers** for official accounts and already adapted providers
- **Lua usage drivers** for new third-party providers

Lua is restricted to host-provided capabilities only. It does not control routing, persistence, account status changes, or shell access.

## Architectural Boundaries

### What stays built-in

- `backend/internal/providers/*` request forwarding logic
- `/chat/completions` and `/responses` request path behavior
- official auth refresh and local-import auth handling
- failover classification and routing
- snapshot persistence and dashboard aggregation

### What moves behind drivers

- account credential/session resolution for usage refresh
- remote usage lookup
- provider-specific response parsing for usage APIs

### What Lua is allowed to do

- make HTTP requests through a host wrapper
- read normalized account/config/credential input
- parse JSON
- perform basic string/time/encoding work
- return one fixed schema

### What Lua is not allowed to do

- read or write arbitrary files
- execute processes
- open raw sockets
- write to the database
- mutate account state
- control retries, cooldowns, or routing

## Core Data Model

Keep `usage.Snapshot` as the routing and dashboard model, but extend it with metadata needed for heterogeneous sources.

Planned additions:

- `Source string`
- `Confidence string`
- `ProviderSnapshotJSON string`
- `Stale bool`
- `LastError string`

This preserves current routing compatibility:

- official accounts continue to use `PrimaryUsedPercent`, `SecondaryUsedPercent`, `PrimaryResetsAt`, `SecondaryResetsAt`
- third-party accounts can populate `Balance`, `QuotaRemaining`, `RPMRemaining`, `TPMRemaining`

Routing code should interpret whichever limit family is present, instead of assuming one global quota model.

## Driver Interfaces

### Account driver

Responsible for obtaining a valid credential representation for usage refresh.

Examples:

- API key driver
- official OAuth/local-import session driver
- cookie-session driver for future providers

Output shape:

- API key or bearer token
- optional refresh token
- optional cookie/session map
- optional extra headers
- optional metadata

### Usage driver

Responsible for one remote usage fetch.

Output shape:

- source
- confidence
- structured limits
- provider metadata
- optional raw payload preview for diagnostics

### Normalizer

Converts a raw limits object into `usage.Snapshot`.

Rules:

- official window percentages take precedence when present
- quota/rate limits fill in the generic remaining fields
- missing fields remain zero or nil and lower confidence
- normalization failure prevents snapshot overwrite

## Provider Strategy

### Built-in usage drivers

Use Go-native implementations for:

- OpenAI official / Codex official flow
- already adapted providers such as PPChat or any provider that already has stable parsing logic in the repository

Benefits:

- preserves mature behavior
- better testability for critical paths
- no script runtime overhead for core providers

### Lua usage drivers

Use Lua only for new third-party usage integrations.

Each Lua adapter is selected by account configuration and receives:

- account identity and provider type
- resolved credential/session
- driver-specific config
- host helper functions

Each script must implement one function:

```lua
function fetch_usage(ctx)
  return {
    ok = true,
    source = "remote",
    confidence = "high",
    limits = {
      balance = 10.5,
      quota_remaining = 120000,
      rpm_remaining = 60,
      tpm_remaining = 80000,
      daily_remaining = nil,
      monthly_remaining = nil,
      primary_used_percent = nil,
      secondary_used_percent = nil,
      primary_resets_at = nil,
      secondary_resets_at = nil
    },
    meta = {
      provider = "vendor-x"
    }
  }
end
```

Failure must return:

```lua
return {
  ok = false,
  error = {
    kind = "auth",
    message = "session expired"
  }
}
```

## Strict Validation

Every Lua result must pass host-side validation before use.

Validation rules:

- top-level value must be an object
- `ok` must exist and be boolean
- `ok=true` requires `limits`
- `ok=false` requires `error.kind` and `error.message`
- numeric fields must be number or null
- time fields must be RFC3339 string or null
- `confidence` must be one of `high`, `medium`, `low`
- `source` must be one of `remote`, `inferred`, `mixed`
- unknown top-level keys should be rejected to prevent silent drift

Validation failure is treated as a driver parse error and must not overwrite the previous snapshot.

## Account Configuration

Accounts need enough data to choose drivers without leaking script details into routing code.

Planned account additions:

- `usage_driver`
- `usage_config_json`
- optional `account_driver`

Examples:

- built-in official account:
  - `usage_driver = "builtin_openai_official"`
- built-in adapted provider:
  - `usage_driver = "builtin_ppchat"`
- Lua adapter:
  - `usage_driver = "lua"`
  - `usage_config_json = {"script":"third_party/vendor_x.lua","timeout_ms":5000,"base_path":"/billing/usage"}`

## Refresh Orchestration

Background refresh flow:

1. scheduler lists accounts
2. registry selects account driver
3. account driver resolves credentials
4. registry selects usage driver
5. usage driver fetches raw usage result
6. normalizer builds `usage.Snapshot`
7. repository saves the snapshot

Requirements:

- no live usage query inside request routing
- per-account timeout
- one account failure does not stop the batch
- preserve last known snapshot on failure
- expose stale/error metadata for UI and diagnostics

## Routing Behavior

Routing must remain snapshot-driven.

Priority of capacity interpretation:

1. official window model if `PrimaryResetsAt` / `SecondaryResetsAt` or used-percent fields are present
2. third-party remaining quota/rate model if `QuotaRemaining`, `RPMRemaining`, or `TPMRemaining` are present
3. balance-only mode if only `Balance` is present
4. manual/state-only mode if no reliable limits are available

This allows official and third-party quota schemes to coexist without branching routing behavior by provider family.

## Security Model For Lua

The runtime should enforce:

- execution timeout
- memory limit if supported by the selected Lua embedding
- no dynamic module import
- host-exposed API only
- immutable input context
- structured request/response logging with secrets redacted

The host should store scripts on disk inside the repository or a dedicated adapter directory, not inside account records.

## Migration Strategy

Implement incrementally:

1. introduce driver interfaces and registry with built-in wrappers around current logic
2. extend account and snapshot schema for driver metadata
3. move official and adapted providers to built-in usage drivers
4. add Lua runtime, schema validation, and one example third-party adapter
5. switch scheduler to orchestrated refresh
6. expose account configuration in the API/UI only after backend contracts are stable

## Testing Strategy

Cover at least:

- registry selection for built-in and Lua drivers
- strict Lua schema validation failures
- official limit normalization
- third-party quota normalization
- scheduler behavior when one driver fails
- stale snapshot preservation
- account repository persistence for new driver fields

## Non-Goals

- scripting the request forwarding path
- arbitrary plugin install/update workflow
- dynamic code download from remote sources
- provider-managed routing policies

## Outcome

This design keeps the critical forwarding path stable, supports both official and third-party quota schemes, and gives the system a controlled extension point for new usage integrations without turning the whole backend into a scripting host.
