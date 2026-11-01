# review-bench

A measurement harness for Phase 9 item C25 (`odd/tasks/phase9-c25-review-hunt.md`): it runs a code review over
commits that are known to carry a defect a later commit fixed, and records what the review found and how many
prompt bytes it sent. It decides whether our own review beats gentle-ai's native review on recall at no higher cost.
It is not part of the product and never runs against the main repository.

## Layout

- `corpus.json`: the cases. `real` holds commits with a real defect (`head`, `base`, `range`, `bytes` of the diff,
  the `fix` commit and the `defect` file, lines and text read from the fix diff); `mutants` holds planted defects.
  Real cases with `"holdout": true` form the hold-out on which the bar is measured.
- `run-gentle.sh <id>`: runs gentle-ai native review (lenses `reliability,risk`, committed range `base..head`) on one
  case and writes `$BENCH/out/<id>/summary.json` (`id`, `state`, `secs`, `lenses[]` with `lens`, `n`, `findings`,
  `runs[]` with the stdin and stdout bytes of every model process).
- `run-ours.sh <id>`: runs our review on one case: `labdrian review sweep` (feature `review-bench`), then
  `labdrian review bind` and one `labdrian review launch --runtime claude` per row of `ROWS` (default `default,hunt`,
  rows read from `ROWS_FILE`, default `roles/reviewer/rows.json`; model `MODEL`, default `claude-sonnet-5-5`), each with
  a brief that names the binding and the row's sentence (`--runtime claude` comes with C25 T4). It writes `$BENCH/out/ours/<id>/summary.json` in the shape of
  gentle's (`state` is `recorded`, or what failed) plus `analyzers[]`, the sweep's T2 lines as repository paths. A
  failed sweep, bind or launch stops the case with exit 4; an unknown case or row is refused with exit 2.
- `shim/claude`: put first on the `PATH` by the runners; logs the arguments and byte sizes of every reviewer process to
  `$BENCH/scratch/shim.log`, then runs `~/bin/claude`.
- `score.sh <id>...`: prints, per case, every finding and analyzer line of `$OUT/<id>/summary.json` located in the
  defect's file within its lines ±3, and the prompt bytes of the case. It never scores: the parent judges each printed
  line by the scoring rule below and records it in `scores/<runner>.json` as `{"id", "score", "quote"}`.

## Running

`BENCH` (default `~/labdrian-sdd-overlay-worktrees/review-bench`) is the bench directory outside the repository: it
holds `repo/`, a separate clone the runners check cases out in, and the outputs `out/` and `scratch/`. `OUT` (default
`$BENCH/out`) selects which runner's summaries `score.sh` reads. The corpus is always the `corpus.json` beside the
scripts.

```sh
tools/review-bench/run-gentle.sh R3
tools/review-bench/score.sh R3
tools/review-bench/run-ours.sh R3
OUT="$BENCH/out/ours" tools/review-bench/score.sh R3
```

## Selection rule

A real case is a commit H whose defect a later commit F on main fixes, with: F's message or diff names the defect it
corrects; the defect's lines lie in a hunk of `git diff H^ H` or a path H created (so a reviewer of H could see it);
`git diff H^ H` is at most 150,000 bytes (`review.DiffLimitBytes`, `engine/review/review.go:80`); the defect is not
pinned as intended by a test at H; H is not already in the corpus. Candidates come from `git log --format='%h %s' main`
filtered to `fix` commits, taken in history order until 23-27 real cases exist, whatever their class; each entry quotes
F's diff for the defect text. Same `base`/`range` convention as R2-R8 (`H^..H`).

## Hold-out

About 10 of the new cases (R9 onward) carry `"holdout": true`: the last 10 the selection rule accepts in history order.
No hold-out defect text, class or diff enters any design artifact (brief template, hunt sentence, analyzer choice); a
change to one of those after the hold-out is recorded voids it. The bar of C25 is measured on the hold-out alone;
numbers over all real cases are evidence only.

## Scoring rule

1 when a finding of any severity, or a new analyzer line, is located in the defect's file within its lines ±3 and its
claim states the defect's failure; 0.5 when such a located finding states only part of it (as R2 was scored on
2026-10-31); 0 otherwise. The parent judges the claim and quotes it in the score file. Cost: the sum of `runs[].in`
(stdin bytes of every model process, from the shim) over the real cases; the analyzers count 0. Tokens are not used:
`--output-format text` reports none for either side (assumption: bytes stand for tokens equally on both sides, the
same model and the same wrapper).
