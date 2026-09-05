#!/usr/bin/env bash
#
# Shell-level tests for bin/labdrian-overlay's longterm-mem install path.
#
# This repository had no shell test harness at all, which is why the shell
# hazards this file guards went unnoticed: every other suite here is a Go
# suite, and none of them execute a single line of bin/labdrian-overlay.
#
# It is a plain bash script on purpose — no new dependency, runnable by hand
# ("engine/shelltest/overlay_longterm_mem_test.sh") — and it is wired into CI
# through overlay_shell_test.go, so the existing "Engine Tests" job runs it
# with no workflow change.
#
# bin/labdrian-overlay guards its own dispatch with a BASH_SOURCE check
# precisely so it can be sourced and its internal functions called directly;
# that is the seam every case below uses.

set -uo pipefail

SCRIPT_DIR="$(cd -P "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
OVERLAY="$REPO_ROOT/bin/labdrian-overlay"

if [[ ! -f "$OVERLAY" ]]; then
  echo "FAIL - overlay entrypoint not found at $OVERLAY" >&2
  exit 1
fi

failures=0
pass() { echo "ok   - $*"; }
fail() {
  echo "FAIL - $1" >&2
  shift
  [[ $# -gt 0 ]] && printf '       %s\n' "$@" >&2
  failures=$((failures + 1))
}

work_root="$(mktemp -d "${TMPDIR:-/tmp}/overlay-shelltest.XXXXXX")"
cleanup() { rm -rf "$work_root"; }
trap cleanup EXIT

# new_case_dir prints a fresh, empty working directory for one case.
new_case_dir() {
  local dir="$work_root/$1"
  mkdir -p "$dir"
  printf '%s\n' "$dir"
}

# ---------------------------------------------------------------------------
# hazard (a): the installed-targets tracking file
# ---------------------------------------------------------------------------

# Concurrent adds must not lose a target. Two overlay invocations racing on
# the same tracking file is the whole reason this file is written through a
# temporary rather than in place; a read-modify-write through one FIXED,
# shared temporary path makes the race worse, not better, because both
# writers truncate the same file before either renames it.
case_parallel_adds_keep_every_target() {
  local dir status out
  dir="$(new_case_dir parallel-adds)"

  out="$(
    STATE_DIR="$dir/state" bash -c '
      source "$1"
      mkdir -p "$(dirname "$LONGTERM_MEM_INSTALLED_TARGETS")"
      for t in claude opencode codex; do
        longtermmem_installed_targets_add "$t" &
      done
      wait
      longtermmem_installed_targets_read
    ' _ "$OVERLAY" 2>&1
  )"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "parallel adds: the run itself failed (exit $status)" "$out"
    return
  fi

  local t missing=()
  for t in claude opencode codex; do
    grep -q -x -F -e "$t" <<<"$out" || missing+=("$t")
  done
  if [[ ${#missing[@]} -gt 0 ]]; then
    fail "parallel adds: lost target(s): ${missing[*]}" "tracked: $(tr '\n' ' ' <<<"$out")"
    return
  fi
  pass "parallel adds keep every target"
}

# A per-process temporary is provable without racing anything: make the one
# FIXED shared path hostile and see whether the write still lands. A
# directory at "<tracking-file>.tmp" is exactly what a crashed concurrent
# writer, or a stray mkdir, can leave behind — with the shared name the
# redirect and the rename both fail against it forever.
case_add_does_not_use_a_shared_temp_path() {
  local dir status out
  dir="$(new_case_dir add-shared-temp)"

  out="$(
    STATE_DIR="$dir/state" bash -c '
      source "$1"
      mkdir -p "$(dirname "$LONGTERM_MEM_INSTALLED_TARGETS")"
      mkdir -p "${LONGTERM_MEM_INSTALLED_TARGETS}.tmp"
      longtermmem_installed_targets_add claude
      longtermmem_installed_targets_read
    ' _ "$OVERLAY" 2>&1
  )"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "add with a hostile shared temp path: run failed (exit $status)" "$out"
    return
  fi
  if ! grep -q -x -F -e claude <<<"$out"; then
    fail "add with a hostile shared temp path: claude was not recorded" "output: $out"
    return
  fi
  pass "add does not use a shared temp path"
}

case_remove_does_not_use_a_shared_temp_path() {
  local dir status out
  dir="$(new_case_dir remove-shared-temp)"

  out="$(
    STATE_DIR="$dir/state" bash -c '
      source "$1"
      mkdir -p "$(dirname "$LONGTERM_MEM_INSTALLED_TARGETS")"
      printf "claude\nopencode\n" > "$LONGTERM_MEM_INSTALLED_TARGETS"
      mkdir -p "${LONGTERM_MEM_INSTALLED_TARGETS}.tmp"
      longtermmem_installed_targets_remove claude
      longtermmem_installed_targets_read
    ' _ "$OVERLAY" 2>&1
  )"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "remove with a hostile shared temp path: run failed (exit $status)" "$out"
    return
  fi
  if grep -q -x -F -e claude <<<"$out"; then
    fail "remove with a hostile shared temp path: claude was not removed" "output: $out"
    return
  fi
  if ! grep -q -x -F -e opencode <<<"$out"; then
    fail "remove with a hostile shared temp path: opencode was dropped" "output: $out"
    return
  fi
  pass "remove does not use a shared temp path"
}

# slow_the_tracking_read emits shell source that replaces
# longtermmem_installed_targets_read with a wrapper that returns the same
# lines and the same status, then sleeps.
#
# A lost update is a timing bug, and a timing bug asserted by luck is a test
# that passes on a fast machine while the bug is still there. Widening the
# read-modify-write window by a fixed half second makes the interleaving
# deterministic in BOTH directions: without serialization the two cycles
# provably overlap and one write provably lands on stale state, and with
# serialization the second cycle provably waits (the delay is spent INSIDE
# the critical section, because every writer reads there).
slow_the_tracking_read() {
  cat <<'SLOW'
      eval "longtermmem_installed_targets_read_real() $(declare -f longtermmem_installed_targets_read | tail -n +2)"
      longtermmem_installed_targets_read() {
        local rc=0
        longtermmem_installed_targets_read_real "$@" || rc=$?
        sleep 0.5
        return "$rc"
      }
SLOW
}

# Two concurrent REMOVES are the twin of the concurrent adds above, and the
# append that makes adds safe is unavailable to them: a removal is a real
# read-modify-write. Unserialized, both cycles read the same three targets
# and each writes back its own two, so whichever renames last silently
# resurrects the target the other one removed -- or, with the operands
# swapped, drops a target that is still installed, which is what lets the
# binary-removal guard delete the shared binary out from under a runtime
# that is still registered.
case_parallel_removes_keep_every_other_target() {
  local dir status out
  dir="$(new_case_dir parallel-removes)"

  out="$(
    STATE_DIR="$dir/state" bash -c '
      source "$1"
      mkdir -p "$(dirname "$LONGTERM_MEM_INSTALLED_TARGETS")"
      printf "claude\nopencode\ncodex\n" > "$LONGTERM_MEM_INSTALLED_TARGETS"
      '"$(slow_the_tracking_read)"'
      longtermmem_installed_targets_remove claude &
      longtermmem_installed_targets_remove opencode &
      wait
      longtermmem_installed_targets_read_real
    ' _ "$OVERLAY" 2>&1
  )"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "parallel removes: the run itself failed (exit $status)" "$out"
    return
  fi
  if ! grep -q -x -F -e codex <<<"$out"; then
    fail "parallel removes: dropped the untouched target codex" "tracked: $(tr '\n' ' ' <<<"$out")"
    return
  fi
  local t survived=()
  for t in claude opencode; do
    grep -q -x -F -e "$t" <<<"$out" && survived+=("$t")
  done
  if [[ ${#survived[@]} -gt 0 ]]; then
    fail "parallel removes: a removal was lost, still tracked: ${survived[*]}" "tracked: $(tr '\n' ' ' <<<"$out")"
    return
  fi
  pass "parallel removes keep every other target and lose no removal"
}

# The mixed race is the one that deletes a shared binary: a remove that read
# BEFORE a concurrent add rewrites the file from its stale copy, so the
# freshly installed target vanishes from tracking while its MCP entry stays
# in that runtime's config. The add is started first and the remove a beat
# later, so the stale read is deterministic rather than a coin flip.
case_add_racing_a_remove_is_not_lost() {
  local dir status out
  dir="$(new_case_dir add-racing-remove)"

  out="$(
    STATE_DIR="$dir/state" bash -c '
      source "$1"
      mkdir -p "$(dirname "$LONGTERM_MEM_INSTALLED_TARGETS")"
      printf "claude\ncodex\n" > "$LONGTERM_MEM_INSTALLED_TARGETS"
      '"$(slow_the_tracking_read)"'
      longtermmem_installed_targets_add opencode &
      sleep 0.1
      longtermmem_installed_targets_remove claude &
      wait
      longtermmem_installed_targets_read_real
    ' _ "$OVERLAY" 2>&1
  )"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "add racing a remove: the run itself failed (exit $status)" "$out"
    return
  fi
  if ! grep -q -x -F -e opencode <<<"$out"; then
    fail "add racing a remove: the concurrent add was lost" "tracked: $(tr '\n' ' ' <<<"$out")"
    return
  fi
  if ! grep -q -x -F -e codex <<<"$out"; then
    fail "add racing a remove: dropped the untouched target codex" "tracked: $(tr '\n' ' ' <<<"$out")"
    return
  fi
  if grep -q -x -F -e claude <<<"$out"; then
    fail "add racing a remove: the removal was lost" "tracked: $(tr '\n' ' ' <<<"$out")"
    return
  fi
  pass "an add racing a remove loses neither"
}

# The binary-removal guard is the THIRD writer of the tracking file, and the
# one with the worst blast radius: it reads the file, and on an empty read it
# deletes BOTH the shared binary and the tracking file. The two concurrency
# cases above pin `add` and `remove` only, so this site's lock could be
# deleted outright and the whole suite still passed -- the exact failure
# shape this file exists to catch, reproduced at the test level.
#
# The add is started first and the guard a beat later, so the stale read is
# deterministic: unserialized, the guard reads the file while the add is
# still inside its own read-modify-write, sees no remaining target, and
# removes the binary the add is registering a runtime against -- deleting the
# add's record along with it. Serialized, the guard waits, reads "opencode",
# and leaves both alone.
case_binary_removal_racing_an_add_is_serialized() {
  local dir status out
  dir="$(new_case_dir remove-binary-racing-add)"

  out="$(
    STATE_DIR="$dir/state" bash -c '
      source "$1"
      mkdir -p "$(dirname "$LONGTERM_MEM_INSTALLED_TARGETS")"
      mkdir -p "$(dirname "$LONGTERM_MEM_BINARY")"
      : > "$LONGTERM_MEM_INSTALLED_TARGETS"
      printf "not-a-real-binary\n" > "$LONGTERM_MEM_BINARY"
      '"$(slow_the_tracking_read)"'
      # The guard consults its callers $tracking_known: a tracking file that
      # exists at all is "known".
      tracking_known=1
      longtermmem_installed_targets_add opencode &
      sleep 0.1
      longtermmem_maybe_remove_binary "" &
      wait
      if [[ -e "$LONGTERM_MEM_BINARY" ]]; then
        echo "BINARY-PRESENT"
      else
        echo "BINARY-GONE"
      fi
      echo "AFTER=[$(longtermmem_installed_targets_read_real | tr "\n" " ")]"
    ' _ "$OVERLAY" 2>&1
  )"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "binary removal racing an add: the run itself failed (exit $status)" "$out"
    return
  fi
  if ! grep -q -F -e BINARY-PRESENT <<<"$out"; then
    fail "binary removal racing an add: the shared binary was deleted while a concurrent add was registering a runtime against it" "$out"
    return
  fi
  if ! grep -q -F -e "opencode" <<<"$out"; then
    fail "binary removal racing an add: the concurrent add was erased with the tracking file" "$out"
    return
  fi
  pass "the binary-removal guard is serialized against a concurrent add"
}

# The staleness escape must itself be mutually exclusive. Two invocations
# that both classify the SAME lock as abandoned each act on that
# classification independently, and an interleaving where the loser breaks in
# AFTER the winner has already taken the lock deletes the winner's LIVE lock
# and puts both inside the critical section -- exactly the lost update the
# lock exists to prevent, reached through the lock's own recovery path.
#
# The window is microseconds wide in production, so it is widened here the
# same way the tracking-file races are: a wrapper that makes the age test
# slow makes the classify-then-act gap deterministic rather than a coin flip.
# Both racers reach the stale branch; only one may end up holding.
case_simultaneous_stale_breakins_admit_only_one() {
  local dir status out entered refused
  dir="$(new_case_dir stale-break-in)"

  out="$(
    STATE_DIR="$dir/state" bash -c '
      source "$1"
      mkdir -p "$STATE_DIR"
      mkdir "$LONGTERM_MEM_TARGETS_LOCK"
      touch -d "-10 minutes" "$LONGTERM_MEM_TARGETS_LOCK" 2>/dev/null \
        || touch -A -001000 "$LONGTERM_MEM_TARGETS_LOCK"

      # Widen the gap between classifying the lock as stale and acting on
      # that classification. The age test goes through find, so wrapping find
      # delays every classification by a fixed half second.
      find() { command find "$@"; sleep 0.5; }

      # A refusal arrives through die, which exits the PROCESS, so the
      # critical section runs in its own subshell and the refusal is
      # observed from outside it -- the same seam case_failed_critical_
      # section_releases_the_lock uses.
      breaker() {
        (
          longtermmem_targets_lock_acquire 2>/dev/null
          echo "ENTERED-$1"
          sleep 0.4
          longtermmem_targets_lock_release
        ) || echo "REFUSED-$1"
      }

      breaker A &
      sleep 0.3
      breaker B &
      wait
    ' _ "$OVERLAY" 2>&1
  )"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "simultaneous stale break-ins: the run itself failed (exit $status)" "$out"
    return
  fi
  entered="$(grep -c -E "^ENTERED-" <<<"$out")"
  refused="$(grep -c -E "^REFUSED-" <<<"$out")"
  if [[ "$entered" -ne 1 ]]; then
    fail "simultaneous stale break-ins: $entered invocation(s) entered the critical section, expected exactly 1" "$out"
    return
  fi
  if [[ "$refused" -ne 1 ]]; then
    fail "simultaneous stale break-ins: $refused invocation(s) were refused, expected exactly 1" "$out"
    return
  fi
  pass "two simultaneous stale break-ins admit only one"
}

# A lock a failed critical section keeps forever is worse than no lock: it
# wedges every later invocation of this entrypoint. The failure is forced
# through the exact path this file uses to report unrecoverable problems --
# `die`, which exits the process rather than returning -- because that is
# the path a RETURN trap alone does not cover.
case_failed_critical_section_releases_the_lock() {
  local dir status out
  dir="$(new_case_dir lock-release-on-failure)"

  out="$(
    STATE_DIR="$dir/state" bash -c '
      source "$1"
      mkdir -p "$(dirname "$LONGTERM_MEM_INSTALLED_TARGETS")"
      printf "claude\n" > "$LONGTERM_MEM_INSTALLED_TARGETS"
      echo "LOCKPATH=[${LONGTERM_MEM_TARGETS_LOCK:-}]"
      # Break the staging step INSIDE the critical section, so the writer
      # dies while holding the lock. A subshell is how cmd_apply already
      # reaches this code, so `die` is observable instead of fatal here.
      mktemp() { return 1; }
      ( longtermmem_installed_targets_remove claude ) || echo "REMOVE-FAILED"
      unset -f mktemp
      if [[ -e "${LONGTERM_MEM_TARGETS_LOCK:-/nonexistent}" ]]; then
        echo "LOCK-HELD"
      else
        echo "LOCK-FREE"
      fi
      longtermmem_installed_targets_add opencode
      echo "AFTER=[$(longtermmem_installed_targets_read | tr "\n" " ")]"
    ' _ "$OVERLAY" 2>&1
  )"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "failed critical section: the run itself failed (exit $status)" "$out"
    return
  fi
  if ! grep -q -E '^LOCKPATH=\[.+\]$' <<<"$out"; then
    fail "failed critical section: no install-tracking lock path is defined" "$out"
    return
  fi
  if ! grep -q -F -e REMOVE-FAILED <<<"$out"; then
    fail "failed critical section: the forced staging failure did not fail the remove" "$out"
    return
  fi
  if ! grep -q -F -e LOCK-FREE <<<"$out"; then
    fail "failed critical section left the lock held; the next invocation is wedged" "$out"
    return
  fi
  if ! grep -q -F -e "opencode" <<<"$out"; then
    fail "failed critical section: the next invocation could not record a target" "$out"
    return
  fi
  pass "a failed critical section releases the lock"
}

