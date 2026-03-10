#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  bash scripts/migrate_openai_history_to_aigate.sh [options]

Options:
  --codex-home <path>      Codex home directory. Default: ~/.codex
  --from-provider <name>   Source provider id. Default: openai
  --to-provider <name>     Target provider id. Default: aigate
  --force-source <kind>    Force session_meta.source on copied/target records (e.g. cli)
  --sync-source-from-provider
                           For migrated target records, align source with source-provider records by forked_from_id
  --skip-archived          Skip archived_sessions migration
  --dry-run                Print planned changes only
  -h, --help               Show this help
EOF
}

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "required command not found: $cmd" >&2
    exit 1
  fi
}

codex_home="${HOME}/.codex"
from_provider="openai"
to_provider="aigate"
include_archived=1
dry_run=0
force_source=""
sync_source_from_provider=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --codex-home)
      codex_home="$2"
      shift 2
      ;;
    --from-provider)
      from_provider="$2"
      shift 2
      ;;
    --to-provider)
      to_provider="$2"
      shift 2
      ;;
    --force-source)
      force_source="$2"
      shift 2
      ;;
    --sync-source-from-provider)
      sync_source_from_provider=1
      shift
      ;;
    --skip-archived)
      include_archived=0
      shift
      ;;
    --dry-run)
      dry_run=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage
      exit 1
      ;;
  esac
done

require_cmd jq
require_cmd uuidgen

if [[ ! -d "$codex_home" ]]; then
  echo "codex home does not exist: $codex_home" >&2
  exit 1
fi

declare -a roots=("$codex_home/sessions")
if [[ "$include_archived" -eq 1 ]]; then
  roots+=("$codex_home/archived_sessions")
fi

declare -a files=()
migrated_ids_file="$(mktemp)"
patched_existing=0
source_lookup_file="$(mktemp)"
db_source_update_file="$(mktemp)"
source_to_target_ids_file="$(mktemp)"
cleanup() {
  rm -f "$migrated_ids_file"
  rm -f "$source_lookup_file"
  rm -f "$db_source_update_file"
  rm -f "$source_to_target_ids_file"
}
trap cleanup EXIT

while IFS= read -r -d '' path; do
  files+=("$path")
done < <(
  for root in "${roots[@]}"; do
    if [[ -d "$root" ]]; then
      find "$root" -type f -name 'rollout-*.jsonl' -print0
    fi
  done
)

