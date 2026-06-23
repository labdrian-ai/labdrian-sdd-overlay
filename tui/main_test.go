package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
)

// spinnerTickMsgForTest creates a spinner.TickMsg to advance the spinner in tests.
func spinnerTickMsgForTest() tea.Msg {
	return spinner.TickMsg{ID: 0, Time: time.Now()}
}

// TestInitialRenderShowsTargets verifies the first screen lists all three
// targets and that they default to selected.
func TestInitialRenderShowsTargets(t *testing.T) {
	tm := teatest.NewTestModel(t, newModel(), teatest.WithInitialTermSize(120, 40))

	// Wait for a frame that shows all three targets, each selected ([✓]).
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		s := string(b)
		return strings.Contains(s, "claude") &&
			strings.Contains(s, "opencode") &&
			strings.Contains(s, "codex") &&
			strings.Count(s, "[✓]") >= 3
	}, teatest.WithDuration(3*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

// TestToggleSelection verifies space toggles the target under the cursor off.
func TestToggleSelection(t *testing.T) {
	m := newModel()
	// All selected by default.
	for i := range m.targets {
		if !m.selected[i] {
			t.Fatalf("target %d should default selected", i)
		}
	}

	updated, _ := m.updateTargets(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = updated.(model)
	if m.selected[0] {
		t.Fatal("target 0 should be deselected after space")
	}

	// Toggle back on.
	updated, _ = m.updateTargets(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = updated.(model)
	if !m.selected[0] {
		t.Fatal("target 0 should be re-selected after second space")
	}
}

// TestParseSyncCheckDashboard verifies a sample VERDICT/ACTION block parses
// into the correct colored statuses for each target.
func TestParseSyncCheckDashboard(t *testing.T) {
	sample := `
=== sync-check: claude ===
  IN_SYNC: skills/foo
VERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0
ACTION:claude: in sync with gentle-ai (healthy)

=== sync-check: opencode ===
  OVERLAY_NOT_DEPLOYED: skills/bar (live missing)
VERDICT:opencode:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=2
ACTION:opencode: run 'overlay apply --target opencode'

=== sync-check: codex ===
  UPSTREAM_CHANGED: skills/baz
VERDICT:codex:UPSTREAM_CHANGED=3 OVERLAY_NOT_DEPLOYED=1
ACTION:codex: gentle-ai sync detected: run 'overlay capture --target codex' then 'overlay apply'
`

	verdicts := ParseSyncCheck(sample)
	if len(verdicts) != 3 {
		t.Fatalf("expected 3 verdicts, got %d", len(verdicts))
	}

	want := []struct {
		target  string
		status  SyncStatus
		uc, ond int
	}{
		{"claude", SyncHealthy, 0, 0},
		{"opencode", SyncNeedsApply, 0, 2},
		{"codex", SyncNeedsCapture, 3, 1},
	}

	for i, w := range want {
		v := verdicts[i]
		if v.Target != w.target {
			t.Errorf("verdict %d: target = %q, want %q", i, v.Target, w.target)
		}
		if v.Status != w.status {
			t.Errorf("%s: status = %d, want %d", w.target, v.Status, w.status)
		}
		if v.UpstreamChanged != w.uc || v.OverlayNotDeployed != w.ond {
			t.Errorf("%s: counts = (%d,%d), want (%d,%d)",
				w.target, v.UpstreamChanged, v.OverlayNotDeployed, w.uc, w.ond)
		}
		if v.Action == "" {
			t.Errorf("%s: action should not be empty", w.target)
		}
	}
}

func TestParseSyncCheckMissingTargetIsNotHealthy(t *testing.T) {
	// R-101: a missing target directory must never render as synchronized/healthy.
	sample := `
=== sync-check: codex (/tmp/missing) ===
SYNC_CHECK:codex: target dir not found at /tmp/missing -- skipping
VERDICT:codex:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=1 TARGET_MISSING=1
ACTION:codex: target directory missing; run 'overlay apply --target codex'
[codex] sync-check: partial — engine binary not installed
`

	verdicts := ParseSyncCheck(sample)
	if len(verdicts) != 1 {
		t.Fatalf("expected one verdict, got %d", len(verdicts))
	}
	if verdicts[0].Status != SyncTargetMissing || !verdicts[0].TargetMissing {
		t.Fatalf("missing target should parse as SyncTargetMissing, got %#v", verdicts[0])
	}
	statuses := ParseRuntimeStatuses(sample)
	if len(statuses) != 1 || statuses[0].Status != RuntimePartial {
		t.Fatalf("missing target output should still include runtime partial evidence, got %#v", statuses)
	}

	m := newModel()
	m.width = 100
	m.height = 30
	m.scr = screenResult
	m.result = commandResult{action: Action{Command: "sync-check"}, output: sample, verdicts: verdicts, runtimeStatuses: statuses}
	rendered := m.View()
	if strings.Contains(rendered, "Sincronizado") {
		t.Fatalf("missing target must not render as synchronized, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Directorio target ausente") {
		t.Fatalf("missing target should render explicit missing-target label, got:\n%s", rendered)
	}
}

// TestSpinnerPresentOnRunning verifies a spinner glyph is present in screenRunning.
func TestSpinnerPresentOnRunning(t *testing.T) {
	m := newModel()
	// Advance into screenRunning by choosing a non-mutating action.
	action := Action{Name: "Estado", Command: "status", Mutating: false, SupportsAll: true}
	m.pendingAction = action
	m.scr = screenRunning

	// Send a spinner tick so the spinner has advanced at least one frame.
	updated, _ := m.Update(spinnerTickMsgForTest())
	m = updated.(model)

	rendered := m.View()
	if !strings.Contains(rendered, "Ejecutando") {
		t.Fatal("running screen must contain 'Ejecutando'")
	}
	// The spinner.View() on a Dot spinner emits one of: "⣾⣽⣻⢿⡿⣟⣯⣷" or similar.
	// We assert the rendered string is non-empty beyond the bare label.
	spinnerStr := m.spinner.View()
	if spinnerStr == "" {
		t.Fatal("spinner.View() must be non-empty while in screenRunning")
	}
	if !strings.Contains(rendered, spinnerStr) {
		t.Fatalf("running screen View() must contain spinner glyph %q, got:\n%s", spinnerStr, rendered)
	}
}

// TestSpinnerAbsentOutsideRunning verifies the spinner glyph does NOT appear in
// screens other than screenRunning. This guards the TickMsg screen-guard in model.go
// (~line 162): if removed, spinner ticks would advance and embed the spinner everywhere.
func TestSpinnerAbsentOutsideRunning(t *testing.T) {
	// Advance the spinner to a known frame so we have a real glyph to look for.
	base := newModel()
	base.scr = screenRunning
	updated, _ := base.Update(spinnerTickMsgForTest())
	base = updated.(model)
	spinnerGlyph := base.spinner.View()
	if spinnerGlyph == "" {
		t.Fatal("test setup: spinner.View() must be non-empty after a tick")
	}

	screens := []struct {
		name string
		scr  screen
	}{
		{"screenTargets", screenTargets},
		{"screenActions", screenActions},
		{"screenConfirm", screenConfirm},
		{"screenResult", screenResult},
	}

	for _, tc := range screens {
		t.Run(tc.name, func(t *testing.T) {
			m := newModel()
			// Copy the already-advanced spinner into the model so the glyph is
			// at a known non-default state and would be visible if rendered.
			m.spinner = base.spinner
			m.scr = tc.scr

			// For screenConfirm we need a pending action.
			if tc.scr == screenConfirm {
				m.pendingAction = Action{Name: "Aplicar cambios", Command: "apply", Mutating: true}
			}
			// For screenResult we need a result.
			if tc.scr == screenResult {
				m.result = commandResult{
					action: Action{Name: "Estado", Command: "status"},
					output: "some output",
				}
			}

			rendered := m.View()
			if strings.Contains(rendered, spinnerGlyph) {
				t.Errorf("%s must NOT render spinner glyph %q, got:\n%s", tc.name, spinnerGlyph, rendered)
			}
		})
	}
}

// TestOutputBoxStyleDeclared is a compile-gate: outputBoxStyle must be a
// non-zero lipgloss.Style accessible from the package.
func TestOutputBoxStyleDeclared(t *testing.T) {
	// outputBoxStyle is a package-level var; accessing it here ensures it
	// compiles and is non-zero (has at least a border set).
	s := outputBoxStyle
	// A zero Style renders the empty string as ""; a styled box will have border chars.
	rendered := s.Render("x")
	if rendered == "x" {
		t.Fatal("outputBoxStyle must have border styling (not a zero style)")
	}
}

// TestNoDoubleGapInAnyScreen asserts no three consecutive newlines appear in
// rendered home or result screens (double-gap elimination).
func TestNoDoubleGapInAnyScreen(t *testing.T) {
	m := newModel()
	// Home screen (screenTargets).
	rendered := m.View()
	if strings.Contains(rendered, "\n\n\n") {
		t.Errorf("screenTargets View() must not contain triple-newline double gap, got:\n%q", rendered)
	}

	// Result screen.
	m.scr = screenResult
	m.result = commandResult{
		action: Action{Name: "Estado", Command: "status"},
		output: "ok",
	}
	rendered = m.View()
	if strings.Contains(rendered, "\n\n\n") {
		t.Errorf("screenResult View() must not contain triple-newline double gap, got:\n%q", rendered)
	}
}

// TestWidthResponsiveRendering asserts that no rendered line exceeds the terminal
// width when a narrow terminal size is used (R-001). It checks multiple screens
// and uses lipgloss.Width to correctly measure visible width ignoring ANSI sequences.
func TestWidthResponsiveRendering(t *testing.T) {
	const narrowWidth = 40

	buildResultModel := func(output string) model {
		m := newModel()
		updated, _ := m.Update(tea.WindowSizeMsg{Width: narrowWidth, Height: 20})
		m = updated.(model)
		m.scr = screenResult
		m.result = commandResult{
			action: Action{Name: "Estado", Command: "status"},
			output: output,
		}
		return m
	}

	// Build a long output string to stress the result screen.
	var longLines []string
	for i := 0; i < 20; i++ {
		longLines = append(longLines, fmt.Sprintf("this is a rather long output line number %d that might overflow the terminal width", i))
	}
	longOutput := strings.Join(longLines, "\n")

	cases := []struct {
		name    string
		prepare func() model
	}{
		{
			name: "screenTargets",
			prepare: func() model {
				m := newModel()
				updated, _ := m.Update(tea.WindowSizeMsg{Width: narrowWidth, Height: 20})
				m = updated.(model)
				return m
			},
		},
		{
			name: "screenActions",
			prepare: func() model {
				m := newModel()
				updated, _ := m.Update(tea.WindowSizeMsg{Width: narrowWidth, Height: 20})
				m = updated.(model)
				m.scr = screenActions
				return m
			},
		},
		{
			name: "screenResult with long output",
			prepare: func() model {
				return buildResultModel(longOutput)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.prepare()
			rendered := m.View()
			for i, line := range strings.Split(rendered, "\n") {
				w := lipgloss.Width(line)
				if w > narrowWidth {
					t.Errorf("line %d exceeds %d columns (%d visible): %q", i+1, narrowWidth, w, line)
				}
			}
		})
	}
}

// TestWidthFallbackTo80 verifies that a fresh newModel() (width==0) renders at 80
// columns via the contentWidth() fallback. Must fail if the fallback is removed.
func TestWidthFallbackTo80(t *testing.T) {
	m := newModel()
	// Confirm no WindowSizeMsg has been received — width must be 0.
	if m.width != 0 {
		t.Fatalf("newModel() must have width==0 before any WindowSizeMsg, got %d", m.width)
	}

	if m.contentWidth() != 80 {
		t.Fatalf("contentWidth() with width==0 must return 80 (fallback), got %d", m.contentWidth())
	}

	// Also verify the rendered View() respects the 80-col constraint.
	rendered := m.View()
	for i, line := range strings.Split(rendered, "\n") {
		w := lipgloss.Width(line)
		if w > 80 {
			t.Errorf("line %d exceeds 80 columns (%d visible) in zero-width fallback: %q", i+1, w, line)
		}
	}
}

// TestScrollClamp verifies m.scroll is clamped to EXACTLY maxScroll() (not merely
// <= some value) and never goes negative (R-005). This catches off-by-one errors.
func TestScrollClamp(t *testing.T) {
	m := newModel()
	m.width = 80
	m.height = 20
	m.scr = screenResult
	// Build multi-line output that exceeds the viewport.
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = "line"
	}
	m.result = commandResult{
		action: Action{Name: "Estado", Command: "status"},
		output: strings.Join(lines, "\n"),
	}

	max := m.maxScroll()
	if max <= 0 {
		t.Fatalf("maxScroll() should be > 0 for 30 lines with height 20, got %d", max)
	}

	// Drive 100 down-key messages — scroll must saturate at EXACTLY maxScroll.
	for i := 0; i < 100; i++ {
		updated, _ := m.updateResult(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = updated.(model)
	}
	// Exact upper-bound assertion: must equal max, not merely be <= max.
	if m.scroll != max {
		t.Errorf("scroll after saturation: got %d, want exactly maxScroll()=%d", m.scroll, max)
	}

	// Drive up past zero — scroll must not go negative.
	for i := 0; i < 200; i++ {
		updated, _ := m.updateResult(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		m = updated.(model)
	}
	if m.scroll < 0 {
		t.Errorf("scroll went negative after many up-keys: %d", m.scroll)
	}
	if m.scroll != 0 {
		t.Errorf("scroll after full rewind: got %d, want exactly 0", m.scroll)
	}
}

func TestResultViewportSizingMatchesRenderAndScrollBehavior(t *testing.T) {
	// Runtime parity review warning: prove the rendered output window and scroll
	// clamp honor the same dashboard-aware viewport sizing behavior.
	lines := make([]string, 12)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%02d", i+1)
	}
	m := newModel()
	m.width = 90
	m.height = 18
	m.scr = screenResult
	m.result = commandResult{
		output: strings.Join(lines, "\n"),
		verdicts: []TargetVerdict{
			{Target: "claude", Status: SyncHealthy, Action: "healthy"},
		},
		runtimeStatuses: []RuntimeStatus{
			{Target: "opencode", Status: RuntimeRestartRequired, Message: "restart required"},
		},
	}

	if got := m.maxScroll(); got != 7 {
		t.Fatalf("maxScroll() = %d, want 7 for 12 lines with 5-line minimum viewport", got)
	}
	rendered := m.View()
	for _, want := range []string{"[lines 1-5 of 12]", "line-01", "line-05"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered output should contain %q; got:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "line-06") {
		t.Fatalf("rendered output should stop at the 5-line viewport before scrolling; got:\n%s", rendered)
	}

	for i := 0; i < 20; i++ {
		updated, _ := m.updateResult(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(model)
	}
	if m.scroll != 7 {
		t.Fatalf("scroll after saturation = %d, want 7", m.scroll)
	}
	rendered = m.View()
	for _, want := range []string{"[lines 8-12 of 12]", "line-08", "line-12"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("scrolled output should contain %q; got:\n%s", want, rendered)
		}
	}
}

func TestAllTargetsIgnoresRelativeXDGConfigHome(t *testing.T) {
	// R-103: TUI target paths must not point OpenCode at a repo-relative XDG root.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join("relative", "xdg"))
	targets := AllTargets()
	var opencodePath string
	for _, target := range targets {
		if target.Name == "opencode" {
			opencodePath = target.Path
		}
	}
	want := filepath.Join(home, ".config", "opencode", "skills")
	if opencodePath != want {
		t.Fatalf("opencode target path = %q, want %q", opencodePath, want)
	}
}

// TestSelectAllToggle verifies 'a' on screenTargets toggles all selections (R-006).
func TestSelectAllToggle(t *testing.T) {
	m := newModel()
	// All selected by default.
	if !m.allSelected() {
		t.Fatal("all targets must default to selected")
	}

	// Press 'a' — all should deselect.
	updated, _ := m.updateTargets(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(model)
	if m.anySelected() {
		t.Fatal("after 'a' when all selected, none should be selected")
	}

	// Press 'a' again — all should select.
	updated, _ = m.updateTargets(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(model)
	if !m.allSelected() {
		t.Fatal("after 'a' when none selected, all should be selected")
	}
}

// TestErrorBannerOnFailure verifies a red "Comando falló" banner when result.err != nil,
// and that the success path does NOT contain the banner (R-003).
func TestErrorBannerOnFailure(t *testing.T) {
	m := newModel()
	m.scr = screenResult
	m.result = commandResult{
		action: Action{Name: "Aplicar cambios", Command: "apply"},
		output: "error output",
		err:    fmt.Errorf("exit status 1"),
	}

	rendered := m.View()
	if !strings.Contains(rendered, "Comando falló") {
		t.Errorf("error result must contain 'Comando falló', got:\n%s", rendered)
	}

	// Success path.
	m.result.err = nil
	rendered = m.View()
	if strings.Contains(rendered, "Comando falló") {
		t.Errorf("success result must NOT contain 'Comando falló'")
	}
}

// TestEmptyVerdictNote verifies the dim note when sync-check produces zero
// verdicts (R-004), and that a non-sync-check command does NOT show the note
// even when verdicts are also empty.
func TestEmptyVerdictNote(t *testing.T) {
	m := newModel()
	m.scr = screenResult
	m.result = commandResult{
		action:   Action{Name: "Verificar sincronización", Command: "sync-check"},
		output:   "some raw output",
		verdicts: nil,
	}

	rendered := m.View()
	if !strings.Contains(rendered, "No se pudieron analizar veredictos") {
		t.Errorf("sync-check with 0 verdicts must show dim note, got:\n%s", rendered)
	}

	// With verdicts present, note must NOT appear.
	m.result.verdicts = []TargetVerdict{{Target: "claude", Status: SyncHealthy}}
	rendered = m.View()
	if strings.Contains(rendered, "No se pudieron analizar veredictos") {
		t.Errorf("sync-check with verdicts must NOT show dim note")
	}

	// NEGATIVE PATH: a non-sync-check command with zero verdicts must NOT show
	// the note — guards the Command=="sync-check" gate in viewResult.
	m.result = commandResult{
		action:   Action{Name: "Estado", Command: "status"},
		output:   "some output",
		verdicts: nil,
	}
	rendered = m.View()
	if strings.Contains(rendered, "No se pudieron analizar veredictos") {
		t.Errorf("non-sync-check command with 0 verdicts must NOT show dim note, got:\n%s", rendered)
	}
}

// TestFooterLegendCorrectness verifies footer key hints for all interactive screens
// (R-007). Covers targets, actions, result, and confirm screens.
func TestFooterLegendCorrectness(t *testing.T) {
	m := newModel()

	// screenTargets footer.
	rendered := m.View()
	if !strings.Contains(rendered, "espacio seleccionar") {
		t.Errorf("targets screen footer must contain 'espacio seleccionar', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "enter continuar") {
		t.Errorf("targets screen footer must contain 'enter continuar', got:\n%s", rendered)
	}

	// screenActions footer.
	m.scr = screenActions
	rendered = m.View()
	if !strings.Contains(rendered, "enter ejecutar") {
		t.Errorf("actions screen footer must contain 'enter ejecutar', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "esc volver") {
		t.Errorf("actions screen footer must contain 'esc volver', got:\n%s", rendered)
	}

	// Result screen footer.
	m.scr = screenResult
	m.result = commandResult{
		action: Action{Name: "Estado", Command: "status"},
		output: "ok",
	}
	rendered = m.View()
	if !strings.Contains(rendered, "esc/enter") {
		t.Errorf("result screen footer must contain 'esc/enter', got footer section in:\n%s", rendered)
	}

	// Confirm screen footer.
	m.scr = screenConfirm
	m.pendingAction = Action{Name: "Aplicar cambios", Command: "apply", Mutating: true}
	rendered = m.View()
	if !strings.Contains(rendered, "esc/n") {
		t.Errorf("confirm screen footer must contain 'esc/n', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "y confirmar") {
		t.Errorf("confirm screen footer must contain 'y confirmar', got:\n%s", rendered)
	}
}

// ---------------------------------------------------------------------------
// PR-2: Hooks command coverage (R-008, R-009, R-010)
// ---------------------------------------------------------------------------

// TestBuildArgSets verifies the pure buildArgSets helper:
//   - TargetAgnostic action → single arg set with no --target (R-008 Scenario 8.1)
//   - Non-TargetAgnostic action with one target → args include --target (R-008 Scenario 8.2)
func TestBuildArgSets(t *testing.T) {
	t.Run("TargetAgnostic omits --target", func(t *testing.T) {
		action := Action{TargetAgnostic: true, Command: "status-hooks"}
		sets := buildArgSets(action, nil, false)
		if len(sets) != 1 {
			t.Fatalf("expected 1 arg set, got %d", len(sets))
		}
		for _, arg := range sets[0] {
			if arg == "--target" {
				t.Fatal("TargetAgnostic action must NOT include --target in arg set")
			}
		}
		if sets[0][0] != "status-hooks" {
			t.Fatalf("first arg must be command name, got %q", sets[0][0])
		}
	})

	t.Run("non-TargetAgnostic with one target includes --target", func(t *testing.T) {
		action := Action{TargetAgnostic: false, Command: "apply", SupportsAll: true}
		targets := []Target{{Name: "agent-x", Path: "/some/path"}}
		sets := buildArgSets(action, targets, false)
		if len(sets) != 1 {
			t.Fatalf("expected 1 arg set, got %d", len(sets))
		}
		args := sets[0]
		foundTarget := false
		for i, arg := range args {
			if arg == "--target" && i+1 < len(args) && args[i+1] == "agent-x" {
				foundTarget = true
			}
		}
		if !foundTarget {
			t.Fatalf("non-TargetAgnostic action must include --target agent-x, got %v", args)
		}
	})
}

// TestHooksActionsRegistered verifies all three hooks actions are in Actions() with
// correct TargetAgnostic and Mutating values (R-009 Scenario 9.1).
func TestHooksActionsRegistered(t *testing.T) {
	actions := Actions()
	byCmd := make(map[string]Action)
	for _, a := range actions {
		byCmd[a.Command] = a
	}

	for _, cmd := range []string{"status-hooks", "install-hooks", "uninstall-hooks"} {
		a, ok := byCmd[cmd]
		if !ok {
			t.Errorf("Actions() must contain %q", cmd)
			continue
		}
		if !a.TargetAgnostic {
			t.Errorf("%s must have TargetAgnostic: true", cmd)
		}
	}

	if sh, ok := byCmd["status-hooks"]; ok && sh.Mutating {
		t.Error("status-hooks must have Mutating: false (read-only)")
	}
	for _, cmd := range []string{"install-hooks", "uninstall-hooks"} {
		if a, ok := byCmd[cmd]; ok && !a.Mutating {
			t.Errorf("%s must have Mutating: true", cmd)
		}
	}
}

// TestConfirmMessageSelection verifies:
//   - Empty ConfirmMessage → generic fallback shown (R-010 Scenario 10.2)
//   - install-hooks ConfirmMessage → specific copy shown (R-010 Scenario 10.3)
//   - TargetAgnostic on screenConfirm → no target list shown (R-010, backwards-compat)
func TestConfirmMessageSelection(t *testing.T) {
	t.Run("empty ConfirmMessage shows generic copy", func(t *testing.T) {
		m := newModel()
		m.scr = screenConfirm
		m.pendingAction = Action{
			Name:     "Aplicar cambios",
			Command:  "apply",
			Mutating: true,
			// ConfirmMessage is empty (zero value)
		}
		rendered := m.View()
		if !strings.Contains(rendered, "Esta acción modifica los destinos") {
			t.Errorf("empty ConfirmMessage must show generic copy, got:\n%s", rendered)
		}
	})

	t.Run("install-hooks ConfirmMessage shows settings.json and .bak", func(t *testing.T) {
		// Find the install-hooks action from Actions().
		var installAction Action
		for _, a := range Actions() {
			if a.Command == "install-hooks" {
				installAction = a
				break
			}
		}
		if installAction.Command == "" {
			t.Skip("install-hooks not yet registered in Actions()")
		}

		m := newModel()
		m.scr = screenConfirm
		m.pendingAction = installAction
		rendered := m.View()
		if !strings.Contains(rendered, "settings.json") {
			t.Errorf("install-hooks confirm must mention 'settings.json', got:\n%s", rendered)
		}
		if !strings.Contains(rendered, ".bak") {
			t.Errorf("install-hooks confirm must mention '.bak', got:\n%s", rendered)
		}
	})

	t.Run("TargetAgnostic action on screenConfirm hides target list", func(t *testing.T) {
		m := newModel()
		m.scr = screenConfirm
		m.pendingAction = Action{
			Name:           "Estado de hooks",
			Command:        "status-hooks",
			Mutating:       true,
			TargetAgnostic: true,
			ConfirmMessage: "Acción global sin destinos.",
		}
		rendered := m.View()
		// The "en: <targets>" clause must NOT appear for TargetAgnostic actions.
		if strings.Contains(rendered, "en: claude") {
			t.Errorf("TargetAgnostic confirm must NOT show target list, got:\n%s", rendered)
		}
	})
}

// TestHooksSeparatorVisible verifies the "─── Hooks ───" separator renders on
// screenActions when hooks actions are registered (R-009 Scenario 9.2).
func TestHooksSeparatorVisible(t *testing.T) {
	m := newModel()
	m.scr = screenActions
	rendered := m.View()
	if !strings.Contains(rendered, "─── Hooks ───") {
		t.Errorf("screenActions must contain '─── Hooks ───' separator, got:\n%s", rendered)
	}
}

// TestStatusHooksSkipsConfirm verifies status-hooks (non-mutating) goes directly
// to screenRunning, not screenConfirm (R-009 Scenario 9.3).
func TestStatusHooksSkipsConfirm(t *testing.T) {
	m := newModel()
	// Navigate to screenActions.
	m.scr = screenActions

	// Find the index of status-hooks in the actions list.
	statusIdx := -1
	for i, a := range m.actions {
		if a.Command == "status-hooks" {
			statusIdx = i
			break
		}
	}
	if statusIdx < 0 {
		t.Fatal("status-hooks not found in Actions(); hooks actions are required by this change")
	}

	m.aCursor = statusIdx
	updated, _ := m.updateActions(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)

	if m.scr == screenConfirm {
		t.Error("status-hooks (read-only) must NOT go to screenConfirm")
	}
	if m.scr != screenRunning {
		t.Errorf("status-hooks must transition to screenRunning, got screen %d", m.scr)
	}
}

// TestInstallHooksRequiresConfirm verifies install-hooks (mutating) routes to
// screenConfirm before running (R-009 Scenario 9.4).
func TestInstallHooksRequiresConfirm(t *testing.T) {
	m := newModel()
	m.scr = screenActions

	installIdx := -1
	for i, a := range m.actions {
		if a.Command == "install-hooks" {
			installIdx = i
			break
		}
	}
	if installIdx < 0 {
		t.Fatal("install-hooks not found in Actions(); hooks actions are required by this change")
	}

	m.aCursor = installIdx
	updated, _ := m.updateActions(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)

	if m.scr != screenConfirm {
		t.Errorf("install-hooks (mutating) must route to screenConfirm, got screen %d", m.scr)
	}
}

// TestUninstallHooksRequiresConfirm verifies uninstall-hooks (mutating) routes to
// screenConfirm before running (mirrors TestInstallHooksRequiresConfirm).
func TestUninstallHooksRequiresConfirm(t *testing.T) {
	m := newModel()
	m.scr = screenActions

	uninstallIdx := -1
	for i, a := range m.actions {
		if a.Command == "uninstall-hooks" {
			uninstallIdx = i
			break
		}
	}
	if uninstallIdx < 0 {
		t.Fatal("uninstall-hooks not found in Actions(); hooks actions are required by this change")
	}

	m.aCursor = uninstallIdx
	updated, _ := m.updateActions(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)

	if m.scr != screenConfirm {
		t.Errorf("uninstall-hooks (mutating) must route to screenConfirm, got screen %d", m.scr)
	}
}

// TestKeyboardNavigationOverSeparator verifies that driving real key events through
// updateActions never lands aCursor on a phantom separator entry.
//
// The separator rendered by viewActions is display-only — it is NOT in m.actions.
// This test guards against a future index mismatch if a separator item were ever
// accidentally inserted into the actions slice.
//
// Assertions:
//
//	(a) Every cursor position after each "down" press maps to a valid m.actions entry
//	    (i.e. m.actions[m.aCursor].Name is non-empty).
//	(b) Navigating down through the full list eventually reaches a hooks action
//	    (install-hooks or uninstall-hooks) and pressing enter there transitions to
//	    screenConfirm.
func TestKeyboardNavigationOverSeparator(t *testing.T) {
	m := newModel()
	m.scr = screenActions

	if len(m.actions) == 0 {
		t.Fatal("Actions() must be non-empty")
	}

	// Verify initial cursor is valid.
	if m.actions[m.aCursor].Name == "" {
		t.Fatalf("initial aCursor=%d points to an action with empty Name", m.aCursor)
	}

	// Drive "down" repeatedly through the full list (one extra press to confirm
	// the cursor saturates at the last valid index, not beyond).
	totalActions := len(m.actions)
	downKey := tea.KeyMsg{Type: tea.KeyDown}

	for step := 0; step < totalActions+2; step++ {
		updated, _ := m.updateActions(downKey)
		m = updated.(model)

		// Assertion (a): cursor must always index a real action.
		if m.aCursor < 0 || m.aCursor >= len(m.actions) {
			t.Fatalf("after %d down-presses: aCursor=%d is out of bounds [0,%d)",
				step+1, m.aCursor, len(m.actions))
		}
		a := m.actions[m.aCursor]
		if a.Name == "" {
			t.Fatalf("after %d down-presses: aCursor=%d points to an action with empty Name (phantom separator?)",
				step+1, m.aCursor)
		}
	}

	// Assertion (b): find a hooks mutating action and confirm enter → screenConfirm.
	hooksIdx := -1
	for i, a := range m.actions {
		if a.Command == "install-hooks" || a.Command == "uninstall-hooks" {
			hooksIdx = i
			break
		}
	}
	if hooksIdx < 0 {
		t.Fatal("no hooks mutating action found in Actions(); required by this change")
	}

	m2 := newModel()
	m2.scr = screenActions
	m2.aCursor = hooksIdx
	updated, _ := m2.updateActions(tea.KeyMsg{Type: tea.KeyEnter})
	m2 = updated.(model)
	if m2.scr != screenConfirm {
		t.Errorf("hooks action at index %d must route to screenConfirm on enter, got screen %d",
			hooksIdx, m2.scr)
	}
}

// TestBackwardsCompatZeroValues verifies existing actions have zero-value TargetAgnostic
// and empty ConfirmMessage, preserving prior behavior (R-010 Scenario 10.4, backwards-compat).
func TestBackwardsCompatZeroValues(t *testing.T) {
	for _, a := range Actions() {
		if a.Command == "apply" || a.Command == "capture" {
			if a.TargetAgnostic {
				t.Errorf("%s must have TargetAgnostic: false (backwards compat)", a.Command)
			}
			if a.ConfirmMessage != "" {
				t.Errorf("%s must have empty ConfirmMessage (backwards compat), got %q", a.Command, a.ConfirmMessage)
			}
		}
	}
}

func TestRuntimeLifecycleActionsRegistered(t *testing.T) {
	// R-101/R-106: TUI exposes target-aware lifecycle commands documented for all runtimes.
	actions := Actions()
	byCmd := make(map[string]Action)
	for _, action := range actions {
		byCmd[action.Command] = action
	}

	for _, cmd := range []string{"install", "update", "rollback", "uninstall", "status", "sync-check"} {
		action, ok := byCmd[cmd]
		if !ok {
			t.Fatalf("Actions() must contain runtime lifecycle command %q", cmd)
		}
		if !action.SupportsAll {
			t.Errorf("%s must support --target all", cmd)
		}
	}

	for _, cmd := range []string{"install", "update", "rollback", "uninstall"} {
		if !byCmd[cmd].Mutating {
			t.Errorf("%s must require confirmation", cmd)
		}
	}
}

func TestParseRuntimeStatuses(t *testing.T) {
	// R-101/R-104: user-visible runtime statuses include supported, restart_required, partial, and unsupported.
	output := `
[claude] status: supported — Claude hooks remain the deterministic baseline
[opencode] status: restart_required — OpenCode restart required to activate plugin hash abc
[codex] status: partial — Codex support is conditional — reasons: deterministic scoped subagent/task rewrite not verified
[opencode] status: unsupported — OpenCode runtime support disabled by local configuration
`

	statuses := ParseRuntimeStatuses(output)
	if len(statuses) != 4 {
		t.Fatalf("expected 4 statuses, got %d", len(statuses))
	}
	want := []RuntimeStatus{
		{Target: "claude", Status: RuntimeSupported},
		{Target: "opencode", Status: RuntimeRestartRequired},
		{Target: "codex", Status: RuntimePartial},
		{Target: "opencode", Status: RuntimeUnsupported},
	}
	for i, status := range statuses {
		if status.Target != want[i].Target || status.Status != want[i].Status {
			t.Errorf("status[%d] = {%s %d}, want {%s %d}", i, status.Target, status.Status, want[i].Target, want[i].Status)
		}
		if status.Message == "" {
			t.Errorf("%s message should not be empty", status.Target)
		}
	}
}

func TestParseRuntimeStatusesIgnoresHookStatusLines(t *testing.T) {
	output := `
[OK  ] binary: /tmp/gentle-ai-overlay
[FAIL] contract: missing
[claude] status: partial — engine binary not installed
`

	statuses := ParseRuntimeStatuses(output)
	if len(statuses) != 1 {
		t.Fatalf("expected only runtime target status, got %#v", statuses)
	}
	if statuses[0].Target != "claude" || statuses[0].Status != RuntimePartial {
		t.Fatalf("unexpected runtime status parse result: %#v", statuses[0])
	}
}

func TestRuntimeDashboardRendersCapabilityStatuses(t *testing.T) {
	// R-106: dashboard renders target-specific limitations without overclaiming parity.
	m := newModel()
	m.width = 100
	m.height = 30
	m.scr = screenResult
	m.result = commandResult{
		action: Action{Name: "Runtime status", Command: "status"},
		output: "[opencode] status: restart_required — restart OpenCode\n[codex] status: partial — deterministic rewrite not verified\n[opencode] status: unsupported — support disabled\n",
		runtimeStatuses: []RuntimeStatus{
			{Target: "opencode", Status: RuntimeRestartRequired, Message: "restart OpenCode"},
			{Target: "codex", Status: RuntimePartial, Message: "deterministic rewrite not verified"},
			{Target: "opencode", Status: RuntimeUnsupported, Message: "support disabled"},
		},
	}

	rendered := m.View()
	for _, want := range []string{"Runtime capabilities", "restart_required", "partial", "unsupported", "opencode", "codex"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("runtime dashboard should contain %q; got:\n%s", want, rendered)
		}
	}
}

func TestRunBackendExecutesTargetAgnosticHooksWithoutTargetFlag(t *testing.T) {
	// R-102: hook actions are behavior-level target-agnostic shell invocations.
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	backend := filepath.Join(binDir, "overlay")
	if err := os.WriteFile(backend, []byte("#!/usr/bin/env bash\nprintf 'args:%s\\n' \"$*\"\n"), 0o755); err != nil {
		t.Fatalf("write fake backend: %v", err)
	}

	result := runBackend(root, Action{Command: "install-hooks", TargetAgnostic: true}, AllTargets())
	if result.err != nil {
		t.Fatalf("runBackend returned error: %v\n%s", result.err, result.output)
	}
	if !strings.Contains(result.output, "$ bin/overlay install-hooks") || strings.Contains(result.output, "--target") {
		t.Fatalf("target-agnostic hook action should execute once without --target, got:\n%s", result.output)
	}
}

func TestRunBackendParsesRuntimeAndSyncStatusFromBehaviorOutput(t *testing.T) {
	// R-101/R-104: backend execution output feeds both sync verdict and runtime status dashboards.
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	backend := filepath.Join(binDir, "overlay")
	script := `#!/usr/bin/env bash
printf 'VERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0\n'
printf 'ACTION:claude: in sync with gentle-ai (healthy)\n'
printf '[claude] status: supported — Claude hooks remain deterministic\n'
`
	if err := os.WriteFile(backend, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake backend: %v", err)
	}

	result := runBackend(root, Action{Command: "sync-check", SupportsAll: true}, AllTargets())
	if result.err != nil {
		t.Fatalf("runBackend returned error: %v\n%s", result.err, result.output)
	}
	if !strings.Contains(result.output, "$ bin/overlay sync-check --target all") {
		t.Fatalf("all-selected sync-check should use --target all, got:\n%s", result.output)
	}
	if len(result.verdicts) != 1 || result.verdicts[0].Status != SyncHealthy {
		t.Fatalf("runBackend should parse sync verdicts, got %#v", result.verdicts)
	}
	if len(result.runtimeStatuses) != 1 || result.runtimeStatuses[0].Status != RuntimeSupported {
		t.Fatalf("runBackend should parse runtime statuses, got %#v", result.runtimeStatuses)
	}
}

func TestModelConfirmAndResultNavigation(t *testing.T) {
	// R-106: runtime result screens preserve navigation and scroll behavior with dashboard content.
	m := newModel()
	m.scr = screenConfirm
	m.pendingAction = Action{Name: "Instalar runtime", Command: "install", Mutating: true}
	updated, cmd := m.updateConfirm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = updated.(model)
	if cmd != nil || m.scr != screenActions {
		t.Fatalf("negative confirmation should return to actions without a command, screen=%v cmd=%v", m.scr, cmd)
	}

	m.scr = screenResult
	m.height = 12
	m.result = commandResult{
		output: strings.Repeat("line\n", 30),
		runtimeStatuses: []RuntimeStatus{
			{Target: "opencode", Status: RuntimeRestartRequired, Message: "restart required"},
		},
	}
	updated, _ = m.updateResult(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)
	if m.scroll == 0 {
		t.Fatal("down key should scroll result output when content exceeds viewport")
	}
	updated, _ = m.updateResult(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if m.scr != screenActions || m.scroll != 0 {
		t.Fatalf("escape should return to actions and reset scroll, screen=%v scroll=%d", m.scr, m.scroll)
	}
}

// TestClassifyPrecedence verifies UPSTREAM_CHANGED wins over OVERLAY_NOT_DEPLOYED.
func TestClassifyPrecedence(t *testing.T) {
	if classify(2, 5, false) != SyncNeedsCapture {
		t.Fatal("upstream_changed>0 must classify as needs-capture (RED)")
	}
	if classify(0, 1, false) != SyncNeedsApply {
		t.Fatal("overlay_not_deployed>0 must classify as needs-apply (YELLOW)")
	}
	if classify(0, 0, false) != SyncHealthy {
		t.Fatal("zero counts must classify as healthy (GREEN)")
	}
	if classify(0, 0, true) != SyncTargetMissing {
		t.Fatal("target_missing must not classify as healthy even with zero counts")
	}
}
