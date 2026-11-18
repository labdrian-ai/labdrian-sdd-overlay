# shellcheck shell=bash
#
# skill_registry
#
# The commands that repair and refresh a project's skill registry: repair-sdd-registry and
# skill-registry refresh.
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

# repair_sdd_registry_file <registry> <target>
#
# The skill registry's table leaves the sdd-* phase skills out (they are executor
# contracts, not skills a person invokes); SDD delegation needs their paths, so a
# row for each installed one is put back.
# That repair is the engine's (`registry repair-sdd`): which skills are
# executors, what a row says, how the registry is edited, the lock it takes (the
# one `propagate` takes, so a repair and the hook that writes the same file take
# turns) and the atomic write live in engine/sddregistry. This script owns what
# the engine cannot know, the directory the target keeps its skills in
# (TARGET_PATHS), and hands it over. The engine speaks for itself: the WARN line
# of each executor not installed, the sum-up on stdout, and a refusal on stderr
# with exit 1.
repair_sdd_registry_file() {
  local registry="$1"
  local target="$2"

  # A target that reads the skills of ~/.claude/skills has no skills directory of its own to read.
  local skills_root="${TARGET_PATHS[$target]}"
  copies_skills "$target" || skills_root="${TARGET_PATHS[claude]}"

  ensure_engine_binary >&2
  "$ENGINE_BINARY" registry repair-sdd --registry "$registry" \
    --target "$target" --skills-root "$skills_root"
}

# refresh_skill_registry <cwd> <force> <quiet>
#
# Writes the table of skills of <cwd>/.atl/skill-registry.md. That is the engine's
# (`registry refresh`, engine/skillregistry): which directories are scanned, how a
# SKILL.md names itself, which skills are left out, the lock it takes (the one
# `propagate` takes) and the atomic write live there, and every line of the
# registry that is not the generated head or a row is left as it was, the rows
# `registry repair-sdd` writes included. It replaced `gentle-ai skill-registry
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

# The table of skills leaves the sdd-* phase skills out, so the executor rows have to be put back
# after every refresh. This command refreshes the registry (forced, quiet) and then
# re-inserts the installed sdd-* rows with Scope=executor.

cmd_repair_sdd_registry() {
  local cwd="$CALLER_CWD"
  local target="codex"
  local refresh=1

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
      --no-refresh)
        refresh=0
        ;;
      *) die "Unknown option: $1. Usage: labdrian repair-sdd-registry [--cwd DIR] [--target claude|opencode|codex] [--no-refresh]" ;;
    esac
    shift || true
  done

  [[ -d "$cwd" ]] || die "--cwd is not a directory: $cwd"

  if [[ "$refresh" -eq 1 ]]; then
    refresh_skill_registry "$cwd" 1 1
  fi

  repair_sdd_registry_file "$cwd/.atl/skill-registry.md" "$target"
}

# Wrapper around `registry refresh` that applies the overlay's SDD executor repair
# immediately after the table is refreshed. This keeps the executor rows from
# disappearing every time users refresh the registry through the overlay.

cmd_skill_registry() {
  local verb="${1:-}"
  [[ "$verb" == "refresh" ]] || die "Usage: labdrian skill-registry refresh [--cwd DIR] [--target claude|opencode|codex] [--no-repair] [--force] [--quiet] [--no-gitignore]"
  shift || true

  local cwd="$CALLER_CWD"
  local target="codex"
  local repair=1
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
        repair=0
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

  if [[ "$repair" -eq 1 ]]; then
    repair_sdd_registry_file "$cwd/.atl/skill-registry.md" "$target"
  fi
}