for file in "${files[@]}"; do
  first_line="$(head -n1 "$file" || true)"
  if [[ -z "$first_line" ]]; then
    continue
  fi
  src_provider="$(
    printf '%s' "$first_line" | jq -r '
      if .item? and .item.type? == "session_meta" then
        (.item.meta.model_provider // empty)
      elif .type? == "session_meta" then
        (.payload.model_provider // empty)
      else
        empty
      end
    ' 2>/dev/null || true
  )"
  src_id="$(
    printf '%s' "$first_line" | jq -r '
      if .item? and .item.type? == "session_meta" then
        (.item.meta.id // empty)
      elif .type? == "session_meta" then
        (.payload.id // empty)
      else
        empty
      end
    ' 2>/dev/null || true
  )"
  src_source="$(
    printf '%s' "$first_line" | jq -r '
      if .item? and .item.type? == "session_meta" then
        (.item.meta.source // empty)
      elif .type? == "session_meta" then
        (.payload.source // empty)
      else
        empty
      end
    ' 2>/dev/null || true
  )"
  if [[ "$src_provider" == "$from_provider" && -n "$src_id" && -n "$src_source" ]]; then
    printf '%s\t%s\n' "$src_id" "$src_source" >>"$source_lookup_file"
  fi
done

for file in "${files[@]}"; do
  first_line="$(head -n1 "$file" || true)"
  if [[ -z "$first_line" ]]; then
    continue
  fi
  model_provider="$(
    printf '%s' "$first_line" | jq -r '
      if .item? and .item.type? == "session_meta" then
        (.item.meta.model_provider // empty)
      elif .type? == "session_meta" then
        (.payload.model_provider // empty)
      else
        empty
      end
    ' 2>/dev/null || true
  )"
  forked_from_id="$(
    printf '%s' "$first_line" | jq -r '
      if .item? and .item.type? == "session_meta" then
        (.item.meta.forked_from_id // empty)
      elif .type? == "session_meta" then
        (.payload.forked_from_id // empty)
      else
        empty
      end
    ' 2>/dev/null || true
  )"
  target_id="$(
    printf '%s' "$first_line" | jq -r '
      if .item? and .item.type? == "session_meta" then
        (.item.meta.id // empty)
      elif .type? == "session_meta" then
        (.payload.id // empty)
      else
        empty
      end
    ' 2>/dev/null || true
  )"
  if [[ "$model_provider" == "$to_provider" && -n "$forked_from_id" ]]; then
    printf '%s\n' "$forked_from_id" >>"$migrated_ids_file"
    desired_source="$force_source"
    if [[ -z "$desired_source" && "$sync_source_from_provider" -eq 1 ]]; then
      desired_source="$(
        awk -F '\t' -v sid="$forked_from_id" '$1 == sid { print $2; exit }' "$source_lookup_file"
      )"
    fi
    if [[ -n "$desired_source" ]]; then
      current_source="$(
        printf '%s' "$first_line" | jq -r '
          if .item? and .item.type? == "session_meta" then
            (.item.meta.source // empty)
          elif .type? == "session_meta" then
            (.payload.source // empty)
          else
            empty
            end
        ' 2>/dev/null || true
      )"
      if [[ "$current_source" != "$desired_source" ]]; then
        patched_first_line="$(
          printf '%s' "$first_line" | jq -c \
            --arg desired_source "$desired_source" \
            '
            if .item? and .item.type? == "session_meta" then
              .item.meta.source = $desired_source
            elif .type? == "session_meta" then
              .payload.source = $desired_source
            else
              .
            end
            '
        )"
        if [[ "$dry_run" -eq 1 ]]; then
          echo "DRY-RUN patch source: $file ($current_source -> $desired_source)"
        else
          tmp_file="$(mktemp)"
          {
            printf '%s\n' "$patched_first_line"
            tail -n +2 "$file"
          } > "$tmp_file"
          mv "$tmp_file" "$file"
          echo "patched source: $file ($current_source -> $desired_source)"
        fi
        patched_existing=$((patched_existing + 1))
      fi
      if [[ -n "$target_id" ]]; then
        printf '%s\t%s\n' "$target_id" "$desired_source" >>"$db_source_update_file"
      fi
    fi
  fi
done

copied=0
skipped_non_source=0
skipped_already_migrated=0
skipped_invalid=0

for src in "${files[@]}"; do
  base_name="$(basename "$src")"
  if [[ ! "$base_name" =~ ^rollout-(.+)-([0-9a-fA-F-]{36})\.jsonl$ ]]; then
    skipped_invalid=$((skipped_invalid + 1))
    continue
  fi
  timestamp_part="${BASH_REMATCH[1]}"

  first_line="$(head -n1 "$src" || true)"
  if [[ -z "$first_line" ]]; then
    skipped_invalid=$((skipped_invalid + 1))
    continue
  fi

  src_provider="$(
    printf '%s' "$first_line" | jq -r '
      if .item? and .item.type? == "session_meta" then
        (.item.meta.model_provider // empty)
      elif .type? == "session_meta" then
        (.payload.model_provider // empty)
      else
        empty
      end
    ' 2>/dev/null || true
  )"
  src_id="$(
    printf '%s' "$first_line" | jq -r '
      if .item? and .item.type? == "session_meta" then
        (.item.meta.id // empty)
      elif .type? == "session_meta" then
        (.payload.id // empty)
      else
        empty
      end
    ' 2>/dev/null || true
  )"
  session_meta_shape="$(
    printf '%s' "$first_line" | jq -r '
      if .item? and .item.type? == "session_meta" then
        "new"
      elif .type? == "session_meta" then
        "legacy"
      else
        empty
      end
    ' 2>/dev/null || true
  )"
  if [[ -z "$session_meta_shape" || -z "$src_id" ]]; then
    skipped_invalid=$((skipped_invalid + 1))
    continue
  fi
  if [[ "$src_provider" != "$from_provider" ]]; then
    skipped_non_source=$((skipped_non_source + 1))
    continue
  fi
  if grep -Fxq "$src_id" "$migrated_ids_file"; then
    skipped_already_migrated=$((skipped_already_migrated + 1))
    continue
  fi

  src_dir="$(dirname "$src")"
  while :; do
    new_id="$(uuidgen | tr '[:upper:]' '[:lower:]')"
    dst="$src_dir/rollout-${timestamp_part}-${new_id}.jsonl"
    if [[ ! -e "$dst" ]]; then
      break
    fi
  done

  desired_source="$force_source"
  if [[ -z "$desired_source" && "$sync_source_from_provider" -eq 1 ]]; then
    desired_source="$(
      awk -F '\t' -v sid="$src_id" '$1 == sid { print $2; exit }' "$source_lookup_file"
    )"
  fi

  patched_first_line="$(
    printf '%s' "$first_line" | jq -c \
      --arg new_id "$new_id" \
      --arg to_provider "$to_provider" \
      --arg source_id "$src_id" \
      --arg desired_source "$desired_source" \
      '
      if .item? and .item.type? == "session_meta" then
        .item.meta.id = $new_id
        | .item.meta.model_provider = $to_provider
        | .item.meta.forked_from_id = $source_id
        | .item.meta.originator = "aigate-history-migrator"
        | if $desired_source == "" then . else .item.meta.source = $desired_source end
      elif .type? == "session_meta" then
        .payload.id = $new_id
        | .payload.model_provider = $to_provider
        | .payload.forked_from_id = $source_id
        | .payload.originator = "aigate-history-migrator"
        | if $desired_source == "" then . else .payload.source = $desired_source end
      else
        .
      end
      '
  )"

  if [[ "$dry_run" -eq 1 ]]; then
    echo "DRY-RUN copy: $src -> $dst"
  else
    {
      printf '%s\n' "$patched_first_line"
      tail -n +2 "$src"
    } > "$dst"
    echo "copied: $src -> $dst"
  fi

  printf '%s\t%s\n' "$src_id" "$new_id" >>"$source_to_target_ids_file"
  printf '%s\n' "$src_id" >>"$migrated_ids_file"
  copied=$((copied + 1))
done

patched_db=0
state_db_path="$codex_home/state_5.sqlite"
if [[ -s "$db_source_update_file" && -f "$state_db_path" ]]; then
  if command -v sqlite3 >/dev/null 2>&1; then
    unique_db_updates_file="$(mktemp)"
    awk -F '\t' '!seen[$1]++ {print $1"\t"$2}' "$db_source_update_file" >"$unique_db_updates_file"
    planned_patched_db="$(wc -l <"$unique_db_updates_file" | tr -d ' ')"
    if [[ "$dry_run" -eq 1 ]]; then
      echo "DRY-RUN patch state db source rows: $planned_patched_db ($state_db_path)"
    else
      sql_file="$(mktemp)"
      {
        echo "BEGIN;"
        while IFS=$'\t' read -r target_id desired_source; do
          printf "UPDATE threads SET source = '%s' WHERE id = '%s';\n" "$desired_source" "$target_id"
        done <"$unique_db_updates_file"
        echo "COMMIT;"
      } >"$sql_file"
      sqlite3 "$state_db_path" <"$sql_file"
      rm -f "$sql_file"
      patched_db="$planned_patched_db"
    fi
    rm -f "$unique_db_updates_file"
  else
    echo "warning: sqlite3 not found, skipped state db source reconciliation" >&2
  fi
fi

patched_index=0
session_index_path="$codex_home/session_index.jsonl"
if [[ -f "$session_index_path" ]]; then
  source_name_map_file="$(mktemp)"
  existing_index_ids_file="$(mktemp)"
  index_update_pairs_file="$(mktemp)"
  while IFS= read -r line; do
    id="$(printf '%s' "$line" | jq -r '.id // empty' 2>/dev/null || true)"
    thread_name="$(printf '%s' "$line" | jq -r '.thread_name // empty' 2>/dev/null || true)"
    updated_at="$(printf '%s' "$line" | jq -r '.updated_at // empty' 2>/dev/null || true)"
    if [[ -n "$id" ]]; then
      printf '%s\n' "$id" >>"$existing_index_ids_file"
      if [[ -n "$thread_name" ]]; then
        printf '%s\t%s\t%s\n' "$id" "$thread_name" "$updated_at" >>"$source_name_map_file"
      fi
    fi
  done <"$session_index_path"

  # Existing migrated copies.
  for file in "${files[@]}"; do
    first_line="$(head -n1 "$file" || true)"
    if [[ -z "$first_line" ]]; then
      continue
    fi
    model_provider="$(
      printf '%s' "$first_line" | jq -r '
        if .item? and .item.type? == "session_meta" then
          (.item.meta.model_provider // empty)
        elif .type? == "session_meta" then
          (.payload.model_provider // empty)
        else
          empty
        end
      ' 2>/dev/null || true
    )"
    forked_from_id="$(
      printf '%s' "$first_line" | jq -r '
        if .item? and .item.type? == "session_meta" then
          (.item.meta.forked_from_id // empty)
        elif .type? == "session_meta" then
          (.payload.forked_from_id // empty)
        else
          empty
        end
      ' 2>/dev/null || true
    )"
    target_id="$(
      printf '%s' "$first_line" | jq -r '
        if .item? and .item.type? == "session_meta" then
          (.item.meta.id // empty)
        elif .type? == "session_meta" then
          (.payload.id // empty)
        else
          empty
        end
      ' 2>/dev/null || true
    )"
    if [[ "$model_provider" == "$to_provider" && -n "$forked_from_id" && -n "$target_id" ]]; then
      printf '%s\t%s\n' "$forked_from_id" "$target_id" >>"$index_update_pairs_file"
    fi
  done

  # Newly copied pairs.
  if [[ -s "$source_to_target_ids_file" ]]; then
    cat "$source_to_target_ids_file" >>"$index_update_pairs_file"
  fi

  if [[ -s "$index_update_pairs_file" ]]; then
    unique_pairs_file="$(mktemp)"
    awk -F '\t' '!seen[$2]++ {print $1"\t"$2}' "$index_update_pairs_file" >"$unique_pairs_file"
    while IFS=$'\t' read -r source_id target_id; do
      if grep -Fxq "$target_id" "$existing_index_ids_file"; then
        continue
      fi
      source_meta="$(
        awk -F '\t' -v sid="$source_id" '$1 == sid { print; found=1 } END { if (found != 1) print "" }' "$source_name_map_file"
      )"
      source_thread_name=""
      source_updated_at=""
      if [[ -n "$source_meta" ]]; then
        source_thread_name="$(printf '%s' "$source_meta" | cut -f2)"
        source_updated_at="$(printf '%s' "$source_meta" | cut -f3)"
      fi
      if [[ -z "$source_thread_name" && -f "$state_db_path" && -x "$(command -v sqlite3 || true)" ]]; then
        source_thread_name="$(
          sqlite3 "$state_db_path" "select replace(substr(first_user_message, 1, 120), char(10), ' ') from threads where id = '$source_id' limit 1;" 2>/dev/null || true
        )"
      fi
      if [[ -z "$source_thread_name" && -f "$state_db_path" && -x "$(command -v sqlite3 || true)" ]]; then
        source_thread_name="$(
          sqlite3 "$state_db_path" "select replace(first_user_message, char(10), ' ') from threads where id = '$target_id' limit 1;" 2>/dev/null || true
        )"
      fi
      if [[ -z "$source_thread_name" ]]; then
        continue
      fi
      if [[ -z "$source_updated_at" ]]; then
        source_updated_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
      fi
      entry="$(
        jq -nc \
          --arg id "$target_id" \
          --arg thread_name "$source_thread_name" \
          --arg updated_at "$source_updated_at" \
          '{id:$id, thread_name:$thread_name, updated_at:$updated_at}'
      )"
      if [[ "$dry_run" -eq 1 ]]; then
        echo "DRY-RUN append session_index: $target_id"
      else
        printf '%s\n' "$entry" >>"$session_index_path"
      fi
      printf '%s\n' "$target_id" >>"$existing_index_ids_file"
      patched_index=$((patched_index + 1))
    done <"$unique_pairs_file"
    rm -f "$unique_pairs_file"
  fi

  rm -f "$source_name_map_file" "$existing_index_ids_file" "$index_update_pairs_file"
fi

echo "summary: copied=$copied patched_existing=$patched_existing patched_db=$patched_db patched_index=$patched_index skipped_non_source=$skipped_non_source skipped_already_migrated=$skipped_already_migrated skipped_invalid=$skipped_invalid"
