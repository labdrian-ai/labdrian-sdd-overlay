#!/usr/bin/env bash
# Bench driver for our review: on one corpus case of corpus.json beside it, runs `labdrian review sweep` and then one
# `labdrian review launch --runtime claude` per row of ROWS (default default,hunt), and writes
# $BENCH/out/ours/<id>/summary.json in the shape of run-gentle.sh's, plus the sweep's analyzer lines.
set -u
export BENCH="${BENCH:-$HOME/labdrian-sdd-overlay-worktrees/review-bench}"
bench="$BENCH"
here="$(cd "$(dirname "$0")" && pwd)"
rows_file="${ROWS_FILE:-$here/../../roles/reviewer/rows.json}"
rows="${ROWS:-default,hunt}"
model="${MODEL:-claude-sonnet-5-5}"
repo="$bench/repo"
[ "$#" -eq 1 ] || { echo "usage: run-ours.sh <id>" >&2; exit 2; }
id="$1"
case_json=$(jq -c --arg id "$id" '(.real + .mutants)[] | select(.id == $id)' "$here/corpus.json")
[ -n "$case_json" ] || { echo "unknown case $id" >&2; exit 2; }
head_ref=$(jq -r '.head' <<<"$case_json")
base_ref=$(jq -r '.range | split("..")[0]' <<<"$case_json")
IFS=, read -r -a row_ids <<<"$rows"
for row in "${row_ids[@]}"; do
  jq -e --arg r "$row" 'any(.rows[]; .id == $r)' "$rows_file" > /dev/null || { echo "unknown row $row" >&2; exit 2; }
done
out="$bench/out/ours/$id"
# A rerun starts from an empty directory, so no launch or result of an earlier run is read.
rm -rf "$out"
mkdir -p "$out" "$bench/scratch"
export PATH="$here/shim:$PATH"
cd "$repo" || exit 2
git checkout -q --detach "$head_ref" || exit 2
base=$(git rev-parse "$base_ref")
head=$(git rev-parse HEAD)
results="$repo/odd/review-results/review-bench"
start_lines=$(wc -l 2>/dev/null < "$bench/scratch/shim.log" || echo 0)
t0=$(date +%s)

state="recorded"
lenses="[]"
analyzers="[]"
if ! labdrian review sweep --feature review-bench --base "$base" --head "$head" --cwd "$repo" \
    > "$out/sweep.json" 2> "$out/sweep.err"; then
  state="sweep failed"
else
  # A T2 candidate is "T2 <tool> <module>: <line>", the line relative to the module; it is kept as a repository path.
  analyzers=$(jq -c '[.candidates[] | capture("^T2 (?<tool>\\S+) (?<module>\\S+): (?<line>.*)$")
    | (if .module == "." then .line else .module + "/" + .line end) + " [" + .tool + "]"]' "$out/sweep.json")
fi
if [ "$state" = "recorded" ]; then
  if ! labdrian review bind --base "$base" --head "$head" --cwd "$repo" > "$out/bind.json" 2> "$out/bind.err"; then
    state="bind failed"
  fi
fi
for row in "${row_ids[@]}"; do
  [ "$state" = "recorded" ] || break
  sentence=$(jq -r --arg r "$row" '.rows[] | select(.id == $r) | .sentence' "$rows_file")
  jq -r --arg row "$row" --arg sentence "$sentence" '"Review the committed candidate below as the reviewer role.",
    "base: \(.base)", "head: \(.head)", "diff_sha256: \(.diff_sha256)", "worktree: \(.worktree)",
    "Lens row \($row): \($sentence)",
    "No tool is available to you: the frozen context below is the whole of what you can inspect; set inspection.status to completed when it covers every changed path."' \
    "$out/bind.json" > "$out/brief-$row.txt"
  before=$(compgen -G "$results/${head:0:12}-*.json" | sort)
  if ! labdrian review launch --runtime claude --feature review-bench --base "$base" --head "$head" --rows "$row" \
      --model "$model" --cwd "$repo" < "$out/brief-$row.txt" > "$out/launch-$row.json" 2> "$out/launch-$row.err"; then
    state="launch failed: $row"
    break
  fi
  # The launch stores its result as the next <head12>-<n>.json; the one file that was not there before is it.
  record=""
  found=0
  while IFS= read -r f; do
    [[ "$f" =~ /[0-9a-f]{12}-[0-9]+\.json$ ]] && { record="$f"; found=$((found + 1)); }
  done < <(comm -13 <(printf '%s\n' "$before") <(compgen -G "$results/${head:0:12}-*.json" | sort))
  [ "$found" -eq 1 ] || { state="no result recorded: $row"; break; }
  lenses=$(jq -c --arg row "$row" --argjson l "$lenses" '$l + [{lens: $row, n: (.reply.findings | length), findings: .reply.findings}]' "$record")
done
t1=$(date +%s)

shim_runs=$(tail -n +"$((start_lines + 1))" "$bench/scratch/shim.log" 2>/dev/null)
jq -n --arg id "$id" --arg state "$state" --argjson secs "$((t1 - t0))" --argjson lenses "$lenses" \
  --argjson analyzers "$analyzers" --arg shim "$shim_runs" '{
    id: $id, state: $state, secs: $secs, lenses: $lenses, analyzers: $analyzers,
    runs: [$shim | split("\n")[] | select(length > 0) | capture("stdin_bytes=(?<in>[0-9]+) stdout_bytes=(?<out>[0-9]+) exit=(?<exit>[0-9]+) secs=(?<secs>[0-9.]+)")]
  }' > "$out/summary.json"
jq -c '{id, state, secs, lenses: [.lenses[] | {lens, n}], analyzers: (.analyzers | length), runs: (.runs | length)}' "$out/summary.json"
[ "$state" = "recorded" ] || { echo "case $id ended without a review result: $state" >&2; exit 4; }
