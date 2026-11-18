# shellcheck shell=bash
#
# apply
#
# The apply command: merge upstream into main and deploy to the targets, with a backup and a
# recorded state.
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

# indent_lines copies stdin with two spaces before every line, the way apply shows a step.
indent_lines() {
  local line
  while IFS= read -r line; do
    printf '  %s\n' "$line"
  done
}

# runtime_install_and_report TARGET runs `engine runtime install --target TARGET` over this checkout, the
# step `apply --target pi` has in pipkg_apply_and_report, for the two copy targets whose runtime adapter
# writes more than a copy of the manifest rows reaches. codex: the adapter keeps the owner rules
# (instructions/owner-rules.md) in one marked block of ~/AGENTS.md, $CODEX_HOME/AGENTS.md and
# $CODEX_HOME/agents.md (CODEX_HOME defaults to ~/.codex). opencode: it writes the plugins, opencode.json
# (the default agent and the MCP servers) and the AGENTS.md block, which longterm-mem register then
# reads (opencode.json must exist). Two cases have nothing to run and do not fail the deploy: a
# checkout without the source its adapter reads (instructions/owner-rules.md for codex,
# skills/_shared/minimalism-contract.md for opencode: an older main; silent, so its output is what it
# was), and an engine binary that is not installed yet (the note names the recovery). `apply` never
# reaches that note: manifest_routes_load calls ensure_engine_binary, which builds or rebuilds the binary
# before this step; the note guards a direct call of this function.
# Non-zero when the engine ran and did not report an install that took effect.
runtime_install_and_report() {
  local target="$1" what="runtime" source_file
  case "$target" in
    codex)
      source_file="instructions/owner-rules.md"
      what="owner rules"
      ;;
    opencode) source_file="skills/_shared/minimalism-contract.md" ;;
    *) return 1 ;;
  esac
  [[ -f "$OVERLAY_DIR/$source_file" ]] || return 0
  if [[ ! -x "$ENGINE_BINARY" ]]; then
    echo "  $target: engine binary not found at $ENGINE_BINARY; $what not installed (run 'labdrian install-hooks', then 'labdrian apply --target $target')"
    return 0
  fi
  local result
  if ! result="$(LABDRIAN_OVERLAY_DIR="$OVERLAY_DIR" "$ENGINE_BINARY" runtime install --target "$target" 2>&1)"; then
    indent_lines <<<"$result" >&2
    return 1
  fi
  indent_lines <<<"$result"
}

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

  # Set when `engine runtime install --target codex|opencode` ran and failed: apply still deploys every
  # target, then exits non-zero, so a block that was not written never reads as a clean apply.
  local runtime_install_failed=""
  # Targets the longterm-mem step leaves alone because their runtime install failed (see the deploy loop).
  LONGTERM_MEM_SKIP_TARGETS=""

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

    if copies_skills "$t" && [[ ! -d "$tpath" ]]; then
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
    # What the OpenCode runtime adapter writes (the plugins, opencode.json and the AGENTS.md block) is not
    # a manifest row either, and without it the target is not deployed: it runs before the state row and
    # before the once-per-apply longterm-mem step below, which reads opencode.json. A failure is shown
    # once, leaves no state row (status keeps saying the target was never deployed; the step runs again
    # on every apply), keeps longterm-mem from registering into the file the install could not write, and
    # fails apply's own exit below.
    if [[ "$t" == "opencode" ]] && ! runtime_install_and_report opencode; then
      runtime_install_failed=1
      LONGTERM_MEM_SKIP_TARGETS+=" opencode"
      echo "  opencode: runtime install failed; target state not recorded (re-run apply once the cause is fixed)" >&2
      # An entry an earlier apply recorded would still claim the old deploy.
      state_forget_target opencode
      continue
    fi

    local target_digest
    target_digest="$(compute_target_digest "$t" live)"
    state_write_target "$t" "$release_version" "$target_digest"

    # The owner rules block of the three Codex AGENTS files is not a manifest row: the Codex runtime
    # adapter writes it, after the state row. A failure is reported like pi's and fails apply's own exit
    # below.
    if [[ "$t" == "codex" ]]; then
      runtime_install_and_report codex || runtime_install_failed=1
    fi
  done

  # D13: run the longterm-mem install path exactly once for this whole
  # apply invocation — never once per deploy target — passing through
  # apply's own --target selection unchanged.
  #
  # Its exit status is inspected, because this runs AFTER the loop above has
  # already committed each target's state row. An unchecked failure would
  # leave apply reporting success with no MCP server installed. The step is
  # not tied to those digests: saw_mcp_row is set on every apply that has an
  # mcp-routed row, so the next apply runs the install again. The failure
  # is still named loudly, the exact recovery command is printed, and apply
  # itself exits non-zero, so the owner sees it without waiting for the next
  # apply.
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

  push_main_to_origin

  # Deploy itself succeeded, but a failed mcp install must not exit 0
  # (R4-silent-package-skip): neither may a run that included pi and only
  # printed its stub, with nothing actually deployed for it.
  [[ -n "$mcp_install_failed" || -n "$pi_stub_included" || -n "$runtime_install_failed" ]] && return 1
  return 0
}

# main is pushed immediately after the merge — same reasoning as capture's
# push of 'upstream': an apply that only lands locally is exactly what lets
# main silently diverge from origin the moment anything else lands there
# (a PR merge, another machine's apply) before this one gets pushed.
# A repo with no 'origin' remote (tests, detached mirrors) has nowhere to
# push: skip loudly instead of failing the apply that already merged.
#
# Only a network remote is pushed to. An origin that is a path on this machine (a scratch clone made
# from the real checkout, a mirror) is not where main is published: pushing there landed unmerged
# commits in the real repository's local main. It is skipped loudly too.
push_main_to_origin() {
  if ! git remote get-url origin >/dev/null 2>&1; then
    info "No 'origin' remote — skipping main push"
  elif ! origin_is_network_remote; then
    info "The 'origin' remote is on this machine (origin is not a network remote) — skipping main push"
  else
    local ahead
    ahead="$(git rev-list --count origin/main..main 2>/dev/null || echo 0)"
    if [[ "$ahead" -gt 0 ]]; then
      git push origin main
      info "Pushed main to origin"
    fi
  fi
}
