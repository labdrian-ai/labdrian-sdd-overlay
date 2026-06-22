# Proposal: tui-professional-polish

## Why

The Bubbletea TUI in `tui/` is functional but feels unfinished in ways that show up the moment it leaves a wide, fast terminal:

- It captures terminal width (`m.width` via `tea.WindowSizeMsg`) but never uses it to constrain any rendered string, so the header, footer, and output box overflow on narrow terminals.
- Slow operations (`apply`, `capture` run git merges and file copies) render a single static "Ejecutando…" line with no spinner, so a working command is indistinguishable from a hang.
- A backend command that FAILS renders identically to one that succeeds — same white title, no error banner — so a merge conflict during `apply` looks like success at a glance.
- The TUI exposes only 4 of the engine's operational commands. The hooks lifecycle (`status-hooks`, `install-hooks`, `uninstall-hooks`) — which mutates `~/.claude/settings.json` — is reachable only from the raw CLI.
- Small correctness and consistency gaps: `m.scroll` has no upper clamp (state drifts past content), double vertical gaps from explicit `\n` after `MarginBottom` titles, an inline border override that breaks the style-constant pattern, no select-all on the targets screen, and footer legend copy that omits working keys.

These are not cosmetic-only: the width overflow and the indistinguishable failure state are genuine usability/robustness defects. This change is incremental polish across all four dimensions (visual, command coverage, UX, robustness) on the existing five-screen surface. It is explicitly NOT a rearchitecture.

## What Changes

Two phased, independently shippable slices (feature-branch-chain, each < 400 lines):

**PR-1 — Visual + Robustness**
- Width-responsive rendering: apply `lipgloss` width constraints to header, footer, and body/output containers, driven by `m.width` with a `width == 0 → 80` fallback guard so first-paint (before the first `WindowSizeMsg`) stays sane.
- Add a spinner during `screenRunning` using `github.com/charmbracelet/bubbles/spinner` (NEW dependency — see Risks).
- Distinct failure state: when `result.err != nil`, render the result title in `errStyle` plus a red "Comando falló" banner.
- Empty-verdict feedback: when `sync-check` produces zero parsed verdicts, show a dim "No se pudieron analizar veredictos" note instead of bare raw output.
- Clamp `m.scroll` to its computed `maxScroll` upper bound so scroll state stops drifting.
- Add `a` = select-all / deselect-all toggle on `screenTargets`.
- Style/footer cleanups: extract an `outputBoxStyle` constant (remove the inline `BorderForeground` override), remove double `\n`/`MarginBottom` gaps, replace hardcoded `fmt` padding/indents with a shared width/indent constant, and fix footer legend copy (`esc/enter` on result, `esc cancelar` on confirm).

**PR-2 — Hooks Command Coverage**
- Add a `TargetAgnostic bool` field to the `Action` struct. When true, `runBackend` invokes the subcommand WITHOUT `--target` (passing `--target` makes these bash commands error) and skips the target-selection requirement.
- Add three actions to the existing flat list, under a dim `─── Hooks ───` separator row (no new screen): `status-hooks` (read-only, no confirmation), `install-hooks` (mutating — confirmation copy mentioning `~/.claude/settings.json` and the `.bak` backup), `uninstall-hooks` (destructive — confirmation).
- Add a per-action `ConfirmMessage` field on `Action` with a backwards-safe empty → generic fallback so existing mutating actions keep their current copy.

All implementation is **test-first** (strict TDD applies — see Test Strategy).

## Capabilities

> Contract for the sdd-spec phase. No existing specs under `openspec/specs/`.

### New Capabilities
- `tui-polish`: visual/UX/robustness behavior of the TUI — width-responsive layout (R-001), spinner feedback (R-002), distinct success/failure result state (R-003), empty-verdict fallback note (R-004), scroll clamping (R-005), select-all (R-006), footer legend copy (R-007). PR-1.
- `tui-hooks-commands`: surfacing the hooks lifecycle (`status-hooks`/`install-hooks`/`uninstall-hooks`) as target-agnostic actions with `TargetAgnostic` routing (R-008), separator grouping (R-009), and per-action confirm copy (R-010). PR-2.

### Modified Capabilities
- None.

## Requirements

- **R-001** — The TUI MUST constrain rendered width to the terminal width (`m.width`), and MUST fall back to a width of 80 when `m.width == 0` (before the first `WindowSizeMsg`), so no screen overflows on narrow terminals.
- **R-002** — During `screenRunning`, the TUI MUST display an animated spinner alongside the "Ejecutando…" label.
- **R-003** — When a backend command fails (`result.err != nil`), the result screen MUST render the title with `errStyle` and display a distinct red failure banner, visually different from a successful result.
- **R-004** — When `sync-check` returns zero parsed verdicts, the result screen MUST display a dim "No se pudieron analizar veredictos" note rather than only raw output.
- **R-005** — `m.scroll` MUST be clamped to its computed maximum so scroll position never advances past the content bounds.
- **R-006** — On `screenTargets`, pressing `a` MUST toggle all targets between fully selected and fully deselected.
- **R-007** — The footer legend MUST reflect actually-handled keys: `esc/enter` to go back on the result screen, and `esc` as cancel on the confirm screen.
- **R-008** — The `Action` struct MUST support a `TargetAgnostic bool` field; when true, `runBackend` MUST invoke the subcommand WITHOUT a `--target` argument and MUST NOT require target selection.
- **R-009** — The TUI MUST expose `status-hooks` (read-only, no confirmation), `install-hooks` (mutating), and `uninstall-hooks` (destructive) as `TargetAgnostic` actions, presented under a dim `─── Hooks ───` separator in the existing actions list (no new screen).
- **R-010** — The `Action` struct MUST support a per-action `ConfirmMessage` field; the confirm screen MUST use it when non-empty and fall back to the existing generic message when empty. `install-hooks` MUST use confirmation copy mentioning `~/.claude/settings.json` and the `.bak` backup.
- **R-011** — All new and modified behavior MUST be covered by tests written test-first (teatest for flows, unit tests for model/`Action` helpers), and all existing tests MUST remain green.
- **R-012** — Spanish user-facing copy MUST remain Spanish; no UI strings are anglicized by this change.