# A tracking file that has picked up garbage — a partial write, a hand edit,
# a file from some other tool — must not wedge the binary-removal guard
# forever. Lines that do not name a target this entrypoint can install for
# are not install state, so they must not be read as "something is still
# installed".
case_corrupt_tracking_file_does_not_wedge_cleanup() {
  local dir status out
  dir="$(new_case_dir corrupt-tracking)"

  out="$(
    STATE_DIR="$dir/state" bash -c '
      source "$1"
      mkdir -p "$(dirname "$LONGTERM_MEM_INSTALLED_TARGETS")"
      printf "\x00garbage\n{not-a-target}\n\n" > "$LONGTERM_MEM_INSTALLED_TARGETS"
      mkdir -p "$(dirname "$LONGTERM_MEM_BINARY")"
      printf "#!/bin/sh\n" > "$LONGTERM_MEM_BINARY"
      chmod +x "$LONGTERM_MEM_BINARY"
      tracking_known=1
      longtermmem_maybe_remove_binary ""
      if [[ -e "$LONGTERM_MEM_BINARY" ]]; then
        echo "BINARY-STILL-PRESENT"
      else
        echo "BINARY-REMOVED"
      fi
    ' _ "$OVERLAY" 2>&1
  )"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "corrupt tracking file: run failed (exit $status)" "$out"
    return
  fi
  if ! grep -q -F -e BINARY-REMOVED <<<"$out"; then
    fail "corrupt tracking file wedged the binary-removal guard" "$out"
    return
  fi
  pass "a corrupt tracking file does not wedge binary cleanup"
}

