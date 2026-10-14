package main

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/domain"
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
			m := newTestModel(t)
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
	// Start unloaded, so the targets reach the screen the way they do in the
	// program: from the catalog command Init issues.
	tm := teatest.NewTestModel(t, unloadedTestModel(t, &fakeCatalog{targets: threeCopyTargets()}), teatest.WithInitialTermSize(120, 40))

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

// ---------------------------------------------------------------------------
// Phase 3: model wiring for the launch-time origin probe (R-001) and the
// dismissible banner (R-002).
// ---------------------------------------------------------------------------

// TestInit_ReturnsNonNilCmd verifies Init() wires up the launch-time probe:
// it must return a non-nil tea.Cmd (R-001 Scenario: probe is async — the
// cmd is what bubbletea runs off the UI goroutine after the first render).
func TestInit_ReturnsNonNilCmd(t *testing.T) {
	m := newTestModel(t)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() must return a non-nil tea.Cmd so the launch-time origin probe runs")
	}
}

// TestNewModel_BehindOriginDefaultsToNA locks in the D5 zero-value guard:
// newModel must initialize behindOrigin to domain.RepoBehindOriginNA, not Go's
// zero value (which is 0 and would collapse into "confirmed 0 behind" —
// the exact R-006 bug class this field's sentinel exists to prevent).
func TestNewModel_BehindOriginDefaultsToNA(t *testing.T) {
	m := newTestModel(t)
	if m.behindOrigin != domain.RepoBehindOriginNA {
		t.Errorf("newTestModel(t).behindOrigin = %d, want domain.RepoBehindOriginNA (%d) before any probe result arrives", m.behindOrigin, domain.RepoBehindOriginNA)
	}
}

// TestUpdate_ProbeDoneMsg_SetsBehindOrigin verifies the Update() branch for
// probeDoneMsg actually assigns the delivered count onto the model —
// exercised with a non-default value so the assertion cannot pass by
// accident against the zero-value/NA default.
func TestUpdate_ProbeDoneMsg_SetsBehindOrigin(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(probeDoneMsg{behind: 5})
	m = updated.(model)
	if m.behindOrigin != 5 {
		t.Errorf("behindOrigin after probeDoneMsg{behind: 5} = %d, want 5", m.behindOrigin)
	}
}

// TestGlobalXKey_DismissesBannerOnlyWhenVisible verifies R-002's dismissal
// scenario: "x" flips bannerDismissed to true only while the banner is
// actually visible (behindOrigin>0, not yet dismissed); it is a no-op
// otherwise (behindOrigin<=0/NA), so an accidental "x" press elsewhere in
// the TUI causes no state change.
func TestGlobalXKey_DismissesBannerOnlyWhenVisible(t *testing.T) {
	t.Run("dismisses when visible", func(t *testing.T) {
		m := newTestModel(t)
		m.behindOrigin = 3
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
		m = updated.(model)
		if !m.bannerDismissed {
			t.Error("bannerDismissed must be true after 'x' while the banner is visible (behindOrigin>0)")
		}
	})

	t.Run("no-op when not behind origin", func(t *testing.T) {
		m := newTestModel(t) // behindOrigin defaults to domain.RepoBehindOriginNA
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
		m = updated.(model)
		if m.bannerDismissed {
			t.Error("bannerDismissed must stay false after 'x' when the banner was never visible (behindOrigin<=0/NA)")
		}
	})
}

