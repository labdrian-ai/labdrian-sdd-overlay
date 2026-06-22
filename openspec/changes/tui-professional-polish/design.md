# Design: tui-professional-polish

> Tier 3, INCREMENTAL. No rearchitecture. Strict TDD (Go + `teatest`).
> Artifact language: English. TUI user-facing copy stays Spanish.

## Context

The TUI (`tui/`) is a thin Bubbletea front-end over the `bin/overlay` bash backend. It
follows the standard Elm/Bubbletea triad split across three files:

- `tui/model.go` — `model` state, `Init`/`Update`, per-screen update handlers.
- `tui/view.go` — `View`, lipgloss style constants, per-screen render functions.
- `tui/run.go` — domain types (`Target`, `Action`, `commandResult`), backend invocation, sync parsing.

This change is incremental polish over the existing five-screen surface (`screenTargets`,
`screenActions`, `screenConfirm`, `screenRunning`, `screenResult`). We preserve all current
idioms: value-receiver `model`, `(tea.Model, tea.Cmd)` returns, style constants in `view.go`,
backend isolation in `run.go`. We do NOT introduce `viewport.Model`, streaming, or new screens.

The change ships as two independently shippable slices (feature-branch-chain):

- **PR-1 — Visual + Robustness**: width-responsive rendering, spinner, distinct failure state,
  empty-verdict note, scroll clamp, select-all, footer copy, style cleanups.
- **PR-2 — Hooks Command Coverage**: `TargetAgnostic` + `ConfirmMessage` on `Action`, three
  hooks actions under a separator, no-`--target` routing.

## Goals / Non-Goals

### Goals
- Use `m.width` to constrain every rendered container so nothing overflows on narrow terminals.
- Make a running command visibly distinct from a hang (animated spinner).
- Make a failed command visibly distinct from a successful one (error title + red banner).
- Surface the hooks lifecycle as target-agnostic actions without a new screen.
- All behavior implemented test-first; existing four tests stay green.

### Non-Goals (OUT OF SCOPE — explicit)
- **Real-time output streaming** (subprocess pipe + channel rework) — deferred.
- **Split-pane / multi-pane layout** — deferred.
- **`viewport.Model` scroll refactor** — DEFERRED to optional slice-3. We keep the manual
  `m.scroll` + line-slicing model in `viewOutput` (view.go:241) and only add an upper clamp.
- **Parallel per-target execution columns** — deferred.
- **Persistent log / session history** — deferred.
- **`bootstrap` / `install-alias` in the TUI** — one-time setup, not operational.
- **Anglicizing Spanish UI copy** — all user-facing strings remain Spanish.

## Architecture Approach

Pattern stays **Elm Architecture (Bubbletea) with backend-isolation**. No layering change.
Each decision below is the smallest edit that satisfies a requirement while honoring the
existing idioms. Where a new capability is needed (spinner), we add a first-party Charm
component rather than hand-rolling animation.

---

## Decision 1 — Width-responsive rendering (R-001)

### Decision
Introduce a single helper `m.contentWidth() int` in `model.go` that returns `m.width` with an
`80` fallback, and feed it into lipgloss `.Width(...)` / `.MaxWidth(...)` at the **render
boundary** in `view.go`. Width is read where styles are applied, NOT baked into the style
constants (constants stay width-agnostic and reusable).

### Where
- **Fallback location**: a method on `model`, evaluated at render time:

  ```go
  // model.go
  func (m model) contentWidth() int {
      if m.width <= 0 {
          return 80
      }
      return m.width
  }
  ```

  We deliberately do NOT set `m.width = 80` inside `newModel()` (model.go:44). Reason: the real
  width arrives via `tea.WindowSizeMsg` (model.go:99) and storing a fake 80 there would make the
  model lie about whether a real size was received. The fallback is a pure read-time concern.

- **Application points** (all in `view.go`):
  - `View()` (view.go:60) — the composed output is the natural place to cap the outer frame.
    Wrap the final join with `lipgloss.NewStyle().MaxWidth(m.contentWidth()).Render(...)` so no
    screen can exceed the terminal width. `MaxWidth` (not `Width`) avoids padding short screens
    to full width and keeps the current compact look.
  - `header()` (view.go:82) and `footer()` (view.go:92) — apply `.Width(w)` to the rendered bar
    so the footer top-border rule and header background span exactly the terminal width. The
    footer border (`BorderTop`) currently renders to content width only; giving it `.Width(w)`
    makes the separator span the screen, which is the visible "finished" cue.
  - Output box in `viewOutput()` (view.go:272) — the `boxStyle` render gets `.Width(w-2)`
    (account for the 1-col padding each side) so long backend output wraps/clips inside the box
    instead of overflowing. `boxStyle` already has `Padding(0, 1)`.
  - Dashboard box in `viewDashboard()` (view.go:237) — same `.Width(w-2)` treatment.