# run_guard_case <case-dir-name> <printf-format-for-the-tracking-file> prints
# BINARY-STILL-PRESENT or BINARY-REMOVED for a tracking file with exactly
# that content, driving the same guard the uninstall path drives.
run_guard_case() {
  local dir="$1" content="$2"
  STATE_DIR="$dir/state" bash -c '
    source "$1"
    mkdir -p "$(dirname "$LONGTERM_MEM_INSTALLED_TARGETS")"
    printf "$2" > "$LONGTERM_MEM_INSTALLED_TARGETS"
    mkdir -p "$(dirname "$LONGTERM_MEM_BINARY")"
    printf "#!/bin/sh\n" > "$LONGTERM_MEM_BINARY"
    chmod +x "$LONGTERM_MEM_BINARY"
    tracking_known=1
    longtermmem_maybe_remove_binary ""
    if [[ -e "$LONGTERM_MEM_BINARY" ]]; then
      echo "BINARY-STILL-PRESENT"
    else
      echo "BINARY-REMOVED"
    fi
  ' _ "$OVERLAY" "$content" 2>&1
}

# Dropping a line the tracking file cannot vouch for is only safe for
# GARBAGE. A line that is a perfectly well-formed target name this build
# simply does not know is the opposite case: the likeliest way it got there
# is a NEWER build that installs for a runtime this one has never heard of,
# and the shared binary is exactly what that runtime is still running. So an
# unknown-but-well-formed name has to keep the guard CLOSED — reading it as
# "nothing is installed" deletes a binary out from under a live runtime.
case_unknown_wellformed_target_keeps_the_guard_closed() {
  local dir status out
  dir="$(new_case_dir unknown-wellformed)"

  out="$(run_guard_case "$dir" 'zed\n')"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "unknown well-formed target: run failed (exit $status)" "$out"
    return
  fi
  if ! grep -q -F -e BINARY-STILL-PRESENT <<<"$out"; then
    fail "an unknown but well-formed target name let the binary be removed" "$out"
    return
  fi
  pass "an unknown but well-formed target name keeps the binary-removal guard closed"
}

# The twin of the case above, in the other direction: the SAME word, made
# unreadable by a NUL byte, is a torn or binary write and must stay
# droppable. Bash cannot hold a NUL in a variable at all, so a reader that
# forgets this sees the NUL silently vanish and reads "zed" — turning the
# one class of content that is provably garbage into the one class that
# wedges the guard forever.
case_nul_byte_makes_a_wellformed_name_garbage() {
  local dir status out
  dir="$(new_case_dir nul-wellformed)"

  out="$(run_guard_case "$dir" '\x00zed\n')"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "NUL-byte tracking line: run failed (exit $status)" "$out"
    return
  fi
  if ! grep -q -F -e BINARY-REMOVED <<<"$out"; then
    fail "a NUL-byte line was read as a live install and wedged the guard" "$out"
    return
  fi
  pass "a NUL byte makes an otherwise well-formed name garbage"
}

# The other garbage twin: a final line with no terminating newline. Every
# write this script makes is newline-terminated, so unterminated trailing
# bytes are a partial write — a name that was still being written, not a
# name. "clau" is a well-formed bare word and would otherwise pass the
# name check.
case_partial_final_line_is_garbage() {
  local dir status out
  dir="$(new_case_dir partial-line)"

  out="$(run_guard_case "$dir" 'clau')"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "partial final line: run failed (exit $status)" "$out"
    return
  fi
  if ! grep -q -F -e BINARY-REMOVED <<<"$out"; then
    fail "an unterminated partial line was read as a live install" "$out"
    return
  fi
  pass "an unterminated final line is garbage, not install state"
}

# A partial line must not take the VALID lines above it down with it, and a
# valid line must still hold the guard when a partial one follows.
case_partial_line_does_not_discard_valid_lines() {
  local dir status out
  dir="$(new_case_dir partial-line-mixed)"

  out="$(run_guard_case "$dir" 'claude\nopenc')"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "partial line after a valid one: run failed (exit $status)" "$out"
    return
  fi
  if ! grep -q -F -e BINARY-STILL-PRESENT <<<"$out"; then
    fail "a trailing partial line discarded the valid target above it" "$out"
    return
  fi
  pass "a trailing partial line does not discard the valid lines above it"
}

# The twin of the guard case at the WRITE side. Removal rewrites the file
# from what the reader returns, so a reader that drops unknown names makes
# an OLDER build silently erase a NEWER build's record: uninstall one target
# it does know, and the unknown one is gone from the file. The very next
# call then reads empty and deletes the shared binary — the same defect, one
# invocation later.
case_removal_preserves_an_unknown_wellformed_target() {
  local dir status out
  dir="$(new_case_dir remove-preserves-unknown)"

  out="$(
    STATE_DIR="$dir/state" bash -c '
      source "$1"
      mkdir -p "$(dirname "$LONGTERM_MEM_INSTALLED_TARGETS")"
      printf "claude\nzed\n" > "$LONGTERM_MEM_INSTALLED_TARGETS"
      longtermmem_installed_targets_remove claude
      echo "FILE=[$(tr "\n" " " < "$LONGTERM_MEM_INSTALLED_TARGETS")]"
    ' _ "$OVERLAY" 2>&1
  )"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "removal with an unknown target present: run failed (exit $status)" "$out"
    return
  fi
  if grep -q -F -e "claude" <<<"$out"; then
    fail "removal did not remove the target it was asked to remove" "$out"
    return
  fi
  if ! grep -q -F -e "zed" <<<"$out"; then
    fail "removal erased an unknown but well-formed target from the tracking file" "$out"
    return
  fi
  pass "removal preserves an unknown but well-formed target"
}

# A tracking file that EXISTS but cannot be read is the third pile, and it
# belongs with "unknown", not with "garbage": the file may well list every
# runtime still depending on the shared binary, and nothing about a
# permission error says otherwise. Reading it as empty is the same fail-open
# as dropping an unknown name, arrived at from the other direction.
#
# chmod 000 does not stop root, so the two cases below verify the fixture
# before trusting it rather than passing vacuously in a root CI container.
case_unreadable_tracking_file_keeps_the_guard_closed() {
  local dir status out
  dir="$(new_case_dir unreadable-tracking)"

  out="$(
    STATE_DIR="$dir/state" bash -c '
      source "$1"
      mkdir -p "$(dirname "$LONGTERM_MEM_INSTALLED_TARGETS")"
      printf "claude\n" > "$LONGTERM_MEM_INSTALLED_TARGETS"
      chmod 000 "$LONGTERM_MEM_INSTALLED_TARGETS"
      if [[ -r "$LONGTERM_MEM_INSTALLED_TARGETS" ]]; then
        echo "FIXTURE-UNUSABLE"
        exit 0
      fi
      mkdir -p "$(dirname "$LONGTERM_MEM_BINARY")"
      printf "#!/bin/sh\n" > "$LONGTERM_MEM_BINARY"
      chmod +x "$LONGTERM_MEM_BINARY"
      tracking_known=1
      longtermmem_maybe_remove_binary ""
      if [[ -e "$LONGTERM_MEM_BINARY" ]]; then
        echo "BINARY-STILL-PRESENT"
      else
        echo "BINARY-REMOVED"
      fi
    ' _ "$OVERLAY" 2>&1
  )"
  status=$?
  chmod -R u+rwX "$dir" 2>/dev/null

  if [[ "$status" -ne 0 ]]; then
    fail "unreadable tracking file: run failed (exit $status)" "$out"
    return
  fi
  if grep -q -F -e FIXTURE-UNUSABLE <<<"$out"; then
    pass "unreadable tracking file: skipped (this user can read a 000 file)"
    return
  fi
  if ! grep -q -F -e BINARY-STILL-PRESENT <<<"$out"; then
    fail "an unreadable tracking file was read as empty and the binary was removed" "$out"
    return
  fi
  pass "an unreadable tracking file keeps the binary-removal guard closed"
}

# The write-side twin. Removal rewrites the file from what the reader
# returns, so a reader that answers "empty" for an unreadable file does not
# just misreport it — it TRUNCATES it, destroying the very state that would
# have kept the guard closed on the next call.
case_unreadable_tracking_file_is_not_truncated() {
  local dir status out
  dir="$(new_case_dir unreadable-tracking-write)"

  out="$(
    STATE_DIR="$dir/state" bash -c '
      source "$1"
      set +e
      mkdir -p "$(dirname "$LONGTERM_MEM_INSTALLED_TARGETS")"
      printf "claude\nopencode\n" > "$LONGTERM_MEM_INSTALLED_TARGETS"
      chmod 000 "$LONGTERM_MEM_INSTALLED_TARGETS"
      if [[ -r "$LONGTERM_MEM_INSTALLED_TARGETS" ]]; then
        echo "FIXTURE-UNUSABLE"
        exit 0
      fi
      ( longtermmem_installed_targets_remove claude )
      echo "REMOVE-STATUS=$?"
      chmod 600 "$LONGTERM_MEM_INSTALLED_TARGETS"
      echo "FILE=[$(tr "\n" " " < "$LONGTERM_MEM_INSTALLED_TARGETS")]"
    ' _ "$OVERLAY" 2>&1
  )"
  status=$?
  chmod -R u+rwX "$dir" 2>/dev/null

  if [[ "$status" -ne 0 ]]; then
    fail "unreadable tracking file (write side): run failed (exit $status)" "$out"
    return
  fi
  if grep -q -F -e FIXTURE-UNUSABLE <<<"$out"; then
    pass "unreadable tracking file (write side): skipped (this user can read a 000 file)"
    return
  fi
  if grep -q -F -e "REMOVE-STATUS=0" <<<"$out"; then
    fail "removal on an unreadable tracking file reported success" "$out"
    return
  fi
  if ! grep -q -F -e "FILE=[claude opencode ]" <<<"$out"; then
    fail "removal truncated a tracking file it could not read" "$out"
    return
  fi
  pass "removal refuses to rewrite a tracking file it could not read"
}

