package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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

// ---------------------------------------------------------------------------
// Phase 3: model wiring for the launch-time origin probe (R-001) and the
// dismissible banner (R-002).
// ---------------------------------------------------------------------------

// TestInit_ReturnsNonNilCmd verifies Init() wires up the launch-time probe:
// it must return a non-nil tea.Cmd (R-001 Scenario: probe is async — the
// cmd is what bubbletea runs off the UI goroutine after the first render).
func TestInit_ReturnsNonNilCmd(t *testing.T) {
	m := newModel()
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() must return a non-nil tea.Cmd so the launch-time origin probe runs")
	}
}

// TestNewModel_BehindOriginDefaultsToNA locks in the D5 zero-value guard:
// newModel must initialize behindOrigin to RepoBehindOriginNA, not Go's
// zero value (which is 0 and would collapse into "confirmed 0 behind" —
// the exact R-006 bug class this field's sentinel exists to prevent).
func TestNewModel_BehindOriginDefaultsToNA(t *testing.T) {
	m := newModel()
	if m.behindOrigin != RepoBehindOriginNA {
		t.Errorf("newModel().behindOrigin = %d, want RepoBehindOriginNA (%d) before any probe result arrives", m.behindOrigin, RepoBehindOriginNA)
	}
}

// TestUpdate_ProbeDoneMsg_SetsBehindOrigin verifies the Update() branch for
// probeDoneMsg actually assigns the delivered count onto the model —
// exercised with a non-default value so the assertion cannot pass by
// accident against the zero-value/NA default.
func TestUpdate_ProbeDoneMsg_SetsBehindOrigin(t *testing.T) {
	m := newModel()
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
		m := newModel()
		m.behindOrigin = 3
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
		m = updated.(model)
		if !m.bannerDismissed {
			t.Error("bannerDismissed must be true after 'x' while the banner is visible (behindOrigin>0)")
		}
	})

	t.Run("no-op when not behind origin", func(t *testing.T) {
		m := newModel() // behindOrigin defaults to RepoBehindOriginNA
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
		m := newModel()
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
		m := newModel()
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
		m := newModel() // behindOrigin defaults to RepoBehindOriginNA
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
		m := newModel()
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
VERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=0
ACTION:claude: in sync with gentle-ai (healthy)

=== sync-check: opencode ===
  OVERLAY_NOT_DEPLOYED: skills/bar (live missing)
VERDICT:opencode:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=2 REPO_BEHIND_ORIGIN=NA
ACTION:opencode: run 'overlay apply --target opencode'

=== sync-check: codex ===
  UPSTREAM_CHANGED: skills/baz
VERDICT:codex:UPSTREAM_CHANGED=3 OVERLAY_NOT_DEPLOYED=1 REPO_BEHIND_ORIGIN=5
ACTION:codex: gentle-ai sync detected: run 'overlay capture --target codex' then 'overlay apply'
`

	verdicts := ParseSyncCheck(sample)
	if len(verdicts) != 3 {
		t.Fatalf("expected 3 verdicts, got %d", len(verdicts))
	}

	want := []struct {
		target       string
		status       SyncStatus
		uc, ond, rbo int
	}{
		{"claude", SyncHealthy, 0, 0, 0},
		{"opencode", SyncNeedsApply, 0, 2, RepoBehindOriginNA},
		{"codex", SyncNeedsCapture, 3, 1, 5},
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
		if v.RepoBehindOrigin != w.rbo {
			t.Errorf("%s: RepoBehindOrigin = %d, want %d (R-002/R-004 NA->-1 parsing)", w.target, v.RepoBehindOrigin, w.rbo)
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
		// status-hooks is nested under Estado's Also slice, not top-level.
		for _, sub := range a.Also {
			byCmd[sub.Command] = sub
		}
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

// TestHooksSeparatorVisible verifies the Hooks group header renders on
// screenActions when hooks actions are registered (R-009 Scenario 9.2).
func TestHooksSeparatorVisible(t *testing.T) {
	m := newModel()
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
	m := newModel()
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

// TestEstadoSkipsConfirmAndMergesStatusHooks verifies the top-level "Estado"
// action (non-mutating) goes directly to screenRunning, not screenConfirm
// (R-009 Scenario 9.3), and that status-hooks was merged into it via Also
// rather than dropped.
func TestEstadoSkipsConfirmAndMergesStatusHooks(t *testing.T) {
	m := newModel()
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
		t.Fatal("status (Estado) not found in Actions(); required by this change")
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

// TestClassifyPrecedence verifies UPSTREAM_CHANGED wins over OVERLAY_NOT_DEPLOYED
// (and digest mismatch, D2's addition to that same tier), which in turn wins
// over REPO_BEHIND_RELEASE (D2), which wins over REPO_BEHIND_ORIGIN (R-006/D2
// full precedence: capture > apply/digest-mismatch > behind-release >
// behind-origin).
func TestClassifyPrecedence(t *testing.T) {
	if classify(2, 5, 0, 0, false) != SyncNeedsCapture {
		t.Fatal("upstream_changed>0 must classify as needs-capture (RED)")
	}
	if classify(0, 1, 0, 0, false) != SyncNeedsApply {
		t.Fatal("overlay_not_deployed>0 must classify as needs-apply (YELLOW)")
	}
	if classify(0, 0, 0, 0, true) != SyncNeedsApply {
		t.Fatal("digest mismatch alone (D2) must classify as needs-apply (YELLOW), same tier as overlay_not_deployed>0")
	}
	if classify(0, 0, 0, 3, false) != SyncBehindRelease {
		t.Fatal("repo_behind_release>0 alone (D2) must classify as SyncBehindRelease")
	}
	if classify(0, 0, 3, 2, false) != SyncBehindRelease {
		t.Fatal("repo_behind_release>0 must outrank repo_behind_origin>0 (D2 precedence)")
	}
	if classify(0, 0, 3, 0, false) != SyncBehindOrigin {
		t.Fatal("repo_behind_origin>0 alone must classify as SyncBehindOrigin (R-006: never silently healthy)")
	}
	if classify(0, 0, 0, 0, false) != SyncHealthy {
		t.Fatal("zero counts and no digest mismatch must classify as healthy (GREEN)")
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

// TestSkillsActionsRegistered verifies the single top-level "skills" action
// (status verb) is registered with correct field values, and that validate/
// list are merged into it via Also rather than appearing as separate
// top-level entries.
func TestSkillsActionsRegistered(t *testing.T) {
	var skills Action
	found := false
	for _, a := range Actions() {
		if a.Command == "skills" {
			skills = a
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Actions() must contain a top-level skills action")
	}
	if len(skills.Args) == 0 || skills.Args[0] != "status" {
		t.Errorf("top-level skills action Args[0] = %v, want [status ...]", skills.Args)
	}
	if !skills.TargetAgnostic {
		t.Error("skills action must have TargetAgnostic: true")
	}
	if skills.Mutating {
		t.Error("skills action must have Mutating: false (read-only)")
	}
	if skills.Hint == "" {
		t.Error("skills action must have a non-empty Hint")
	}

	if len(skills.Also) != 2 {
		t.Fatalf("skills action must have exactly 2 Also entries, got %d: %+v", len(skills.Also), skills.Also)
	}
	byVerb := make(map[string]Action)
	for _, sub := range skills.Also {
		if sub.Command != "skills" {
			t.Errorf("Also entry must have Command %q, got %q", "skills", sub.Command)
		}
		if len(sub.Args) == 0 {
			t.Fatalf("Also entry must have a non-empty Args, got %+v", sub)
		}
		byVerb[sub.Args[0]] = sub
	}
	for _, verb := range []string{"validate", "list"} {
		sub, ok := byVerb[verb]
		if !ok {
			t.Errorf("skills Also must contain a verb %q, got %+v", verb, skills.Also)
			continue
		}
		if !sub.TargetAgnostic {
			t.Errorf("skills %s (Also) must have TargetAgnostic: true", verb)
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

// TestSkillRegistryRefreshNotInTUIMenu is a regression guard: the
// "Actualizar registry SDD Codex" (skill-registry refresh) action was
// intentionally removed from the TUI menu as part of the 11->7 simplification.
// The backend capability (bin/labdrian-overlay skill-registry refresh) is
// untouched and remains reachable via the bash CLI directly — only its TUI
// menu entry is dropped, and it must not exist anywhere in the menu, top-level
// or nested under Also.
func TestSkillRegistryRefreshNotInTUIMenu(t *testing.T) {
	for _, a := range Actions() {
		if a.Command == "skill-registry" {
			t.Errorf("Actions() must NOT contain a top-level skill-registry action, found %+v", a)
		}
		for _, sub := range a.Also {
			if sub.Command == "skill-registry" {
				t.Errorf("Actions() must NOT contain a nested skill-registry action, found %+v under %q", sub, a.Name)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// GAP 3: Action menu simplification (11 -> 7) and Also composition
// ---------------------------------------------------------------------------

// TestAllArgSetsFlattensPrimaryThenAlso verifies allArgSets returns the
// primary action's arg set(s) followed by each Also entry's arg set(s), in
// that order.
func TestAllArgSetsFlattensPrimaryThenAlso(t *testing.T) {
	action := Action{
		Command:     "status",
		SupportsAll: true,
		Also: []Action{
			{Command: "status-hooks", TargetAgnostic: true},
		},
	}
	targets := []Target{{Name: "claude", Path: "/some/path"}}

	sets := allArgSets(action, targets, false)
	if len(sets) != 2 {
		t.Fatalf("expected 2 arg sets, got %d: %v", len(sets), sets)
	}
	if sets[0][0] != "status" {
		t.Errorf("first arg set must start with %q, got %v", "status", sets[0])
	}
	want := []string{"status-hooks"}
	if len(sets[1]) != len(want) || sets[1][0] != want[0] {
		t.Errorf("second arg set = %v, want %v", sets[1], want)
	}
}

// TestAllArgSetsSkillsActionComposition verifies allArgSets on the actual
// Skills action from Actions() yields the status/validate/list arg sets in
// order.
func TestAllArgSetsSkillsActionComposition(t *testing.T) {
	var skills Action
	for _, a := range Actions() {
		if a.Command == "skills" {
			skills = a
			break
		}
	}
	if skills.Command == "" {
		t.Fatal("Actions() must contain a top-level skills action")
	}

	sets := allArgSets(skills, nil, false)
	want := [][]string{
		{"skills", "status"},
		{"skills", "validate"},
		{"skills", "list"},
	}
	if len(sets) != len(want) {
		t.Fatalf("expected %d arg sets, got %d: %v", len(want), len(sets), sets)
	}
	for i, w := range want {
		if len(sets[i]) != len(w) {
			t.Fatalf("arg set %d = %v, want %v", i, sets[i], w)
		}
		for j := range w {
			if sets[i][j] != w[j] {
				t.Errorf("arg set %d = %v, want %v", i, sets[i], w)
				break
			}
		}
	}
}

// TestActionMenuShapeAndOrder locks in the 11->7 simplification (now 9 with
// this slice's "restore" addition): top-level actions in the order matching
// the "Verificar → Capturar → Aplicar → Restaurar" flow copy (viewActions),
// with Capturar coming before Aplicar and restore immediately after apply
// (R-011: a recovery action belongs next to the deploy action it undoes).
func TestActionMenuShapeAndOrder(t *testing.T) {
	actions := Actions()
	if len(actions) != 9 {
		t.Fatalf("expected 9 top-level actions, got %d: %v", len(actions), actions)
	}

	want := []string{"status", "sync-check", "capture", "apply", "restore", "self-update", "install-hooks", "uninstall-hooks", "skills"}
	got := make([]string, len(actions))
	for i, a := range actions {
		got[i] = a.Command
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("action %d: Command = %q, want %q (full sequence: %v)", i, got[i], w, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Phase 2: launch-time origin probe (R-001). probeBehind is the pure
// extraction helper — it reuses ParseSyncCheck rather than re-deriving
// REPO_BEHIND_ORIGIN detection (spec: "MUST NOT redefine or duplicate
// sync-check-verdicts' detection logic").
// ---------------------------------------------------------------------------

// TestProbeBehind covers the table from design.md's Testing Strategy: a
// concrete count, an explicit NA, zero verdicts (e.g. every target dir
// missing so sync-check emits no VERDICT line), and garbage/unparseable
// output. Every failure mode must degrade to RepoBehindOriginNA rather than
// Go's zero value, which would collapse into "confirmed 0 behind" (the
// R-006 bug class ParseSyncCheck already guards against).
func TestProbeBehind(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   int
	}{
		{
			name: "concrete count",
			output: "\n=== sync-check: claude ===\n" +
				"VERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=4\n" +
				"ACTION:claude: run 'git pull' to update your local clone\n",
			want: 4,
		},
		{
			name: "explicit NA",
			output: "\n=== sync-check: claude ===\n" +
				"VERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=NA\n" +
				"ACTION:claude: in sync with gentle-ai (healthy)\n",
			want: RepoBehindOriginNA,
		},
		{
			name:   "zero verdicts (e.g. every target dir missing)",
			output: "\nSYNC_CHECK:claude: target dir not found -- skipping\n",
			want:   RepoBehindOriginNA,
		},
		{
			name:   "garbage output",
			output: "not even close to a sync-check line\n\x00\xff binary noise",
			want:   RepoBehindOriginNA,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := probeBehind(tc.output)
			if got != tc.want {
				t.Errorf("probeBehind(%q) = %d, want %d", tc.name, got, tc.want)
			}
		})
	}
}

// writeStubBackend installs a fake bin/labdrian-overlay under root that
// records its invocation (cwd + args) to recorderPath, prints stdout, and
// exits with exitCode — used to prove probeBehindOriginCmd's contract
// (no --fetch, cmd.Dir=root, output fed through even on nonzero exit)
// without touching a real git repo or the real backend script.
func writeStubBackend(t *testing.T, root, recorderPath, stdout string, exitCode int) {
	t.Helper()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	script := "#!/bin/sh\n" +
		"printf '%s|%s\\n' \"$PWD\" \"$*\" >> " + strconv.Quote(recorderPath) + "\n" +
		"cat <<'STUBEOF'\n" + stdout + "\nSTUBEOF\n" +
		"exit " + strconv.Itoa(exitCode) + "\n"
	if err := os.WriteFile(filepath.Join(binDir, "labdrian-overlay"), []byte(script), 0o755); err != nil {
		t.Fatalf("write stub backend: %v", err)
	}
}

// TestProbeBehindOriginCmd proves probeBehindOriginCmd's exec contract
// (D4): no --fetch/--check-origin flag (cached-only probe), cmd.Dir=root,
// and the output is fed to probeBehind even when the process exits
// non-zero — the probe's job is reading a cached ref, not requiring a clean
// exit.
func TestProbeBehindOriginCmd(t *testing.T) {
	root := t.TempDir()
	recorder := filepath.Join(root, "invoked.log")
	const sample = "\n=== sync-check: claude ===\n" +
		"VERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=7\n" +
		"ACTION:claude: run 'git pull' to update your local clone\n"
	writeStubBackend(t, root, recorder, sample, 1) // nonzero exit on purpose

	cmd := probeBehindOriginCmd(root)
	if cmd == nil {
		t.Fatal("probeBehindOriginCmd must return a non-nil tea.Cmd")
	}
	msg := cmd()

	pd, ok := msg.(probeDoneMsg)
	if !ok {
		t.Fatalf("expected probeDoneMsg, got %T", msg)
	}
	if pd.behind != 7 {
		t.Errorf("behind = %d, want 7 (output must be parsed even though the stub exits nonzero)", pd.behind)
	}

	data, err := os.ReadFile(recorder)
	if err != nil {
		t.Fatalf("read recorder (backend was never invoked?): %v", err)
	}
	invoked := strings.TrimSpace(string(data))
	if strings.Contains(invoked, "--fetch") || strings.Contains(invoked, "--check-origin") {
		t.Errorf("probeBehindOriginCmd must not pass --fetch/--check-origin (cached-only probe per R-001), got invocation: %q", invoked)
	}
	pwdPart, _, _ := strings.Cut(invoked, "|")
	gotDir, _ := filepath.EvalSymlinks(pwdPart)
	wantDir, _ := filepath.EvalSymlinks(root)
	if gotDir == "" || gotDir != wantDir {
		t.Errorf("cmd.Dir mismatch: backend ran with cwd %q, want %q", pwdPart, root)
	}
}

// ---------------------------------------------------------------------------
// Phase 5: action entry + re-probe (R-003, D7, D5 tail).
// ---------------------------------------------------------------------------

// TestSelfUpdateActionRegistered verifies Actions() registers the
// "Actualizar repositorio" entry (R-003) with TargetAgnostic and Mutating
// both true, positioned immediately after "restore" (this slice inserted
// restore between apply and self-update, R-011) and immediately before
// "install-hooks" per design.md D7's repo-maintenance grouping.
func TestSelfUpdateActionRegistered(t *testing.T) {
	actions := Actions()
	idx := -1
	for i, a := range actions {
		if a.Command == "self-update" {
			idx = i
			break
		}
	}
	if idx == -1 {
		t.Fatal(`Actions() must contain an entry with Command "self-update"`)
	}

	a := actions[idx]
	if a.Name != "Actualizar repositorio" {
		t.Errorf("self-update Name = %q, want %q", a.Name, "Actualizar repositorio")
	}
	if !a.Mutating {
		t.Error("self-update must have Mutating: true")
	}
	if !a.TargetAgnostic {
		t.Error("self-update must have TargetAgnostic: true")
	}

	prev := ""
	if idx > 0 {
		prev = actions[idx-1].Command
	}
	if prev != "restore" {
		t.Errorf("self-update must come immediately after restore, got previous entry %q", prev)
	}

	next := ""
	if idx+1 < len(actions) {
		next = actions[idx+1].Command
	}
	if next != "install-hooks" {
		t.Errorf("self-update must come immediately before install-hooks, got next entry %q", next)
	}
}

// TestSelfUpdateConfirmScreen proves, for the ACTUAL registered self-update
// entry (sourced from Actions(), not a hand-built stand-in), that
// screenConfirm SHOWS the target list — self-update now chains "apply" via
// Also (menos-pasos change), so it is no longer purely target-agnostic from
// the user's point of view even though its own primary invocation still
// receives no --target — and that the confirm copy names both "main" (the
// branch fast-forwarded) and the fact that it also deploys.
func TestSelfUpdateConfirmScreen(t *testing.T) {
	var selfUpdate Action
	found := false
	for _, a := range Actions() {
		if a.Command == "self-update" {
			selfUpdate = a
			found = true
			break
		}
	}
	if !found {
		t.Fatal(`Actions() must contain "self-update" before its confirm screen can be exercised`)
	}

	m := newModel()
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

// TestSelfUpdateActionChainsApply verifies the self-update entry merges
// "apply" into it via Also (menos-pasos change): running "Actualizar
// repositorio" once both fast-forwards main AND deploys it to the selected
// targets, collapsing what used to be two separate manual steps (self-update,
// then remembering to run apply) into one. Mirrors the existing skills/status
// Also-composition pattern (TestSkillsActionsRegistered) rather than a
// hand-rolled shape.
func TestSelfUpdateActionChainsApply(t *testing.T) {
	var selfUpdate Action
	found := false
	for _, a := range Actions() {
		if a.Command == "self-update" {
			selfUpdate = a
			found = true
			break
		}
	}
	if !found {
		t.Fatal(`Actions() must contain a top-level self-update action`)
	}

	if len(selfUpdate.Also) != 1 {
		t.Fatalf("self-update action must have exactly 1 Also entry (apply), got %d: %+v", len(selfUpdate.Also), selfUpdate.Also)
	}
	apply := selfUpdate.Also[0]
	if apply.Command != "apply" {
		t.Errorf("self-update's Also entry Command = %q, want %q", apply.Command, "apply")
	}
	if !apply.SupportsAll {
		t.Error("chained apply must have SupportsAll: true, so every selected target deploys in one invocation")
	}
	if apply.TargetAgnostic {
		t.Error("chained apply must NOT be TargetAgnostic -- it deploys to the selected targets")
	}

	if !selfUpdate.usesTargets() {
		t.Error("self-update must report usesTargets() == true once apply is chained onto it via Also")
	}
}

// TestAllArgSetsSelfUpdateActionComposition verifies allArgSets on the
// actual registered self-update action: the primary self-update invocation
// (no --target, TargetAgnostic) followed by the chained apply invocation for
// the selected targets — the exact sequence the TUI now runs from a single
// button press.
func TestAllArgSetsSelfUpdateActionComposition(t *testing.T) {
	var selfUpdate Action
	for _, a := range Actions() {
		if a.Command == "self-update" {
			selfUpdate = a
			break
		}
	}
	if selfUpdate.Command == "" {
		t.Fatal("Actions() must contain a top-level self-update action")
	}

	targets := []Target{{Name: "claude", Path: "/some/path"}, {Name: "opencode", Path: "/other/path"}}

	t.Run("all targets selected -> apply --target all", func(t *testing.T) {
		sets := allArgSets(selfUpdate, targets, true)
		want := [][]string{
			{"self-update"},
			{"apply", "--target", "all"},
		}
		if len(sets) != len(want) {
			t.Fatalf("expected %d arg sets, got %d: %v", len(want), len(sets), sets)
		}
		for i, w := range want {
			if strings.Join(sets[i], " ") != strings.Join(w, " ") {
				t.Errorf("arg set %d = %v, want %v", i, sets[i], w)
			}
		}
	})

	t.Run("one target selected -> apply --target <name>", func(t *testing.T) {
		sets := allArgSets(selfUpdate, targets[:1], false)
		want := [][]string{
			{"self-update"},
			{"apply", "--target", "claude"},
		}
		if len(sets) != len(want) {
			t.Fatalf("expected %d arg sets, got %d: %v", len(want), len(sets), sets)
		}
		for i, w := range want {
			if strings.Join(sets[i], " ") != strings.Join(w, " ") {
				t.Errorf("arg set %d = %v, want %v", i, sets[i], w)
			}
		}
	})
}

// TestUpdate_SelfUpdateSuccess_RefiresProbe verifies the D5 tail: a
// successful self-update run (err == nil) re-fires the launch-time probe so
// the banner can self-correct against the just-refreshed cached ref. A
// failed self-update run, and a successful run of a DIFFERENT action, must
// NOT re-fire the probe (triangulation — proves the branch is gated on both
// the command AND the error, not just one of them).
func TestUpdate_SelfUpdateSuccess_RefiresProbe(t *testing.T) {
	t.Run("self-update success re-fires probe", func(t *testing.T) {
		m := newModel()
		msg := runDoneMsg{result: commandResult{
			action: Action{Command: "self-update"},
			err:    nil,
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
		m := newModel()
		msg := runDoneMsg{result: commandResult{
			action: Action{Command: "self-update"},
			err:    errors.New("boom"),
		}}
		_, cmd := m.Update(msg)
		if cmd != nil {
			t.Error("failed self-update runDoneMsg must NOT re-fire the probe")
		}
	})

	t.Run("other action success does not re-fire probe", func(t *testing.T) {
		m := newModel()
		msg := runDoneMsg{result: commandResult{
			action: Action{Command: "apply"},
			err:    nil,
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
// TargetVerdict gains RecordedVersion/DigestMatch/RepoBehindRelease (D2),
// classify's precedence extends to capture > apply/digest-mismatch >
// behind-release > behind-origin, the launch-time probe delivers both
// behind-origin and behind-release from one sync-check run, and Actions()
// gains a per-target "Restaurar respaldo" entry gated on backup
// availability (R-003, D4).
// ---------------------------------------------------------------------------

// TestParseSyncCheck_ReleaseFields verifies the three new VERDICT keys parse
// onto TargetVerdict, including the REPO_BEHIND_RELEASE NA sentinel and the
// resulting classify() precedence (digest mismatch -> needs-apply here,
// since OVERLAY_NOT_DEPLOYED=0 but DIGEST_MATCH=no).
func TestParseSyncCheck_ReleaseFields(t *testing.T) {
	sample := `
=== sync-check: claude ===
VERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=0 REPO_BEHIND_RELEASE=2 RECORDED_VERSION=v1.3.0 DIGEST_MATCH=no
ACTION:claude: run 'overlay apply --target claude' (release v1.4.0 available)

=== sync-check: opencode ===
VERDICT:opencode:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=0 REPO_BEHIND_RELEASE=0 RECORDED_VERSION=v1.4.0 DIGEST_MATCH=yes
ACTION:opencode: in sync with gentle-ai at v1.4.0

=== sync-check: codex ===
VERDICT:codex:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=1 REPO_BEHIND_ORIGIN=NA REPO_BEHIND_RELEASE=NA RECORDED_VERSION=NA DIGEST_MATCH=NA
ACTION:codex: run 'overlay apply --target codex'
`
	verdicts := ParseSyncCheck(sample)
	if len(verdicts) != 3 {
		t.Fatalf("expected 3 verdicts, got %d", len(verdicts))
	}

	claude := verdicts[0]
	if claude.RecordedVersion != "v1.3.0" {
		t.Errorf("claude.RecordedVersion = %q, want v1.3.0", claude.RecordedVersion)
	}
	if claude.DigestMatch != "no" {
		t.Errorf("claude.DigestMatch = %q, want no", claude.DigestMatch)
	}
	if claude.RepoBehindRelease != 2 {
		t.Errorf("claude.RepoBehindRelease = %d, want 2", claude.RepoBehindRelease)
	}
	if claude.Status != SyncNeedsApply {
		t.Errorf("claude.Status = %d, want SyncNeedsApply (digest mismatch alone must classify as needs-apply, D2)", claude.Status)
	}

	opencode := verdicts[1]
	if opencode.RepoBehindRelease != 0 {
		t.Errorf("opencode.RepoBehindRelease = %d, want 0", opencode.RepoBehindRelease)
	}
	if opencode.Status != SyncHealthy {
		t.Errorf("opencode.Status = %d, want SyncHealthy", opencode.Status)
	}

	codex := verdicts[2]
	if codex.RepoBehindRelease != RepoBehindOriginNA {
		t.Errorf("codex.RepoBehindRelease = %d, want RepoBehindOriginNA (NA parsing)", codex.RepoBehindRelease)
	}
	if codex.RecordedVersion != "NA" || codex.DigestMatch != "NA" {
		t.Errorf("codex RecordedVersion/DigestMatch = %q/%q, want NA/NA (never deployed)", codex.RecordedVersion, codex.DigestMatch)
	}
}

// TestParseSyncCheck_ReleaseFieldsAbsentDefaultsBackwardsCompatible verifies
// a legacy VERDICT line without the new keys (pre-Slice-3b sync-check
// output, or a hand-built verdict in an older test) leaves RepoBehindRelease
// at the NA sentinel and RecordedVersion/DigestMatch empty, never a
// fabricated zero/healthy value.
func TestParseSyncCheck_ReleaseFieldsAbsentDefaultsBackwardsCompatible(t *testing.T) {
	sample := "\n=== sync-check: claude ===\nVERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=0\nACTION:claude: in sync with gentle-ai (healthy)\n"
	verdicts := ParseSyncCheck(sample)
	if len(verdicts) != 1 {
		t.Fatalf("expected 1 verdict, got %d", len(verdicts))
	}
	v := verdicts[0]
	if v.RepoBehindRelease != RepoBehindOriginNA {
		t.Errorf("RepoBehindRelease = %d, want RepoBehindOriginNA when the key is absent", v.RepoBehindRelease)
	}
	if v.RecordedVersion != "" || v.DigestMatch != "" {
		t.Errorf("RecordedVersion/DigestMatch = %q/%q, want empty when the keys are absent", v.RecordedVersion, v.DigestMatch)
	}
	if v.Status != SyncHealthy {
		t.Errorf("Status = %d, want SyncHealthy for a legacy all-zero verdict", v.Status)
	}
}

// TestProbeBehindRelease mirrors TestProbeBehind's table for the D2
// counterpart: a concrete count, an explicit NA, zero verdicts, and
// unparseable output all handled with the same degrade-to-NA contract.
func TestProbeBehindRelease(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   int
	}{
		{
			name: "concrete count",
			output: "\n=== sync-check: claude ===\n" +
				"VERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_RELEASE=3\n" +
				"ACTION:claude: run 'overlay self-update'\n",
			want: 3,
		},
		{
			name: "explicit NA",
			output: "\n=== sync-check: claude ===\n" +
				"VERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_RELEASE=NA\n" +
				"ACTION:claude: (no releases published yet)\n",
			want: RepoBehindOriginNA,
		},
		{
			name:   "zero verdicts",
			output: "\nSYNC_CHECK:claude: target dir not found -- skipping\n",
			want:   RepoBehindOriginNA,
		},
		{
			name:   "garbage output",
			output: "not even close to a sync-check line\n\x00\xff binary noise",
			want:   RepoBehindOriginNA,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := probeBehindRelease(tc.output)
			if got != tc.want {
				t.Errorf("probeBehindRelease(%q) = %d, want %d", tc.name, got, tc.want)
			}
		})
	}
}

// TestProbeBehindOriginCmd_AlsoDeliversBehindRelease proves the D2 wiring:
// one sync-check invocation feeds both probeDoneMsg fields, so no second
// exec is spent picking up the release-behind count.
func TestProbeBehindOriginCmd_AlsoDeliversBehindRelease(t *testing.T) {
	root := t.TempDir()
	recorder := filepath.Join(root, "invoked.log")
	const sample = "\n=== sync-check: claude ===\n" +
		"VERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=1 REPO_BEHIND_RELEASE=4\n" +
		"ACTION:claude: run 'overlay self-update'\n"
	writeStubBackend(t, root, recorder, sample, 0)

	msg := probeBehindOriginCmd(root)()
	pd, ok := msg.(probeDoneMsg)
	if !ok {
		t.Fatalf("expected probeDoneMsg, got %T", msg)
	}
	if pd.behind != 1 {
		t.Errorf("behind = %d, want 1", pd.behind)
	}
	if pd.behindRelease != 4 {
		t.Errorf("behindRelease = %d, want 4", pd.behindRelease)
	}
}

// TestNewModel_BehindReleaseDefaultsToNA mirrors
// TestNewModel_BehindOriginDefaultsToNA for the new field: newModel must
// initialize behindRelease to RepoBehindOriginNA, not Go's zero value.
func TestNewModel_BehindReleaseDefaultsToNA(t *testing.T) {
	m := newModel()
	if m.behindRelease != RepoBehindOriginNA {
		t.Errorf("newModel().behindRelease = %d, want RepoBehindOriginNA (%d)", m.behindRelease, RepoBehindOriginNA)
	}
}

// TestUpdate_ProbeDoneMsg_SetsBehindRelease mirrors
// TestUpdate_ProbeDoneMsg_SetsBehindOrigin for the new field.
func TestUpdate_ProbeDoneMsg_SetsBehindRelease(t *testing.T) {
	m := newModel()
	updated, _ := m.Update(probeDoneMsg{behind: 0, behindRelease: 7})
	m = updated.(model)
	if m.behindRelease != 7 {
		t.Errorf("behindRelease after probeDoneMsg{behindRelease: 7} = %d, want 7", m.behindRelease)
	}
}

// ---------------------------------------------------------------------------
// Slice 3b: latestBackup (D3/D4) — pure filesystem read, no backend exec.
// ---------------------------------------------------------------------------

// writeBackupFixture creates a backup directory for target at the given
// timestamp under home, with metaContent written to its .meta file (skipped
// when metaContent is the sentinel noMeta).
const noMeta = "\x00__no_meta__"

func writeBackupFixture(t *testing.T, home, target, timestamp, metaContent string) {
	t.Helper()
	dir := filepath.Join(home, ".labdrian-overlay", "backups", target, timestamp)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir backup fixture: %v", err)
	}
	if metaContent != noMeta {
		if err := os.WriteFile(filepath.Join(dir, ".meta"), []byte(metaContent), 0o644); err != nil {
			t.Fatalf("write .meta fixture: %v", err)
		}
	}
}

// TestLatestBackup_NoBackupsReturnsNotOK verifies a target with zero
// retained backups (or no backups directory at all) reports ok=false — the
// exact signal restore-selectability gating depends on (R-003).
func TestLatestBackup_NoBackupsReturnsNotOK(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, _, ok := latestBackup("claude"); ok {
		t.Error("latestBackup() for a target with no backups directory must return ok=false")
	}
}

// TestLatestBackup_PicksLexicallyLastAsMostRecent verifies multiple backups
// resolve to the chronologically newest one (D4: TUI always targets the
// most recent backup only), and that its recorded version is read from
// .meta (tab-separated: version, digest, applied_at).
func TestLatestBackup_PicksLexicallyLastAsMostRecent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeBackupFixture(t, home, "claude", "20260101T000000Z", "v1.2.0\tabc123\t2026-01-01T00:00:00Z")
	writeBackupFixture(t, home, "claude", "20260215T120000Z", "v1.3.0\tdef456\t2026-02-15T12:00:00Z")

	ts, version, ok := latestBackup("claude")
	if !ok {
		t.Fatal("latestBackup() must report ok=true when backups exist")
	}
	if ts != "20260215T120000Z" {
		t.Errorf("timestamp = %q, want the lexically/chronologically last one", ts)
	}
	if version != "v1.3.0" {
		t.Errorf("version = %q, want v1.3.0 (from the most recent backup's .meta)", version)
	}
}

// TestLatestBackup_NeverDeployedMetaStillReportsOK verifies a backup whose
// .meta is the literal "NEVER_DEPLOYED" sentinel (the backup was taken
// while the target had no prior recorded version) still reports ok=true —
// the backup itself is restorable, only the version label degrades to
// "unknown".
func TestLatestBackup_NeverDeployedMetaStillReportsOK(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeBackupFixture(t, home, "claude", "20260101T000000Z", "NEVER_DEPLOYED")

	ts, version, ok := latestBackup("claude")
	if !ok {
		t.Fatal("latestBackup() must report ok=true even when .meta is NEVER_DEPLOYED")
	}
	if ts != "20260101T000000Z" {
		t.Errorf("timestamp = %q, want 20260101T000000Z", ts)
	}
	if version == "v1.2.0" {
		t.Errorf("version must not fabricate a real version from a NEVER_DEPLOYED meta, got %q", version)
	}
}

// TestLatestBackup_MissingMetaStillReportsOK verifies a backup directory
// with no .meta file at all (corrupt/partial write) still reports ok=true
// with a degraded version label, mirroring cmd_restore --list's own
// "unknown" fallback rather than erroring out.
func TestLatestBackup_MissingMetaStillReportsOK(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeBackupFixture(t, home, "claude", "20260101T000000Z", noMeta)

	_, _, ok := latestBackup("claude")
	if !ok {
		t.Fatal("latestBackup() must report ok=true even when .meta is missing")
	}
}

// TestLatestBackup_TargetIsolation verifies one target's backups never leak
// into another target's lookup.
func TestLatestBackup_TargetIsolation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeBackupFixture(t, home, "claude", "20260101T000000Z", "v1.0.0\tabc\t2026-01-01T00:00:00Z")

	if _, _, ok := latestBackup("opencode"); ok {
		t.Error("latestBackup(\"opencode\") must be ok=false when only \"claude\" has backups")
	}
}

// ---------------------------------------------------------------------------
// Slice 3b: Actions() registration for restore and the folded-in version
// command (R-011).
// ---------------------------------------------------------------------------

// TestRestoreActionRegistered verifies the "Restaurar respaldo" entry:
// Mutating, per-target (never SupportsAll, mirroring capture's --target-all
// refusal), and carrying a non-empty overwrite-warning ConfirmMessage.
func TestRestoreActionRegistered(t *testing.T) {
	var restore Action
	found := false
	for _, a := range Actions() {
		if a.Command == "restore" {
			restore = a
			found = true
			break
		}
	}
	if !found {
		t.Fatal(`Actions() must contain a top-level "restore" action`)
	}
	if !restore.Mutating {
		t.Error("restore must have Mutating: true")
	}
	if restore.SupportsAll {
		t.Error("restore must have SupportsAll: false (the backend refuses --target all for restore)")
	}
	if restore.TargetAgnostic {
		t.Error("restore must NOT be TargetAgnostic -- it always operates on a specific target")
	}
	if restore.ConfirmMessage == "" {
		t.Error("restore must have a non-empty ConfirmMessage (R-003: must state that restore overwrites deployed files)")
	}
	if !strings.Contains(restore.ConfirmMessage, "sobrescribiendo") {
		t.Errorf("restore ConfirmMessage must warn about overwriting deployed files, got %q", restore.ConfirmMessage)
	}
}

// TestVersionFoldedIntoEstadoAlso verifies the "version" subcommand (R-002)
// is merged into Estado's Also list, TargetAgnostic and read-only, rather
// than appearing as a separate top-level menu entry.
func TestVersionFoldedIntoEstadoAlso(t *testing.T) {
	var estado Action
	found := false
	for _, a := range Actions() {
		if a.Command == "status" {
			estado = a
			found = true
			break
		}
		if a.Command == "version" {
			t.Fatal(`"version" must NOT appear as a separate top-level action`)
		}
	}
	if !found {
		t.Fatal(`Actions() must contain the top-level "status" (Estado) action`)
	}

	var version Action
	versionFound := false
	for _, sub := range estado.Also {
		if sub.Command == "version" {
			version = sub
			versionFound = true
		}
	}
	if !versionFound {
		t.Fatal(`Estado's Also must contain a "version" entry`)
	}
	if !version.TargetAgnostic {
		t.Error(`version (Also) must have TargetAgnostic: true`)
	}
	if version.Mutating {
		t.Error(`version (Also) must have Mutating: false (read-only)`)
	}
}

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
	t.Fatalf("Actions() must contain a %q entry", command)
	return -1
}

