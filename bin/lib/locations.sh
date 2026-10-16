# shellcheck shell=bash
# shellcheck disable=SC2034  # every variable here is read by the other modules
#
# locations
#
# Where this overlay keeps and finds things: the gentle-ai snapshot tarball, the manifest, the
# state directory and file, the engine binary and source, and the longterm-mem binary and
# source. Every other module reads these; none assigns them.
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

TARBALL="$HOME/.gentle-ai/backups/upgrade-20260615T175529Z/snapshot.tar.gz"
# The gentle-ai version the snapshot at TARBALL was taken from: bootstrap names it in the upstream
# baseline commit, so the two change together.
TARBALL_GENTLE_AI_VERSION="1.40.2"
MANIFEST="${MANIFEST:-"$OVERLAY_DIR/overlay.manifest"}"

# Home-scoped per design decision D3: state describes deployments under
# $HOME (TARGET_PATHS/AGENT_TARGET_PATHS are all $HOME-anchored), so a
# repo-scoped file would let two clones of this repo hold divergent records
# of the same live targets.
#
# Normalized ONCE, here, so every path derived from it below and every
# `--state-dir "$STATE_DIR"` handed to the engine already looks the way the
# engine's own filepath.Join will make it look. Doing it at the point of use
# instead would mean remembering it at each of them, which is exactly the
# kind of agreement that decays.
STATE_DIR="$(path_clean "${STATE_DIR:-"$HOME/.labdrian-overlay"}")"
STATE_FILE="$STATE_DIR/state.json"

# Path where the engine binary is deployed. Using ~/.claude/bin/ keeps all
# Claude Code tooling co-located and makes the binary easy to find in hooks.
ENGINE_BINARY="$HOME/.claude/bin/gentle-ai-overlay"
ENGINE_SRC="$OVERLAY_DIR/engine"

# LONGTERM_MEM_BINARY is the fixed, documented, persistent install path for
# the longterm-mem binary (R-015): it never moves between an install and a
# later inspection. STATE_DIR-scoped (not $HOME/.claude/-scoped like
# ENGINE_BINARY) because longterm-mem is a cross-runtime component, not a
# Claude-specific one (D4).
#
# Routed through path_clean even though $STATE_DIR is already clean: this
# exact string is what `register --binary` writes into every runtime's MCP
# entry, and what the engine re-derives with filepath.Join to decide whether
# that entry is ours. path_clean here is what makes the two halves agree for
# EVERY $STATE_DIR rather than only the already-clean ones — including the
# one case normalizing $STATE_DIR alone does not cover, a state dir of "/",
# where plain concatenation still yields "//bin/longterm-mem".
LONGTERM_MEM_BINARY="$(path_clean "$STATE_DIR/bin/longterm-mem")"
LONGTERM_MEM_SRC="$OVERLAY_DIR/longterm-mem"
