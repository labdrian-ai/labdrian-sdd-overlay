package main

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
)

// ansiRE matches SGR color escape sequences for ANSI-stripping in assertions.
var ansiRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

func leadingSpaces(s string) int { return len(s) - len(strings.TrimLeft(s, " ")) }

// TestConsistentLeftAxis locks in the single-axis invariant: at a fixed width,
// the menu title, the first menu row, and the footer legend all start at the
// exact same left edge (the centered content column), and the logo block is
// centered WITHIN that same column (never left of the column edge, never
// centered to the full screen independently). Verified on both the targets and
// actions screens so the gutter/base column is uniform across them.
func TestConsistentLeftAxis(t *testing.T) {
	const width = 100

	cases := []struct {
		name      string
		scr       screen
		titleText string
		footerSub string
	}{
		{"targets", screenTargets, "Seleccionar destinos", "navegar"},
		{"actions", screenActions, "Elegir una acción", "ejecutar"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newModel()
			updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
			m = updated.(model)
			m.scr = tc.scr

			lines := strings.Split(stripANSI(m.View()), "\n")

			// The cursor row ("▸ ") reveals the true column left edge — the menu
			// gutter is the only left affordance and is uniform across screens.
			titlePad, rowPad, footerPad := -1, -1, -1
			logoMin := 1 << 30
			for _, ln := range lines {
				if titlePad < 0 && strings.Contains(ln, tc.titleText) {
					titlePad = leadingSpaces(ln)
				}
				if rowPad < 0 && strings.Contains(ln, "▸") {
					rowPad = leadingSpaces(ln)
				}
				if footerPad < 0 && strings.Contains(ln, tc.footerSub) {
					footerPad = leadingSpaces(ln)
				}
				// Logo rows are the only lines made of Braille glyphs.
				if strings.TrimSpace(ln) != "" && strings.ContainsAny(ln, "⠀⣀⣧⣶⣿⢷⣦") {
					if p := leadingSpaces(ln); p < logoMin {
						logoMin = p
					}
				}
			}

			if titlePad < 0 || rowPad < 0 || footerPad < 0 || logoMin == 1<<30 {
				t.Fatalf("could not locate all anchors (title=%d row=%d footer=%d logo=%d):\n%s",
					titlePad, rowPad, footerPad, logoMin, strings.Join(lines, "\n"))
			}

			// Single axis: title, first row, and footer share the same left edge.
			if titlePad != rowPad {
				t.Errorf("title pad %d != first row pad %d — column edge not shared", titlePad, rowPad)
			}
			if titlePad != footerPad {
				t.Errorf("title pad %d != footer pad %d — column edge not shared", titlePad, footerPad)
			}

			// Logo is centered within the SAME column: its left edge never sits to
			// the left of the column, and (since it is centered) it is inset further.
			if logoMin < titlePad {
				t.Errorf("logo left edge %d is left of the column edge %d", logoMin, titlePad)
			}
			if logoMin <= titlePad {
				t.Errorf("logo edge %d should be inset past the column edge %d (centered over the column)", logoMin, titlePad)
			}
		})
	}
}

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

