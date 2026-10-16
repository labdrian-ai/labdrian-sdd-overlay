# shellcheck shell=bash
#
# self_update
#
# The self-update command: fast-forward local main to the latest release without leaving the
# branch the caller is on.
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

# selfupdate_converge_main <target_commit> [<label suffix>]
#
# Fast-forwards local main to <target_commit> WITHOUT ever leaving the branch
# the operator is standing on. This replaces the checkout/trap/checkout-back
# dance both self-update modes used to carry byte-for-byte, which had three
# compounding defects:
#
#   1. the restoring EXIT trap swallowed its own failure ('>/dev/null 2>&1 ||
#      true'), so a failed return left the operator on main with nothing said
#      -- they kept working and committed onto main, which is the divergence
#      reported from real use;
#   2. 'git checkout main' fails for reasons that are not the operator's --
#      most obviously when main is checked out in another worktree, an
#      ordinary layout and this repository's own -- and under 'set -e' that
#      abort handed control straight to the silent trap above;
#   3. it was duplicated, so any fix had to land twice and the copies could
#      drift.
#
# On main we fast-forward in place with merge --ff-only, exactly as before:
# no checkout, no trap, nothing to restore. Off main we move the REF with
# 'git fetch . <commit>:refs/heads/main', which fast-forwards a local branch
# from this same repository and REFUSES a non-fast-forward (the same
# guarantee merge --ff-only gives, so R-006's spirit survives the rewrite)
# and refuses outright when main is checked out in another worktree. That
# refusal is the correct outcome and is reported, never silenced. The
# working tree and HEAD are untouched, so there is nothing to put back and
# no window in which an interruption can strand the operator.
#
# Finally it VERIFIES that main actually landed on the intended commit and
# names the branch the operator is on -- the fact that was going wrong and
# that the old success line never reported.
selfupdate_converge_main() {
  local target_commit="$1"
  local suffix="${2:-}"

  local current_branch old_main new_main
  current_branch="$(git rev-parse --abbrev-ref HEAD)"
  old_main="$(git rev-parse main)"

  if [[ "$current_branch" == "main" ]]; then
    git merge --ff-only "$target_commit" \
      || die "main could not be fast-forwarded to ${target_commit:0:12}. Nothing was changed; you are still on 'main'."
  else
    git fetch . "${target_commit}:refs/heads/main" \
      || die "main could not be fast-forwarded to ${target_commit:0:12}: either that is not a fast-forward, or main is checked out in another worktree. Nothing was changed; you are still on '$current_branch'."
  fi

  # Read main back and insist it is where it was sent. This is DEFENSIVE and
  # carries no test on purpose: both writers above already fail loudly on a
  # non-fast-forward, so reaching this line with the wrong value would mean
  # git reported success and did not move the ref. Provoking that means
  # breaking git itself, and a test that stubs git into lying would pin the
  # stub, not the guard. It is recorded as untested rather than covered by a
  # fabricated case -- the assertion costs one rev-parse, and the alternative
  # is trusting a silent success.
  new_main="$(git rev-parse main)"
  [[ "$new_main" == "$target_commit" ]] \
    || die "main is at ${new_main:0:12}, not the expected ${target_commit:0:12}; self-update did not converge. Nothing else was changed; you are still on '$current_branch'."

  info "Updated main: ${old_main:0:12}..${new_main:0:12} (fast-forward)${suffix}"
  info "main is now at ${new_main:0:12}; you are on '$current_branch' (working tree untouched)"
}

