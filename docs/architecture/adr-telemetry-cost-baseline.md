# ADR: a read-only telemetry module measures what the agent workflow costs (own-system Phase 1)

- **Status:** Accepted, 2026-10-21.
- **Decided by:** the owner ("vamos a arrancamos por ahi"), after a hand analysis of his own session logs.
- **Ledger:** `odd/tasks/telemetry-cost-baseline.md`.

## Context

A hand analysis of Claude Code (1,849 files, 2026-07-30 to 2026-10-20) and Pi session logs showed that cost is
dominated by context re-read, not by generated text: about 24B context tokens against 80M output tokens
(roughly 300 to 1), 97.6% of the context read from cache. Subagents are 70% of the calls. The
`general-purpose` agent holds about 39% of all context tokens, against 9% for `sdd-apply`.

The own-system relaunch (cheaper, faster, better software built to measure) needs those numbers to be
reproducible, not a one-off script. Nothing in the repository measured them.

## Decision

1. **A separate Go module, `telemetry/`**, standard library only, with its own CI job, like `tools/*`. It imports
   neither `engine/` nor `longterm-mem/`. It is a standalone binary (`telemetry report`) for now; making it a
   `labdrian` verb is deferred (decision D2 of the ledger).
2. **Read-only over the logs.** Files are opened O_RDONLY; a test proves they are not modified. Nothing is
   written outside the process.
3. **Aggregates only.** Output holds counts, tokens, model, project, agent type and day. It never holds prompt,
   response or tool content, and the readers decode only the fields they need.
4. **Recorded cost is never mixed with estimated cost.** Pi logs record a cost and it is reported as recorded.
   Claude Code logs record none: the report says `n/a`. An estimate needs a price table the user supplies
   (`--prices`), matches model ids exactly, never re-prices a recorded cost, and is flagged with `~`. The
   module ships no prices.
5. **Deterministic output.** The same logs give byte-identical output: calls are read in a fixed order and
   floating-point sums use a canonical order. Output of a run without `--prices` is byte-identical to the
   output before the price feature (checked against a copy of the previous binary on the real corpus).
6. **Attribution by agent role** uses the `agentType` the subagent `meta.json` records. Mapping roles to workflow
   phases, and "cost per delivered change" (Claude records carry `gitBranch`), are deferred until the owner
   defines them (decisions D1 and the T5 mapping).

## Consequences

- The numbers above can be regenerated at any time and compared before and after a change to the workflow.
- Claude Code costs are only as good as the price table the user maintains.
- The readers do not follow symlinked directories inside a root; the default roots are real directories.
- Logs are live files: two runs a few seconds apart can differ by the calls written in between. Determinism is
  guaranteed for unchanged input.
- A single call with 3.08M context tokens appears in the distribution maximum and has not been investigated.
