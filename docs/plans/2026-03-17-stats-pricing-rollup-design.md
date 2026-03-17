# Stats Pricing Rollup Design

## Goal

Upgrade the stats system so token units use `K` and `M`, dashboard windows follow calendar-aligned ranges, pricing is configurable with account-over-type precedence, and historical usage data is sparsified over time.

## Scope

- Normalize token display to `K` and `M` in cards, charts, and recent-event rows.
- Change stats windows from rolling hours to calendar buckets:
  - `24h`: current day, 24 hourly buckets
  - `7d`: last 7 natural days including today
  - `30d`: last 30 natural days including today
- Return zero-filled trend buckets from the backend.
- Add pricing settings with fallback order:
  - account pricing
  - provider-type pricing
  - built-in model defaults
- Compute dashboard estimated cost from effective pricing rules at query time.
- Add historical rollup tables and retention logic:
  - recent raw events stay detailed
  - medium-range data rolls up to hourly buckets
  - older data rolls up to daily buckets

## Architecture

The backend remains the source of truth for usage aggregation. Dashboard endpoints will accept a range key instead of raw hour counts and will resolve the exact UTC time window plus bucket granularity. Trend responses will already include zero-filled buckets, so the frontend only binds data into charts without rebuilding page state.

Pricing rules will live in app settings and be resolved at read time. Raw usage events remain useful for short-range analysis, while a rollup job compacts older usage into aggregate tables. Dashboard queries will combine raw and rolled-up data so the UI sees one consistent stream.

## Data Model

### Settings

Add pricing settings into the app settings payload:

- `provider_pricing`: map keyed by provider type
- `account_pricing`: map keyed by account id

Each pricing rule contains:

- `input_per_million`
- `output_per_million`

### Usage Storage

Keep `usage_events` as the raw source for recent data and add rollup storage:

- `usage_rollups_hourly`
- `usage_rollups_daily`

Each row stores:

- bucket start
- account id
- provider type
- request kind
- model
- request count
- success count
- failure count
- input tokens
- output tokens
- total tokens
- balance delta
- quota delta

## Query Semantics

### Summary

Summaries will compute:

- request counts
- success and failure counts
- input, output, and total tokens
- estimated cost using effective pricing
- balance and quota deltas

### Trends

The backend will:

1. resolve the calendar-aligned window
2. read raw events and rollups as needed
3. aggregate into the requested bucket size
4. fill missing buckets with zeros

### Effective Pricing

Resolution order for each event or aggregate row:

1. explicit account pricing if configured
2. explicit provider-type pricing if configured
3. built-in model default

Account pricing applies regardless of model. Provider pricing is the fallback for all accounts of that source type.

## Frontend Behavior

- Keep the page shell mounted during filter changes.
- Preserve previous stats until the new request resolves.
- Show only local chart loading states if needed.
- Update ECharts series with transition animation instead of recreating chart instances.
- Use a shared formatter for compact token units in cards, axes, tooltips, and event rows.

## Retention and Sparsification

Use a backend compaction pass to reduce storage volume:

- keep raw events for the last 7 days
- roll up events older than 7 days into hourly buckets and delete those raw rows
- roll up events older than 30 days into daily buckets and delete the hourly rows they supersede

The compaction routine must be idempotent and safe to run repeatedly.

## Testing

- Backend tests for calendar window resolution and zero-filled buckets.
- Backend tests for pricing precedence and dynamic cost calculation.
- Backend tests for hourly and daily rollup compaction.
- Frontend tests for range switching behavior, compact unit formatting, and pricing settings UI.
- Frontend tests to ensure the stats page no longer replaces the full page with a loading spinner during filter changes.
