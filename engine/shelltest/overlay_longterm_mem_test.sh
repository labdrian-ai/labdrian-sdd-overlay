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

case_parallel_adds_keep_every_target
case_add_does_not_use_a_shared_temp_path
case_remove_does_not_use_a_shared_temp_path
case_corrupt_tracking_file_does_not_wedge_cleanup
case_install_failure_is_reported_with_recovery_text
case_register_refusal_does_not_mark_target_installed
case_register_hard_failure_fails_the_run
case_register_failure_under_target_all_converges
case_register_is_told_the_deployed_binary_path
case_install_succeeds_under_errexit
case_undeployable_binary_is_not_reported_as_deployed

if [[ "$failures" -gt 0 ]]; then
  echo "$failures shell test case(s) failed" >&2
  exit 1
fi
echo "all shell test cases passed"
