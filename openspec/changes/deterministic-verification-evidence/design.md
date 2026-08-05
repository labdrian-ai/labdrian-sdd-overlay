# Design: Deterministic Verification Evidence

## Technical Approach

New Go module `tools/deterministic-check-runner/`, a sibling of `tools/entry-contract-validator` (`go.mod`, `main.go`, `main_test.go`, `testdata/`, single `package main`, testable `run(args, stdout, stderr) int`). It executes the hardcoded v1 checks (`gofmt`, `go vet`, `staticcheck` blocking; `deadcode` WARNING) across every Go module (sorted `go.mod` walk), emits one `tool | exit_code | summary` row per check, and encodes the aggregate outcome in its process exit code. `bin/labdrian-overlay deterministic-checks` mirrors `cmd_validate_entry_contract` (two exit-3 guards, `CALLER_CWD` normalization, temp-binary build, exit propagation). `sdd-verify` declares `normalize` pre-`review start`, runs `check` post-freeze, pipes stdout into `gentle-ai review capture-evidence --input -`, and maps exit code to `--outcome` mechanically. Slice 1 lands the severity policy first so all Go code is authored under it.

## Architecture Decisions

| # | Decision | Choice | Rejected | Rationale |
|---|---|---|---|---|
| D1 | `tools` test reachability | 4th `testing.layers` entry + append `for m in tools/*/go.mod; do (cd "$(dirname "$m")" && go test ./...) \|\| exit 1; done` (from repo root) to both `test_command`s; keep dedicated CI job for the runner (R-005 acceptance) | Per-module lines; CI-only | Glob absorbs future `tools/*` modules with no config edit; fails loud; `sdd-verify` PASS gains its own evidence chain instead of leaning on external CI |
| D2 | Check registry | Hardcoded `[]check` literal in `main.go`; each check declares `deterministic` AND `blocking` explicitly (`gofmt`, `go vet`, `staticcheck` blocking; `deadcode` not); effective blocking = `blocking && deterministic`, derived only inside `classify()` — `selectOutcome` consumes `classify()` output for every gate; registry invariant test | Config file; separate package; deriving blocking from tool name | User decision (no config parser); one enforcement point; precedent is single `package main` |
| D3 | Failure predicate | Per-check `failed(exit, count)`: `gofmt -l` exits 0 with findings, so gofmt fails on `count > 0`; others on `exit != 0`; rows always carry the raw exit code | Synthesized exit codes | Rows stay machine-truthful; blocking decision separated from transport |
| D4 | Outcome precedence (amended R-016) | Pure `selectOutcome([]result)`: unexecutable BLOCKING-set check OR runner-internal error → `procedural_tooling_failed` (exit 3) ≻ failed blocking deterministic check → `verification_failed` (exit 1) ≻ `passed` (exit 0); usage=2. Unavailable tools emit exit-127 rows; an absent WARNING-set tool (`deadcode`) classifies as WARNING and never alone yields exit 3. `unavailable` and `failed` stay distinct per-result states | Uniform unavailability escalation (superseded first choice); precedence in skill prose | A tool that can never block must not invalidate the run's outcome by being absent; one unit-testable function still owns the decision |
| D5 | R-010 byte/mode neutrality | Structural: check-mode argv allowlist has no fixer flags; `check` rejects `--out-dir` inside repo root (usage error). Proof: integration test snapshotting `git status --porcelain` + per-file `(mode, size, sha256)` walk (skip `.git`) before/after `check` on a dirty `t.TempDir()` git fixture with a 0755 file | Runtime self-snapshot per run | Guarantee comes from what `check` may execute, proven once; runtime git dependency rejected (minimalism) |
| D6 | Summary rendering | `count=N; top: e1; …; full: <path>`; zero findings renders `0`; excerpts capped 200 chars; stable default out-dir `${TMPDIR}/labdrian-deterministic-checks/<tool>.log` (overwritten); `capPayload` drops excerpts above 4 MiB, keeping counts+paths | Timestamped out-dirs; no cap | Stable paths keep rerun rows byte-identical; caps make the 4 MiB branch testable synthetically |
| D7 | Slice-1 strict-TDD gate | Content RED/GREEN: fixed `rg` assertions failing pre-edit, passing post-edit — CRITICAL text at `strict-tdd-verify.md:121/:127`, `:266` scoped to coverage/dead-code only, blocking `staticcheck ./...` in both CI jobs, deadcode `continue-on-error: true`, `name: tools` in config.yaml. Green `go test` MUST NOT be cited as slice-1 evidence | Trusting trivially-green tests | Markdown edits need content assertions; a green Go suite proves nothing about the policy diff |
| D8 | Missing CLI in deployed runtimes | `sdd-verify` adds a `command -v` guard; absent `bin/labdrian-overlay` → `--outcome procedural_tooling_failed`; absent `gentle-ai` → plain-report note; never `verification_failed`, never silent pass | Manifest row for the CLI | Deployment boundary is behavioral (exploration Q3); mirrors fail-loud precedent |

