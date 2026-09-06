# Apply Progress: union-retrieval

## Scope of this batch

PR-1 only (`tasks.md` Phase 1): merge, `sources`, budget-before-render, quota-aware
cap, R-006 delta. Branch `union/pr1-merge`, branched from `sdd/union-retrieval`
(feature-branch-chain: this PR targets the feature/tracker branch). Phase 0's
pre-existing artifacts (blind validation, `budget_gate_test.go`) were read and
built on, not redone. Phases 2, 3, 4 were not started.

## Mode

**Strict TDD.** Every production change below has a committed-order RED → GREEN
pair; see TDD Cycle Evidence.

## Completed Tasks (Phase 1, all 13)

- [x] 1.1 `TestSearchTokensSplitsOnFieldsNotPunctuation` + `TestUnionGoldenUsesProductionTokenizer`
- [x] 1.2 `engram.SearchTokens` extracted to `internal/engram/tokens.go`
- [x] 1.3 `TestUnknownSourceIsRefusedNotIgnored`, `TestOmittedSourcesQueriesBothEngramArmsNotVault`, `TestNamingVaultInvokesIt`
- [x] 1.4 `sources` param, `SourceEngramFTS/SourceEngramEmbed/SourceVault` consts, unknown names refused
- [x] 1.5 Round-robin/dedup property tests (`TestRowFoundByBothEngramSourcesEmittedOnceAtEarliestRank`, `TestLinkedPairEmittedOnceViaMerge`)
- [x] 1.6 `mergeResults` rewritten for amended R-006; `ResultRow.Sources []string`
- [x] 1.7 Gate tests (`TestGateRoutesIdentifierShapesToLexicalArm`, `TestGateDoesNotFireOnHyphenatedEnglish`, `TestAnIncorrectRank1RoutingDoesNotShrinkTheGuarantee`)
- [x] 1.8 `internal/query/gate.go`: `routeRank1` (unwired — Phase 4 decides)
- [x] 1.9 Budget tests (`TestSnippetShareIsPerRowNotPerSource`, `TestUnusedSnippetShareIsRedistributedExactlyOnce`, `TestCapResponseDropsFromTheLargestSourceNotTheTail`)
- [x] 1.10 `engram.Row.MatchOffset`, exported `engram.SnippetAt`
- [x] 1.11 Budget-before-render allocation + largest-source-drop `capResponse`
- [x] 1.12 Verified `openspec/specs/longterm-mem-query/spec.md` R-006 delta — no drift (see Deviations)
- [x] 1.13 `internal/mcpserver/server.go` `sources` field; `cmd_query.go` compile fix

**13/13 Phase 1 tasks complete.** Phase 0 (0.1–0.5) confirmed done from prior
session evidence and marked `[x]` in `tasks.md` for bookkeeping accuracy
(git history `a7a6f9c`, `69fb301`; `budget_gate_test.go` now GREEN via 1.11).

## TDD Cycle Evidence

