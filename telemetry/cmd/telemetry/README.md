# telemetry report

From `telemetry/`, build with `go build -o /tmp/telemetry ./cmd/telemetry`.
Run `telemetry report [--claude-root DIR] [--pi-root DIR] [--json] [--top N] [--prices FILE]`.

Defaults are `$HOME/.claude/projects`, `$HOME/.pi/agent/sessions`, and top 10.
Missing default roots are skipped with a stderr note; explicit missing roots
and no readable source exit 1. Invalid commands/flags exit 2 with usage.
Stats always go to stderr. Logs are read-only; no network is used.

Human output shows summary, role, model, agent type, project, source, and day.
Model/agent/project rows rank by context descending, then key ascending;
`--top` limits those rows and selects the latest N dates. Other rows show all.
Cost shows recorded dollars and covered calls, or `n/a`, never inferred prices.
With `--prices FILE`, a separate **Est. cost** column appears only if at least
one call was estimated: `~$123.45 (n calls)`, or `-` for unestimated groups.
Recorded costs are never re-estimated. Without a matching price, cost stays
unavailable. No built-in prices are provided.
`--json` emits the library's complete `Report.JSON()` without top truncation.
Only aggregate keys and counts are emitted, never conversation content.

## Optional price table

Supply a strict JSON file (all four rates are required, finite, and nonnegative):

```json
{"unit":"usd_per_million_tokens","models":{"exact-model-id":{"input":0,"cache_read":0,"cache_write":0,"output":0}}}
```

Model IDs match exactly: no prefixes, wildcards, or case folding. Unknown fields,
duplicate keys, trailing data, and empty model tables are errors. An explicitly
supplied missing or invalid price file exits 1 on stderr. Zero rates are allowed.

Only calls without a recorded cost are eligible. The estimate is
`(Input*input + CacheRead*cache_read + CacheWrite*cache_write + Output*output) / 1e6`.
Reasoning tokens are assumed already included in Output and are not added again.
JSON adds `estimated_cost_usd` and `estimated_calls` only for populated groups;
recorded `cost_usd` and `cost_calls` remain separate. With `--prices`, stderr adds
`prices: models=K estimated_calls=E unpriced_calls=U`; U excludes recorded and
estimated calls. Without `--prices`, output is unchanged.

`testdata/prices.example.json` uses only synthetic fixture model IDs and
**obviously fake placeholder values, not real prices**. Supply your own real
prices; do not use the example for budgeting. From `telemetry/`, try it only
on the synthetic fixtures:

```sh
/tmp/telemetry report --claude-root testdata/claude --pi-root testdata/pi --prices testdata/prices.example.json
```

Synthetic fixture golden: `go test ./cmd/telemetry -run TestHumanGolden -update`.
The update flag writes only `telemetry/testdata/golden/report.txt`.