### Constraints applied
| Container | Style | Constraint |
|---|---|---|
| Outer frame (`View`) | inline `MaxWidth` | `MaxWidth(w)` |
| Header / footer | `headerStyle` / `footerStyle` | `.Width(w)` at render |
| Output box | `outputBoxStyle` (new, see D8) | `.Width(w-2)` |
| Dashboard box | `boxStyle` | `.Width(w-2)` |

### Rejected alternatives
- **Bake width into style constants**: rejected — constants become stateful and non-reusable,
  and width is only known at render time.
- **Set `m.width = 80` in `newModel`**: rejected — masks "no WindowSizeMsg yet" and complicates
  tests that assert first-paint behavior.

### Test hook
`teatest.WithInitialTermSize(40, 20)` (narrow) → assert no rendered line exceeds 40 columns
(strip ANSI, measure rune width per line).

---

## Decision 2 — Spinner during screenRunning (R-002)

### Decision
Add `github.com/charmbracelet/bubbles/spinner` as a real dependency. Embed a `spinner.Model`
in `model`, tick it ONLY while on `screenRunning`, and render it in `viewRunning`.

### Where
- **Dependency**: add `github.com/charmbracelet/bubbles` to `tui/go.mod` `require` block
  (currently absent — only `bubbletea`, `lipgloss`, `teatest`). Run `go get
  github.com/charmbracelet/bubbles@latest` then `go mod tidy` to populate `go.sum`. The module
  is first-party Charm and version-aligned with `bubbletea v1.3.10` / `lipgloss v1.1.0`, so
  integration risk is low.

- **Model field** (`model.go:19` struct):
  ```go
  spinner spinner.Model
  ```
  Initialized in `newModel()` (model.go:53 return):
  ```go
  sp := spinner.New()
  sp.Spinner = spinner.Dot
  sp.Style = lipgloss.NewStyle().Foreground(colorAccent)
  // ...
  return model{ ..., spinner: sp }
  ```

- **Ticking — start on transition, not in `Init`**: `Init()` (model.go:63) stays `nil` (we are
  on `screenTargets` at start, no spinner needed). The spinner command is batched into the
  `tea.Cmd` returned when we ENTER `screenRunning`:
  - `updateActions` (model.go:177-178) non-mutating enter:
    ```go
    m.scr = screenRunning
    return m, tea.Batch(m.spinner.Tick, m.runActionCmd(action))
    ```
  - `updateConfirm` (model.go:186-187) confirmed enter: same `tea.Batch(m.spinner.Tick,
    m.runActionCmd(m.pendingAction))`.

- **Ticking — sustain only while running**: handle `spinner.TickMsg` in the top-level `Update`
  (model.go:97) BEFORE the `tea.KeyMsg` switch:
  ```go
  case spinner.TickMsg:
      if m.scr != screenRunning {
          return m, nil // drop ticks once we've left the running screen — stops the loop
      }
      var cmd tea.Cmd
      m.spinner, cmd = m.spinner.Update(msg)
      return m, cmd
  ```
  This is the key idiom: a spinner self-perpetuates by returning a new tick from `Update`. By
  refusing to forward the tick once `m.scr != screenRunning`, the tick loop dies naturally when
  the backend completes (the `runDoneMsg` handler at model.go:104 has already switched to
  `screenResult`). No timers to cancel.

- **Render** (`viewRunning`, view.go:186):
  ```go
  return titleStyle.Render(m.spinner.View() + " Ejecutando " + m.pendingAction.Name + "…")
  ```
  Spanish copy preserved.

### Rejected alternatives
- **Hand-rolled `tea.Tick` + frame index**: rejected — reinvents `bubbles/spinner`, more code,
  more test surface, for no benefit.
- **Always tick (start in `Init`)**: rejected — wastes a tick loop on screens that never show a
  spinner and muddies the "ticks only while running" invariant the test relies on.

### Test hook
teatest: enter a non-mutating action → assert `screenRunning` frame contains a spinner glyph
(`spinner.Dot` frames) alongside "Ejecutando".

