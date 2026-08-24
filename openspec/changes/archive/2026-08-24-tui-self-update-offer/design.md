# Design: TUI Self-Update Offer on Launch

**Change**: `tui-self-update-offer` | **Store**: hybrid | **Base**: main (post-PR #140, `ef35927`)

## Technical Approach

Three additive pieces on proven rails: (1) new flagless backend subcommand `self-update` cloning `cmd_capture`/`cmd_apply`'s pre-checks and trap-based checkout pattern; (2) a `tea.Cmd` from `Init()` running cached-only `sync-check`, parsed by the existing `ParseSyncCheck`; (3) a dismissible amber banner in `repoLine()` plus one new `Actions()` row through the untouched confirm/run/result pipeline. Zero new screens; `REPO_BEHIND_ORIGIN` semantics unchanged (consumer only).

## Architecture Decisions

### D1 — Subcommand name and flags
**Choice**: `self-update`, zero options (`*) die "Unknown option"`); registered in dispatcher (`bin/labdrian-overlay:1798`) and `usage()`.
**Rejected**: `--target` (signal is repo-level); a no-fetch mode (updating against a stale cached ref "succeeds" without converging — the action must fetch); other names (proposal precedent: `cmd_self_update`).

### D2 — Operation order: every refusal fires before any checkout
1. No `origin` remote → `die`.
2. Dirty tracked tree (`git status --porcelain --untracked-files=no`; untracked ignored — capture/apply parity, `:586`/`:716`) → `die`.
3. Bounded fetch: `timeout 10 git -c http.lowSpeedLimit=1000 -c http.lowSpeedTime=10 fetch origin main` (idiom from `compute_repo_behind_origin:922-935`; `timeout` optional) → `die` on failure.
4. `git rev-list --count origin/main..main` > 0 → `die` — hard refusal, deliberately stricter than `cmd_apply:805-814`'s advisory (proposal-settled).
5. `git rev-list --count main..origin/main` == 0 → "already up to date", exit 0, no branch switch at all.
6. Only now: capture `current_branch` → `trap "git checkout '${current_branch}' >/dev/null 2>&1 || true" EXIT` (name pre-expanded — the `set -u` lesson from `c891c94`) → `git checkout main` → `git merge --ff-only origin/main` → checkout back → `trap - EXIT`.

**Rejected**: `git fetch origin main:main` ref-update without checkout — forks into on-main/off-main cases and abandons the mandated, audited trap pattern; stash/merge reconciliation — refusals stay refusals.

### D3 — Output contract: exit code + plain lines, zero TUI special-casing
`runBackend` (`tui/run.go:387-432`) captures CombinedOutput and maps exit 0 → "✓ Completado", 1 → "✗ Comando falló", 2 → degraded; verdicts parse only when `Command == "sync-check"`. So: stdout `Updated main: <old>..<new> (fast-forward)` or `main is already up to date with origin/main`; refusals are one-line `die` stderr, exit 1. No VERDICT lines, never exit 2.

### D4 — Launch probe reuses sync-check + ParseSyncCheck
**Choice**: `Init()` returns `probeBehindOriginCmd()`: run `bin/labdrian-overlay sync-check` (no `--fetch` → cached-only), `cmd.Dir = root`; feed CombinedOutput (even on non-zero exit) to pure helper `probeBehind(output string) int` = first `ParseSyncCheck` verdict's `RepoBehindOrigin`, else `RepoBehindOriginNA`. Any failure (rootErr, exec error, zero verdicts — e.g. all target dirs missing skip VERDICT at `:981-984` — or `NA`) degrades to `RepoBehindOriginNA`.
**Rejected**: direct `git rev-list` in Go (duplicates detection; proposal: "adds a consumer only"); new `--repo-only` backend flag (backend surface beyond proposal scope); routing through `runBackend` (drags Action/result semantics into a headless probe).

### D5 — Msg type and Update branch
`type probeDoneMsg struct{ behind int }`. Model gains `behindOrigin int` — initialized to `RepoBehindOriginNA` in `newModel` (a zero value collapses into "confirmed 0 behind", the R-006 bug class `ParseSyncCheck:171-174` guards) — and `bannerDismissed bool`. `Update`: `case probeDoneMsg:` set field, return nil cmd. On `runDoneMsg` with `action.Command == "self-update" && err == nil`, also re-fire the probe (fetch just refreshed the cache; banner self-corrects). The probe structurally cannot block `screenTargets`: `Init`'s cmd is async, the first frame renders before it resolves, and every probe path returns a msg.

### D6 — Banner rendering and dismissal
In `repoLine()` (`tui/view.go:125`): `rootErr` keeps precedence; else when `behindOrigin > 0 && !bannerDismissed`, render in `colorAmber` (defined for exactly this, `view.go:15`):
`▲ Repo N commit(s) detrás de origin/main · «Actualizar repositorio» pone al día solo main · x ocultar`
Dismissal: global `x` intercept in `Update`'s `tea.KeyMsg` (beside `ctrl+c`), guarded by banner-visible — works on any screen, independent of navigation; session-scoped. Honest caveat: the signal is `HEAD..origin/main`, so on a stale feature branch the banner may legitimately survive a successful update (main converged; HEAD didn't) — copy promises nothing it can't keep.

