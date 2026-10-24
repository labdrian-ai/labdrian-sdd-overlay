# shellcheck shell=bash
#
# engine_verbs
#
# The commands that are a forward to a verb of the engine: skills, shaper, roles, memory,
# workflow and gadu-generate.
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

# cmd_skills forwards to the engine binary's `skills` subcommand.
# Injects default --registry and --manifest paths from $OVERLAY_DIR so that
# `labdrian skills list` / `labdrian skills validate` work from any cwd.
# Flags already supplied by the caller are never overridden.
cmd_skills() {
  if [[ ! -x "$ENGINE_BINARY" ]]; then
    die "engine binary not found at $ENGINE_BINARY; run 'labdrian install-hooks' first"
  fi

  local has_registry=0 has_manifest=0 has_source_root=0
  for arg in "$@"; do
    [[ "$arg" == "--registry" ]] && has_registry=1
    [[ "$arg" == "--manifest" ]] && has_manifest=1
    [[ "$arg" == "--source-root" ]] && has_source_root=1
  done

  local -a defaults=()
  [[ "$has_registry" -eq 0 ]] && defaults+=("--registry" "$OVERLAY_DIR/skills.registry.yaml")
  [[ "$has_manifest" -eq 0 ]] && defaults+=("--manifest" "$OVERLAY_DIR/overlay.manifest")
  [[ "$has_source_root" -eq 0 ]] && defaults+=("--source-root" "$OVERLAY_DIR/skills")

  # Verb-first: "$@" carries the verb (and any user flags); injected defaults go
  # AFTER it so the engine's positional verb extraction is not shadowed by a flag value.
  exec "$ENGINE_BINARY" skills "$@" "${defaults[@]}"
}

# cmd_shaper forwards `labdrian shaper <verb>` to the engine unchanged, so the
# engine's exit codes (0 ready, 3 draft, 2 invalid, 1 error) reach the caller.
cmd_shaper() {
  if [[ ! -x "$ENGINE_BINARY" ]]; then
    die "engine binary not found at $ENGINE_BINARY; run 'labdrian install-hooks' first"
  fi
  exec "$ENGINE_BINARY" shaper "$@"
}

# cmd_roles forwards `labdrian roles <verb>` to the engine unchanged. Roles
# are data only: no verb launches an agent, dispatches work, or grants
# execution authority.
cmd_roles() {
  if [[ ! -x "$ENGINE_BINARY" ]]; then
    die "engine binary not found at $ENGINE_BINARY; run 'labdrian install-hooks' first"
  fi
  exec "$ENGINE_BINARY" roles "$@"
}

# cmd_memory forwards `labdrian memory <verb>` to the engine unchanged.
# Memory plans are read-only data: no verb queries or writes memory; query
# execution belongs to a runtime adapter outside this overlay (Phase 7).
cmd_memory() {
  if [[ ! -x "$ENGINE_BINARY" ]]; then
    die "engine binary not found at $ENGINE_BINARY; run 'labdrian install-hooks' first"
  fi
  exec "$ENGINE_BINARY" memory "$@"
}

# cmd_workflow forwards `labdrian workflow <verb>` to the engine unchanged.
# The Phase 6 standalone workflow lifecycle is local bookkeeping under
# $XDG_STATE_HOME/labdrian/workflows/; no verb requires Gentle AI, gentle-pi,
# a runtime, memory, or auth, and no verb executes a workflow step or check
# (that is Phase 7).
cmd_workflow() {
  if [[ ! -x "$ENGINE_BINARY" ]]; then
    die "engine binary not found at $ENGINE_BINARY; run 'labdrian install-hooks' first"
  fi
  exec "$ENGINE_BINARY" workflow "$@"
}

cmd_gadu_generate() {
  if [[ ! -x "$ENGINE_BINARY" ]]; then
    die "engine binary not found at $ENGINE_BINARY; run 'labdrian install-hooks' first"
  fi
  exec env OVERLAY_DIR="$OVERLAY_DIR" "$ENGINE_BINARY" gadu-generate "$@"
}
