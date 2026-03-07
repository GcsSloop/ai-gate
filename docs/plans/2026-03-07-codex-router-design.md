# Codex Router Design

## Goal

Build a self-hosted Codex router product with a web console and a background gateway service that can:

- authenticate official accounts through web-based authorization
- manage third-party OpenAI-compatible API endpoints with API keys
- keep conversations alive while switching accounts
- monitor usage, cooldown windows, and health across accounts
- automatically fail over to the next account when the current one cannot safely complete the next turn
- expose a unified OpenAI-compatible API that routes outbound traffic through the selected account

## Product Scope

The first release targets single-instance self-hosting. The architecture must still preserve clean boundaries for a later multi-user deployment without redesigning the routing core.

Supported platform scope for v1:

- OpenAI-compatible external API surface
- official account authorization via web login / OAuth-style flow
- third-party `base_url + api_key` accounts
- adapter boundaries reserved for future Anthropic and Gemini support

## Recommended Architecture

The product should be implemented as a modular monolith.

### Why this approach

- faster to ship than microservices
- easier to operate as a single binary backend plus a web console
- preserves clean module boundaries for adapters, scheduling, monitoring, and future multi-user isolation

### Top-level components

1. `Web Console`
   - account management
   - authorization entry points
   - routing policy configuration
   - usage dashboards
   - conversation and failover inspection

2. `Control API`
   - account CRUD
   - OAuth callback handling
   - policy management
   - monitoring and session query endpoints

3. `Gateway API`
   - OpenAI-compatible endpoints such as `/v1/chat/completions`, `/v1/responses`, `/v1/models`
   - request normalization
   - stream proxying
   - request tracing and failover execution

4. `Core Services`
   - `Account Registry`
   - `Auth Connectors`
   - `Routing Engine`
   - `Session Store`
   - `Usage Monitor`
   - `Scheduler`
   - `Provider Adapters`

## Core Design Principles

- Conversations belong to the router, not to any upstream account.
- Accounts are execution carriers that can be replaced mid-run.
- Management-plane APIs and data-plane APIs stay separate.
- OpenAI-compatible inbound APIs are the first compatibility target.
- Single-host deployment is the first operational target, but schemas and service boundaries must not block later multi-user support.

## Domain Model

### Provider

Defines the upstream family and capability set.

Suggested examples:

- `openai-official`
- `openai-compatible-third-party`
- reserved later: `anthropic`, `gemini`

Responsibilities:

- authentication type
- supported endpoint families
- quota/usage probing behavior
- adapter implementation selection

### Account

Unified representation for both official authorized accounts and third-party API credentials.

Suggested fields:

- `id`
- `provider_type`
- `account_name`
- `auth_mode` (`oauth`, `api_key`)
- `credential_ref`
- `base_url`
- `model_allowlist`
- `priority`
- `status`
- `last_health_score`
- `cooldown_until`
- `last_probe_at`

### AccountUsageSnapshot

Captures current usage and health state used by routing decisions.

Suggested fields:

- `account_id`
- `balance`
- `quota_remaining`
- `rpm_remaining`
- `tpm_remaining`
- `recent_error_rate`
- `avg_latency_ms`
- `rate_limited_until`
- `last_checked_at`

### RoutingPolicy

Configures selection and failover behavior.

Suggested fields:

- `id`
- `candidate_order`
- `minimum_balance_threshold`
- `minimum_quota_threshold`
- `token_budget_factor`
- `max_error_rate`
- `cooldown_seconds`
- `failover_mode`
- `model_pool_rules`

### Conversation

Router-owned conversation state independent of any one account.

Suggested fields:

- `id`
- `client_id`
- `target_provider_family`
- `default_model`
- `current_account_id`
- `state`
- `created_at`
- `updated_at`

### Message

Stores the canonical message history used for replay and continuation.

Suggested fields:

- `id`
- `conversation_id`
- `role`
- `content`
- `sequence_no`
- `created_at`

### Run

Represents one outbound execution attempt, including chained retries and stream continuation.

Suggested fields:

- `id`
- `conversation_id`
- `request_fingerprint`
- `selected_account_id`
- `fallback_from_run_id`
- `stream_offset`
- `status`
- `input_tokens`
- `output_tokens`
- `error_code`
- `started_at`
- `finished_at`

## Account State Machine

Accounts need recoverable lifecycle states, not just enabled or disabled.

### States

- `active`: eligible for routing
- `degraded`: still routable but with lower score
- `cooldown`: temporarily excluded until recovery criteria are met
- `invalid`: credentials expired or authorization revoked; manual intervention required
- `disabled`: manually disabled; never auto-restored

### Cooldown Types

1. Explicit platform cooldown
   - quota reset time
   - rate-limit reset time
   - provider-supplied recovery timestamp

2. Router-inferred cooldown
   - daily usage exhaustion
   - rolling-window exhaustion such as "used up within 5 hours"
   - repeated capacity failures or throttling patterns

### Auto-Recovery Rule

Cooldown expiry does not directly switch the account to `active`.

Recovery flow:

1. scheduler finds accounts with expired `cooldown_until`
2. account enters probe attempt
3. lightweight validation checks run
   - credential validity
   - quota / balance refresh
   - cheap provider health endpoint or model listing when possible
