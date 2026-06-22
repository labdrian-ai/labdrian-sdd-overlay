# Spec: tui-polish

**Change**: tui-professional-polish  
**Capability**: tui-polish  
**PR slice**: PR-1 (Visual + Robustness)  
**Strict TDD**: YES — teatest + unit; `cd tui && go test ./...`

---

## Scope

Covers width-responsive layout, spinner feedback, distinct failure state, empty-verdict fallback, scroll clamping, select-all toggle, footer legend correctness, and style constant consolidation. No new screens. No backend changes.

---

## Requirement Table

| ID    | Category   | SHALL statement |
|-------|------------|-----------------|
| R-001 | Visual     | The TUI SHALL constrain all rendered containers (header, footer, body/output) to `m.width` characters wide, and SHALL use a fallback width of 80 when `m.width == 0`. |
| R-002 | Visual     | During `screenRunning`, the TUI SHALL display an animated spinner component alongside the "Ejecutando…" label. |
| R-003 | Robustness | When `result.err != nil`, the result screen SHALL render the command title with `errStyle` and SHALL display a visually distinct red failure banner; this state SHALL be visually different from a successful result. |
| R-004 | Robustness | When the `sync-check` command produces zero parsed verdicts, the result screen SHALL display a dim "No se pudieron analizar veredictos" note in addition to (or in place of) bare raw output. |
| R-005 | Robustness | `m.scroll` SHALL be clamped to `max(0, totalLines - viewportLines)` so scroll position cannot exceed content bounds. |
| R-006 | UX         | On `screenTargets`, pressing the `a` key SHALL toggle all targets: if any target is deselected, all become selected; if all are already selected, all become deselected. |
| R-007 | UX         | The footer legend SHALL list only keys that are actually handled: on the result screen it SHALL show `esc/enter` to go back; on the confirm screen it SHALL show `esc` as cancel. |
| R-S1  | Style      | A named `outputBoxStyle` constant SHALL replace any inline `BorderForeground` override in `view.go`, maintaining the style-constant pattern. |
| R-S2  | Style      | Double vertical gaps (explicit `\n` combined with `MarginBottom`) SHALL be eliminated; a single gap source SHALL remain per section boundary. |
| R-S3  | Style      | Padding and indent values that are currently hardcoded in `fmt` calls SHALL be consolidated into a shared width/indent constant. |
| R-011 | Testing    | All new and modified behavior in PR-1 SHALL be covered by tests written test-first; all four existing tests SHALL remain green. |
| R-012 | I18n       | All Spanish user-facing copy (labels, banners, footer, notes) SHALL remain in Spanish; no UI string SHALL be anglicized. |

---

## Scenarios

### R-001 — Width-responsive rendering

**Scenario 1.1 — Narrow terminal clamps output**
```
Given   the terminal reports a width of 40 columns (WindowSizeMsg{Width: 40})
When    any screen is rendered (home, targets, running, result, confirm)
Then    no rendered string in the View output exceeds 40 characters per line
```

**Scenario 1.2 — Zero-width fallback**
```
Given   m.width is 0 (no WindowSizeMsg received yet)
When    View() is called
Then    all containers are rendered at exactly 80 columns wide (fallback guard)
```

**Test note**: Use `teatest.NewTestModel` with `tea.WindowSize(40, 24)` in a model initialization helper; assert `len(line) <= 40` for each line in the output.

---

### R-002 — Spinner on screenRunning

**Scenario 2.1 — Spinner visible during running state**
```
Given   the model is in screenRunning (a command is executing)
When    View() is called after at least one spinner Tick message
Then    the View output contains a spinner glyph alongside the "Ejecutando…" label
```

**Scenario 2.2 — Spinner not visible on other screens**
```
Given   the model is on screenHome
When    View() is called
Then    the View output does NOT contain a spinner glyph
```

**Test note**: Unit-test: advance model with `spinner.TickMsg`; assert spinner string is non-empty in the running view. The spinner component is `github.com/charmbracelet/bubbles/spinner`.

---

### R-003 — Distinct failure state

**Scenario 3.1 — Error banner rendered on failure**
```
Given   the model transitions to screenResult with result.err != nil
When    View() is called
Then    the View output contains a red failure banner (e.g. "Comando falló")
  AND   the command title is rendered in errStyle (not the default title style)
```

**Scenario 3.2 — Success state unchanged**
```
Given   the model transitions to screenResult with result.err == nil
When    View() is called
Then    the View output does NOT contain a failure banner
  AND   the command title is rendered in the default (non-error) title style
```

**Test note**: Use a `testBackendFunc` that returns a non-nil error; drive model through Update to screenResult; inspect rendered output string for the banner text. Also assert the errStyle ANSI prefix differs from the normal title prefix.

---

### R-004 — Empty-verdict fallback for sync-check

**Scenario 4.1 — Dim note shown when verdicts are empty**
```
Given   the active action is "sync-check"
  AND   the backend returns output that yields zero parsed verdicts
When    the model transitions to screenResult and View() is called
Then    the View output contains the dim note "No se pudieron analizar veredictos"
```

