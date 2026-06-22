# Tasks: tui-professional-polish

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | PR-1: ~280–340 lines; PR-2: ~130–180 lines; Total: ~410–520 lines |
| 400-line budget risk | Medium (each PR is under 400; total across both PRs fits the chain) |
| Chained PRs recommended | Yes |
| Suggested split | PR-1 (tui-polish) → PR-2 (tui-hooks-commands), both targeting feature/tui-professional-polish tracker |
| Delivery strategy | feature-branch-chain |
| Chain strategy | feature-branch-chain |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: Medium

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Visual polish + robustness (R-001–R-007, R-S1–R-S3) | PR-1 | Base = feature/tui-professional-polish tracker branch |
| 2 | Hooks command coverage (R-008–R-010) | PR-2 | Base = PR-1 branch; depends on PR-1 struct changes |

---

## PR-1: Visual + Robustness (tui-polish)

> Base branch: `feature/tui-professional-polish` (tracker)
> Test command: `cd tui && go test ./...`

### Phase 1 — Dependency + Style Constants (Foundation)

- [ ] 1.1 **[RED R-002]** Write failing test `TestSpinnerPresentOnRunning` in `tui/main_test.go`: advance model into `screenRunning`, assert spinner glyph present in rendered output. Run `go test ./...` → must fail (import missing).
- [ ] 1.2 **[GREEN R-002]** Add `github.com/charmbracelet/bubbles` to `tui/go.mod` and run `go mod tidy` to update `go.sum`. Confirm `go build ./...` passes.
- [ ] 1.3 **[RED R-S1]** Write compile-gate test `TestOutputBoxStyleDeclared` in `tui/main_test.go` asserting `outputBoxStyle` is a non-zero lipgloss.Style (reachable from the package). Run → fail.
- [ ] 1.4 **[GREEN R-S1]** Add `outputBoxStyle` named constant to `tui/view.go` style vars block; remove inline `.BorderForeground(colorGray)` call from `viewOutput`. Run test → green.
- [ ] 1.5 **[RED R-S3]** Write test `TestNoDoublGapInAnyScreen` asserting `!strings.Contains(rendered, "\n\n\n")` for home + result screens. Run → fail.
- [ ] 1.6 **[GREEN R-S2, R-S3]** Remove redundant explicit `"\n"` after `titleStyle.Render(...)` calls in `tui/view.go` (view.go:112, 147, 193); extract shared padding/indent constant. Run tests → green.

### Phase 2 — Core Behavior (model.go + view.go)

