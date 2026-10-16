# shellcheck shell=bash
#
# apply
#
# The apply command: merge upstream into main and deploy to the targets, with a backup and a
# recorded state.
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

cmd_apply() {
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

  # Refuse to run over uncommitted tracked changes: apply switches branches
  # and merges, and must never entangle a session's work in progress.
  [[ -z "$(git status --porcelain --untracked-files=no)" ]] \
    || die "Working tree has uncommitted tracked changes. Commit or stash them before apply."

  local current_branch
  current_branch="$(git rev-parse --abbrev-ref HEAD)"

  # Always return to the original branch, even if a later step fails — apply
  # must never strand a session's checkout on 'main'. On merge conflicts git
  # refuses the checkout, so the trap degrades to staying on main to resolve.
  # Expand the branch name NOW: an EXIT trap fires after the function's locals
  # are gone, and referencing them there is an unbound variable under 'set -u'.
  trap "git checkout '${current_branch}' >/dev/null 2>&1 || true" EXIT

  git checkout main

  info "Merging upstream into main..."
  if ! git merge upstream; then
    echo ""
    echo "MERGE CONFLICTS detected. Resolve the following files, then run:"
    echo "  git -C \"$OVERLAY_DIR\" add <file>"
    echo "  git -C \"$OVERLAY_DIR\" commit"
    echo "  labdrian apply  (re-run to deploy after resolving)"
    echo ""
    echo "Conflicted files:"
    git diff --name-only --diff-filter=U
    exit 1
  fi

  # Resolved once (repo-level, not target-scoped): the release version this
  # apply is deploying. D1's pre-first-tag bootstrap records "untagged"
  # rather than fabricating or failing when no release tag exists yet; the
  # per-target digest recorded below is still real either way.
  local release_version
  release_version="$(resolve_latest_release_tag main)"
  [[ "$release_version" == "none" ]] && release_version="untagged"

  # D13: set when any manifest row processed below resolves to route=mcp.
  # cmd_longterm_mem install is invoked once, after the ENTIRE deploy loop
  # below (all targets), not once per target inside it — see the flag check
  # after the loop.
  local saw_mcp_row=""

  # Set when the pi package target fails to build (R4-silent-package-skip):
  # forces apply's own aggregate exit non-zero below, so unattended
  # automation cannot read a run that failed to build pi (or skipped it via
  # a future package-target stub) as a clean success. Reachable both for an
  # explicit --target pi and for pi riding along inside --target all.
  local pi_stub_included=""

  for t in $(resolve_targets "$target_arg"); do
    echo ""
    echo "=== Deploying to: $t ==="

    if ! is_copy_target "$t"; then
      if [[ "$t" == "pi" ]]; then
        pipkg_apply_and_report || pi_stub_included=1
      else
        package_target_stub_message "$t"
        pi_stub_included=1
      fi
      continue
    fi

    local tpath="${TARGET_PATHS[$t]}"

    if [[ ! -d "$tpath" ]]; then
      mkdir -p "$tpath"
      echo "  (created target directory: $tpath)"
    fi

    # overlay-backup-restore R-001: snapshot this target's currently
    # deployed managed files BEFORE any of them are overwritten below.
    # backup_target itself no-ops (creates nothing, prints nothing) when
    # nothing would actually change for this target.
    local backup_ts
    backup_ts="$(backup_target "$t")"
    if [[ -n "$backup_ts" ]]; then
      echo "  backup: $backup_ts"
    fi

    local deployed=0
    local in_sync=0

    local rr_route rr_repo_rel rr_src rr_dest src dest
    manifest_routes_load "$t"
    while IFS=$'\t' read -r _ rr_route rr_repo_rel rr_src rr_dest; do
      # D13: mcp-routed rows have no destination and would otherwise be
      # silently skipped by the "[[ -z "$rr_dest" ]] && continue" guard
      # below with no trace of ever having been seen — record the sighting
      # here, before that guard, so the install hook after the deploy loop
      # knows whether to fire (the same pass that refuses, in the engine, any
      # unrouted/unrecognized longterm-mem/** row, task 9.11).
      if [[ "$rr_route" == "mcp" ]]; then
        saw_mcp_row=1
      fi
      # Skip if the row does not go to the current target ($t)
      [[ -z "$rr_dest" ]] && continue

      src="$rr_src"
      dest="$rr_dest"
      ensure_parent "$dest"

      if [[ -f "$src" ]]; then
        if [[ -f "$dest" ]] && diff --brief "$src" "$dest" > /dev/null 2>&1; then
          in_sync=$((in_sync + 1))
        else
          cp "$src" "$dest"
          deployed=$((deployed + 1))
        fi
      else
        echo "  WARNING: ${rr_repo_rel} not found in overlay, skipping"
      fi
    done < <(manifest_routes_rows)

    echo "  $t: deployed=$deployed in-sync=$in_sync"

    # overlay-release-state R-001: persist this target's deployed version
    # and content digest so a target with no entry can be reported honestly
    # as "never deployed" rather than staying silently unversioned forever.
    local target_digest
    target_digest="$(compute_target_digest "$t" live)"
    state_write_target "$t" "$release_version" "$target_digest"
  done

  # D13: run the longterm-mem install path exactly once for this whole
  # apply invocation — never once per deploy target — passing through
  # apply's own --target selection unchanged.
  #
  # Its exit status is inspected, because this runs AFTER the loop above has
  # already committed each target's state row. An unchecked failure would
  # leave apply reporting success with no MCP server installed, and — since
  # the digests it just wrote are unchanged — a later apply would see
  # nothing to do and never retry. That degraded state would be sticky
  # rather than self-healing, so the failure is named loudly, the exact
  # recovery command is printed, and apply itself exits non-zero.
  #
  # The step goes through longtermmem_install_step rather than calling
  # cmd_longterm_mem here: see that function for why a bare call in this
  # condition context could not observe the failures it most needed to.
  local mcp_install_failed=""
  if [[ -n "$saw_mcp_row" ]]; then
    echo ""
    info "longterm-mem: mcp-routed row detected — running install once (D13)"
    if ! longtermmem_install_step "$target_arg"; then
      mcp_install_failed=1
    fi
  fi

  echo ""
  echo "Deploy complete."

  git checkout "$current_branch"
  trap - EXIT
  info "Returned to branch: $current_branch"

  # main is pushed immediately after the merge — same reasoning as capture's
  # push of 'upstream': an apply that only lands locally is exactly what lets
  # main silently diverge from origin the moment anything else lands there
  # (a PR merge, another machine's apply) before this one gets pushed.
  # A repo with no 'origin' remote (tests, detached mirrors) has nowhere to
  # push: skip loudly instead of failing the apply that already merged.
  if ! git remote get-url origin >/dev/null 2>&1; then
    info "No 'origin' remote — skipping main push"
  else
    local ahead
    ahead="$(git rev-list --count origin/main..main 2>/dev/null || echo 0)"
    if [[ "$ahead" -gt 0 ]]; then
      git push origin main
      info "Pushed main to origin"
    fi
  fi

  # Deploy itself succeeded, but a failed mcp install must not exit 0
  # (R4-silent-package-skip): neither may a run that included pi and only
  # printed its stub, with nothing actually deployed for it.
  [[ -n "$mcp_install_failed" || -n "$pi_stub_included" ]] && return 1
  return 0
}