// TestDegradedBannerOnExitTwo verifies that a commandResult with exitCode==2 renders
// the YELLOW "Degradado" banner and NOT the red "Comando falló" banner (exit-2 path).
func TestDegradedBannerOnExitTwo(t *testing.T) {
	m := newModel()
	m.scr = screenResult
	m.result = commandResult{
		action:   Action{Name: "Aplicar cambios", Command: "apply"},
		output:   "degraded output",
		err:      fmt.Errorf("exit status 2"),
		exitCode: 2,
	}

	rendered := m.View()
	if !strings.Contains(rendered, "Degradado") {
		t.Errorf("exitCode==2 result must contain 'Degradado', got:\n%s", rendered)
	}
	if strings.Contains(rendered, "Comando falló") {
		t.Errorf("exitCode==2 result must NOT contain 'Comando falló', got:\n%s", rendered)
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
	if !strings.Contains(rendered, "n/esc") {
		t.Errorf("confirm screen footer must contain 'n/esc', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "y/enter confirmar") {
		t.Errorf("confirm screen footer must contain 'y/enter confirmar', got:\n%s", rendered)
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
			Name:    "Aplicar cambios",
			Command: "apply",
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

// TestHooksSeparatorVisible verifies the Hooks group header renders on
// screenActions when hooks actions are registered (R-009 Scenario 9.2).
func TestHooksSeparatorVisible(t *testing.T) {
	m := newModel()
	m.scr = screenActions
	rendered := m.View()
	if !strings.Contains(rendered, "── Hooks (global ~/.claude) ──") {
		t.Errorf("screenActions must contain Hooks group header, got:\n%s", rendered)
	}
}

// TestActionGroupHeadersVisible verifies BOTH action groups are labeled with
// section headers — the operational ("Sincronización") and hooks groups. This
// locks in the discoverability fix: the top group is no longer silent.
func TestActionGroupHeadersVisible(t *testing.T) {
	m := newModel()
	m.scr = screenActions
	rendered := m.View()
	if !strings.Contains(rendered, "── Sincronización ──") {
		t.Errorf("screenActions must contain operational group header '── Sincronización ──', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "── Hooks (global ~/.claude) ──") {
		t.Errorf("screenActions must contain hooks group header, got:\n%s", rendered)
	}
}

// TestActionMenuShowsHintsAndTags verifies per-row purpose hints and danger tags
// render, asserting presence and grouping (token-level) rather than column spacing,
// since the %-32s/%-40s padding makes exact whitespace brittle.
func TestActionMenuShowsHintsAndTags(t *testing.T) {
	m := newModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(model)
	m.scr = screenActions
	rendered := m.View()

	for _, token := range []string{
		"Aplicar cambios",
		"Despliega el overlay en los destinos",
		"Capturar (actualizar upstream)",
		"Trae cambios de upstream al overlay",
		"[modifica]",
		"[solo lectura]",
	} {
		if !strings.Contains(rendered, token) {
			t.Errorf("action menu must contain %q, got:\n%s", token, rendered)
		}
	}
}

// TestSuccessBannerOnResult verifies the green success affordance renders on the
// success path and is absent on failure (locks in the positive-affordance fix).
func TestSuccessBannerOnResult(t *testing.T) {
	m := newModel()
	m.scr = screenResult
	m.result = commandResult{
		action: Action{Name: "Estado", Command: "status"},
		output: "ok",
	}
	rendered := m.View()
	if !strings.Contains(rendered, "✓ Completado") {
		t.Errorf("success result must contain '✓ Completado', got:\n%s", rendered)
	}

	// Failure path must NOT show the success banner.
	m.result.err = fmt.Errorf("exit status 1")
	rendered = m.View()
	if strings.Contains(rendered, "✓ Completado") {
		t.Errorf("failure result must NOT contain '✓ Completado', got:\n%s", rendered)
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

// ---------------------------------------------------------------------------
// GAP 1: Surface agents in the TUI dashboard
// ---------------------------------------------------------------------------

// TestParseSyncCheckAgentFiles verifies that per-file agent lines under a target
// section are captured into AgentFiles on the corresponding TargetVerdict.
func TestParseSyncCheckAgentFiles(t *testing.T) {
	sample := `
=== sync-check: claude (/home/user/.claude) ===
  IN_SYNC: skills/foo
  OVERLAY_NOT_DEPLOYED: agents/GADU.md (live missing)
VERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=1
ACTION:claude: run 'overlay apply --target claude'

=== sync-check: opencode (/home/user/.config/opencode) ===
  IN_SYNC: skills/bar
VERDICT:opencode:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0
ACTION:opencode: in sync with gentle-ai (healthy)
`
	verdicts := ParseSyncCheck(sample)
	if len(verdicts) != 2 {
		t.Fatalf("expected 2 verdicts, got %d", len(verdicts))
	}

	claudeV := verdicts[0]
	if claudeV.Target != "claude" {
		t.Fatalf("first verdict target = %q, want claude", claudeV.Target)
	}
	if len(claudeV.AgentFiles) != 1 {
		t.Fatalf("claude: expected 1 agent file entry, got %d: %+v", len(claudeV.AgentFiles), claudeV.AgentFiles)
	}
	af := claudeV.AgentFiles[0]
	if af.Path != "agents/GADU.md" {
		t.Errorf("claude agent file path = %q, want %q", af.Path, "agents/GADU.md")
	}
	if af.Status != "OVERLAY_NOT_DEPLOYED" {
		t.Errorf("claude agent file status = %q, want OVERLAY_NOT_DEPLOYED", af.Status)
	}

	// skills/foo must NOT appear in AgentFiles (not an agents/ path).
	opencodeV := verdicts[1]
	if len(opencodeV.AgentFiles) != 0 {
		t.Errorf("opencode: expected 0 agent file entries, got %d: %+v", len(opencodeV.AgentFiles), opencodeV.AgentFiles)
	}
}

// TestParseSyncCheckAgentFilesAllStatuses verifies all three per-file statuses
// (IN_SYNC, OVERLAY_NOT_DEPLOYED, UPSTREAM_CHANGED) are captured correctly.
func TestParseSyncCheckAgentFilesAllStatuses(t *testing.T) {
	cases := []struct {
		line   string
		status string
	}{
		{"  IN_SYNC: agents/GADU.md", "IN_SYNC"},
		{"  OVERLAY_NOT_DEPLOYED: agents/GADU.md (live missing)", "OVERLAY_NOT_DEPLOYED"},
		{"  UPSTREAM_CHANGED: agents/GADU.md", "UPSTREAM_CHANGED"},
	}

	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			sample := "=== sync-check: claude (/home/user/.claude) ===\n" + tc.line + "\nVERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0\n"
			verdicts := ParseSyncCheck(sample)
			if len(verdicts) != 1 {
				t.Fatalf("expected 1 verdict, got %d", len(verdicts))
			}
			if len(verdicts[0].AgentFiles) != 1 {
				t.Fatalf("expected 1 agent file entry, got %d", len(verdicts[0].AgentFiles))
			}
			if verdicts[0].AgentFiles[0].Status != tc.status {
				t.Errorf("status = %q, want %q", verdicts[0].AgentFiles[0].Status, tc.status)
			}
			if verdicts[0].AgentFiles[0].Path != "agents/GADU.md" {
				t.Errorf("path = %q, want agents/GADU.md", verdicts[0].AgentFiles[0].Path)
			}
		})
	}
}

// TestViewDashboardShowsAgentsSection verifies the Agents sub-section renders
// in viewDashboard when a verdict has AgentFiles populated.
func TestViewDashboardShowsAgentsSection(t *testing.T) {
	m := newModel()
	m.width = 80
	m.scr = screenResult
	m.result = commandResult{
		action: Action{Name: "Verificar sincronización", Command: "sync-check"},
		verdicts: []TargetVerdict{
			{
				Target:             "claude",
				Status:             SyncNeedsApply,
				OverlayNotDeployed: 1,
				Action:             "run 'overlay apply --target claude'",
				AgentFiles: []AgentFileEntry{
					{Path: "agents/GADU.md", Status: "OVERLAY_NOT_DEPLOYED"},
				},
			},
		},
		output: "ok",
	}

	rendered := stripANSI(m.viewDashboard())
	if !strings.Contains(rendered, "agents/GADU.md") {
		t.Errorf("viewDashboard must show agent file path 'agents/GADU.md', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Agents") {
		t.Errorf("viewDashboard must show 'Agents' sub-section label, got:\n%s", rendered)
	}
}

// TestViewDashboardNoAgentsSectionWhenEmpty verifies the Agents sub-section is
// absent when AgentFiles is empty (no spurious label for skills-only targets).
func TestViewDashboardNoAgentsSectionWhenEmpty(t *testing.T) {
	m := newModel()
	m.width = 80
	m.scr = screenResult
	m.result = commandResult{
		action: Action{Name: "Verificar sincronización", Command: "sync-check"},
		verdicts: []TargetVerdict{
			{Target: "opencode", Status: SyncHealthy, Action: "in sync"},
		},
		output: "ok",
	}

	rendered := stripANSI(m.viewDashboard())
	if strings.Contains(rendered, "Agents") {
		t.Errorf("viewDashboard must NOT show 'Agents' label when AgentFiles is empty, got:\n%s", rendered)
	}
}

// ---------------------------------------------------------------------------
// GAP 2: Skills registry actions
// ---------------------------------------------------------------------------

// TestBuildArgSetsAppendsArgs verifies that action.Args are appended after
// action.Command for TargetAgnostic actions.
func TestBuildArgSetsAppendsArgs(t *testing.T) {
	action := Action{
		TargetAgnostic: true,
		Command:        "skills",
		Args:           []string{"validate"},
	}
	sets := buildArgSets(action, nil, false)
	if len(sets) != 1 {
		t.Fatalf("expected 1 arg set, got %d", len(sets))
	}
	args := sets[0]
	if len(args) < 2 {
		t.Fatalf("expected at least 2 args (command + verb), got %v", args)
	}
	if args[0] != "skills" {
		t.Errorf("args[0] = %q, want skills", args[0])
	}
	if args[1] != "validate" {
		t.Errorf("args[1] = %q, want validate", args[1])
	}
}

// TestBuildArgSetsNoArgsUnchanged verifies a TargetAgnostic action with no Args
// produces the same single-element arg set (backwards-compat for hooks actions).
func TestBuildArgSetsNoArgsUnchanged(t *testing.T) {
	action := Action{TargetAgnostic: true, Command: "status-hooks"}
	sets := buildArgSets(action, nil, false)
	if len(sets) != 1 || len(sets[0]) != 1 {
		t.Fatalf("expected [[status-hooks]], got %v", sets)
	}
	if sets[0][0] != "status-hooks" {
		t.Errorf("sets[0][0] = %q, want status-hooks", sets[0][0])
	}
}

// TestSkillsActionsRegistered verifies the three skills actions are in Actions()
// with correct field values.
func TestSkillsActionsRegistered(t *testing.T) {
	actions := Actions()
	byVerb := make(map[string]Action) // key: Args[0] verb
	for _, a := range actions {
		if a.Command == "skills" && len(a.Args) > 0 {
			byVerb[a.Args[0]] = a
		}
	}

	for _, verb := range []string{"validate", "list", "status"} {
		a, ok := byVerb[verb]
		if !ok {
			t.Errorf("Actions() must contain a skills action with Args[0]=%q", verb)
			continue
		}
		if !a.TargetAgnostic {
			t.Errorf("skills %s must have TargetAgnostic: true", verb)
		}
		if a.Mutating {
			t.Errorf("skills %s must have Mutating: false (read-only)", verb)
		}
		if a.Hint == "" {
			t.Errorf("skills %s must have a non-empty Hint", verb)
		}
	}
}

// TestSkillsActionsSectionHeaderVisible verifies a "── Skills ──" section header
// renders on screenActions when skills actions are registered.
func TestSkillsActionsSectionHeaderVisible(t *testing.T) {
	m := newModel()
	m.scr = screenActions
	rendered := m.View()
	if !strings.Contains(rendered, "── Skills ──") {
		t.Errorf("screenActions must contain '── Skills ──' section header, got:\n%s", rendered)
	}
}

func TestSkillRegistryRefreshRepairsCodexSDDExecutors(t *testing.T) {
	var refresh Action
	for _, a := range Actions() {
		if a.Command == "skill-registry" && len(a.Args) > 0 && a.Args[0] == "refresh" {
			refresh = a
			break
		}
	}

	if refresh.Command == "" {
		t.Fatal("Actions() must contain skill-registry refresh")
	}
	if !refresh.Mutating {
		t.Error("skill-registry refresh must require confirmation because it rewrites .atl/skill-registry.md")
	}
	if !refresh.TargetAgnostic {
		t.Error("skill-registry refresh TUI action must be target-agnostic so all-selected targets do not trigger repeated refreshes")
	}
	if refresh.SupportsAll {
		t.Error("skill-registry refresh must not use --target all")
	}
	if len(refresh.Args) != 3 || refresh.Args[0] != "refresh" || refresh.Args[1] != "--target" || refresh.Args[2] != "codex" {
		t.Errorf("skill-registry refresh TUI action must refresh and repair Codex, got Args=%v", refresh.Args)
	}
	if !strings.Contains(refresh.Hint, "sdd-*") {
		t.Errorf("skill-registry refresh hint should mention sdd-* executors, got %q", refresh.Hint)
	}
}