# ---------------------------------------------------------------------------
# hazard (b): a failed install must be observable by its caller
# ---------------------------------------------------------------------------

# cmd_apply prints a recovery line when the longterm-mem install step fails,
# because apply has already written each target's state row by then and will
# never retry the step on its own. Every likely failure in that step is a
# `die`, and `die` exits the process — so the recovery line was unreachable
# for exactly the failures it exists to explain.
case_install_failure_is_reported_with_recovery_text() {
  local dir status out
  dir="$(new_case_dir install-failure)"

  local engine_stub="$dir/engine-stub"
  printf '#!/usr/bin/env bash\nexit 0\n' > "$engine_stub"
  chmod +x "$engine_stub"

  out="$(
    STATE_DIR="$dir/state" bash -c '
      source "$1"
      set +e
      ENGINE_BINARY="$2"
      LONGTERM_MEM_SRC="$3/definitely-not-a-module"
      longtermmem_install_step all
      echo "STEP-STATUS=$?"
    ' _ "$OVERLAY" "$engine_stub" "$dir" 2>&1
  )"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "forced install failure: the harness itself exited $status (the step aborted the shell)" "$out"
    return
  fi
  if grep -q -F -e "STEP-STATUS=0" <<<"$out"; then
    fail "forced install failure reported success" "$out"
    return
  fi
  if ! grep -q -F -e "STEP-STATUS=" <<<"$out"; then
    fail "forced install failure aborted the caller instead of returning a status" "$out"
    return
  fi
  if ! grep -q -F -e "longterm-mem install FAILED" <<<"$out"; then
    fail "forced install failure printed no recovery text" "$out"
    return
  fi
  if ! grep -q -F -e "longterm-mem install --target all" <<<"$out"; then
    fail "forced install failure printed no recovery command" "$out"
    return
  fi
  pass "a forced install failure returns non-zero and prints the recovery text"
}

# ---------------------------------------------------------------------------
# finding A2: register's exit status must be inspected, not discarded
# ---------------------------------------------------------------------------

# writes a fake toolchain into $1: a `go` that satisfies `go build -o <path>`
# by installing $2 there, and an engine stub that always succeeds.
write_fake_toolchain() {
  local dir="$1" register_exit="$2"
  mkdir -p "$dir/bin" "$dir/src"

  cat > "$dir/bin/go" <<'GOSTUB'
#!/usr/bin/env bash
# Minimal `go build -o <path> <pkg>` stand-in: copies the prepared fake
# longterm-mem binary to the requested output path.
#
# FAKE_GO_VANISH=1 makes it report success and leave no artifact behind —
# the fault injection for "the build step said ok and the file is not
# there", which is what the deploy steps after it have to notice.
out=""
while [[ $# -gt 0 ]]; do
  if [[ "$1" == "-o" ]]; then
    shift
    out="${1:-}"
  fi
  shift || true
done
[[ -n "$out" ]] || exit 1
if [[ "${FAKE_GO_VANISH:-}" == "1" ]]; then
  rm -f "$out"
  exit 0
fi
cp "$FAKE_LONGTERM_MEM" "$out"
chmod +x "$out"
GOSTUB
  chmod +x "$dir/bin/go"

  cat > "$dir/fake-longterm-mem" <<FAKEBIN
#!/usr/bin/env bash
if [[ "\${1:-}" == "register" ]]; then
  printf '%s\n' "\$*" >> "\${FAKE_REGISTER_ARGV:-/dev/null}"
  echo "longterm-mem: register: refused" >&2
  exit $register_exit
fi
# FAKE_UNREGISTER_EXIT drives the uninstall-side cases; it defaults to 0 so
# every case that does not set it sees the old always-succeeds behaviour.
if [[ "\${1:-}" == "unregister" ]]; then
  exit "\${FAKE_UNREGISTER_EXIT:-0}"
fi
exit 0
FAKEBIN
  chmod +x "$dir/fake-longterm-mem"

  printf '#!/usr/bin/env bash\nexit 0\n' > "$dir/engine-stub"
  chmod +x "$dir/engine-stub"
}

# `register` exits 6 when a runtime already carries a same-named MCP entry
# it does not own: it refuses to touch that entry. Recording the target as
# installed anyway makes the uninstall guard hold the shared binary hostage
# for a target this overlay never registered.
case_register_refusal_does_not_mark_target_installed() {
  local dir status out
  dir="$(new_case_dir register-refused)"
  write_fake_toolchain "$dir" 6

  out="$(
    STATE_DIR="$dir/state" \
    FAKE_LONGTERM_MEM="$dir/fake-longterm-mem" \
    PATH="$dir/bin:$PATH" \
    bash -c '
      source "$1"
      set +e
      ENGINE_BINARY="$2"
      LONGTERM_MEM_SRC="$3/src"
      cmd_longterm_mem install --target claude
      echo "INSTALL-STATUS=$?"
      echo "TRACKED=[$(longtermmem_installed_targets_read | tr "\n" " ")]"
    ' _ "$OVERLAY" "$dir/engine-stub" "$dir" 2>&1
  )"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "register refusal: harness exited $status" "$out"
    return
  fi
  if grep -q -F -e "TRACKED=[claude " <<<"$out" || grep -q -F -e "TRACKED=[claude]" <<<"$out"; then
    fail "register refusal (exit 6) still marked claude as installed" "$out"
    return
  fi
  if ! grep -q -F -e "INSTALL-STATUS=0" <<<"$out"; then
    fail "register refusal (exit 6) should converge, not fail the run" "$out"
    return
  fi
  if ! grep -q -F -e "does not own" <<<"$out"; then
    fail "register refusal (exit 6) was not named distinctly on stderr" "$out"
    return
  fi
  pass "a refused (exit 6) register does not mark the target installed"
}

# A hard register failure (exit 1) is not the same outcome: the target is not
# installed AND the run did not converge, so it must surface on the exit code.
case_register_hard_failure_fails_the_run() {
  local dir status out
  dir="$(new_case_dir register-failed)"
  write_fake_toolchain "$dir" 1

  out="$(
    STATE_DIR="$dir/state" \
    FAKE_LONGTERM_MEM="$dir/fake-longterm-mem" \
    PATH="$dir/bin:$PATH" \
    bash -c '
      source "$1"
      set +e
      ENGINE_BINARY="$2"
      LONGTERM_MEM_SRC="$3/src"
      cmd_longterm_mem install --target claude
      echo "INSTALL-STATUS=$?"
      echo "TRACKED=[$(longtermmem_installed_targets_read | tr "\n" " ")]"
    ' _ "$OVERLAY" "$dir/engine-stub" "$dir" 2>&1
  )"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "register hard failure: harness exited $status" "$out"
    return
  fi
  if grep -q -F -e "INSTALL-STATUS=0" <<<"$out"; then
    fail "a hard register failure (exit 1) was reported as a successful install" "$out"
    return
  fi
  if ! grep -q -F -e "TRACKED=[]" <<<"$out"; then
    fail "a hard register failure (exit 1) still marked the target installed" "$out"
    return
  fi
  pass "a hard register failure (exit 1) fails the run and records nothing"
}

# `register --target all` skips a runtime whose configuration file is
# absent; a target named explicitly is a different statement and fails. This
# loop expands "all" itself and calls register once per target, so it has to
# carry that distinction — otherwise every runtime the machine does not have
# becomes a named-target hard failure, and `apply` fails on any machine that
# does not run all three.
case_register_failure_under_target_all_converges() {
  local dir status out
  dir="$(new_case_dir register-failed-all)"
  write_fake_toolchain "$dir" 1

  out="$(
    STATE_DIR="$dir/state" \
    FAKE_LONGTERM_MEM="$dir/fake-longterm-mem" \
    PATH="$dir/bin:$PATH" \
    bash -c '
      source "$1"
      set +e
      ENGINE_BINARY="$2"
      LONGTERM_MEM_SRC="$3/src"
      cmd_longterm_mem install --target all
      echo "INSTALL-STATUS=$?"
      echo "TRACKED=[$(longtermmem_installed_targets_read | tr "\n" " ")]"
    ' _ "$OVERLAY" "$dir/engine-stub" "$dir" 2>&1
  )"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "register failure under --target all: harness exited $status" "$out"
    return
  fi
  if ! grep -q -F -e "INSTALL-STATUS=0" <<<"$out"; then
    fail "a register failure under --target all should converge, not fail the run" "$out"
    return
  fi
  if ! grep -q -F -e "TRACKED=[]" <<<"$out"; then
    fail "a register failure under --target all still marked targets installed" "$out"
    return
  fi
  pass "a register failure under --target all converges and records nothing"
}

