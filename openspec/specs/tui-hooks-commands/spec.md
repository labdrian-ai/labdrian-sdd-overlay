# Spec: tui-hooks-commands

**Change**: tui-professional-polish  
**Capability**: tui-hooks-commands  
**PR slice**: PR-2 (Hooks Command Coverage)  
**Strict TDD**: YES — teatest + unit; `cd tui && go test ./...`

---

## Scope

Covers the `TargetAgnostic` routing field on `Action`, the three hooks lifecycle actions (`status-hooks`, `install-hooks`, `uninstall-hooks`), the `─── Hooks ───` separator in the actions list, and the per-action `ConfirmMessage` field with its backwards-safe fallback. No new screen, no backend changes.

---

## Requirement Table

| ID    | Category        | SHALL statement |
|-------|-----------------|-----------------|
| R-008 | Data model      | The `Action` struct SHALL have a `TargetAgnostic bool` field; when `TargetAgnostic` is true, `runBackend` SHALL invoke the subcommand WITHOUT a `--target` argument and SHALL NOT trigger target selection. |
| R-009 | Command coverage | The TUI SHALL expose `status-hooks`, `install-hooks`, and `uninstall-hooks` as `TargetAgnostic` actions in the existing flat actions list, grouped under a dim `─── Hooks ───` separator row (no new screen required). |
| R-010 | UX              | The `Action` struct SHALL have a `ConfirmMessage string` field; the confirm screen SHALL display `Action.ConfirmMessage` when non-empty, and SHALL fall back to the existing generic confirmation message when `ConfirmMessage` is empty. `install-hooks` SHALL carry confirmation copy that mentions `~/.claude/settings.json` and the `.bak` backup. |
| R-011 | Testing         | All new and modified behavior in PR-2 SHALL be covered by tests written test-first; all previously-green tests SHALL remain green. |
| R-012 | I18n            | All Spanish user-facing copy introduced or modified in PR-2 SHALL remain in Spanish; no UI string SHALL be anglicized. |

---

## Scenarios

### R-008 — TargetAgnostic routing

**Scenario 8.1 — TargetAgnostic action does not emit --target**
```
Given   an Action with TargetAgnostic: true and Cmd: "status-hooks"
When    runBackend executes the action
Then    the command-line arguments passed to the subprocess do NOT include "--target"
  AND   the command-line arguments do NOT include any target name string
```

**Scenario 8.2 — Non-TargetAgnostic action still emits --target**
```
Given   an Action with TargetAgnostic: false and Cmd: "apply"
  AND   a target "some-agent" is selected
When    runBackend executes the action
Then    the command-line arguments include "--target" followed by "some-agent"
```

**Scenario 8.3 — TargetAgnostic action skips target-selection screen**
```
Given   an Action with TargetAgnostic: true is selected on screenHome
When    the user confirms the action (presses enter)
Then    the model transitions directly to screenRunning (NOT to screenTargets)
```

**Scenario 8.4 — TargetAgnostic does not require targets to be configured**
```
Given   an Action with TargetAgnostic: true
  AND   the target list is empty (no configured targets)
When    the action is invoked
Then    the model proceeds to screenRunning without error
```

**Test note**: Unit-test `runBackend` with a fake executor that captures args; assert "--target" absent. Unit-test model Update to confirm screen transition skips screenTargets for TargetAgnostic actions.

---

### R-009 — Hooks actions in the actions list

**Scenario 9.1 — Three hooks actions are present**
```
Given   the Actions() list is inspected at startup
When    action commands are enumerated
Then    the list contains exactly "status-hooks", "install-hooks", and "uninstall-hooks"
  AND   all three have TargetAgnostic: true
```

**Scenario 9.2 — Hooks separator appears in the rendered home screen**
```
Given   the model is on screenHome
When    View() is called
Then    the rendered output contains "─── Hooks ───" (or equivalent dim separator text)
  AND   the separator appears between the last non-hooks action and the first hooks action
```

**Scenario 9.3 — status-hooks has no confirmation step**
```
Given   the model is on screenHome with "status-hooks" selected
When    the user presses enter
Then    the model does NOT transition to screenConfirm
  AND   the model transitions directly to screenRunning
```

