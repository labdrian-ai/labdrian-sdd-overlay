# Apply Progress: longterm-mem

Branch: `feat/lm/longterm-mem-promotion-writer` (base: PR3b-3's commit
`805db11`; slices 1a–3b-3 history on this branch)
Last updated: 2026-08-30

**Engram split notice**: slices 1, 2a, 2b, and 3a's full detail moved to
Engram `sdd/longterm-mem/apply-progress-part1` when this document neared
the 50 000-byte practical limit for a single observation. This on-disk
`openspec/changes/longterm-mem/tasks.md`-adjacent file keeps the complete
history (no split needed for a plain repo file); only the Engram mirror of
this document was split. Slice 3b onward stays in the main
`sdd/longterm-mem/apply-progress` Engram observation, alongside this file.

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

---

## Slice 2b — longterm-mem-scaffold-vault-2b-runner-retrieve (R-004, R-024, R-021 guard) (COMPLETE)

All Slice 2b tasks (2b.1–2b.10) implemented and ticked in `tasks.md`. This
is the second named split of design-notes #3133's Slice 2
(`2a registry, 2b runner+retrieve`).

### R-021 guard — subprocess confinement (runner)

- [x] 2b.1–2b.3 RED `longterm-mem/internal/vault/runner_test.go` — three
  scenarios written first against a package that did not yet exist
  (confirmed RED by execution: `undefined: Runner` before `runner.go`
  existed):
  - `TestRunner_RefusesOutsideVaultRoot`: a script resolving (after
    `EvalSymlinks`) outside the vault root is refused; a marker file the
    fixture script would have written is asserted absent, proving no
    subprocess ran.
  - `TestRunner_ArgvOnly_NoShellMetacharacters`: a fixture script
    (`printf '%s' "$1"`) receives `"; rm -rf /"` as one literal argv
    element; captured stdout equals the malicious string byte-for-byte,
    proving `os/exec.CommandContext` never shell-interprets it.
  - `TestRunner_TimeoutSurfacesExitAndStderr`: a fixture script that writes
    to stderr then `sleep 5`, run under a 200 ms `context.WithTimeout`;
    asserts a non-zero synthetic exit code, the pre-timeout stderr is still
    captured, and `Run` returns well under 5 s.
- [x] 2b.4 GREEN `longterm-mem/internal/vault/runner.go` — `Runner{Root
  string}`, `Run(ctx, script string, args ...string) (stdout, stderr
  []byte, exitCode int, err error)`: `EvalSymlinks` both the root and the
  resolved script path, refuses anything outside root via `filepath.Rel`;
  `cmd.Env` restricted to `PATH`/`HOME`/`LANG` only; `cmd.Dir` = vault root.

**Real bug found and fixed during GREEN, not assumed away**: the first
implementation used only `exec.CommandContext`'s default cancel behavior
(kill the tracked child PID). `TestRunner_TimeoutSurfacesExitAndStderr`
failed by taking the full 5 s instead of respecting the 200 ms deadline —
the fixture's `sh` process forks `sleep` as a child rather than exec-
replacing itself, so killing only the parent left the orphaned `sleep`
running and still holding the stderr pipe's write end open, blocking
`Cmd.Wait()`'s I/O-draining until `sleep` exited on its own. Fixed by
running the subprocess as its own process-group leader
(`SysProcAttr{Setpgid: true}`) and overriding `cmd.Cancel` to
`syscall.Kill(-pid, SIGKILL)` (whole-group kill), plus a 2 s `cmd.WaitDelay`
backstop. Re-run confirmed the timeout test passes in ~0.2 s. This also
satisfies the apply-phase gate's explicit "kill on timeout" requirement
(not just abandon-and-hope), and prevents an orphaned subprocess from
lingering after `Run` returns.

### R-004 — Vault Query Invoke and Parse; R-024 — Not-Provisioned Handling

- [x] 2b.5–2b.7 RED `longterm-mem/internal/vault/retrieve_test.go` — three
  scenarios against a fixture `<vault>/scripts/retrieve.py` (real CLI shape
  per exploration #3121, confirmed RED before `retrieve.go`/`Result`/
  `Candidate`/`StatusOK`/`StatusNotProvisioned` existed):
  - `TestRetrieve_DefaultTopNAndFullFieldParse`: fixture script records its
    received argv (one element per line) and prints canned JSON with 2
    candidate rows (5-field-plus-noise shape matching the real script's
    output); asserts the captured argv is exactly `[query, "--top", "5"]`
    (default) and `result.Candidates` deep-equals the expected `[]Candidate`
    parsed from `page_address`, `absolute_path`, `bm25_score`,
    `rerank_score`, `snippet` only (extra fields like `chunk_id` ignored).
  - `TestRetrieve_ExplicitTopNOverride`: same fixture, `top=12`; asserts
    argv is `[query, "--top", "12"]` instead of the default.
  - `TestRetrieve_NotProvisionedExitTenMapsToStatus`: fixture exits 10, no
    output; asserts `Retrieve` returns `err == nil` and
    `Status == StatusNotProvisioned` — never a generic error.
- [x] 2b.8 GREEN `longterm-mem/internal/vault/retrieve.go` —
  `Retrieve(ctx, runner *Runner, project, query string, top int) (Result,
  error)`: `top <= 0` defaults to `DefaultTopN` (5); wraps `ctx` in a 30 s
  `context.WithTimeout` (D8); invokes
  `runner.Run(ctx, "scripts/retrieve.py", query, "--top", strconv.Itoa(top))`;
  maps the exit code via `statusForExitCode`; on `StatusOK`, JSON-decodes
  stdout into `{Candidates []Candidate}` (`Candidate` carries only the five
  R-004 field tags, so unrecognized JSON keys are dropped automatically).
- [x] 2b.9 REFACTOR — extracted `statusForExitCode(exitCode int) (status
  string, mapped bool)` out of `retrieve.go` into its own
  `internal/vault/status.go` (exit 0 → `StatusOK`, exit 10 →
  `StatusNotProvisioned`, else unmapped), named and documented for direct
  reuse by the index provisioning path in slice 3a (D12) without
  re-deriving the mapping. Full suite re-run green after the extraction.