- [ ] 2.1 **[RED R-001]** Write test `TestWidthResponsiveRendering` using `teatest.WithInitialTermSize(40, 20)`: strip ANSI, assert every rendered line `<= 40` runes wide. Run → fail.
- [ ] 2.2 **[GREEN R-001]** Add `func (m model) contentWidth() int` to `tui/model.go` (returns `m.width` or 80 fallback). Apply `.Width(w)` to header/footer in `tui/view.go`, `.MaxWidth(w)` to outer `View()`, `.Width(w-2)` to output box and dashboard box. Run test → green.
- [ ] 2.3 **[GREEN R-002]** Embed `spinner.Model` in `model` struct (`tui/model.go`); initialize in `newModel()`; batch `m.spinner.Tick` with `runActionCmd` on `screenRunning` entry in `updateActions` and `updateConfirm`; handle `spinner.TickMsg` in top-level `Update` (drop ticks when `scr != screenRunning`); render `m.spinner.View()` in `viewRunning` (`tui/view.go`). Run `TestSpinnerPresentOnRunning` → green.
- [ ] 2.4 **[RED R-005]** Write unit test `TestScrollClamp` in `tui/main_test.go`: build a model with short output + small height, drive repeated `down` key messages via `updateResult`, assert `m.scroll` never exceeds `m.maxScroll()` and never goes negative. Run → fail.
- [ ] 2.5 **[GREEN R-005]** Add `func (m model) maxScroll() int` helper to `tui/model.go`; clamp down-key branch in `updateResult` against `m.maxScroll()`. Run `TestScrollClamp` → green.
- [ ] 2.6 **[RED R-006]** Write unit test `TestSelectAllToggle` in `tui/main_test.go`: from all-selected state, press `a` → none; press `a` again → all; press `a` on `screenHome` → no change. Run → fail.
- [ ] 2.7 **[GREEN R-006]** Add `func (m model) allSelected() bool` to `tui/model.go`; add `case "a"` to `updateTargets` switch with toggle logic. Run `TestSelectAllToggle` → green.
- [ ] 2.8 **[RED R-003]** Write test `TestErrorBannerOnFailure` in `tui/main_test.go`: construct model with `scr: screenResult, result.err != nil`, render, assert "Comando falló" present; also assert success path does NOT contain banner. Run → fail.
- [ ] 2.9 **[GREEN R-003]** Branch `viewResult` in `tui/view.go` on `m.result.err != nil`: render title with `errStyle` + red "Comando falló" banner. Run `TestErrorBannerOnFailure` → green.
- [ ] 2.10 **[RED R-004]** Write unit test `TestEmptyVerdictNote` in `tui/main_test.go`: model with `action.Command == "sync-check"`, zero verdicts → rendered result contains "No se pudieron analizar veredictos"; non-empty verdicts → note absent. Run → fail.
- [ ] 2.11 **[GREEN R-004]** Add empty-verdict note branch in `viewResult` (`tui/view.go`): when sync-check and `len(m.result.verdicts) == 0`, emit `dimStyle.Render("No se pudieron analizar veredictos")`. Run → green.
- [ ] 2.12 **[RED R-007]** Write test `TestFooterLegendCorrectness` in `tui/main_test.go`: assert result screen footer contains `esc/enter`; confirm screen footer contains `esc` and `enter`. Run → fail.
- [ ] 2.13 **[GREEN R-007]** Update `footer()` in `tui/view.go` to return correct copy per screen: result → `esc/enter volver`; confirm → `y confirmar · esc/n cancelar`. Run `TestFooterLegendCorrectness` → green.

### Phase 3 — PR-1 Verification Gate

- [ ] 3.1 **[R-011]** Run `cd tui && go test ./...`: confirm all pre-existing tests green and all new tests (2.1–2.13) green. Zero failures.
- [ ] 3.2 **[R-011]** Run `go vet ./...` and `go build ./...` from `tui/`: confirm no errors (validates R-S1, R-S3 compile gates).
- [ ] 3.3 **[R-012]** Review all string literals added in `tui/view.go` and `tui/model.go` in the PR-1 diff: confirm no Spanish string has been anglicized (banner, footer, notes stay in Spanish).

---

## PR-2: Hooks Command Coverage (tui-hooks-commands)

> Base branch: PR-1 branch (feature-branch-chain; diff targets PR-1 branch only)
> Test command: `cd tui && go test ./...`

### Phase 4 — Data Model Extension (run.go)

