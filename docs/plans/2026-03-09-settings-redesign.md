# Settings Redesign Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a real, persistent desktop settings center covering General, Proxy, Advanced, and About, including startup behavior, proxy host/port, explicit failover queue, SQL import/export, and database backup/restore.

**Architecture:** Backend SQLite becomes the durable source of truth for app settings and explicit failover queue ordering. Desktop startup-sensitive values are mirrored into a tiny local cache so Tauri can apply them before the frontend mounts and before the sidecar starts. Frontend replaces the current backup-only screen with a tabbed settings experience that drives the new APIs and desktop commands.

**Tech Stack:** Go HTTP handlers + SQLite, Tauri 2 / Rust desktop shell, React 19 + Ant Design 6 + Vitest.

---

### Task 1: Add persistent settings and failover queue storage

**Files:**
- Modify: `backend/internal/store/sqlite/migrations.go`
- Create: `backend/internal/settings/repository.go`
- Create: `backend/internal/settings/repository_test.go`
- Modify: `backend/internal/bootstrap/bootstrap.go`

**Step 1: Write the failing storage test**

Create `backend/internal/settings/repository_test.go` with coverage for:

- default settings returned when storage is empty
- saving settings and reading them back
- saving ordered failover queue entries and reading them back in stable order

Use a temp SQLite DB from `backend/internal/store/sqlite`.

**Step 2: Run test to verify it fails**

Run:

```bash
cd /Users/gcssloop/WorkSpace/AIGC/codex-router/.worktrees/settings-redesign/backend && go test ./internal/settings -run TestRepository -count=1
```

Expected: FAIL because the `settings` package and tables do not exist yet.

**Step 3: Add schema**

Extend `backend/internal/store/sqlite/migrations.go` with:

- `app_settings`
- `failover_queue_items`

Keep schema additive and migration-safe.

**Step 4: Write minimal repository**

Create `backend/internal/settings/repository.go` implementing:

- `GetAppSettings()`
- `SaveAppSettings(AppSettings)`
- `ListFailoverQueue()`
- `SaveFailoverQueue([]int64)`

Default values:

```go
AppSettings{
    LaunchAtLogin:         false,
    SilentStart:           false,
    CloseToTray:           true,
    ShowProxySwitchOnHome: true,
    ProxyHost:             "127.0.0.1",
    ProxyPort:             6789,
    AutoFailoverEnabled:   false,
    AutoBackupIntervalHours: 24,
    BackupRetentionCount:  10,
}
```

**Step 5: Run the storage test to verify it passes**

Run:

```bash
cd /Users/gcssloop/WorkSpace/AIGC/codex-router/.worktrees/settings-redesign/backend && go test ./internal/settings -run TestRepository -count=1
```

Expected: PASS.

**Step 6: Wire repository into bootstrap**

Instantiate the repository in `backend/internal/bootstrap/bootstrap.go` so later handlers can use it.

**Step 7: Commit**

```bash
cd /Users/gcssloop/WorkSpace/AIGC/codex-router/.worktrees/settings-redesign && git add backend/internal/store/sqlite/migrations.go backend/internal/settings/repository.go backend/internal/settings/repository_test.go backend/internal/bootstrap/bootstrap.go && git commit -m "feat: add persistent app settings storage"
```

### Task 2: Expose settings, queue, and proxy address APIs

**Files:**
- Modify: `backend/internal/api/settings_handler.go`
- Modify: `backend/internal/api/settings_handler_test.go`
- Modify: `backend/internal/bootstrap/bootstrap.go`
- Modify: `frontend/src/lib/api.ts`

**Step 1: Write the failing handler tests**

Extend `backend/internal/api/settings_handler_test.go` for:

- `GET /settings/app`
- `PUT /settings/app`
- `GET /settings/failover-queue`
- `PUT /settings/failover-queue`
- proxy status response includes current host/port

**Step 2: Run the handler tests to verify failure**

Run:

```bash
cd /Users/gcssloop/WorkSpace/AIGC/codex-router/.worktrees/settings-redesign/backend && go test ./internal/api -run TestSettingsHandler -count=1
```

Expected: FAIL because routes and payloads are incomplete.

**Step 3: Add handler request/response types**

In `backend/internal/api/settings_handler.go`, add JSON contracts for:

- app settings payload
- failover queue payload
- backup summary payload

**Step 4: Implement new routes**

Add route handling for:

- `GET /settings/app`
- `PUT /settings/app`
- `GET /settings/failover-queue`
- `PUT /settings/failover-queue`

