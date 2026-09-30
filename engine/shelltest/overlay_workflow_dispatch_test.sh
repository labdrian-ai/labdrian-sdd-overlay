#!/usr/bin/env bash
#
# Shell-level tests for the `labdrian workflow <verb>` dispatch in
# bin/labdrian-overlay. The overlay forwards workflow verbs to the engine
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

work_root="$(mktemp -d "${TMPDIR:-/tmp}/overlay-workflow-shelltest.XXXXXX")"
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
  out="$(HOME="$home" bash "$OVERLAY" workflow create --project proj-1 --workflow wf-1 --goal "goal 1.json" --profile odd 2>&1)"
  status=$?
  if [[ $status -eq 0 && "$out" == *"engine-argv: [workflow] [create] [--project] [proj-1] [--workflow] [wf-1] [--goal] [goal 1.json] [--profile] [odd]"* ]]; then
    pass "workflow verbs and arguments reach the engine verbatim"
  else
    fail "workflow dispatch did not forward argv verbatim" "status=$status" "output=$out"
  fi
}

case_preserves_engine_exit_status() {
  local home status
  home="$(fake_home status)"
  HOME="$home" FAKE_ENGINE_STATUS=2 bash "$OVERLAY" workflow status --project proj-1 --workflow wf-1 >/dev/null 2>&1
  status=$?
  if [[ $status -eq 2 ]]; then
    pass "the engine exit status is preserved"
  else
    fail "engine exit status was not preserved" "status=$status, want 2"
  fi
}

case_refuses_when_engine_missing() {
  local home out status
  home="$work_root/missing"
  mkdir -p "$home"
  out="$(HOME="$home" bash "$OVERLAY" workflow status --project proj-1 --workflow wf-1 2>&1)"
  status=$?
  if [[ $status -ne 0 && "$out" == *"engine binary not found"* ]]; then
    pass "a missing engine binary is refused with a clear message"
  else
    fail "missing engine binary was not refused" "status=$status" "output=$out"
  fi
}

case_help_lists_workflow_with_its_own_verbs() {
  local line
  line="$(HOME="$work_root" bash "$OVERLAY" --help 2>&1 | grep -A1 '^  workflow <verb>' | tail -1)"
  if [[ "$line" == *"Local bookkeeping"* ]]; then
    pass "--help lists workflow with its own verbs, not the memory sub-verbs"
  else
    fail "--help attaches the wrong sub-verbs to workflow" "next line: $line"
  fi
}

case_help_lists_the_binding_verbs() {
  local block verb missing=""
  # The workflow entry runs from its "  workflow <verb>" header to the next
  # command header (two spaces then a letter); its continuation lines are
  # indented much deeper.
  block="$(HOME="$work_root" bash "$OVERLAY" --help 2>&1 | awk '/^  workflow <verb>/ { on = 1; next } on && /^  [a-z]/ { exit } on { print }')"
  # Each verb is matched with its own help wording: the bare word "binding"
  # already appears in the description of create.
  for verb in "bind   --project ID --workflow ID" "unbind    remove this repository's binding" "binding   read-only"; do
    if [[ "$block" != *"$verb"* ]]; then
      missing="$missing [$verb]"
    fi
  done
  if [[ -z "$missing" ]]; then
    pass "--help lists the workflow binding verbs"
  else
    fail "--help does not list the workflow binding verbs" "missing:$missing"
  fi
}

case_help_states_every_binding_refusal() {
  local block phrase missing=""
  block="$(HOME="$work_root" bash "$OVERLAY" --help 2>&1 | awk '/^  workflow <verb>/ { on = 1; next } on && /^  [a-z]/ { exit } on { print }')"
  # Exit 2 covers every refusal of the binding verbs, not only a binding file
  # that is not ours: an unusable file, a repository whose lock is taken, and a
  # binding another process changed. Each phrase sits on one help line.
  for phrase in "not ours or cannot be used" "(foreign, malformed, unavailable)" "in progress (busy)" "another process changed"; do
    if [[ "$block" != *"$phrase"* ]]; then
      missing="$missing [$phrase]"
    fi
  done
  if [[ -z "$missing" ]]; then
    pass "--help states every refusal the binding verbs exit 2 for"
  else
    fail "--help leaves out binding refusals from its exit codes" "missing:$missing"
  fi
}

case_help_lists_workflow_with_its_own_verbs
case_help_lists_the_binding_verbs
case_help_states_every_binding_refusal
case_forwards_verb_and_args_verbatim
case_preserves_engine_exit_status
case_refuses_when_engine_missing

if [[ $failures -gt 0 ]]; then
  echo "$failures case(s) failed" >&2
  exit 1
fi
echo "all workflow dispatch cases passed"