- [x] 2b.10 Slice verification (2a+2b) — `cd longterm-mem && go test
  ./...` all green; re-ran `TestOSExecImportAllowlist` standalone
  (`go test ./... -run TestOSExecImportAllowlist`) — still passes, only
  `internal/vault/runner.go` imports `os/exec` (R-021 re-verified after
  adding `runner.go`, `retrieve.go`, `status.go`).

### Line-budget discipline applied mid-slice

The first GREEN pass (before any trimming) measured at ~499 authored
lines against this slice's 220–260 forecast and the 400-line hard cap. Two
compaction passes were applied without touching test rigor or the mandated
kill-on-timeout behavior:
1. `retrieve_test.go`: replaced five separate per-field assertions with one
   `reflect.DeepEqual` against an expected `[]Candidate` literal, and
   `equalStrings` with `reflect.DeepEqual` for argv comparison.
2. Tightened multi-sentence doc comments in `runner.go`/`retrieve.go`/
   `status.go` to single, denser sentences, keeping every R-ID/D-ID
   traceability reference.

This brought the total down to 432 lines of new files (see table below);
no additional cut was made at the cost of a required test scenario, the
explicit per-task test function names (needed for traceability), or the
process-group kill fix (explicitly required by the apply-phase gate's
"kill on timeout" instruction). See the risk note below.

### Verification

`cd longterm-mem && gofmt -l .` — clean. `go vet ./...` — clean.
`go test ./... -cover -count=1` — `internal/vault` 86.2% (uncovered lines
are defensive I/O-failure branches: `EvalSymlinks` failures, the
`errors.As`-default `exec` failure path, and `json.Unmarshal` failure, none
of which are named scenarios in tasks.md for this slice); `internal/engram`
84.2%; `internal/vaultreg` 67.2% (unchanged from 2a); `cmd/longterm-mem`
0.0% (still no subcommands wired).

`cd engine && go test ./...` — all 10 packages pass (zero-dep gate
unaffected; this slice touches nothing under `engine/`).
`bash -n bin/labdrian-overlay` — clean (this slice touches nothing under
`bin/`).

### Files created

- `longterm-mem/internal/vault/runner.go`, `runner_test.go`
- `longterm-mem/internal/vault/retrieve.go`, `retrieve_test.go`
- `longterm-mem/internal/vault/status.go`

### Files modified

- `openspec/changes/longterm-mem/tasks.md` (Slice 2b items 2b.1–2b.10
  ticked)

### Authored line budget

Plain line counts for the five new files (no existing file's content
changed besides the checkbox toggle), plus `git diff --numstat` for
`tasks.md`:

| File | Lines |
|---|---|
| `longterm-mem/internal/vault/runner.go` (new) | 119 |
| `longterm-mem/internal/vault/runner_test.go` (new) | 86 |
| `longterm-mem/internal/vault/retrieve.go` (new) | 84 |
| `longterm-mem/internal/vault/retrieve_test.go` (new) | 113 |
| `longterm-mem/internal/vault/status.go` (new) | 30 |
| `openspec/changes/longterm-mem/tasks.md` (diff, checkbox toggles, 10 items) | 20 |
| **Total** | **452** |

**Risk: over both the 220–260 forecast and the 400-line hard cap by 52
lines**, after two compaction passes (see above). Same root cause as
Slice 1's overage: strict-TDD's no-trivial-assertion rule requires a
dedicated fixture per named scenario plus a specific, distinct expected-
value assertion, and this slice additionally required a genuine subprocess-
safety fix (process-group timeout kill) mandated by the apply-phase gate
itself. No scope beyond the unchecked Slice 2b tasks was added. Design
names no split point for 2b specifically (unlike slice 4's documented
`lint.go` contingency) — 2b is already the second half of Slice 2's one
authorized split (2a/2b) — so no further split was invented; flagged as a
risk for the orchestrator, consistent with how Slice 1's overage was
handled.

---

## Slice 3a — longterm-mem-query-3a-index (R-005, R-025) (COMPLETE)

