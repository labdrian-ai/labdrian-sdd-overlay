# shellcheck shell=bash
#
# targets
#
# The deployment targets: where each one keeps its skills and native agents, the catalog that
# declares them (the ONE place a target is declared), the rules that read it
# (is_valid_target, is_copy_target, resolve_targets), and the targets command.
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

# shellcheck disable=SC2034  # read by the other modules
declare -A TARGET_PATHS=(
  [claude]="$HOME/.claude/skills"
  [opencode]="$HOME/.config/opencode/skills"
  [codex]="$HOME/.codex/skills"
)

# Native agent-route destinations. Claude Code and OpenCode use different
# frontmatter schemas, so they are deployed from separate generated files.
# shellcheck disable=SC2034  # read by the other modules
declare -A AGENT_TARGET_PATHS=(
  [claude]="$HOME/.claude/agents"
  [opencode]="$HOME/.config/opencode/agents"
)

# Every recognized --target, in the order `--target all` expands to, each as
# "<name>:<kind>" (pi-target-plumbing, R-001/R-008). "copy": per-file,
# deployed via TARGET_PATHS above. "package": built and installed as a whole
# (pi, via `pi install`/`pi remove` — engine/pipkg, wired starting in the
# pi-package-build slice).
#
# This is the ONE place a target is declared. ALL_TARGETS (the order and the
# `--target all` expansion, see resolve_targets), TARGET_KINDS (the lookup
# behind is_valid_target/is_copy_target) and the `targets` subcommand (the
# catalog the TUI reads) are all derived from it just below, so the set a
# front-end shows and the set `all` acts on cannot drift apart. Add a target
# here and nowhere else.
TARGET_CATALOG=(
  claude:copy
  opencode:copy
  codex:copy
  pi:package
)

# Every call site that used to probe TARGET_PATHS directly for "is this a
# known target at all" and "does it have a TARGET_PATHS entry" now goes
# through is_valid_target/is_copy_target below instead, so a package target is
# never silently treated as a copy target with an empty destination.
declare -a ALL_TARGETS=()
declare -A TARGET_KINDS=()
for _catalog_entry in "${TARGET_CATALOG[@]}"; do
  ALL_TARGETS+=("${_catalog_entry%%:*}")
  TARGET_KINDS["${_catalog_entry%%:*}"]="${_catalog_entry#*:}"
done
unset _catalog_entry

# True (exit 0) when $1 names any recognized target, copy or package.
is_valid_target() {
  [[ -v TARGET_KINDS["${1:-}"] ]]
}

# True (exit 0) when $1 is a per-file copy target with a TARGET_PATHS entry
# (claude/opencode/codex). False for a package target (pi) or an unknown
# name — callers must check is_valid_target first if they need to
# distinguish "package" from "unknown".
is_copy_target() {
  [[ "${TARGET_KINDS[${1:-}]:-}" == "copy" ]]
}

# Emits the shared stub line for a recognized package target whose
# package_target_stub_message is the honest fallback for a package-model
# target that has no adapter wired yet (today every package target, i.e.
# pi, is dispatched explicitly above this call).
package_target_stub_message() {
  echo "  $1: package target without a lifecycle adapter (not implemented)"
}

# Resolve --target argument to a space-separated list of target names.
# `all` expands to ALL_TARGETS, the same list `targets` prints.
resolve_targets() {
  local arg="${1:-all}"
  if [[ "$arg" == "all" ]]; then
    echo "${ALL_TARGETS[*]}"
  elif is_valid_target "$arg"; then
    echo "$arg"
  else
    local valid
    printf -v valid '%s, ' "${ALL_TARGETS[@]}"
    die "Unknown target: $arg. Valid: ${valid}all"
  fi
}

# cmd_targets: print the target catalog, one target per line, as
# "<name><TAB><kind>" (kind is "copy" or "package"), in the order `--target
# all` expands to. The TUI reads this instead of keeping a list of its own,
# and a front-end that sends `--target all` only when the user selected every
# line printed here can never act on a target the user did not see. Read-only:
# it takes no arguments and touches no file.
cmd_targets() {
  [[ $# -eq 0 ]] || die "targets takes no arguments (got: $*)"
  local name
  for name in "${ALL_TARGETS[@]}"; do
    printf '%s\t%s\n' "$name" "${TARGET_KINDS[$name]}"
  done
}
