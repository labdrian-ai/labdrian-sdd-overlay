package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
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
// RED: will fail until bubbles/spinner is added and embedded in model.
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

// TestOutputBoxStyleDeclared is a compile-gate: outputBoxStyle must be a
// non-zero lipgloss.Style accessible from the package.
// RED: will fail until outputBoxStyle constant is declared in view.go.
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
// RED: will fail until redundant "\n" after titleStyle.Render() calls are removed.
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

// TestWidthResponsiveRendering asserts that no rendered line exceeds the
// terminal width when a narrow terminal size is used (R-001).
func TestWidthResponsiveRendering(t *testing.T) {
	m := newModel()
	// Simulate receiving a narrow WindowSizeMsg.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	m = updated.(model)

	rendered := m.View()
	// Strip ANSI escape sequences for accurate line-width measurement.
	plain := stripANSI(rendered)
	for i, line := range strings.Split(plain, "\n") {
		runes := []rune(line)
		if len(runes) > 40 {
			t.Errorf("line %d exceeds 40 columns (%d runes): %q", i+1, len(runes), line)
		}
	}
}

// stripANSI removes ANSI escape sequences from a string for width measurement.
func stripANSI(s string) string {
	var result strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEscape = false
			}
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}

// TestScrollClamp verifies m.scroll is clamped to maxScroll() and never goes
// negative (R-005).
func TestScrollClamp(t *testing.T) {
	m := newModel()
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

	// Drive 100 down-key messages — scroll must saturate at maxScroll.
	for i := 0; i < 100; i++ {
		updated, _ := m.updateResult(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = updated.(model)
	}
	if m.scroll > max {
		t.Errorf("scroll %d exceeded maxScroll %d", m.scroll, max)
	}
	if m.scroll < 0 {
		t.Errorf("scroll went negative: %d", m.scroll)
	}

	// Drive up past zero — scroll must not go negative.
	for i := 0; i < 200; i++ {
		updated, _ := m.updateResult(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		m = updated.(model)
	}
	if m.scroll < 0 {
		t.Errorf("scroll went negative after many up-keys: %d", m.scroll)
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

	// 'a' on screenHome (via updateTargets from a different scr) should not panic.
	m2 := newModel()
	m2.scr = screenActions
	// updateTargets is only called by Update when scr==screenTargets, so calling it
	// directly here is the correct unit-test approach (no screen guard inside).
	updated, _ = m2.updateTargets(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	_ = updated // just ensure no panic
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

// TestEmptyVerdictNote verifies the dim note when sync-check produces zero verdicts (R-004).
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
}

// TestFooterLegendCorrectness verifies result screen shows esc/enter and
// confirm screen shows esc/n cancel (R-007).
func TestFooterLegendCorrectness(t *testing.T) {
	m := newModel()

	// Result screen footer.
	m.scr = screenResult
	rendered := m.View()
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