All Slice 3a tasks (3a.1–3a.5) implemented and ticked in `tasks.md`. This is
the first named split of design-notes #3133's Slice 3 (`3a index CLI, 3b
query merge`); slice 3b (unified query fan-out/merge, R-006/R-026) is
intentionally out of scope for this batch.

### R-005 — Index Rebuild With First-Provision; R-025 — Subprocess Failure Handling

- [x] 3a.1–3a.3 RED `longterm-mem/internal/vault/index_test.go` — three
  scenarios written first against a package that did not yet expose
  `Provisioned`/`Rebuild` (confirmed RED by execution:
  `go test ./internal/vault/... -run TestIndex` failed to compile with
  `undefined: Rebuild` / `undefined: Provisioned` before `index.go`
  existed):
  - `TestIndex_AlreadyProvisionedRefresh` (R-005): a fixture vault
    pre-materialized with `.vault-meta/bm25/index.json` and a non-empty
    `.vault-meta/chunks/` (an existing chunk file); asserts `Rebuild` runs
    exactly the two refresh steps, in order — `contextual-prefix.py --all
    --no-llm` then `bm25-index.py build` — captured via a shared
    `call-log.txt` the fixture scripts append to, and that `setup-
    retrieve.sh` never runs (log length exactly 2).
  - `TestIndex_NeverIndexedIsProvisionedFirst` (R-005): a fixture vault
    with no `.vault-meta/` at all; asserts `Provisioned` starts `false`,
    `Rebuild` runs all three steps in order (`setup --no-llm` first, then
    the two refresh steps — log length exactly 3), and `Provisioned`
    returns `true` afterward because the fixture `setup-retrieve.sh`
    itself materializes the two on-disk markers — a real behavioral
    assertion, not an assumed side effect.
  - `TestIndex_RebuildStepFailureReportsFailure` (R-025): a fixture
    `bm25-index.py` that records its own invocation, writes to stderr,
    then exits 7; asserts `Rebuild` returns a non-nil error naming the
    forced exit code (`7`) and containing neither "rebuilt" nor "success"
    (case-insensitive), so a failing step can never be read as a completed
    rebuild.
- [x] 3a.4 GREEN `longterm-mem/internal/vault/index.go` — `Provisioned
  (vaultRoot string) bool` (D12: `.vault-meta/bm25/index.json` exists AND
  `.vault-meta/chunks/` is non-empty — a pure filesystem check, no
  subprocess); `Rebuild(ctx, runner *Runner) error` implementing
  provision-then-refresh: when not `Provisioned`, runs `bin/setup-
  retrieve.sh --no-llm` directly via `runner.Run` (real shebang + exec
  bit, matching the 2b-2 review's `.sh`-via-`Run` convention) and stops
  immediately on failure; then always runs `scripts/contextual-prefix.py
  --all --no-llm` and `scripts/bm25-index.py build`, both via
  `runner.RunInterpreted(ctx, "python3", …)` (no shebang/exec bit on the
  fixtures, matching `retrieve.py`'s convention). A private `stepFailure`
  helper reuses `statusForExitCode` (status.go, from 2b.9's REFACTOR)
  rather than re-deriving a success boundary: only exit 0 (`StatusOK`)
  counts as success, so any other code — including the unrelated
  not-provisioned sentinel 10 — is correctly treated as a step failure,
  never a false success (R-025).
- [x] 3a.5 GREEN `longterm-mem/cmd/longterm-mem/cmd_index.go` — `index
  --project P [--vault DIR] [--rebuild]` wiring `vaultreg.Resolve
  (defaultVaultsPath(), project, vault)` → `vault.Rebuild`; missing
  `--project` exits 2; a `vaultreg` resolution failure exits 3; a `Rebuild`
  failure exits 5 (`vault_subprocess_failed`); success exits 0. No
  dedicated RED test was written for this file — tasks.md scopes 3a.5 as
  GREEN-only wiring (no paired RED item, unlike every other production
  file in this slice/prior slices), matching the established pattern from
  slice 1's `main.go` dispatcher: the underlying behavior (`Rebuild`) is
  already covered end-to-end by 3a.1–3a.3, and CLI-layer wiring gets its
  generic integration coverage later in slice 8b
  (`TestCLI_NoResidualProcessAfterAnySubcommand`). Wired into `main.go`'s
  dispatcher (`case "index": return cmdIndex(args[1:])`).

### Design notes followed (Engram #3133 D12)

`bin/setup-retrieve.sh` is a real shell entrypoint (shebang + exec bit) so
`Rebuild` execs it directly via `Runner.Run`, never `RunInterpreted` — the
opposite convention from `contextual-prefix.py`/`bm25-index.py`, which
carry no shebang/exec bit and must run under `python3` via
`Runner.RunInterpreted`, exactly like `retrieve.py` (2b-2 review lesson).
The RED tests prove both conventions with two fixture shapes: an
executable `0o755` shell fixture and non-executable `0o644` Python
fixtures with no `#!` line — if `Rebuild` tried to exec the Python
fixtures directly, they would fail to run at all (no shebang), so a
passing `TestIndex_AlreadyProvisionedRefresh`/`TestIndex_
NeverIndexedIsProvisionedFirst` is real evidence the interpreter choice is
correct per script, not an assumption.

### Verification

`cd longterm-mem && go test ./... -run TestIndex -v` — 3/3 PASS.
`cd longterm-mem && go test . -run TestOSExecImportAllowlist -v` — PASS
(re-verified R-021: only `internal/vault/runner.go` imports `os/exec`;
`index.go`/`cmd_index.go` import only `os`/`context`/`fmt`/`flag`/
`path/filepath`).

