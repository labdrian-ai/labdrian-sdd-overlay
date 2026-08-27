package main

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// screen identifies the active view in the TUI.
type screen int

const (
	screenTargets screen = iota // multi-select target checklist
	screenActions               // action menu
	screenConfirm               // confirmation for mutating actions
	screenRunning               // running the backend
	screenResult                // output pane + sync dashboard
)

// model is the root bubbletea state.
type model struct {
	repoRoot string
	rootErr  error

	scr screen

	targets  []Target
	selected map[int]bool // index into targets -> selected
	tCursor  int

	actions []Action
	aCursor int

	pendingAction Action // action awaiting confirmation / running

	result commandResult
	scroll int // line offset into the output pane

	width  int
	height int

	spinner spinner.Model

	quitting bool

	// behindOrigin is the launch-time probe's REPO_BEHIND_ORIGIN reading
	// (D4/D5). It is initialized to RepoBehindOriginNA in newModel, NOT left
	// at Go's zero value — a bare `0` would collapse into "confirmed 0
	// commits behind" before the probe ever resolves, reproducing the exact
	// R-006 bug class ParseSyncCheck already guards against.
	behindOrigin int
	// bannerDismissed tracks whether the user dismissed the behind-origin
	// banner this session (R-002); it never resets automatically.
	bannerDismissed bool
}

// newModel builds the initial state with every target selected.
func newModel() model {
	targets := AllTargets()
	selected := make(map[int]bool, len(targets))
	for i := range targets {
		selected[i] = true
	}

	root, err := RepoRoot()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorAccent)

	return model{
		repoRoot:     root,
		rootErr:      err,
		scr:          screenTargets,
		targets:      targets,
		selected:     selected,
		actions:      Actions(),
		spinner:      sp,
		behindOrigin: RepoBehindOriginNA,
	}
}

// Init returns the launch-time cached-only origin probe (R-001). It runs
// async off the UI goroutine and never blocks the first render — the result
// arrives later as a probeDoneMsg.
func (m model) Init() tea.Cmd { return probeBehindOriginCmd(m.repoRoot) }

// runDoneMsg is delivered when a backend invocation completes.
type runDoneMsg struct{ result commandResult }

// probeDoneMsg is delivered when the launch-time cached-only origin probe
// (probeBehindOriginCmd, D4) completes. Its field is consumed by the
// Update() branch wired in Phase 3.
type probeDoneMsg struct{ behind int }

// runActionCmd executes the backend off the UI goroutine.
func (m model) runActionCmd(action Action) tea.Cmd {
	root := m.repoRoot
	selected := m.selectedTargets()
	return func() tea.Msg {
		return runDoneMsg{result: runBackend(root, action, selected)}
	}
}

// selfUpdateAction returns the registered self-update Action from m.actions,
// so the banner shortcut always confirms/runs the exact SAME entry Actions()
// wires up (including its chained-apply Also) rather than a hand-built
// stand-in that could drift from it.
func (m model) selfUpdateAction() (Action, bool) {
	for _, a := range m.actions {
		if a.Command == "self-update" {
			return a, true
		}
	}
	return Action{}, false
}

// selectedTargets returns the chosen targets in canonical order.
func (m model) selectedTargets() []Target {
	out := []Target{}
	for i, t := range m.targets {
		if m.selected[i] {
			out = append(out, t)
		}
	}
	return out
}

func (m model) anySelected() bool {
	for i := range m.targets {
		if m.selected[i] {
			return true
		}
	}
	return false
}

// allSelected returns true iff every target is currently selected.
func (m model) allSelected() bool {
	for i := range m.targets {
		if !m.selected[i] {
			return false
		}
	}
	return true
}

// contentWidth returns m.width with an 80-column fallback when no
// WindowSizeMsg has been received yet (m.width == 0).
// bannerVisible reports whether the behind-origin banner (R-002) is
// currently shown: rootErr keeps precedence (mirrors repoLine()'s
// rendering rule so both stay in lockstep), the probe must have resolved a
// concrete positive count, and the user must not have dismissed it yet.
func (m model) bannerVisible() bool {
	return m.rootErr == nil && m.behindOrigin > 0 && !m.bannerDismissed
}

func (m model) contentWidth() int {
	if m.width <= 0 {
		return 80
	}
	return m.width
}

