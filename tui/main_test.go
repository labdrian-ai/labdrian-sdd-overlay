package main

import (
	"fmt"
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
		target string
		status SyncStatus
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

// TestClassifyPrecedence verifies UPSTREAM_CHANGED wins over OVERLAY_NOT_DEPLOYED.
func TestClassifyPrecedence(t *testing.T) {
	if classify(2, 5) != SyncNeedsCapture {
		t.Fatal("upstream_changed>0 must classify as needs-capture (RED)")
	}
	if classify(0, 1) != SyncNeedsApply {
		t.Fatal("overlay_not_deployed>0 must classify as needs-apply (YELLOW)")
	}
	if classify(0, 0) != SyncHealthy {
		t.Fatal("zero counts must classify as healthy (GREEN)")
	}
}