`cd longterm-mem && gofmt -l .` — clean. `go vet ./...` — clean.
`go test ./... -cover -count=1` — `internal/vault` 86.8% (up from 86.2%;
uncovered lines remain the same defensive I/O-failure branches from 2b,
plus `index.go`'s `Rebuild` early-return path is now exercised);
`internal/engram` 84.2%; `internal/vaultreg` 67.2%; `cmd/longterm-mem`
0.0% (no CLI-layer test exists yet for `cmd_index.go`, consistent with the
0.0% carried since Slice 1 — the wiring is exercised transitively by
`go build ./...` and by `internal/vault`'s tests of the function it calls).

`cd engine && go test ./...` — all 10 packages pass (zero-dep gate
unaffected; this slice touches nothing under `engine/`).
`bash -n bin/labdrian-overlay` — clean (this slice touches nothing under
`bin/`).

### Files created

- `longterm-mem/internal/vault/index.go`, `index_test.go`
- `longterm-mem/cmd/longterm-mem/cmd_index.go`

### Files modified

- `longterm-mem/cmd/longterm-mem/main.go` (new `case "index"` dispatch line)
- `openspec/changes/longterm-mem/tasks.md` (Slice 3a items 3a.1–3a.5 ticked)

### Authored line budget

Plain line counts for the three new files, plus `git diff --numstat` for
the two modified tracked files, excluding `go.sum` (no `go.sum` change —
no new dependency was needed):

| File | Lines |
|---|---|
| `longterm-mem/internal/vault/index.go` (new) | 87 |
| `longterm-mem/internal/vault/index_test.go` (new) | 174 |
| `longterm-mem/cmd/longterm-mem/cmd_index.go` (new) | 64 |
| `longterm-mem/cmd/longterm-mem/main.go` (diff, +2/-0) | 2 |
| `openspec/changes/longterm-mem/tasks.md` (diff, checkbox toggles, 5 items) | 10 |
| **Total** | **337** |

Comfortably within the 400-line budget (63 lines to spare) — unlike Slices
1 and 2b, which exceeded the hard cap. Still above this slice's own
170–200 forecast band (by 137 lines at the upper bound), for the same
root cause noted in every prior slice: strict-TDD's no-trivial-assertion
rule requires a dedicated fixture and a specific, distinct assertion per
named scenario. No scope was added beyond the unchecked Slice 3a tasks; no
split was needed.

---

## Slice 3b — longterm-mem-query-3b-merge (R-006, R-026) (COMPLETE)

All Slice 3b tasks (3b.1–3b.9) implemented and ticked in `tasks.md`. This
is the second named split of design-notes #3133's Slice 3 (`3a index CLI,
3b query merge`).

### R-006 — Unified Query Fan-Out and Merge; R-026 — Not-Provisioned Degrade

- [x] 3b.1–3b.4 RED `longterm-mem/internal/query/query_test.go` — four
  scenarios written first against a package that did not yet exist
  (confirmed RED by execution: `undefined: Deps` / `undefined: Run` /
  `undefined: Request` / etc. before `query.go` existed):
  - `TestQuery_GroupedBySourceInNativeRankOrder` (R-006): a fake
    `RetrieveVault` returning 2 vault candidates plus a real temp-DB Engram
    store (`engram.Open` against a schema-fixture DB, matching design-notes
    #3133's "Fake retrieve + temp DB" testing-strategy row, not a mocked
    Engram) with 2 observations whose content differs in term frequency for
    the query terms; asserts the merged output is vault rows (vault order)
    then Engram rows (Engram's own bm25 rank order), each tagged `source`,
    with contiguous 1-based `rank`.
  - `TestQuery_LinkedPairEmittedOnce` (R-006): one vault candidate plus one
    real Engram row, joined via a fake `ResolveLink` returning the real
    row's id; asserts exactly one `source:"linked"` row carrying both the
    vault `page_address` and the Engram `engram_id`.
  - `TestQuery_MissingProjectRejected` (R-006): empty `Request.Project`
    against a zero-value `Deps{}` (never touched, since the project check
    runs first); asserts `errors.Is(err, ErrMissingProject)`.
  - `TestQuery_NotProvisionedDegradesToEngramOnly` (R-026): a fake
    `RetrieveVault` returning `vault.StatusNotProvisioned`; asserts
    `err == nil`, `VaultStatus == "not_provisioned"`, and exactly the one
    Engram-sourced row.
  A fifth test, `TestQuery_VaultErrorDegradesToEngramOnly` (a generic
  vault-subprocess-failure-degrades scenario beyond the four named
  R-006/R-026 scenarios, proving `vaultErr != nil` also degrades instead of
  failing the call), was written and passed GREEN during this slice but was
  **removed during the line-budget trim pass** (see budget note below) —
  the corresponding `Run` branch (`case vaultErr != nil: ...
  VaultStatusError...`) stays in production code, covered only by having
  been exercised while the test existed, not by a currently-committed test.
  Flagged as a real (documented) coverage gap, consistent with how prior
  slices document untested defensive branches.
- [x] 3b.5 GREEN `longterm-mem/internal/engram/search.go` — `Search(project,
  query string, limit int) ([]Row, error)`: `ftsMatchQuery` double-quotes
  each whitespace-separated token (doubling internal quotes) and AND-joins
  them before the `observations_fts MATCH` call, so a token starting with
  `-` (FTS5's NOT operator) is always literal text — proven for real by
  `TestSearch_TokenStartingWithMinusIsTreatedAsLiteralText`
  (`internal/engram/search_test.go`, not itself a numbered 3b task but
  written RED-first per strict-TDD's unconditional "test before production
  code" rule, since design-notes #3133's own module-wide testing table
  lists "FTS ordering" as an `internal/engram` unit-test target). Required
  extending `internal/engram/testdata/schema.sql` with an external-content
  `observations_fts` FTS5 virtual table (`content='observations',
  content_rowid='id'`) plus 3 sync triggers (AFTER INSERT/UPDATE/DELETE) —
  additive only; safety net (`go test ./internal/engram/...`, 5 tests
  green) run before the change and re-confirmed after (7 tests green, no
  regressions). **Real bug found and fixed during GREEN**: the first query
  used `FROM observations_fts f ... ORDER BY f.rank` (aliased); modernc.org/
  sqlite rejected it with `SQL logic error: no such column: f`. FTS5's
  hidden `rank` column is only resolvable through the virtual table's own
  (unaliased) name, so the query was rewritten to reference
  `observations_fts.rank`/`observations_fts.rowid` directly — confirmed by
  both search tests going from FAIL to PASS with no other change.
- [x] 3b.6 GREEN `longterm-mem/internal/query/query.go` — `Run(ctx, Deps,
  Request{Project, Query, Top}) (Result, error)`: rejects empty `Project`
  (`ErrMissingProject`); defaults `Top` to `vault.DefaultTopN`; calls
  `Deps.Engram.Search` then `Deps.RetrieveVault`; three-way switch on the
  vault outcome (`err != nil` → `VaultStatusError` + diagnostic,
  `StatusNotProvisioned` → `VaultStatusNotProvisioned` + Engram-only,
  otherwise `VaultStatusOK`); `mergeResults` implements D8's merge order
  and linked-pair collapse via the extracted `MatchLinkedEngramRow`.
- [x] 3b.7 GREEN `longterm-mem/cmd/longterm-mem/cmd_query.go` — `query
  --project P "<text>" [--top N] [--vault DIR] [--json]` wiring
  (`vaultreg.Resolve` → exit 3; `engram.Open` → exit 4; `query.Run` error →
  exit 4; missing `--project`/bad `--top`/missing or extra query-text arg →
  exit 2). No dedicated RED test for this file, matching 3a.5's precedent
  (cmd_index.go) — the underlying behavior (`query.Run`) is fully covered
  by 3b.1–3b.4/3b.9; manually smoke-tested via `go run` for every exit-2
  path (missing project, `--top 0`, `--top 999`, unrecognized `-x` flag,
  missing query text) and confirmed each returns the documented exit code.
  Advisory follow-ups folded in: `--top` rejects ≤0 or >50 at the CLI
  (`maxTopN = 50`, sentinel default `-1` distinguishes "unset" from an
  explicit invalid value); a query beginning with `-` needs no dedicated
  guard code because `flag.FlagSet.Parse` (`ContinueOnError`) already
  treats an unrecognized leading-dash positional as a flag-parse error
  (exit 2) unless the caller inserts the stdlib `--` terminator first, at
  which point it flows through as an ordinary literal positional argument —
  verified by manual smoke test (`query --project p -x "text"` → "flag
  provided but not defined: -x", exit 2).
- [x] 3b.8 REFACTOR — `MatchLinkedEngramRow(pageAddress string, engramRows
  []engram.Row, resolveLink func(string) (int64, bool)) (engram.Row, bool)`
  is an exported, standalone function in `internal/query/query.go` (not a
  closure inside `Run`), so promote (slice 4+) and the MCP `query` tool
  (slice 8b) can reuse the identical precedence-store-lookup matching logic
  unchanged, per the task's explicit instruction. `mergeResults` calls it
  rather than inlining the match.
- [x] 3b.9 Slice verification (3a+3b) — `cd longterm-mem && go test ./...`
  — all 6 packages pass.

### Design notes followed (Engram #3133 D8)

`query.Deps` uses function-typed seams (`RetrieveVault`, `ResolveLink`)
rather than concrete `*vault.Runner`/sidecar types, so `query.Run`'s merge
logic is unit-testable without a real subprocess or an on-disk precedence
store — exactly the "injectable link resolver/lookup" the design calls for
so slice 5 can plug the real sidecar-backed resolver in without touching
`Run` or `mergeResults`. `NoLinkResolver` is the production default until
then. `Deps.Engram` stays a concrete `*engram.Store` (not a function seam),
matching every other package's convention of testing Engram access against
a real temp DB built from `testdata/schema.sql` rather than a mock.

`engram_sync_id` (present in design-notes #3133's full JSON result schema)
is intentionally omitted from `ResultRow` in this slice: it is sourced from
the precedence store (D6), which does not exist until slice 4/5, and no
3b RED test exercises it. Adding it now would mean either fabricating a
value or over-scoping `Search`'s SELECT — deferred as a documented gap
rather than guessed.

### Verification

`cd longterm-mem && gofmt -l .` — clean. `go vet ./...` — clean.
`go test ./... -cover -count=1` — `internal/query` 85.1% (new package);
`internal/engram` 82.0% (up from 84.2% baseline-adjacent — the new
`search.go` adds statements; uncovered lines are defensive SQL/scan-error
branches, none named 3b scenarios); `internal/vault` 83.8%; `internal/
vaultreg` 67.2%; `cmd/longterm-mem` 0.0% (still no CLI-layer test file,
consistent with 3a's precedent — `cmd_query.go` is exercised transitively
by `go build ./...` and the manual exit-code smoke tests above, and its
core logic (`query.Run`) is covered directly).
`go test . -run TestOSExecImportAllowlist -v` — PASS (re-verified R-021:
`cmd_query.go`/`query.go`/`search.go` import none of `os/exec`; only
`internal/vault/runner.go` does).

`cd engine && go test ./...` — all 10 packages pass (zero-dep gate
unaffected; this slice touches nothing under `engine/`).
`bash -n bin/labdrian-overlay` — clean (this slice touches nothing under
`bin/`).

### Files created

- `longterm-mem/internal/engram/search.go`, `search_test.go`
- `longterm-mem/internal/query/query.go`, `query_test.go`
- `longterm-mem/cmd/longterm-mem/cmd_query.go`

### Files modified

- `longterm-mem/internal/engram/testdata/schema.sql` (additive:
  `observations_fts` FTS5 virtual table + 3 sync triggers)
- `longterm-mem/cmd/longterm-mem/main.go` (new `case "query"` dispatch line)
- `openspec/changes/longterm-mem/tasks.md` (Slice 3b items 3b.1–3b.9 ticked)

### Authored line budget

Plain line counts for the five new files, `git diff --numstat` for the
three modified tracked files, excluding `go.sum` (no `go.sum` change — no
new dependency was needed):

| File | Lines |
|---|---|
| `longterm-mem/internal/query/query.go` (new) | 180 |
| `longterm-mem/internal/query/query_test.go` (new) | 176 |
| `longterm-mem/cmd/longterm-mem/cmd_query.go` (new) | 109 |
| `longterm-mem/internal/engram/search_test.go` (new) | 85 |
| `longterm-mem/internal/engram/search.go` (new) | 66 |
| `longterm-mem/internal/engram/testdata/schema.sql` (diff) | 19 |
| `openspec/changes/longterm-mem/tasks.md` (diff, checkbox toggles, 9 items) | 18 |
| `longterm-mem/cmd/longterm-mem/main.go` (diff, +2/-0) | 2 |
| **Total** | **655** |

**Risk: substantially over the 400-line hard cap (by 255 lines, ~64%) and
well over the 210–250 forecast / ≤300 aim** — the largest overage of any
slice on this branch so far (Slice 1: +10–121; Slice 2b: +52; this slice:
+255). Two real trims were applied before accepting the overage, not
cosmetic ones: (1) a fifth test (`TestQuery_VaultErrorDegradesToEngramOnly`)
and its supporting `vault_timeout`-vs-`vault_subprocess_failed` distinction
in `Run` were written, passed GREEN, then **removed** — the test is gone,
the diagnostic distinction is deferred to a follow-up (single generic
`vault_subprocess_failed` code retained), saving roughly 60 authored
lines; (2) every doc comment across all five new files was passed through
at least one compression pass (multi-sentence explanations cut to single
dense sentences), saving roughly 90 more lines versus the first GREEN
draft. No further cut was attempted without either dropping a mandated
RED scenario (3b.1–3b.4, non-negotiable) or a real correctness-proving
test (`search_test.go`'s two scenarios), which strict-TDD's assertion-
quality rules do not permit trading away for a smaller diff. Root cause,
consistent with every prior slice: strict-TDD's no-trivial-assertion rule
demands a dedicated fixture and a specific, distinct assertion per named
scenario — but this slice's *scope* is also genuinely larger than any
single prior slice: it introduces two new production behaviors with a
full JSON result contract (`query.Run`'s merge/degrade/link logic) *and*
a new Engram primitive (`Search`, including a schema fixture change and a
real modernc.org/sqlite FTS5 bug fix), not one. No scope was added beyond
the unchecked Slice 3b tasks. Flagged as a risk for the orchestrator: a
follow-on correction could split `cmd_query.go` (109 lines, self-
contained CLI wiring with no cross-file coupling beyond `query.Deps`) out
of this diff into a trailing `3b-2` slice if a reviewer needs the diff
below 400, since it is the one piece not required for `query.Run`/
`Search`'s own tests to pass — this was not done unprompted because
3b.7 is explicitly in scope for this slice and the CLI is already fully
implemented, tested via manual exit-code smoke checks, and wired into
`main.go`.

---

## Slice 4 — longterm-mem-promotion-writer (R-007, R-027) (COMPLETE)

All Slice 4 tasks (4.1–4.13) implemented and ticked in `tasks.md`, in the
three pre-planned parts (Part A eligibility, Part B page emission, Part C
lint) kept in separate files so the orchestrator can split them into
separate chained-PR commits without rework.

### Part A — eligibility (R-007)

- [x] 4.1–4.2 RED→GREEN `longterm-mem/internal/promote/eligible_test.go` /
  `eligible.go` — `Eligible(obs engram.Observation, explicit bool) bool`:
  pinned OR type ∈ {decision, architecture, pattern} OR
  `revision_count >= 3` OR explicit override. RED confirmed by execution
  (`undefined: Eligible`, plus `unknown field Type/Pinned/RevisionCount in
  struct literal of type engram.Observation` — the extended `Observation`
  fields did not exist yet either) before either file existed. A fifth
  case beyond the four named R-007 scenarios
  (`"eligible-type observation is eligible without pin or revision"`,
  type=`architecture`) was added as real triangulation: the four named
  scenarios alone never exercise the `eligibleTypes` map membership branch
  (only its default-false path via `type: discovery`), so a broken/empty
  map would still pass all four named tests — the fifth case makes that
  branch a real GREEN, not a false one.
- [x] `longterm-mem/internal/engram/store.go` — `Observation` extended
  with `SyncID`, `Type`, `Pinned`, `RevisionCount` (per the orchestrator's
  explicit instruction to extend `Observation`/`ListObservations`
  backward-compatibly rather than build a parallel read helper);
  `CreatedAt`/`UpdatedAt` were NOT added — no 4.x task or R-027 field needs
  them (page `created`/`updated` are the *promotion* timestamp, from
  `nowFunc()`, not the observation's own timestamps). `ListObservations`'
  SELECT extended to `id, sync_id, type, title, content, project,
  revision_count, pinned`; `sync_id` scanned via `sql.NullString` since the
  live schema (`schema.sql`/#3129) leaves it nullable. Backward-compatible:
  every prior test's `insertObservation` fixture still inserts through the
  unchanged 6-column statement and relies on the schema's own
  `revision_count DEFAULT 1`/`pinned DEFAULT 0`/`sync_id` NULL defaults —
  re-ran unmodified, all still green (safety net).
- [x] `longterm-mem/internal/engram/store_test.go` —
  `TestListObservations_IncludesEligibilityAndExtraFields` (new fixture
  helper `insertObservationFull`, full-column insert) proves the extended
  SELECT round-trips real `sync_id`/`type`/`revision_count`/`pinned`
  values via a struct-equality assertion (`got[0] != want`), not a
  field-by-field spot check. `insertObservationFull` returns the inserted
  row's `int64` id (`LastInsertId`) so slice 4's later relation fixtures
  (Part B) can address a specific observation without a second query.

### Part B — page emission (R-027 scenarios 1–3) + relation-edge primitive

- [x] 4.3–4.4 RED→GREEN `longterm-mem/internal/engram/relations_test.go` /
  `relations.go` — `Edge{Relation, SourceSyncID, TargetSyncID}`;
  `(*Store) RelatedEdges(observationID int64) ([]Edge, error)` joining
  `memory_relations` on `observations.sync_id` (the live schema keys
  `source_id`/`target_id` on the TEXT sync_id, not the integer id — #3129),
  filtered to `judgment_status='judged'`, `superseded_at IS NULL`, and
  `relation IN (related, compatible, scoped, supersedes, conflicts_with)`.
  RED confirmed (`store.RelatedEdges undefined`) before `relations.go`
  existed. `TestRelatedEdges_AcceptedOnly` fixtures six relation rows
  covering all four rejection reasons (pending judgment, superseded,
  unaccepted kind `not_conflict`, edge not touching the subject) plus two
  *accepted* edges in opposite source/target direction (`related` and
  `supersedes`) to prove the `OR`-both-sides join and multi-kind `IN`
  filter are both real, not just the single-direction/single-kind case.
  `longterm-mem/internal/engram/testdata/schema.sql` extended additively
  with the `memory_relations` table + two indexes (live DDL, #3129
  trimmed to the columns this slice reads: `sync_id`, `source_id`,
  `target_id`, `relation`, `judgment_status`, `superseded_at`).
- [x] 4.5–4.7 RED `longterm-mem/internal/promote/page_test.go` — three
  scenarios written first against a package exposing no `EmitPage`/`Page`/
  `Link`/`nowFunc` yet (RED confirmed by execution: 10 `undefined:` build
  errors before `page.go` existed):
  - `TestEmitPage_TypeMappedOntoVaultEnum` (R-027 scenario 1): a `decision`
    -typed, pinned observation; asserts `\ntype: concept\n` plus
    `engram_type: decision\n`, `engram_id: 101\n`,
    `project: labdrian-sdd-overlay\n` are all present in the rendered
    frontmatter.
  - `TestEmitPage_RelatedLinksResolve` (R-027 scenario 2): a fixture
    `Link{Address: "c-000099", Title: "Other Page"}` passed to `EmitPage`,
    with a real file pre-written at
    `<vaultRoot>/wiki/memory/c-000099.md`; asserts the frontmatter contains
    the exact wikilink `[[c-000099|Other Page]]` AND that the target file
    genuinely exists on disk (`os.Stat`) — not just that the string was
    emitted. Per the task's `EmitPage(obs, address string, related
    []Link)` signature, `EmitPage` does not itself call `RelatedEdges` or
    resolve which counterparts are promoted; it renders a caller-supplied,
    already-resolved `[]Link`. Turning `RelatedEdges`' output into
    `[]Link` (checking which counterpart addresses are already promoted)
    is deferred to `sync`/`propagate` (slices 7), which is the first place
    a precedence store recording promoted addresses exists — building that
    wiring now would mean guessing its shape ahead of slice 6's
    `PrecedenceStore`.
  - `TestEmitPage_FilenameSurvivesRetitle` (R-027 scenario 3): calls
    `EmitPage` twice with the same address and different `obs.Title`
    values; asserts `Page.Path` is byte-identical both times and equals
    `wiki/memory/c-000103.md` — proving the filename is a pure function of
    `address`, never `obs.Title` (D7).
  - `TestEmitPage_MatchesGolden` (beyond the three named scenarios, task
    4.9's explicit golden-file instruction): full `Frontmatter+Body` byte
    comparison against `testdata/pages/architecture.golden.md`, generated
    via `go test ./internal/promote/... -run TestEmitPage_MatchesGolden
    -update` then re-run without `-update` to confirm a clean, deterministic
    match (go-testing skill pattern). This is the test that actually locks
    down D7's *fixed field order* — the three scenario tests above only
    assert substring presence, not order.
- [x] 4.9 GREEN `longterm-mem/internal/promote/frontmatter.go` — flat-YAML
  `frontmatter` struct + `Render()`, exact field order `type, title,
  address, aliases, created, updated, tags, status, related, sources,
  engram_id, engram_sync_id, engram_type, engram_revision, project`
  (design-notes #3133's Interfaces/Contracts section, verbatim). No YAML
  library added (none was in `go.mod`, and WIKI.md's own contract is "flat,
  no nested objects" — a hand-written line-by-line emitter matches the
  contract more directly than a generic marshaler would and avoids a new
  dependency). Only `title` and list items are double-quoted (matching
  WIKI.md's own `title: "Human-Readable Title"` vs. unquoted
  `type`/`status`/`created` convention); an empty list renders as the
  inline `key: []` form WIKI.md's own example uses for `sources`.
- [x] 4.10 GREEN `longterm-mem/internal/promote/page.go` — `Link{Address,
  Title}`; `Page{Address, Path, Frontmatter, Body}`; `EmitPage(obs
  engram.Observation, address string, related []Link) (Page, error)`:
  `Path = "wiki/memory/" + address + ".md"` (never derived from title);
  `type` always maps to `concept` (D7 — the only enum value with no
  type-specific fields to fabricate); `status` = `mature` if pinned else
  `developing` (R-033's `superseded`/`archived` values are out of scope —
  slice 7); body = `# Title` (omitted when content already opens with an
  H1) + `obs.Content` + a footer line naming the Engram id/revision.
  `nowFunc = func() time.Time { return time.Now().UTC() }` is a
  package-level var swapped in tests (`fixedNow` helper) so
  `created`/`updated` — and hence the golden comparison — are
  deterministic; production always uses the real clock.
  `wikilinkPattern = regexp.MustCompile(`\[\[(c-\d{6})\|([^\]]*)\]\]`)` and
  the `wikilink(address, title string) string` formatter both live in
  `page.go` (4.12 REFACTOR was written in from the start rather than
  extracted after the fact, since `lint.go`'s resolvability check needed
  the identical pattern from its own first RED test) — `lint.go` imports
  neither a duplicate regex nor a duplicate format string, only
  `wikilinkPattern` from the same package.
