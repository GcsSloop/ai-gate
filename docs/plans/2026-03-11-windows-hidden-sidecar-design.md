# Windows Hidden Sidecar Design

## Goal

Keep Windows desktop behavior aligned with macOS:

- launching AI Gate must not show a visible console window for the backend sidecar
- clicking the main window close button must still minimize to tray when `close_to_tray = true`
- only explicit app exit paths should terminate the backend sidecar

## Current State

The desktop shell already starts the backend as a sidecar from Tauri and already shuts it down on explicit application exit. The main process also uses `windows_subsystem = "windows"`, so the remaining Windows-specific leak is the sidecar spawn path itself.

## Recommended Approach

Use a Windows-only command extension on the existing `std::process::Command` in `spawn_sidecar()`.

- Preserve the current cross-platform lifecycle logic.
- Add a small Windows-only helper that applies `CREATE_NO_WINDOW` to the sidecar spawn command.
- Keep `shutdown_sidecar()` unchanged so explicit exit still kills the child process.
- Keep the existing `WindowCloseAction` behavior unchanged so closing the main window still minimizes to tray when configured.

## Why This Approach

This is the smallest safe change.

- It avoids refactoring the sidecar model.
- It keeps macOS behavior untouched.
- It localizes Windows-specific behavior to process creation only.
- It matches the current architectural split where Tauri owns lifecycle and the Go backend remains a child process.

## Risks

- If the sidecar needs a console for diagnostics during local Windows debugging, `CREATE_NO_WINDOW` hides it. That is acceptable for packaged desktop behavior.
- Windows-only code must compile cleanly on non-Windows targets, so the helper must be gated with `#[cfg(windows)]`.

## Validation

- Add a unit test proving the Windows helper sets the expected creation flags.
- Keep existing window-close tests green to ensure tray/exit semantics do not regress.
- Build the desktop app after the change to verify the Tauri bundle still compiles.
