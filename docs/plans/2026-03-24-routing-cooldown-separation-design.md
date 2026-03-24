# Routing Cooldown Separation Design

## Goal

Separate persistent account state from temporary routing cooldown so the UI does not report a third-party account as unavailable when the account still has quota and can still be used manually.

## Problem

The current implementation stores temporary routing avoidance in `accounts.status = cooldown`. The account list UI reads that persisted status directly, while usage and remaining quota are read from usage snapshots. This creates inconsistent cards such as "has remaining quota" but "cooling down".

It is worse for third-party providers such as PPChat because:
- usage refresh and request routing use different upstream signals
- `chat/completions` can still succeed even if `/responses` previously wrote cooldown
- scheduler recovery only waits for `cooldown_until`; it does not actively verify recovery

## Design

### Account state model

Keep `accounts.status` for durable account state only:
- `active`
- `invalid`
- `disabled`

Add routing cooldown fields to `accounts`:
- `routing_cooldown_until DATETIME NULL`
- `routing_cooldown_reason TEXT NOT NULL DEFAULT ''`

This preserves routing behavior without polluting the main account state.

### Routing behavior

For `/responses` thin routing:
- skip candidates when `routing_cooldown_until > now`
- write and clear only routing cooldown fields
- do not set `accounts.status = cooldown`

Cooldown write rules:
- official low-remaining preemptive cooldown remains supported
- hard quota and real rate-limit responses can set routing cooldown
- soft/network/upstream errors do not persist routing cooldown for third-party accounts

### Recovery behavior

Scheduler recovery should restore expired routing cooldowns by clearing the routing cooldown fields.

For third-party usage refresh:
- when refresh succeeds and the account has a healthy usage snapshot with positive remaining quota, clear expired or stale routing cooldown if present
- do not mutate `status`

### API and UI

Account list payload should expose:
- `status`
- `routing_cooldown_remaining_seconds`
- `routing_cooldown_reason`

UI rules:
- main badge uses durable `status`
- routing cooldown is a secondary badge or helper text
- usage meters continue to use snapshot data

This keeps the visual hierarchy consistent:
- account state first
- routing avoidance second
- quota/usage independent

## Testing

Add regression coverage for:
- `/responses` writes routing cooldown without mutating account `status`
- active PPChat account with routing cooldown still shows main status as active in list payload
- cooldown candidate skipping uses routing cooldown fields instead of `status`
- recovery job clears expired routing cooldown without changing active accounts incorrectly
- third-party soft errors do not persist routing cooldown

## Rollout

1. Add schema and repository support.
2. Update routing and scheduler logic.
3. Update API payloads and frontend rendering.
4. Run backend and frontend verification.
5. Release through the normal branch, PR, tag, and release workflow.
