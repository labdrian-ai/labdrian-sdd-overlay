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
# gentle-ai leaves the sdd-* phase skills out of the skill registry it writes;
# SDD delegation needs their paths, so a row for each installed one is put back.
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

  ensure_engine_binary >&2
  "$ENGINE_BINARY" registry repair-sdd --registry "$registry" \
    --target "$target" --skills-root "${TARGET_PATHS[$target]}"
}

# Gentle's default skill-registry intentionally skips sdd-* phase skills because
# they are executor contracts, not normal user-invocable skills. Codex/Gentle SDD
# delegation still needs their exact paths. This command makes the registry
# durable after `gentle-ai sync` / `gentle-ai skill-registry refresh` by
# refreshing first, then re-inserting installed sdd-* rows with Scope=executor.

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
    command -v gentle-ai >/dev/null 2>&1 || die "gentle-ai not found on PATH; use --no-refresh to repair an existing registry only"
    gentle-ai skill-registry refresh --cwd "$cwd" --force --quiet --no-gitignore
  fi

  repair_sdd_registry_file "$cwd/.atl/skill-registry.md" "$target"
}

# Wrapper around `gentle-ai skill-registry refresh` that applies the overlay's
# SDD executor repair immediately after the generated registry is refreshed.
# This prevents the known Gentle default skip of sdd-* phase skills from
# reappearing every time users refresh the registry through the overlay.

cmd_skill_registry() {
  local verb="${1:-}"
  [[ "$verb" == "refresh" ]] || die "Usage: labdrian skill-registry refresh [--cwd DIR] [--target claude|opencode|codex] [--no-repair] [--force] [--quiet] [--no-gitignore]"
  shift || true

  local cwd="$CALLER_CWD"
  local target="codex"
  local repair=1
  local saw_no_gitignore=0
  local -a gentle_args=("skill-registry" "refresh")

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --cwd)
        shift
        cwd="${1:-}"
        [[ -n "$cwd" ]] || die "--cwd requires an argument"
        gentle_args+=("--cwd" "$cwd")
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
        saw_no_gitignore=1
        gentle_args+=("$1")
        ;;
      --force|--quiet)
        gentle_args+=("$1")
        ;;
      *) die "Unknown option: $1. Usage: labdrian skill-registry refresh [--cwd DIR] [--target claude|opencode|codex] [--no-repair] [--force] [--quiet] [--no-gitignore]" ;;
    esac
    shift || true
  done

  [[ -d "$cwd" ]] || die "--cwd is not a directory: $cwd"
  [[ "$saw_no_gitignore" -eq 1 ]] || gentle_args+=("--no-gitignore")

  command -v gentle-ai >/dev/null 2>&1 || die "gentle-ai not found on PATH"
  gentle-ai "${gentle_args[@]}"

  if [[ "$repair" -eq 1 ]]; then
    repair_sdd_registry_file "$cwd/.atl/skill-registry.md" "$target"
  fi
}