// TestGlobalUKey_JumpsToSelfUpdateConfirmOnlyWhenVisible verifies the
// banner-shortcut change (menos-pasos follow-up): pressing "u" while the
// behind-origin banner is visible jumps straight to the self-update confirm
// screen, from ANY screen (global, like "x"), skipping the menu-navigation
// step entirely for the most common recovery. It must stay a no-op when the
// banner isn't visible, and must not hijack a command that is actively
// running.
func TestGlobalUKey_JumpsToSelfUpdateConfirmOnlyWhenVisible(t *testing.T) {
	pressU := func(m model) model {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
		return updated.(model)
	}

	t.Run("jumps to self-update confirm when visible", func(t *testing.T) {
		m := newTestModel(t)
		m.behindOrigin = 3
		m = pressU(m)
		if m.scr != screenConfirm {
			t.Fatalf("scr after 'u' while banner visible = %v, want screenConfirm", m.scr)
		}
		if m.pendingAction.Command != "self-update" {
			t.Errorf("pendingAction after 'u' = %+v, want Command == %q", m.pendingAction, "self-update")
		}
	})

	t.Run("works from any screen, not only the default one", func(t *testing.T) {
		m := newTestModel(t)
		m.behindOrigin = 3
		m.scr = screenResult
		m = pressU(m)
		if m.scr != screenConfirm {
			t.Errorf("scr after 'u' from screenResult = %v, want screenConfirm", m.scr)
		}
		if m.pendingAction.Command != "self-update" {
			t.Errorf("pendingAction after 'u' from screenResult = %+v, want Command == %q", m.pendingAction, "self-update")
		}
	})

	t.Run("no-op when not behind origin", func(t *testing.T) {
		m := newTestModel(t) // behindOrigin defaults to domain.RepoBehindOriginNA
		before := m.scr
		m = pressU(m)
		if m.scr != before {
			t.Errorf("scr changed to %v after 'u' with no banner visible, want unchanged %v", m.scr, before)
		}
		if m.pendingAction.Command != "" {
			t.Errorf("pendingAction set to %+v after 'u' with no banner visible, want zero value", m.pendingAction)
		}
	})

	t.Run("does not hijack a command that is actively running", func(t *testing.T) {
		m := newTestModel(t)
		m.behindOrigin = 3
		m.scr = screenRunning
		m = pressU(m)
		if m.scr != screenRunning {
			t.Errorf("scr after 'u' while screenRunning = %v, want unchanged screenRunning", m.scr)
		}
	})
}