**Scenario 9.4 — install-hooks and uninstall-hooks require confirmation**
```
Given   the model is on screenHome with "install-hooks" (or "uninstall-hooks") selected
When    the user presses enter
Then    the model transitions to screenConfirm before screenRunning
```

**Test note**: Inspect `Actions()` return value in a unit test; assert Cmd, TargetAgnostic, and NeedsConfirm fields. Use teatest to drive the transition flow for 9.3 and 9.4.

---

### R-010 — Per-action ConfirmMessage

**Scenario 10.1 — Custom ConfirmMessage is displayed**
```
Given   an Action with ConfirmMessage: "Do you want to proceed with X?"
  AND   the model is on screenConfirm for that action
When    View() is called
Then    the rendered output contains "Do you want to proceed with X?"
  AND   the generic fallback message is NOT shown
```

**Scenario 10.2 — Empty ConfirmMessage falls back to generic message**
```
Given   an Action with ConfirmMessage: "" (empty string)
  AND   the model is on screenConfirm for that action
When    View() is called
Then    the rendered output contains the existing generic confirmation message
```

**Scenario 10.3 — install-hooks confirm copy mentions settings.json and backup**
```
Given   the "install-hooks" action is selected and the model is on screenConfirm
When    View() is called
Then    the rendered output contains "settings.json"
  AND   the rendered output contains ".bak" (or "backup"/"respaldo")
  AND   the copy is in Spanish
```

**Scenario 10.4 — Existing actions (apply, capture) retain their prior confirm copy**
```
Given   the "apply" action is selected and the model is on screenConfirm
When    View() is called
Then    the rendered confirm message is identical to the pre-change generic confirmation text
  (i.e. ConfirmMessage is empty for apply, triggering the fallback)
```

**Test note**: Unit-test `confirmMessageFor(action Action) string` helper (or equivalent logic) with table cases: custom non-empty, empty fallback, install-hooks string content. For 10.4, assert the `apply` Action struct has an empty ConfirmMessage.

---

### R-011 — Test-first coverage (PR-2)

**Scenario 11.1 — All prior tests remain green after PR-2**
```
Given   the PR-2 implementation is applied on top of PR-1
When    `cd tui && go test ./...` is run
Then    all tests (pre-existing + PR-1 additions) pass with exit code 0
```

**Scenario 11.2 — New behaviors in PR-2 have test coverage**
```
Given   the PR-2 implementation is applied
When    test coverage is inspected
Then    each of R-008 through R-010 has at least one corresponding test function
  AND   each test function was written BEFORE its production code (verified by git log order)
```

---

### R-012 — Spanish copy preserved (PR-2)

**Scenario 12.1 — Hooks action labels are in Spanish**
```
Given   the PR-2 diff
When    all added or modified display-name strings for the three hooks actions are reviewed
Then    none are in English where a Spanish equivalent existed or is expected
  (action Cmd fields remain in English as they are CLI subcommand names, not display strings)
```

**Scenario 12.2 — install-hooks confirm copy is in Spanish**
```
Given   the install-hooks ConfirmMessage string
When    inspected
Then    the copy is written in Spanish (neutral/professional)
  AND   it mentions "settings.json" and the ".bak" backup concept
```

---

## Backwards-Compatibility Invariants

These are non-regression guarantees that the test suite MUST enforce:

1. The `Action` struct remains backward-compatible: zero-value `TargetAgnostic` (false) preserves existing routing behavior for all pre-existing actions.
2. The `Action` struct remains backward-compatible: zero-value `ConfirmMessage` (empty string) triggers the generic fallback, preserving existing confirmation copy for `apply` and `capture`.
3. All hooks actions (`status-hooks`, `install-hooks`, `uninstall-hooks`) have `TargetAgnostic: true`; none should ever appear in the `screenTargets` flow.
4. The `─── Hooks ───` separator row is a display-only element and is NOT a selectable action; pressing enter on it has no effect (or the cursor skips it).

---

## Out of Scope (explicitly excluded)

- New screens for hooks actions.
- `bootstrap` and `install-alias` as TUI actions (one-time setup tools, explicitly excluded).
- Changes to backend engine command behavior.
- Any English UI copy replacing Spanish strings.
- Real-time streaming, split-pane, or parallel execution columns.
