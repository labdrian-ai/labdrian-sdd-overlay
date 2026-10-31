#!/usr/bin/env bash
# Bench scorer: for each case id, prints every finding and analyzer line of $OUT/<id>/summary.json located in the
# defect's file within its lines +-3, and the prompt bytes of the case (sum of runs[].in). The parent judges each
# printed line by the scoring rule of README.md and records the score; this script never scores by itself.
set -u
bench="${BENCH:-$HOME/labdrian-sdd-overlay-worktrees/review-bench}"
here="$(cd "$(dirname "$0")" && pwd)"
out_dir="${OUT:-$bench/out}"
[ "$#" -gt 0 ] || { echo "usage: score.sh <id>..."; exit 2; }
status=0
for id in "$@"; do
  case_json=$(jq -c --arg id "$id" '.real[] | select(.id == $id)' "$here/corpus.json")
  [ -n "$case_json" ] || { echo "unknown case $id"; exit 2; }
  summary="$out_dir/$id/summary.json"
  if [ ! -f "$summary" ]; then
    echo "$id no summary at $summary"
    status=1
    continue
  fi
  jq -r --argjson c "$case_json" '
    def span: split("-") | map(tonumber) | {from: .[0], to: (.[1] // .[0])};
    ($c.defect.file) as $file | ($c.defect.lines | span) as $d
    | def near($loc): ($loc | capture("^(?<f>[^:]+):(?<l>[0-9]+(-[0-9]+)?)")) as $m
        | $m != null and $m.f == $file
          and (($m.l | span) as $s | $s.from <= $d.to + 3 and $s.to >= $d.from - 3);
    ([ (.lenses // [])[] | .lens as $lens | .findings[] | select(near(.location))
        | "  finding \($lens) \(.severity) \(.location): \(.claim)" ]
    + [ (.analyzers // [])[] | select(near(.)) | "  analyzer \(.)" ]) as $lines
    | "\($c.id) \($file):\($c.defect.lines) prompt_bytes=\([(.runs // [])[].in | tonumber] | add // 0)",
      (if ($lines | length) == 0 then "  none located" else $lines[] end)
  ' "$summary" || status=1
done
exit "$status"