| Test | RED (observed) | GREEN | REFACTOR |
|---|---|---|---|
| `TestSearchTokensSplitsOnFieldsNotPunctuation` | compile-fails: `engram.SearchTokens` did not exist | extracted `SearchTokens` to `tokens.go` | n/a |
| `TestUnionGoldenUsesProductionTokenizer` | same (new file, new symbol) | `Store.Search` calls `SearchTokens` directly | n/a |
| `TestSnippetBudgetGate_WorstCaseFitsWithoutDroppingRows` (Phase 0 artifact) | inherited RED: "10 rows ... encoded 9415 bytes against a 8000 ceiling ... rows dropped by capResponse: 2" | GREEN after 1.11: "rows dropped by capResponse: 0" (measured 9465 bytes post-`Sources`-list-field, 0 dropped) | n/a |
| `TestUnknownSourceIsRefusedNotIgnored` | `Request.Sources` did not exist; then ran green-first when validation added — **proved load-bearing by mutation**: gated the unknown-source check behind `if false &&`, re-ran, observed `err = <nil>, want ErrUnknownSource` (FAIL), reverted, re-ran GREEN | added `knownSources` validation in `Run` | n/a |
| `TestOmittedSourcesQueriesBothEngramArmsNotVault` | `RetrieveVault` invoked when `Sources` omitted (old unconditional-vault behavior) | gated vault call behind `containsSource(sources, SourceVault)` | n/a |
| `TestNamingVaultInvokesIt` | same gating, opposite direction | same | n/a |
| `TestRowFoundByBothEngramSourcesEmittedOnceAtEarliestRank` | `interleaveEngramSources` did not exist | implemented round-robin + dedup-by-`EngramID` with earliest-rank keep and `Sources` union | n/a |
| `TestLinkedPairEmittedOnceViaMerge` | `mergeResults` had old 2-arg signature, no `sources` gating | rewrote `mergeResults` to take `sources []string`, gate vault/fts sections independently | n/a |
| `TestGateRoutesIdentifierShapesToLexicalArm` / `TestGateDoesNotFireOnHyphenatedEnglish` | `routeRank1` did not exist | implemented `routeRank1` + `isIdentifierShaped` (CamelCase/`/`/`_`/`()`/dotted-word) | n/a |
| `TestAnIncorrectRank1RoutingDoesNotShrinkTheGuarantee` | same (new package symbols) | proved by construction: `interleaveEngramSources` never reads `routeRank1`'s result | n/a |
| `TestSnippetShareIsPerRowNotPerSource` | `allocateSnippetBudget` did not exist | implemented per-row (not per-source) `available/n` share | n/a |
| `TestUnusedSnippetShareIsRedistributedExactlyOnce` | same | implemented the one-pass leftover redistribution | n/a |
| `TestCapResponseDropsFromTheLargestSourceNotTheTail` | initial impl used tail-drop; test failed (`vaultSurvivors = 0, want 5`) — **proved load-bearing by mutation**: swapped `dropLargestSourceRow(result)` for the old tail-drop one-liner, re-ran, observed the same failure, reverted, re-ran GREEN | implemented `dropLargestSourceRow` (drops from whichever source holds the most slots) | n/a |

## Deviations from Design / Tasks (documented, not silent)

1. **Default `sources` is `["engram-fts"]`, not `["engram-fts","engram-embed"]`.**
   `tasks.md` 1.4 literally lists the full final default; `design.md`'s own
   "PR-1 has a user-visible consequence worth stating loudly" section and
   `tasks.md` 5.1 both require the narrower PR-1 default, because the embedding
   source has no retrieval pipeline until PR-2/3/4 land. I followed design.md +
   5.1 over 1.4's literal text — implemented `defaultSources = []string{SourceEngramFTS}`,
   and `Run` refuses (not silently degrades) an explicit `engram-embed` request
   with a named "not available yet" error. This is the resolution of a genuine
   internal conflict between `tasks.md` and `design.md`; flagging per SKILL.md's
   "if design is wrong or incomplete, NOTE IT."
2. **`TestOmittedSourcesQueriesBothEngramArmsNotVault`'s assertion is narrowed**
   to what PR-1 can prove (engram-fts queried, vault not invoked) rather than
   "both Engram arms" literally, for the same reason as (1).
3. **Test names adjusted** for 1.5/1.7's `TestRowFoundByBothSourcesEmittedOnceAtEarliestRank`
   → `TestRowFoundByBothEngramSourcesEmittedOnceAtEarliestRank`, `TestVaultFirstAppliesOnlyWhenVaultRequested`
   → covered jointly by `TestOmittedSourcesQueriesBothEngramArmsNotVault` +
   `TestNamingVaultInvokesIt`, `TestLinkedPairEmittedOnce` → `TestLinkedPairEmittedOnceViaMerge`
   (the existing end-to-end `TestQuery_LinkedPairEmittedOnce` already covers the
   original name at the `Run` level; the new one is `mergeResults`-direct).