- [ ] 4.1 **[RED R-008]** Write unit test `TestBuildArgSets` in `tui/main_test.go` (or `tui/run_test.go`): assert `buildArgSets(Action{TargetAgnostic: true, Command: "status-hooks"}, nil, false)` returns `[["status-hooks"]]` with no `--target`; assert non-TargetAgnostic action with one target returns `[["apply", "--target", "agent-x"]]`. Run → fail (function does not exist yet).
- [ ] 4.2 **[GREEN R-008]** Extract `buildArgSets(action Action, selected []Target, allSelected bool) [][]string` pure function from `runBackend` in `tui/run.go`; add `TargetAgnostic bool` and `ConfirmMessage string` fields to `Action` struct in `tui/run.go` with gofmt realignment; add `TargetAgnostic` branch as first case in the switch. Run `TestBuildArgSets` → green.
- [ ] 4.3 **[RED R-009]** Write unit test `TestHooksActionsRegistered` in `tui/main_test.go`: inspect `Actions()` return; assert "status-hooks", "install-hooks", "uninstall-hooks" all present with `TargetAgnostic: true`; assert `status-hooks` has `Mutating: false`; assert install/uninstall have `Mutating: true`. Run → fail.
- [ ] 4.4 **[GREEN R-009]** Append three hooks `Action` entries to `Actions()` in `tui/run.go` with Spanish display names, correct `Mutating`/`TargetAgnostic` values, and `ConfirmMessage` copy for install/uninstall (Spanish, mentioning `settings.json` and `.bak`). Run `TestHooksActionsRegistered` → green.
- [ ] 4.5 **[VERIFY R-008 assumption]** Manually test `bin/overlay status-hooks --target some-target` against the live engine: confirm it errors or rejects `--target`. Document the observed behavior as a comment in `tui/run.go` above the `TargetAgnostic` branch. (This is a verification step, not a code change; if the engine accepts `--target` silently, note it as a risk in the PR description.)

### Phase 5 — Routing + Rendering (model.go + view.go)

- [ ] 5.1 **[RED R-010]** Write unit test `TestConfirmMessageSelection` in `tui/main_test.go`: model with `pendingAction.ConfirmMessage == ""` on screenConfirm → rendered output contains the generic phrase "Esta acción modifica los destinos"; model with install-hooks action → rendered output contains "settings.json" and ".bak". Run → fail.
- [ ] 5.2 **[GREEN R-010]** Update `viewConfirm` in `tui/view.go` to branch on `m.pendingAction.ConfirmMessage != ""` → use custom copy; else → existing generic copy. For `TargetAgnostic` actions, omit the "en: <targets>" line. Run `TestConfirmMessageSelection` → green.
- [ ] 5.3 **[RED R-009]** Write teatest `TestHooksSeparatorVisible` in `tui/main_test.go`: drive model to screenHome, render, assert output contains "─── Hooks ───". Run → fail.
- [ ] 5.4 **[GREEN R-009]** Add separator rendering to `viewActions` loop in `tui/view.go`: emit `dimStyle.Render("  ─── Hooks ───")` immediately before the first `TargetAgnostic` action, using the boundary-detection logic from the design. Run `TestHooksSeparatorVisible` → green.
- [ ] 5.5 **[RED R-009]** Write teatest `TestStatusHooksSkipsConfirm` in `tui/main_test.go`: navigate to "status-hooks" action, press enter, assert model goes to `screenRunning` directly (not `screenConfirm`). Also write `TestInstallHooksRequiresConfirm`: navigate to "install-hooks", press enter, assert `screenConfirm`. Run → fail (or verify existing `Mutating` routing already handles this — confirm either way).
- [ ] 5.6 **[GREEN R-009]** Confirm routing: `status-hooks` has `Mutating: false` → existing `updateActions` path already skips confirm (model.go:173). `install-hooks` / `uninstall-hooks` have `Mutating: true` → existing path routes to confirm. If any gap exists, patch `tui/model.go`. Run `TestStatusHooksSkipsConfirm` and `TestInstallHooksRequiresConfirm` → green.

### Phase 6 — PR-2 Verification Gate

- [ ] 6.1 **[R-011]** Run `cd tui && go test ./...`: confirm all pre-existing + PR-1 + PR-2 tests green. Zero failures.
- [ ] 6.2 **[R-011]** Run `go vet ./...` and `go build ./...` from `tui/`: no errors.
- [ ] 6.3 **[R-012]** Review all string literals added in PR-2 diff: hooks display names and confirm copy are in Spanish; `Cmd` fields ("status-hooks" etc.) are CLI names and may remain English.
- [ ] 6.4 **[Backwards compat]** Assert in test or review that `apply` and `capture` `Action` entries have empty `ConfirmMessage` and `TargetAgnostic: false` — zero-value defaults preserve prior behavior.
