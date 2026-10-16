# shellcheck shell=bash
#
# status
#
# The status command: drift between the repository and each target.
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

cmd_status() {
  # shellcheck disable=SC2164  # errexit is on (set -e in bin/labdrian-overlay): a failed cd ends the run
  cd "$OVERLAY_DIR"

  local target_arg="all"

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --target)
        shift
        target_arg="${1:-}"
        [[ -n "$target_arg" ]] || die "--target requires an argument"
        ;;
      *) die "Unknown option: $1" ;;
    esac
    shift || true
  done

  echo "=== Branch ==="
  git rev-parse --abbrev-ref HEAD

  echo ""
  echo "=== Diff stat upstream..main ==="
  git diff --stat upstream..main || echo "(no diff)"

  # Set when the pi package target has nothing healthy to report --
  # not built, or built but drifted (R4-silent-package-skip): forces
  # status's own aggregate exit non-zero below, so pi's incompleteness
  # cannot read as a clean report. Reachable both for an explicit
  # --target pi and for pi riding along inside --target all.
  local pi_stub_included=""

  for t in $(resolve_targets "$target_arg"); do
    echo ""
    echo "=== Status: $t ==="

    if ! is_copy_target "$t"; then
      if [[ "$t" == "pi" ]]; then
        pipkg_status_and_report || pi_stub_included=1
      else
        package_target_stub_message "$t"
        pi_stub_included=1
      fi
      continue
    fi

    local rel_path rr_src rr_dest repo_file live_file
    manifest_routes_load "$t"
    while IFS=$'\t' read -r rel_path _ _ rr_src rr_dest; do
      # Skip if the route does not apply to this target
      [[ -z "$rr_dest" ]] && continue

      repo_file="$rr_src"
      live_file="$rr_dest"

      if [[ ! -f "$repo_file" ]]; then
        echo "  MISSING in repo: $rel_path"
      elif [[ ! -f "$live_file" ]]; then
        echo "  MISSING in target: $rel_path"
      else
        if diff --brief "$repo_file" "$live_file" > /dev/null 2>&1; then
          echo "  in sync  : $rel_path"
        else
          echo "  DIFFERS  : $rel_path"
        fi
      fi
    done < <(manifest_routes_rows)

    # overlay-release-state R-001/R-002, sync-check-verdicts R-008: report
    # this target's recorded release version against its live aggregate
    # digest, independent of the per-file drift lines above.
    local target_digest recorded recorded_version recorded_digest
    target_digest="$(compute_target_digest "$t" live)"
    recorded="$(state_read_target "$t")"
    if [[ "$recorded" == "NEVER_DEPLOYED" ]]; then
      echo "  version: never deployed"
    else
      IFS=$'\t' read -r recorded_version recorded_digest _ <<< "$recorded"
      if [[ "$target_digest" == "$recorded_digest" ]]; then
        echo "  version: in sync at $recorded_version"
      else
        echo "  version: $recorded_version (drift detected -- run 'labdrian apply --target $t')"
      fi
    fi
  done

  # Read-only, but not exempt: a run that included pi and only printed its
  # stub reported nothing about pi's own health, and must not exit 0 as if
  # it had (R4-silent-package-skip).
  [[ -n "$pi_stub_included" ]] && return 1
  return 0
}
