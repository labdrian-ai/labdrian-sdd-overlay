# Proposal: TUI Self-Update Offer on Launch

## Intent

Opening the labdrian TUI on a stale clone gives no update prompt; host-vs-`origin` drift accumulates silently until sync issues surface. Mirror gentle-ai's check/apply split: detect "behind origin" at launch and offer an explicit, safe update so the clone converges with `origin/main` — no diffs, no conflicts, no silent mutation.

## Scope

### In Scope

- Launch-time probe of the existing `REPO_BEHIND_ORIGIN` signal as a `tea.Cmd` from `Init()`, cached-ref-only (no `git fetch`).
- Dismissible behind-origin banner rendered via the existing `repoLine()` slot — no new screen, honoring the `tui-hooks-commands` "no new screens" precedent.
- New `Actions()` entry ("Update repository", `TargetAgnostic: true`, `Mutating: true`) using the existing confirm/run/result pipeline unchanged.
- New `bin/labdrian-overlay` subcommand (e.g. `cmd_self_update`): checkout `main` → fast-forward-only update from `origin/main` → return to original branch via the existing trap pattern; hard-refuses (exit 1) on dirty tracked tree or local-ahead divergence.

### Out of Scope

- `UPSTREAM_CHANGED` / `OVERLAY_NOT_DEPLOYED` — different remote relationship (`upstream`), already surfaced via "Verificar sincronización". **Explicit decision: this change targets `origin` only.**
- Live fetch at launch. **Explicit decision: probe stays cached-only**, preserving `sync-check-verdicts` R-001's opt-in-fetch tradeoff; banner freshness equals last fetch.
- Any merge/rebase/stash reconciliation — refusals stay refusals.
- Touching the user's checked-out feature branch; only `main` ever moves.

## Capabilities

### New Capabilities

- `tui-self-update`: launch-time behind-origin offer plus main-scoped ff-only update action; references `sync-check-verdicts` for detection instead of duplicating it.

### Modified Capabilities

- None — `REPO_BEHIND_ORIGIN` semantics, VERDICT fields, and exit codes are unchanged; this adds a consumer only.

## Approach

Exploration Approach 1 (Engram obs #3035): passive cached-only banner + explicit Action. Reuses the data-driven `Action` struct and confirm/run/result screens (precedent: `install-hooks`/`uninstall-hooks`) and mirrors `cmd_capture`/`cmd_apply` safety checks rather than inventing new git logic. Blocking modal and silent auto-pull were evaluated and rejected.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `tui/model.go` | Modified | `Init()` probe cmd; new msg branch in `Update()` |
| `tui/view.go` | Modified | Banner in `repoLine()`/`View()` |
| `tui/run.go` | Modified | New self-update `Action` |
| `bin/labdrian-overlay` | Modified | New ff-only main-scoped subcommand |
| `tui/main_test.go`, `tui/view_test.go` | Modified | Coverage for probe, banner, action |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| User reads "repo behind" as "my branch behind" | Med | Banner + confirm copy state only `main` is updated |
| Stale cached ref hides staleness | Med | Accepted R-001 tradeoff; `--check-origin` path stays the fresh check |
| No `tea.Cmd`-in-`Init()` test precedent | Med | Establish convention in spec/tasks; strict TDD |

## Rollback Plan

Purely additive: revert the change commits; removing banner, action, and subcommand restores current behavior. No state or data migration.

## Dependencies

- None external. Relies on existing `compute_repo_behind_origin()` and the TUI action pipeline.

## Success Criteria

- [ ] Launch on a behind clone (cached ref) shows a dismissible banner; no network call at launch.
- [ ] Update on a clean tree fast-forwards `main` to `origin/main`, returns to the original branch; `REPO_BEHIND_ORIGIN=0` after.
- [ ] Dirty tree or local-ahead divergence → exit-1 refusal with clear message; repo untouched.
- [ ] No new screen enum value introduced.
