# shellcheck shell=bash
#
# release
#
# Release identity as git knows it: the newest published release tag, and how far local main
# is behind origin/main and behind that tag.
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

# resolve_latest_release_tag <ref> [do_fetch=true]
#
# R-003: by default (do_fetch=true, every caller except cmd_sync_check's
# read-only default path) explicitly fetches v*-matching tags from origin
# first (closing the gap where a plain branch fetch does not guarantee tag
# visibility), THEN resolves the highest semver-ordered ANNOTATED tag
# reachable from <ref>, using git's version-aware refname sort rather than a
# plain lexical one (v1.10.0 must outrank v1.9.0). Lightweight tags are
# skipped -- R-001 requires an annotated tag. Emits the literal token "none"
# when no qualifying tag exists (design decision D1's pre-first-tag
# bootstrap case), never an error -- callers decide how to degrade (D1:
# legacy convergence / "untagged" / "no releases published yet").
#
# Passing do_fetch=false resolves from whatever tags are already cached
# locally, without touching the network -- the same cached-only idiom
# compute_repo_behind_origin/compute_repo_behind_release use for their own
# default (non---check-origin) path. cmd_sync_check is the only caller that
# needs this: sync-check-verdicts R-001 and tui-self-update R-001 both
# require its default (no --check-origin/--fetch) path to invoke no
# `git fetch` at all, and every OTHER caller (cmd_apply, cmd_self_update,
# cmd_update, cmd_version) already requires or accepts a fresh fetch by its
# own spec/design, so they are left on the do_fetch=true default untouched.
resolve_latest_release_tag() {
  local ref="${1:-HEAD}"
  local do_fetch="${2:-true}"

  if [[ "$do_fetch" == true ]] && git remote get-url origin >/dev/null 2>&1; then
    # Bounded like every other network call of the script (compute_repo_behind_origin
    # and compute_repo_behind_release below, the main fetch of cmd_self_update in
    # self_update.sh): this function is now called from cmd_self_update, cmd_update, and cmd_sync_check's release_version line
    # (Slice 2), so a stalled connection here can no longer be a silent, unbounded hang.
    if command -v timeout >/dev/null 2>&1; then
      timeout 10 git -c http.lowSpeedLimit=1000 -c http.lowSpeedTime=10 \
        fetch origin '+refs/tags/v*:refs/tags/v*' >/dev/null 2>&1 || true
    else
      git -c http.lowSpeedLimit=1000 -c http.lowSpeedTime=10 \
        fetch origin '+refs/tags/v*:refs/tags/v*' >/dev/null 2>&1 || true
    fi
  fi

  local tag objtype
  while IFS=$'\t' read -r tag objtype; do
    [[ -z "$tag" ]] && continue
    if [[ "$objtype" == "tag" ]]; then
      echo "$tag"
      return
    fi
  done < <(git for-each-ref --merged "$ref" --sort=-v:refname \
             --format=$'%(refname:short)\t%(objecttype)' 'refs/tags/v*' 2>/dev/null)

  echo "none"
}

# Computes how many commits local main is behind this repo's own origin/main,
# repo-level (not target-scoped). Emits an integer count on stdout, or the
# literal token "NA" when the comparison is unavailable (no origin remote, no
# cached ref, or a requested live fetch failed).
#
# When check_origin is "true", runs `git fetch origin` first for a live
# comparison. Every new git invocation here is wrapped in the same
# `if <cmd> 2>/dev/null; then ... else ...NA...; fi` guard idiom cmd_sync_check
# already uses (sync_check.sh: `if git cat-file -e
# "main:${rr_repo_rel}" 2>/dev/null; then ...`), so a missing remote/ref or a
# failed fetch degrades to NA instead of aborting the script under
# `set -euo pipefail`.
#
# The fetch is best-effort time-bounded (wrapped in `timeout` when that
# coreutil is available -- not on stock macOS, so this degrades gracefully
# rather than being a hard new dependency) so a stalled network path (VPN
# half-up, black-holed proxy) cannot hang sync-check indefinitely; a
# `low-speed` git config guards the HTTPS-transport case the same way.
#
# Every NA-producing branch below writes a one-line explanation to stderr
# (not stdout, so it never contaminates the count/NA value captured by
# callers via command substitution) using the `SYNC_CHECK:(repo):` prefix --
# "(repo)" is a deliberate, documented repo-scope marker distinct from the
# `SYNC_CHECK:$t:` per-target prefix of cmd_sync_check (sync_check.sh; this
# comparison is repo-level, not target-scoped, and no real target name would
# fit here). The fetch-failure branch additionally includes the first line
# of git's own error text, so a security-relevant failure (e.g. a host-key
# mismatch or TLS verification error) is distinguishable from a benign
# offline failure instead of both collapsing into the same generic message.
compute_repo_behind_origin() {
  local check_origin="$1"

  if ! git remote get-url origin >/dev/null 2>&1; then
    echo "SYNC_CHECK:(repo): no 'origin' remote configured -- REPO_BEHIND_ORIGIN unavailable" >&2
    echo "NA"
    return
  fi

  if [[ "$check_origin" == true ]]; then
    local fetch_err="" fetch_ok=true
    if command -v timeout >/dev/null 2>&1; then
      fetch_err="$(timeout 10 git -c http.lowSpeedLimit=1000 -c http.lowSpeedTime=10 fetch origin 2>&1 >/dev/null)" || fetch_ok=false
    else
      fetch_err="$(git -c http.lowSpeedLimit=1000 -c http.lowSpeedTime=10 fetch origin 2>&1 >/dev/null)" || fetch_ok=false
    fi
    if [[ "$fetch_ok" == false ]]; then
      local fetch_err_line="${fetch_err%%$'\n'*}"
      echo "SYNC_CHECK:(repo): git fetch origin failed -- degrading REPO_BEHIND_ORIGIN to NA (${fetch_err_line:-no error output})" >&2
      echo "NA"
      return
    fi
  fi

  if ! git rev-parse --verify -q refs/remotes/origin/main >/dev/null 2>&1; then
    echo "SYNC_CHECK:(repo): no cached refs/remotes/origin/main (never fetched) -- REPO_BEHIND_ORIGIN unavailable" >&2
    echo "NA"
    return
  fi

  # Compared against local 'main', never the checked-out branch (HEAD). This
  # must track the exact ref self-update converges: self-update fast-forwards
  # ONLY main and always returns to whatever branch was current (D1-D3), so a
  # HEAD-relative comparison stays nonzero forever when the current branch
  # isn't main -- the TUI's "Actualizar repositorio" button would fast-forward
  # main for real but its own banner would never report convergence. This is
  # also the same ref cmd_sync_check's own per-file comparisons already use
  # ("main:${rr_repo_rel}" in sync_check.sh), so this makes the diagnostic agree with
  # both its producer (self-update) and its sibling check (per-file drift).
  if ! git rev-parse --verify -q refs/heads/main >/dev/null 2>&1; then
    echo "SYNC_CHECK:(repo): no local 'main' branch -- REPO_BEHIND_ORIGIN unavailable" >&2
    echo "NA"
    return
  fi

  local count
  if count="$(git rev-list main..origin/main --count 2>/dev/null)"; then
    echo "$count"
  else
    echo "SYNC_CHECK:(repo): git rev-list main..origin/main failed -- REPO_BEHIND_ORIGIN unavailable" >&2
    echo "NA"
  fi
}