# The path register writes INTO each runtime's MCP entry has to be the path
# the binary was just deployed to. register resolves its own default from
# $HOME, while this entrypoint deploys to $STATE_DIR/bin/longterm-mem — so
# under a $STATE_DIR override the two disagree and every entry names a path
# nothing was ever deployed to. engine/runtime re-derives ownership from
# that same path, so the disagreement also makes an entry we did write stop
# looking like ours.
case_register_is_told_the_deployed_binary_path() {
  local dir status out argv
  dir="$(new_case_dir register-binary-arg)"
  write_fake_toolchain "$dir" 0
  argv="$dir/register-argv"

  out="$(
    STATE_DIR="$dir/state" \
    FAKE_LONGTERM_MEM="$dir/fake-longterm-mem" \
    FAKE_REGISTER_ARGV="$argv" \
    PATH="$dir/bin:$PATH" \
    bash -c '
      source "$1"
      set +e
      ENGINE_BINARY="$2"
      LONGTERM_MEM_SRC="$3/src"
      cmd_longterm_mem install --target claude
      echo "DEPLOYED=$LONGTERM_MEM_BINARY"
    ' _ "$OVERLAY" "$dir/engine-stub" "$dir" 2>&1
  )"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "register --binary: harness exited $status" "$out"
    return
  fi
  if [[ ! -f "$argv" ]]; then
    fail "register --binary: register was never invoked" "$out"
    return
  fi
  local deployed
  deployed="$dir/state/bin/longterm-mem"
  if ! grep -q -F -e "--binary $deployed" "$argv"; then
    fail "register was not told the deployed binary path" "argv: $(cat "$argv")" "expected --binary $deployed"
    return
  fi
  pass "register is told the deployed binary path"
}

# ---------------------------------------------------------------------------
# register/unregister exit 8: a path that could not be resolved at all
# ---------------------------------------------------------------------------

# register separates exit 8 from exit 1 ON PURPOSE (register_paths.go:
# "nothing was attempted and nothing was touched, and the fix is the
# caller's environment"). Folding it into the catch-all arm threw that
# distinction away and, under --target all, actively misreported it as "a
# runtime it does not have" — telling the operator their machine simply
# lacks the runtime when what it actually lacks is a resolvable HOME.
case_register_path_unresolvable_is_named() {
  local dir status out
  dir="$(new_case_dir register-unresolvable)"
  write_fake_toolchain "$dir" 8

  out="$(
    STATE_DIR="$dir/state" \
    FAKE_LONGTERM_MEM="$dir/fake-longterm-mem" \
    PATH="$dir/bin:$PATH" \
    bash -c '
      source "$1"
      set +e
      ENGINE_BINARY="$2"
      LONGTERM_MEM_SRC="$3/src"
      cmd_longterm_mem install --target claude
      echo "INSTALL-STATUS=$?"
      echo "TRACKED=[$(longtermmem_installed_targets_read | tr "\n" " ")]"
    ' _ "$OVERLAY" "$dir/engine-stub" "$dir" 2>&1
  )"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "register exit 8: harness exited $status" "$out"
    return
  fi
  if ! grep -q -F -e "TRACKED=[]" <<<"$out"; then
    fail "register exit 8 still marked the target installed" "$out"
    return
  fi
  if grep -q -F -e "INSTALL-STATUS=0" <<<"$out"; then
    fail "register exit 8 was reported as a successful install" "$out"
    return
  fi
  if ! grep -q -F -e "set HOME" <<<"$out"; then
    fail "register exit 8 did not name the remedy (set HOME)" "$out"
    return
  fi
  pass "register exit 8 is named with its remedy and does not record the target"
}

# The twin: under --target all the catch-all arm prints the "a runtime it
# does not have" line, which is exactly the wrong story for exit 8. Exit 8
# is not per-runtime at all — it is one environment fault that every target
# reproduces — so it must neither wear that line nor converge at 0.
case_register_path_unresolvable_under_target_all_is_named() {
  local dir status out
  dir="$(new_case_dir register-unresolvable-all)"
  write_fake_toolchain "$dir" 8

  out="$(
    STATE_DIR="$dir/state" \
    FAKE_LONGTERM_MEM="$dir/fake-longterm-mem" \
    PATH="$dir/bin:$PATH" \
    bash -c '
      source "$1"
      set +e
      ENGINE_BINARY="$2"
      LONGTERM_MEM_SRC="$3/src"
      cmd_longterm_mem install --target all
      echo "INSTALL-STATUS=$?"
      echo "TRACKED=[$(longtermmem_installed_targets_read | tr "\n" " ")]"
    ' _ "$OVERLAY" "$dir/engine-stub" "$dir" 2>&1
  )"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "register exit 8 under --target all: harness exited $status" "$out"
    return
  fi
  if grep -q -F -e "a runtime it does not have" <<<"$out"; then
    fail "register exit 8 under --target all was misreported as a missing runtime" "$out"
    return
  fi
  if ! grep -q -F -e "set HOME" <<<"$out"; then
    fail "register exit 8 under --target all did not name the remedy (set HOME)" "$out"
    return
  fi
  if grep -q -F -e "INSTALL-STATUS=0" <<<"$out"; then
    fail "register exit 8 under --target all converged as if nothing were wrong" "$out"
    return
  fi
  pass "register exit 8 under --target all is named with its remedy and fails the run"
}

# unregister carries the same exit 8 for the same reason, and the uninstall
# branch folded it into the same catch-all. It has to keep the target
# tracked (its entry may still be in the runtime config) AND say what to do.
case_unregister_path_unresolvable_is_named() {
  local dir status out
  dir="$(new_case_dir unregister-unresolvable)"
  write_fake_toolchain "$dir" 0

  out="$(
    STATE_DIR="$dir/state" \
    FAKE_UNREGISTER_EXIT=8 \
    bash -c '
      source "$1"
      set +e
      ENGINE_BINARY="$2"
      mkdir -p "$(dirname "$LONGTERM_MEM_BINARY")" "$(dirname "$LONGTERM_MEM_INSTALLED_TARGETS")"
      cp "$3" "$LONGTERM_MEM_BINARY"
      chmod +x "$LONGTERM_MEM_BINARY"
      printf "claude\n" > "$LONGTERM_MEM_INSTALLED_TARGETS"
      cmd_longterm_mem uninstall --target claude
      echo "UNINSTALL-STATUS=$?"
      echo "TRACKED=[$(longtermmem_installed_targets_read | tr "\n" " ")]"
    ' _ "$OVERLAY" "$dir/engine-stub" "$dir/fake-longterm-mem" 2>&1
  )"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "unregister exit 8: harness exited $status" "$out"
    return
  fi
  if ! grep -q -F -e "TRACKED=[claude ]" <<<"$out"; then
    fail "unregister exit 8 dropped the target's tracking" "$out"
    return
  fi
  if grep -q -F -e "UNINSTALL-STATUS=0" <<<"$out"; then
    fail "unregister exit 8 was reported as a clean uninstall" "$out"
    return
  fi
  if ! grep -q -F -e "set HOME" <<<"$out"; then
    fail "unregister exit 8 did not name the remedy (set HOME)" "$out"
    return
  fi
  if [[ ! -e "$dir/state/bin/longterm-mem" ]]; then
    fail "unregister exit 8 removed the shared binary anyway" "$out"
    return
  fi
  pass "unregister exit 8 is named with its remedy and keeps the target tracked"
}

# unregister exit 6 ("an entry exists that longterm-mem does not own") is NOT
# the same event as exit 0. Exit 0 means "the entry is gone"; exit 6 means "I
# did not touch it". Dropping the target's tracking on 6 lets the guard below
# delete the SHARED BINARY while a longterm-mem MCP entry may still sit in
# that runtime's config -- an orphaned entry pointing at a binary that no
# longer exists, which is the one state the tracking file exists to prevent.
case_unregister_unmanaged_keeps_the_target_tracked() {
  local dir status out
  dir="$(new_case_dir unregister-unmanaged)"
  write_fake_toolchain "$dir" 0

  out="$(
    STATE_DIR="$dir/state" \
    FAKE_UNREGISTER_EXIT=6 \
    bash -c '
      source "$1"
      set +e
      ENGINE_BINARY="$2"
      mkdir -p "$(dirname "$LONGTERM_MEM_BINARY")" "$(dirname "$LONGTERM_MEM_INSTALLED_TARGETS")"
      cp "$3" "$LONGTERM_MEM_BINARY"
      chmod +x "$LONGTERM_MEM_BINARY"
      printf "claude\n" > "$LONGTERM_MEM_INSTALLED_TARGETS"
      cmd_longterm_mem uninstall --target claude
      echo "UNINSTALL-STATUS=$?"
      echo "TRACKED=[$(longtermmem_installed_targets_read | tr "\n" " ")]"
    ' _ "$OVERLAY" "$dir/engine-stub" "$dir/fake-longterm-mem" 2>&1
  )"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "unregister exit 6: harness exited $status" "$out"
    return
  fi
  if ! grep -q -F -e "TRACKED=[claude ]" <<<"$out"; then
    fail "unregister exit 6 dropped the target's tracking, so the shared binary is removable while the entry may remain" "$out"
    return
  fi
  if [[ ! -e "$dir/state/bin/longterm-mem" ]]; then
    fail "unregister exit 6 removed the shared binary while claude's entry was left untouched" "$out"
    return
  fi
  if ! grep -q -F -e "UNINSTALL-STATUS=0" <<<"$out"; then
    fail "unregister exit 6 should converge: a foreign entry is a machine state, not a failure of this run" "$out"
    return
  fi
  pass "a refused (exit 6) unregister keeps the target tracked and the shared binary in place"
}

