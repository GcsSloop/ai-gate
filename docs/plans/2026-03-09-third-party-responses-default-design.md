# Third-Party Responses Default Design

**Context**
- Thin gateway mode now supports native third-party `/responses` accounts.
- Current create-account behavior defaults third-party `supports_responses` to `false`, which causes newly added capable providers to be rejected until manually edited.
- Existing stored accounts already rely on an explicit `supports_responses` flag and should not be silently migrated.

**Decision**
- New third-party accounts created through `POST /accounts` default `supports_responses=true`.
- Clients can still explicitly send `"supports_responses": false` to opt out.
- Official accounts remain `supports_responses=true`.
- Existing stored accounts are unchanged.

**UI Impact**
- The account edit page must expose a visible toggle for `supports_responses` so the user can correct or override the default.
- The create flow may inherit the new backend default without forcing the user to make a choice.

**Error Handling**
- Thin gateway selection logic is unchanged: accounts without `supports_responses` are rejected.
- This change only affects the default value assigned at account creation time and the visibility of the edit control.

**Testing**
- Add a backend handler test proving third-party account creation defaults `supports_responses=true`.
- Add a backend handler test proving explicit `false` is preserved.
- Add or update frontend account-page tests to verify the toggle is rendered and submitted from the edit flow.