// TestUpdateActions_Restore_NoBackupIsNoOp verifies R-003's negative path:
// entering "Restaurar respaldo" when the (only) selected target has zero
// backups stays on screenActions -- restore is never offered/run for a
// target with no backup to restore.
func TestUpdateActions_Restore_NoBackupIsNoOp(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // no backups anywhere
	m := newModel()
	m.scr = screenActions
	m.aCursor = findAction(t, m, "restore")

	updated, _ := m.updateActions(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)

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
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeBackupFixture(t, home, "claude", "20260301T093000Z", "v1.5.0\tdigest123\t2026-03-01T09:30:00Z")

	m := newModel()
	// Only "claude" selected, to make the confirm text assertion unambiguous.
	m.selected = map[int]bool{0: true, 1: false, 2: false}
	m.scr = screenActions
	m.aCursor = findAction(t, m, "restore")

	updated, _ := m.updateActions(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)

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
	if !strings.Contains(rendered, "20260301T093000Z") {
		t.Errorf("confirm screen must name the backup timestamp being restored, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "v1.5.0") {
		t.Errorf("confirm screen must name the backup version being restored, got:\n%s", rendered)
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
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeBackupFixture(t, home, "claude", "20260301T093000Z", "v1.5.0\tdigest123\t2026-03-01T09:30:00Z")
	// "opencode" deliberately has no backup.

	m := newModel()
	m.selected = map[int]bool{0: true, 1: true, 2: false} // claude + opencode
	m.scr = screenActions
	m.aCursor = findAction(t, m, "restore")

	updated, _ := m.updateActions(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)

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
	// runBackend's worst-severity aggregation misreport the whole action as
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
	// done.result.targets is runBackend's own record of exactly which targets
	// it was invoked against (set before any subprocess runs) -- the direct
	// proof that "opencode" (zero backups) was never actually invoked, not
	// just omitted from the confirm text.
	for _, tgt := range done.result.targets {
		if tgt.Name == "opencode" {
			t.Errorf("runBackend was invoked against opencode (zero backups); targets = %+v", done.result.targets)
		}
	}
	if len(done.result.targets) != 1 || done.result.targets[0].Name != "claude" {
		t.Errorf("runBackend invoked against targets = %+v, want exactly [claude]", done.result.targets)
	}
}