// TestToggleSelection verifies space toggles the target under the cursor off.
func TestToggleSelection(t *testing.T) {
	m := newTestModel(t)
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

// TestSpinnerPresentOnRunning verifies a spinner glyph is present in screenRunning.
func TestSpinnerPresentOnRunning(t *testing.T) {
	m := newTestModel(t)
	// Advance into screenRunning by choosing a non-mutating action.
	action := domain.Action{Name: "Estado", Command: "status", Mutating: false, SupportsAll: true}
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
	base := newTestModel(t)
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
			m := newTestModel(t)
			// Copy the already-advanced spinner into the model so the glyph is
			// at a known non-default state and would be visible if rendered.
			m.spinner = base.spinner
			m.scr = tc.scr

			// For screenConfirm we need a pending action.
			if tc.scr == screenConfirm {
				m.pendingAction = domain.Action{Name: "Aplicar cambios", Command: "apply", Mutating: true}
			}
			// For screenResult we need a result.
			if tc.scr == screenResult {
				m.result = domain.CommandResult{
					Action: domain.Action{Name: "Estado", Command: "status"},
					Output: "some output",
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
	m := newTestModel(t)
	// Home screen (screenTargets).
	rendered := m.View()
	if strings.Contains(rendered, "\n\n\n") {
		t.Errorf("screenTargets View() must not contain triple-newline double gap, got:\n%q", rendered)
	}

	// Result screen.
	m.scr = screenResult
	m.result = domain.CommandResult{
		Action: domain.Action{Name: "Estado", Command: "status"},
		Output: "ok",
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
		m := newTestModel(t)
		updated, _ := m.Update(tea.WindowSizeMsg{Width: narrowWidth, Height: 20})
		m = updated.(model)
		m.scr = screenResult
		m.result = domain.CommandResult{
			Action: domain.Action{Name: "Estado", Command: "status"},
			Output: output,
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
				m := newTestModel(t)
				updated, _ := m.Update(tea.WindowSizeMsg{Width: narrowWidth, Height: 20})
				m = updated.(model)
				return m
			},
		},
		{
			name: "screenActions",
			prepare: func() model {
				m := newTestModel(t)
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

// TestWidthFallbackTo80 verifies that a fresh newTestModel(t) (width==0) renders at 80
// columns via the contentWidth() fallback. Must fail if the fallback is removed.
func TestWidthFallbackTo80(t *testing.T) {
	m := newTestModel(t)
	// Confirm no WindowSizeMsg has been received — width must be 0.
	if m.width != 0 {
		t.Fatalf("newTestModel(t) must have width==0 before any WindowSizeMsg, got %d", m.width)
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
	m := newTestModel(t)
	m.width = 80
	m.height = 20
	m.scr = screenResult
	// Build multi-line output that exceeds the viewport.
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = "line"
	}
	m.result = domain.CommandResult{
		Action: domain.Action{Name: "Estado", Command: "status"},
		Output: strings.Join(lines, "\n"),
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
	m := newTestModel(t)
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

// TestErrorBannerOnFailure verifies a red "Comando falló" banner when result.Err != nil,
// and that the success path does NOT contain the banner (R-003).
func TestErrorBannerOnFailure(t *testing.T) {
	m := newTestModel(t)
	m.scr = screenResult
	m.result = domain.CommandResult{
		Action: domain.Action{Name: "Aplicar cambios", Command: "apply"},
		Output: "error output",
		Err:    fmt.Errorf("exit status 1"),
	}

	rendered := m.View()
	if !strings.Contains(rendered, "Comando falló") {
		t.Errorf("error result must contain 'Comando falló', got:\n%s", rendered)
	}

	// Success path.
	m.result.Err = nil
	rendered = m.View()
	if strings.Contains(rendered, "Comando falló") {
		t.Errorf("success result must NOT contain 'Comando falló'")
	}
}

// TestDegradedBannerOnExitTwo verifies that a domain.CommandResult with exitCode==2 renders
// the YELLOW "Degradado" banner and NOT the red "Comando falló" banner (exit-2 path).
func TestDegradedBannerOnExitTwo(t *testing.T) {
	m := newTestModel(t)
	m.scr = screenResult
	m.result = domain.CommandResult{
		Action:   domain.Action{Name: "Aplicar cambios", Command: "apply"},
		Output:   "degraded output",
		Err:      fmt.Errorf("exit status 2"),
		ExitCode: 2,
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
	m := newTestModel(t)
	m.scr = screenResult
	m.result = domain.CommandResult{
		Action:   domain.Action{Name: "Verificar sincronización", Command: "sync-check"},
		Output:   "some raw output",
		Verdicts: nil,
	}

	rendered := m.View()
	if !strings.Contains(rendered, "No se pudieron analizar veredictos") {
		t.Errorf("sync-check with 0 verdicts must show dim note, got:\n%s", rendered)
	}

	// With verdicts present, note must NOT appear.
	m.result.Verdicts = []domain.TargetVerdict{{Target: "claude", Status: domain.SyncHealthy}}
	rendered = m.View()
	if strings.Contains(rendered, "No se pudieron analizar veredictos") {
		t.Errorf("sync-check with verdicts must NOT show dim note")
	}

	// NEGATIVE PATH: a non-sync-check command with zero verdicts must NOT show
	// the note — guards the Command=="sync-check" gate in viewResult.
	m.result = domain.CommandResult{
		Action:   domain.Action{Name: "Estado", Command: "status"},
		Output:   "some output",
		Verdicts: nil,
	}
	rendered = m.View()
	if strings.Contains(rendered, "No se pudieron analizar veredictos") {
		t.Errorf("non-sync-check command with 0 verdicts must NOT show dim note, got:\n%s", rendered)
	}
}

// TestFooterLegendCorrectness verifies footer key hints for all interactive screens
// (R-007). Covers targets, actions, result, and confirm screens.
func TestFooterLegendCorrectness(t *testing.T) {
	m := newTestModel(t)

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
	m.result = domain.CommandResult{
		Action: domain.Action{Name: "Estado", Command: "status"},
		Output: "ok",
	}
	rendered = m.View()
	if !strings.Contains(rendered, "esc/enter") {
		t.Errorf("result screen footer must contain 'esc/enter', got footer section in:\n%s", rendered)
	}

	// Confirm screen footer.
	m.scr = screenConfirm
	m.pendingAction = domain.Action{Name: "Aplicar cambios", Command: "apply", Mutating: true}
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

// TestConfirmMessageSelection verifies:
//   - Empty ConfirmMessage → generic fallback shown (R-010 Scenario 10.2)
//   - install-hooks ConfirmMessage → specific copy shown (R-010 Scenario 10.3)
//   - TargetAgnostic on screenConfirm → no target list shown (R-010, backwards-compat)
func TestConfirmMessageSelection(t *testing.T) {
	t.Run("empty ConfirmMessage shows generic copy", func(t *testing.T) {
		m := newTestModel(t)
		m.scr = screenConfirm
		m.pendingAction = domain.Action{
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
		// Find the install-hooks action from domain.Actions().
		var installAction domain.Action
		for _, a := range domain.Actions() {
			if a.Command == "install-hooks" {
				installAction = a
				break
			}
		}
		if installAction.Command == "" {
			t.Skip("install-hooks not yet registered in domain.Actions()")
		}

		m := newTestModel(t)
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
		m := newTestModel(t)
		m.scr = screenConfirm
		m.pendingAction = domain.Action{
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
	m := newTestModel(t)
	m.scr = screenActions
	rendered := m.View()
	if !strings.Contains(rendered, "── Hooks ──") {
		t.Errorf("screenActions must contain Hooks group header, got:\n%s", rendered)
	}
}

// TestActionGroupHeadersVisible verifies BOTH action groups are labeled with
// section headers — the operational ("Sincronización") and hooks groups. This
// locks in the discoverability fix: the top group is no longer silent.
func TestActionGroupHeadersVisible(t *testing.T) {
	m := newTestModel(t)
	m.scr = screenActions
	rendered := m.View()
	if !strings.Contains(rendered, "── Sincronización ──") {
		t.Errorf("screenActions must contain operational group header '── Sincronización ──', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "── Hooks ──") {
		t.Errorf("screenActions must contain hooks group header, got:\n%s", rendered)
	}
}

// TestActionMenuShowsHints verifies per-row purpose hints render, asserting
// presence (token-level) rather than column spacing, since the %-32s padding
// makes exact whitespace brittle.
func TestActionMenuShowsHints(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(model)
	m.scr = screenActions
	rendered := m.View()

	for _, token := range []string{
		"Aplicar cambios",
		"Despliega el overlay en los destinos",
		"Capturar (actualizar upstream)",
		"Trae cambios de upstream al overlay",
	} {
		if !strings.Contains(rendered, token) {
			t.Errorf("action menu must contain %q, got:\n%s", token, rendered)
		}
	}
}

// TestSuccessBannerOnResult verifies the green success affordance renders on the
// success path and is absent on failure (locks in the positive-affordance fix).
func TestSuccessBannerOnResult(t *testing.T) {
	m := newTestModel(t)
	m.scr = screenResult
	m.result = domain.CommandResult{
		Action: domain.Action{Name: "Estado", Command: "status"},
		Output: "ok",
	}
	rendered := m.View()
	if !strings.Contains(rendered, "✓ Completado") {
		t.Errorf("success result must contain '✓ Completado', got:\n%s", rendered)
	}

	// Failure path must NOT show the success banner.
	m.result.Err = fmt.Errorf("exit status 1")
	rendered = m.View()
	if strings.Contains(rendered, "✓ Completado") {
		t.Errorf("failure result must NOT contain '✓ Completado', got:\n%s", rendered)
	}
}

// TestEstadoSkipsConfirmAndMergesStatusHooks verifies the top-level "Estado"
// action (non-mutating) goes directly to screenRunning, not screenConfirm
// (R-009 Scenario 9.3), and that status-hooks was merged into it via Also
// rather than dropped.
func TestEstadoSkipsConfirmAndMergesStatusHooks(t *testing.T) {
	m := newTestModel(t)
	// Navigate to screenActions.
	m.scr = screenActions

	// Find the index of the top-level "status" (Estado) action.
	statusIdx := -1
	for i, a := range m.actions {
		if a.Command == "status" {
			statusIdx = i
			break
		}
	}
	if statusIdx < 0 {
		t.Fatal("status (Estado) not found in domain.Actions(); required by this change")
	}

	// Lock in the merge: status-hooks must be nested under Estado's Also.
	hasStatusHooks := false
	for _, sub := range m.actions[statusIdx].Also {
		if sub.Command == "status-hooks" {
			hasStatusHooks = true
		}
	}
	if !hasStatusHooks {
		t.Error("Estado action must have status-hooks nested under Also")
	}

	m.aCursor = statusIdx
	updated, _ := m.updateActions(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)

	if m.scr == screenConfirm {
		t.Error("Estado (read-only) must NOT go to screenConfirm")
	}
	if m.scr != screenRunning {
		t.Errorf("Estado must transition to screenRunning, got screen %d", m.scr)
	}
}

// TestInstallHooksRequiresConfirm verifies install-hooks (mutating) routes to
// screenConfirm before running (R-009 Scenario 9.4).
func TestInstallHooksRequiresConfirm(t *testing.T) {
	m := newTestModel(t)
	m.scr = screenActions

	installIdx := -1
	for i, a := range m.actions {
		if a.Command == "install-hooks" {
			installIdx = i
			break
		}
	}
	if installIdx < 0 {
		t.Fatal("install-hooks not found in domain.Actions(); hooks actions are required by this change")
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
	m := newTestModel(t)
	m.scr = screenActions

	uninstallIdx := -1
	for i, a := range m.actions {
		if a.Command == "uninstall-hooks" {
			uninstallIdx = i
			break
		}
	}
	if uninstallIdx < 0 {
		t.Fatal("uninstall-hooks not found in domain.Actions(); hooks actions are required by this change")
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
	m := newTestModel(t)
	m.scr = screenActions

	if len(m.actions) == 0 {
		t.Fatal("domain.Actions() must be non-empty")
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
		t.Fatal("no hooks mutating action found in domain.Actions(); required by this change")
	}

	m2 := newTestModel(t)
	m2.scr = screenActions
	m2.aCursor = hooksIdx
	updated, _ := m2.updateActions(tea.KeyMsg{Type: tea.KeyEnter})
	m2 = updated.(model)
	if m2.scr != screenConfirm {
		t.Errorf("hooks action at index %d must route to screenConfirm on enter, got screen %d",
			hooksIdx, m2.scr)
	}
}

// ---------------------------------------------------------------------------
// GAP 1: Surface agents in the TUI dashboard
// ---------------------------------------------------------------------------

// TestViewDashboardShowsAgentsSection verifies the Agents sub-section renders
// in viewDashboard when a verdict has AgentFiles populated.
func TestViewDashboardShowsAgentsSection(t *testing.T) {
	m := newTestModel(t)
	m.width = 80
	m.scr = screenResult
	m.result = domain.CommandResult{
		Action: domain.Action{Name: "Verificar sincronización", Command: "sync-check"},
		Verdicts: []domain.TargetVerdict{
			{
				Target:             "claude",
				Status:             domain.SyncNeedsApply,
				OverlayNotDeployed: 1,
				Action:             "run 'labdrian apply --target claude'",
				AgentFiles: []domain.AgentFileEntry{
					{Path: "agents/GADU.md", Status: "OVERLAY_NOT_DEPLOYED"},
				},
			},
		},
		Output: "ok",
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
	m := newTestModel(t)
	m.width = 80
	m.scr = screenResult
	m.result = domain.CommandResult{
		Action: domain.Action{Name: "Verificar sincronización", Command: "sync-check"},
		Verdicts: []domain.TargetVerdict{
			{Target: "opencode", Status: domain.SyncHealthy, Action: "in sync"},
		},
		Output: "ok",
	}

	rendered := stripANSI(m.viewDashboard())
	if strings.Contains(rendered, "Agents") {
		t.Errorf("viewDashboard must NOT show 'Agents' label when AgentFiles is empty, got:\n%s", rendered)
	}
}

// ---------------------------------------------------------------------------
// GAP 2: Skills registry actions
// ---------------------------------------------------------------------------

// TestSkillsActionsSectionHeaderVisible verifies a "── Skills ──" section header
// renders on screenActions when skills actions are registered.
func TestSkillsActionsSectionHeaderVisible(t *testing.T) {
	m := newTestModel(t)
	m.scr = screenActions
	rendered := m.View()
	if !strings.Contains(rendered, "── Skills ──") {
		t.Errorf("screenActions must contain '── Skills ──' section header, got:\n%s", rendered)
	}
}

// ---------------------------------------------------------------------------
// GAP 3: Action menu simplification (11 -> 7) and Also composition
// ---------------------------------------------------------------------------
// ---------------------------------------------------------------------------
// Phase 5: action entry + re-probe (R-003, D7, D5 tail).
// ---------------------------------------------------------------------------

// TestSelfUpdateConfirmScreen proves, for the ACTUAL registered self-update
// entry (sourced from domain.Actions(), not a hand-built stand-in), that
// screenConfirm SHOWS the target list — self-update now chains "apply" via
// Also (menos-pasos change), so it is no longer purely target-agnostic from
// the user's point of view even though its own primary invocation still
// receives no --target — and that the confirm copy names both "main" (the
// branch fast-forwarded) and the fact that it also deploys.
func TestSelfUpdateConfirmScreen(t *testing.T) {
	var selfUpdate domain.Action
	found := false
	for _, a := range domain.Actions() {
		if a.Command == "self-update" {
			selfUpdate = a
			found = true
			break
		}
	}
	if !found {
		t.Fatal(`domain.Actions() must contain "self-update" before its confirm screen can be exercised`)
	}

	m := newTestModel(t)
	m.scr = screenConfirm
	m.pendingAction = selfUpdate
	rendered := stripANSI(m.View())

	if !strings.Contains(rendered, "en: claude") {
		t.Errorf("self-update confirm must show the target list now that it also deploys (Also: apply), got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "main") {
		t.Errorf("self-update confirm text must name 'main' as the branch updated, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "despliega") {
		t.Errorf("self-update confirm text must mention that it also deploys, got:\n%s", rendered)
	}
}

// TestUpdate_SelfUpdateSuccess_RefiresProbe verifies the D5 tail: a
// successful self-update run (err == nil) re-fires the launch-time probe so
// the banner can self-correct against the just-refreshed cached ref. A
// failed self-update run, and a successful run of a DIFFERENT action, must
// NOT re-fire the probe (triangulation — proves the branch is gated on both
// the command AND the error, not just one of them).
func TestUpdate_SelfUpdateSuccess_RefiresProbe(t *testing.T) {
	t.Run("self-update success re-fires probe", func(t *testing.T) {
		m := newTestModel(t)
		msg := runDoneMsg{result: domain.CommandResult{
			Action: domain.Action{Command: "self-update"},
			Err:    nil,
		}}
		updated, cmd := m.Update(msg)
		m = updated.(model)
		if m.scr != screenResult {
			t.Fatalf("scr after runDoneMsg = %v, want screenResult", m.scr)
		}
		if cmd == nil {
			t.Fatal("successful self-update runDoneMsg must return a non-nil re-probe cmd")
		}
	})

	t.Run("self-update failure does not re-fire probe", func(t *testing.T) {
		m := newTestModel(t)
		msg := runDoneMsg{result: domain.CommandResult{
			Action: domain.Action{Command: "self-update"},
			Err:    errors.New("boom"),
		}}
		_, cmd := m.Update(msg)
		if cmd != nil {
			t.Error("failed self-update runDoneMsg must NOT re-fire the probe")
		}
	})

	t.Run("other action success does not re-fire probe", func(t *testing.T) {
		m := newTestModel(t)
		msg := runDoneMsg{result: domain.CommandResult{
			Action: domain.Action{Command: "apply"},
			Err:    nil,
		}}
		_, cmd := m.Update(msg)
		if cmd != nil {
			t.Error("a non-self-update runDoneMsg must NOT re-fire the probe")
		}
	})
}

// ---------------------------------------------------------------------------
// Slice 3b: overlay-release-tui-surfacing (R-011).
//
// domain.TargetVerdict gains RecordedVersion/DigestMatch/RepoBehindRelease (D2),
// classify's precedence extends to capture > apply/digest-mismatch >
// behind-release > behind-origin, the launch-time probe delivers both
// behind-origin and behind-release from one sync-check run, and domain.Actions()
// gains a per-target "Restaurar respaldo" entry gated on backup
// availability (R-003, D4).
// ---------------------------------------------------------------------------

// TestNewModel_BehindReleaseDefaultsToNA mirrors
// TestNewModel_BehindOriginDefaultsToNA for the new field: newModel must
// initialize behindRelease to domain.RepoBehindOriginNA, not Go's zero value.
func TestNewModel_BehindReleaseDefaultsToNA(t *testing.T) {
	m := newTestModel(t)
	if m.behindRelease != domain.RepoBehindOriginNA {
		t.Errorf("newTestModel(t).behindRelease = %d, want domain.RepoBehindOriginNA (%d)", m.behindRelease, domain.RepoBehindOriginNA)
	}
}

// TestUpdate_ProbeDoneMsg_SetsBehindRelease mirrors
// TestUpdate_ProbeDoneMsg_SetsBehindOrigin for the new field.
func TestUpdate_ProbeDoneMsg_SetsBehindRelease(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(probeDoneMsg{behind: 0, behindRelease: 7})
	m = updated.(model)
	if m.behindRelease != 7 {
		t.Errorf("behindRelease after probeDoneMsg{behindRelease: 7} = %d, want 7", m.behindRelease)
	}
}

// ---------------------------------------------------------------------------
// Slice 3b: domain.Actions() registration for restore and the folded-in version
// command (R-011).
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Slice 3b: restore selectability gating in updateActions (R-003).
// ---------------------------------------------------------------------------

// findAction returns the index of the action with the given Command in
// m.actions, failing the test if not found.
func findAction(t *testing.T, m model, command string) int {
	t.Helper()
	for i, a := range m.actions {
		if a.Command == command {
			return i
		}
	}
	t.Fatalf("domain.Actions() must contain a %q entry", command)
	return -1
}

// TestUpdateActions_Restore_NoBackupIsNoOp verifies R-003's negative path:
// entering "Restaurar respaldo" when the (only) selected target has zero
// backups stays on screenActions -- restore is never offered/run for a
// target with no backup to restore.
func TestUpdateActions_Restore_NoBackupIsNoOp(t *testing.T) {
	m := enterRestore(t, newTestModel(t)) // its backup query holds no backups

	if m.scr != screenActions {
		t.Errorf("scr after entering restore with zero backups = %v, want unchanged screenActions", m.scr)
	}
	if m.pendingAction.Command == "restore" {
		t.Error("pendingAction must not be set to restore when no selected target has a backup")
	}
}

// TestUpdateActions_Restore_WithBackupShowsConfirmNamingTimestampVersion
// verifies R-003's positive path, and D4's exact requirement: the confirm
// screen names the specific backup timestamp + version that will be
// restored, and states that restore overwrites deployed files, following
// the EXACT existing confirm->run->result pattern (Mutating: true ->
// screenConfirm -> y/enter -> screenRunning).
func TestUpdateActions_Restore_WithBackupShowsConfirmNamingTimestampVersion(t *testing.T) {
	m := newLoadedModel(t, &fakeCatalog{targets: threeCopyTargets()}, &fakeRestore{backed: []string{"claude"}})
	// Only "claude" selected, to make the confirm text assertion unambiguous.
	m.selected = map[int]bool{0: true, 1: false, 2: false}
	m = enterRestore(t, m)

	if m.scr != screenConfirm {
		t.Fatalf("scr after entering restore with an available backup = %v, want screenConfirm", m.scr)
	}
	if m.pendingAction.Command != "restore" {
		t.Fatalf("pendingAction.Command = %q, want restore", m.pendingAction.Command)
	}
	if !m.pendingAction.Mutating {
		t.Error("restore pendingAction must be Mutating: true")
	}

	rendered := stripANSI(m.View())
	// The planner words the backups (package app tests the timestamp and the
	// version it names); the confirm screen shows its text under the action's.
	if !strings.Contains(rendered, "BACKUPS FOR: claude") {
		t.Errorf("confirm screen must show the planner's text naming the backup being restored, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "sobrescribiendo") {
		t.Errorf("confirm screen must still state that restore overwrites deployed files, got:\n%s", rendered)
	}

	// y/enter must proceed through the SAME existing pattern into screenRunning.
	updated, cmd := m.updateConfirm(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.scr != screenRunning {
		t.Errorf("scr after confirming restore = %v, want screenRunning", m.scr)
	}
	if cmd == nil {
		t.Error("confirming restore must return a non-nil run command")
	}
}

// TestUpdateActions_Restore_PartialBackupAvailabilityAmongSelection verifies
// that when MULTIPLE targets are selected and only SOME have a backup, the
// confirm screen still proceeds (naming only the targets that do have one)
// rather than refusing the whole action.
func TestUpdateActions_Restore_PartialBackupAvailabilityAmongSelection(t *testing.T) {
	// "opencode" deliberately has no backup.
	m := newLoadedModel(t, &fakeCatalog{targets: threeCopyTargets()}, &fakeRestore{backed: []string{"claude"}})
	m.selected = map[int]bool{0: true, 1: true, 2: false} // claude + opencode
	m = enterRestore(t, m)

	if m.scr != screenConfirm {
		t.Fatalf("scr = %v, want screenConfirm when at least one selected target has a backup", m.scr)
	}
	rendered := stripANSI(m.View())
	if !strings.Contains(rendered, "claude") {
		t.Errorf("confirm screen must name claude's backup, got:\n%s", rendered)
	}

	// Regression (adversarial review, Slice 3b): the ACTUAL invocation must
	// be narrowed to the same backup-bearing subset the confirm text names.
	// Before the fix, pendingTargets didn't exist and runActionCmd recomputed
	// m.selectedTargets() at run time -- i.e. it would have invoked restore
	// against "opencode" too, which has zero backups and would fail, making
	// the runner's worst-severity aggregation misreport the whole action as
	// failed even though "claude"'s destructive restore had already
	// succeeded.
	if len(m.pendingTargets) != 1 || m.pendingTargets[0].Name != "claude" {
		t.Fatalf("pendingTargets = %+v, want exactly [claude] (the only selected target with a backup) -- restore must never be invoked against a target with zero backups", m.pendingTargets)
	}

	updated, cmd := m.updateConfirm(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.scr != screenRunning {
		t.Errorf("scr after confirming partial-availability restore = %v, want screenRunning", m.scr)
	}
	if cmd == nil {
		t.Fatal("confirming restore must return a non-nil run command")
	}
	// updateConfirm's cmd is tea.Batch(spinner.Tick, runActionCmd(...)) --
	// executing it yields a tea.BatchMsg (the un-run sub-commands), so run
	// each one to find runActionCmd's actual runDoneMsg.
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("cmd() = %T, want tea.BatchMsg", cmd())
	}
	var done runDoneMsg
	var found bool
	for _, sub := range batch {
		if sub == nil {
			continue
		}
		if d, ok := sub().(runDoneMsg); ok {
			done, found = d, true
		}
	}
	if !found {
		t.Fatal("confirming restore's batched commands never produced a runDoneMsg")
	}
	// done.result.Targets is the runner's own record of exactly which targets
	// it was invoked against (set before any subprocess runs) -- the direct
	// proof that "opencode" (zero backups) was never actually invoked, not
	// just omitted from the confirm text.
	for _, tgt := range done.result.Targets {
		if tgt.Name == "opencode" {
			t.Errorf("the runner was asked to run against opencode (zero backups); targets = %+v", done.result.Targets)
		}
	}
	if len(done.result.Targets) != 1 || done.result.Targets[0].Name != "claude" {
		t.Errorf("the runner was asked to run against targets = %+v, want exactly [claude]", done.result.Targets)
	}
}

// ---------------------------------------------------------------------------
// longterm-mem menu surface: status / install / uninstall.
//
// These three entries share one Command and differ only in Args, so every
// helper below keys by Command+Args. A byCmd-style map keyed on Command alone
// would silently assert against whichever of the three landed last.
// ---------------------------------------------------------------------------

// TestLongtermMemActionsRenderUnderTheirOwnHeader pins the one record the
// operator actually reads.
//
// viewActions emits a section header only at i==0, at the first
// TargetAgnostic row, and at the first "skills" row. The longterm-mem rows
// match none of those, so before this guard they rendered UNDER "── Skills ──"
// — an MCP server that edits three runtime configuration files, filed by the
// menu as a skill, while domain.Actions()' doc comment and the code comment beside
// the group both called it a separate lifecycle. Two records said one thing
// and the screen said another; only the screen is read.
func TestLongtermMemActionsRenderUnderTheirOwnHeader(t *testing.T) {
	m := model{actions: domain.Actions(), width: 100, height: 40}
	out := m.viewActions()

	memHeader := strings.Index(out, "── Memoria ──")
	if memHeader < 0 {
		t.Fatalf("action menu has no ── Memoria ── header, so the longterm-mem rows inherit whichever section precedes them:\n%s", out)
	}
	firstRow := strings.Index(out, "Memoria: estado")
	if firstRow < 0 {
		t.Fatalf("action menu does not render the longterm-mem status row:\n%s", out)
	}
	if memHeader > firstRow {
		t.Errorf("── Memoria ── renders AFTER the first longterm-mem row, so the rows still inherit the previous section:\n%s", out)
	}
	// The header must sit between the skills section and the memory rows —
	// otherwise it exists but the grouping it claims does not.
	if skills := strings.Index(out, "── Skills ──"); skills >= 0 && skills > memHeader {
		t.Errorf("── Skills ── renders after ── Memoria ──; the group order does not match domain.Actions():\n%s", out)
	}
}
