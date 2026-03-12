# Desktop Recent Logs Design

## Goal
Add a low-risk desktop-only recent log viewer for sidecar lifecycle and auto-recovery events without introducing persistent storage or unbounded frontend rendering.

## Scope
- Keep logs in desktop memory only
- Record sidecar `spawn`, `restart`, `shutdown`, auto-recovery attempts, and results
- Expose a Tauri command that returns the most recent bounded slice
- Add a settings card that renders only recent entries in a fixed-height scroll area

## Constraints
- No database writes
- No file log rotation
- Frontend must request a bounded count and render inside a scroll container
- Desktop must cap total retained entries with a ring-buffer style policy

## Recommended Limits
- Desktop in-memory cap: 200 entries
- UI fetch cap: 50 entries by default, hard-clamped in desktop command
- Fixed-height log panel with `overflow-y: auto`

## Data Shape
- `timestamp`
- `level`
- `category`
- `message`

## Triggered Events
- sidecar spawn start / success / failure
- sidecar restart start / success / failure
- sidecar shutdown
- backend request recovery attempt
- backend request recovery success / final failure
- macOS reopen recovery probe

## Risk
Low. Logs are isolated to desktop memory, bounded, and read-only from the frontend.
