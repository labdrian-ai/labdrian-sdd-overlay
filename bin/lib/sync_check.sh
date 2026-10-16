# shellcheck shell=bash
#
# sync_check
#
# The sync-check command: validate the gentle-ai sync state of each target and print the
# verdicts the TUI reads.
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

# cmd_sync_check's --target pi output always includes a "pipkg check:
# compared against ..." basis-disclosure line (via pipkg_sync_check_and_report,
# R-006/R-007): "builtFrom ref <sha>" when the deployed package's recorded
# commit was resolvable locally, "main; builtFrom <sha> is not resolvable
# locally" on fallback, or "the worktree" when OVERLAY_DIR is not a git
# repository at all. This is what closes #315: a feature branch with
# unrelated changes no longer reports false drift, because the comparison
# target is disclosed rather than silently assumed to be the current
# checkout.
cmd_sync_check() {
  # shellcheck disable=SC2164  # errexit is on (set -e in bin/labdrian-overlay): a failed cd ends the run
  cd "$OVERLAY_DIR"

  local target_arg="all"
  local check_origin=false

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --target)
        shift
        target_arg="${1:-}"
        [[ -n "$target_arg" ]] || die "--target requires an argument"
        ;;
      --check-origin|--fetch)
        check_origin=true
        ;;
      *) die "Unknown option: $1" ;;
    esac
    shift || true
  done

  local repo_behind_origin
  repo_behind_origin="$(compute_repo_behind_origin "$check_origin")"

  local repo_behind_release
  repo_behind_release="$(compute_repo_behind_release "$check_origin")"

  # overlay-update-check/overlay-release-identity: the release `apply` would
  # currently deploy, resolved from local main (D1 fallback: "untagged").
  # Cached-only unless --check-origin/--fetch was passed (mirrors
  # compute_repo_behind_origin/compute_repo_behind_release, in release.sh) --
  # sync-check-verdicts R-001 and tui-self-update R-001 both require this
  # default path to invoke no `git fetch`.
  local release_version
  release_version="$(resolve_latest_release_tag main "$check_origin")"
  [[ "$release_version" == "none" ]] && release_version="untagged"

  # Set when the pi package target's sync-check is anything other than
  # IN_SYNC -- not built, drifted, or (for a future package target with no
  # real check yet) unsupported (R4-silent-package-skip): forces
  # sync-check's own aggregate exit non-zero below, so pi's incompleteness
  # cannot read as a clean, healthy report. Reachable both for an explicit
  # --target pi and for pi riding along inside --target all.
  local pi_stub_included=""

  for t in $(resolve_targets "$target_arg"); do
    if ! is_copy_target "$t"; then
      echo ""
      echo "=== sync-check: $t ==="
      if [[ "$t" == "pi" ]]; then
        pipkg_sync_check_and_report || pi_stub_included=1
      else
        # R2-sync-check-success-without-check: "partial" without a
        # VERDICT/SYNC_CHECK line a health check can key off read as "this
        # ran and found nothing wrong". "unsupported" plus an explicit
        # VERDICT line below make its incompleteness detectable the same
        # way every other target's VERDICT line is.
        echo "SYNC_CHECK:$t: unsupported -- package target without a lifecycle adapter"
        package_target_stub_message "$t"
        echo ""
        echo "VERDICT:$t:UNSUPPORTED"
        pi_stub_included=1
      fi
      continue
    fi

    local tpath="${TARGET_PATHS[$t]}"
    echo ""
    echo "=== sync-check: $t ($tpath) ==="

    if [[ ! -d "$tpath" ]]; then
      echo "SYNC_CHECK:$t: target dir not found at $tpath -- skipping"
      continue
    fi

    local upstream_changed=0
    local overlay_not_deployed=0

    local rr_repo_rel rr_dest
    manifest_routes_load "$t"
    while IFS=$'\t' read -r _ _ rr_repo_rel _ rr_dest; do
      # Skip the row if its route does not apply to this target
      [[ -z "$rr_dest" ]] && continue

      local live_file="$rr_dest"

      # Hash of live file (empty string if missing)
      local live_hash=""
      if [[ -f "$live_file" ]]; then
        live_hash="$(sha256sum "$live_file" | awk '{ print $1 }')"
      fi

      # Hash of main version in repo (CORRECTNESS-CRITICAL: use route subdir, not hardcoded skills/)
      local main_hash=""
      if git cat-file -e "main:${rr_repo_rel}" 2>/dev/null; then
        main_hash="$(git show "main:${rr_repo_rel}" 2>/dev/null | sha256sum | awk '{ print $1 }')"
      fi

      # PRIMARY check: live == main → IN SYNC, nothing to do
      if [[ -n "$live_hash" && "$live_hash" == "$main_hash" ]]; then
        echo "  IN_SYNC: ${rr_repo_rel}"
        continue
      fi

      # live != main (or live missing) — determine why
      if [[ -z "$live_hash" ]]; then
        # File missing at target
        overlay_not_deployed=$((overlay_not_deployed + 1))
        echo "  OVERLAY_NOT_DEPLOYED: ${rr_repo_rel} (live missing)"
        continue
      fi

      # Determine if this is a MANAGED file (exists on upstream branch)
      local is_managed=false
      if git cat-file -e "upstream:${rr_repo_rel}" 2>/dev/null; then
        is_managed=true
      fi

      if [[ "$is_managed" == true ]]; then
        # Compare live vs upstream to distinguish subtypes
        local upstream_hash
        upstream_hash="$(git show "upstream:${rr_repo_rel}" 2>/dev/null | sha256sum | awk '{ print $1 }')"

        if [[ "$live_hash" == "$upstream_hash" ]]; then
          # live == upstream but live != main → overlay not deployed (customization reverted)
          overlay_not_deployed=$((overlay_not_deployed + 1))
          echo "  OVERLAY_NOT_DEPLOYED: ${rr_repo_rel} (live matches upstream, customization not deployed)"
        else
          # live != upstream AND live != main → upstream has a new pristine version we haven't captured
          upstream_changed=$((upstream_changed + 1))
          echo "  UPSTREAM_CHANGED: ${rr_repo_rel}"
        fi
      else
        # CUSTOM file (no upstream counterpart): live != main → overlay not deployed
        overlay_not_deployed=$((overlay_not_deployed + 1))
        echo "  OVERLAY_NOT_DEPLOYED: ${rr_repo_rel} (custom file, not deployed)"
      fi

    done < <(manifest_routes_rows)

    # sync-check-verdicts R-007/R-008: aggregate per-target digest vs. the
    # recorded deploy (from `apply`), independent of the per-file
    # classification above.
    local target_digest recorded recorded_version recorded_digest digest_match
    target_digest="$(compute_target_digest "$t" live)"
    recorded="$(state_read_target "$t")"
    if [[ "$recorded" == "NEVER_DEPLOYED" ]]; then
      recorded_version="NA"
      digest_match="NA"
    else
      IFS=$'\t' read -r recorded_version recorded_digest _ <<< "$recorded"
      if [[ "$target_digest" == "$recorded_digest" ]]; then
        digest_match="yes"
      else
        digest_match="no"
      fi
    fi

    echo ""
    echo "VERDICT:$t:UPSTREAM_CHANGED=$upstream_changed OVERLAY_NOT_DEPLOYED=$overlay_not_deployed REPO_BEHIND_ORIGIN=$repo_behind_origin REPO_BEHIND_RELEASE=$repo_behind_release RECORDED_VERSION=$recorded_version DIGEST_MATCH=$digest_match"

    if [[ "$upstream_changed" -gt 0 ]]; then
      echo "ACTION:$t: gentle-ai sync detected: run 'labdrian capture --target $t' then 'labdrian apply' to re-merge your customizations"
    elif [[ "$overlay_not_deployed" -gt 0 ]]; then
      # R-007: name the release `apply` would bring only when there IS a
      # recorded version that has gone stale (digest_match=no) -- a
      # genuinely never-deployed target (digest_match=NA) gets no
      # fabricated version, and the pre-first-tag bootstrap (no real
      # version to name yet) is never papered over either.
      if [[ "$digest_match" == "no" && "$release_version" != "untagged" ]]; then
        echo "ACTION:$t: run 'labdrian apply --target $t' (release $release_version available)"
      else
        echo "ACTION:$t: run 'labdrian apply --target $t'"
      fi
    elif [[ "$repo_behind_origin" != "NA" && "$repo_behind_origin" -gt 0 ]]; then
      echo "ACTION:$t: run 'git pull' to update your local clone (behind origin/main by $repo_behind_origin commit(s))"
    elif [[ "$digest_match" == "yes" ]]; then
      # R-008: name the matching version when fully in sync.
      echo "ACTION:$t: in sync with gentle-ai (healthy) at $recorded_version"
    else
      echo "ACTION:$t: in sync with gentle-ai (healthy)"
    fi
  done

  # A run that included pi and only printed its stub reported nothing
  # about pi's own health, and must not exit 0 as if it had
  # (R4-silent-package-skip).
  [[ -n "$pi_stub_included" ]] && return 1
  return 0
}
