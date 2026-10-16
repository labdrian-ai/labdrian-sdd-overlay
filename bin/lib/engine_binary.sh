# shellcheck shell=bash
#
# engine_binary
#
# The engine binary: whether it is stale, the sentence that says so, and ensure_engine_binary,
# which makes it or makes it again.
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

# engine_stale_diagnosis prints the sentence a reader cannot reconstruct on
# their own: that an unknown flag / unknown subcommand error coming OUT OF THE
# ENGINE is this staleness and not an overlay bug. That mistaken reading, not
# the staleness itself, is what made the original incident expensive, so every
# operator-facing report of a stale binary uses this one text.
#
# It names 'doctor --fix' as the remedy, not 'install-hooks': the repair the
# reader wants here is the engine binary, not a rewrite of settings.json.
engine_stale_diagnosis() {
  printf '%s' "engine binary at $ENGINE_BINARY is STALE — older than the engine source at $ENGINE_SRC, so it may not understand the flags and subcommands this overlay passes it. If a command fails with an unknown flag or unknown subcommand error from the engine, that is this staleness, not an overlay bug. Rebuild it with 'labdrian doctor --fix'."
}

# engine_binary_is_stale answers whether any Go source under $ENGINE_SRC is
# NEWER than $ENGINE_BINARY, i.e. whether the compiled artifact predates the
# source that defines it.
#
# The signal is mtime, not a recorded git revision, because mtime catches BOTH
# a repository update (git stamps the files it writes) and an uncommitted
# local edit during development; a recorded revision catches only the first.
#
# go.mod and go.sum count as source: a dependency bump changes behaviour as
# surely as a source edit does.
#
# The find expression is deliberately plain POSIX. `-newer` is portable, but
# `-print -quit` is GNU-only, and this script goes out of its way to stay
# macOS-safe (see the mkdir-not-flock reasoning in the install-tracking lock),
# so the "stop at the first hit" job goes to head instead.
engine_binary_is_stale() {
  [[ -d "$ENGINE_SRC" ]] || return 1
  # The listing is captured whole rather than piped into `head -n 1`, for the
  # reason this script already records twice elsewhere ("Pre-build tar listing
  # once to avoid SIGPIPE issues with set -euo pipefail"): under pipefail a
  # `find | head` whose output exceeds the pipe buffer leaves find killed by
  # SIGPIPE and the substitution exiting 141, which errexit turns into an
  # abort at any call site that is not a condition. Today the only call site
  # IS a condition, so the hazard is latent -- and a fixture-sized source tree
  # never reaches the buffer, so no test can catch it either. Both of those
  # are reasons to write it safely now rather than to rely on staying lucky.
  # An engine source tree is small; reading all of it costs nothing.
  local newer
  newer="$(
    find "$ENGINE_SRC" -type f \
      \( -name '*.go' -o -name 'go.mod' -o -name 'go.sum' \) \
      -newer "$ENGINE_BINARY" -print 2>/dev/null
  )"
  [[ -n "$newer" ]]
}

# ensure_engine_binary builds $ENGINE_BINARY on demand if it is missing,
# mirroring the existing cmd_uninstall_hooks fallback (hooks.sh) — cmd_longterm_mem
# needs the same binary to invoke 'engine runtime --component longterm-mem'.
#
# It also rebuilds when the binary is merely OUT OF DATE. Building only on
# absence is what let a fast-forwarded repository keep running an eleven-day-old
# engine against new expectations, until an overlay command died with
# `unknown flag "--component"` — which reads as an overlay bug and is a binary
# that predates the subcommand.
#
# This is the ONLY place that check lives, and it must stay that way. Every
# path that needs the engine already calls ensure_engine_binary, so one
# mechanism covers them all; adding the same check to self-update or apply
# would be two mechanisms to drift apart, not one safety net plus a backup.
#
# WHEN STALE AND NOT REBUILDABLE the function WARNS AND CONTINUES, which is a
# deliberate departure from the absent-binary path immediately above it:
#   - an absent binary means nothing can run at all, so dying is the only
#     option there;
#   - a stale binary may well serve the command at hand perfectly, and
#     refusing would brick every overlay command for a user whose Go toolchain
#     disappeared;
#   - what made this defect expensive was never the staleness itself, it was
#     that the resulting failure looked like an overlay bug. So the warning
#     names the staleness, PREDICTS the symptom (an unknown flag or unknown
#     subcommand error coming out of the engine is this, not a bug), and says
#     how to fix it.
# That loud, diagnosed warning is NOT the silent skip this repository keeps
# removing — a skip that reads as a pass is. Do not "tighten" this into a hard
# failure without understanding that trade.
ensure_engine_binary() {
  if [[ ! -x "$ENGINE_BINARY" ]]; then
    command -v go >/dev/null 2>&1 || die "engine binary not found at $ENGINE_BINARY and go is not installed; cannot continue"
    [[ -d "$ENGINE_SRC" ]] || die "engine source not found at $ENGINE_SRC"
    info "Engine binary not found; building..."
    mkdir -p "$(dirname "$ENGINE_BINARY")"
    (cd "$ENGINE_SRC" && go build -o "$ENGINE_BINARY" ./cmd/) \
      || die "go build failed for the engine binary"
    return 0
  fi

  engine_binary_is_stale || return 0

  local stale_warning="the engine binary at $ENGINE_BINARY is OLDER than the engine source at $ENGINE_SRC, so it may not understand the flags and subcommands this overlay passes it. If a command below fails with an unknown flag or unknown subcommand error from the engine, that is this staleness, not an overlay bug. Fix it with 'labdrian install-hooks', or by installing a Go toolchain so the overlay can rebuild the engine itself."

  if ! command -v go >/dev/null 2>&1; then
    warn "Engine binary is stale and go is not installed: $stale_warning"
    return 0
  fi

  info "Engine binary is out of date; rebuilding..."
  if ! (cd "$ENGINE_SRC" && go build -o "$ENGINE_BINARY" ./cmd/); then
    warn "Engine binary is stale and the rebuild failed: $stale_warning"
    return 0
  fi
}
