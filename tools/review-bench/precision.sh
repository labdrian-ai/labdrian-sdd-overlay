#!/usr/bin/env bash
# Bench precision (phase9-c28-review-precision S6): joins the labels of the blind judge to the frozen findings of both
# runners by key and prints, ours and gentle side by side, one table of "group metric ours gentle" rows.
#   usage: precision.sh <ours-findings> <gentle-findings> <labels> [<repro>]
# The findings files are the arrays findings.sh prints; each finding's runner and case come from the finding itself.
# The labels file is {"labels":[{key, label, failure_stated, ...}]} with label real, false, unsure or not-judged; a
# finding with no label is not-judged. The optional repro file has the same shape and its label of a key replaces the
# reading label (S4). A label whose key is in neither findings file, a key labeled twice in one file or another label
# value exits 2 and prints nothing on stdout.
# Groups: headline first, then overall, failure_stated=true and failure_stated=false (judged findings only),
# severity=<s>, row=<row> and case=<id>. Metrics per group: real, false, unsure, not-judged; precision
# real / (real + false); low real / judged (every unsure counted false); high (real + unsure) / judged (every unsure
# counted real); judged is every finding but not-judged. The headline leaves unsure out and counts a real finding
# that states no failure as false: precision real_failure / (real_failure + false + real_no_failure), then unsure
# (the count) and its low and high bounds over the same findings plus every unsure (an unsure that states no failure
# counts false in both). A ratio over 0 findings prints null; a group with no finding of a runner prints - for it.
set -u
[ "$#" -eq 3 ] || [ "$#" -eq 4 ] || { echo "usage: precision.sh <ours-findings> <gentle-findings> <labels> [<repro>]" >&2; exit 2; }
for f in "$@"; do
  [ -f "$f" ] || { echo "no file at $f" >&2; exit 2; }
done
out=$(jq -n -r --slurpfile ours "$1" --slurpfile gentle "$2" --slurpfile labels "$3" --slurpfile repro "${4:-/dev/null}" '
  def ratio(n; d): if d == 0 then null else (n / d * 1000 | round) / 1000 end;
  def count(f): map(select(f)) | length;
  def stats:
    count(.label == "real") as $r | count(.label == "false") as $f | count(.label == "unsure") as $u
    | count(.label == "not-judged") as $n | ($r + $f + $u) as $j
    | [["real", $r], ["false", $f], ["unsure", $u], ["not-judged", $n],
       ["precision", ratio($r; $r + $f)], ["low", ratio($r; $j)], ["high", ratio($r + $u; $j)]];
  def headline:
    count(.label == "real" and .failure_stated == true) as $rf
    | count(.label == "real" and .failure_stated != true) as $rn
    | count(.label == "false") as $f | count(.label == "unsure") as $u
    | count(.label == "unsure" and .failure_stated == true) as $uf
    | ($rf + $rn + $f) as $d
    | [["precision", ratio($rf; $d)], ["unsure", $u], ["low", ratio($rf; $d + $u)], ["high", ratio($rf + $uf; $d + $u)]];
  def check(file):
    (file // error("a labels file holds no labels array")) as $ls
    | ([$ls[] | select(.label | IN("real", "false", "unsure", "not-judged") | not) | .key] | unique) as $bad
    | if $bad != [] then error("label not real, false, unsure or not-judged: \($bad | join(" "))") else . end
    | ([$ls[].key] | group_by(.) | map(select(length > 1)[0])) as $twice
    | if $twice != [] then error("key labeled twice: \($twice | join(" "))") else . end
    | $ls;
  ($ours[0] + $gentle[0]) as $fs
  | ($labels[0].labels | check(.)) as $ls
  | (if $repro == [] then [] else ($repro[0].labels | check(.)) end) as $rs
  | ([$fs[].key]) as $keys
  | ([$ls[], $rs[]] | map(.key) | unique | map(select(IN($keys[]) | not))) as $unknown
  | if $unknown != [] then error("label key in neither findings file: \($unknown | join(" "))") else . end
  | ((($ls | map({(.key): .}) | add) // {}) + (($rs | map({(.key): .}) | add) // {})) as $by
  | [$fs[] | . + {label: ($by[.key].label // "not-judged"), failure_stated: $by[.key].failure_stated}] as $items
  | def side(rows): [rows[] | [.[0], (.[1] | if . == null then "null" else tostring end)]];
    def group(name; f; m):
      ([$items[] | select(.runner == "ours") | select(f)]) as $o
      | ([$items[] | select(.runner == "gentle") | select(f)]) as $g
      | (if $o == [] then null else side($o | m) end) as $os
      | (if $g == [] then null else side($g | m) end) as $gs
      | [range(0; ($os // $gs) | length) as $i
         | [name, (($os // $gs)[$i][0]), (if $os == null then "-" else $os[$i][1] end), (if $gs == null then "-" else $gs[$i][1] end)]];
    def pad(n): if length < n then . + (" " * (n - length)) else . end;
    ([["group", "metric", "ours", "gentle"]]
     + group("headline"; true; headline)
     + group("overall"; true; stats)
     + group("failure_stated=true"; .label != "not-judged" and .failure_stated == true; stats)
     + group("failure_stated=false"; .label != "not-judged" and .failure_stated != true; stats)
     + ([$items[].severity] | unique | map(. as $s | group("severity=\($s)"; .severity == $s; stats)) | add)
     + ([$items[].row] | unique | map(. as $r | group("row=\($r)"; .row == $r; stats)) | add)
     + ([$items[].id] | unique | map(. as $c | group("case=\($c)"; .id == $c; stats)) | add))
    | .[] | "\(.[0] | pad(24)) \(.[1] | pad(11)) \(.[2] | pad(8)) \(.[3])"
' 2>&1) || { echo "${out#jq: error (at <unknown>): }" >&2; exit 2; }
printf '%s\n' "$out"