Reuse the existing settings handler instead of creating another handler class.

**Step 5: Extend frontend API client**

Add typed functions in `frontend/src/lib/api.ts`:

- `getAppSettings`
- `saveAppSettings`
- `getFailoverQueue`
- `saveFailoverQueue`

Do not add frontend UI yet.

**Step 6: Run handler tests to verify they pass**

Run:

```bash
cd /Users/gcssloop/WorkSpace/AIGC/codex-router/.worktrees/settings-redesign/backend && go test ./internal/api -run TestSettingsHandler -count=1
```

Expected: PASS.

**Step 7: Commit**

```bash
cd /Users/gcssloop/WorkSpace/AIGC/codex-router/.worktrees/settings-redesign && git add backend/internal/api/settings_handler.go backend/internal/api/settings_handler_test.go backend/internal/bootstrap/bootstrap.go frontend/src/lib/api.ts && git commit -m "feat: add settings and failover queue APIs"
```

### Task 3: Add database snapshot backup and SQL import/export

**Files:**
- Modify: `backend/internal/api/settings_handler.go`
- Modify: `backend/internal/api/settings_handler_test.go`
- Create: `backend/internal/settings/sql_transfer.go`
- Create: `backend/internal/settings/sql_transfer_test.go`
- Create: `backend/internal/settings/db_backup.go`
- Create: `backend/internal/settings/db_backup_test.go`

**Step 1: Write the failing data-management tests**

Cover:

- SQL export returns application tables
- SQL import replaces data transactionally
- DB snapshot backup creates files
- retention cleanup deletes oldest files
- restore creates a pre-restore snapshot first

**Step 2: Run tests to verify failure**

Run:

```bash
cd /Users/gcssloop/WorkSpace/AIGC/codex-router/.worktrees/settings-redesign/backend && go test ./internal/settings -run 'Test(SQLTransfer|DBBackup)' -count=1
```

Expected: FAIL because import/export and DB backup helpers do not exist.

**Step 3: Implement minimal SQL export/import**

Create `backend/internal/settings/sql_transfer.go` with helper functions that:

- export schema + data for AI Gate-owned tables
- import into a temp database
- replace current table contents in a transaction

**Step 4: Implement DB snapshot backup helpers**

Create `backend/internal/settings/db_backup.go` with functions to:

- create snapshot copy
- list snapshots
- restore snapshot
- prune old snapshots by retention count

**Step 5: Connect helpers to settings handler**

Add routes for:

- SQL import
- SQL export
- DB backup list/create/restore/delete if needed by UI

Keep existing `.codex` backup endpoints intact because they still protect proxy toggling.

**Step 6: Run focused tests to verify pass**

Run:

```bash
cd /Users/gcssloop/WorkSpace/AIGC/codex-router/.worktrees/settings-redesign/backend && go test ./internal/settings -run 'Test(SQLTransfer|DBBackup)' -count=1
cd /Users/gcssloop/WorkSpace/AIGC/codex-router/.worktrees/settings-redesign/backend && go test ./internal/api -run TestSettingsHandler -count=1
```

Expected: PASS.

**Step 7: Commit**

```bash
cd /Users/gcssloop/WorkSpace/AIGC/codex-router/.worktrees/settings-redesign && git add backend/internal/settings/sql_transfer.go backend/internal/settings/sql_transfer_test.go backend/internal/settings/db_backup.go backend/internal/settings/db_backup_test.go backend/internal/api/settings_handler.go backend/internal/api/settings_handler_test.go && git commit -m "feat: add settings data import export and db backups"
```

### Task 4: Make failover queue affect gateway and responses routing

**Files:**
- Modify: `backend/internal/api/gateway_handler.go`
- Modify: `backend/internal/api/gateway_handler_test.go`
- Modify: `backend/internal/api/responses_handler.go`
- Modify: `backend/internal/api/responses_handler_test.go`
- Create: `backend/internal/settings/failover.go`
- Create: `backend/internal/settings/failover_test.go`
- Modify: `backend/internal/streaming/proxy.go`
- Modify: `backend/internal/routing/executor.go`

**Step 1: Write the failing failover tests**

Add tests proving:

- when auto failover is enabled, queue order wins
- disabled/cooldown/incompatible accounts are skipped
- `/chat/completions` and `/responses` behave consistently
- retryable classes continue to the next queued account

**Step 2: Run tests to verify failure**

Run:

