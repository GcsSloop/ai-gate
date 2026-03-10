#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT_PATH="$ROOT_DIR/scripts/migrate_openai_history_to_aigate.sh"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_eq() {
  local got="$1"
  local want="$2"
  local msg="$3"
  if [[ "$got" != "$want" ]]; then
    fail "$msg (got=$got want=$want)"
  fi
}

assert_nonempty() {
  local value="$1"
  local msg="$2"
  if [[ -z "$value" ]]; then
    fail "$msg"
  fi
}

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

codex_home="$tmp_dir/codex-home"
mkdir -p \
  "$codex_home/sessions/2026/03/10" \
  "$codex_home/archived_sessions/2026/03/09"

cat >"$codex_home/session_index.jsonl" <<'JSONL'
{"id":"11111111-1111-1111-1111-111111111111","thread_name":"source-openai-live","updated_at":"2026-03-10T08:00:00Z"}
{"id":"22222222-2222-2222-2222-222222222222","thread_name":"source-openai-archived","updated_at":"2026-03-09T07:00:00Z"}
JSONL

src_live_id="11111111-1111-1111-1111-111111111111"
src_archived_id="22222222-2222-2222-2222-222222222222"
other_id="33333333-3333-3333-3333-333333333333"

cat >"$codex_home/sessions/2026/03/10/rollout-2026-03-10T08-00-00-$src_live_id.jsonl" <<'JSONL'
{"timestamp":"2026-03-10T08:00:00Z","item":{"type":"session_meta","meta":{"id":"11111111-1111-1111-1111-111111111111","forked_from_id":null,"timestamp":"2026-03-10T08:00:00Z","cwd":"/tmp","originator":"test","cli_version":"0.0.0","source":"vscode","agent_nickname":null,"agent_role":null,"model_provider":"openai","base_instructions":null,"dynamic_tools":null,"memory_mode":null},"git":null}}
{"timestamp":"2026-03-10T08:00:01Z","item":{"type":"event_msg","payload":{"type":"user_message","message":"hello live","images":null,"local_images":[],"text_elements":[]}}}
JSONL

cat >"$codex_home/archived_sessions/2026/03/09/rollout-2026-03-09T07-00-00-$src_archived_id.jsonl" <<'JSONL'
{"timestamp":"2026-03-09T07:00:00Z","item":{"type":"session_meta","meta":{"id":"22222222-2222-2222-2222-222222222222","forked_from_id":null,"timestamp":"2026-03-09T07:00:00Z","cwd":"/tmp","originator":"test","cli_version":"0.0.0","source":"vscode","agent_nickname":null,"agent_role":null,"model_provider":"openai","base_instructions":null,"dynamic_tools":null,"memory_mode":null},"git":null}}
{"timestamp":"2026-03-09T07:00:01Z","item":{"type":"event_msg","payload":{"type":"user_message","message":"hello archived","images":null,"local_images":[],"text_elements":[]}}}
JSONL

cat >"$codex_home/sessions/2026/03/10/rollout-2026-03-10T06-00-00-$other_id.jsonl" <<'JSONL'
{"timestamp":"2026-03-10T06:00:00Z","item":{"type":"session_meta","meta":{"id":"33333333-3333-3333-3333-333333333333","forked_from_id":null,"timestamp":"2026-03-10T06:00:00Z","cwd":"/tmp","originator":"test","cli_version":"0.0.0","source":"cli","agent_nickname":null,"agent_role":null,"model_provider":"router","base_instructions":null,"dynamic_tools":null,"memory_mode":null},"git":null}}
{"timestamp":"2026-03-10T06:00:01Z","item":{"type":"event_msg","payload":{"type":"user_message","message":"router history","images":null,"local_images":[],"text_elements":[]}}}
JSONL

bash "$SCRIPT_PATH" --codex-home "$codex_home" --from-provider openai --to-provider aigate

copied_live_path="$(
  rg -l --fixed-strings "\"forked_from_id\":\"$src_live_id\"" "$codex_home/sessions" | head -n1 || true
)"
assert_nonempty "$copied_live_path" "live openai session should be copied to aigate"

copied_archived_path="$(
  rg -l --fixed-strings "\"forked_from_id\":\"$src_archived_id\"" "$codex_home/archived_sessions" | head -n1 || true
)"
assert_nonempty "$copied_archived_path" "archived openai session should be copied to aigate"