4. success moves account to `active`
5. failure refreshes cooldown timing and records probe error

## Routing Engine

### Request Handling Flow

1. receive inbound request through `Gateway API`
2. normalize request into internal execution format
3. estimate the token budget for this turn
4. load the matching policy and candidate account pool
5. filter out accounts that are not feasible:
   - invalid credentials
   - disabled state
   - cooldown state
   - insufficient balance or quota
   - excessive recent error rate
6. score the remaining accounts
7. execute against the best candidate
8. update snapshots, run records, and conversation state
9. fail over when needed according to failure classification

### Feasibility Check

The router should determine whether an account is likely to survive the next turn before sending the request.

Inputs:

- estimated prompt tokens
- recent average completion length
- configured safety multiplier
- current balance / quota
- remaining RPM / TPM

If the account cannot satisfy the projected turn with safety margin, it must be skipped before execution starts.

### Scoring Inputs

- configured priority
- resource safety margin
- recent success rate
- recent latency
- recent throttling frequency
- cost preference

## Failover and Stream Continuation

### Non-stream Requests

- retry the same account once only for transient errors
- immediately fail over on capacity failures and confirmed rate limits
- permanently exclude invalid credentials until re-authorization

### Stream Requests

The first release should support segmented continuation, not byte-perfect seamless continuation.

Flow:

1. start upstream stream and buffer emitted assistant text
2. on mid-stream failure:
   - stop the current upstream connection
   - persist partial output and run metadata
   - create a continuation request using the full conversation plus a continuation instruction
3. select the next feasible account
4. resume streaming new content to the client
5. record chained runs for observability

Expected limitation:

- output style may shift between accounts
- some repetition or paraphrasing may occur
- product promise should be "best-effort continuity" rather than exact upstream continuity

## Failure Classification

- `Hard Fail`
  - invalid credentials
  - forbidden model access
  - revoked authorization
  - action: mark `invalid` or exclude immediately

- `Soft Fail`
  - intermittent network errors
  - transient 5xx
  - action: one local retry, then fail over

- `Capacity Fail`
  - insufficient balance
  - insufficient quota
  - projected turn not feasible
  - action: place in cooldown or inferred cooldown window

- `Rate Limit Fail`
  - 429 or equivalent platform throttling
  - action: set `cooldown_until` from provider signal or local policy

## Web Console

### Account Management

- add official account through web authorization
- add third-party OpenAI-compatible endpoint with `base_url + api_key`
- inspect status, allowed models, priority, and cooldown countdown
- manually disable, re-enable, or trigger a probe

### Routing Policy

- configure candidate order
- define minimum balance and quota thresholds
- define token safety multiplier
- define error-rate thresholds
- define cooldown and recovery behavior
- assign model families to account pools

### Conversation Monitoring

- list active and recent conversations
- inspect which account served each run
- inspect failover chains and continuation events

### Usage Monitoring

- platform-level request count
- token usage by account
- success rate
- latency
- cooldown state
- next recovery probe time

### System Settings

- listen address
- callback URLs
- encryption configuration
- scheduler intervals
- retention windows

## Background Services

The backend process should run three continuous responsibilities:

1. `HTTP Server`
   - control-plane APIs
   - gateway APIs
   - static frontend serving in production

2. `Scheduler`
   - usage refresh
   - cooldown auto-recovery
   - OAuth token refresh
   - stale data cleanup

3. `Router Workers`
   - outbound request execution
   - stream proxying
   - failover and continuation
   - run tracing

## Security Requirements

- encrypt all stored credentials
- never log plaintext tokens or API keys
- separate management auth from gateway auth
- audit all account lifecycle changes
- auto-recovery applies only to `cooldown`, never to `disabled` or `invalid`

## Testing Strategy

### Unit Tests

- routing score calculation
- feasibility checks
- cooldown transitions
- recovery probe behavior
- run state transitions

### Adapter Tests

- OpenAI-compatible request translation
- OAuth credential refresh
- API key authentication behavior

### Integration Tests

- normal request routing
- automatic failover on insufficient quota
- cooldown on throttling
- scheduler-driven recovery probe
- stream continuation after mid-stream failure

### End-to-End Tests

- add accounts from the web console
- complete official authorization flow
- configure routing policy
- send traffic through gateway
- verify dashboard and run traces

## Suggested Technical Stack

- Backend: Go
- Frontend: React + Vite
- Storage: SQLite for v1, schema designed to migrate later to PostgreSQL
- Secret encryption: application-level encryption with master key from environment or local config

## Delivery Strategy

### M1

- repository bootstrap
- backend skeleton
- frontend console skeleton
- account registry
- manual account selection
- OpenAI-compatible gateway
- basic usage dashboard

### M2

- routing policy engine
- automatic failover
- cooldown lifecycle
- scheduler-driven auto-recovery
- richer monitoring

### M3

- stream continuation
- run chain inspection
- audit trails
- expanded health monitoring

## Non-goals for v1

- full microservice deployment
- complete multi-user RBAC
- Anthropic/Gemini live adapters
- byte-perfect cross-account stream continuation
