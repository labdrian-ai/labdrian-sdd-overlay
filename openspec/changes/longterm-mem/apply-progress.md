# Apply Progress: longterm-mem

Branch: `feat/lm/longterm-mem-scaffold-vault-2a-registry` (base: PR1 commit
`abf69a2`, slices 1a/1b history on this branch)
Last updated: 2026-08-30

---

## Slice 1 — longterm-mem-scaffold-module (R-001, R-021, R-002, R-020) (COMPLETE)

All Slice 1 tasks (Bootstrap 1.0.1–1.0.4, R-001 1.1–1.3, R-021 1.4–1.5, R-002
1.6–1.8, R-020 1.9–1.10, verification 1.11) implemented and ticked in
`tasks.md`.

### Bootstrap

- [x] 1.0.1 `longterm-mem/go.mod` (`module github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem`, `go 1.26.1`) + `go get` for `modernc.org/sqlite@v1.57.0`, `github.com/modelcontextprotocol/go-sdk@v1.7.0`, `github.com/pelletier/go-toml/v2@v2.4.3` (pinned upfront; only `modernc.org/sqlite` is imported by production code so far — the other two are locked in `go.mod`/`go.sum` for later slices).
- [x] 1.0.2 `.github/workflows/ci.yml` job `test-longterm-mem` (checkout, `setup-go@v5` on `longterm-mem/go.mod`, gofmt, vet, staticcheck, `go test ./... -cover`), cloned from `test-tui`.
- [x] 1.0.3 `openspec/config.yaml`: new `longterm-mem` entry under `testing.layers`; `apply.tdd.test_command`/`verify.test_command`/`verify.build_command` now run `cd longterm-mem && go test ./...` first (with an explicit `cd ..` back to repo root before the existing engine/tui/tools chain — verified to execute correctly end-to-end).
- [x] 1.0.4 `longterm-mem/README.md` refreshed: per-project component description, module-boundary note (own `go.mod`, third-party deps allowed, outside `engine/`'s zero-dep boundary), fixed binary install path; full CLI/MCP surface documentation deferred to 10b.8 per CHK-05.

### R-001 — Standalone Module Outside engine/

- [x] 1.1–1.2 `longterm-mem/cmd/longterm-mem/main.go` + `main_test.go::TestMain_BuildsIndependentModule` — RED confirmed (`no packages to build`) before `main.go` existed; GREEN after a minimal subcommand dispatcher (usage text, exit 2 on unknown subcommand). The build test writes its output to `t.TempDir()` via `-o` so it never leaves a compiled binary inside the module's own source tree.
- [x] 1.3 Re-ran `engine/skills/zero_fetch_test.go::TestZeroFetchImportAllowlist` unmodified — still passes; `engine/go.mod` still carries zero third-party requirements (`go 1.21`, no `require` block) despite `longterm-mem/go.mod` adding sqlite/mcp-sdk/go-toml.

### R-021 — No CLI Shelling to Engram (guard)

- [x] 1.4–1.5 `longterm-mem/exec_allowlist_test.go::TestOSExecImportAllowlist` — statically walks every non-test `.go` file under `longterm-mem/` (skipping `testdata/`) and fails if any file other than `internal/vault/runner.go` imports `os/exec`. Passes vacuously today (only `main.go` exists, no `os/exec` import). Triangulated by temporarily adding a throwaway file that imports `os/exec` outside the allowed path — confirmed the guard fails with a named violation — then removed it and reconfirmed green.

### R-002 — Read-Only Engram Connection

- [x] 1.6–1.8 `longterm-mem/internal/engram/store.go` — `Open(dbPath string) (*Store, error)` opens `modernc.org/sqlite` with DSN `file:<path>?mode=ro&_txlock=deferred&_pragma=query_only(1)&_pragma=busy_timeout(2000)` (D1; verified against the modernc.org/sqlite v1.57.0 driver source — `mode=ro` is SQLite's own URI open-mode parameter, `_pragma`/`_txlock` are the Go driver's DSN shorthand keys). Default path resolves to `$HOME/.engram/engram.db` when `dbPath` is empty. `TestOpen_DefaultIsReadOnly` and `TestOpen_OverridePathStaysReadOnly` both build their fixture under `t.TempDir()` (the default-path case overrides `$HOME` via `t.Setenv`, never touching the real `~/.engram/engram.db`) and assert both a successful read and a failing `INSERT`/`UPDATE`. REFACTOR extracted `readOnlyDSN(path string) string`.
- [x] `internal/engram/testdata/schema.sql` — the live `observations` table DDL (dumped read-only from `~/.engram/engram.db`, matching Engram design-notes #3133/#3129) minus the unused `sessions` foreign key.

### R-020 — Mid-Term Query Scoping

- [x] 1.9–1.10 `longterm-mem/internal/engram/store.go` — `ListObservations(project string) ([]Observation, error)` (`WHERE project = ? AND deleted_at IS NULL`). `TestListObservations_ScopesProjectAndExcludesSoftDeleted` fixtures three rows (active/P, soft-deleted/P, active/other-project) and asserts exactly the active/P row comes back.

### Verification

- [x] 1.11 `cd longterm-mem && go test ./...` — all packages pass. `bash -n bin/labdrian-overlay` — clean.

`cd longterm-mem && gofmt -l .` — clean. `go vet ./...` — clean. `go test ./... -cover` — `internal/engram` 75.9%, `cmd/longterm-mem` 0.0% (scaffold dispatcher, no subcommands yet), root `guard` package has no statements to cover.
`cd engine && go test ./...` — all packages pass (zero-dep gate intact, `engine/go.mod` unchanged).

### Files created

- `longterm-mem/go.mod`, `longterm-mem/go.sum`
- `longterm-mem/cmd/longterm-mem/main.go`, `main_test.go`
- `longterm-mem/exec_allowlist_test.go`
- `longterm-mem/internal/engram/store.go`, `store_test.go`, `testdata/schema.sql`

### Files modified

- `.github/workflows/ci.yml` (new `test-longterm-mem` job)
- `openspec/config.yaml` (new `longterm-mem` testing layer + chained test commands)
- `longterm-mem/README.md` (module-boundary + install-path refresh)
- `openspec/changes/longterm-mem/tasks.md` (Slice 1 items ticked)

### Authored line budget

Authored changed lines (git diff --numstat additions+deletions for tracked
files, plain line counts for untracked files), **excluding `go.sum`**:

| File | Lines |
|---|---|
| `.github/workflows/ci.yml` (diff) | 29 |
| `longterm-mem/README.md` (diff) | 23 |
| `openspec/config.yaml` (diff) | 10 |
| `openspec/changes/longterm-mem/tasks.md` (diff, checkbox toggles) | 30 |
| `longterm-mem/go.mod` (new) | 19 |
| `longterm-mem/exec_allowlist_test.go` (new) | 67 |
| `longterm-mem/cmd/longterm-mem/main.go` (new) | 33 |
| `longterm-mem/cmd/longterm-mem/main_test.go` (new) | 26 |
| `longterm-mem/internal/engram/store.go` (new) | 101 |
| `longterm-mem/internal/engram/store_test.go` (new) | 141 |
| `longterm-mem/internal/engram/testdata/schema.sql` (new) | 42 |
| **Total (go.sum excluded only)** | **521** |
| Total excluding `go.mod` too (cached session-preflight convention) | 502 |
| Go+SQL implementation/test code only (excl. tasks.md/README/CI/config/go.mod) | 410 |

This is over the 400-line budget under every counting convention tried
(exceeds by 10–121 lines depending on what is excluded). The design's own
forecast for this slice was 330–370 authored lines and did not name a split
point for Slice 1 (design-notes #3133 Slice Map: only slices 2, 3, 8, 10, 11,
12 have named split points). No scope was added beyond the unchecked Slice 1
tasks; the overage comes from: (a) `store_test.go`'s three scenarios needing
two shared fixture helpers plus real read/write assertions per strict-TDD's
no-trivial-assertion rules, (b) the live-schema fixture (`schema.sql`, 42
lines) matching the design's own instruction to mirror #3129 verbatim, and
(c) tasks.md checkbox-toggle diff noise (30 lines for 15 ticked items — 1
add+1 del per line). Flagged as a risk for the orchestrator; no further split
was invented since none was authorized for this slice.

---

## Slice 2a — longterm-mem-scaffold-vault-2a-registry (R-003, R-022, R-023) (COMPLETE)

All Slice 2a tasks (2a.1–2a.7) implemented and ticked in `tasks.md`. This
slice is the first named split point of design-notes #3133's Slice 2
(`2a registry, 2b runner+retrieve`); slice 2b (runner/retrieve, R-004/R-024)
is intentionally out of scope for this batch.

### R-003, R-022, R-023 — Vault Registry Resolution

- [x] 2a.1–2a.5 RED `longterm-mem/internal/vaultreg/registry_test.go` — five
  scenario tests written first against a package that did not yet exist
  (`go test ./internal/vaultreg/...` failed with `undefined: Registry` /
  `undefined: Resolve` / etc. before `registry.go` was created — confirmed
  RED by execution, not by inspection):
  - `TestResolve_ConfiguredOverrideWins` (R-003): a fixture row for
    `some-other-project` is returned verbatim.
  - `TestResolve_DefaultSeedEntryForOverlayProject` (R-022): with no
    `vaults.json` on disk at all, resolving `labdrian-sdd-overlay` succeeds
    via the pre-seeded row Resolve itself materializes; the test then
    reloads the file and asserts the row is an ordinary
    `{"path":"~/labdrian-brain","seeded":true}` entry, not a value
    reproduced from a code constant on every call.
  - `TestResolve_UnconfiguredNonDefaultProjectRejected` (R-023): project
    `some-new-project` with no `vaults.json` on disk; Resolve seeds only
    the `labdrian-sdd-overlay` default row, so `some-new-project` still has
    no row and gets `ErrVaultNotConfigured`, never a guessed path.
  - `TestPrecedence_FlagEnvFile` (D5): three subtests over one fixture row
    prove flag > env > row precedence with three distinct expected paths.
  - `TestSeed_OnlyWhenFileAbsent`: a `vaults.json` that already exists (with
    an unrelated row, no `labdrian-sdd-overlay` row) is never auto-seeded —
    Resolve still returns `ErrVaultNotConfigured` and a reload confirms no
    row was added, proving "delete the seed row" means "not configured".
- [x] 2a.6 GREEN `longterm-mem/internal/vaultreg/registry.go` —
  `Registry{Schema, Vaults}` / `VaultEntry{Path, Seeded}` JSON model;
  `Load(path)`; `Seed(path)` (writes the seed row via `os.Stat` +
  `os.IsNotExist` — a no-op for any existing file, regardless of content);
  `Resolve(vaultsPath, project, flagVault string) (string, error)`
  implementing flag > `LONGTERM_MEM_VAULT` env > registry-row precedence,
  `~`/`~/...` expansion via `os.UserHomeDir`, absolute-path validation
  (`filepath.IsAbs` after expansion), and `ErrVaultNotConfigured` as a
  wrapped sentinel (`errors.Is` works through the added detail). All five
  RED tests pass unmodified against this implementation
  (`go test ./internal/vaultreg/... -run 'TestResolve|TestSeed|TestPrecedence' -v`
  — 5 tests + 3 subtests, all PASS).
- [x] 2a.7 REFACTOR — the tmp+fsync+rename JSON write (`os.CreateTemp` in
  the target directory, `Write`, `Sync`, `Close`, `os.Rename`) is factored
  out of `Seed` into `writeJSONAtomic(path string, v any) error`, mirroring
  the `bin/labdrian-overlay:590-628` `state_write_target` tmp+mv precedent
  (adding an explicit `fsync` before rename, matching the design's stated
  D5/D6 convention). Full suite re-run green after the refactor.

### Design notes followed (Engram #3133 D5)

`Resolve`'s only literal-default write path is `Seed`; the lookup path is
uniform for every project (including `labdrian-sdd-overlay`) — there is no
`if project == "labdrian-sdd-overlay"` special case anywhere in `Resolve`,
which is what makes deleting the seeded row equivalent to any other missing
row (R-022's "not a code constant" requirement, verified by
`TestSeed_OnlyWhenFileAbsent`).

### Verification

`cd longterm-mem && go test ./... -run 'TestResolve|TestSeed|TestPrecedence'`
— 3 packages with no matching tests report `[no tests to run]` (expected —
the pattern only matches vaultreg); `internal/vaultreg` 5 tests + 3 subtests
PASS.

`cd longterm-mem && gofmt -l .` — clean. `go vet ./...` — clean.
`go test ./... -cover -count=1` — `internal/vaultreg` 67.7% (uncovered
lines are defensive I/O-failure branches in `Load`/`Seed`/`writeJSONAtomic`
and the bare-`~`-only branch of `expandVaultPath`, none of which are named
scenarios in tasks.md for this slice); `internal/engram` 84.2%;
`cmd/longterm-mem` 0.0% (still no subcommands wired — unchanged from Slice
1); root `guard` package has no statements to cover.

`cd engine && go test ./...` — all 10 packages pass (zero-dep gate
unaffected; this slice touches nothing under `engine/`).
`bash -n bin/labdrian-overlay` — clean (this slice touches nothing under
`bin/`).

### Files created

- `longterm-mem/internal/vaultreg/registry.go`
- `longterm-mem/internal/vaultreg/registry_test.go`

### Files modified

- `openspec/changes/longterm-mem/tasks.md` (Slice 2a items 2a.1–2a.7 ticked)

### Authored line budget

Authored changed lines (plain line counts for the two new files, `git diff
--numstat` for the tracked `tasks.md` checkbox toggle), excluding `go.sum`
(no `go.sum` change — no new dependency was needed):

| File | Lines |
|---|---|
| `longterm-mem/internal/vaultreg/registry.go` (new) | 179 |
| `longterm-mem/internal/vaultreg/registry_test.go` (new) | 188 |
| `openspec/changes/longterm-mem/tasks.md` (diff, checkbox toggles, 7 items) | 14 |
| **Total** | **381** |

Within the 400-line budget (forecast for this slice: 150–180; the design's
own combined Slice 2 forecast was 380–430 with this file as the named first
split point). The overage above the 150–180 per-slice forecast, same as
Slice 1, comes from strict-TDD's no-trivial-assertion requirement: each of
the five named scenarios needed its own fixture setup (a fresh `vaults.json`
path per test, explicit `HOME`/`LONGTERM_MEM_VAULT` isolation via
`t.Setenv`) and a specific, distinct expected-path assertion rather than a
shared trivial check — still comfortably inside budget, so no split was
needed for this slice.
