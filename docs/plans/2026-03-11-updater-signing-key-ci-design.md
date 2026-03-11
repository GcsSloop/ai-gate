# Updater Signing Key CI Decode Design

## Context
Windows release build currently fails while Tauri tries to decode the updater signing key. The error message is `Missing comment in secret key`, which indicates the CLI is not receiving the expected minisign secret key text.

## Root Cause
The repository's local updater key file is stored as a single-line base64 payload. CI currently forwards the GitHub secret directly into `TAURI_SIGNING_PRIVATE_KEY`, so Tauri receives the encoded blob instead of the decoded minisign secret key text that starts with `untrusted comment:`.

## Decision
Keep the existing GitHub secret, but decode it inside the release workflow before running `tauri build`. Export the decoded result through `GITHUB_ENV` so the build step sees the proper multiline secret key format. Add a workflow regression test that asserts this decode step remains present.

## Expected Result
- `tauri build` receives a valid minisign secret key text.
- Existing public key config remains unchanged.
- Release workflow stays compatible with the current secret storage format.