---

## Decision 3 — TargetAgnostic actions + hooks coverage (R-008, R-009)

### Decision
Add `TargetAgnostic bool` to `Action` (run.go:31). When true: `runBackend` issues a single
invocation with NO `--target`, and the Update flow does NOT gate on target selection. Surface
three hooks actions under a decorative separator in the existing flat list.

### Where
- **Struct field** (run.go:31-36):
  ```go
  type Action struct {
      Name           string
      Command        string
      Mutating       bool
      SupportsAll    bool
      TargetAgnostic bool   // when true: invoke WITHOUT --target, skip target selection
      ConfirmMessage string // per-action confirm copy (see D5)
  }
  ```

- **`runBackend` routing** (run.go:219-232) — add a branch BEFORE the `SupportsAll`/per-target
  logic:
  ```go
  var argSets [][]string
  switch {
  case action.TargetAgnostic:
      argSets = [][]string{{action.Command}}        // no --target at all
  case action.SupportsAll && allSelected:
      argSets = [][]string{{action.Command, "--target", "all"}}
  default:
      for _, t := range selected {
          argSets = append(argSets, []string{action.Command, "--target", t.Name})
      }
  }
  ```
  Rationale: `status-hooks`/`install-hooks`/`uninstall-hooks` operate on `~/.claude/settings.json`
  globally; passing `--target` makes these bash subcommands error (proposal Risk: must be
  verified against the live engine during apply/verify).

- **Actions list** (`Actions()`, run.go:39-46) — append the three hooks actions. Display order
  keeps the four existing operational actions first, then the hooks group:
  ```go
  {Name: "Estado de hooks", Command: "status-hooks", Mutating: false, TargetAgnostic: true},
  {Name: "Instalar hooks", Command: "install-hooks", Mutating: true, TargetAgnostic: true,
      ConfirmMessage: "..."},   // see D5
  {Name: "Desinstalar hooks", Command: "uninstall-hooks", Mutating: true, TargetAgnostic: true,
      ConfirmMessage: "..."},
  ```
  Spanish labels. `status-hooks` is read-only (`Mutating: false`, no confirm). `install-hooks`
  and `uninstall-hooks` are mutating/destructive → routed through `screenConfirm` by the
  existing `action.Mutating` check (model.go:173).

- **Separator rendering** (`viewActions`, view.go:150 loop) — the separator is a **decorative
  row**, NOT an `Action` (keeping it out of the list avoids polluting cursor navigation and
  `m.actions[m.aCursor]` indexing). We detect the boundary between operational and hooks actions
  by `a.TargetAgnostic`: render a dim `─── Hooks ───` line immediately before the first
  `TargetAgnostic` action:
  ```go
  for i, a := range m.actions {
      if a.TargetAgnostic && (i == 0 || !m.actions[i-1].TargetAgnostic) {
          b.WriteString(dimStyle.Render("  ─── Hooks ───") + "\n")
      }
      // ... existing cursor/tag/name rendering ...
  }
  ```
  This keeps the separator purely visual and self-positioning (it appears once, before the first
  hooks entry).

- **Target-selection screen handling for TargetAgnostic** — the cursor still passes through
  `screenTargets` first (no new screen). For a `TargetAgnostic` action the selected targets are
  simply ignored by `runBackend` (it emits no `--target`). `m.selectedTargets()` is still called
  (model.go:71) but its result is unused for these actions. No special-casing needed in the
  navigation flow; target selection is decorative/no-op for these three actions. The existing
  `enter`-requires-`anySelected()` gate on `screenTargets` (model.go:148) still applies as a
  harmless pre-step. This is the minimal-change choice; we explicitly accept that the user picks
  (ignored) targets before reaching a hooks action.

### Rejected alternatives
- **Separator as a sentinel `Action`**: rejected — would require skip-logic in cursor movement
  (model.go:163-168) and `enter` handling (model.go:171) to avoid "selecting" the separator.
  More code, more bug surface. A decorative row is simpler.
- **New `screenHooks`**: rejected — proposal explicitly forbids a new screen; the flat list +
  separator is the agreed surface.
- **Skip `screenTargets` for TargetAgnostic actions**: rejected for this slice — would require
  re-entry routing from `screenActions` and complicate the linear screen flow; the no-op target
  pick is acceptable and strictly smaller.

