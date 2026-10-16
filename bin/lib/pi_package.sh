# shellcheck shell=bash
#
# pi_package
#
# The pi package target: building, installing, checking and reporting the labdrian-pi package
# (a package target, not a per-file copy target).
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

# pipkg_dest_dir prints the built labdrian-pi package directory
# ($STATE_DIR/pi/labdrian-pi), the single source of truth every pipkg function
# resolves it from, and through them apply, status and sync-check.
pipkg_dest_dir() {
  echo "$STATE_DIR/pi/labdrian-pi"
}

# pipkg_build_and_report runs `engine pipkg build` for the pi target and
# prints the install hint on success. `pi install` itself is NOT run here
# (R-002 scope boundary; deferred to a manual step / a later slice). Returns
# non-zero on a build failure (R4-silent-package-skip): a failed pi package
# build must never read as a clean apply.
pipkg_build_and_report() {
  local dest
  dest="$(pipkg_dest_dir)"
  if "$ENGINE_BINARY" pipkg build --overlay-root "$OVERLAY_DIR" --registry "$OVERLAY_DIR/skills.registry.yaml" --dest-dir "$dest"; then
    echo "  install hint: pi install $dest"
    return 0
  else
    echo "  pi: package build failed" >&2
    return 1
  fi
}

# pipkg_apply_and_report builds (pipkg_build_and_report), then runs the
# real `pi install` via PiAdapter when `pi` is on PATH; the install hint
# already covers the "pi not on PATH" case. Non-zero on a build failure or
# a `pi` that IS on PATH but whose install failed (R4-silent-package-skip).
pipkg_apply_and_report() {
  pipkg_build_and_report || return 1
  command -v pi >/dev/null 2>&1 || return 0
  local result
  if ! result="$(OVERLAY_DIR="$OVERLAY_DIR" STATE_DIR="$STATE_DIR" "$ENGINE_BINARY" runtime install --target pi 2>&1)"; then
    echo "  $result" >&2
    return 1
  fi
  echo "  $result"
}

# pipkg_sync_check_and_report runs `engine pipkg check` for the pi target,
# emitting the SYNC_CHECK:pi: line other sync-check callers grep for, plus
# an explicit VERDICT:pi:<STATE> line (R2-sync-check-success-without-check)
# so a health check can key off pi the same way it does every copy target.
# `engine pipkg check` always prints a "pipkg check: compared against ..."
# basis-disclosure line on stdout (R-006/R-007) before its OK/drift verdict;
# that line is preserved verbatim in $drift_output and echoed in both the
# no-drift and drift branches below, so this command always discloses
# whether it compared against the recorded builtFrom ref, a main fallback,
# or the plain worktree -- never just silently "no drift".
# Returns non-zero for anything other than IN_SYNC (R4-silent-package-skip):
# a not-built or drifted pi package must never read as a healthy sync-check.
pipkg_sync_check_and_report() {
  local dest drift_output
  dest="$(pipkg_dest_dir)"
  if [[ ! -e "$dest/package.json" ]]; then
    echo "SYNC_CHECK:pi: not built (run: labdrian-overlay apply --target pi)"
    echo ""
    echo "VERDICT:pi:NOT_BUILT"
    return 1
  fi
  if drift_output="$("$ENGINE_BINARY" pipkg check --overlay-root "$OVERLAY_DIR" --registry "$OVERLAY_DIR/skills.registry.yaml" --dest-dir "$dest" 2>&1)"; then
    echo "SYNC_CHECK:pi: no drift"
    echo "$drift_output"
    echo ""
    echo "VERDICT:pi:IN_SYNC"
    return 0
  else
    echo "SYNC_CHECK:pi: drift -- $drift_output"
    echo ""
    echo "VERDICT:pi:DRIFT"
    return 1
  fi
}

# pipkg_status_and_report prints the pi package's status for cmd_status.
# The "not built" case is decided in pure bash (no ENGINE_BINARY call): it
# must stay reachable even when the engine binary has not been built yet
# (see TestOverlayStatusAndSyncCheck_PiStubNoEmptyMkdir). Once the package
# IS built, it delegates to `engine runtime status --target pi` -- the
# same PiAdapter.Status() the "runtime" subcommand uses -- for the honest
# three-entry proof (built+in-sync, listed in ~/.pi/agent/settings.json,
# longterm-mem MCP-registered), rather than reimplementing that logic here
# from a bare drift check. Its message already carries the R-007
# "--no-extensions"/"--no-skills" disclosure in every branch, so this
# helper only adds its own disclosure line for the not-built early return,
# which never reaches the delegate call. Returns non-zero exactly when the
# delegated `runtime status --target pi` does (not built, unproven/partial,
# or drifted); 0 only once every owned entry is proven supported.
pipkg_status_and_report() {
  local dest result
  dest="$(pipkg_dest_dir)"
  if [[ ! -e "$dest/package.json" ]]; then
    echo "  pi: note -- 'pi --no-extensions' disables the before_agent_start contract-gate extension for that session, and 'pi --no-skills' disables skill discovery, for that session only; neither flag's use is detected at runtime (short aliases: -ne and -ns)"
    echo "  pi: not built (run: labdrian-overlay apply --target pi)"
    return 1
  fi
  if result="$(OVERLAY_DIR="$OVERLAY_DIR" STATE_DIR="$STATE_DIR" "$ENGINE_BINARY" runtime status --target pi)"; then
    echo "  $result"
    return 0
  else
    echo "  $result"
    return 1
  fi
}
