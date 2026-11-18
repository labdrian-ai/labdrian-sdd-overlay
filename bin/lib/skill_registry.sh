# shellcheck shell=bash
#
# skill_registry
#
# The command that refreshes a project's skill registry: skill-registry refresh.
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

# refresh_skill_registry <cwd> <force> <quiet>
#
# Writes the table of skills of <cwd>/.atl/skill-registry.md. That is the engine's
# (`registry refresh`, engine/skillregistry): which directories are scanned, how a
# SKILL.md names itself, which skills are left out, the lock it takes (the one
# `propagate` takes) and the atomic write live there, and every line of the
# registry that is not the generated head or a row is left as it was, the rows
# of the executor repair that is gone included. It replaced `gentle-ai skill-registry
# refresh`, so no gentle-ai binary is needed. <force> and <quiet> are 1 or 0. The
# engine speaks for itself, and ends this command with its exit code when it fails.
refresh_skill_registry() {
  local cwd="$1"
  local force="$2"
  local quiet="$3"
  local -a args=(registry refresh --cwd "$cwd")
  [[ "$force" -eq 1 ]] && args+=(--force)
  [[ "$quiet" -eq 1 ]] && args+=(--quiet)

  ensure_engine_binary >&2
  "$ENGINE_BINARY" "${args[@]}"
}

# Wrapper around `registry refresh`. It used to repair the SDD executor rows after the refresh;
# --no-repair and --target are still accepted and do nothing.

cmd_skill_registry() {
  local verb="${1:-}"
  [[ "$verb" == "refresh" ]] || die "Usage: labdrian skill-registry refresh [--cwd DIR] [--target claude|opencode|codex] [--no-repair] [--force] [--quiet] [--no-gitignore]"
  shift || true

  local cwd="$CALLER_CWD"
  local target="codex"
  local force=0
  local quiet=0

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --cwd)
        shift
        cwd="${1:-}"
        [[ -n "$cwd" ]] || die "--cwd requires an argument"
        ;;
      --target)
        shift
        target="${1:-}"
        [[ -n "$target" ]] || die "--target requires an argument"
        is_valid_target "$target" || die "Unknown target: $target. Valid: claude, opencode, codex, pi"
        is_copy_target "$target" || die "--target $target is a package target with no gentle-ai skill-registry.md; only claude, opencode, codex are supported here"
        ;;
      --no-repair)
        # Accepted as it always was; there is no repair step left to turn off.
        ;;
      --no-gitignore)
        # Accepted as it always was; the engine's refresh never edits .gitignore, so there is
        # nothing to turn off.
        ;;
      --force)
        force=1
        ;;
      --quiet)
        quiet=1
        ;;
      *) die "Unknown option: $1. Usage: labdrian skill-registry refresh [--cwd DIR] [--target claude|opencode|codex] [--no-repair] [--force] [--quiet] [--no-gitignore]" ;;
    esac
    shift || true
  done

  [[ -d "$cwd" ]] || die "--cwd is not a directory: $cwd"

  refresh_skill_registry "$cwd" "$force" "$quiet"
}