// maxScroll returns the maximum valid scroll offset for the output pane,
// clamped to max(0, totalLines-viewport).
func (m model) maxScroll() int {
	if m.result.output == "" {
		return 0
	}
	lines := splitOutputLines(m.result.output)
	viewport := m.height - 10
	if len(m.result.verdicts) > 0 {
		viewport -= len(m.result.verdicts)*3 + 3
	}
	if viewport < 5 {
		viewport = 5
	}
	max := len(lines) - viewport
	if max < 0 {
		return 0
	}
	return max
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case runDoneMsg:
		m.result = msg.result
		m.scroll = 0
		m.scr = screenResult
		// D5 tail: a successful self-update just fetched and fast-forwarded
		// main, refreshing the cached origin/main ref the launch-time probe
		// reads from. Re-fire it so the banner can self-correct without
		// requiring a restart.
		if msg.result.action.Command == "self-update" && msg.result.err == nil {
			return m, probeBehindOriginCmd(m.repoRoot)
		}
		return m, nil

	case probeDoneMsg:
		m.behindOrigin = msg.behind
		return m, nil

	case spinner.TickMsg:
		if m.scr != screenRunning {
			// Drop ticks once we've left screenRunning — stops the self-perpetuating loop.
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		// Global quit.
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
		// Global banner dismissal (R-002): works on any screen, independent
		// of navigation; session-scoped, never resets. No-op when the
		// banner isn't visible.
		if msg.String() == "x" && m.bannerVisible() {
			m.bannerDismissed = true
			return m, nil
		}
		// Global banner shortcut (menos-pasos follow-up): jumps straight to
		// the self-update confirm screen from wherever the user is, skipping
		// the menu-navigation step for the single most common recovery. Like
		// "x" above, works on any screen and is a no-op when the banner
		// isn't visible. Excluded while a command is actively running
		// (screenRunning) so it cannot hijack an in-flight invocation.
		if msg.String() == "u" && m.bannerVisible() && m.scr != screenRunning {
			if a, ok := m.selfUpdateAction(); ok {
				m.pendingAction = a
				m.scr = screenConfirm
				return m, nil
			}
		}
		switch m.scr {
		case screenTargets:
			return m.updateTargets(msg)
		case screenActions:
			return m.updateActions(msg)
		case screenConfirm:
			return m.updateConfirm(msg)
		case screenResult:
			return m.updateResult(msg)
		case screenRunning:
			return m, nil
		}
	}
	return m, nil
}

func (m model) updateTargets(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		m.quitting = true
		return m, tea.Quit
	case "up", "k":
		if m.tCursor > 0 {
			m.tCursor--
		}
	case "down", "j":
		if m.tCursor < len(m.targets)-1 {
			m.tCursor++
		}
	case " ":
		m.selected[m.tCursor] = !m.selected[m.tCursor]
	case "a":
		// Toggle all: if all selected → deselect all; otherwise → select all.
		target := !m.allSelected()
		for i := range m.targets {
			m.selected[i] = target
		}
	case "enter":
		if m.anySelected() {
			m.scr = screenActions
		}
	}
	return m, nil
}

func (m model) updateActions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		m.quitting = true
		return m, tea.Quit
	case "esc", "left", "h":
		m.scr = screenTargets
	case "up", "k":
		if m.aCursor > 0 {
			m.aCursor--
		}
	case "down", "j":
		if m.aCursor < len(m.actions)-1 {
			m.aCursor++
		}
	case "enter":
		action := m.actions[m.aCursor]
		m.pendingAction = action
		if action.Mutating {
			m.scr = screenConfirm
			return m, nil
		}
		m.scr = screenRunning
		return m, tea.Batch(m.spinner.Tick, m.runActionCmd(action))
	}
	return m, nil
}

func (m model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		m.scr = screenRunning
		return m, tea.Batch(m.spinner.Tick, m.runActionCmd(m.pendingAction))
	case "n", "N", "esc", "q":
		m.scr = screenActions
	}
	return m, nil
}

func (m model) updateResult(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		m.quitting = true
		return m, tea.Quit
	case "esc", "left", "h", "enter":
		m.scr = screenActions
		m.scroll = 0
	case "up", "k":
		if m.scroll > 0 {
			m.scroll--
		}
	case "down", "j":
		if m.scroll < m.maxScroll() {
			m.scroll++
		}
	}
	return m, nil
}
