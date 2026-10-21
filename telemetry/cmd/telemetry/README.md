# telemetry report

From `telemetry/`, build with `go build -o /tmp/telemetry ./cmd/telemetry`.
Run `telemetry report [--claude-root DIR] [--pi-root DIR] [--json] [--top N]`.

Defaults are `$HOME/.claude/projects`, `$HOME/.pi/agent/sessions`, and top 10.
Missing default roots are skipped with a stderr note; explicit missing roots
and no readable source exit 1. Invalid commands/flags exit 2 with usage.
Stats always go to stderr. Logs are read-only; no network is used.

Human output shows summary, role, model, agent type, project, source, and day.
Model/agent/project rows rank by context descending, then key ascending;
`--top` limits those rows and selects the latest N dates. Other rows show all.
Cost shows recorded dollars and covered calls, or `n/a`, never inferred prices.
`--json` emits the library's complete `Report.JSON()` without top truncation.
Only aggregate keys and counts are emitted, never conversation content.

Synthetic fixture golden: `go test ./cmd/telemetry -run TestHumanGolden -update`.
The update flag writes only `telemetry/testdata/golden/report.txt`.
