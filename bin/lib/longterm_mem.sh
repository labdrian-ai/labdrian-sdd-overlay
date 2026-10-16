# shellcheck shell=bash
#
# longterm_mem
#
# The longterm-mem component: build and deploy its binary, register it with each runtime, report
# its status, uninstall it, and forward sync-trigger. The record of the runtimes it is
# installed for is the engine's (installed-targets); this module asks it.
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

# longtermmem_targets <verb> [args...] runs a verb of the engine's installed-targets command on this
# overlay's state directory: the record of the runtimes longterm-mem is installed for (record, release,
# list, assess, retire). The record is the engine's (Phase 9, B3). It is the only thing that says whether
# a runtime still depends on the shared binary at $LONGTERM_MEM_BINARY -- the binary is removed only
# when the record is empty, or with --purge -- and it is read as evidence, written under a lock
# ($STATE_DIR/longterm-mem/installed-targets, under $STATE_DIR/.installed-targets.flock), and judged by
# the engine, which says why on stderr, in the words this script always used, when a verb
# fails; the caller then ends the run. True per-runtime un-registration in each runtime's own config
# file is the longterm-mem module's (register, unregister); the engine's registration.json (engine
# runtime install|uninstall --component longterm-mem) records per-runtime state but is removed whole,
# regardless of --target, so it cannot answer "does any target still count as installed" and is
# removed only once this record says that none does.
#
# The engine is a prerequisite: cmd_longterm_mem makes sure there is one before it gets here, and an
# engine too old to know the command ends the run with its own "unknown subcommand".
longtermmem_targets() {
  local verb="$1"
  shift
  "$ENGINE_BINARY" installed-targets "$verb" --state-dir "$STATE_DIR" "$@"
}

# longtermmem_engine_knows_installed_targets answers whether the engine has the installed-targets
# command at all. An engine older than this overlay answers any call to it with `error: unknown
# subcommand "installed-targets"` and exit 1, so the bare command (which a current engine refuses
# for want of a verb, and which changes nothing in either) tells an engine that does not know the
# command from one that knows it and failed. Only a caller that has seen exit 1 asks.
longtermmem_engine_knows_installed_targets() {
  local said
  said="$("$ENGINE_BINARY" installed-targets 2>&1 </dev/null)" || true
  [[ "$said" != *"unknown subcommand"* ]]
}

# longtermmem_install_step runs cmd_apply's D13 "install once per apply
# invocation" step and reports whether it succeeded, printing the recovery
# line itself when it did not.
#
# The call is wrapped in a SUBSHELL deliberately. cmd_longterm_mem and the
# helpers it reaches report unrecoverable problems through `die` — the
# script-wide convention, used in dozens of places — and `die` exits the
# PROCESS. Invoked directly as `if ! cmd_longterm_mem install`, that took
# the entire apply run down with it, so the recovery line below, which
# exists precisely to explain a failed install, was unreachable for the
# likeliest failures: no go toolchain, no longterm-mem module, a failed
# build, an unresolvable --target, a missing engine binary.
#
# A subshell turns die's exit into an observable status at exactly this one
# call site, and it does so without rewriting `die` into a dual-mode
# "return when I am a callee" that every one of its call sites would then
# have to honour correctly. Nothing is lost to the subshell: every effect of
# this step lands on the filesystem, never in shell state the caller reads.
#
# Note that the subshell does NOT restore errexit inside the callee — bash
# suppresses it for the whole condition context either way — which is why
# the install path checks each of its own deploy steps explicitly.
longtermmem_install_step() {
  local target_arg="$1"
  local status=0
  ( cmd_longterm_mem install --target "$target_arg" ) || status=$?
  if [[ "$status" -ne 0 ]]; then
    warn "longterm-mem install FAILED after per-target state was already written: the MCP server is not installed. Target state now records a successful deploy, so re-running apply will NOT retry this step — run 'labdrian longterm-mem install --target $target_arg' directly."
  fi
  return "$status"
}

