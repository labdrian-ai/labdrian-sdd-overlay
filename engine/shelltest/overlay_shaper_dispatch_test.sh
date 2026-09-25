#!/usr/bin/env bash
#
# Shell-level tests for the `labdrian shaper <verb>` dispatch in
# bin/labdrian-overlay. The overlay forwards shaper verbs to the engine
# binary; these cases run the real entrypoint under a throwaway HOME whose
# engine binary is a fake that records its argv, so nothing real is touched.

set -uo pipefail

SCRIPT_DIR="$(cd -P "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
OVERLAY="$REPO_ROOT/bin/labdrian-overlay"

failures=0
pass() { echo "ok   - $*"; }
fail() {
  echo "FAIL - $1" >&2
  shift
  [[ $# -gt 0 ]] && printf '       %s\n' "$@" >&2
  failures=$((failures + 1))
}

work_root="$(mktemp -d "${TMPDIR:-/tmp}/overlay-shaper-shelltest.XXXXXX")"
cleanup() { rm -rf "$work_root"; }
trap cleanup EXIT

# fake_home prints a HOME whose engine binary echoes its arguments and exits
# with the status given in FAKE_ENGINE_STATUS (default 0).
fake_home() {
  local home="$work_root/$1"
  mkdir -p "$home/.claude/bin"
  cat >"$home/.claude/bin/gentle-ai-overlay" <<'ENGINE'
#!/usr/bin/env bash
printf 'engine-argv:'
printf ' [%s]' "$@"
printf '\n'
exit "${FAKE_ENGINE_STATUS:-0}"
ENGINE
  chmod +x "$home/.claude/bin/gentle-ai-overlay"
  printf '%s\n' "$home"
}

case_forwards_verb_and_args_verbatim() {
  local home out status
  home="$(fake_home forward)"
  out="$(HOME="$home" bash "$OVERLAY" shaper clearance record --root "/w t" --stdin 2>&1)"
  status=$?
  if [[ $status -eq 0 && "$out" == *"engine-argv: [shaper] [clearance] [record] [--root] [/w t] [--stdin]"* ]]; then
    pass "shaper verbs and arguments reach the engine verbatim"
  else
    fail "shaper dispatch did not forward argv verbatim" "status=$status" "output=$out"
  fi
}

case_preserves_engine_exit_status() {
  local home status
  home="$(fake_home status)"
  HOME="$home" FAKE_ENGINE_STATUS=3 bash "$OVERLAY" shaper assess --root /w >/dev/null 2>&1
  status=$?
  if [[ $status -eq 3 ]]; then
    pass "the engine exit status (3 = draft) is preserved"
  else
    fail "engine exit status was not preserved" "status=$status, want 3"
  fi
}

case_refuses_when_engine_missing() {
  local home out status
  home="$work_root/missing"
  mkdir -p "$home"
  out="$(HOME="$home" bash "$OVERLAY" shaper assess 2>&1)"
  status=$?
  if [[ $status -ne 0 && "$out" == *"engine binary not found"* ]]; then
    pass "a missing engine binary is refused with a clear message"
  else
    fail "missing engine binary was not refused" "status=$status" "output=$out"
  fi
}

case_forwards_verb_and_args_verbatim
case_preserves_engine_exit_status
case_refuses_when_engine_missing

if [[ $failures -gt 0 ]]; then
  echo "$failures case(s) failed" >&2
  exit 1
fi
echo "all shaper dispatch cases passed"