**Scenario 4.2 — Note absent when verdicts exist**
```
Given   the active action is "sync-check"
  AND   the backend returns output that yields at least one parsed verdict
When    View() is called on screenResult
Then    the dim note is NOT present in the View output
```

**Test note**: Unit-test the verdict-count predicate in isolation; teatest drives the full flow with a stubbed output string.

---

### R-005 — Scroll clamping

**Scenario 5.1 — Scroll does not exceed max**
```
Given   the result output has N content lines
  AND   the viewport shows V lines
When    the model receives more than (N - V) scroll-down key messages
Then    m.scroll equals max(0, N - V) and does not exceed it
```

**Scenario 5.2 — Scroll lower-bound**
```
Given   m.scroll is at some positive value
When    the model receives more scroll-up messages than the current scroll value
Then    m.scroll equals 0 and does not go negative
```

**Test note**: Unit-test the clamp helper `clampScroll(scroll, total, viewport int) int` directly; table-drive with negative, zero, mid, and over-max inputs.

---

### R-006 — Select-all on targets screen

**Scenario 6.1 — All deselected → all selected**
```
Given   the model is on screenTargets with at least one target deselected
When    the user presses 'a'
Then    all targets in m.selected are true
```

**Scenario 6.2 — All selected → all deselected**
```
Given   the model is on screenTargets with all targets selected
When    the user presses 'a'
Then    all targets in m.selected are false
```

**Scenario 6.3 — 'a' key is a no-op on other screens**
```
Given   the model is on screenHome
When    the user presses 'a'
Then    model state is unchanged (no panic, no screen transition)
```

**Test note**: Unit-test the toggle logic on the model struct directly; pass `tea.KeyMsg` with `Type: tea.KeyRunes, Runes: []rune{'a'}`.

---

### R-007 — Footer legend correctness

**Scenario 7.1 — Result screen footer**
```
Given   the model is on screenResult
When    View() is called
Then    the footer contains "esc/enter" (go back) and does NOT contain "q salir" as the only quit instruction
```

**Scenario 7.2 — Confirm screen footer**
```
Given   the model is on screenConfirm
When    View() is called
Then    the footer contains "esc" labeled as cancel/cancelar
  AND   the footer contains "enter" labeled as confirm
```

**Test note**: String-match the footer section of the rendered output.

---

### R-S1 — outputBoxStyle constant

**Scenario S1.1 — No inline BorderForeground override**
```
Given   the source code of view.go
When    it is compiled and its style declarations are inspected
Then    no inline .BorderForeground() call appears outside of a named style constant
  AND   a constant named outputBoxStyle (or equivalent) is declared
```

**Test note**: This is a compile-time / code-structure requirement. Verified by code review and by the fact that `go vet ./...` and `go build ./...` produce no errors. No runtime test needed beyond the build gate.

---

### R-S2 — No double vertical gaps

**Scenario S2.1 — Single gap between sections**
```
Given   any rendered screen output
When    consecutive blank lines are counted between section boundaries
Then    no two consecutive blank lines appear (i.e. no "\n\n\n" sequence in View output)
```

**Test note**: Assert `!strings.Contains(rendered, "\n\n\n")` in a rendered-output snapshot test.

---

### R-S3 — Shared padding/indent constant

**Scenario S3.1 — No hardcoded numeric padding in fmt calls**
```
Given   the source code of view.go
When    it is compiled
Then    padding/indent values used in fmt.Sprintf / lipgloss padding are referenced via named constants, not magic numbers
```

**Test note**: Static code-review gate; no dedicated runtime test. Verified by code review + build success.

---

### R-011 — Test-first coverage

**Scenario 11.1 — Existing tests green**
```
Given   the PR-1 implementation is applied
When    `cd tui && go test ./...` is run
Then    all pre-existing tests pass with exit code 0
```

**Scenario 11.2 — New behaviors have tests**
```
Given   the PR-1 implementation is applied
When    test coverage is inspected
Then    each of R-001 through R-007 has at least one corresponding test function
  AND   each test function was written BEFORE its production code (verified by git log order)
```

---

### R-012 — Spanish copy preserved

**Scenario 12.1 — No anglicized UI strings**
```
Given   the PR-1 diff
When    all added or modified UI string literals in view.go and model.go are reviewed
Then    none replace a Spanish string with an English equivalent
  AND   the failure banner text, footer labels, and notes are in Spanish
```

---

## Observable Invariants

These are cross-requirement assertions that any test suite MUST uphold:

1. `View()` never panics regardless of model state (fuzz with random key sequences).
2. `m.width == 0` before the first `WindowSizeMsg` is a valid state; View must not panic and must render at 80 columns.
3. The spinner component is `github.com/charmbracelet/bubbles/spinner`; it is initialized in the model `Init()` command chain.
4. `errStyle` is a distinct lipgloss style from the normal title style (different foreground color).

---

## Out of Scope (explicitly excluded)

- Real-time streaming of backend stdout.
- viewport.Model scroll refactor (deferred).
- `bootstrap` and `install-alias` in TUI.
- Any English UI copy.
- Split-pane or multi-pane layouts.
