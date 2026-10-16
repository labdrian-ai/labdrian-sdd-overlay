# shellcheck shell=bash
#
# tool_runners
#
# The commands that build a tool under tools/ and run it: validate-entry-contract,
# deterministic-checks and archive-reconcile.
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

cmd_validate_entry_contract() {
  local validator_src="$OVERLAY_DIR/tools/entry-contract-validator"
  if [[ ! -f "$validator_src/go.mod" || ! -f "$validator_src/main.go" ]]; then
    echo "error: entry-contract validator source is unavailable under $validator_src" >&2
    return 3
  fi
  if ! command -v go >/dev/null 2>&1; then
    echo "error: go is required to build the entry-contract validator" >&2
    return 3
  fi

  local -a normalized_args=()
  local flag value
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --schema|--instance)
        flag="$1"
        shift
        if [[ $# -eq 0 ]]; then
          normalized_args+=("$flag")
          break
        fi
        value="$1"
        [[ "$value" == /* ]] || value="$CALLER_CWD/$value"
        normalized_args+=("$flag" "$value")
        ;;
      --schema=*|--instance=*)
        flag="${1%%=*}"
        value="${1#*=}"
        [[ "$value" == /* ]] || value="$CALLER_CWD/$value"
        normalized_args+=("$flag=$value")
        ;;
      *)
        normalized_args+=("$1")
        ;;
    esac
    shift
  done

  local validator_binary
  if ! validator_binary="$(mktemp "${TMPDIR:-/tmp}/entry-contract-validator.XXXXXX")"; then
    echo "error: could not allocate a temporary entry-contract validator binary" >&2
    return 3
  fi
  rm -f "$validator_binary"

  if ! (cd "$validator_src" && go build -o "$validator_binary" .); then
    rm -f "$validator_binary"
    echo "error: could not build the entry-contract validator" >&2
    return 3
  fi

  local status=0
  "$validator_binary" "${normalized_args[@]}" || status=$?
  rm -f "$validator_binary"
  return "$status"
}

cmd_deterministic_checks() {
  local runner_src="$OVERLAY_DIR/tools/deterministic-check-runner"
  if [[ ! -f "$runner_src/go.mod" || ! -f "$runner_src/main.go" ]]; then
    echo "error: deterministic-check-runner source is unavailable under $runner_src" >&2
    return 3
  fi
  if ! command -v go >/dev/null 2>&1; then
    echo "error: go is required to build the deterministic-check-runner" >&2
    return 3
  fi

  local runner_binary
  if ! runner_binary="$(mktemp "${TMPDIR:-/tmp}/deterministic-check-runner.XXXXXX")"; then
    echo "error: could not allocate a temporary deterministic-check-runner binary" >&2
    return 3
  fi
  rm -f "$runner_binary"

  if ! (cd "$runner_src" && go build -o "$runner_binary" .); then
    rm -f "$runner_binary"
    echo "error: could not build the deterministic-check-runner" >&2
    return 3
  fi

  local status=0
  (cd "$CALLER_CWD" && "$runner_binary" "$@") || status=$?
  if [[ "$status" -ne 0 && ! -d "$CALLER_CWD" ]]; then
    status=3
  fi
  rm -f "$runner_binary"
  return "$status"
}

# Same shape as cmd_deterministic_checks above, deliberately: every failure to
# reach the guard is exit 3 ("could not determine"), never exit 0, so a missing
# source tree or a missing toolchain can never be read as "no stranded change
# exists here". The guard's own exit code is propagated verbatim - the wrapper
# never re-derives it.
cmd_archive_reconcile() {
  local guard_src="$OVERLAY_DIR/tools/archive-reconcile"
  if [[ ! -f "$guard_src/go.mod" || ! -f "$guard_src/main.go" ]]; then
    echo "error: archive-reconcile source is unavailable under $guard_src" >&2
    return 3
  fi
  if ! command -v go >/dev/null 2>&1; then
    echo "error: go is required to build the archive-reconcile guard" >&2
    return 3
  fi

  local guard_binary
  if ! guard_binary="$(mktemp "${TMPDIR:-/tmp}/archive-reconcile.XXXXXX")"; then
    echo "error: could not allocate a temporary archive-reconcile binary" >&2
    return 3
  fi
  rm -f "$guard_binary"

  if ! (cd "$guard_src" && go build -o "$guard_binary" .); then
    rm -f "$guard_binary"
    echo "error: could not build the archive-reconcile guard" >&2
    return 3
  fi

  local status=0
  (cd "$CALLER_CWD" && "$guard_binary" "$@") || status=$?
  # A CALLER_CWD that no longer exists makes the subshell's own `cd` fail with
  # exit 1, which for this guard would read as "STRANDED change found" - a
  # verdict about a scan that was never performed. Remap that one case to 3.
  if [[ "$status" -ne 0 && ! -d "$CALLER_CWD" ]]; then
    status=3
  fi
  rm -f "$guard_binary"
  return "$status"
}