### Test hooks
- Unit: `runBackend` with `Action{TargetAgnostic: true, Command: "status-hooks"}` → assert the
  emitted arg set is `["status-hooks"]` with no `--target`. (Extract arg construction into a
  testable helper `buildArgSets(action, selected, allSelected) [][]string` so it is unit-testable
  without executing the binary.)
- teatest: navigate to a hooks action → assert it is reachable and the separator row renders.

---

## Decision 4 — Error-state rendering (R-003)

### Decision
In `viewResult` (view.go:190), branch on `m.result.err != nil` to render the title with
`errStyle` and emit a red "Comando falló" banner above the output.

### Where
- `viewResult` (view.go:190-202):
  ```go
  func (m model) viewResult() string {
      var b strings.Builder
      if m.result.err != nil {
          b.WriteString(errStyle.Render("Resultado · " + m.result.action.Name))
          b.WriteString("\n")
          b.WriteString(lipgloss.NewStyle().
              Foreground(colorRed).Bold(true).
              Render("  ✗ Comando falló") + "\n")
      } else {
          b.WriteString(titleStyle.Render("Resultado · " + m.result.action.Name))
          b.WriteString("\n")
      }
      // ... existing dashboard + output ...
  }
  ```
  `errStyle` already exists (view.go:57: red bold). The banner is a distinct red bold line so a
  merge-conflict `apply` no longer reads as success. Spanish copy.

### Rejected alternatives
- **Reuse `warnBoxStyle` (yellow)**: rejected — failure must be red and unambiguously different
  from the yellow "modifica" warning semantics used elsewhere.

### Test hook
teatest with a mocked failing action (`result.err != nil`) → assert the result frame contains
"Comando falló". Achieved by constructing a `model` with a pre-set failing `commandResult` and
`scr: screenResult`, then rendering — no real backend needed.

---

## Decision 5 — Per-action ConfirmMessage (R-010)

### Decision
Add `ConfirmMessage string` to `Action` (see D3 struct). `viewConfirm` (view.go:172) uses it
when non-empty; falls back to the current generic copy when empty (backwards-safe for
`apply`/`capture`).

### Where
- `viewConfirm` (view.go:172-184):
  ```go
  detail := "Esta acción modifica los destinos."
  if m.pendingAction.ConfirmMessage != "" {
      detail = m.pendingAction.ConfirmMessage
  }
  msg := fmt.Sprintf(
      "Ejecutar %s\n\n%s %s",
      lipgloss.NewStyle().Bold(true).Foreground(colorYellow).Render(m.pendingAction.Name),
      lipgloss.NewStyle().Foreground(colorYellow).Bold(true).Render("Atención:"),
      detail,
  )
  ```
  Note: for `TargetAgnostic` actions the "en: <targets>" clause is dropped (targets are
  irrelevant). For target-bound actions we keep the destinations line. Implementation branches on
  `m.pendingAction.TargetAgnostic` to choose the header form. Existing `apply`/`capture` keep
  empty `ConfirmMessage` → unchanged generic copy.

- **`install-hooks` copy** (run.go `Actions()`): must mention `~/.claude/settings.json` and the
  `.bak` backup, e.g.:
  ```
  Modifica ~/.claude/settings.json (se crea un respaldo .bak antes de escribir).
  ```
- **`uninstall-hooks` copy**: e.g. `Elimina los hooks de ~/.claude/settings.json.`

### Rejected alternatives
- **Always require `ConfirmMessage`**: rejected — would force touching `apply`/`capture` entries
  and risk changing their proven wording. Empty→generic fallback is the backwards-safe choice.

### Test hook
Unit: `viewConfirm` with `pendingAction.ConfirmMessage == ""` contains the generic phrase; with
the `install-hooks` action contains `settings.json` and `.bak`.

---

## Decision 6 — Scroll clamp (R-005)

### Decision
Clamp `m.scroll` on the `down`/`j` path so it never exceeds `maxScroll = max(0, len(lines) -
viewport)`. The clamp lives where the bound is known.

### Where
The viewport math currently lives in `viewOutput` (view.go:245-251), and `m.scroll` is mutated
in `updateResult` (model.go:206-207) where the line count is NOT known. To clamp correctly we
extract the viewport/line computation into a model helper so both the view and the update path
share one source of truth:

- New helper (model.go) `func (m model) maxScroll() int` that reproduces the line-count and
  viewport reservation from `viewOutput` (view.go:242-251) and returns `max(0, len(lines) -
  viewport)`.
