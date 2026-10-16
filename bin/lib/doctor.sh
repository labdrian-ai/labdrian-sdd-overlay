# shellcheck shell=bash
#
# doctor
#
# The doctor command: a read-only host-toolchain preflight, and --fix, which repairs the engine
# binary and provisions the discovery tools.
#
# Sourced by bin/labdrian-overlay in the order it lists, never run: it defines functions and
# variables and does nothing else.

# doctor_report_engine_binary prints the engine binary's state as one doctor
# line: absent / stale / current. Three states, because they need three
# different remedies -- "present" alone was the gap that let an eleven-day-old
# engine keep executing every hook unreported. Staleness is asked of
# engine_binary_is_stale so the script keeps exactly ONE staleness rule.
# Neither problem state is a hard FAIL: a stale engine usually still serves the
# command at hand, and an absent one is repairable from here.
#
# It is a function rather than an inline block so --fix can RE-RUN it after
# rebuilding, the way --fix already re-runs recheck_discovery_tools after
# provisioning. Without that, a --fix run left its own "is STALE" line standing
# as the report's only state for the binary, contradicted further down by
# "Engine binary rebuilt" -- a report that stops describing reality before it
# finishes.
doctor_report_engine_binary() {
  if [[ ! -x "$ENGINE_BINARY" ]]; then
    echo "WARN  engine binary not found at $ENGINE_BINARY — nothing the overlay hands to the engine can run. Rebuild it with 'labdrian doctor --fix' (or 'labdrian install-hooks', which also rewires ~/.claude/settings.json)."
  elif engine_binary_is_stale; then
    echo "WARN  $(engine_stale_diagnosis)"
  else
    echo "PASS  engine binary present and current at $ENGINE_BINARY"
  fi
}

# doctor_report_registry prints, as one doctor line, what the engine says of the
# skill registry of the project doctor looks at. The judgment is the engine's
# (`status --only registry`, the same check `status-hooks` makes), so the
# overlay has one rule for what a registry may be: absent is quiet (a project
# that does not use the overlay is no problem), empty, white space only or
# unreadable is a FAIL (never taken for "zero skills"), and one that lacks a
# scoped block warns. This function only chooses the project -- the overlay
# repository's registry when it has one, else the directory doctor runs in --
# and puts the engine's level on the line: ok is PASS, a warning is WARN, a
# failure is FAIL and the function returns 1, for the caller to fail the run.
#
# Doctor must still say something when the engine cannot answer, because
# diagnosing a missing engine is part of its job: an absent binary, or one so
# old that it does not know the question, is a WARN that says the registry was
# not checked, never a silent pass. An engine that ran and ended with a code it
# does not give for a report is a fault, and is a FAIL that fails doctor.
doctor_report_registry() {
  local dir="$PWD"
  [[ -f "$OVERLAY_DIR/.atl/skill-registry.md" ]] && dir="$OVERLAY_DIR"

  if [[ ! -x "$ENGINE_BINARY" ]]; then
    echo "WARN  registry not checked: the engine judges it and its binary is not at $ENGINE_BINARY. Rebuild it with 'labdrian doctor --fix'."
    return 0
  fi

  local report rc=0
  report="$("$ENGINE_BINARY" status --only registry --cwd "$dir" 2>&1)" || rc=$?
  # The engine's codes for a report are 0 (ok), 2 (a warning and no failure) and 1
  # (a failure); a usage error is 1 too, but it has no line of the shape below. An
  # engine that ran and ended with any other code (a crash, a kill, a program that
  # could not be run, a code of its own) is not an engine that left the registry
  # unchecked for want of one, it is an engine that failed: FAIL, and doctor fails
  # with it. A line printed before such a code may be half an answer, so it is not
  # shown.
  case "$rc" in
    0 | 1 | 2) ;;
    *)
      echo "FAIL  registry not checked: the engine at $ENGINE_BINARY ended with code $rc on 'status --only registry'; only 0 (ok), 2 (a warning) and 1 (a failure) are answers. Rebuild it with 'labdrian doctor --fix'."
      return 1
      ;;
  esac
  # One line, "[LEVEL] registry: ...": an engine older than this script ignores
  # --only and prints its whole report, and then none of it is an answer.
  if [[ "$report" != "["*"] registry: "* || "$report" == *$'\n'* ]]; then
    echo "WARN  registry not checked: the engine at $ENGINE_BINARY did not answer 'status --only registry' (is it older than this overlay?). Rebuild it with 'labdrian doctor --fix'."
    return 0
  fi
  local line="${report#*] }"
  case "$rc" in
    0) echo "PASS  $line" ;;
    2) echo "WARN  $line" ;;
    1) echo "FAIL  $line"; return 1 ;;
  esac
}

