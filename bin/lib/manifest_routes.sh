# shellcheck shell=bash
#
# manifest_routes
#
# Where each row of overlay.manifest goes, as the engine decides it (manifest routes), and where
# a row is stored inside a gentle-ai snapshot tarball (route_tar_path).
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

# The decision is the engine's (`manifest routes`), not this script's: how a
# manifest is read, which route a row takes, which rows are never deployed,
# and the longterm-mem/** route-domain guard (R-012 in overlay-agent-route)
# live in engine/skills, in one place. This script owns what the engine
# cannot know -- the directories its targets keep their skills and native
# agents in (TARGET_PATHS, AGENT_TARGET_PATHS) -- and hands them over.
#
# manifest_routes_load <target> [managed]
#
# Fills MANIFEST_ROUTES with one line per row of the manifest, in the order it
# is written (only the rows tagged managed when the second argument is
# "managed"), in five columns separated by a tab:
#
#   <path> <route> <repo_rel> <repo_source> <dest>
#
#   path         the first column of the row
#   route        skill, agent, opencode-agent, codex-agent, mcp or non-deployable;
#                a third column nobody knows keeps its own name and is the skill
#                route
#   repo_rel     the file's place in the overlay repository, which is how git
#                knows it on the branches (skills/<path> for a skill)
#   repo_source  the absolute path it is copied from; empty when non-deployable
#   dest         the absolute path it is copied to for <target>; empty when it
#                goes nowhere for that target (an agent for opencode or codex,
#                an mcp row, a row that is never deployed)
#
# An empty column is never followed by one that is not, so `IFS=$'\t' read -r`
# (which drops empty trailing fields) reads every line whole. Callers skip a
# row with an empty dest, as they always did.
#
# Any refusal ends the run: a longterm-mem/** row with no route or an unknown
# one, a manifest that cannot be read, an engine that is not there. The engine
# says why on stderr and answers nothing, so the run stops before it acts on
# part of a manifest. (The `|| exit $?` is for the callers that run inside
# $(...), where errexit is off and a plain failure would go unnoticed.)
#
# The engine is made on first use, or again when it is older than its source
# (ensure_engine_binary), with its words on stderr: this runs inside $(...) too,
# and a line on stdout there would end up in the value being computed.
MANIFEST_ROUTES=""

manifest_routes_load() {
  local target="$1"
  local -a args=(manifest routes --overlay-root "$OVERLAY_DIR" --manifest "$MANIFEST"
    --target "$target" --skills-root "${TARGET_PATHS[$target]}")
  if [[ -v AGENT_TARGET_PATHS["$target"] ]]; then
    args+=(--agents-root "${AGENT_TARGET_PATHS[$target]}")
  fi
  if [[ "${2:-}" == "managed" ]]; then
    args+=(--managed)
  fi
  ensure_engine_binary >&2
  MANIFEST_ROUTES="$("$ENGINE_BINARY" "${args[@]}")" || exit $?
}

# manifest_routes_rows prints the lines manifest_routes_load filled, one per
# row and none when there are none, for a `while read ... done < <(...)` loop.
manifest_routes_rows() {
  [[ -z "$MANIFEST_ROUTES" ]] || printf '%s\n' "$MANIFEST_ROUTES"
}

# route_tar_path resolves where a manifest row is stored inside a backup tarball.
# The legacy backup layout is under ~/.claude. OpenCode-native agent rows must use
# ~/.config/opencode instead, and Codex-native agent rows ~/.codex.
route_tar_path() {
  local route="$1"
  local repo_rel="$2"  # the repo_rel column of manifest_routes_load

  case "$route" in
    opencode-agent)
      printf 'files/home/labdrian/.config/opencode/%s' "${repo_rel#opencode/}"
      ;;
    codex-agent)
      printf 'files/home/labdrian/.codex/%s' "${repo_rel#codex/}"
      ;;
    *)
      printf 'files/home/labdrian/.claude/%s' "$repo_rel"
      ;;
  esac
}
