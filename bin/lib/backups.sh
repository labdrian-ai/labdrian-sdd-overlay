# shellcheck shell=bash
#
# backups
#
# The backups apply takes of a target's deployed files, and the pruning that keeps the newest
# three.
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

# backup_target <target>
#
# overlay-backup-restore R-001 (design decision D3/D4): creates a
# timestamped backup of <target>'s CURRENTLY DEPLOYED managed files, but
# only for rows that are about to actually change -- a managed row whose
# live file does not yet exist (never deployed) or whose live file already
# matches the repo-source file about to be deployed (true no-op) has
# nothing meaningful to preserve, so it is excluded from both the
# needs-backup decision and the copy. Prints the created backup's
# timestamp directory name on stdout when a backup was made; prints
# nothing when the apply for this target is a full no-op. Backups live
# under $STATE_DIR/backups/<target>/<utc-ts>/ (D3), holding the full
# managed live-file set for the target under its repo-relative paths, plus
# a '.meta' file capturing the PRIOR state_read_target entry (read before
# any file is copied) so restore can recover the version that backup
# represents.
backup_target() {
  local target="$1"

  local -a changed_repo_rels=()
  local -a changed_dests=()
  local rr_repo_rel rr_src rr_dest
  manifest_routes_load "$target" managed
  while IFS=$'\t' read -r _ _ rr_repo_rel rr_src rr_dest; do
    [[ -z "$rr_dest" ]] && continue

    # Only an EXISTING live file that is about to be OVERWRITTEN needs
    # preserving. A row whose repo source is missing is skipped by apply
    # itself (never overwritten), and a target with no live file yet has
    # nothing to preserve -- both degrade to "nothing to back up" here.
    if [[ -f "$rr_dest" && -f "$rr_src" ]] && ! diff --brief "$rr_src" "$rr_dest" > /dev/null 2>&1; then
      changed_repo_rels+=("$rr_repo_rel")
      changed_dests+=("$rr_dest")
    fi
  done < <(manifest_routes_rows)

  # Explicit 'return 0' (not a bare 'return' after a failing [[ ]] test):
  # under this script's 'set -e', a bare 'return' here would propagate the
  # [[ ]] test's own nonzero status as this function's exit code, tripping
  # errexit for any caller that assigns its stdout via command substitution
  # (e.g. cmd_apply's `backup_ts="$(backup_target "$t")"`).
  [[ "${#changed_dests[@]}" -gt 0 ]] || return 0

  local backups_dir="$STATE_DIR/backups/$target"
  mkdir -p "$backups_dir"

  local ts candidate suffix=0
  ts="$(date -u +%Y%m%dT%H%M%SZ)"
  candidate="$ts"
  while [[ -e "$backups_dir/$candidate" ]]; do
    suffix=$((suffix + 1))
    candidate="${ts}-${suffix}"
  done

  local backup_dir="$backups_dir/$candidate"
  mkdir -p "$backup_dir"

  local i dest_path
  for i in "${!changed_dests[@]}"; do
    dest_path="$backup_dir/${changed_repo_rels[$i]}"
    mkdir -p "$(dirname "$dest_path")"
    cp "${changed_dests[$i]}" "$dest_path"
  done

  local prior
  prior="$(state_read_target "$target" 2>/dev/null || true)"
  echo "$prior" > "$backup_dir/.meta"

  prune_backups "$target"

  echo "$candidate"
}

# prune_backups <target>
#
# overlay-backup-restore R-002 (design decision D4): retains only the 3
# most recent backups for a target, deleting older ones. Backup dir names
# are UTC timestamps (YYYYMMDDTHHMMSSZ, optionally suffixed by
# backup_target on a same-second collision, e.g. "<ts>-1"). Bash's own
# pathname expansion sorts the *globbed paths*, trailing slash included --
# that inverts a same-second collision, because ASCII '-' (0x2D) sorts
# before '/' (0x2F), so "<ts>-1/" sorts before "<ts>/" even though "<ts>-1"
# is the chronologically newer one. Re-sort the bare basenames explicitly
# so a same-second collision can't prune the wrong (newer) backup.
prune_backups() {
  local target="$1"
  local backups_dir="$STATE_DIR/backups/$target"

  [[ -d "$backups_dir" ]] || return 0

  local -a entries=()
  local d
  shopt -s nullglob
  for d in "$backups_dir"/*/; do
    entries+=("$(basename "$d")")
  done
  shopt -u nullglob

  if [[ "${#entries[@]}" -gt 0 ]]; then
    mapfile -t entries < <(printf '%s\n' "${entries[@]}" | LC_ALL=C sort)
  fi

  local total="${#entries[@]}"
  local keep=3
  [[ "$total" -le "$keep" ]] && return 0

  local excess=$((total - keep))
  local i
  for ((i = 0; i < excess; i++)); do
    rm -rf "${backups_dir:?}/${entries[$i]}"
  done
}
