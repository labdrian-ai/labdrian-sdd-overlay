# shellcheck shell=bash
#
# restore
#
# The restore command: roll one target back from a retained backup.
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

# cmd_restore --target <t> [--list] [--backup <ts>]
#
# overlay-backup-restore R-003, design decision D4: rolls a single target
# back from one of its retained backups (default: most recent). Refuses
# --target all (a single-target destructive rollback, never a blanket one).
# --list prints the retained backups (newest last) without touching
# anything. --backup <ts> selects a specific retained backup instead of the
# most recent. Deliberately performs ZERO git operations (design.md threat
# matrix) and never `cd`s into $OVERLAY_DIR: it only reads $MANIFEST (an
# absolute path already) to map repo-relative backup entries back onto
# their live destinations, and reads/writes files under
# $STATE_DIR/backups/<target>/ and the target's live directory.
cmd_restore() {
  local target_arg=""
  local list_mode=0
  local backup_ts=""

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --target)
        shift
        target_arg="${1:-}"
        [[ -n "$target_arg" ]] || die "--target requires an argument"
        ;;
      --list)
        list_mode=1
        ;;
      --backup)
        shift
        backup_ts="${1:-}"
        [[ -n "$backup_ts" ]] || die "--backup requires a timestamp argument"
        ;;
      *) die "Unknown option: $1. Usage: labdrian restore --target claude|opencode|codex [--list] [--backup TIMESTAMP]" ;;
    esac
    shift || true
  done

  [[ -n "$target_arg" ]] || die "restore requires --target claude|opencode|codex"
  [[ "$target_arg" != "all" ]] \
    || die "restore does not support --target all. Specify a single target: claude, opencode, codex"
  is_valid_target "$target_arg" \
    || die "Unknown target: $target_arg. Valid: claude, opencode, codex, pi"
  is_copy_target "$target_arg" \
    || die "restore does not support --target $target_arg (package target, no per-target backups); restore only supports claude, opencode, codex"

  local backups_dir="$STATE_DIR/backups/$target_arg"

  local -a entries=()
  local d
  shopt -s nullglob
  for d in "$backups_dir"/*/; do
    entries+=("$(basename "$d")")
  done
  shopt -u nullglob

  # See prune_backups' comment: bash's glob sorts the paths *with* their
  # trailing slash, which inverts a same-second collision suffix ("<ts>-1"
  # sorting before "<ts>" instead of after it). Re-sort the bare basenames
  # so both --list and the "most recent" default (below) are correct even
  # when two backups for the same target were taken in the same second.
  if [[ "${#entries[@]}" -gt 0 ]]; then
    mapfile -t entries < <(printf '%s\n' "${entries[@]}" | LC_ALL=C sort)
  fi

  if [[ "$list_mode" -eq 1 ]]; then
    if [[ "${#entries[@]}" -eq 0 ]]; then
      echo "No backups available for target '$target_arg'."
      return
    fi
    echo "Backups for $target_arg (newest last):"
    local e meta version
    for e in "${entries[@]}"; do
      version="unknown"
      if [[ -f "$backups_dir/$e/.meta" ]]; then
        meta="$(cat "$backups_dir/$e/.meta")"
        if [[ "$meta" != "NEVER_DEPLOYED" && -n "$meta" ]]; then
          version="$(echo "$meta" | awk -F'\t' '{ print $1 }')"
        fi
      fi
      echo "  $e ($version)"
    done
    return
  fi

  if [[ "${#entries[@]}" -eq 0 ]]; then
    die "No backups available for target '$target_arg'. Nothing to restore."
  fi

  local selected
  if [[ -n "$backup_ts" ]]; then
    local found=0
    for e in "${entries[@]}"; do
      [[ "$e" == "$backup_ts" ]] && found=1 && break
    done
    [[ "$found" -eq 1 ]] \
      || die "Backup '$backup_ts' not found for target '$target_arg'. Run 'labdrian restore --target $target_arg --list' to see available backups."
    selected="$backup_ts"
  else
    # 'entries' was explicitly re-sorted above; the last element is the
    # most recent backup (D4 default).
    selected="${entries[-1]}"
  fi

  local backup_dir="$backups_dir/$selected"

  info "Restoring $target_arg from backup $selected..."

  local rr_repo_rel rr_dest src_path
  manifest_routes_load "$target_arg" managed
  while IFS=$'\t' read -r _ _ rr_repo_rel _ rr_dest; do
    [[ -z "$rr_dest" ]] && continue

    src_path="$backup_dir/$rr_repo_rel"

    if [[ -f "$src_path" ]]; then
      ensure_parent "$rr_dest"
      cp "$src_path" "$rr_dest"
    fi
  done < <(manifest_routes_rows)

  local restored_digest
  restored_digest="$(compute_target_digest "$target_arg" live)"

  local restored_version="untagged"
  if [[ -f "$backup_dir/.meta" ]]; then
    local meta_content
    meta_content="$(cat "$backup_dir/.meta")"
    if [[ "$meta_content" != "NEVER_DEPLOYED" && -n "$meta_content" ]]; then
      restored_version="$(echo "$meta_content" | awk -F'\t' '{ print $1 }')"
    fi
  fi

  state_write_target "$target_arg" "$restored_version" "$restored_digest"

  info "Restored $target_arg from backup $selected (version $restored_version)."
}