- `updateResult` down branch (model.go:206-207):
  ```go
  case "down", "j":
      if m.scroll < m.maxScroll() {
          m.scroll++
      }
  ```
  The up branch (model.go:203-204) already guards `> 0`. This stops the documented drift past
  content bounds. `viewOutput`'s existing defensive `start`/`end` clamps (view.go:253-263) remain
  as a belt-and-suspenders guard.

### Rejected alternatives
- **`viewport.Model` refactor**: explicitly OUT OF SCOPE (deferred slice-3). The manual clamp is
  the minimal fix.
- **Clamp inside `viewOutput` only**: insufficient — the proposal requires `m.scroll` state
  itself to stop drifting (R-005), so the clamp must be on the mutation, not just on render.

### Test hook
Unit: build a `model` with a known small output + height, drive repeated `down` keys via
`updateResult`, assert `m.scroll` saturates at `maxScroll()` and never exceeds it.

---

## Decision 7 — Select-all on screenTargets (R-006)

### Decision
Add `a` to `updateTargets` (model.go:132) as a select-all / deselect-all toggle.

### Where
- `updateTargets` switch (model.go:133), new case:
  ```go
  case "a":
      target := !m.allSelected()
      for i := range m.targets {
          m.selected[i] = target
      }
  ```
  Helper `func (m model) allSelected() bool` (model.go, alongside `anySelected` at model.go:88):
  returns true iff every index in `m.selected` is true. Toggle semantics: if all are currently
  selected → deselect all; otherwise → select all.

### Rejected alternatives
- **Separate `a` = all, `A` = none**: rejected — proposal specifies a single `a` toggle.

### Test hook
Unit: from default (all selected), `a` → none selected; `a` again → all selected.

---

## Decision 8 — Style + footer cleanups (R-007, plus consistency)

### Decision (budget-flexible — trim first if PR-1 nears 400 lines)
- **`outputBoxStyle` constant** (view.go vars block, ~view.go:47): extract the inline
  `boxStyle.BorderForeground(colorGray)` from `viewOutput` (view.go:272) into a named constant:
  ```go
  outputBoxStyle = lipgloss.NewStyle().
      Border(lipgloss.RoundedBorder()).
      BorderForeground(colorGray).
      Padding(0, 1)
  ```
  Render via `outputBoxStyle.Width(w-2).Render(shown)`.
- **Footer legend copy** (`footer`, view.go:103-104): result screen → `↑/↓ desplazar  ·
  esc/enter volver  ·  q salir`; confirm screen → `y confirmar  ·  esc/n cancelar`. Reflects the
  keys actually handled in `updateResult` (enter at model.go:199) and `updateConfirm` (esc at
  model.go:188).
- **Double-gap removal**: the `titleStyle` constant has `MarginBottom(1)` (view.go:44) yet
  several views append an explicit `"\n"` right after rendering it (view.go:112, 147, 193),
  producing a double vertical gap. Remove the redundant `"\n"` after `titleStyle.Render(...)`
  calls. (Verify visually; this is the lowest-priority cleanup.)
- **Empty-verdict note (R-004)**: in `viewResult`, when `m.result.action.Command == "sync-check"`
  and `len(m.result.verdicts) == 0`, emit `dimStyle.Render("No se pudieron analizar
  veredictos")` before the raw output.

### Trim rule
Per proposal Risk "400-line budget": if PR-1 approaches the budget, the double-gap removal and
shared-padding constant may be split to a follow-up WITHOUT affecting the robustness ACs
(R-001/R-002/R-003/R-005). The `outputBoxStyle` extraction and footer copy stay (cheap, tied to
R-007).

### Test hook
teatest: footer on result screen contains `esc/enter`; sync-check with zero verdicts contains
"No se pudieron analizar veredictos".

---

## Data Flow Summary

```
WindowSizeMsg ──► m.width ──► m.contentWidth() ──► .Width/.MaxWidth at render (D1)
enter(action) ──► screenRunning + tea.Batch(spinner.Tick, runActionCmd) (D2)
spinner.TickMsg ─► forwarded only while scr==screenRunning, self-perpetuates (D2)
runDoneMsg ────► m.result, scr=screenResult (kills tick loop) (D2)
runActionCmd ──► runBackend ──► buildArgSets switch: TargetAgnostic|SupportsAll|per-target (D3)
viewResult ────► err!=nil → errStyle title + red banner (D4)
                 sync-check & 0 verdicts → dim note (D8/R-004)
updateConfirm ─► viewConfirm uses ConfirmMessage || generic (D5)
updateResult ──► down clamped to m.maxScroll() (D6)
updateTargets ─► "a" toggles all via m.allSelected() (D7)
```