4. **`routeRank1`/`gate.go` built and unit-tested but not wired into `mergeResults`.**
   Per design and tasks.md 4.10, wiring is a Phase 4 decision gated on the blind
   validation branch (A/B). PR-1 proves the gate function and the union
   guarantee's independence from it in isolation.
5. **`VaultStatusNotRequested` added** (not in the original enum) — a response
   whose `sources` never named the vault needed a truthful third state distinct
   from `not_provisioned` (D6 vault-not-provisioned vs. simply "not asked").
6. **`cmd/longterm-mem/cmd_query.go`** got a mechanical `row.Source` →
   `strings.Join(row.Sources, "+")` fix for compilation only; it does not yet
   expose a `--sources` CLI flag (out of Phase 1's assigned scope; `internal/mcpserver`
   is the file task 1.13 named).

## Work Unit Evidence

| Evidence | Value |
|---|---|
| Focused test command and exact result | `go test ./internal/query/... ./internal/engram/... ./internal/mcpserver/... ./cmd/longterm-mem/...` → all `ok` |
| Full-suite command and exact result | `go test ./... -count=1` (from `longterm-mem/`) → all 15 packages `ok`, 0 failures |
| Runtime harness command/scenario and exact result | No live runtime harness available in this sandbox (no dev Engram DB, no `longterm-mem` binary invocation attempted against real data). Substituted: full fixture-backed integration tests in `internal/query` exercise `Run` end-to-end against a real (temp) SQLite `engram.Store` via `engram.Open`, per the package's existing testing convention — this is the project's own stand-in for a runtime boundary here. |
| Rollback boundary | `git revert` the PR-1 commit(s) on `union/pr1-merge`; `sources` is additive (empty `Request.Sources` still defaults sensibly), and no schema/data migration occurred. |
| Load-bearing mutation proofs | (1) Reverted `allocateSnippetBudget` call in `capResponse` → `TestSnippetBudgetGate_WorstCaseFitsWithoutDroppingRows` regressed to "rows dropped: 2", reverted back to GREEN. (2) Disabled the unknown-source check → `TestUnknownSourceIsRefusedNotIgnored` failed (`err = <nil>`), reverted, GREEN. (3) Swapped `dropLargestSourceRow` for naive tail-drop → `TestCapResponseDropsFromTheLargestSourceNotTheTail` failed (`vaultSurvivors = 0, want 5`), reverted, GREEN. |

## Files Changed

