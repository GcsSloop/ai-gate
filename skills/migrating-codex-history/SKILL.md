---
name: migrating-codex-history
description: Use when the user wants to copy Codex session history from one provider namespace to another, especially migrating official OpenAI sessions into the current AI Gate provider without overwriting the source sessions.
---

# Migrating Codex History

## Overview

Reuse `scripts/migrate_openai_history_to_aigate.sh` to copy local Codex session history from one `model_provider` to another.

Script source priority:

- local repository path: `scripts/migrate_openai_history_to_aigate.sh`
- remote raw URL: `https://raw.githubusercontent.com/GcsSloop/ai-gate/main/scripts/migrate_openai_history_to_aigate.sh`

Default behavior:

- source provider: `openai`
- target provider: `aigate`
- codex home: `~/.codex`

The migration copies sessions. It does not overwrite the original source sessions.

## When To Use

- The user wants official Codex or OpenAI sessions to appear under the current AI Gate provider.
- The user asks to migrate, duplicate, sync, or re-home Codex history between providers.
- The user wants a dry run before changing local session files.

Do not use this skill for remote exports, cloud backups, or non-Codex history formats.

## Workflow

1. Resolve the script path:
   - if `scripts/migrate_openai_history_to_aigate.sh` exists in the current repository, use it
   - otherwise download `https://raw.githubusercontent.com/GcsSloop/ai-gate/main/scripts/migrate_openai_history_to_aigate.sh` to a temporary file and use that file
2. Detect the operating system:
   - on macOS or Linux, the shell script can be executed directly
   - on Windows, do not try to execute the `.sh` file as-is; first convert the script logic into equivalent PowerShell or native Windows command steps, then run the Windows-adapted workflow
3. Run a dry run first unless the user explicitly asks to skip it.
4. Show the migration summary before making changes.
5. Run the real migration with the approved options.
6. Report the final summary and call out whether `state_5.sqlite` and `session_index.jsonl` were updated.

## Commands

If the repository is already present locally:

```bash
bash scripts/migrate_openai_history_to_aigate.sh --dry-run
```

If the repository is not present locally, download the script first:

```bash
tmp_script="$(mktemp)"
curl -fsSL "https://raw.githubusercontent.com/GcsSloop/ai-gate/main/scripts/migrate_openai_history_to_aigate.sh" -o "$tmp_script"
chmod +x "$tmp_script"
bash "$tmp_script" --dry-run
```

For Windows users:

- treat the downloaded `.sh` as the source of truth for migration behavior
- translate its file scanning, JSON patching, UUID generation, and SQLite/session index updates into PowerShell before execution
- do not claim Windows support by directly invoking the shell script unless the environment explicitly provides a compatible Unix shell and the required tools

Default migration:

```bash
bash scripts/migrate_openai_history_to_aigate.sh
```

Migrate to a custom provider:

```bash
bash scripts/migrate_openai_history_to_aigate.sh \
  --from-provider openai \
  --to-provider router \
  --dry-run
```

Align migrated `source` fields with the source-provider sessions:

```bash
bash scripts/migrate_openai_history_to_aigate.sh --sync-source-from-provider
```

Skip archived history:

```bash
bash scripts/migrate_openai_history_to_aigate.sh --skip-archived
```

## Requirements

- `jq`
- `uuidgen`
- `curl` when downloading the remote script
- optional `sqlite3` for `state_5.sqlite` source reconciliation
- on Windows, equivalent PowerShell support for JSON, file traversal, UUID generation, and optional SQLite updates

## Safety Notes

- Source sessions stay untouched.
- Re-running the script does not duplicate already migrated sessions with the same `forked_from_id`.
- The script updates `session_index.jsonl` so migrated sessions remain visible in local session lists.
- The script may update `state_5.sqlite` source fields when `sqlite3` is available.
- Prefer downloading to a temporary file and then executing it. Do not use `curl | bash`.
- On Windows, the AI must adapt the script behavior rather than pretend the `.sh` runs natively.

## Response Format

When using this skill, report:

- the exact command that was run
- whether it was a dry run or a real migration
- the final `summary: copied=... patched_existing=... patched_db=... patched_index=...`
- any skipped conditions or missing tools