## Integration Points
- **External dependency added**: `github.com/charmbracelet/bubbles` (spinner). `go.mod` +
  `go.sum` updated via `go get` + `go mod tidy`. Only first-party Charm transitive deps.
- **Backend contract**: relies on `status-hooks`/`install-hooks`/`uninstall-hooks` accepting NO
  `--target`. Verified during apply/verify against live `bin/overlay`.
- **No engine changes.** TUI only invokes existing subcommands.

## Test Strategy (strict TDD)

Existing harness: `tui/main_test.go` mixes `teatest` flow tests (NewTestModel + WaitFor) with
direct unit tests on update handlers (`m.updateTargets(...)`) and pure functions
(`ParseSyncCheck`, `classify`). New tests follow the SAME file/approach (extend `main_test.go`
or add focused `*_test.go` in package `main`). All four existing tests MUST stay green.

| Behavior | Test type | Approach |
|---|---|---|
| Narrow-width no overflow (R-001) | teatest | `WithInitialTermSize(40,20)`, strip ANSI, assert max line width ≤ 40 |
| Spinner present on running (R-002) | teatest | enter non-mutating action, assert spinner glyph + "Ejecutando" |
| Error banner (R-003) | unit/teatest | pre-set failing `commandResult`, `scr=screenResult`, assert "Comando falló" |
| Empty-verdict note (R-004) | unit | sync-check action, zero verdicts → assert dim note string |
| Scroll clamp (R-005) | unit | drive `down` keys via `updateResult`, assert `scroll ≤ maxScroll()` |
| Select-all (R-006) | unit | `a` from default → none; `a` again → all |
| Footer copy (R-007) | unit/teatest | assert `esc/enter` on result, `esc/n` on confirm |
| No `--target` for TargetAgnostic (R-008) | unit | `buildArgSets` returns `[[command]]` |
| Hooks reachable + separator (R-009) | teatest | navigate to hooks action, assert separator row |
| ConfirmMessage selection (R-010) | unit | generic fallback when empty; `settings.json`+`.bak` for install |

Helper extraction for testability:
- `buildArgSets(action, selected, allSelected) [][]string` — pure, unit-testable arg
  construction (lifted out of `runBackend`, run.go:225-232).
- `m.maxScroll() int` — pure clamp bound, unit-testable.
- `m.allSelected() bool` — pure, unit-testable.
- `m.contentWidth() int` — pure, unit-testable.

## Risks
- **New dependency (`bubbles`)**: low — first-party Charm, version-aligned. Must update `go.sum`;
  CI/build must run `go mod tidy`.
- **TargetAgnostic backend contract**: hooks subcommands erroring on `--target` is assumed, not
  yet verified against the live engine. Verify in apply/verify before merge.
- **Width consistency**: if `.Width`/`.MaxWidth` is applied unevenly, narrow terminals could
  still clip mid-cleanup. The narrow-width teatest exercises this, not just visual inspection.
- **Double-gap cleanup**: cosmetic and visual-verify dependent; lowest priority, first to trim
  under budget pressure.
- **maxScroll duplication**: the viewport math is duplicated between `viewOutput` and the new
  `maxScroll()` helper. Acceptable for this incremental slice; the `viewport.Model` refactor
  (slice-3) would collapse it.

## ADR-style Decision Log
| # | Decision | Chosen | Rejected |
|---|---|---|---|
| 1 | Width fallback | `contentWidth()` read-time helper, `MaxWidth` outer | bake into constants; `m.width=80` in newModel |
| 2 | Spinner | `bubbles/spinner`, tick only while running via batch | hand-rolled tick; always-tick from Init |
| 3 | Hooks routing | `TargetAgnostic` field + arg-set switch; decorative separator | sentinel Action; new screen; skip targets |
| 4 | Error state | `errStyle` title + red banner in viewResult | reuse yellow warnBoxStyle |
| 5 | Confirm copy | per-action `ConfirmMessage`, empty→generic | always-required message |
| 6 | Scroll clamp | clamp mutation against `maxScroll()` helper | viewport.Model; render-only clamp |
| 7 | Select-all | single `a` toggle via `allSelected()` | separate a/A keys |
| 8 | Style cleanup | `outputBoxStyle` const + footer copy (trimmable extras) | leave inline override |
