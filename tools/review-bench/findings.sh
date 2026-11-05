#!/usr/bin/env bash
# Bench findings freezer (phase9-c28-review-precision S2): prints, as one JSON array, every finding of one runner's
# summaries for the case ids given and the runs of RUNS, so they can be committed before a rerun erases them.
# RUNNER=ours reads $BENCH/out/ours/run<k>/<id>/summary.json, RUNNER=gentle $BENCH/out/run<k>/<id>/summary.json.
# Each entry: key (the first 12 hex digits of sha256 of <runner>/<id>/<run>/<row>/<finding>), runner, id, run, row
# (the lens of the summary), finding, severity, location, claim, proof_refs, evidence_class and causal_disposition
# (null when the finding leaves them out) and c27_control: true when a score-1 or score-0.5 quote of
# $SCORES (default scores/<runner>-c27.json) for the same id and run starts with "<row> <SEVERITY> <location>:" of
# exactly one finding. A quote that names several findings marks none and is named on stderr. Nothing is printed
# on stdout unless every summary is read.
set -u
bench="${BENCH:-$HOME/labdrian-sdd-overlay-worktrees/review-bench}"
here="$(cd "$(dirname "$0")" && pwd)"
runner="${RUNNER:-}"
case "$runner" in
  ours) out_dir="$bench/out/ours" ;;
  gentle) out_dir="$bench/out" ;;
  *) echo "RUNNER must be ours or gentle, not $runner" >&2; exit 2 ;;
esac
[ "$#" -gt 0 ] || { echo "usage: RUNNER=ours|gentle findings.sh <id>..." >&2; exit 2; }
runs="${RUNS:-1 2 3}"
for k in $runs; do
  case "$k" in
    '' | *[!0-9]* | 0*) echo "RUNS must hold positive integers, not $k" >&2; exit 2 ;;
  esac
done
for id in "$@"; do
  jq -e --arg id "$id" 'any(.real[]; .id == $id)' "$here/corpus.json" >/dev/null || { echo "unknown case $id" >&2; exit 2; }
  for k in $runs; do
    summary="$out_dir/run$k/$id/summary.json"
    [ -f "$summary" ] || { echo "$id run $k: no summary at $summary" >&2; exit 2; }
  done
done
scores="${SCORES:-$here/scores/$runner-c27.json}"
quotes='[]'
if [ -f "$scores" ]; then
  quotes=$(jq -c '[.[] | select(.score > 0)]' "$scores") || exit 2
fi
entries=()
for id in "$@"; do
  for k in $runs; do
    summary="$out_dir/run$k/$id/summary.json"
    rows=$(jq -c --arg runner "$runner" --arg id "$id" --argjson run "$k" --argjson quotes "$quotes" '
      [ (.lenses // [])[] | .lens as $row | (.findings // [])[]
        | {runner: $runner, id: $id, run: $run, row: $row, finding: .id, severity, location, claim,
           proof_refs: (.proof_refs // []), evidence_class: (.evidence_class // null),
           causal_disposition: (.causal_disposition // null),
           prefix: "\($row) \(.severity) \(.location):"} ] as $fs
      | [ $quotes[] | select(.id == $id and .run == $run) | .quote as $q
          | {quote: $q, hits: [ $fs[] | select(.prefix as $p | $q | startswith($p)) | [.row, .finding] ]} ] as $qs
      | ($qs[] | select((.hits | length) > 1) | {warn: "quote of \($id) run \($run) names \(.hits | length) findings; none is marked"}),
        ($fs[] | [.row, .finding] as $f
          | . + {c27_control: any($qs[]; .hits == [$f])}
          | del(.prefix))
    ' "$summary") || exit 2
    while IFS= read -r row; do
      [ -n "$row" ] || continue
      warn=$(jq -r '.warn // empty' <<<"$row")
      if [ -n "$warn" ]; then
        echo "$warn" >&2
        continue
      fi
      name=$(jq -r '"\(.runner)/\(.id)/\(.run)/\(.row)/\(.finding)"' <<<"$row")
      key=$(printf '%s' "$name" | sha256sum | cut -c1-12)
      entries+=("$(jq -c --arg key "$key" '{key: $key} + .' <<<"$row")")
    done <<<"$rows"
  done
done
if [ "${#entries[@]}" -eq 0 ]; then
  echo '[]'
else
  printf '%s\n' "${entries[@]}" | jq -s '.'
fi
