# Apply Progress: union-retrieval

## Phase 0 — done (pre-existing, verified by this agent, unchanged)
## Phase 1 (PR-1) — done (pre-existing, unchanged)
## Phase 2 (PR-2) — done (pre-existing, unchanged)

## Phase 3 (PR-3, branch `union/pr3-vecindex` off `union/pr2-embed`) — PARTIAL, stopped on budget overrun

### TDD Cycle Evidence

| Task | RED | GREEN | REFACTOR |
|---|---|---|---|
| 3.1 | `go test ./internal/vecindex/...` failed to build (`undefined: Fingerprint`, `undefined: Index`, `undefined: Manifest`) before `fingerprint.go`/`index.go` existed | `TestFingerprintIgnoresContentBeyondInputLimit`, `TestFingerprintChangesWithContractFields`, `TestSaveThenLoadRoundTrips`, `TestLoadDetectsManifestRevisionMismatch`, `TestLoadNoIndexReportsErrNoIndex` all PASS | n/a — first implementation |
| 3.2 | (paired with 3.1) | `internal/vecindex/{fingerprint,index}.go` created | n/a |
| 3.3 | `go test ./internal/vecindex/...` failed to build (`undefined: Row`, `undefined: Build`) before `build.go` existed | `TestBuildFromScratchEmbedsEveryLiveRow`, `TestBuildReembedsOnlyMissingOrChanged`, `TestBuildRemovesEntriesForRowsNoLongerLive`, `TestBuildRefusesOnCorruptedExistingIndex` all PASS | n/a |
| 3.4 | `go test ./cmd/longterm-mem/... -run TestCmdIndexEmbeddings` failed: `flag provided but not defined: -embeddings` | `TestCmdIndexEmbeddings_BuildsIndexOnDisk`, `TestCmdIndexEmbeddings_NonLoopbackRefusedWithoutFlag` PASS | n/a |
| 3.9 | (shipped inside 3.4's RED/GREEN cycle — the `--allow-remote-embedder` flag was written together with `--embeddings`) | `TestCmdIndexEmbeddings_NonLoopbackRefusedWithoutFlag` proves the flag exists on `index` and gates the refusal | n/a |
| 3.5–3.8 | **NOT STARTED** | | |

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