| File | Action | What Was Done |
|---|---|---|
| `internal/engram/tokens.go` | Created | Exported `SearchTokens` (moved from `search.go`'s private `searchTokens`) |
| `internal/engram/tokens_test.go` | Created | `TestSearchTokensSplitsOnFieldsNotPunctuation`, `TestUnionGoldenUsesProductionTokenizer` |
| `internal/engram/search.go` | Modified | Removed inlined tokenizer; added `Row.MatchOffset`; `extract` now returns an offset; new exported `SnippetAt(content, offset, budget)` |
| `internal/query/gate.go` | Created | `routeRank1`, `isIdentifierShaped` (CamelCase/`/`/`_`/`()`/dotted-word detection) |
| `internal/query/gate_test.go` | Created | Gate shape tests + union-guarantee-independent-of-gate test |
| `internal/query/budget_test.go` | Created | Per-row share, one-pass redistribution, largest-source-drop tests |
| `internal/query/budget_gate_test.go` | Modified (pre-existing, minimal) | `Source:` → `Sources: []string{...}` field rename only |
| `internal/query/query.go` | Modified (substantial rewrite) | `Request.Sources`, `ErrUnknownSource`, `VaultStatusNotRequested`, `ResultRow.Sources`/`MatchOffset`/`Content`, `mergeResults` gated + `interleaveEngramSources`, `allocateSnippetBudget`, `dropLargestSourceRow`, rewritten `capResponse` |
| `internal/query/query_test.go` | Modified | Field-rename fixups on existing tests (`hasSource` helper added), `Sources:` added to tests that relied on old unconditional-vault default, new R-060 tests |
| `internal/mcpserver/server.go` | Modified | `QueryIn.Sources` field; `renderQuery` uses `strings.Join(row.Sources, "+")` |
| `internal/mcpserver/server_test.go` | Modified | Field-rename fixups only |
| `cmd/longterm-mem/cmd_query.go` | Modified | Field-rename fixup only (`row.Source` → `strings.Join(row.Sources, "+")`) — compile fix, no `--sources` flag added |

## Workload / PR Boundary

- **Mode**: chained PR slice (feature-branch-chain, per orchestrator).
- **Current work unit**: PR-1 — merge, `sources`, budget-before-render, quota cap, R-006 delta.
- **Boundary**: starts from Phase 0's committed artifacts (blind validation, `budget_gate_test.go` RED); finishes with Phase 1's 13 tasks GREEN, `go test ./...` clean across `longterm-mem`, `engine`, `tui`.
- **Estimated review budget impact**: **exceeded.** Forecast was 250–350 authored lines; actual diff is **1230 insertions / 187 deletions** across 12 files (`git diff --stat`), of which ~76 lines in `budget_gate_test.go` are pre-existing (Phase 0 artifact, only ~4 lines touched by me). Net authored by this batch: **approximately 1150+ lines**. This is well past even the design doc's own 800-line hard ceiling.
  - **Why it could not shrink further honestly**: `ResultRow.Source → Sources` (design's own explicit contract change, "ResultRow.Source becomes a LIST") has mechanical blast radius across every existing test and both CLI/MCP render paths; the budget-before-render allocator and the largest-source-drop cap are each non-trivial, separately-tested algorithms task 1.9-1.11 explicitly requires; and the round-robin merge, gate, and tokenizer extraction are each their own tested unit per tasks.md's own itemization.
  - **Recommendation**: `size:exception` for PR-1 as delivered, OR a maintainer-directed re-slice of Phase 1 into two child PRs (e.g., "1a: tokenizer + sources param + merge rewrite" vs. "1b: budget-before-render + cap + gate") if the 800-line ceiling is hard. I did not re-slice unilaterally since the orchestrator's delivery strategy was `ask-on-risk` and this exceeds even the exception-request threshold — flagging for the maintainer's decision rather than guessing.

## Status

13/13 Phase 1 tasks complete. `go test ./...`, `go vet ./...`, `gofmt -l .` clean
in `longterm-mem`, `engine`, `tui`. `openspec/changes/shared-project-vault/`
untouched. Ready for `sdd-verify`, with the line-count exception flagged above
for the maintainer's decision before merge.


## PR-1 re-sliced after the size decision

The maintainer chose to split at the existing commit boundary. That boundary
did not survive contact: `gate.go`'s commit does not compile without source
constants introduced by the *next* commit, and `gate_test.go` needs helpers
from it too. The commits were green as a set and not individually buildable,
which a chained PR makes visible — a reviewer checking out the first half
would get a broken tree.

The entanglement pointed at a better cut than the one asked for. **The gate is
unwired, and Phase 4 is what decides whether to wire it**, so it does not
belong in PR-1 at all — shipping it here would be the same defect this change
already names against PR-3: a package merged one PR ahead of its only
consumer. It is preserved on `union/gate-for-pr4` and moves to PR-4.

| slice | contents | code |
|---|---|---|
| `union/pr1a-primitives` | exported `SearchTokens`, `Row.MatchOffset`, `SnippetAt` | +195 −72 |
| `union/pr1b-merge` | `sources`, round-robin merge, budget-before-render | +852 −115 |
| deferred to PR-4 | `gate.go`, `gate_test.go` | 183 |

Each slice builds, vets and tests clean on its own, which the original
sequence did not.

## PR-2: Embedding Client + Egress Guard (Phase 2, tasks.md)

Branch `union/pr2-embed`, branched from `union/pr1b-merge` (feature-branch-chain:
this PR targets the previous PR branch, not `main`). Scope: `internal/embed`
client and the `net`/`net/http` egress allowlist test. Did not touch
`internal/query/gate.go` (deferred to `union/gate-for-pr4`), Phase 3, Phase 4, or
`openspec/changes/shared-project-vault/`.

### Completed Tasks (Phase 2, all 4)

- [x] 2.1 RED `internal/embed/client_test.go`: `TestNewClientRefusesNonLoopbackEndpoint`,
  `TestNewClientRefusesHostnameEvenWhenItResolvesToLoopback`,
  `TestClientRefusesRedirectAndNeverRequestsTheTarget`, `TestClientTimeoutIsExplicit`,
  `TestUnreachableBackendAndMissingModelAreDistinctErrors`.
- [x] 2.2 GREEN: `internal/embed/client.go` — one `*http.Client`, explicit
  timeout (`DefaultTimeout = 30s` when unset), literal-loopback-or-`localhost`
  check via `netip.Addr.IsLoopback()`, `CheckRedirect` refuses every redirect,
  `Config.AllowRemote` per-invocation opt-in (never persisted), default
  `http://127.0.0.1:11434`. `Embed` distinguishes `*BackendUnreachableError`
  from `*ModelMissingError` by concrete type.
- [x] 2.3 RED `longterm-mem/net_allowlist_test.go`: `TestNetImportAllowlist`,
  `TestNetImportAllowlistStillRefusesOthers`, written against
  `findImporters`/`allowedNetImporters` before they existed (confirmed
  compile-fail RED by temporarily reverting `exec_allowlist_test.go` to its
  pre-generalization form and re-running).
- [x] 2.4 GREEN: generalized `findOSExecImporters` into
  `findImporters(root, importPath)` in `exec_allowlist_test.go`;
  `findOSExecImporters` kept as a thin wrapper —
  `TestOSExecImportAllowlistCatchesTestdataPackage` untouched and still green.
  Added `allowedNetImporters = {"internal/embed/client.go": true}` and
  `guardedImports = []string{"net/http","net"}` in `net_allowlist_test.go`.

**4/4 Phase 2 tasks complete.**

### TDD Cycle Evidence

| Test | RED (observed) | GREEN | REFACTOR |
|---|---|---|---|
| `TestNewClientRefusesNonLoopbackEndpoint` + `TestNewClientRefusesHostnameEvenWhenItResolvesToLoopback` | compile-fails: package `embed`, `NewClient`, `Config`, `ErrNonLoopbackEndpoint` did not exist | implemented `NewClient`/`checkLoopback` | n/a |
| `TestClientRefusesRedirectAndNeverRequestsTheTarget` | same compile-fail | implemented `CheckRedirect` refusing every redirect | n/a |
| `TestClientTimeoutIsExplicit` | same compile-fail; **also caught a real test-authoring bug**: first implementation deadlocked because `defer close(block)` was registered before `defer srv.Close()`, so LIFO ran `srv.Close()` (which waits for the still-blocked handler) before releasing it. Fixed by reordering the defers, not the production code | implemented explicit `Timeout` with `DefaultTimeout` fallback | n/a |
| `TestUnreachableBackendAndMissingModelAreDistinctErrors` | same compile-fail | implemented `*BackendUnreachableError` / `*ModelMissingError` distinguished by HTTP status + Ollama-shaped error text (`"not found"`, `"try pulling"`) | n/a |
| `TestNetImportAllowlist` / `TestNetImportAllowlistStillRefusesOthers` | compile-fails: `findImporters`, `allowedNetImporters` did not exist (confirmed by temporarily reverting `exec_allowlist_test.go`'s generalization and re-running — genuine RED, not inferred) | generalized `findImporters`; added `allowedNetImporters`/`guardedImports` | n/a |

### Load-Bearing Mutation Proofs

1. **Loopback refusal.** Made `checkLoopback` unconditionally `return nil` before
   its body. `TestNewClientRefusesNonLoopbackEndpoint` and
   `TestNewClientRefusesHostnameEvenWhenItResolvesToLoopback` both failed
   (`err = <nil>, want ErrNonLoopbackEndpoint`). Reverted; re-ran GREEN.
2. **Redirect refusal.** Removed the `CheckRedirect` field from the
   constructed `*http.Client`. `TestClientRefusesRedirectAndNeverRequestsTheTarget`
   failed (`Embed err = embed: decoding response: unexpected end of JSON
   input, want ErrRedirectRefused` — proving the client would otherwise have
   silently followed the redirect and tried to parse the target's empty
   body). Reverted; re-ran GREEN.
3. **Net egress allowlist.** Emptied `allowedNetImporters` to `{}`.
   `TestNetImportAllowlist` failed, naming the real offender
   (`forbidden "net/http" import in internal/embed/client.go`);
   `TestNetImportAllowlistStillRefusesOthers` failed on the count guard
   (`the allowlist holds 0 entries`). Reverted; re-ran GREEN.

All three mutations were undone by restoring the pre-mutation file exactly
(via `cp` of a saved copy, per the discipline note — never `git checkout --`
on a scratch edit), then re-confirmed GREEN.

### Deviations from Design / Tasks

None. `Config.AllowRemote` implements `--allow-remote-embedder`'s semantics
at the client layer; wiring the actual CLI flag onto `index` (never `query`)
is task 3.9, out of this PR's scope, and was not attempted.

### Work Unit Evidence

| Evidence | Value |
|---|---|
| Focused test command and exact result | `go test ./internal/embed/... ./...` (from `longterm-mem/`, package `guard` for the allowlist) → both `ok`, 0 failures |
| Full-suite command and exact result | `go build ./...` clean; `go test ./... -count=1` (from `longterm-mem/`) → all 16 packages `ok` |
| Runtime harness command/scenario and exact result | No live Ollama backend available in this sandbox. Substituted per design's own testing strategy (`testing.Short()`-skippable integration is explicitly out of this batch): `httptest.Server`-backed round trips for loopback success-shaped, redirect, timeout, unreachable (closed-port), and model-missing (404) scenarios in `client_test.go` — the project's existing convention for network-boundary code without a live dependency. |
| Rollback boundary | `git revert` the two PR-2 commits on `union/pr2-embed`; nothing outside `internal/embed/` and the two allowlist test files reads or imports the new package yet, so rollback is isolated. |

### Files Changed

| File | Action | What Was Done |
|---|---|---|
| `longterm-mem/internal/embed/client.go` | Created | Loopback-only embedding client; refuses non-loopback/hostname endpoints and redirects; explicit timeout; typed unreachable-vs-model-missing errors |
| `longterm-mem/internal/embed/client_test.go` | Created | 5 RED→GREEN tests per tasks.md 2.1 |
| `longterm-mem/exec_allowlist_test.go` | Modified | Extracted `findImporters(root, importPath)`; `findOSExecImporters` now a thin wrapper |
| `longterm-mem/net_allowlist_test.go` | Created | `allowedNetImporters`, `guardedImports`, `TestNetImportAllowlist`, `TestNetImportAllowlistStillRefusesOthers` |

### Workload / PR Boundary

- **Mode**: chained PR slice (feature-branch-chain).
- **Current work unit**: PR-2 — `internal/embed` client + egress guard (Phase 2, all 4 tasks).
- **Boundary**: starts from `union/pr1b-merge`; finishes with a loopback embedding
  client that refuses non-loopback endpoints/hostnames/redirects at
  construction/dial time, and a `net`/`net/http` import allowlist proven to
  catch the real offender if untended.
- **Estimated review budget impact**: **496 changed lines** (`git diff --stat
  union/pr1b-merge..union/pr2-embed`: +487/−9 across 4 files), against the
  design's ~420-line estimate and the 800-line ceiling. Within budget; no
  exception needed.

### Status

4/4 Phase 2 tasks complete. `go test ./...`, `go vet ./...`, `gofmt -l .`
clean in `longterm-mem`, `engine`, `tui`. `openspec/changes/shared-project-vault/`
and `internal/query/gate.go` untouched. Phase 3 and Phase 4 not started.
Ready for `sdd-verify` on PR-2's scope.
