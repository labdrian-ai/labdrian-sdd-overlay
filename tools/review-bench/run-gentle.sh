#!/usr/bin/env bash
# Bench driver: runs gentle-ai native review (lenses risk + reliability) on one corpus case of corpus.json beside it and
# writes $BENCH/out/<id>/summary.json with the finding count per lens, the bytes sent and the time taken.
set -u
export BENCH="${BENCH:-$HOME/labdrian-sdd-overlay-worktrees/review-bench}"
bench="$BENCH"
here="$(cd "$(dirname "$0")" && pwd)"
repo="$bench/repo"
id="$1"
case_json=$(jq -c --arg id "$id" '(.real + .mutants)[] | select(.id == $id)' "$here/corpus.json")
[ -n "$case_json" ] || { echo "unknown case $id" >&2; exit 2; }
head_ref=$(jq -r '.head' <<<"$case_json")
base_ref=$(jq -r '.range | split("..")[0]' <<<"$case_json")
out="$bench/out/$id"
# A rerun starts from an empty directory, so no capture or final result of an earlier run is read.
rm -rf "$out"
mkdir -p "$out" "$bench/scratch"
export PATH="$here/shim:$PATH"
cd "$repo" || exit 2
git checkout -q --detach "$head_ref" || exit 2
base=$(git rev-parse "$base_ref")
start_lines=$(wc -l < "$bench/scratch/shim.log" 2>/dev/null || echo 0)
plain_status() {
  gentle-ai review status --cwd "$repo" --contract gentle-ai.review-integration/v2 --agent claude-code \
    --next-transition --base-ref "$base" --committed-only
}
# Runs the acknowledgement an approved review waits for, so its authority is burned and the case can be rerun.
acknowledge_pending() {
  plain_status > "$out/ack-status.json" 2> "$out/ack-status.err"
  [ "$(jq -r '.next_transition.reason_code' "$out/ack-status.json")" = "approved_acknowledgement_required" ] || return 0
  bash -c "$(jq -r '.next_transition.execute.command' "$out/ack-status.json")" > "$out/ack.json" 2> "$out/ack.err"
}
t0=$(date +%s)

gentle-ai review status --cwd "$repo" --contract gentle-ai.review-integration/v2 --agent claude-code \
  --next-transition --base-ref "$base" --committed-only --lenses reliability,risk \
  --lenses-reason "bench $id: committed change under review" > "$out/status0.json" 2> "$out/status0.err"
cmd=$(jq -r '.next_transition.execute.command // empty' "$out/status0.json")
if [ -z "$cmd" ]; then
  # A case reviewed by an earlier run stops on target_already_acknowledged (after its approval is acknowledged,
  # if an earlier run left it pending); its contract names the START of a deliberate new independent review,
  # built from the fields of the plain STATUS.
  acknowledge_pending
  plain_status > "$out/status0-plain.json" 2> "$out/status0-plain.err"
  [ "$(jq -r '.next_transition.reason_code' "$out/status0-plain.json")" = "target_already_acknowledged" ] \
    || { echo "no start command" >&2; exit 3; }
  mapfile -t start_args < <(jq -r '.target_identity, .projection.projection, .projection.base_tree' "$out/status0-plain.json")
  cmd=$(printf '%q ' gentle-ai review start --cwd "$repo" --contract gentle-ai.review-integration/v2 \
    --agent claude-code --target "${start_args[0]}" --projection "${start_args[1]}" \
    --base-ref "${start_args[2]}" --committed-only --consent relay --lenses reliability,risk \
    --lenses-reason "bench $id: committed change under review")
fi
bash -c "$cmd" > "$out/start.json" 2> "$out/start.err"
if [ "$(jq -r '.action' "$out/start.json")" = "consent_required" ]; then
  granted=$(jq -r '.choices[] | select(.answer == "granted") | .invocation' "$out/start.json")
  bash -c "$granted" > "$out/started.json" 2> "$out/started.err"
else
  cp "$out/start.json" "$out/started.json"
fi
status_cmd=$(jq -r '.next_transition.execute.command // empty' "$out/started.json")
[ -n "$status_cmd" ] || { echo "no status command after start" >&2; exit 3; }

state=""
for round in 1 2 3 4; do
  bash -c "$status_cmd" > "$out/status$round.json" 2> "$out/status$round.err"
  kind=$(jq -r '.next_transition.kind' "$out/status$round.json")
  [ "$kind" = "collect" ] || { state="status:$kind:$(jq -r '.next_transition.reason_code' "$out/status$round.json")"; break; }
  n=$(jq '.next_transition.collect.inputs | length' "$out/status$round.json")
  pids=()
  for i in $(seq 0 $((n - 1))); do
    mapfile -t args < <(jq -r --argjson i "$i" '.next_transition.collect.inputs[$i].arguments[] | .token // .' "$out/status$round.json")
    gentle-ai review capture-result "${args[@]}" > "$out/cap$round-$i.json" 2> "$out/cap$round-$i.err" &
    pids+=($!)
  done
  for p in "${pids[@]}"; do wait "$p"; done
  for f in "$out"/cap"$round"-*.json; do
    s=$(jq -r '.state // empty' "$f" 2>/dev/null)
    case "$s" in approved|correction_required) state="$s"; cp "$f" "$out/final.json";; esac
  done
  [ -n "$state" ] && break
done

[ "$state" = "approved" ] && acknowledge_pending
t1=$(date +%s)
shim_runs=$(tail -n +"$((start_lines + 1))" "$bench/scratch/shim.log")
# A correction_required capture carries no findings: they are kept in the transaction's review state.
lineage=$(jq -r '.lineage_id // empty' "$out/started.json")
review_state="$repo/.git/gentle-ai/review-transactions/v2/$lineage/review-state.json"
jq -n --arg id "$id" --arg state "$state" --argjson secs "$((t1 - t0))" \
  --arg shim "$shim_runs" \
  --slurpfile final <(cat "$out/final.json" 2>/dev/null || echo '{}') \
  --slurpfile rs <(cat "$review_state" 2>/dev/null || echo '{}') '{
    id: $id, state: $state, secs: $secs,
    lenses: [($final[0].reviewer_results // [($rs[0].state.admitted_role_results // [])[].value.result])[]
      | select(.lens != null) | {lens, n: (.findings | length), findings}],
    runs: [$shim | split("\n")[] | select(length > 0) | capture("stdin_bytes=(?<in>[0-9]+) stdout_bytes=(?<out>[0-9]+) exit=(?<exit>[0-9]+) secs=(?<secs>[0-9.]+)")]
  }' > "$out/summary.json"
jq -c '{id, state, secs, lenses: [.lenses[] | {lens, n}], runs: (.runs | length)}' "$out/summary.json"
case "$state" in approved|correction_required) ;; *) echo "case $id ended without a review result: ${state:-none}" >&2; exit 4;; esac
