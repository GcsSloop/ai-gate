# Unified Tabs And Backup Delete Design

**Context**

The desktop frontend currently uses one visual language for the home view switcher and another for the settings sub-tabs. The settings page also keeps its own top structure, which makes the title, tabs, save action, and scroll behavior feel disconnected. The backup section still places the manual backup action below the card header and does not support deleting a backup.

**Decision**

Adopt one navigation language across the shell and the settings page.

- Keep three top-level views: accounts, stats, settings.
- Render them as text tabs inside the fixed top header.
- Keep the header fixed and let only the content region scroll.
- Keep settings sub-tabs, but restyle them to match the home tabs.
- Move the settings save button onto the same row as the settings sub-tabs.
- Move `界面偏好` ahead of `窗口行为` in the `通用` tab.
- Move `立即备份` into the backup card header actions.
- Add per-backup delete support with confirmation.

**Approach**

Frontend layout changes stay inside the existing shell.

- `App.tsx` will keep the global top header and use it for all three top-level views.
- The settings screen will stop rendering its own separate top page chrome and will instead render only its sub-tab toolbar plus scrollable content sections.
- Shared tab visuals will be driven by a common CSS pill-tab style, reused by both the home switcher and the settings sub-tabs.

Backup deletion requires a backend capability.

- Add a delete-backup API endpoint in the settings handler.
- Reuse the existing backup repository path logic so deletion stays consistent with restore and list operations.
- Add frontend API wiring and a delete button in each backup row.

**Why this design**

This is the smallest change that satisfies the requested layout without rewriting page routing or introducing a new layout system. It preserves the current data flow, keeps settings sub-tabs separate from global views, and contains the backend addition to one new backup-management action.

**Validation**

- Frontend tests: top-level tab behavior, settings tab routing, backup actions layout hooks, delete backup action flow.
- Backend tests: delete backup success, not-found handling, and list refresh behavior.
- Regression: backend `go test ./...`, frontend `npm test -- --run`, frontend `npm run build`.
