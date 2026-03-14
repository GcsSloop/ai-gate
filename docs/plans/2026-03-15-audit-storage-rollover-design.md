# Audit Storage Rollover Design

## Goal
Reduce SQLite growth caused by official-mode thin audit records while preserving enough metadata for debugging, dashboard statistics, and basic history review.

## Root Cause
Database backups are full SQLite snapshots. The live database is dominated by the `messages` table, especially `raw_item_json` for `function_call_output`, `message`, `reasoning`, `function_call`, and `custom_tool_call` items. There is currently no retention or compaction policy for those records.

## Decision
Implement type-aware audit rollover with three layers:

1. Ingest-time compaction
- New audit records are normalized before insert.
- Heavy item types keep only summary fields once the configured per-type limit is exceeded.

2. Silent historical optimization
- On startup, run a bounded compaction pass against existing records.
- Convert oversized historical rows into summaries and reclaim space.

3. Configurable retention ceilings
- Add app settings for per-type raw retention limits.
- Preserve lightweight metadata after rollover instead of deleting rows outright.

## Data Strategy
### `message`
- Keep `content` as a short preview.
- Drop full `raw_item_json` after rollover.
- Preserve role, item type, original lengths, hash, and request-level token summary.

### `function_call` / `custom_tool_call`
- Preserve tool name, call ID, short argument preview, argument hash, lengths, and request-level token summary.
- Drop full payload after rollover.

### `function_call_output` / `custom_tool_call_output`
- Preserve short output preview, hash, lengths, tool correlation fields, and request-level token summary.
- Drop full payload after rollover.

### `reasoning`
- Preserve only compact metadata and preview.
- Drop full reasoning body.

## Schema Changes
Add compact-summary columns to `messages` so rolled-over rows remain queryable without large blobs.

Candidate fields:
- `content_preview`
- `content_sha256`
- `content_bytes`
- `raw_preview`
- `raw_sha256`
- `raw_bytes`
- `tool_name`
- `tool_call_id`
- `summary_json`
- `storage_mode` (`full` or `summary`)

Add app settings for per-type raw retention ceilings.

## Runtime Flow
1. Request completes and usage snapshot is collected.
2. Audit message is normalized into a `MessageRecord`.
3. Repository decides whether this item type is still under raw-retention limit.
4. If yes, store full record.
5. If not, store summary form only.
6. Background optimizer compacts historical rows that exceed policy.

## Historical Optimization
- Run once on startup and on settings save when thresholds shrink.
- Work in batches to avoid blocking requests.
- Compact oldest rows first per item type.
- Finish with `VACUUM` only when compaction changed data and no write transaction is open.

## UI Changes
Add an `审计存储` section in settings:
- show current strategy
- allow tuning per-type raw retention limits
- expose a manual `立即优化` action
- describe that older rows are summarized instead of fully deleted

## Verification
- Unit tests for summarization rules by item type.
- Repository tests for rollover thresholds.
- Startup optimization test against seeded SQLite data.
- Settings API tests for new fields.
