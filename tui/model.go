package main

import (
	tea "github.com/charmbracelet/bubbletea"
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

	result   commandResult
	scroll   int // line offset into the output pane

	width  int
	height int

	quitting bool
}

// newModel builds the initial state with every target selected.
func newModel() model {
	targets := AllTargets()
	selected := make(map[int]bool, len(targets))
	for i := range targets {
		selected[i] = true
	}

	root, err := RepoRoot()

	return model{
		repoRoot: root,
		rootErr:  err,
		scr:      screenTargets,
		targets:  targets,
		selected: selected,
		actions:  Actions(),
	}
}

func (m model) Init() tea.Cmd { return nil }

// runDoneMsg is delivered when a backend invocation completes.
type runDoneMsg struct{ result commandResult }

// runActionCmd executes the backend off the UI goroutine.
func (m model) runActionCmd(action Action) tea.Cmd {
	root := m.repoRoot
	selected := m.selectedTargets()
	return func() tea.Msg {
		return runDoneMsg{result: runBackend(root, action, selected)}
	}
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
		return m, nil

	case tea.KeyMsg:
		// Global quit.
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
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
		return m, m.runActionCmd(action)
	}
	return m, nil
}

func (m model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		m.scr = screenRunning
		return m, m.runActionCmd(m.pendingAction)
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
		m.scroll++
	}
	return m, nil
}