# doctor_fix_engine_binary is doctor --fix's half of "update the engine on
# purpose". ensure_engine_binary heals silently on the way to running
# something; this is the DELIBERATE repair, and until it existed the only way
# to rebuild the engine was 'labdrian install-hooks', which also rewrites
# ~/.claude/settings.json — a hammer for this tack, and so nobody ran it after
# a repository update.
#
# It never fails the run: doctor's engine-binary check is a WARN, and a repair
# that could not happen is REPORTED as not-happened rather than claimed. That
# is the whole point of the check — a repair reported but not performed is the
# same invisible failure mode in a new place.
doctor_fix_engine_binary() {
  local reason
  if [[ ! -x "$ENGINE_BINARY" ]]; then
    reason="absent"
  elif engine_binary_is_stale; then
    reason="stale"
  else
    echo "Engine binary already current at $ENGINE_BINARY — nothing to rebuild."
    return 0
  fi

  if ! command -v go >/dev/null 2>&1; then
    echo "COULD NOT REPAIR the $reason engine binary at $ENGINE_BINARY: go is not on PATH. Install a Go toolchain, then run 'labdrian doctor --fix' again."
    return 0
  fi
  if [[ ! -d "$ENGINE_SRC" ]]; then
    echo "COULD NOT REPAIR the $reason engine binary at $ENGINE_BINARY: engine source not found at $ENGINE_SRC."
    return 0
  fi

  echo "Rebuilding the $reason engine binary from $ENGINE_SRC..."
  mkdir -p "$(dirname "$ENGINE_BINARY")"
  if (cd "$ENGINE_SRC" && go build -o "$ENGINE_BINARY" ./cmd/); then
    echo "Engine binary rebuilt: $ENGINE_BINARY"
  else
    echo "COULD NOT REPAIR the $reason engine binary at $ENGINE_BINARY: go build failed (see the build output above)."
  fi
}

# Read-only host-toolchain preflight. Prints PASS / WARN / FAIL per check and
# exits non-zero only on a hard FAIL (missing REQUIRED tool or empty registry).
#
# Honors the three robustness principles:
#   - PORTABLE DISCOVERY: every tool is probed with 'command -v'. Never calls
#     fd/eza/bat directly.
#   - FAIL-LOUD: no bare '2>/dev/null' on discovery; command-not-found stays
#     visible. A missing required tool or an empty registry is a hard FAIL with
#     an actionable message.
#   - REGISTRY-AUTHORITATIVE: the registry file is the source of truth, and the
#     engine judges it (doctor_report_registry): an empty or whitespace-only
#     registry is reported as FAIL ("inconclusive, not zero skills"), never
#     silently passed.
#
# Classification:
#   FAIL  -> registry empty/whitespace-only or unreadable; go absent; an engine that
#            ran and ended with a code it does not give for a report
#   WARN  -> registry without a scoped block, or not checked (no engine to ask);
#            gentle-ai absent; engine binary absent; fd/eza/bat absent
#   (a registry that is absent is PASS: a project that does not use the overlay is no problem)
#   PASS  -> check satisfied

