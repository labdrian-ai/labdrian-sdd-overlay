# Apply Progress: union-retrieval

## Phase 0 — done (pre-existing, verified by this agent, unchanged)
## Phase 1 (PR-1) — done (pre-existing, unchanged)
## Phase 2 (PR-2) — done (pre-existing, unchanged)

## Phase 3 (PR-3, branch `union/pr3-vecindex` off `union/pr2-embed`) — 3.1-3.4/3.9 done

### TDD Cycle Evidence

| Task | RED | GREEN | REFACTOR |
|---|---|---|---|
| 3.1 | `go test ./internal/vecindex/...` failed to build (`undefined: Fingerprint`, `undefined: Index`, `undefined: Manifest`) before `fingerprint.go`/`index.go` existed | `TestFingerprintIgnoresContentBeyondInputLimit`, `TestFingerprintChangesWithContractFields`, `TestSaveThenLoadRoundTrips`, `TestLoadDetectsManifestRevisionMismatch`, `TestLoadNoIndexReportsErrNoIndex` all PASS | n/a — first implementation |
| 3.2 | (paired with 3.1) | `internal/vecindex/{fingerprint,index}.go` created | n/a |
| 3.3 | `go test ./internal/vecindex/...` failed to build (`undefined: Row`, `undefined: Build`) before `build.go` existed | `TestBuildFromScratchEmbedsEveryLiveRow`, `TestBuildReembedsOnlyMissingOrChanged`, `TestBuildRemovesEntriesForRowsNoLongerLive`, `TestBuildRefusesOnCorruptedExistingIndex` all PASS | n/a |
| 3.4 | `go test ./cmd/longterm-mem/... -run TestCmdIndexEmbeddings` failed: `flag provided but not defined: -embeddings` | `TestCmdIndexEmbeddings_BuildsIndexOnDisk`, `TestCmdIndexEmbeddings_NonLoopbackRefusedWithoutFlag` PASS | n/a |
| 3.9 | (shipped inside 3.4's RED/GREEN cycle — the `--allow-remote-embedder` flag was written together with `--embeddings`) | `TestCmdIndexEmbeddings_NonLoopbackRefusedWithoutFlag` proves the flag exists on `index` and gates the refusal | n/a |

## Phase 3b (PR-3b, branch `union/pr3b-ops-wiring` off `union/pr3-vecindex`) — 3.5-3.8 done

Split out per the maintainer's decision recorded above: PR-3's authored diff
had already passed the 800-line ceiling before 3.5–3.8 were started, so this
batch landed as its own branch/PR rather than growing PR-3 further.

### TDD Cycle Evidence

| Task | RED | GREEN | REFACTOR |
|---|---|---|---|
| 3.5 | `go test ./internal/ops/...` failed to build: `unknown field StateDir/LiveObservationIDs/EmbeddingBackendCheck in struct literal of type DoctorDeps`, `undefined: CheckEmbeddingIndexPresent/CheckEmbeddingIndexFresh/CheckEmbeddingBackendReachable` (captured via `go vet`) | `TestDoctor/Missing_embedding_index_is_named`, `TestDoctor/A_stale_index_is_named_with_its_coverage_gap`, `TestDoctor/An_unreachable_backend_is_distinguished_from_a_missing_model`, `TestDoctor/A_reachable_backend_missing_its_model_is_named_distinctly_from_unreachable` all PASS | n/a — first implementation |
| 3.6 | (paired with 3.5) | three new `Check` rows added to `internal/ops/doctor.go`'s `Doctor()` assembly (now 8 checks, was 5); `DoctorDeps` gained `StateDir`, `LiveObservationIDs`, `EmbeddingBackendCheck` seams | n/a |
| 3.7 | `go vet ./internal/ops/...` failed: `unknown field StateDir in struct literal of type StatusDeps` | `TestStatus_EmbeddingIndexBuiltAt/A_built_index_reports_its_build_time`, `TestStatus_EmbeddingIndexBuiltAt/Never-built_reports_never,_not_a_fabricated_timestamp` PASS | n/a |
| 3.8 | (paired with 3.7) | `StatusReport.EmbeddingIndexBuiltAt` added, sourced via `vecindex.Load` in `internal/ops/status.go`; `StatusDeps` gained `StateDir` | n/a |

### Work Unit Evidence

| Evidence | Value |
|---|---|
| Focused test command and exact result | `go test ./internal/ops/... ./cmd/longterm-mem/... -v -count=1` — all PASS (TestDoctor's 12 subtests including 4 new, TestStatus's 3 cases plus new TestStatus_EmbeddingIndexBuiltAt's 2, TestCmdDoctor_DocumentedCheckCountMatchesOpsDoctor, TestCmdStatus_ReportsEveryFieldAndExitsZeroWhenUnhealthy) |
| Runtime harness command/scenario and exact result | `longterm-mem doctor --project P` and `longterm-mem status --project P` exercised end to end via `TestCmdDoctor_PrintsEveryCheckOpsDoctorReturns`/`TestCmdDoctor_ReportsEveryCheckDespiteOneFailing` and `TestCmdStatus_ReportsEveryFieldAndExitsZeroWhenUnhealthy`: real `run([]string{...})` invocations against a fresh `HOME`/vault, with no live Ollama backend available (`embedding-backend-reachable`'s production probe hits real loopback `127.0.0.1:11434` and reports FAIL — connection refused — in this environment, exercising the actual unreachable path, not a fake) |
| Rollback boundary | `git revert 512e9ec 4655b4e` on `union/pr3b-ops-wiring`; both commits touch only `internal/ops/{doctor,status}.go` + their tests and `cmd/longterm-mem/cmd_{doctor,status}.go` + their tests — nothing else in the module reads the new `DoctorDeps`/`StatusDeps` fields or `StatusReport.EmbeddingIndexBuiltAt` |

### Load-bearing mutation proofs (go test -count=1, never cached)

- `checkEmbeddingIndexFresh` forced to always return `CheckPassed`: turned `TestDoctor/A_stale_index_is_named_with_its_coverage_gap` red (`want FAILed`, got PASS). Reverted from a saved copy (never `git checkout -- <file>`), confirmed green again.
- `checkEmbeddingBackendReachable` forced to always report the "unreachable" wording regardless of `errors.As` result: turned `TestDoctor/A_reachable_backend_missing_its_model_is_named_distinctly_from_unreachable` red (detail reused "unreachable" wording for a model-missing failure). Reverted, confirmed green again.
- `Status`'s `EmbeddingIndexBuiltAt` assignment forced to the literal `"never"` unconditionally: turned `TestStatus_EmbeddingIndexBuiltAt/A_built_index_reports_its_build_time` red. Reverted, confirmed green again.

### Full-module verification (both commits, standing alone and cumulatively)

- `gofmt -l .` — clean, in `longterm-mem`, after each commit
- `go vet ./...` — clean, in `longterm-mem`, after each commit
- `go test ./...` — all packages PASS, in `longterm-mem`, after each commit (`count=1`)
- `gofmt -l .` / `go vet ./...` / `go test ./... -count=1` — clean/PASS, in `engine` and `tui` (untouched, independently re-verified)
- `go test . -run TestNetImportAllowlist` — PASS: `internal/ops/doctor.go` imports `internal/embed` (for its two typed error structs, `*embed.BackendUnreachableError`/`*embed.ModelMissingError`) but never `net`/`net/http` directly, so the allowlist still holds at exactly one entry (`internal/embed/client.go`)

### Deviations from Design

None. `embedding-index-present`/`embedding-index-fresh`/`embedding-backend-reachable` and `status`'s `embedding_index_built_at` match R-064/R-065's scenarios exactly. One design choice made where design left the mechanism open: `EmbeddingBackendCheck` is a `DoctorDeps` function seam (production wires `embed.NewClient(...).Embed(ctx, probe)`) rather than `doctor.go` importing `internal/embed`'s network path directly — required by `net_allowlist_test.go`'s `allowedNetImporters` staying at exactly one entry. Freshness compares live-ID *sets* against the manifest's entries (not a bare length/count diff), so a corpus that both grew and shrank by the same amount is still caught — this is a deliberate strengthening consistent with the launch prompt's own "a check that cannot fail is worse than no check" framing, not a spec deviation.

### Issues Found

None.

### Remaining Tasks (Phase 3/3b)

None — 3.1–3.9 are all complete. Phase 4 (PR-4) is untouched and out of scope, per the launch instructions ("Do not start Phase 4"). `internal/query/gate.go` and `openspec/changes/shared-project-vault/` are untouched, per the launch instructions.

### Commits (branch `union/pr3b-ops-wiring`, off `union/pr3-vecindex`)

1. `4655b4e` — `test(ops): RED+GREEN — doctor names a missing/stale embedding index and an unreachable backend distinctly from a missing model (R-064)` (5 files, +311/-12)
2. `512e9ec` — `test(ops): RED+GREEN — status reports embedding_index_built_at or never (R-065)` (4 files, +107/-14)

Combined authored diff: 418 additions / 26 deletions = 444 lines, over the
~150–250 line forecast (driven by four pre-existing `DoctorDeps`/`StatusDeps`
struct-literal call sites across `doctor_test.go`, `doctor_reconcile_test.go`,
and `cmd_doctor_test.go` all needing the new required seam fields, plus a
full RED+GREEN table for 4 new `TestDoctor` subtests and 2 new
`TestStatus_EmbeddingIndexBuiltAt` cases) but well under the 800-line
ceiling. Not pushed; no PR opened, per the launch instructions.

### Work Unit Evidence

| Evidence | Value |
|---|---|
| Focused test command and exact result | `go test ./internal/vecindex/... ./cmd/longterm-mem/... -v` — all PASS (9 vecindex tests, 2 new cmd tests, all pre-existing cmd tests unaffected) |
| Runtime harness command/scenario and exact result | `longterm-mem index --project P --embeddings --embed-endpoint <fake ollama>` exercised end-to-end via `TestCmdIndexEmbeddings_BuildsIndexOnDisk`: reads live Engram rows through `engram.Store.ListObservations`, embeds through a `httptest`-backed loopback server (no live Ollama available in this environment, per PR-2's own precedent), and writes a loadable `manifest.json` + `vectors.blob` under `<state-dir>/index/<project>/` |
| Rollback boundary | `git revert f6cc93b e2aea89` (or `git checkout union/pr2-embed -- longterm-mem/internal/vecindex longterm-mem/cmd/longterm-mem/cmd_index.go longterm-mem/cmd/longterm-mem/cmd_index_embeddings.go longterm-mem/cmd/longterm-mem/cmd_index_embeddings_test.go`); deleting `internal/vecindex/` and reverting `cmd_index.go`'s new flags is safe — nothing else in the module reads `vecindex` yet (Phase 4 is the first consumer) |

### Full-module verification (this apply's own gate, not just the touched packages)

- `gofmt -l .` — clean, in `longterm-mem`
- `go vet ./...` — clean, in `longterm-mem`
- `go test ./...` — all packages PASS, in `longterm-mem`
- `gofmt -l .` / `go vet ./...` — clean, in `engine` and `tui` (untouched by this PR; verified independently per the phase contract's instruction, not skipped)
- `go test . -run TestNetImportAllowlist` — PASS: `internal/vecindex/build.go` still refused (it never imports `net`/`net/http`; `Build` calls into an `Embedder` interface, never `internal/embed` directly)

### Deviations from Design

None. `EmbedInput` (`title‖NUL‖content[:limit]`) matches design's measured shape exactly. `Fingerprint` hashes `model‖dim‖input_limit‖EmbedInput(...)`, matching design's "the fingerprint puts the embedding contract inside the hash." The manifest is self-digesting (`Revision` field, recomputed and checked on every `Load`) per R-066's corruption scenario. `Store.ObservationByID`/`LiveObservationsByID` are untouched — Phase 3 never reads observations by id; it lists live rows for a whole project via the already-R-020-scoped `ListObservations`, and the soft-delete-safe fetch design calls for (`LiveObservationsByID`) is explicitly Phase 4's own task (4.2), not Phase 3's.

### Why this stopped: budget overrun (maintainer decision needed)

Authored diff for PR-3 so far (`git diff --numstat union/pr2-embed..union/pr3-vecindex`) is **988 lines** (984 additions / 4 deletions across `longterm-mem/internal/vecindex/{fingerprint,index,build}.go` + their tests, `longterm-mem/cmd/longterm-mem/cmd_index.go` + `cmd_index_embeddings.go` + its test, and this file). That is already **past the 800-line review budget** named in the launch instructions and far past the **~450-line forecast** in `tasks.md`'s own Review Workload Forecast table — before tasks 3.5–3.8 (three new `ops.Check` tests + `doctor.go` wiring, plus a `status.go` field and its own RED/GREEN test) are even started, which would add an estimated further 150–250 lines.

Per the apply contract's own instruction ("If it grows past that, stop and report — that call is the maintainer's"), this agent stopped rather than continuing into 3.5–3.8 and pushing the total further past budget. Two options for the maintainer:

1. **Split**: land 3.1–3.4/3.9 as PR-3 (this branch, `union/pr3-vecindex`, already reviewable end-to-end: an index builds, rebuilds incrementally, and is wired into the CLI), and open a `union/pr3b-ops-wiring` branch off this one for 3.5–3.8 (doctor + status wiring) as its own PR.
2. **Exception**: accept `size:exception` for PR-3 as a single ~1150–1250 line unit and let a follow-up apply batch finish 3.5–3.8 on this same branch before opening the PR.

Either way, nothing here needs to be undone: 3.1–3.4/3.9 are a complete, independently revertable, fully green unit on their own (index builds, incrementally rebuilds, and CLI-wires without touching anything Phase 4 or later PRs depend on).

### Remaining Tasks (Phase 3)

- [ ] 3.5 RED: three `ops.Check` tests — `embedding-index-present`, `embedding-index-fresh` (names N), `embedding-backend-reachable` distinct from missing-model (R-064).
- [ ] 3.6 GREEN: add the three checks to `internal/ops/doctor.go`.
- [ ] 3.7 RED: `status` reports `embedding_index_built_at` or literal `never` (R-065).
- [ ] 3.8 GREEN: add the field to `internal/ops/status.go`.

Phase 4 (PR-4) is untouched and out of scope, per the launch instructions ("Do not start Phase 4"). `internal/query/gate.go` and `openspec/changes/shared-project-vault/` are untouched, per the launch instructions.