### D7 — Action entry and placement
After "Aplicar cambios", before the hooks block (repo-maintenance grouping; core flow order untouched):
```go
{Name: "Actualizar repositorio", Command: "self-update", Mutating: true, TargetAgnostic: true,
 ConfirmMessage: "Actualiza SOLO la rama main a origin/main (fast-forward); tu rama actual no se toca y se vuelve a ella al terminar. Rechaza con árbol sucio o main local adelantado.",
 Hint: "Pone main al día con origin/main (ff-only)"}
```

## Data Flow

    Init() ─probeBehindOriginCmd→ sync-check (cached) ─ParseSyncCheck→ probeDoneMsg → m.behindOrigin → repoLine() banner
    banner names action → Actions() row → screenConfirm → runBackend("self-update") → runDoneMsg → screenResult
                                                                              └─ success → probe re-fired

## File Changes

| File | Action | Description |
|---|---|---|
| `bin/labdrian-overlay` | Modify | `cmd_self_update` (D1/D2/D3) + dispatcher row + `usage()` entry |
| `tui/model.go` | Modify | fields, `Init()` cmd, `probeDoneMsg` branch, `x` intercept, success re-probe |
| `tui/run.go` | Modify | Action entry (D7); `probeBehind` helper + exec wrapper (D4) |
| `tui/view.go` | Modify | banner branch in `repoLine()` (D6) |
| `tui/main_test.go`, `tui/view_test.go` | Modify | unit coverage below |
| `tui/selfupdate_backend_test.go` | Create | Go→bash integration harness (scratch repos via `OVERLAY_DIR` override, sanctioned at `bin/labdrian-overlay:14-16`) |

## Interfaces / Contracts

Defined in D3 (backend stdout/exit) and D5 (`probeDoneMsg`, model fields). No other public surface changes.

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit (Go, `tui`) | `probeBehind`: count / `NA` / zero verdicts / garbage output; `Init()` non-nil; `probeDoneMsg` sets field; `x` dismisses only when visible; self-update success re-fires probe; `repoLine` states (>0 shows, 0/NA hide, dismissed hides, rootErr wins); `Actions()` order/entry pins | table-driven, pure functions first (strict TDD: RED before code) |
| Integration | `self-update` against scratch clone + bare origin (`OVERLAY_DIR` env) | behind→ff+return-to-branch; up-to-date→exit 0, no checkout; dirty→exit 1 untouched; ahead→exit 1; no origin→exit 1; blocked checkout→branch restored |
| E2E | TUI smoke on a real behind clone | manual, per proposal success criteria |

## Threat Matrix

| Boundary / scenario | Applicability | Design response | Planned RED tests |
|---|---|---|---|
| Documentation-like paths | N/A — git refs only, no file classification/execution | — | — |
| Git repository selection | Applicable — backend `cd "$OVERLAY_DIR"` (env-overridable); probe sets `cmd.Dir = root` | all mutations scoped to the resolved clone | scratch-repo tests prove `OVERLAY_DIR` scoping |
| Commit state | Applicable — dirty tracked refusal; untracked ignored | D2 step 2, pre-checkout | dirty→exit 1, repo untouched; untracked-only→proceeds |
| Push state | N/A — fetch-only, never pushes | — | — |
| PR commands | N/A — no PR automation | — | — |
| Local-ahead divergence | Applicable | D2 step 4 hard `die`, pre-checkout | ahead→exit 1, refs untouched |
| No origin remote | Applicable | backend: D2 step 1 `die`; probe: `NA` → no banner | exit 1; `probeBehind` NA unit |
| Never-fetched cache / malformed probe | Applicable | any probe failure → `RepoBehindOriginNA` → no banner; `screenTargets` always reachable | `probeBehind` + `repoLine` units |
| `main` checkout fails mid-op | Applicable | pre-expanded EXIT trap restores original branch best-effort; exit 1 | untracked file colliding with main's content → checkout refused → original branch intact |
| Concurrent external git op | Applicable — git index/ref locks fail our steps loudly under `set -euo pipefail`; same trap path; ref updates atomic | fail loudly, restore branch | pre-created `.git/index.lock` → exit 1, branch intact |

## Migration / Rollout

Purely additive; revert = drop the subcommand and TUI hunks. No migration.

## Open Questions

- None blocking.