# cmd_self_update fast-forwards ONLY the local 'main' branch, and NEVER changes the branch
# the caller is standing on -- see selfupdate_converge_main for how.
#
# This used to check main out and restore the caller's branch through an EXIT
# trap. The restore was silenced with `|| true`, so when it failed the
# operator was left standing on main with nothing saying so; they kept working
# and committed there. That is the divergence this was reported for. The
# checkout is gone: off main the ref is moved on its own, so there is nothing
# to restore and no window in which an interruption can strand anyone.
#
# Every refusal (no origin, dirty tree, local-ahead divergence) still fires
# BEFORE anything is fetched or moved, and every refusal now names the branch
# the caller is on. Flagless: this signal is repo-level, not target-scoped.
#
# R-004 (MODIFIED): once at least one release tag exists anywhere, main
# converges to the highest semver-ordered release tag reachable from
# origin/main -- NEVER to origin/main's raw HEAD, which may carry untagged
# commits beyond the latest tag. D1's pre-first-tag bootstrap keeps the
# original origin/main-HEAD convergence verbatim (legacy path below) until
# the very first release tag is cut, so every zero-tag caller keeps its
# exact documented behavior.
cmd_self_update() {
  # shellcheck disable=SC2164  # errexit is on (set -e in bin/labdrian-overlay): a failed cd ends the run
  cd "$OVERLAY_DIR"

  [[ $# -eq 0 ]] || die "Unknown option: $1"

  # 1. No 'origin' remote -> nothing to converge against.
  git remote get-url origin >/dev/null 2>&1 \
    || die "self-update requires an 'origin' remote; none is configured."

  # 2. Dirty tracked tree -> hard refusal, before any checkout, fetch, or tag
  #    resolution. Untracked files are ignored (capture/apply parity).
  [[ -z "$(git status --porcelain --untracked-files=no)" ]] \
    || die "Working tree has uncommitted tracked changes. Commit or stash them before self-update."

  # 3. Bounded fetch of main only (idiom from compute_repo_behind_origin).
  #    'timeout' is optional -- not on stock macOS -- so this degrades to an
  #    unbounded (but still low-speed-guarded) fetch rather than a hard dep.
  #    Also refreshes the ref resolve_latest_release_tag (release.sh) resolves
  #    tags against.
  if command -v timeout >/dev/null 2>&1; then
    timeout 10 git -c http.lowSpeedLimit=1000 -c http.lowSpeedTime=10 fetch origin main \
      || die "git fetch origin main failed."
  else
    git -c http.lowSpeedLimit=1000 -c http.lowSpeedTime=10 fetch origin main \
      || die "git fetch origin main failed."
  fi

  # 4. Local main ahead of origin/main -> hard refusal (R-006), unaffected by
  #    tag mode: self-update converges main, it never silently diverges
  #    further from a local-only commit that only exists on this clone.
  local ahead
  ahead="$(git rev-list --count origin/main..main)"
  [[ "$ahead" -eq 0 ]] \
    || die "Local main is $ahead commit(s) ahead of origin/main; self-update refuses to diverge further. Resolve manually (e.g. 'git -C \"$OVERLAY_DIR\" push origin main')."

  # 5. R-003/D1: resolve the highest release tag reachable from origin/main.
  #    "none" means no release tag exists anywhere yet -- fall back verbatim
  #    to the original origin/main-HEAD convergence (D1 pre-first-tag
  #    bootstrap).
  local release_tag
  release_tag="$(resolve_latest_release_tag origin/main)"

  if [[ "$release_tag" == "none" ]]; then
    info "no release tags yet -- legacy convergence"

    local behind
    behind="$(git rev-list --count main..origin/main)"
    if [[ "$behind" -eq 0 ]]; then
      info "main is already up to date with origin/main"
      return
    fi

    selfupdate_converge_main "$(git rev-parse 'origin/main^{commit}')"
    return
  fi

  # 6. Tag mode: converge main to the resolved tag's commit only -- NEVER
  #    past it, even when origin/main's raw HEAD carries untagged commits
  #    beyond this tag. merge-base --is-ancestor also covers "main is
  #    already AHEAD of this tag" (e.g. a prior self-update already landed
  #    it) as a no-op, not only exact equality.
  local target_commit main_head
  target_commit="$(git rev-parse "${release_tag}^{commit}")"
  main_head="$(git rev-parse main)"

  if git merge-base --is-ancestor "$target_commit" "$main_head"; then
    info "main is already up to date with $release_tag"
    return
  fi

  selfupdate_converge_main "$target_commit" " -> $release_tag"
}