copied_live_provider="$(head -n1 "$copied_live_path" | jq -r '.item.meta.model_provider')"
assert_eq "$copied_live_provider" "aigate" "copied live session provider"

copied_archived_provider="$(head -n1 "$copied_archived_path" | jq -r '.item.meta.model_provider')"
assert_eq "$copied_archived_provider" "aigate" "copied archived session provider"

copied_live_id="$(head -n1 "$copied_live_path" | jq -r '.item.meta.id')"
if [[ "$copied_live_id" == "$src_live_id" ]]; then
  fail "copied live session id must differ from source id"
fi

copied_live_name="$(
  jq -r --arg id "$copied_live_id" 'select(.id == $id) | .thread_name' "$codex_home/session_index.jsonl" | tail -n1
)"
assert_eq "$copied_live_name" "source-openai-live" "copied live session should get session_index entry"

original_live_provider="$(head -n1 "$codex_home/sessions/2026/03/10/rollout-2026-03-10T08-00-00-$src_live_id.jsonl" | jq -r '.item.meta.model_provider')"
assert_eq "$original_live_provider" "openai" "source live session must stay openai"

original_archived_provider="$(head -n1 "$codex_home/archived_sessions/2026/03/09/rollout-2026-03-09T07-00-00-$src_archived_id.jsonl" | jq -r '.item.meta.model_provider')"
assert_eq "$original_archived_provider" "openai" "source archived session must stay openai"

router_copy_count="$(
  { rg -l --fixed-strings "\"forked_from_id\":\"$other_id\"" "$codex_home/sessions" "$codex_home/archived_sessions" || true; } | wc -l | tr -d ' '
)"
assert_eq "$router_copy_count" "0" "non-openai session must not be copied"

bash "$SCRIPT_PATH" --codex-home "$codex_home" --from-provider openai --to-provider aigate >/dev/null

live_copy_count="$(
  { rg -l --fixed-strings "\"forked_from_id\":\"$src_live_id\"" "$codex_home/sessions" || true; } | wc -l | tr -d ' '
)"
assert_eq "$live_copy_count" "1" "re-running script must not duplicate live copy"

archived_copy_count="$(
  { rg -l --fixed-strings "\"forked_from_id\":\"$src_archived_id\"" "$codex_home/archived_sessions" || true; } | wc -l | tr -d ' '
)"
assert_eq "$archived_copy_count" "1" "re-running script must not duplicate archived copy"

existing_migrated_id="44444444-4444-4444-4444-444444444444"
cat >"$codex_home/sessions/2026/03/10/rollout-2026-03-10T05-00-00-$existing_migrated_id.jsonl" <<JSONL
{"timestamp":"2026-03-10T05:00:00Z","item":{"type":"session_meta","meta":{"id":"$existing_migrated_id","forked_from_id":"$src_live_id","timestamp":"2026-03-10T05:00:00Z","cwd":"/tmp","originator":"aigate-history-migrator","cli_version":"0.0.0","source":"cli","agent_nickname":null,"agent_role":null,"model_provider":"aigate","base_instructions":null,"dynamic_tools":null,"memory_mode":null},"git":null}}
{"timestamp":"2026-03-10T05:00:01Z","item":{"type":"event_msg","payload":{"type":"user_message","message":"old migrated copy","images":null,"local_images":[],"text_elements":[]}}}
JSONL

bash "$SCRIPT_PATH" --codex-home "$codex_home" --from-provider openai --to-provider aigate --sync-source-from-provider

patched_source="$(
  head -n1 "$codex_home/sessions/2026/03/10/rollout-2026-03-10T05-00-00-$existing_migrated_id.jsonl" | jq -r '.item.meta.source'
)"
assert_eq "$patched_source" "vscode" "sync-source-from-provider should align migrated source with source-provider"

live_copy_count_after_sync="$(
  { rg -l --fixed-strings "\"forked_from_id\":\"$src_live_id\"" "$codex_home/sessions" || true; } | wc -l | tr -d ' '
)"
assert_eq "$live_copy_count_after_sync" "2" "source sync patching must not create duplicate files"

echo "PASS: migrate_openai_history_to_aigate_test"