```bash
cd /Users/gcssloop/WorkSpace/AIGC/codex-router/.worktrees/settings-redesign/backend && go test ./internal/api -run 'Test(GatewayHandler|ResponsesHandler)' -count=1
```

Expected: FAIL because handlers still use current active/score-only selection.

**Step 3: Implement queue-aware candidate ordering**

Create `backend/internal/settings/failover.go` to:

- filter queue entries against current accounts
- map queue order to candidates
- fall back to existing active/score selection when disabled or queue empty

**Step 4: Wire queue order into gateway and responses**

Update:

- `backend/internal/api/gateway_handler.go`
- `backend/internal/api/responses_handler.go`
- `backend/internal/routing/executor.go`
- `backend/internal/streaming/proxy.go`

Keep one shared definition of retryable failure classes.

**Step 5: Run focused tests to verify pass**

Run:

```bash
cd /Users/gcssloop/WorkSpace/AIGC/codex-router/.worktrees/settings-redesign/backend && go test ./internal/api -run 'Test(GatewayHandler|ResponsesHandler)' -count=1
cd /Users/gcssloop/WorkSpace/AIGC/codex-router/.worktrees/settings-redesign/backend && go test ./internal/settings -run TestFailover -count=1
```

Expected: PASS.

**Step 6: Commit**

```bash
cd /Users/gcssloop/WorkSpace/AIGC/codex-router/.worktrees/settings-redesign && git add backend/internal/api/gateway_handler.go backend/internal/api/gateway_handler_test.go backend/internal/api/responses_handler.go backend/internal/api/responses_handler_test.go backend/internal/settings/failover.go backend/internal/settings/failover_test.go backend/internal/streaming/proxy.go backend/internal/routing/executor.go && git commit -m "feat: apply explicit failover queue to routing"
```

### Task 5: Add desktop startup cache, launch-at-login, and dynamic backend address

**Files:**
- Modify: `desktop/src-tauri/src/main.rs`
- Modify: `desktop/src-tauri/Cargo.toml`
- Modify: `desktop/src-tauri/tauri.conf.json` only if metadata access needs it
- Create: `desktop/src-tauri/src/desktop_settings.rs` if extraction is needed
- Create or modify tests in `desktop/src-tauri/src/main.rs`

**Step 1: Write the failing desktop tests**

Add tests for:

- reading default startup settings
- reading cached startup settings
- `close_to_tray` true hides window on close
- `close_to_tray` false allows exit path
- LaunchAgent plist content generation
- backend address formatting from host/port

**Step 2: Run test to verify failure**

Run:

```bash
cd /Users/gcssloop/WorkSpace/AIGC/codex-router/.worktrees/settings-redesign && cargo test --manifest-path desktop/src-tauri/Cargo.toml
```

Expected: FAIL because no startup cache or launch-at-login helper exists yet.

**Step 3: Implement startup cache and desktop commands**

Add commands to:

- apply desktop settings
- return app metadata for About
- restart sidecar when backend address changes

Use `~/.aigate/data/desktop-settings.json` as startup cache.

**Step 4: Implement launch-at-login**

For macOS, create/remove `~/Library/LaunchAgents/com.aigate.desktop.plist` with current app executable path and silent-start env wiring as needed.

**Step 5: Replace hardcoded backend address**

Remove `BACKEND_ADDR` constant usage from runtime request helpers and sidecar startup. Read the current listen address from startup settings.

**Step 6: Run desktop tests to verify pass**

Run:

```bash
cd /Users/gcssloop/WorkSpace/AIGC/codex-router/.worktrees/settings-redesign && cargo test --manifest-path desktop/src-tauri/Cargo.toml
```

Expected: PASS.

**Step 7: Commit**

```bash
cd /Users/gcssloop/WorkSpace/AIGC/codex-router/.worktrees/settings-redesign && git add desktop/src-tauri/src/main.rs desktop/src-tauri/src/desktop_settings.rs desktop/src-tauri/Cargo.toml desktop/src-tauri/tauri.conf.json && git commit -m "feat: persist desktop startup settings"
```

### Task 6: Redesign frontend settings page and app shell integration

