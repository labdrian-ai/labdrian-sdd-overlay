# shellcheck shell=bash
# shellcheck disable=SC2164  # errexit is on (set -e in bin/labdrian-overlay): a failed cd ends the run
#
# capture
#
# The capture command: copy the managed files of a target (or of a snapshot) onto the upstream
# branch and push it.
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

cmd_capture() {
  cd "$OVERLAY_DIR"

  local from_backup=""
  local target_arg="claude"

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --target)
        shift
        target_arg="${1:-}"
        [[ -n "$target_arg" ]] || die "--target requires an argument"
        ;;
      --from-backup)
        shift
        from_backup="${1:-}"
        [[ -n "$from_backup" ]] || die "--from-backup requires a path argument"
        ;;
      *) die "Unknown option: $1" ;;
    esac
    shift || true
  done

  # Validate single target (capture only supports one at a time)
  if [[ "$target_arg" == "all" ]]; then
    die "capture does not support --target all. Specify a single target: claude, opencode, codex"
  fi
  is_valid_target "$target_arg" || die "Unknown target: $target_arg. Valid: claude, opencode, codex, pi"
  is_copy_target "$target_arg" \
    || die "capture does not support --target $target_arg (package target, no per-file copy source); capture only supports claude, opencode, codex"

  local target_dir="${TARGET_PATHS[$target_arg]}"

  # Refuse to run over uncommitted tracked changes: capture switches branches
  # and must never mix a dirty working tree into the sync commit.
  [[ -z "$(git status --porcelain --untracked-files=no)" ]] \
    || die "Working tree has uncommitted tracked changes. Commit or stash them before capture."

  local current_branch
  current_branch="$(git rev-parse --abbrev-ref HEAD)"

  # The engine says where each row goes (manifest_routes_load), and 'upstream',
  # which is checked out below, holds no engine source to build it from: it has
  # to be there, or be made, before this script leaves the branch it is on.
  ensure_engine_binary >&2

  # Always return to the original branch, even if a later step fails under
  # 'set -euo pipefail' — prevents stranding the working tree on 'upstream'.
  # Expand the branch name NOW: an EXIT trap fires after the function's locals
  # are gone, and referencing them there is an unbound variable under 'set -u'.
  trap "git checkout '${current_branch}' >/dev/null 2>&1 || true" EXIT

  git checkout upstream

  # Reconcile with origin before capturing. 'upstream' is a mechanical
  # mirror pushed immediately after every capture (see below), so it should
  # normally already match origin/upstream — but if origin/upstream advanced
  # independently since this clone's last fetch (another machine captured
  # and pushed, or a manual reconcile merge landed there), a stale local ref
  # makes the push below either get rejected non-fast-forward (crashing the
  # whole capture under 'set -euo pipefail' despite the local capture
  # already having succeeded) or, worse, silently skip the push entirely
  # (the "anything to push?" check below reads the same stale cached
  # origin/upstream). Fetch first so that check is accurate, then
  # fast-forward when possible; when local ALSO carries unpushed commits,
  # merge non-interactively rather than discarding them — capture's own
  # comment below already promises a sync never strands local-only commits.
  # A fetch failure (offline) degrades gracefully: capture's core job is the
  # local commit, not the network round trip, so warn and proceed rather
  # than dying (parity with the "no origin remote" skip a few lines down).
  if git remote get-url origin >/dev/null 2>&1; then
    if git fetch origin upstream >/dev/null 2>&1; then
      if git rev-parse --verify -q origin/upstream >/dev/null; then
        if ! git merge --ff-only origin/upstream >/dev/null 2>&1; then
          git merge --no-edit origin/upstream \
            || die "'upstream' diverged from origin/upstream and auto-merge conflicted; resolve manually on the 'upstream' branch, then retry capture."
          info "Reconciled local 'upstream' with origin/upstream before capturing"
        fi
      fi
    else
      warn "git fetch origin upstream failed — proceeding without reconciling; push below may fail if origin has advanced"
    fi
  fi

  if [[ -n "$from_backup" ]]; then
    info "Capturing managed files from tarball: $from_backup"
    [[ -f "$from_backup" ]] || die "Tarball not found: $from_backup"

    local cap_listing
    cap_listing="$(tar tzf "$from_backup" 2>/dev/null)"

    local rr_route rr_repo_rel rr_src
    manifest_routes_load "$target_arg" managed
    while IFS=$'\t' read -r _ rr_route rr_repo_rel rr_src _; do
      tar_path="$(route_tar_path "$rr_route" "$rr_repo_rel")"
      dest="$rr_src"
      ensure_parent "$dest"

      if grep -qF "$tar_path" <<< "$cap_listing"; then
        tar xzf "$from_backup" --to-stdout "$tar_path" > "$dest"
        info "  extracted: ${rr_repo_rel}"
      else
        echo "  WARNING: not found in tarball, skipping: $tar_path"
      fi
    done < <(manifest_routes_rows)
  else
    info "Capturing managed files from $target_dir"
    local rel_path rr_src rr_dest
    manifest_routes_load "$target_arg" managed
    while IFS=$'\t' read -r rel_path _ _ rr_src rr_dest; do
      # Live path for the requested target; skip if not applicable (e.g. native agent + codex)
      [[ -z "$rr_dest" ]] && continue

      src="$rr_dest"
      dest="$rr_src"
      ensure_parent "$dest"

      if [[ -f "$src" ]]; then
        cp "$src" "$dest"
        info "  copied: $rel_path"
      else
        echo "  WARNING: source file not found, skipping: $src"
      fi
    done < <(manifest_routes_rows)
  fi

  git add skills/
  test -d "$OVERLAY_DIR/agents" && git add agents/ || true
  test -d "$OVERLAY_DIR/opencode" && git add opencode/ || true
  # Commit only if deployable tracked files have STAGED changes. `git diff --cached --quiet`
  # ignores unrelated untracked paths (.engram/, tui/) present on 'upstream'
  # that previously caused a doomed `git commit` (exit 1) stranding the tree.
  if git diff --cached --quiet; then
    info "No changes detected — upstream already up to date"
  else
    git commit -m "upstream: sync from gentle-ai $(date +%Y-%m-%d)"
    info "upstream updated"
  fi

  # 'upstream' is a mechanical mirror of gentle-ai — push it immediately so a
  # sync never strands local-only commits (also flushes earlier unpushed runs).
  # A repo with no 'origin' remote (tests, detached mirrors) has nowhere to
  # push: skip loudly instead of failing the capture that already committed.
  if ! git remote get-url origin >/dev/null 2>&1; then
    info "No 'origin' remote — skipping upstream push"
  elif git rev-parse --verify -q origin/upstream >/dev/null; then
    if [[ -n "$(git rev-list origin/upstream..upstream)" ]]; then
      git push origin upstream
      info "Pushed upstream to origin"
    fi
  else
    git push -u origin upstream
    info "Pushed upstream to origin (new remote branch)"
  fi

  git checkout "$current_branch"
  trap - EXIT
  info "Returned to branch: $current_branch"
}
