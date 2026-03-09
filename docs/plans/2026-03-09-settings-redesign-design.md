# Settings Redesign Design

**Date:** 2026-03-09

**Goal:** Replace the current single-purpose settings screen with a full desktop settings center covering General, Proxy, Advanced, and About, while making the requested options real, persistent features.

## Context

The current settings page only manages Codex backup and restore. Requested behavior spans three layers:

- backend-persisted application settings
- desktop-shell behavior that must apply before the frontend loads
- proxy and failover behavior already partially implemented in backend routing

The redesign therefore needs coordinated changes in `backend`, `frontend`, and `desktop/src-tauri`.

## Chosen Approach

Use a unified settings model persisted in backend SQLite, with a small desktop startup cache for values that must be applied before the web UI mounts.

This keeps one durable source of truth while still allowing `silent_start`, `close_to_tray`, and custom backend listen address to take effect at app startup.

## Settings Model

Add durable application settings for:

- `launch_at_login`
- `silent_start`
- `close_to_tray`
- `show_proxy_switch_on_home`
- `proxy_host`
- `proxy_port`
- `auto_failover_enabled`
- `auto_backup_interval_hours`
- `backup_retention_count`

Add an explicit failover queue table keyed by existing `accounts.id` so the user can control ordered provider fallback independently from the current score-based routing.

## Persistence Design

### Backend

Add:

- `app_settings` table for singleton key/value settings
- `failover_queue_items` table for ordered account queue entries

Expose new API routes:

- `GET /settings/app`
- `PUT /settings/app`
- `GET /settings/failover-queue`
- `PUT /settings/failover-queue`
- SQL import/export endpoints
- database snapshot backup/list/restore endpoints

Default values are returned when no rows exist yet, so upgrades keep current behavior.

### Desktop Startup Cache

Add a small JSON file under `~/.aigate/data/desktop-settings.json` for values that must be readable before the backend sidecar and main window initialize:

- `listen_addr`
- `silent_start`
- `close_to_tray`
- `launch_at_login`

Backend SQLite remains the durable truth. The desktop cache is a startup mirror written whenever desktop-specific settings are applied.

## General Tab

### Window Behavior

- `开机自启`: implemented for macOS by creating/removing `~/Library/LaunchAgents/com.aigate.desktop.plist`
- `静默启动`: when enabled, app boots hidden and remains available through tray/Dock reopen
- `关闭是最小化到托盘`: controls whether clicking the window close button hides the app or allows a real exit sequence

Default `close_to_tray` stays enabled to preserve current behavior.

## Proxy Tab

### Local Proxy

- show current proxy state badge
- `在主界面显示代理开关`: controls the top-right proxy panel in the main shell
- `代理总开关`: reuses the existing enable/disable proxy APIs
- `基础设置`: real editable host/port for the local backend listener

### Listen Address Behavior

Current code hardcodes `127.0.0.1:6789` in both backend config and Tauri request plumbing. This must become dynamic.

Rules:

- only local addresses are allowed: `127.0.0.1`, `localhost`, `::1`
- changing host/port restarts the sidecar
- if proxy mode is currently enabled, `.codex/config.toml` proxy injection is rewritten to the new address and the session hash is refreshed

### Automatic Failover

Add:

- `自动故障转移开关`
- explicit ordered queue editor

Behavior when enabled:

- `/chat/completions` and `/responses` both try queue order first
- invalid, disabled, cooling-down, or capability-incompatible accounts are skipped
- retryable upstream failure classes move to the next queue item

Behavior when disabled:

- preserve current behavior: active account first, then score-based routing fallback

If enabled but queue is empty, the router falls back to default routing and the UI shows a warning instead of hard-failing.

## Advanced Tab

### Data Management

Implement SQL import/export for AI Gate-owned tables:

- `accounts`
- `account_usage_snapshots`
- `conversations`
- `messages`
- `runs`
- `app_settings`
- `failover_queue_items`

Import flow:

1. validate uploaded SQL
2. load into temporary database
3. verify required schema/tables
4. replace current data transactionally
5. rollback on any error

### Backup and Restore

Implement application database snapshot backup management distinct from the existing `.codex` safety backup flow.

Features:

- immediate snapshot backup
- auto-backup interval in hours
- retention count cleanup
- snapshot list with restore action
- pre-restore safety backup before applying a snapshot

Existing `.codex` backup logic remains in place internally for proxy enable/disable safety and is not removed.

## About Tab

Display:

- app icon from desktop bundle assets
- program name: `AI Gate`
- program version from desktop package metadata
- short app description
- author: `GcsSloop`

## UI Design

Follow the reference direction:

- wide rounded segmented tabs
- pale gray grouped cards
- one rounded setting row per action
- blue primary CTA buttons
- icon chips matching each section

Use `@ant-design/icons` already present in the repo instead of adding a new icon package.

## Migration and Compatibility

Default values:

- `launch_at_login = false`
- `silent_start = false`
- `close_to_tray = true`
- `show_proxy_switch_on_home = true`
- `proxy_host = "127.0.0.1"`
- `proxy_port = 6789`
- `auto_failover_enabled = false`
- `auto_backup_interval_hours = 24`
- `backup_retention_count = 10`

These defaults preserve current startup, tray, and proxy behavior for upgraded users.

## Validation Strategy

Backend:

- settings repository tests
- failover queue persistence tests
- failover selection tests for both gateway and responses paths
- SQL import/export tests
- database backup retention and restore tests

Frontend:

- settings tab rendering and switching
- save flows
- queue editing behavior
- top-level proxy switch visibility behavior

Desktop:

- startup cache read/write tests
- close-to-tray branch tests
- LaunchAgent creation/removal tests
- sidecar address change handling tests

Environment caveat discovered during setup:

- backend baseline tests pass
- desktop baseline tests pass once ignored local sidecar resources are linked into the worktree
- frontend tests require a newer Node runtime than the default `v14.16.1`; use local Node `v20.x` for verification
