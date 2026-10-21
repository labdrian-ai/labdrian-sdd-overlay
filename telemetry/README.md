# telemetry

Read-only cost and efficiency report over agent session logs (Claude Code and Pi).
Standard library only; imports nothing else from this repository.
Design record: `docs/architecture/adr-telemetry-cost-baseline.md`. Ledger: `odd/tasks/telemetry-cost-baseline.md`.

```bash
cd telemetry
go run ./cmd/telemetry report                 # human tables, default roots
go run ./cmd/telemetry report --json          # the same data as stable JSON
go run ./cmd/telemetry report --prices prices.json --top 5
```

Usage details, exit codes and the price-table format: `cmd/telemetry/README.md`.

## What it measures

Per call: fresh input, cache read, cache write and output tokens, plus the recorded cost when the log has one.
Grouped by main thread vs subagent, model, agent type, project, source and day, with context-per-call
percentiles (nearest rank), the cache-read share and the context tokens read per output token.

## Guarantees

- Logs are opened read-only; output never contains conversation or tool content.
- No network access. No built-in prices: unknown cost prints `n/a`, an estimate is marked `~`.
- Unchanged input gives byte-identical output.

## Packages

| File | Role |
|---|---|
| `readers.go` | Streaming Claude Code and Pi readers, dedupe, normalized `Call` |
| `aggregate.go` | Deterministic totals, ratios, percentiles and groupings |
| `prices.go` | Strict optional price table |
| `cmd/telemetry` | The `report` command |
