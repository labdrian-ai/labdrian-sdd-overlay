# shellcheck shell=bash
#
# release_report
#
# The update and version commands: the latest published release, this clone's version, and
# what each target has recorded.
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

# Read-only update-availability check (overlay-update-check R-001, design
# D6). Refreshes ONLY refs/tags/*/refs/remotes/* -- a bounded 'git fetch
# origin main' plus resolve_latest_release_tag's own explicit tag fetch --
# and NEVER touches a local branch head, the working tree, target files, or
# the state file. A fetch is byte-idempotent absent a remote change, so
# running this twice in a row leaves every git ref, target file, and
# state-file byte identical (R-001's zero-mutation scenario).
cmd_update() {
  # shellcheck disable=SC2164  # errexit is on (set -e in bin/labdrian-overlay): a failed cd ends the run
  cd "$OVERLAY_DIR"

  [[ $# -eq 0 ]] || die "Unknown option: $1"

  if git remote get-url origin >/dev/null 2>&1; then
    if command -v timeout >/dev/null 2>&1; then
      timeout 10 git -c http.lowSpeedLimit=1000 -c http.lowSpeedTime=10 fetch origin main \
        >/dev/null 2>&1 || warn "git fetch origin main failed -- reporting from cached refs"
    else
      git -c http.lowSpeedLimit=1000 -c http.lowSpeedTime=10 fetch origin main \
        >/dev/null 2>&1 || warn "git fetch origin main failed -- reporting from cached refs"
    fi
  else
    warn "no 'origin' remote configured -- reporting from cached refs only"
  fi

  local latest_ref="main"
  git rev-parse --verify -q refs/remotes/origin/main >/dev/null 2>&1 && latest_ref="origin/main"

  local release_version
  release_version="$(resolve_latest_release_tag "$latest_ref")"

  if [[ "$release_version" == "none" ]]; then
    echo "Latest release: (no releases published yet)"
  else
    echo "Latest release: $release_version"
  fi

  for t in $(resolve_targets all); do
    local recorded recorded_version
    recorded="$(state_read_target "$t")"

    if [[ "$recorded" == "NEVER_DEPLOYED" ]]; then
      echo "  $t: never deployed"
      continue
    fi

    IFS=$'\t' read -r recorded_version _ <<< "$recorded"

    if [[ "$release_version" == "none" ]]; then
      echo "  $t: $recorded_version (no releases published yet)"
    elif [[ "$recorded_version" == "$release_version" ]]; then
      echo "  $t: up-to-date ($recorded_version)"
    else
      echo "  $t: behind ($release_version available, have $recorded_version)"
    fi
  done
}

# cmd_version (overlay-release-surfacing R-002; also reachable via the
# --version flag, see the dispatch in bin/labdrian-overlay): reports this clone's own current
# release version (resolved from local 'main', matching what 'apply' would
# deploy right now -- unlike 'update', which checks the newest PUBLISHED
# release on origin) and, per target, its recorded deployed version. A
# target with no recorded state is reported as "never deployed" rather than
# a stale or fabricated version (R-002).
cmd_version() {
  # shellcheck disable=SC2164  # errexit is on (set -e in bin/labdrian-overlay): a failed cd ends the run
  cd "$OVERLAY_DIR"

  [[ $# -eq 0 ]] || die "Unknown option: $1"

  local repo_version
  repo_version="$(resolve_latest_release_tag main)"

  if [[ "$repo_version" == "none" ]]; then
    echo "Repository version: (no releases published yet)"
  else
    echo "Repository version: $repo_version"
  fi

  for t in $(resolve_targets all); do
    local recorded recorded_version
    recorded="$(state_read_target "$t")"

    if [[ "$recorded" == "NEVER_DEPLOYED" ]]; then
      echo "  $t: never deployed"
      continue
    fi

    IFS=$'\t' read -r recorded_version _ <<< "$recorded"

    if [[ "$repo_version" != "none" && "$recorded_version" != "$repo_version" ]]; then
      echo "  $t: $recorded_version (behind)"
    else
      echo "  $t: $recorded_version"
    fi
  done
}