Slice-2 split seam (if >800 lines): `runner-registry-and-rows` (registry, classify, selectOutcome, rows) / `runner-summary-rendering` (parse funcs, top-N, capPayload, goldens).

## Data Flow

```
sdd-verify (post-freeze, read-only)
 ├→ guard: command -v labdrian-overlay ── absent → outcome=procedural_tooling_failed
 ├→ bin/labdrian-overlay deterministic-checks check [--top-n 5] [--out-dir D]
 │    guards: runner source? go toolchain? → exit 3
 │    └→ runner: per module × check → rows on stdout, full logs in D, exit 0|1|3
 └→ map exit {0→passed, 1→verification_failed, 3→procedural_tooling_failed}
      └→ gentle-ai review capture-evidence --input - --outcome <o>
normalize (gofmt -w only) runs pre-`review start` ONLY (R-011)
```

## File Changes

| File | Action | Description |
|---|---|---|
| `tools/deterministic-check-runner/{go.mod,main.go,main_test.go,testdata/}` | Create | Runner (slices 2-3) |
| `skills/sdd-verify/strict-tdd-verify.md` | Modify | Severity at :121/:127/:266 (slice 1) |
| `skills/sdd-verify/SKILL.md` | Modify | R-011 ordering, wiring, guards (slice 4) |
| `.github/workflows/ci.yml` | Modify | staticcheck blocking, deadcode non-blocking, runner test job (slices 1-2) |
| `openspec/config.yaml` | Modify | `tools` layer + test_command loop (slice 1) |
| `bin/labdrian-overlay` | Modify | `cmd_deterministic_checks` + dispatch + help (slice 4) |

`overlay.manifest`: no new rows — edited skill files are rows 22/41; no new `skills/` file planned, so R-019 is satisfied by the unchanged `skills validate` step.

## Interfaces / Contracts

```go
type check struct {
    name          string
    deterministic bool     // required for CRITICAL eligibility (R-002)
    blocking      bool     // combined with deterministic ONLY in classify()
    checkArgv     []string // read-only invocation; no fixer flags
    normalizeArgv []string // nil when the tool has no fixer
    parse         func(exit int, out []byte) (count int, top []string)
    failed        func(exit, count int) bool
}
type result struct{ /* check + exit, count, top, logPath, unavailable */ }
func selectOutcome(rs []result) int // 0|1|3 per D4
```

Row: `%s | %d | %s\n`. Stdout always emits ≥4 rows, so `minLength 1` holds structurally.

## Testing Strategy

| Layer | What | Approach |
|---|---|---|
| Unit | `classify`; `selectOutcome` precedence: blocking tool absent → 3, `deadcode` absent alone → 0, `deadcode` absent + blocking failed → 1, runner error → 3, non-blocking red → 0; summary 0/1/N/N+1; banned literals; `capPayload`; parse funcs | Table-driven |
| Golden | Full stdout row block (R-006) | testdata goldens via `-update` path |
| Integration | Byte/mode neutrality (D5); missing tool via stubbed `PATH` | `t.TempDir()` git fixture; skip in `-short` |
| Shell | `bash -n`, `shellcheck`, dispatch smoke, exit-3 guards | Existing lint-shell pattern |
| Content | Slice-1 RED/GREEN assertions (D7); `git diff main -- bin/overlay` empty (R-001) | Recorded commands + exit codes |
| CI | staticcheck gate, deadcode visible + green job, runner job | Green run on branch |

## Threat Matrix

| Boundary | Applicability | Design response | Planned RED tests |
|---|---|---|---|
| Documentation-like paths | N/A — fixed hardcoded tool set; no file-type-based execution | — | — |
| Git repository selection | Applicable — module discovery and out-dir guard depend on repo root | Root = `CALLER_CWD`-normalized cwd; sorted `go.mod` walk; `check` rejects out-dir inside root | Run from subdir; out-dir inside repo → usage error |
| Commit state | N/A — runner issues no git write commands | — | — |
| Push state | N/A — no push automation | — | — |
| PR commands | N/A — no PR automation | — | — |

## Migration / Rollout

No migration; slices revert independently. Named assumption: native review never consumes `strict-tdd-verify.md` as reviewer input and `sdd-verify` reads disk live — documented behavior of external `gentle-ai` v2.2.4, not source-verifiable here; chain ordering isolates slice 1 regardless.

## Open Questions

None blocking.