# cmd_longterm_mem implements 'longterm-mem install|status|uninstall
# [--target claude|opencode|codex|all] [--purge]' (D4, D13).
#
# install: go build the longterm-mem binary, copy it to the fixed path
# (LONGTERM_MEM_BINARY, R-015), then per target: 'longterm-mem register
# --target t --binary "$LONGTERM_MEM_BINARY"' (module-owned, writes the
# ownership-tagged MCP entry and its install-state.json record at the
# binary's own default state dir, D9), then 'engine runtime install
# --component longterm-mem --target t' to record and report per-runtime
# status. R-022's vault seeding is lazy (vaultreg.Resolve) and needs no
# separate call here -- the 'longterm-mem vaults seed' invocation this
# replaced named a subcommand that never existed and never will.
#
# The two overrides are NOT symmetrical, and conflating them is how this
# comment previously ended up justifying a flag with the other flag's
# reason:
#
#   --state-dir is deliberately NOT passed, here or on the unregister call
#   below, so that both resolve the SAME install-state.json — the module's
#   own default. --state-dir is what sets that file's location, so passing
#   it on one call and not the other is exactly how install and uninstall
#   come to disagree about where the ownership record lives (see the
#   unregister call's own comment for why that agreement is load-bearing).
#
#   --binary IS passed. It does not move install-state.json and so cannot
#   break that agreement; it sets the path written INTO each runtime's MCP
#   entry, which must be where the binary was actually just deployed. See
#   the call site for the full reason.
#
# status/uninstall skip the build step entirely (R-014 scenario 2) and only
# ever talk to the engine runtime lifecycle surface.
cmd_longterm_mem() {
  local subcmd="${1:-}"
  shift || true
  case "$subcmd" in
    install|status|uninstall|sync-trigger) ;;
    *) die "Unknown longterm-mem subcommand: ${subcmd:-<none>}. Valid: install, status, uninstall, sync-trigger" ;;
  esac

  # sync-trigger is a pure forward to the engine verb, not a variant of
  # install/status/uninstall's build-or-report shape, and it MUST NOT
  # inherit any of it: it never resolves --target, never calls
  # ensure_engine_binary (which can `die` trying to build a missing
  # engine -- exactly what a SessionEnd hook or archive closure-feedback
  # caller must never observe, R-001/R-003), and it always returns 0.
  # A missing/non-executable engine binary degrades to a warning instead.
  #
  # The wrapper MUST return without waiting on the engine at all -- a
  # bounded wait (even a short `timeout`) is still a wait, and a review
  # correction rejected exactly that: it blocks the SessionEnd hook or
  # archive closure-feedback caller for up to the bound. The engine is
  # therefore launched fully detached -- its own session via `setsid`
  # when available, so it outlives the wrapper's own exit -- with stdin
  # from /dev/null and stdout/stderr discarded, backgrounded, and never
  # awaited; `disown` best-effort covers shells where the background job
  # is not already detached. The engine process owns its own bounded
  # child timeout internally; the wrapper adds no wait of its own on top
  # of that.
  if [[ "$subcmd" == "sync-trigger" ]]; then
    if [[ ! -x "$ENGINE_BINARY" ]]; then
      warn "longterm-mem sync-trigger: engine binary not found or not executable at $ENGINE_BINARY; skipping sync"
      return 0
    fi
    if command -v setsid >/dev/null 2>&1; then
      setsid "$ENGINE_BINARY" sync-trigger --state-dir "$STATE_DIR" "$@" </dev/null >/dev/null 2>&1 &
    else
      "$ENGINE_BINARY" sync-trigger --state-dir "$STATE_DIR" "$@" </dev/null >/dev/null 2>&1 &
    fi
    disown 2>/dev/null || true
    return 0
  fi

  local target_arg="all"
  local purge=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --target)
        shift
        target_arg="${1:-}"
        [[ -n "$target_arg" ]] || die "--target requires an argument"
        ;;
      --purge)
        [[ "$subcmd" == "uninstall" ]] || die "--purge is only valid with 'uninstall'"
        purge=1
        ;;
      *) die "Unknown option: $1" ;;
    esac
    shift || true
  done

  local targets
  targets="$(resolve_targets "$target_arg")"

  # pi joined this loop in pi-longterm-mem-mcp (R-005): its MCP config is
  # mcp.json inside the built package (pipkg_dest_dir), not a per-runtime
  # config file, so it gets --config-root below instead of the module's
  # own default. `register`/`unregister --target pi` apply the SAME
  # explicit-vs-all probe asymmetry as claude/opencode/codex's missing
  # config (register_paths.go's piInstalled): under --target all a pi
  # package nobody has run `pi install` on yet is skipped, softened by
  # expanded_all below exactly like an absent codex config is; named
  # explicitly it is a real failure.
  if [[ "$target_arg" == "all" ]]; then
    local copy_targets="" ct
    for ct in $targets; do
      is_copy_target "$ct" && copy_targets+="$ct "
    done
    targets="${copy_targets}pi"
  fi

  ensure_engine_binary

  if [[ "$subcmd" == "install" ]]; then
    command -v go >/dev/null 2>&1 || die "go is not installed; required to build longterm-mem"
    [[ -d "$LONGTERM_MEM_SRC" ]] || die "longterm-mem module not found at $LONGTERM_MEM_SRC"

    info "Building longterm-mem binary..."
    # Every step of the deploy below is checked EXPLICITLY rather than left
    # to 'set -e'. cmd_longterm_mem is reached from a condition context in
    # cmd_apply, and bash suppresses errexit for the whole of a function
    # called that way — an unchecked mkdir, mktemp, chmod or mv there
    # deploys nothing and still prints "binary deployed to".
    local binary_dir
    binary_dir="$(dirname "$LONGTERM_MEM_BINARY")"
    mkdir -p "$binary_dir" || die "could not create $binary_dir for the longterm-mem binary"
    local build_tmp
    build_tmp="$(mktemp "$binary_dir/.longterm-mem.build.XXXXXX")" \
      || die "could not create a temporary build file in $binary_dir"
    if ! (cd "$LONGTERM_MEM_SRC" && go build -o "$build_tmp" ./cmd/longterm-mem); then
      rm -f "$build_tmp"
      die "go build failed for longterm-mem; binary not deployed"
    fi
    chmod +x "$build_tmp" || {
      rm -f "$build_tmp"
      die "could not make $build_tmp executable; longterm-mem binary not deployed"
    }
    mv "$build_tmp" "$LONGTERM_MEM_BINARY" || {
      rm -f "$build_tmp"
      die "could not install the longterm-mem binary at $LONGTERM_MEM_BINARY; not deployed"
    }
    info "longterm-mem binary deployed to: $LONGTERM_MEM_BINARY"

    # install_failed marks a register outcome that neither registered $t nor
    # converged, so the caller (cmd_apply's recovery line) hears about it on
    # the exit code. A refusal and a version skew are NOT that: both are
    # states every re-run reproduces identically, so both converge.
    local install_failed=""

    # expanded_all mirrors cmd_register.go's OWN distinction, and it has to,
    # because this loop takes that distinction away from it: register's
    # "--target all" means "every runtime this machine has" and SKIPS a
    # runtime whose configuration file is absent, while a target named
    # explicitly is a different statement and fails. Expanding "all" here
    # and calling register once per target — which the per-target tracking
    # below needs — turns every runtime the machine does not have into a
    # named-target hard failure. Escalating those would fail the run on any
    # machine that does not run all three runtimes.
    #
    # So a hard failure is escalated only for a target the operator named,
    # which is exactly the case register itself escalates. Under "all" it is
    # still warned about, and $t is still not recorded as installed.
    local expanded_all=""
    [[ "$target_arg" == "all" ]] && expanded_all=1

    local t
    for t in $targets; do
      # No --state-dir here: this must resolve to the exact same
      # install-state.json the uninstall path below reads (see that call's
      # comment) — the module's own default (defaultRegisterStateDir(),
      # ~/.labdrian-overlay/longterm-mem, D9) is that single source of
      # truth, not $STATE_DIR.
      #
      # --binary IS passed, and must be: it is the path written INTO each
      # runtime's MCP entry, and $LONGTERM_MEM_BINARY (derived from
      # $STATE_DIR) is where the binary was just deployed. register's own
      # default resolves from $HOME instead, so under a $STATE_DIR override
      # it wrote entries naming a path nothing was ever deployed to. Under
      # the default $STATE_DIR the two values are identical, so this changes
      # no byte of an ordinary install.
      #
      # register's exit status is INSPECTED, not discarded (cmd_register.go:
      # 0 registered, 1 hard failure, 2 usage/version skew, 6 an untagged
      # same-named entry it refuses to touch, 8 a path it could not resolve
      # from the environment at all). 9 is the module's ninth published code
      # and is deliberately absent from that list: it is `doctor`'s verdict
      # ("the checks ran and a VAULT check FAILED", exitDoctorChecksFailed),
      # not a register outcome, and register never raises it. It is named
      # here so the next reader does not read this list as the module's whole
      # contract -- and so that if this entrypoint ever calls `doctor`, 9 gets
      # its own arm there rather than falling into a catch-all that would
      # report a failed vault check as a generic failure.
      #
      # Only 0 means this entrypoint actually
      # registered $t, and only 0 records $t as installed — recording a
      # target register refused would make the binary-removal guard hold the
      # shared binary hostage on behalf of a runtime that never had an entry
      # of ours.
      #
      # Every code the module names has its own arm below, because the
      # catch-all arm carries a story ("a runtime this machine does not
      # have") that is only true for exit 1. A code that falls through to it
      # is not merely reported vaguely — it is reported WRONG.
      local register_exit=0 register_args=(--target "$t" --binary "$LONGTERM_MEM_BINARY")
      # pi's config root IS the built package (pipkg_dest_dir), not the
      # module's own per-runtime default.
      [[ "$t" == "pi" ]] && register_args+=(--config-root "$(pipkg_dest_dir)")
      "$LONGTERM_MEM_BINARY" register "${register_args[@]}" \
        || register_exit=$?

      # The engine record is rebuilt from OBSERVED state on every call and
      # never records an entry it cannot prove it wrote, so it runs for
      # every outcome above: after a refusal it is what reports $t's entry
      # as the unmanaged entry it is, and skipping it would leave a stale
      # record behind instead.
      "$ENGINE_BINARY" runtime install --component longterm-mem --target "$t" --state-dir "$STATE_DIR" \
        || warn "engine runtime install --component longterm-mem --target $t reported a non-supported status"

      case "$register_exit" in
        0)
          longtermmem_targets record --target "$t" || exit 1
          ;;
        6)
          warn "longterm-mem register --target $t: refused — an entry exists that longterm-mem does not own, and it was left untouched. $t is NOT recorded as installed; remove that entry by hand and re-run install to register $t."
          ;;
        2)
          # Exit 2 fails the run for the exact reason exit 8 does, one arm
          # below: it is environment-independent, it reproduces identically
          # on every target (so --target all softens nothing here either),
          # and converging on it at exit 0 would report a component that
          # registered NOTHING as a clean install. The two codes differ only
          # in whose fault they name -- 8 the caller's environment, 2 this
          # binary's own vintage -- and neither difference is visible to a
          # script reading $?.
          #
          # Exit 6 above is deliberately NOT in this bucket: it is not a
          # failure to register but a machine state (a same-named entry this
          # overlay never wrote), the install did everything it is allowed to
          # do, and re-running changes nothing until a human removes that
          # entry. Failing the run there would make every install on such a
          # machine non-zero forever.
          warn "longterm-mem register --target $t: this binary does not accept the invocation (exit 2, version skew), so $t was not registered and is NOT recorded as installed. Not retried; every re-run reproduces this -- deploy a longterm-mem build whose register subcommand matches this overlay."
          install_failed=1
          ;;
        8)
          # register_paths.go separates 8 from 1 precisely so a caller can
          # tell "you forgot a flag / your environment is broken" from "your
          # ~/.claude.json is broken": nothing was attempted and nothing was
          # touched. It is therefore NOT softened under --target all the way
          # exit 1 is. Exit 1 under "all" is genuinely ambiguous with "this
          # machine does not run that runtime"; exit 8 is not ambiguous at
          # all — it is one environment fault that every target reproduces
          # identically, and converging on it at exit 0 would report a
          # completely uninstalled component as a clean run.
          warn "longterm-mem register --target $t: exit 8 — register could not resolve a path from the environment at all (no usable HOME means no runtime config root and no install-state.json directory), so nothing was attempted and $t is NOT recorded as installed. The fix is the environment, not the runtime: set HOME to a writable home directory and re-run install."
          install_failed=1
          ;;
        *)
          if [[ -n "$expanded_all" ]]; then
            warn "longterm-mem register --target $t failed (exit $register_exit); $t was not registered and is NOT recorded as installed. Not failing the run: --target all asks for the runtimes this machine has, and a runtime it does not have looks exactly like this from here."
          else
            warn "longterm-mem register --target $t failed (exit $register_exit); $t was not registered and is NOT recorded as installed"
            install_failed=1
          fi
          ;;
      esac
    done

    if [[ -n "$install_failed" ]]; then
      return 1
    fi
  elif [[ "$subcmd" == "status" ]]; then
    # The engine exits non-zero for CapabilityUnsupported/CapabilityPartial.
    # Swallowing that into a bare warn made this branch return 0, and the TUI
    # maps a zero exit to a green "Completado" -- so a component installed on
    # no runtime at all was reported as complete, with the warning sentence
    # sitting right underneath the tick.
    #
    # The exit is 2, not 1, and the distinction is the whole point: `status`
    # is a READ-ONLY REPORT and it SUCCEEDED at reporting. 1 would claim the
    # status command itself failed, which is false, and the TUI paints that as
    # a hard failure. 2 is the code the TUI already renders as DEGRADED, which
    # is exactly what "reported, and what it reported is unhealthy" means.
    #
    # The warn stays: the operator still needs the sentence.
    if ! "$ENGINE_BINARY" runtime status --component longterm-mem --state-dir "$STATE_DIR"; then
      warn "engine runtime status --component longterm-mem reported a non-supported status"
      return 2
    fi
  else
    local t
    # uninstall_failed tracks whether any target's unregister call returned
    # a status other than 0 (removed) or 6 (unmanaged) -- see the case
    # statement below. It is the guard that keeps a real failure (state dir
    # unreadable, a corrupted install-state.json, ...) from silently
    # clearing this run's tracking and letting the binary-removal branch
    # below fire for a target whose entry may still be sitting in its
    # runtime config.
    local uninstall_failed=""

    # Selective removal needs a usable binary. When it is absent or not
    # executable there is nothing for the operator to retry: every re-run
    # reproduces the same state, so treating it as a recoverable failure
    # would strand the engine record and the shared binary forever, with
    # --purge -- the one action that orphans the entries -- as the only
    # escape. Say plainly what could not be done, then converge.
    local unregister_usable=1
    if [[ ! -x "$LONGTERM_MEM_BINARY" ]]; then
      unregister_usable=""
      warn "longterm-mem: $LONGTERM_MEM_BINARY is missing or not executable, so the per-runtime MCP entries cannot be removed. Any entry longterm-mem installed stays in its runtime config -- remove it by hand, or reinstall and re-run uninstall."
    fi

    for t in $targets; do
      if [[ -z "$unregister_usable" ]]; then
        longtermmem_targets release --target "$t" || exit 1
        continue
      fi
      # unregister is module-owned, real per-runtime selective removal
      # (R-019, 12b.4/12b.5): it removes only $t's own ownership-tagged MCP
      # entry from $t's own runtime config, byte-preserving everything
      # else, and clears $t's own install-state.json record. It never
      # touches another target's entry or record, which is exactly what
      # makes releasing $t from the installed-targets record below, and the
      # binary guard after it (the engine's `installed-targets assess`), safe
      # to key off that record alone.
      #
      # No --state-dir here either: it MUST resolve to the exact same
      # install-state.json the install path above wrote to (register's own
      # default, D9) -- passing $STATE_DIR here was the CRITICAL-1 defect
      # (install and uninstall disagreeing on where install-state.json
      # lives, so uninstall could never find a record install actually
      # wrote, and reported every entry as unmanaged/exit 6 instead of
      # removing it).
      #
      # cmd_unregister.go's exit codes (0 removed, 1 hard failure, 2 usage,
      # 6 unmanaged, 8 a path it could not resolve from the environment at
      # all, 1 outranking 6) are inspected, not swallowed. 9 is not in that
      # set: it is `doctor`'s "a vault check FAILED" verdict and unregister
      # never raises it (see the register call's comment above). ONLY exit 0 means
      # "$t has nothing of ours left in its runtime config", and only exit 0
      # clears this target's tracking below.
      #
      # 6 used to clear it too, on the reasoning that "an unmanaged entry was
      # never ours to begin with". That reasoning describes the entry, not
      # the run: exit 6 says only that unregister DID NOT REMOVE the entry it
      # found, and unregister cannot tell "someone else's server that happens
      # to share our name" from "our own entry, after install-state.json was
      # lost or restored from a backup that predates the install". Clearing
      # tracking on the second shape frees the guard below to delete the
      # SHARED BINARY while our MCP entries still sit in every runtime config
      # -- orphaned entries pointing at a binary that no longer exists, which
      # is the single state this tracking file exists to prevent. So 6 keeps
      # $t tracked; --purge stays the explicit way out.
      #
      # 2 is the same shape and the same hazard, and is nonetheless treated
      # the other way, for the reason its own arm gives: exit 6 leaves a
      # recovery open (fix the entry by hand, or let `register` adopt it
      # back) while exit 2 leaves none, because the tool that would perform
      # the recovery is the thing that is broken.
      #
      # 8 gets its own arm for the same reason it does on the install side:
      # it lands in the same bucket as the catch-all, but the catch-all's
      # message ("failed") names no remedy, and 8's remedy is the one the
      # operator can actually act on.
      local unregister_exit=0 unregister_args=(--target "$t")
      [[ "$t" == "pi" ]] && unregister_args+=(--config-root "$(pipkg_dest_dir)")
      "$LONGTERM_MEM_BINARY" unregister "${unregister_args[@]}" || unregister_exit=$?
      case "$unregister_exit" in
        0)
          longtermmem_targets release --target "$t" || exit 1
          ;;
        6)
          warn "longterm-mem unregister --target $t: unmanaged (an entry exists that longterm-mem does not own; left untouched). $t stays tracked as installed, because that entry was NOT removed and may be ours under a lost install-state.json -- inspect $t's config and remove the entry by hand, then re-run uninstall, or pass --purge to drop the shared binary regardless."
          ;;
        2)
          # Usage error: this binary does not accept the invocation
          # (version skew, a subcommand it does not know). It is the same
          # "the entry was not removed" shape as exit 6 above, and the same
          # orphan hazard, with ONE difference that decides it the other
          # way: the tool itself is unusable, so no re-run of `uninstall`
          # can ever remove the entry or clear the tracking, and keeping $t
          # tracked would leave --purge -- the action that orphans the
          # entries outright -- as the only way to finish. Exit 6 has a
          # real recovery (fix the entry, or let `register` adopt it back);
          # this has none until the binary is replaced. So it converges and
          # clears tracking, and says what was left behind.
          #
          # engine/installer's TestUninstall_VersionSkewStillConverges pins
          # this branch, alongside the missing-binary branch it mirrors.
          warn "longterm-mem unregister --target $t: this binary does not accept the invocation (exit 2, version skew), so $t's entry stays in its runtime config -- remove it by hand, or reinstall and re-run uninstall. Not retried; every re-run reproduces this."
          longtermmem_targets release --target "$t" || exit 1
          ;;
        8)
          # Unlike exit 2, this one IS operator-fixable and a re-run after
          # the fix succeeds, so it keeps $t tracked and fails the run --
          # the same treatment the catch-all gives, with the remedy named.
          warn "longterm-mem unregister --target $t: exit 8 — unregister could not resolve a path from the environment at all (no usable HOME means no runtime config root and no install-state.json directory), so nothing was attempted and $t's entry may still be in its runtime config. Keeping $t tracked as installed. The fix is the environment, not the runtime: set HOME to a writable home directory and re-run uninstall."
          uninstall_failed=1
          ;;
        *)
          # A real, operator-fixable failure (unreadable config, corrupted
          # install-state.json, permissions): keep $t tracked so the guard
          # below refuses to remove the shared binary while $t's entry may
          # still be present, and re-running after the fix can succeed.
          warn "longterm-mem unregister --target $t failed (exit $unregister_exit); keeping it tracked as installed"
          uninstall_failed=1
          ;;
      esac
    done

    # Whether the shared binary and the engine's registration record go is the
    # engine's to say, from the record of the installed targets as it is now
    # (`assess`). It fails CLOSED: no record at all means "this entrypoint does
    # not know what is installed", never "nothing is installed" -- another code
    # path, a lost file or a reset STATE_DIR all read empty while other runtimes
    # may still depend on the shared binary -- and a record that cannot be read
    # is the same not-knowing; --purge is the one explicit way to force removal.
    # A target that failed to unregister outranks both: neither the registration
    # record nor the binary is safe to remove while its entry may still be in
    # its runtime's config.
    #
    # 0 says they may go; 3 says they are kept and has said why (on the stream
    # this script always said it on); anything else is the engine not answering
    # (too old to know the command, or a failure), which keeps them as well.
    local assess_args=() assess_exit=0
    [[ -n "$purge" ]] && assess_args+=(--purge)
    [[ -n "$uninstall_failed" ]] && assess_args+=(--unregister-failed)
    longtermmem_targets assess ${assess_args[@]+"${assess_args[@]}"} || assess_exit=$?

    case "$assess_exit" in
      0)
        # Full uninstall: safe to wipe the engine-owned registration record
        # too. LongtermMemAdapter.Uninstall() removes the WHOLE record in one
        # call regardless of --target -- the per-runtime config entries
        # themselves were already removed selectively above, by unregister;
        # this only clears engine/runtime's own separate doctor/status
        # tracking (D4), and calling it only once every tracked target is
        # gone (or --purge forces it) is what keeps this call from wiping
        # another still-installed target's record.
        "$ENGINE_BINARY" runtime uninstall --component longterm-mem --state-dir "$STATE_DIR" \
          || warn "engine runtime uninstall --component longterm-mem reported a non-supported status"
        # The binary and the record go only if the record, read again under the
        # lock, still says that nothing depends on the binary: a target recorded
        # since the answer above keeps it. The path is the one the binary was
        # deployed to and written into every runtime's entry.
        local retire_args=(--binary "$LONGTERM_MEM_BINARY")
        [[ -n "$purge" ]] && retire_args+=(--purge)
        longtermmem_targets retire "${retire_args[@]}" || exit 1
        ;;
      3)
        ;;
      *)
        # Neither of the two answers. An engine too old to have the command ends with exit 1 and its own
        # "unknown subcommand", and is told from one that knows the command and failed (a crash, a kill, a
        # program that could not be run, a code of its own) the way doctor_report_registry tells them:
        # the first is stale and is rebuilt, the second is a fault and is read from what it said. Either
        # way the registration record and the binary stay, and the run fails.
        if [[ "$assess_exit" -eq 1 ]] && ! longtermmem_engine_knows_installed_targets; then
          warn "longterm-mem: the engine at $ENGINE_BINARY does not know 'installed-targets' (it is older than this overlay), so whether any runtime still depends on the shared binary is unknown; leaving the registration record and the shared binary untouched. Rebuild it with 'labdrian doctor --fix' and re-run uninstall."
        else
          warn "longterm-mem: the engine at $ENGINE_BINARY ended with code $assess_exit on 'installed-targets assess'; only 0 (the registration record and the shared binary may go) and 3 (they are kept) are answers, so whether any runtime still depends on the shared binary is unknown; leaving the registration record and the shared binary untouched. Fix what it said above, or check the engine with 'labdrian doctor', and re-run uninstall."
        fi
        uninstall_failed=1
        ;;
    esac

    # Surface the failure on the process exit code even when --purge forced
    # cleanup to proceed anyway -- the caller still asked for every target
    # and at least one did not actually get unregistered cleanly.
    if [[ -n "$uninstall_failed" ]]; then
      return 1
    fi
  fi
}