## Impact

- **Affected files**: `tui/model.go` (spinner state, scroll clamp, select-all, target-agnostic routing in Update), `tui/view.go` (width constraints, error banner, empty-verdict note, separator, per-action confirm, style/footer cleanups), `tui/run.go` (`Action` struct fields `TargetAgnostic`/`ConfirmMessage`, `Actions()` entries, `runBackend` no-`--target` path), `tui/main_test.go` (and/or new test files).
- **Dependencies**: PR-1 adds `github.com/charmbracelet/bubbles` to `tui/go.mod` (NOT currently present — only `bubbletea`, `lipgloss`, and `teatest` are). `go.sum` updates accordingly.
- **Backend / engine**: no changes. The TUI only invokes existing engine subcommands.
- **Users**: terminal users gain the hooks lifecycle in the TUI, a non-deceptive failure state, a spinner on slow ops, and correct rendering on narrow terminals. No breaking changes to existing flows.

## Scope

### In scope
- Width-responsive rendering driven by `m.width` with an 80-wide fallback.
- Spinner during `screenRunning`.
- Distinct failure-state styling + banner.
- Empty-verdict fallback note for `sync-check`.
- `m.scroll` upper-bound clamp.
- Select-all/deselect-all (`a`) on targets.
- Footer legend copy fixes.
- Style cleanups: `outputBoxStyle` constant, double-gap removal, shared padding/indent constant.
- `TargetAgnostic` + `ConfirmMessage` fields on `Action`.
- `status-hooks`, `install-hooks`, `uninstall-hooks` in the actions list under a `─── Hooks ───` separator, with per-action confirm copy.

### Out of scope (non-goals)
- Real-time streaming of backend output (requires subprocess pipe + channel rework).
- Split-pane / multi-pane layout.
- Parallel per-target execution columns.
- Persistent log / session history.
- `bootstrap` and `install-alias` in the TUI (one-time setup tools, not operational).
- The `viewport.Model` scroll refactor — DEFERRED as an optional future slice-3 candidate, NOT part of this change.
- Anglicizing any Spanish UI copy.

## Test Strategy (strict TDD — applies here)

Unlike a docs-only change, this is testable Go code with a real harness (`teatest` + unit tests), so strict TDD applies: every behavior is implemented test-first.
- **Unit tests**: `Action{TargetAgnostic: true}` routing (no `--target` emitted), `ConfirmMessage` selection vs. generic fallback, scroll-clamp helper, select-all toggle on the model.
- **teatest flow tests**: error-state rendering on a mocked failing command (`result.err != nil` → error banner), spinner present on `screenRunning`, hooks actions reachable and invoking the backend without `--target`.
- All four existing tests must stay green.

## Acceptance Criteria

- **AC-1** — The TUI renders without overflow at narrow terminal widths; rendering uses `m.width` with an 80-wide fallback when width is unset. (R-001)
- **AC-2** — A spinner animates during `screenRunning`. (R-002)
- **AC-3** — A failed backend command is visually distinct: error-styled title plus a red failure banner. (R-003)
- **AC-4** — `sync-check` with zero verdicts shows a dim "No se pudieron analizar veredictos" note. (R-004)
- **AC-5** — Scroll position does not drift past content bounds. (R-005)
- **AC-6** — `a` toggles all targets selected/deselected on the targets screen. (R-006)
- **AC-7** — `status-hooks`, `install-hooks`, and `uninstall-hooks` are invokable from the TUI; `TargetAgnostic` actions invoke the backend WITHOUT `--target`; `install-hooks` shows confirmation copy mentioning `~/.claude/settings.json` and the `.bak` backup. (R-008, R-009, R-010)
- **AC-8** — All new behavior is covered by test-first teatest/unit tests; existing tests remain green. (R-011)
- **AC-9** — All Spanish UI copy is preserved. (R-012)

## Risks

- **New dependency**: `github.com/charmbracelet/bubbles` is NOT currently in `tui/go.mod` (the exploration assumed it might be — it is not). PR-1 must add it and update `go.sum`. Mitigation: it is a first-party Charm module already aligned with the existing `bubbletea v1.3.10` / `lipgloss v1.1.0` versions; low integration risk, but a deliberate dependency addition rather than a free helper.
- **TargetAgnostic backend contract**: the assumption that `status-hooks`/`install-hooks`/`uninstall-hooks` ERROR when passed `--target` must hold; the `runBackend` no-`--target` path depends on it. To verify during apply/verify against the actual engine command behavior.
- **Width fallback edge**: if `MaxWidth`/`Width` are applied inconsistently across containers, narrow terminals could still clip mid-cleanup. Width behavior must be exercised by tests at a narrow width, not only visually.
- **Confirm-copy backwards safety**: the empty → generic `ConfirmMessage` fallback must preserve existing `apply`/`capture` confirmation wording so current flows are unchanged.
- **400-line budget**: PR-1 carries the most surface (width + spinner + styling). If it approaches the budget, the style-cleanup sub-items (double-gap, constants) can be trimmed to a follow-up without affecting the robustness ACs.