# The TWIN of the case above, decided the other way and pinned so the
# difference stays deliberate rather than accidental: exit 2 (version skew)
# also means "I did not remove the entry" and carries the same orphan
# hazard, but the tool that would perform exit 6's recovery is itself the
# broken thing, so keeping the target tracked would leave --purge -- the
# action that orphans the entries outright -- as the only way to finish. It
# therefore converges AND clears tracking (engine/installer's
# TestUninstall_VersionSkewStillConverges pins the same branch end to end).
case_unregister_version_skew_converges() {
  local dir status out
  dir="$(new_case_dir unregister-skew)"
  write_fake_toolchain "$dir" 0

  out="$(
    STATE_DIR="$dir/state" \
    FAKE_UNREGISTER_EXIT=2 \
    bash -c '
      source "$1"
      set +e
      ENGINE_BINARY="$2"
      mkdir -p "$(dirname "$LONGTERM_MEM_BINARY")" "$(dirname "$LONGTERM_MEM_INSTALLED_TARGETS")"
      cp "$3" "$LONGTERM_MEM_BINARY"
      chmod +x "$LONGTERM_MEM_BINARY"
      printf "claude\n" > "$LONGTERM_MEM_INSTALLED_TARGETS"
      cmd_longterm_mem uninstall --target claude
      echo "UNINSTALL-STATUS=$?"
      echo "TRACKED=[$(longtermmem_installed_targets_read | tr "\n" " ")]"
    ' _ "$OVERLAY" "$dir/engine-stub" "$dir/fake-longterm-mem" 2>&1
  )"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "unregister exit 2: harness exited $status" "$out"
    return
  fi
  if grep -q -F -e "TRACKED=[claude" <<<"$out"; then
    fail "unregister exit 2 kept claude tracked, so the run can never finish without --purge" "$out"
    return
  fi
  if ! grep -q -F -e "UNINSTALL-STATUS=0" <<<"$out"; then
    fail "unregister exit 2 did not converge" "$out"
    return
  fi
  if ! grep -q -F -e "by hand" <<<"$out"; then
    fail "unregister exit 2 did not say the entry was left behind" "$out"
    return
  fi
  pass "a version-skewed (exit 2) unregister converges and says what it left behind"
}

# register exit 2 is the install-side mirror of exit 8, and the exit-8 arm's
# own reasoning applies to it verbatim: it is environment-independent, it
# reproduces identically on every target, and reporting a component that
# registered NOTHING as a clean run is the failure that arm was written to
# stop. So it must fail the run.
case_register_version_skew_fails_the_run() {
  local dir status out
  dir="$(new_case_dir register-skew)"
  write_fake_toolchain "$dir" 2

  out="$(
    STATE_DIR="$dir/state" \
    FAKE_LONGTERM_MEM="$dir/fake-longterm-mem" \
    PATH="$dir/bin:$PATH" \
    bash -c '
      source "$1"
      set +e
      ENGINE_BINARY="$2"
      LONGTERM_MEM_SRC="$3/src"
      cmd_longterm_mem install --target claude
      echo "INSTALL-STATUS=$?"
      echo "TRACKED=[$(longtermmem_installed_targets_read | tr "\n" " ")]"
    ' _ "$OVERLAY" "$dir/engine-stub" "$dir" 2>&1
  )"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "register exit 2: harness exited $status" "$out"
    return
  fi
  if grep -q -F -e "INSTALL-STATUS=0" <<<"$out"; then
    fail "register exit 2 reported a clean install having registered nothing" "$out"
    return
  fi
  if grep -q -F -e "TRACKED=[claude " <<<"$out" || grep -q -F -e "TRACKED=[claude]" <<<"$out"; then
    fail "register exit 2 marked claude as installed" "$out"
    return
  fi
  pass "a version-skewed (exit 2) register fails the install run"
}

# Every case above runs with errexit off, because it needs to observe an
# exit status. The CLI dispatch does NOT: `labdrian-overlay longterm-mem
# install` runs the same code under the `set -euo pipefail` at the top of
# the script, where any command that reports non-zero — an `[[ ]]` test in
# an AND-list, a grep that matched nothing — takes the whole run down. This
# case pins the successful path under those settings.
case_install_succeeds_under_errexit() {
  local dir status out
  dir="$(new_case_dir errexit)"
  write_fake_toolchain "$dir" 0

  out="$(
    STATE_DIR="$dir/state" \
    FAKE_LONGTERM_MEM="$dir/fake-longterm-mem" \
    PATH="$dir/bin:$PATH" \
    bash -c '
      source "$1"
      ENGINE_BINARY="$2"
      LONGTERM_MEM_SRC="$3/src"
      cmd_longterm_mem install --target claude
      # Twice: the second add takes the "already recorded" path.
      cmd_longterm_mem install --target claude
      echo "TRACKED=[$(longtermmem_installed_targets_read | tr "\n" " ")]"
    ' _ "$OVERLAY" "$dir/engine-stub" "$dir" 2>&1
  )"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "install under errexit aborted (exit $status)" "$out"
    return
  fi
  if ! grep -q -F -e "TRACKED=[claude ]" <<<"$out"; then
    fail "install under errexit did not record exactly one claude entry" "$out"
    return
  fi
  pass "a successful install runs to completion under errexit"
}

# ---------------------------------------------------------------------------
# hazard (b): the deploy steps must be checked, not assumed
# ---------------------------------------------------------------------------

# `set -e` is suppressed for a function called in a condition context, and
# cmd_longterm_mem install is reached from exactly such a context in
# cmd_apply. Every step of the deploy therefore has to check itself.
#
# The steps AFTER the build are the ones nothing downstream covers for: a
# failing mkdir or mktemp still surfaces (wearing the wrong error, as a
# build failure), but a chmod or mv that fails on an artifact the build
# claims to have produced is followed straight by the "binary deployed to"
# line — announcing a deploy that did not happen, to an operator who now has
# no reason to look.
case_undeployable_binary_is_not_reported_as_deployed() {
  local dir status out
  dir="$(new_case_dir undeployable)"
  write_fake_toolchain "$dir" 0

  out="$(
    STATE_DIR="$dir/state" \
    FAKE_LONGTERM_MEM="$dir/fake-longterm-mem" \
    FAKE_GO_VANISH=1 \
    PATH="$dir/bin:$PATH" \
    bash -c '
      source "$1"
      set +e
      ENGINE_BINARY="$2"
      LONGTERM_MEM_SRC="$3/src"
      # Reached the way cmd_apply reaches it, so a `die` in the deploy is
      # observable rather than fatal to the caller.
      longtermmem_install_step claude
      echo "INSTALL-STATUS=$?"
    ' _ "$OVERLAY" "$dir/engine-stub" "$dir" 2>&1
  )"
  status=$?

  if [[ "$status" -ne 0 ]]; then
    fail "undeployable binary: harness exited $status" "$out"
    return
  fi
  if grep -q -F -e "binary deployed to" <<<"$out"; then
    fail "a failed deploy still reported 'binary deployed'" "$out"
    return
  fi
  if grep -q -F -e "INSTALL-STATUS=0" <<<"$out"; then
    fail "a failed deploy reported success" "$out"
    return
  fi
  if [[ -e "$dir/state/bin/longterm-mem" ]]; then
    fail "a failed deploy left something at the binary path" "$out"
    return
  fi
  pass "a failed binary deploy is not reported as deployed"
}

# ---------------------------------------------------------------------------
# hazard (e): an engine binary older than the engine source it was built from
# ---------------------------------------------------------------------------
#
# ensure_engine_binary used to build only when the binary was ABSENT, so a
# repository that moved forward left a compiled artifact silently out of sync
# with the source that defines it. The symptom is an engine that rejects a
# flag the overlay legitimately passes, which reads as an overlay bug.

