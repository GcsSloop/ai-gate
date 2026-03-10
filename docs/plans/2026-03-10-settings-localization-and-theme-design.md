# Settings Localization And Theme Design

## Context

AI Gate currently stores desktop settings in `app_settings` and renders nearly all copy in Chinese. Theme is hardcoded to a light Ant Design token set. The user needs two independent capabilities:

1. Runtime language switching between Simplified Chinese and English, defaulting to Chinese.
2. Runtime theme switching between light, dark, and follow-system, defaulting to follow-system.

Both controls must live in Settings, apply immediately, and persist automatically without requiring the existing manual save action.

## Approach

### Option A: Persist in existing `app_settings` and drive runtime from app state

- Add `language` and `theme_mode` fields to backend and frontend `AppSettings`.
- Extend SQLite migration-on-open logic with additive columns, preserving existing databases.
- Add a small frontend i18n layer with a typed dictionary and runtime translator.
- Lift language/theme application into `App.tsx`, then pass translator/context into the active pages.
- Add dedicated immediate-save handlers for language/theme in `SettingsPage`.

Pros:
- One source of truth across desktop restarts.
- Immediate effect matches the current bootstrap/settings pipeline.
- Minimal operational complexity and no duplicate local storage path.

Cons:
- Requires touching backend, API types, and multiple frontend screens.

### Option B: Store in browser local storage only

- Keep backend untouched and store language/theme in the renderer only.

Pros:
- Less code.

Cons:
- Diverges from existing settings model.
- Weak desktop persistence guarantees.
- Harder to keep the desktop shell and bootstrap behavior coherent.

### Recommendation

Use Option A. It fits the existing product architecture and is the least surprising path for restart persistence and immediate UI updates.

## Behavior Design

### Language

- New setting key: `language`, values `zh-CN` and `en-US`.
- Default: `zh-CN`.
- Settings control: segmented/radio style selector under a new appearance/preferences card in Settings.
- On change:
  - persist immediately through `PUT /settings/app`
  - update app-level language state immediately
  - keep the rest of the draft settings intact
- Scope for this iteration:
  - `App.tsx`
  - `SettingsPage.tsx`
  - `AccountsPage.tsx`
  - shared API error/fallback strings where they surface in those flows

### Theme

- New setting key: `theme_mode`, values `system`, `light`, `dark`.
- Default: `system`.
- Settings control: same appearance/preferences card.
- On change:
  - persist immediately through `PUT /settings/app`
  - update `ConfigProvider` theme immediately
  - if `system`, follow `prefers-color-scheme`
- Light and dark themes should share the same brand primary while switching algorithm and container/background tokens.

## Data Model

- Backend `AppSettings` gains:
  - `Language string`
  - `ThemeMode string`
- SQLite additive migration:
  - `language TEXT NOT NULL DEFAULT 'zh-CN'`
  - `theme_mode TEXT NOT NULL DEFAULT 'system'`
- Sanitization normalizes unsupported values back to defaults.

## Testing

- Backend repository test for defaults, persistence, and sanitization of new fields.
- Settings handler test for GET/PUT round-trip including new fields.
- Frontend settings test verifying language/theme controls auto-save without clicking save.
- Frontend app test verifying:
  - Chinese remains default
  - English copy appears after settings update
  - theme mode changes apply to the app container/config path immediately

