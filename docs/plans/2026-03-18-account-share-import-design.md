# Account Share Import Design

**Goal:** Add a safe account share and import flow that lets users export a full, re-importable account credential package only after explicit confirmation, and lets users paste that package back into the app with strict validation, without changing routing priority or exposing secrets in the UI.

## Context

The accounts dashboard already supports create, edit, duplicate, delete, official import, and local import. What it does not support is transferring an existing account between devices or teammates without manually re-entering every field.

This feature must satisfy four constraints:

- The copied payload must contain the full credential and enough metadata to recreate the account.
- Sensitive content must never be rendered in plaintext in the UI.
- User-controlled sort order must remain untouched. Import creates a new account record, but sharing must not mutate the source account.
- Validation must be strict so malformed or partial payloads fail early before any account is created.

## Approaches

### Option A: Backend-issued JSON share package with frontend confirmation

Add a backend endpoint that loads the account, normalizes its portable fields, and returns a JSON payload string. The frontend shows a minimal confirmation modal and only writes the payload to the clipboard after the user confirms. Import pastes the payload into a modal, sends it to a backend validator/import endpoint, and then refreshes the account list.

**Pros**

- Keeps the canonical export schema in one place.
- Prevents the frontend from constructing or guessing sensitive payload structure.
- Makes import validation identical for all clients.
- Keeps future schema upgrades manageable with explicit `schema_version`.

**Cons**

- Requires two new backend endpoints.

### Option B: Frontend builds and validates the share package from list data

Use the existing account list payload to assemble the exported package in the browser and parse it locally on import.

**Pros**

- Smaller backend change set.

**Cons**

- The account list intentionally omits credential material, so this cannot produce a full importable package.
- Frontend validation would drift from backend account rules.
- Would encourage exposing more sensitive fields to normal list responses.

## Decision

Use **Option A**.

The backend will own the share-package schema, the export serialization, and the import validation. The frontend will only orchestrate confirmation, clipboard copy, paste, and success/error feedback.

## Share Package

Export a JSON string in this shape:

```json
{
  "kind": "aigate-account-share",
  "schema_version": 1,
  "exported_at": "2026-03-18T15:20:00Z",
  "account": {
    "provider_type": "openai-compatible",
    "account_name": "mirror-east",
    "source_icon": "ppchat",
    "auth_mode": "api_key",
    "base_url": "https://code.ppchat.vip/v1",
    "credential_ref": "sk-...",
    "account_driver": "builtin_openai_compatible",
    "usage_driver": "lua",
    "usage_config_json": "{\"script\":\"adapters/vendor.lua\"}",
    "supports_responses": true
  }
}
```

The package intentionally excludes runtime and local-state fields:

- `id`
- `status`
- `priority`
- `is_active`
- usage snapshots or health metrics

That keeps export portable and prevents importing stale operational state.

## Backend Design

### Export endpoint

Add `POST /accounts/{id}/share`.

Behavior:

- Load the account by id using the repository so `credential_ref` is already decrypted.
- Apply built-in driver defaults before export so the payload stays self-contained.
- Build the share package with current UTC `exported_at`.
- Return JSON:

```json
{ "payload": "<json-string>" }
```

### Import endpoint

Add `POST /accounts/import-shared`.

Request:

```json
{ "payload": "<json-string>" }
```

Behavior:

- Decode the outer request.
- Parse the payload string as JSON.
- Validate:
  - `kind == "aigate-account-share"`
  - `schema_version == 1`
  - `account` object present
  - required account fields present and non-empty where applicable
  - `provider_type` is supported
  - `auth_mode` is supported
  - `base_url` is a valid absolute HTTP or HTTPS URL
  - `usage_config_json` is either empty or valid JSON text
- Normalize `source_icon` and apply built-in driver defaults.
- Create a fresh account with:
  - `status = active`
  - `priority = 0`
  - `is_active = false`
- Return `201 Created`.

Import will not mutate any existing account. If the imported name already exists, creation is still allowed because the current schema does not enforce unique `account_name`.

## Frontend Design

### Share flow

In the account action area, add a share icon button.

Flow:

1. User clicks share.
2. Show a minimal confirmation modal stating that importable account data is about to be copied and must not be leaked.
3. On confirm, call the backend share endpoint.
4. Copy the returned payload string to the clipboard.
5. Show success or failure toast.

The modal must not render the payload itself, any credential preview, or derived secret fragments.

### Import flow

In the add-account area, add an import entry alongside the existing add-account options.

Flow:

1. User opens the import modal.
2. Paste the share payload into a multiline input.
3. Submit to backend import endpoint.
4. On success, close modal, refresh the account list, and refresh tray state.
5. On failure, keep the modal open and show the validation error.

The frontend may do lightweight empty-state validation, but the backend remains the source of truth.

## Error Handling

- Invalid payload format returns `400 Bad Request`.
- Unknown schema version returns `400 Bad Request`.
- Unsupported provider or auth mode returns `400 Bad Request`.
- Invalid `base_url` or malformed `usage_config_json` returns `400 Bad Request`.
- Clipboard failure returns a user-facing error and must not silently claim success.
- Canceling the share modal performs no backend call and no clipboard write.

## Testing

### Backend

Add handler tests for:

- exporting a valid share payload with credential included
- importing a valid payload creates a new account with reset runtime fields
- invalid `kind` is rejected
- invalid `schema_version` is rejected
- invalid `base_url` is rejected
- invalid `usage_config_json` is rejected

### Frontend

Add page tests for:

- share cancel does not call clipboard or backend
- share confirm calls backend and copies payload
- import modal submits pasted payload
- import failure keeps modal open and surfaces the backend error

## Success Criteria

- Users can share an account without seeing secret material in the UI.
- The copied content is sufficient to import the same account elsewhere.
- Import rejects malformed payloads before creating records.
- Existing manual sort order is not rewritten by the share flow.
- The interaction stays visually minimal and consistent with the current account page.