# compute_repo_behind_release <check_origin>
#
# Design decision D2: how many commits local main is behind the newest
# LOCALLY-KNOWN annotated v*-tag reachable from this repo's cached
# refs/remotes/origin/main -- cached-only by default, exactly like its
# sibling compute_repo_behind_origin, so a plain 'sync-check'/'status' never
# hits the network. Pass check_origin=true (mirrors sync-check's
# --check-origin/--fetch flag) to explicitly refresh v*-tags from origin
# first, still without moving any local branch head.
#
# Filtering candidate tags with --merged origin/main (rather than every known
# tag) keeps this consistent with resolve_latest_release_tag's own
# reachability rule and avoids a rogue/unrelated-branch tag producing a
# nonsensical count. Emits the literal token "NA" whenever no cached
# origin/main ref, no local main branch, or no qualifying tag exists yet
# (D1's pre-first-tag bootstrap) -- never an error.
compute_repo_behind_release() {
  local check_origin="$1"

  if ! git remote get-url origin >/dev/null 2>&1; then
    echo "SYNC_CHECK:(repo): no 'origin' remote configured -- REPO_BEHIND_RELEASE unavailable" >&2
    echo "NA"
    return
  fi

  if [[ "$check_origin" == true ]]; then
    if command -v timeout >/dev/null 2>&1; then
      timeout 10 git -c http.lowSpeedLimit=1000 -c http.lowSpeedTime=10 \
        fetch origin '+refs/tags/v*:refs/tags/v*' >/dev/null 2>&1 || true
    else
      git -c http.lowSpeedLimit=1000 -c http.lowSpeedTime=10 \
        fetch origin '+refs/tags/v*:refs/tags/v*' >/dev/null 2>&1 || true
    fi
  fi

  if ! git rev-parse --verify -q refs/remotes/origin/main >/dev/null 2>&1; then
    echo "SYNC_CHECK:(repo): no cached refs/remotes/origin/main (never fetched) -- REPO_BEHIND_RELEASE unavailable" >&2
    echo "NA"
    return
  fi

  if ! git rev-parse --verify -q refs/heads/main >/dev/null 2>&1; then
    echo "SYNC_CHECK:(repo): no local 'main' branch -- REPO_BEHIND_RELEASE unavailable" >&2
    echo "NA"
    return
  fi

  local tag objtype latest_tag=""
  while IFS=$'\t' read -r tag objtype; do
    [[ -z "$tag" ]] && continue
    if [[ "$objtype" == "tag" ]]; then
      latest_tag="$tag"
      break
    fi
  done < <(git for-each-ref --merged origin/main --sort=-v:refname \
             --format=$'%(refname:short)\t%(objecttype)' 'refs/tags/v*' 2>/dev/null)

  if [[ -z "$latest_tag" ]]; then
    echo "SYNC_CHECK:(repo): no release tag known yet -- REPO_BEHIND_RELEASE unavailable" >&2
    echo "NA"
    return
  fi

  local count
  if count="$(git rev-list --count "main..${latest_tag}" 2>/dev/null)"; then
    echo "$count"
  else
    echo "SYNC_CHECK:(repo): git rev-list main..${latest_tag} failed -- REPO_BEHIND_RELEASE unavailable" >&2
    echo "NA"
  fi
}
