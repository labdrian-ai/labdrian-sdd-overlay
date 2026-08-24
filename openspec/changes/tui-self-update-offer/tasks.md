# Tasks: TUI Self-Update Offer on Launch

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~500-560 (6 files modify, 1 create; incl. table-driven Go tests + bash integration harness) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR1 backend → PR2 probe+banner → PR3 action+re-probe |
| Delivery strategy | auto-chain |
| Chain strategy | feature-branch-chain |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | `cmd_self_update` backend (D1-D3) + dispatcher/usage | PR1 (base: tracker) | `go test ./tui/... -run SelfUpdate` | scratch clone + bare origin via `OVERLAY_DIR` | drop `cmd_self_update`, dispatcher row, usage line |
| 2 | Probe + banner (D4-D6) | PR2 (base: PR1 branch) | `go test ./tui/... -run 'ProbeBehind|RepoLine'` | N/A — pure funcs + table tests | drop probe wiring, banner branch, new model fields |
| 3 | Action entry + re-probe (D7, D5 tail) | PR3 (base: PR2 branch) | `go test ./tui/...` | N/A — unit-testable via `runDoneMsg` fixture | drop `Actions()` row + re-probe branch |

## Phase 1: Backend — `bin/labdrian-overlay`

- [x] 1.1 Create `tui/selfupdate_backend_test.go`: scratch-clone + bare-origin harness via `OVERLAY_DIR`.
- [x] 1.2 RED: cases — behind→ff+return (R-004); up-to-date→exit0 no checkout; dirty tracked→exit1 (R-005); untracked-only→proceeds; ahead→exit1 (R-006); no origin→exit1; blocked checkout mid-op→branch restored; held `.git/index.lock`→exit1, branch intact.
- [x] 1.3 GREEN: implement `cmd_self_update()` per D1/D2/D3 — die checks (no origin, dirty tree, ahead), bounded fetch, up-to-date short-circuit, trap-based checkout swap, `merge --ff-only`.
- [x] 1.4 Wire dispatcher row (`:1798`) + `usage()` entry.
- [x] 1.5 R-007: assert `REPO_BEHIND_ORIGIN=0` via sync-check after a successful update.

## Phase 2: Probe — `tui/run.go`

- [x] 2.1 RED: table test `probeBehind()` — count / `NA` / zero verdicts / garbage output.
- [x] 2.2 GREEN: implement `probeBehind()` (D4), default `RepoBehindOriginNA` on any failure.
- [x] 2.3 RED: test `probeBehindOriginCmd()` — no `--fetch`, `cmd.Dir=root`, feeds output to `probeBehind` even on nonzero exit.
- [x] 2.4 GREEN: implement `probeBehindOriginCmd()`.

## Phase 3: Model wiring — `tui/model.go`

- [x] 3.1 RED: `Init()` returns a non-nil cmd batch including the probe.
- [x] 3.2 GREEN: wire `probeBehindOriginCmd()` into `Init()`.
- [x] 3.3 RED: `probeDoneMsg` sets `behindOrigin`; `newModel` inits it to `RepoBehindOriginNA` (guards R-006-class zero-value bug).
- [x] 3.4 GREEN: add `behindOrigin`/`bannerDismissed` fields + `case probeDoneMsg` (D5).
- [x] 3.5 RED: `x` dismisses the banner only when visible; no-op otherwise.
- [x] 3.6 GREEN: add global `x` intercept in `tea.KeyMsg` handling, guarded by banner-visible (D6).

## Phase 4: Banner — `tui/view.go`

- [x] 4.1 RED: `repoLine()` states — `behindOrigin>0` shows (R-002); `0`/`NA` hide; dismissed hides; `rootErr` takes precedence.
- [x] 4.2 GREEN: add banner branch in `repoLine()` (`:125`), `colorAmber` copy per D6.

## Phase 5: Action entry + re-probe — `tui/run.go`, `tui/model.go`

- [ ] 5.1 RED: `Actions()` includes "Actualizar repositorio", `TargetAgnostic`/`Mutating` true, positioned after "Aplicar cambios" and before the hooks block (R-003).
- [ ] 5.2 GREEN: add the entry per D7's struct literal.
- [ ] 5.3 RED: `screenConfirm` skips `screenTargets`; confirm text names `main` as sole updated branch.
- [ ] 5.4 RED: successful `self-update` `runDoneMsg` re-fires the probe.
- [ ] 5.5 GREEN: add re-probe branch (D5 tail) on `runDoneMsg` when `action.Command=="self-update" && err==nil`.

## Phase 6: Verification

- [ ] 6.1 Run full `go test ./tui/...`; confirm all RED tasks are now GREEN.
- [ ] 6.2 Manual E2E smoke: real clone behind `origin/main`, banner shows, run "Actualizar repositorio", banner clears/re-probes.
- [ ] 6.3 Confirm every applicable threat-matrix row (repo selection, commit state, local-ahead, no origin, malformed probe, mid-op checkout failure, concurrent git op) has passing coverage.
