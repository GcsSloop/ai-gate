# Update Download Backend Design

**Goal:** Move desktop update download task ownership out of the frontend component lifecycle and into the Tauri host so page refreshes do not lose the active download, progress stays queryable, duplicate downloads are prevented, and the user can cancel an in-flight download.

## Context

The current update flow is split across `frontend/src/features/updates/updateService.ts`, `UpdateCard.tsx`, and `HomeUpdatePanel.tsx`. The frontend calls `@tauri-apps/plugin-updater` directly, keeps the active `downloadAndInstall` closure in JS memory, and updates progress with local React state. That creates three problems:

- Refreshing the page destroys the in-memory download task and its progress state.
- Multiple views can attempt to start the same update independently.
- There is no authoritative owner for cancellation or task deduplication.

The actual download/install capability already lives in the desktop shell via Tauri. The correct place to centralize task ownership is therefore the Rust host layer, not the Go router backend.

## Approaches

### Option A: Tauri host manages download state and lifecycle

Add a small update task manager in `desktop/src-tauri/src/main.rs` backed by global process memory. Expose Tauri commands for:

- fetching the current update task snapshot
- checking for the latest update
- starting or resuming a download/install task
- cancelling the active task
- relaunching after install

The frontend becomes a thin client that polls the authoritative task snapshot and renders it in both update surfaces.

**Pros**

- Matches where the updater capability already exists.
- Survives frontend refresh without pretending to support cross-process recovery.
- Prevents duplicate downloads with one in-memory task owner.
- Keeps cancellation local to the component that actually owns the task.

**Cons**

- Requires a Rust-side state machine and background worker wiring.

### Option B: Go backend stores update progress, frontend still triggers Tauri updater

Persist progress into the Go backend and let the frontend restore UI state after refresh.

**Pros**

- Reuses existing HTTP API patterns.

**Cons**

- Does not solve the real problem because the actual updater task still dies with the frontend runtime.
- Adds an unnecessary bridge between Go and Tauri.
- Makes cancellation and deduplication harder.

## Decision

Use **Option A**.

The Tauri host will be the single authority for update task state during the desktop process lifetime. The frontend will query and render that state. No cross-process persistence will be added in this task.

## Architecture

### Rust host state

Add a global `UpdateManagerState` guarded by `Mutex`, containing:

- `current_version`
- `latest_update` metadata when known
- `task_state`
- `progress`
- `error`
- `cancel_requested`
- `active_version`

Represent state with a small enum:

- `idle`
- `checking`
- `up_to_date`
- `available`
- `downloading`
- `ready`
- `unsupported`
- `cancelled`
- `error`

The state must be serializable so the frontend can query it directly.

### Tauri commands

Add commands:

- `get_update_state`
- `check_for_app_update`
- `start_update_download`
- `cancel_update_download`
- `relaunch_after_update`

Rules:

- `start_update_download` returns the current task snapshot.
- If the same target version is already downloading, it must not start a second task.
- If a different version is requested while another task is active, return a clear error.
- `cancel_update_download` flips a cancellation flag; the background task stops at the next progress callback boundary and transitions to `cancelled`.

### Background task model

The host starts one background thread per accepted download request. The thread:

1. Loads or revalidates the pending update.
2. Calls `downloadAndInstall`.
3. Updates shared state on `Started`, `Progress`, `Finished`.
4. Checks the cancellation flag during progress callbacks and aborts cleanly.

Only one active download/install task is allowed at a time.

### Frontend model

Replace direct updater calls in `frontend/src/features/updates/updateService.ts` with a Tauri-command-backed service:

- `getState()`
- `check()`
- `downloadAndInstall()`
- `cancelDownload()`
- `relaunch()`

`UpdateCard` and `HomeUpdatePanel` stop owning progress. They only:

- hydrate from `getState()`
- invoke `check()` or `downloadAndInstall()`
- poll while state is `checking` or `downloading`
- render `取消下载` during active download

This keeps both views consistent after refresh and prevents separate local state machines from diverging.

## Error Handling

- Non-desktop environment: return `unsupported` without crashing.
- Duplicate start: return current `downloading` snapshot.
- Cancel after finish: no-op and return current state.
- Cancel during install completion race: final terminal state wins.
- Check failure or download failure: transition to `error` with a user-facing message.

## Testing

### Frontend

Add service-level tests for:

- hydrating an existing `downloading` state after refresh
- deduplicated `start_update_download`
- cancellation support

Add component tests for:

- rendering a backend-owned download already in progress on first paint
- showing `取消下载`
- preserving progress after remount

### Rust

Add focused unit tests around the update state reducer/helpers:

- progress math
- duplicate task rejection/reuse
- cancellation transition
- terminal state transition

Full updater integration does not need to run in tests; state transitions can be verified with isolated helpers.

## Success Criteria

- Refreshing the frontend during an update still shows the active task and current progress.
- Starting download from one surface is reflected in the other surface.
- Re-clicking download does not create a second task.
- User can cancel an active download from the UI.
- No cross-process recovery is implied or implemented.
