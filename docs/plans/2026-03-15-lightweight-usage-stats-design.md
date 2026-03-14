# Lightweight Usage Stats Design

## Goal

Keep the local database small by removing request-level audit persistence and replacing it with lightweight token, cost, and balance statistics that remain useful for daily operation.

## Current Problem

The database is dominated by `messages.raw_item_json` and related audit payloads in `conversations`, `messages`, and `runs`. Even after compaction, the retained payload volume stays too large for a lightweight desktop utility. The strong product requirement is token and cost visibility, not request replay or audit forensics.

## Product Direction

Adopt a usage-first storage model.

- Stop persisting request audit details for `/responses` and chat completions.
- Keep account records, app settings, failover queue, and sparse usage snapshots.
- Add lightweight per-request usage events for token and cost reporting.
- Remove audit storage controls from settings.
- Add a dedicated stats page for token and cost visibility.

## Approaches Considered

### 1. Keep audit data and compact it harder

Pros:
- Preserves detailed history.
- Smaller code change than a full model shift.

Cons:
- Still keeps the largest data category alive.
- Does not reliably hit the desired long-term size target.
- Continues product complexity around retention and manual optimization.

### 2. Keep only sampled audit payloads

Pros:
- Smaller than full audit.
- Some debugging history remains.

Cons:
- Still stores the wrong class of data for the current product.
- Sampling rules are hard to explain and hard to trust.
- Adds edge cases without solving the storage problem cleanly.

### 3. Remove audit persistence and keep only usage facts

Pros:
- Best fit for a local lightweight tool.
- Directly serves token, cost, and balance tracking.
- Simplifies backend, import/export, and settings.

Cons:
- Loses local request replay and deep forensic history.

Recommendation: option 3.

## Data Model

### Keep

- `accounts`
- `account_usage_snapshots`
- `app_settings`
- `failover_queue_items`

### Stop Using for Ongoing Persistence

- `conversations`
- `messages`
- `runs`

These tables can remain in schema for compatibility during rollout, but new request paths should stop writing to them. Existing rows should be cleared during one-time cleanup.

### Add

Create `usage_events` as the primary lightweight observability table.

Suggested fields:

- `id`
- `account_id`
- `provider_type`
- `request_kind`
- `model`
- `status`
- `input_tokens`
- `output_tokens`
- `total_tokens`
- `estimated_cost`
- `balance_before`
- `balance_after`
- `quota_before`
- `quota_after`
- `latency_ms`
- `created_at`

This stores facts, not payload bodies.

## Retention

`usage_events` should roll automatically.

- Keep recent detailed rows up to a fixed ceiling.
- When the ceiling is exceeded, aggregate older rows into time buckets and delete the original detailed rows.
- Aggregation should preserve totals needed for charts and summaries:
  - request count
  - success and failure count
  - input, output, and total tokens
  - estimated cost
  - balance delta
  - quota delta

`account_usage_snapshots` should remain sparse and continue to be pruned.

## Backend Behavior

### Request Execution

When a request finishes:

1. Determine account, model, status, token totals, latency, and estimated cost.
2. Read the latest usage snapshot when available.
3. Persist one `usage_event`.
4. Update or keep usage snapshots as the routing layer already requires.
5. Do not persist conversation, message, or run rows.

### Dashboard and Stats APIs

Replace conversation-based dashboard APIs with usage-stat APIs that support:

- summary cards
- trends over time
- status distribution
- per-account and per-model aggregation
- recent event list

### Migration and Cleanup

On startup:

1. Ensure `usage_events` exists.
2. Run a one-time cleanup marker check.
3. If not done, clear `conversations`, `messages`, and `runs`.
4. Vacuum the database after cleanup.
5. Mark cleanup complete.

This avoids repeated destructive work on every startup.

## Frontend Changes

### Navigation

Add a top-level stats page alongside accounts and settings.

### Stats Page

The page should present two accounting views together but clearly separated:

- estimated cost
- balance or quota change

Recommended layout:

- summary cards for requests, tokens, estimated cost, balance delta, quota delta
- line or area trend for tokens and cost
- status distribution chart
- recent event table
- filters for range, account, model

### Settings

Remove audit storage controls and manual optimize action. Keep only settings that still map to live product behavior.

## Error Handling

- If token data is missing from an upstream response, write a usage event with zero token values and keep status plus latency.
- If snapshot values are unavailable, leave balance and quota before/after null instead of inventing numbers.
- Cleanup failures must log clearly but must not block app startup.

## Testing

Backend tests should cover:

- schema creation for `usage_events`
- repository save and aggregate queries
- retention and bucket rollup
- request handlers writing usage events instead of audit rows
- startup cleanup removing old audit rows once

Frontend tests should cover:

- stats page rendering
- API contract integration
- filter behavior
- removal of audit controls from settings

## Non-Goals

- Local replay of prior requests
- Detailed transcript browsing
- Tool-call payload auditing
- Backfilling exact historical token costs from old raw payloads
