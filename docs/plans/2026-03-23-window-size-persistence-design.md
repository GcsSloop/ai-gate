# Window Size Persistence Design

**Goal:** Desktop app launches at the minimum supported window size by default, and only switches to a remembered size after the user has manually resized the window.

## Decision

- Use the existing desktop settings cache file to persist the main window size.
- Change the built-in default window size to the current minimum size: `1024x700`.
- Keep persisted window size separate from ordinary app settings, but preserve it when app settings are saved.
- Restore the remembered size during desktop startup before normal window interaction.
- Persist logical window size on resize so the remembered size remains stable across DPI changes.

## Why this approach

- It matches the requested behavior exactly: minimum by default, remembered only after user adjustment.
- It reuses the existing desktop persistence path instead of introducing a second storage location.
- Persisting logical size avoids cross-monitor DPI surprises better than raw physical pixels.
- Preserving the saved size during `apply_app_settings` avoids an easy regression where opening Settings would silently reset the remembered window size.