cmd_doctor() {
  local fail=0
  local fix=0

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --fix) fix=1 ;;
      *) die "Unknown option: $1. Usage: labdrian doctor [--fix]" ;;
    esac
    shift || true
  done

  echo "labdrian doctor — host-toolchain preflight"
  echo ""

  # (a) REQUIRED: go on PATH (needed to build the engine binary).
  if command -v go >/dev/null 2>&1; then
    echo "PASS  go on PATH"
  else
    echo "FAIL  go not found on PATH — required to build the engine binary (run 'labdrian install-hooks')."
    fail=1
  fi

  # (b) REQUIRED-with-context: gentle-ai on PATH. The overlay layers onto
  #     gentle-ai; absence is a loud WARN, not a hard FAIL (overlay can still
  #     install and run, but propagation/scoping may be inert).
  if command -v gentle-ai >/dev/null 2>&1; then
    echo "PASS  gentle-ai on PATH"
  else
    echo "WARN  gentle-ai not found on PATH — the overlay layers onto gentle-ai; skill propagation/scoping may be inert until it is installed."
  fi

  # (c) bat / rg / fd / sd / eza — the full set the persona rule mandates.
  #     Absence is a WARN, never a hard FAIL: the safety guard is the fallback.
  #     Run 'labdrian doctor --fix' to auto-install via Homebrew when available.
  local missing_discovery=()
  local t
  for t in "${DISCOVERY_TOOLS[@]}"; do
    command -v "$t" >/dev/null 2>&1 || missing_discovery+=("$t")
  done
  if [[ ${#missing_discovery[@]} -eq 0 ]]; then
    echo "PASS  bat/rg/fd/sd/eza on PATH"
  else
    echo "WARN  not found: ${missing_discovery[*]}. Without these tools agents that run"
    echo "      e.g. 'fd ... 2>/dev/null' get empty output (command not found, swallowed)"
    echo "      and may misread it as zero skills — the silent-empty footgun."
    echo "      Run 'labdrian doctor --fix' to auto-install via Homebrew (if available)."
    echo "      Skill discovery MUST use the registry (.atl/skill-registry.md or engram),"
    echo "      never a bare filesystem scan. An empty result is INCONCLUSIVE, not zero skills."
  fi

  # (d) Engine binary: absent / stale / current. Three states, because they
  #     need three different remedies — "present" alone was the gap that let an
  #     eleven-day-old engine keep executing every hook unreported. Staleness is
  #     asked of engine_binary_is_stale so the script keeps exactly ONE staleness
  #     rule. Neither state is a hard FAIL: a stale engine usually still serves
  #     the command at hand, and an absent one is repairable from here.
  doctor_report_engine_binary

  # (e) REGISTRY-AUTHORITATIVE + FAIL-LOUD: the skill registry of the project,
  #     judged by the engine (doctor_report_registry), not by a second rule of
  #     this script. An empty, white-space-only or unreadable registry is a hard
  #     FAIL; one that lacks a scoped block is a WARN.
  doctor_report_registry || fail=1

  # (f) overlay-release-surfacing R-001: per-target deployed-version digest
  # consistency. A target with no recorded state (never deployed) has
  # nothing to compare yet -- silently skipped, not reported as a failure
  # or warning. A digest mismatch is a WARN, never a FAIL: it never touches
  # `fail`, preserving doctor's "exits non-zero only on a hard FAIL"
  # contract (R-001's explicit requirement).
  local dt drecorded drecorded_version drecorded_digest dtarget_digest
  for dt in "${!TARGET_PATHS[@]}"; do
    drecorded="$(state_read_target "$dt")"
    [[ "$drecorded" == "NEVER_DEPLOYED" ]] && continue
    IFS=$'\t' read -r drecorded_version drecorded_digest _ <<< "$drecorded"
    dtarget_digest="$(compute_target_digest "$dt" live)"
    if [[ "$dtarget_digest" == "$drecorded_digest" ]]; then
      echo "PASS  $dt: deployed at $drecorded_version, digest matches"
    else
      echo "WARN  $dt: deployed at $drecorded_version, digest drift detected -- run 'labdrian apply --target $dt'"
    fi
  done

  echo ""
  if [[ "$fail" -ne 0 ]]; then
    echo "doctor: FAIL — one or more required checks did not pass (see above)." >&2
    # Still run --fix provisioning if requested, then re-check tools, but exit 1 after.
    if [[ "$fix" -eq 1 ]]; then
      echo ""
      echo "==> --fix: engine binary..."
      doctor_fix_engine_binary
      echo ""
      echo "==> Re-checking the engine binary after --fix..."
      doctor_report_engine_binary
      echo ""
      echo "==> --fix: attempting to provision discovery tools despite FAIL above..."
      ensure_discovery_tools
      echo ""
      echo "==> Re-checking discovery tools after provisioning..."
      recheck_discovery_tools
    fi
    exit 1
  fi
  echo "doctor: OK — required checks passed (review any WARN above)."

  # --fix: explicit opt-in to provision missing discovery tools, then re-check.
  if [[ "$fix" -eq 1 ]]; then
    echo ""
    echo "==> --fix: engine binary..."
    doctor_fix_engine_binary
    echo ""
    echo "==> Re-checking the engine binary after --fix..."
    doctor_report_engine_binary
    echo ""
    echo "==> --fix: provisioning discovery tools (${DISCOVERY_TOOLS[*]})..."
    ensure_discovery_tools
    echo ""
    echo "==> Re-checking discovery tools after provisioning..."
    recheck_discovery_tools
  fi
}