# write_fake_engine_tree lays out an engine source tree plus a `go` stand-in
# whose "build" writes a recognizable marker into the output path. That makes
# a rebuild provable from the binary's CONTENT, not from log text.
write_fake_engine_tree() {
  local dir="$1"
  mkdir -p "$dir/bin" "$dir/engine/cmd"
  printf 'package main\n\nfunc main() {}\n' > "$dir/engine/cmd/main.go"
  printf 'module engine\n' > "$dir/engine/go.mod"
  printf '\n' > "$dir/engine/go.sum"

  cat > "$dir/bin/go" <<'GOSTUB'
#!/usr/bin/env bash
# Minimal `go build -o <path> ./cmd/` stand-in.
out=""
while [[ $# -gt 0 ]]; do
  if [[ "$1" == "-o" ]]; then
    shift
    out="${1:-}"
  fi
  shift || true
done
[[ -n "$out" ]] || exit 1
if [[ "${FAKE_GO_BUILD_FAILS:-}" == "1" ]]; then
  echo "fake go: build failed" >&2
  exit 1
fi
printf '#!/usr/bin/env bash\n# %s\nexit 0\n' "${FAKE_BUILD_TAG:-rebuilt}" > "$out"
chmod +x "$out"
GOSTUB
  chmod +x "$dir/bin/go"
}

# write_stale_binary drops a binary at <path> whose mtime predates the source
# tree, i.e. exactly the fast-forwarded-repository shape.
write_stale_binary() {
  local path="$1"
  mkdir -p "$(dirname "$path")"
  printf '#!/usr/bin/env bash\n# original\nexit 0\n' > "$path"
  chmod +x "$path"
  touch -t 200001010000 "$path"
}

run_ensure_engine_binary() {
  local dir="$1"
  shift
  env PATH="$dir/bin:/usr/bin:/bin" "$@" bash -c '
    source "$1"
    set +e
    ENGINE_BINARY="$2"
    ENGINE_SRC="$3"
    ensure_engine_binary
    echo "ENSURE-STATUS=$?"
  ' _ "$OVERLAY" "$dir/binary/gentle-ai-overlay" "$dir/engine" 2>&1
}

case_binary_older_than_engine_source_is_rebuilt() {
  local dir out
  dir="$(new_case_dir engine-stale-go)"
  write_fake_engine_tree "$dir"
  write_stale_binary "$dir/binary/gentle-ai-overlay"

  out="$(run_ensure_engine_binary "$dir" FAKE_BUILD_TAG=rebuilt-by-go-change)"

  if ! grep -q -F -e "ENSURE-STATUS=0" <<<"$out"; then
    fail "stale binary: ensure_engine_binary did not succeed" "$out"
    return
  fi
  if ! grep -q -F -e "rebuilt-by-go-change" "$dir/binary/gentle-ai-overlay"; then
    fail "a binary older than a .go source was not rebuilt" "$out"
    return
  fi
  pass "a binary older than an engine source is rebuilt"
}

# The twin the find expression can silently miss: a dependency bump changes
# behaviour as surely as a source edit, so go.mod and go.sum count as source.
case_binary_older_than_go_mod_is_rebuilt() {
  local dir out
  dir="$(new_case_dir engine-stale-gomod)"
  write_fake_engine_tree "$dir"
  touch -t 200001010000 "$dir/engine/cmd/main.go" "$dir/engine/go.sum"
  write_stale_binary "$dir/binary/gentle-ai-overlay"
  touch "$dir/engine/go.mod"

  out="$(run_ensure_engine_binary "$dir" FAKE_BUILD_TAG=rebuilt-by-gomod)"

  if ! grep -q -F -e "rebuilt-by-gomod" "$dir/binary/gentle-ai-overlay"; then
    fail "a binary older than go.mod was not rebuilt" "$out"
    return
  fi
  pass "a binary older than go.mod is rebuilt"
}

case_binary_older_than_go_sum_is_rebuilt() {
  local dir out
  dir="$(new_case_dir engine-stale-gosum)"
  write_fake_engine_tree "$dir"
  touch -t 200001010000 "$dir/engine/cmd/main.go" "$dir/engine/go.mod"
  write_stale_binary "$dir/binary/gentle-ai-overlay"
  touch "$dir/engine/go.sum"

  out="$(run_ensure_engine_binary "$dir" FAKE_BUILD_TAG=rebuilt-by-gosum)"

  if ! grep -q -F -e "rebuilt-by-gosum" "$dir/binary/gentle-ai-overlay"; then
    fail "a binary older than go.sum was not rebuilt" "$out"
    return
  fi
  pass "a binary older than go.sum is rebuilt"
}

# The twin that stops the fix from becoming its own defect: rebuilding on
# every invocation would put a go build in front of every overlay command.
case_binary_newer_than_every_source_is_not_rebuilt() {
  local dir out before after
  dir="$(new_case_dir engine-fresh)"
  write_fake_engine_tree "$dir"
  touch -t 200001010000 "$dir/engine/cmd/main.go" "$dir/engine/go.mod" "$dir/engine/go.sum"
  write_stale_binary "$dir/binary/gentle-ai-overlay"
  touch "$dir/binary/gentle-ai-overlay"
  before="$(cat "$dir/binary/gentle-ai-overlay")"

  out="$(run_ensure_engine_binary "$dir" FAKE_BUILD_TAG=should-not-happen)"
  after="$(cat "$dir/binary/gentle-ai-overlay")"

  if ! grep -q -F -e "ENSURE-STATUS=0" <<<"$out"; then
    fail "fresh binary: ensure_engine_binary did not succeed" "$out"
    return
  fi
  if [[ "$before" != "$after" ]]; then
    fail "a binary newer than every engine source was rebuilt anyway" "$out"
    return
  fi
  pass "a binary newer than every engine source is not rebuilt"
}

case_absent_binary_is_still_built() {
  local dir out
  dir="$(new_case_dir engine-absent)"
  write_fake_engine_tree "$dir"
  mkdir -p "$dir/binary"

  out="$(run_ensure_engine_binary "$dir" FAKE_BUILD_TAG=built-from-absent)"

  if ! grep -q -F -e "ENSURE-STATUS=0" <<<"$out"; then
    fail "absent binary: ensure_engine_binary did not succeed" "$out"
    return
  fi
  if [[ ! -x "$dir/binary/gentle-ai-overlay" ]]; then
    fail "an absent binary was not built" "$out"
    return
  fi
  if ! grep -q -F -e "built-from-absent" "$dir/binary/gentle-ai-overlay"; then
    fail "an absent binary was not built from source" "$out"
    return
  fi
  pass "an absent binary is still built"
}

# Stale and unbuildable is NOT the absent case: a stale binary may serve the
# command at hand perfectly, so refusing would brick every overlay command for
# a user whose Go toolchain disappeared. It must warn — naming the staleness
# AND the unknown-flag symptom it predicts — and continue.
assert_stale_warning_is_diagnosed() {
  local label="$1" out="$2"
  if ! grep -q -i -F -e "stale" <<<"$out"; then
    fail "$label: the warning does not name the staleness" "$out"
    return 1
  fi
  if ! grep -q -i -F -e "unknown flag" <<<"$out"; then
    fail "$label: the warning does not predict the unknown-flag symptom" "$out"
    return 1
  fi
  return 0
}

case_stale_binary_without_go_warns_and_continues() {
  local dir out before after
  dir="$(new_case_dir engine-stale-no-go)"
  write_fake_engine_tree "$dir"
  rm -f "$dir/bin/go"
  write_stale_binary "$dir/binary/gentle-ai-overlay"
  before="$(cat "$dir/binary/gentle-ai-overlay")"

  # A PATH with no `go` on it at all. It carries symlinks to the few
  # utilities the staleness probe itself needs, so "go is missing" is the
  # only thing this case injects.
  local nogo="$dir/nogo-bin" util
  mkdir -p "$nogo"
  for util in bash dirname find head mkdir; do
    ln -sf "$(command -v "$util")" "$nogo/$util"
  done

  out="$(
    env PATH="$nogo" /usr/bin/env bash -c '
      source "$1"
      set +e
      ENGINE_BINARY="$2"
      ENGINE_SRC="$3"
      ensure_engine_binary
      echo "ENSURE-STATUS=$?"
    ' _ "$OVERLAY" "$dir/binary/gentle-ai-overlay" "$dir/engine" 2>&1
  )"
  after="$(cat "$dir/binary/gentle-ai-overlay")"

  if ! grep -q -F -e "ENSURE-STATUS=0" <<<"$out"; then
    fail "stale binary without go: the command did not continue" "$out"
    return
  fi
  if [[ "$before" != "$after" ]]; then
    fail "stale binary without go: the existing binary was disturbed" "$out"
    return
  fi
  assert_stale_warning_is_diagnosed "stale binary without go" "$out" || return
  pass "a stale binary with no go toolchain warns and continues"
}

case_stale_binary_with_failing_build_warns_and_continues() {
  local dir out before after
  dir="$(new_case_dir engine-stale-build-fails)"
  write_fake_engine_tree "$dir"
  write_stale_binary "$dir/binary/gentle-ai-overlay"
  before="$(cat "$dir/binary/gentle-ai-overlay")"

  out="$(run_ensure_engine_binary "$dir" FAKE_GO_BUILD_FAILS=1)"
  after="$(cat "$dir/binary/gentle-ai-overlay")"

  if ! grep -q -F -e "ENSURE-STATUS=0" <<<"$out"; then
    fail "stale binary with a failing build: the command did not continue" "$out"
    return
  fi
  if [[ "$before" != "$after" ]]; then
    fail "stale binary with a failing build: the existing binary was replaced" "$out"
    return
  fi
  assert_stale_warning_is_diagnosed "stale binary with a failing build" "$out" || return
  pass "a stale binary whose rebuild fails warns and continues"
}

# ---------------------------------------------------------------------------
# hazard (f): the operator can neither SEE nor deliberately repair the engine
# ---------------------------------------------------------------------------
#
# ensure_engine_binary heals on the way to running something, silently and by
# design. That leaves two gaps this section pins: `doctor` never reported the
# engine binary's freshness at all, and the only deliberate rebuild was
# `install-hooks`, which also rewrites ~/.claude/settings.json.

# run_doctor runs cmd_doctor against a fixture engine tree. <bindir> is the
# first PATH entry, which is how a case chooses whether `go` exists at all.
run_doctor() {
  local dir="$1" path="$2"
  shift 2
  env PATH="$path" STATE_DIR="$dir/state" bash -c '
    overlay="$1"; binary="$2"; src="$3"; shift 3
    source "$overlay"
    set +e
    ENGINE_BINARY="$binary"
    ENGINE_SRC="$src"
    cmd_doctor "$@"
    echo "DOCTOR-STATUS=$?"
  ' _ "$OVERLAY" "$dir/binary/gentle-ai-overlay" "$dir/engine" "$@" 2>&1
}

# engine_doctor_line prints doctor's engine-binary line(s) only, so a case can
# assert on that check without matching the rest of the preflight.
engine_doctor_line() {
  grep -i -e "engine binary" <<<"$1"
}

# no_go_bindir builds a PATH entry with every utility doctor's own checks need
# and no `go`, so "the toolchain is missing" is the only injected condition.
no_go_bindir() {
  local dir="$1" nogo="$1/nogo-bin" util
  mkdir -p "$nogo"
  for util in bash cat dirname find grep head mkdir tr; do
    ln -sf "$(command -v "$util")" "$nogo/$util"
  done
  printf '%s\n' "$nogo"
}

case_doctor_reports_a_stale_engine_binary() {
  local dir out line
  dir="$(new_case_dir doctor-stale)"
  write_fake_engine_tree "$dir"
  write_stale_binary "$dir/binary/gentle-ai-overlay"

  out="$(run_doctor "$dir" "$dir/bin:/usr/bin:/bin")"
  line="$(engine_doctor_line "$out")"

  if ! grep -q -i -F -e "stale" <<<"$line"; then
    fail "doctor: a stale engine binary is not reported as stale" "$out"
    return
  fi
  # The expensive part of the original incident was not the staleness, it was
  # that the resulting engine error read as an overlay bug.
  if ! grep -q -i -F -e "unknown flag" <<<"$line"; then
    fail "doctor: the stale line does not name the unknown-flag symptom" "$out"
    return
  fi
  pass "doctor reports a stale engine binary and names its symptom"
}

case_doctor_reports_an_absent_engine_binary_differently() {
  local dir out line
  dir="$(new_case_dir doctor-absent)"
  write_fake_engine_tree "$dir"
  mkdir -p "$dir/binary"

  out="$(run_doctor "$dir" "$dir/bin:/usr/bin:/bin")"
  line="$(engine_doctor_line "$out")"

  if ! grep -q -i -F -e "not found" <<<"$line"; then
    fail "doctor: an absent engine binary is not reported as absent" "$out"
    return
  fi
  # Absent and stale need different remedies, so they must not read alike.
  if grep -q -i -F -e "stale" <<<"$line"; then
    fail "doctor: an absent engine binary is reported as if it were stale" "$out"
    return
  fi
  # The deliberate repair is doctor --fix; install-hooks also rewrites
  # ~/.claude/settings.json, which is a hammer for this tack.
  if ! grep -q -F -e "doctor --fix" <<<"$line"; then
    fail "doctor: the absent-binary line does not name the deliberate repair" "$out"
    return
  fi
  pass "doctor reports an absent engine binary differently from a stale one"
}

# The twin that stops the check from reporting a problem on every run.
case_doctor_reports_a_current_engine_binary_as_healthy() {
  local dir out line
  dir="$(new_case_dir doctor-healthy)"
  write_fake_engine_tree "$dir"
  touch -t 200001010000 "$dir/engine/cmd/main.go" "$dir/engine/go.mod" "$dir/engine/go.sum"
  write_stale_binary "$dir/binary/gentle-ai-overlay"
  touch "$dir/binary/gentle-ai-overlay"

  out="$(run_doctor "$dir" "$dir/bin:/usr/bin:/bin")"
  line="$(engine_doctor_line "$out")"

  if ! grep -q -F -e "PASS" <<<"$line"; then
    fail "doctor: a current engine binary is not reported as healthy" "$out"
    return
  fi
  if grep -q -i -e "stale" -e "not found" <<<"$line"; then
    fail "doctor: a current engine binary is reported as a problem" "$out"
    return
  fi
  # The healthy state is the third of three, and says so.
  if ! grep -q -i -F -e "current" <<<"$line"; then
    fail "doctor: the healthy line does not distinguish current from merely present" "$out"
    return
  fi
  pass "doctor reports a current engine binary as healthy"
}

case_doctor_fix_rebuilds_a_stale_engine_binary() {
  local dir out
  dir="$(new_case_dir doctor-fix-stale)"
  write_fake_engine_tree "$dir"
  write_stale_binary "$dir/binary/gentle-ai-overlay"

  out="$(FAKE_BUILD_TAG=rebuilt-by-doctor-fix run_doctor "$dir" "$dir/bin:/usr/bin:/bin" --fix)"

  # Proven by CONTENT, not by log text.
  if ! grep -q -F -e "rebuilt-by-doctor-fix" "$dir/binary/gentle-ai-overlay"; then
    fail "doctor --fix did not rebuild a stale engine binary" "$out"
    return
  fi
  pass "doctor --fix rebuilds a stale engine binary"
}

case_doctor_fix_without_go_does_not_claim_a_repair() {
  local dir out before after nogo
  dir="$(new_case_dir doctor-fix-no-go)"
  write_fake_engine_tree "$dir"
  rm -f "$dir/bin/go"
  write_stale_binary "$dir/binary/gentle-ai-overlay"
  before="$(cat "$dir/binary/gentle-ai-overlay")"
  nogo="$(no_go_bindir "$dir")"

  out="$(run_doctor "$dir" "$nogo" --fix)"
  after="$(cat "$dir/binary/gentle-ai-overlay")"

  if [[ "$before" != "$after" ]]; then
    fail "doctor --fix without go: the existing binary was disturbed" "$out"
    return
  fi
  if ! grep -q -i -F -e "could not repair" <<<"$out"; then
    fail "doctor --fix without go does not say it could not repair the engine binary" "$out"
    return
  fi
  if grep -q -i -e "^Engine binary rebuilt" <<<"$out"; then
    fail "doctor --fix without go claims a rebuild it did not perform" "$out"
    return
  fi
  pass "doctor --fix without a Go toolchain reports that it could not repair"
}

# The TUI's "Estado" row folds status-hooks in, so the overlay's own
# status path is where an operator sees this without any new TUI code.
case_status_hooks_surfaces_a_stale_engine_binary() {
  local dir out hits
  dir="$(new_case_dir status-hooks-stale)"
  write_fake_engine_tree "$dir"
  write_stale_binary "$dir/binary/gentle-ai-overlay"

  out="$(
    env PATH="$dir/bin:/usr/bin:/bin" bash -c '
      overlay="$1"; binary="$2"; src="$3"
      source "$overlay"
      set +e
      ENGINE_BINARY="$binary"
      ENGINE_SRC="$src"
      cmd_status_hooks
    ' _ "$OVERLAY" "$dir/binary/gentle-ai-overlay" "$dir/engine" 2>&1
  )"

  hits="$(grep -c -i -F -e "stale" <<<"$out")"
  if [[ "$hits" -eq 0 ]]; then
    fail "status does not surface a stale engine binary" "$out"
    return
  fi
  # status is a summary, not a report.
  if [[ "$hits" -ne 1 ]]; then
    fail "status surfaces the stale engine binary in $hits lines, not one" "$out"
    return
  fi
  if ! grep -q -i -F -e "unknown flag" <<<"$out"; then
    fail "status does not name the unknown-flag symptom" "$out"
    return
  fi
  pass "status surfaces a stale engine binary in one line"
}