**Files:**
- Modify: `frontend/src/features/settings/SettingsPage.tsx`
- Modify: `frontend/src/features/settings/SettingsPage.test.tsx`
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/App.test.tsx`
- Modify: `frontend/src/styles.css`
- Modify: `frontend/src/lib/api.ts`
- Modify: `frontend/src/lib/desktop-shell.ts`

**Step 1: Write the failing frontend tests**

Update tests for:

- rendering `通用 / 代理 / 高级 / 关于`
- showing real settings values loaded from API
- saving toggles and host/port
- failover queue rendering and saving
- advanced SQL / backup actions
- about card metadata
- hiding the main-shell proxy switch when `show_proxy_switch_on_home` is false

**Step 2: Run test to verify failure**

Run with newer Node:

```bash
PATH=/Users/gcssloop/.nvm/versions/node/v20.19.6/bin:$PATH npm --prefix /Users/gcssloop/WorkSpace/AIGC/codex-router/.worktrees/settings-redesign/frontend test
```

Expected: FAIL because the page still renders the old backup-only layout.

**Step 3: Implement minimal frontend API/state integration**

Load:

- app settings
- proxy status
- failover queue
- DB backups
- about metadata

Save settings through the new backend APIs and desktop commands.

**Step 4: Implement visual redesign**

Rebuild `SettingsPage.tsx` into:

- segmented tabs
- grouped section cards
- row-style toggles and inputs
- queue editor
- SQL import/export controls
- backup list
- about card

Keep the style direction close to the provided reference images using the existing blue/gray palette.

**Step 5: Integrate app-shell proxy visibility**

Update `frontend/src/App.tsx` so the top-right proxy panel respects `show_proxy_switch_on_home`.

**Step 6: Run focused frontend tests to verify pass**

Run:

```bash
PATH=/Users/gcssloop/.nvm/versions/node/v20.19.6/bin:$PATH npm --prefix /Users/gcssloop/WorkSpace/AIGC/codex-router/.worktrees/settings-redesign/frontend test
```

Expected: PASS.

**Step 7: Commit**

```bash
cd /Users/gcssloop/WorkSpace/AIGC/codex-router/.worktrees/settings-redesign && git add frontend/src/features/settings/SettingsPage.tsx frontend/src/features/settings/SettingsPage.test.tsx frontend/src/App.tsx frontend/src/App.test.tsx frontend/src/styles.css frontend/src/lib/api.ts frontend/src/lib/desktop-shell.ts && git commit -m "feat: redesign settings screen"
```

### Task 7: Add scheduler-driven auto backup and final end-to-end verification

**Files:**
- Modify: `backend/internal/bootstrap/bootstrap.go`
- Modify: `backend/internal/scheduler/scheduler.go` or create a sibling job file
- Create: `backend/internal/scheduler/db_backup_job.go`
- Create: `backend/internal/scheduler/db_backup_job_test.go`
- Modify: `docs/plans/2026-03-09-settings-redesign-design.md` only if implementation materially diverges

**Step 1: Write the failing scheduler tests**

Cover:

- auto-backup runs when interval has elapsed
- no backup runs before interval elapses
- retention pruning is applied after backup creation

**Step 2: Run tests to verify failure**

Run:

```bash
cd /Users/gcssloop/WorkSpace/AIGC/codex-router/.worktrees/settings-redesign/backend && go test ./internal/scheduler -run TestDBBackupJob -count=1
```

Expected: FAIL because no database backup job exists.

**Step 3: Implement the backup job**

Create `backend/internal/scheduler/db_backup_job.go` and wire it from `backend/internal/bootstrap/bootstrap.go`.

Use app settings values for:

- interval hours
- retention count

**Step 4: Run scheduler tests to verify pass**

Run:

```bash
cd /Users/gcssloop/WorkSpace/AIGC/codex-router/.worktrees/settings-redesign/backend && go test ./internal/scheduler -run TestDBBackupJob -count=1
```

Expected: PASS.

**Step 5: Run full verification**

Run:

```bash
PATH=/Users/gcssloop/.nvm/versions/node/v20.19.6/bin:$PATH npm --prefix /Users/gcssloop/WorkSpace/AIGC/codex-router/.worktrees/settings-redesign/frontend test
cd /Users/gcssloop/WorkSpace/AIGC/codex-router/.worktrees/settings-redesign/backend && go test ./...
cd /Users/gcssloop/WorkSpace/AIGC/codex-router/.worktrees/settings-redesign && cargo test --manifest-path desktop/src-tauri/Cargo.toml
```

Expected: all passing.

**Step 6: Commit**

```bash
cd /Users/gcssloop/WorkSpace/AIGC/codex-router/.worktrees/settings-redesign && git add backend/internal/bootstrap/bootstrap.go backend/internal/scheduler/db_backup_job.go backend/internal/scheduler/db_backup_job_test.go && git commit -m "feat: schedule automatic database backups"
```

