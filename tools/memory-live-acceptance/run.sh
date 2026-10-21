#!/usr/bin/env bash
# C23 M11, ADR decision 3. Manual only: authorize ONE runtime, then run:
# LABDRIAN_LIVE_MEMORY=1 KEEP=1 bash tools/memory-live-acceptance/run.sh claude
# Requires labdrian (or LABDRIAN_BIN), git, timeout and the selected runtime.
# LIVE_TIMEOUT is a positive number of seconds (default 300). KEEP=1 retains
# transcripts; otherwise their printed paths are removed on exit. This script
# installs no hooks/skills: it measures the runtime's existing memory wiring.
# The verdict requires an exact MEMORY_PLAN_SOURCES report line, ignoring
# surrounding whitespace/backticks/asterisks, not prompt echoes or prose.
set -euo pipefail

PROMPT='For this session, identify the memory sources you may read and the project name to use for Engram reads. Take both from the memory plan instructions you were given, or run labdrian memory plan --instructions. For claude only, also call the Engram mem_context read tool exactly once for that project and say that you did. For codex and pi, do not call a memory read tool. END your reply with exactly one line in this format, replacing the placeholders with the plan values: MEMORY_PLAN_SOURCES=<comma-separated readable sources>; PROJECT=<project name for Engram reads>'

if [[ ${LABDRIAN_LIVE_MEMORY:-} != 1 ]]; then
    printf '%s\n' 'live memory acceptance requires LABDRIAN_LIVE_MEMORY=1 (authorize one runtime per run)' >&2
    exit 2
fi
if [[ $# != 1 ]]; then
    printf '%s\n' 'usage: run.sh <claude|codex|pi>' >&2
    exit 2
fi
runtime=$1
case "$runtime" in
    claude|codex|pi) ;;
    *) printf '%s\n' 'usage: run.sh <claude|codex|pi>' >&2; exit 2 ;;
esac
labdrian=${LABDRIAN_BIN:-labdrian}
for cli in "$runtime" "$labdrian" git timeout mktemp mkdir rm cat; do
    if ! command -v "$cli" >/dev/null 2>&1; then
        printf 'required CLI not found: %s\n' "$cli" >&2
        exit 2
    fi
done
limit=${LIVE_TIMEOUT:-300}
if [[ ! $limit =~ ^[0-9]+$ || $limit =~ ^0+$ ]]; then
    printf '%s\n' 'LIVE_TIMEOUT must be a positive integer in seconds' >&2
    exit 2
fi

scratch=$(mktemp -d)
cleanup() {
    if [[ ${KEEP:-} == 1 ]]; then
        printf 'Scratch kept: %s\n' "$scratch"
    else
        rm -rf -- "$scratch"
    fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
repo=$scratch/proj-1
export XDG_STATE_HOME=$scratch/state
transcript=$scratch/$runtime.out
: > "$transcript"
finish() {
    local reason=${1:-}
    if [[ -z $reason ]]; then
        printf 'PASS %s\n' "$runtime"
    else
        printf 'FAIL %s: %s\n' "$runtime" "$reason"
    fi
    printf 'Transcript: %s\n' "$transcript"
    printf '%s\n' 'A pass is one session on one day, not a guarantee.'
    [[ -z $reason ]]
}

# Goal v2 has the exact JSON shape of memoryTestGoalJSON("proj-1", "goal-1") in
# engine/cmd/memory_test.go, also used by memory_acceptance_test.go.
cat > "$scratch/goal.json" <<'GOAL'
{
  "version": 2,
  "project_id": "proj-1",
  "goal_id": "goal-1",
  "objective": "obj",
  "scope": "scope",
  "constraints": [],
  "non_goals": [],
  "acceptance_criteria": ["done"],
  "memory_scope": "goal",
  "runtime_scope": "local",
  "delivery_boundary": "local"
}
GOAL
# Use a local origin named for the project: hosted origins normalize to
# host/path, whereas this fixture must retain identity proj-1. No remote I/O.
# Disable external git templates/config, without changing HOME (credentials).
setup() {
    mkdir -p "$repo" "$XDG_STATE_HOME" &&
    cd "$repo" &&
    GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null git init -q --template= &&
    GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null git remote add origin "$scratch/proj-1.git" &&
    "$labdrian" workflow create --project proj-1 --workflow wf-1 --goal "$scratch/goal.json" --profile sdd &&
    "$labdrian" workflow start --project proj-1 --workflow wf-1 &&
    "$labdrian" workflow bind --project proj-1 --workflow wf-1
}
# Resolve an override before changing directories (it may be a relative path).
labdrian=$(command -v "$labdrian")
if [[ $labdrian != /* ]]; then
    labdrian=$PWD/$labdrian
fi
if ! setup > "$scratch/setup.out" 2>&1; then
    finish "workflow setup failed (see $scratch/setup.out)"
    exit 1
fi

args=(-p "$PROMPT")
if [[ $runtime == codex ]]; then
    args=(exec --cd "$repo" "$PROMPT")
fi
code=0
timeout --kill-after=5 "$limit" "$runtime" "${args[@]}" </dev/null > "$transcript" 2>&1 || code=$?
if [[ $code != 0 ]]; then
    finish "runtime exited $code (timeout exits 124)"
    exit 1
fi
matched=0
reported=''
while IFS= read -r line || [[ -n $line ]]; do
    # Strip only the permitted edge decoration, leaving the report itself exact.
    while [[ $line == [[:space:]\`\*]* ]]; do
        line=${line#?}
    done
    while [[ $line == *[[:space:]\`\*] ]]; do
        line=${line%?}
    done
    if [[ $line == MEMORY_PLAN_SOURCES=* ]]; then
        reported=$line
    fi
    if [[ $line == 'MEMORY_PLAN_SOURCES=engram; PROJECT=proj-1' ]]; then
        matched=1
    fi
done < "$transcript"
if [[ $matched == 0 ]]; then
    if [[ -n $reported ]]; then
        finish "agent reported: $reported"
    else
        finish 'no MEMORY_PLAN_SOURCES line'
    fi
    exit 1
fi
sidecar=$XDG_STATE_HOME/labdrian/workflows/proj-1/wf-1.memory.jsonl
if [[ $runtime == claude ]] && { [[ ! -f $sidecar ]] || ! IFS= read -r _ < "$sidecar"; }; then
    finish 'no Claude memory-read evidence line'
    exit 1
fi
finish