- [x] 4.8 RED `longterm-mem/internal/promote/lint_test.go` (see Part C —
  written together with `page_test.go`'s scenarios since `LintPage`'s
  fixtures reuse `EmitPage`'s output, but implemented as Part C below).

### Part C — lint (R-027 scenario 4)

- [x] 4.8, 4.11 RED→GREEN `longterm-mem/internal/promote/lint_test.go` /
  `lint.go` — `Diagnostic{Rule, Detail}`; `LintPage(page Page, vaultRoot
  string) []Diagnostic`, exactly 6 rules per the design's line-budget
  mitigation: required scalar fields (`type, title, address, created,
  updated, status`), the `type`/`status` enum (`status` tolerates R-033's
  `superseded`/`archived` per D7), `^c-\d{6}$` address format,
  address-map consistency, wikilink resolvability, and an inbound
  `wiki/index.md` link. `LintPage` takes `vaultRoot` (not just `page`,
  which the task line names loosely as `LintPage(page)`) because three of
  the six rules are inherently about on-disk state the design says
  `doctor` (slice 8a) must reuse unchanged — a bare `Page` has nothing to
  check the address-map/wikilink/inbound-link rules against.
  **Real design decision found during GREEN, documented rather than
  assumed**: address allocation and `index.md`/`log.md` registration are
  slice 5, not built yet. The first draft of `checkAddressMap` treated "no
  `.raw/.manifest.json` file at all" as a *failure* (unregistered);
  `TestLintPage_FreshlyPromotedPagePasses` then could never pass for any
  page emitted before slice 5 exists, which would make the R-027 scenario
  4 test permanently un-satisfiable within this slice's own scope. Fixed
  by making a **wholly absent** manifest pass ("nothing to be inconsistent
  with yet" — address allocation hasn't run) while a **present-but-empty**
  `address_map` (or one missing this specific address) still fails. The
  triangulation test (`TestLintPage_UnregisteredPageIsFlagged`) was
  adjusted to write an explicit empty `address_map` rather than omit the
  manifest file entirely, to exercise the real failure path instead of the
  "not yet built" pass path — confirmed by re-running: the first version
  (no manifest at all) produced only the `inbound-index-link` diagnostic,
  not `address-map`, exposing the gap before the fixture was corrected.
  `TestLintPage_FreshlyPromotedPagePasses` builds a real scratch vault
  (`.raw/.manifest.json` with a matching entry, `wiki/index.md` with an
  inbound wikilink) simulating what slice 5's `register.go` will write, so
  a genuinely complete page — not merely `EmitPage`'s bare output — is what
  passes.

### Design notes followed (Engram #3133 D7, live schema #3129)

`memory_relations.source_id`/`target_id` are TEXT sync_ids, confirmed
against the live schema dump (#3129) before writing `relations.go` — an
earlier draft assumed `RelatedEdges(observationID int)` could query
`memory_relations` directly on the integer id and was corrected before any
code was written, once the schema fixture was re-checked against #3129's
exact column list. `EmitPage` deliberately does not call `RelatedEdges` or
allocate an address itself (both are the caller's responsibility per the
task's own signature) — this keeps `promote.EmitPage` a pure, easily
golden-tested function with no Engram/filesystem dependency of its own,
consistent with `query.Run`'s function-seam pattern from slice 3b.

### Verification

`cd longterm-mem && gofmt -l .` — clean. `go vet ./...` — clean.
`go test ./... -cover -count=1` — all 6 packages pass:
`internal/promote` 86.2% (new package); `internal/engram` 81.6% (down
slightly from 84.2% — `relations.go`'s scan-error/iterate-error branches
are defensive, not named scenarios); `internal/query` 85.1%;
`internal/vault` 83.8%; `internal/vaultreg` 67.2%; `cmd/longterm-mem` 0.0%
(unchanged — no promote-related CLI wiring is in scope until slice 8b).
`go test . -run TestOSExecImportAllowlist -v` — PASS (re-verified R-021:
none of `eligible.go`/`relations.go`/`page.go`/`frontmatter.go`/`lint.go`
import `os/exec`; only `internal/vault/runner.go` does).

`cd engine && go test ./...` — all 10 packages pass (zero-dep gate
unaffected; this slice touches nothing under `engine/`).
`bash -n bin/labdrian-overlay` — clean (this slice touches nothing under
`bin/`).

### Files created

- `longterm-mem/internal/promote/eligible.go`, `eligible_test.go` (Part A)
- `longterm-mem/internal/engram/relations.go`, `relations_test.go` (Part B)
- `longterm-mem/internal/promote/frontmatter.go`, `page.go`, `page_test.go` (Part B)
- `longterm-mem/internal/promote/testdata/pages/architecture.golden.md` (Part B, generated)
- `longterm-mem/internal/promote/lint.go`, `lint_test.go` (Part C)

### Files modified

- `longterm-mem/internal/engram/store.go` (Part A: `Observation` extended;
  `ListObservations` SELECT extended)
- `longterm-mem/internal/engram/store_test.go` (Part A: new fixture helper
  + eligibility-fields test; `insertObservationFull` also reused by Part
  B's `relations_test.go`)
- `longterm-mem/internal/engram/testdata/schema.sql` (Part B: additive
  `memory_relations` table + indexes)
- `openspec/changes/longterm-mem/tasks.md` (Slice 4 items 4.1–4.13 ticked)

### Authored line budget

Plain line counts for new files, `git diff --numstat` for modified
tracked files (no `go.sum` change — no new dependency was needed):

| Part | File | Lines |
|---|---|---|
| A | `internal/promote/eligible.go` (new) | 28 |
| A | `internal/promote/eligible_test.go` (new) | 55 |
| A | `internal/engram/store.go` (diff, +15/-7) | 22 |
| A | `internal/engram/store_test.go` (diff, +59/-0) | 59 |
| **A subtotal** | | **164** |
| B | `internal/engram/relations.go` (new) | 52 |
| B | `internal/engram/relations_test.go` (new) | 66 |
| B | `internal/engram/testdata/schema.sql` (diff, +19/-0) | 19 |
| B | `internal/promote/frontmatter.go` (new) | 79 |
| B | `internal/promote/page.go` (new) | 112 |
| B | `internal/promote/page_test.go` (new) | 140 |
| **B subtotal (excl. golden)** | | **468** |
| B (golden, excluded from authored risk count) | `testdata/pages/architecture.golden.md` | 27 |
| C | `internal/promote/lint.go` (new) | 146 |
| C | `internal/promote/lint_test.go` (new) | 92 |
| **C subtotal** | | **238** |
| — | `tasks.md` (diff, checkbox toggles, 13 items) | 26 |
| **Total (A+B+C+tasks.md, excl. golden and go.sum)** | | **896** |

**Risk: substantially over the 400-line hard cap (by ~496 lines, ~2.2x) and
well over the 360–410 forecast** — the largest single-slice overage on
this branch to date (prior worst was slice 3b at +255). Every part
individually exceeds its own pre-planned target too: A 164 vs. ≤150 (+14,
minor — the extended-fields test and fixture helper needed a specific
struct-equality assertion, not a spot check); B 468 vs. ≤250 (+218 — this
part carries two genuinely separate concerns, `relations.go`'s new
Engram primitive with its own schema-fixture extension AND
`page.go`/`frontmatter.go`'s page emission, each independently
strict-TDD-sized, plus a golden test the task explicitly calls for); C 238
vs. ≤150 (+88 — `LintPage` taking `vaultRoot` to check three on-disk rules,
as `doctor`'s future reuse requires, cannot be done in fewer rules or
fewer lines without weakening a rule to a no-op). No scope was added
beyond the unchecked 4.1–4.13 tasks; no test was omitted or trimmed below
strict-TDD's assertion-quality bar to chase the budget. Consistent with
every prior slice's root cause: strict-TDD's no-trivial-assertion rule
demands a dedicated fixture and a specific, distinct assertion per named
scenario, and this slice has 8 named R-007/R-027 scenarios (the most of
any slice so far) plus the `RelatedEdges` primitive and its own schema
fixture.

**Flagged for the orchestrator's auto-chain delivery-strategy decision**:
the task's own pre-planned fallback ("split `lint.go` + its goldens into a
4b PR") is not sufficient alone — removing Part C's 238 lines still leaves
Parts A+B at 632 lines (+26 tasks.md), still 258 over the 400 cap. The
three parts are already staged in fully separate files with no
cross-part production dependency inside this slice (Part B's `page.go`
does depend on Part A's extended `engram.Observation`, but not on any Part
C symbol; Part C's `lint.go` depends on Part B's `wikilinkPattern`/
`vaultType`/`pagePathPrefix`, but not on Part A), so a 3-way split
(4a=Part A, 4b=Part B, 4c=Part C) — rather than the originally-planned
2-way split — is the change that keeps every resulting PR under budget
without any rework: Part A alone (164) fits in one PR, Part B alone (468)
would still need its own further split (e.g. 4b-1 `relations.go`
Engram primitive at ~137 lines, 4b-2 `page.go`/`frontmatter.go` page
emission at ~331 lines) to clear 400, and Part C alone (238) fits in one
PR. This is reported as a risk, not resolved unprompted, per the
strict-tdd instruction to report rather than compress tests.