# ---------------------------------------------------------------------------

case_parallel_adds_keep_every_target
case_add_does_not_use_a_shared_temp_path
case_remove_does_not_use_a_shared_temp_path
case_parallel_removes_keep_every_other_target
case_add_racing_a_remove_is_not_lost
case_binary_removal_racing_an_add_is_serialized
case_simultaneous_stale_breakins_admit_only_one
case_failed_critical_section_releases_the_lock
case_corrupt_tracking_file_does_not_wedge_cleanup
case_unknown_wellformed_target_keeps_the_guard_closed
case_nul_byte_makes_a_wellformed_name_garbage
case_partial_final_line_is_garbage
case_partial_line_does_not_discard_valid_lines
case_removal_preserves_an_unknown_wellformed_target
case_unreadable_tracking_file_keeps_the_guard_closed
case_unreadable_tracking_file_is_not_truncated
case_install_failure_is_reported_with_recovery_text
case_register_refusal_does_not_mark_target_installed
case_register_hard_failure_fails_the_run
case_register_failure_under_target_all_converges
case_register_is_told_the_deployed_binary_path
case_register_path_unresolvable_is_named
case_register_path_unresolvable_under_target_all_is_named
case_unregister_path_unresolvable_is_named
case_unregister_unmanaged_keeps_the_target_tracked
case_unregister_version_skew_converges
case_register_version_skew_fails_the_run
case_install_succeeds_under_errexit
case_undeployable_binary_is_not_reported_as_deployed
case_binary_older_than_engine_source_is_rebuilt
case_binary_older_than_go_mod_is_rebuilt
case_binary_older_than_go_sum_is_rebuilt
case_binary_newer_than_every_source_is_not_rebuilt
case_absent_binary_is_still_built
case_stale_binary_without_go_warns_and_continues
case_stale_binary_with_failing_build_warns_and_continues
case_doctor_reports_a_stale_engine_binary
case_doctor_reports_an_absent_engine_binary_differently
case_doctor_reports_a_current_engine_binary_as_healthy
case_doctor_fix_rebuilds_a_stale_engine_binary
case_doctor_fix_without_go_does_not_claim_a_repair
case_status_hooks_surfaces_a_stale_engine_binary

if [[ "$failures" -gt 0 ]]; then
  echo "$failures shell test case(s) failed" >&2
  exit 1
fi
echo "all shell test cases passed"
