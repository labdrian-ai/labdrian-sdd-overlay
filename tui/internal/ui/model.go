// Package ui is the TUI's driving adapter: the bubbletea model that turns keys
// into the questions and the actions of the use cases, and the screens that
// show what they answered. It holds no list of targets, runs no process and
// reads no file: everything it knows comes from the domain, and everything it
// does goes through the collaborators main gave it (Deps). Only the
// composition root imports it.
package ui

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/domain"
)

// screen identifies the active view in the TUI.
type screen int

const (
	screenTargets screen = iota // multi-select target checklist
	screenActions               // action menu
	screenConfirm               // confirmation for mutating actions
	screenRunning               // running the backend
	screenResult                // output pane + sync dashboard
	screenLookup                // asking the backend for the latest backups (restore confirm)
)

// model is the root bubbletea state.
type model struct {
	rootErr error

	// catalog is the port the targets come from. The model holds no list of
	// its own: targets stays empty until the catalog answers.
	catalog domain.TargetCatalog
	// runner carries out an action against the targets it is given, restore
	// plans what restore may be offered for, and probe reads how far the
	// repository is behind. The model runs none of them itself: it asks, in a
	// command, off the UI goroutine.
	runner  ActionRunner
	restore RestorePlanner
	probe   RepoProber

	scr screen

	// targets is the backend's catalog exactly as the operator was shown it,
	// in the backend's order. It is what `--target all` may be taken to mean.
	targets []domain.Target
	// targetsLoaded is false until the catalog's answer arrives; targetsErr is
	// that answer when it is a failure. With a failed catalog targets stays
	// empty, so there is nothing to select and no target action can start.
	targetsLoaded bool
	targetsErr    error
	selected      map[int]bool // index into targets -> selected
	tCursor       int

	actions []domain.Action
	aCursor int

	pendingAction domain.Action // action awaiting confirmation / running
	// pendingTargets is the exact target set runActionCmd runs pendingAction
	// against. It is computed once, when the action is chosen (updateActions,
	// the answer of the restore lookup, or the "u" banner shortcut), and
	// reused verbatim by updateConfirm's "y" handler; it is never recomputed
	// from m.selectedTargets() at run time. For an action that requires a
	// backup it is the subset the RestorePlanner vouched for, the targets its
	// confirm text names: running against the whole selection would invoke
	// targets that have nothing to restore.
	pendingTargets []domain.Target
	// restoreReq numbers the restore backup lookups; only the latest answer counts.
	restoreReq int

	result domain.CommandResult
	scroll int // line offset into the output pane

	width  int
	height int

	spinner spinner.Model

	quitting bool

	// behindOrigin is the launch-time probe's REPO_BEHIND_ORIGIN reading
	// (D4/D5). It is initialized to RepoBehindOriginNA in newModel, NOT left
	// at Go's zero value — a bare `0` would collapse into "confirmed 0
	// commits behind" before the probe ever resolves, reproducing the exact
	// R-006 bug class the reader of a report (domain.BehindOf) already guards
	// against.
	behindOrigin int
	// behindRelease is the launch-time probe's REPO_BEHIND_RELEASE reading
	// (D2). Initialized to RepoBehindOriginNA in newModel, same rationale as
	// behindOrigin above — while no release tag exists yet (D1 pre-first-tag
	// bootstrap), it stays NA and bannerVisible/repoLine fall back to the
	// legacy origin-only behavior unchanged.
	behindRelease int
	// bannerDismissed tracks whether the user dismissed the behind-origin
	// banner this session (R-002); it never resets automatically.
	bannerDismissed bool
}

// ActionRunner is what the model needs to have an action carried out:
// action's invocations against selected, with shown the catalog the operator
// was shown. The result says everything that happened, including a refusal.
type ActionRunner interface {
	Run(action domain.Action, selected, shown []domain.Target) domain.CommandResult
}

// RestorePlanner is what the model needs to decide whether restore is offered:
// the confirm text for the candidates that have a backup, and those targets.
// ok is false when none does. It may spawn the backend, so the model calls it
// in a command.
type RestorePlanner interface {
	Confirmation(action domain.Action, candidates []domain.Target) (message string, targets []domain.Target, ok bool)
}

// RepoProber is what the model needs to show how far the repository is
// behind: commits behind origin/main and behind the latest release tag, each
// domain.RepoBehindOriginNA when unknown.
type RepoProber interface {
	Behind() domain.RepoBehind
}

// Deps is what the composition root (main) hands the model: the error of
// locating the repository, the action menu, the catalog and the use cases the
// model talks to. The model builds none of them.
type Deps struct {
	RootErr error
	Actions []domain.Action
	Catalog domain.TargetCatalog
	Runner  ActionRunner
	Restore RestorePlanner
	Probe   RepoProber
}

// New returns the program's model over d. It is the only way in from outside
// the package: the model is not exported, and main hands bubbletea this.
func New(d Deps) tea.Model { return newModel(d) }

// newModel builds the initial state. It has no targets yet: the catalog is
// asked from Init, off the UI goroutine, and its answer selects them all.
func newModel(d Deps) model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorAccent)

	return model{
		rootErr:       d.RootErr,
		catalog:       d.Catalog,
		runner:        d.Runner,
		restore:       d.Restore,
		probe:         d.Probe,
		scr:           screenTargets,
		selected:      map[int]bool{},
		actions:       d.Actions,
		spinner:       sp,
		behindOrigin:  domain.RepoBehindOriginNA,
		behindRelease: domain.RepoBehindOriginNA,
	}
}

// Init returns the launch-time commands: the target catalog (D5) and the
// cached-only origin probe (R-001). Both run async off the UI goroutine and
// never block the first render -- the results arrive later as a
// targetsLoadedMsg and a probeDoneMsg.
func (m model) Init() tea.Cmd {
	return tea.Batch(loadTargetsCmd(m.catalog), probeCmd(m.probe))
}

// probeCmd asks the prober how far the repository is behind and delivers the
// answer as a probeDoneMsg. bubbletea runs the returned func() tea.Msg off the
// UI goroutine, so this never blocks Init()'s first render. One report feeds
// both fields (D2): no second run is spent just to pick up the release count.
// Without a prober there is nothing to ask.
func probeCmd(p RepoProber) tea.Cmd {
	if p == nil {
		return nil
	}
	return func() tea.Msg {
		behind := p.Behind()
		return probeDoneMsg{behind: behind.Origin, behindRelease: behind.Release}
	}
}

// targetsLoadedMsg is delivered with the catalog's answer. err is set when the
// backend's catalog could not be read, in which case targets is empty.
type targetsLoadedMsg struct {
	targets []domain.Target
	err     error
}

// loadTargetsCmd asks the catalog port for the targets.
func loadTargetsCmd(catalog domain.TargetCatalog) tea.Cmd {
	return func() tea.Msg {
		targets, err := catalog.Targets()
		return targetsLoadedMsg{targets: targets, err: err}
	}
}

// catalogReady reports whether the backend's catalog was read successfully.
// Until it is, the TUI has no targets and offers no target action.
func (m model) catalogReady() bool { return m.targetsLoaded && m.targetsErr == nil }

// runDoneMsg is delivered when a backend invocation completes.
type runDoneMsg struct{ result domain.CommandResult }

// restoreLookupMsg is the outcome of request req's backup lookup: targets is
// the backup-bearing subset action.ConfirmMessage names (empty: none has one).
type restoreLookupMsg struct {
	req     int
	action  domain.Action
	targets []domain.Target
}

// probeDoneMsg is delivered when the launch-time cached-only origin probe
// (probeCmd, D4) completes. Its fields are consumed by the
// Update() branch wired in Phase 3 (behind) and D2 (behindRelease).
type probeDoneMsg struct {
	behind        int
	behindRelease int
}

// runActionCmd executes the backend off the UI goroutine against the exact
// targets given -- never m.selectedTargets() internally, since a caller may
// need to invoke a filtered subset of the current selection (restore/D4).
// The catalog the operator was shown travels with the run, so `--target all`
// can be checked against it.
func (m model) runActionCmd(action domain.Action, targets []domain.Target) tea.Cmd {
	runner := m.runner
	shown := append([]domain.Target(nil), m.targets...)
	return func() tea.Msg {
		return runDoneMsg{result: runner.Run(action, targets, shown)}
	}
}

// remedyAction returns the first action of m.actions that declares
// RefreshProbe: the one whose success moves what the banner reports, so the
// banner shortcut offers it. It is the exact SAME entry Actions() wires up
// (including self-update's chained-apply Also) rather than a hand-built
// stand-in that could drift from it.
func (m model) remedyAction() (domain.Action, bool) {
	for _, a := range m.actions {
		if a.RefreshProbe {
			return a, true
		}
	}
	return domain.Action{}, false
}

// selectedTargets returns the chosen targets in canonical order.
func (m model) selectedTargets() []domain.Target {
	out := []domain.Target{}
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
// bannerVisible reports whether the actionable "you should self-update"
// banner (R-002) is currently shown: rootErr keeps precedence (mirrors
// repoLine()'s rendering rule so both stay in lockstep), and the user must
// not have dismissed it yet.
//
// D2: once a release tag exists (behindRelease resolved to a concrete
// value, not NA), REPO_BEHIND_RELEASE is the primary "up to date" signal —
// raw REPO_BEHIND_ORIGIN drift alone no longer triggers this actionable
// banner (it demotes to an informational line in repoLine()). While no
// release tag exists anywhere yet (behindRelease == NA, D1 pre-first-tag
// bootstrap), this falls back to the legacy origin-only behavior,
// byte-identical to before D2.
func (m model) bannerVisible() bool {
	if m.rootErr != nil || m.bannerDismissed {
		return false
	}
	if m.behindRelease != domain.RepoBehindOriginNA {
		return m.behindRelease > 0
	}
	return m.behindOrigin > 0
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
	if m.result.Output == "" {
		return 0
	}
	lines := splitOutputLines(m.result.Output)
	viewport := m.height - 10
	if len(m.result.Verdicts) > 0 {
		viewport -= len(m.result.Verdicts)*3 + 3
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
		// D5 tail: a successful run of an action that declares RefreshProbe
		// (self-update just fetched and fast-forwarded main) refreshed the
		// cached origin/main ref the launch-time probe reads from. Re-fire it
		// so the banner can self-correct without requiring a restart.
		if msg.result.Action.RefreshProbe && msg.result.Err == nil {
			return m, probeCmd(m.probe)
		}
		return m, nil

	case probeDoneMsg:
		m.behindOrigin = msg.behind
		m.behindRelease = msg.behindRelease
		return m, nil

	case targetsLoadedMsg:
		m.targetsLoaded = true
		m.targetsErr = msg.err
		m.targets = msg.targets
		m.selected = make(map[int]bool, len(m.targets))
		// Every target starts selected, as it always has: the operator sees
		// the whole catalog ticked and unticks what they do not want.
		for i := range m.targets {
			m.selected[i] = true
		}
		return m, nil

	case restoreLookupMsg:
		// Stale if the operator backed out or began a newer lookup.
		if m.scr != screenLookup || msg.req != m.restoreReq {
			return m, nil
		}
		m.scr = screenActions
		if len(msg.targets) > 0 {
			m.pendingAction, m.pendingTargets, m.scr = msg.action, msg.targets, screenConfirm
		}
		return m, nil

	case spinner.TickMsg:
		if m.scr != screenRunning && m.scr != screenLookup {
			// Drop ticks once we've left a spinner screen — stops the self-perpetuating loop.
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
		// (screenRunning) so it cannot hijack an in-flight invocation, and
		// while the target catalog is unknown: self-update chains an apply,
		// which is a target action, and with no catalog there is no target
		// it may run against.
		if msg.String() == "u" && m.bannerVisible() && m.scr != screenRunning && m.catalogReady() {
			if a, ok := m.remedyAction(); ok {
				m.pendingAction = a
				m.pendingTargets = m.selectedTargets()
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
		case screenLookup:
			return m.updateLookup(msg)
		case screenRunning:
			return m, nil
		}
	}
	return m, nil
}

// updateLookup handles keys while the backup lookup runs: quit, or go back.
func (m model) updateLookup(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.scr = screenActions
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
		// Only the selected targets this action can actually run against.
		targets := action.ApplicableTo(m.selectedTargets())
		if action.UsesTargets() && len(targets) == 0 {
			// Nothing to run it on -- no catalog was read, or every selected
			// target is one the backend refuses for this action (capture on
			// pi alone). Stay put: no confirm screen, no process.
			return m, nil
		}
		if action.RequiresBackup {
			// R-003: an action that requires a backup (restore) is never offered for a target with zero backups.
			// The lookup spawns the backend, so it runs in a command and its
			// restoreLookupMsg opens the confirm screen (or does nothing when
			// no target has a backup). Each press is a new request.
			m.restoreReq++
			m.scr = screenLookup
			return m, tea.Batch(m.spinner.Tick, m.restoreLookupCmd(m.restoreReq, action, targets))
		}
		m.pendingAction = action
		m.pendingTargets = targets
		if action.Mutating {
			m.scr = screenConfirm
			return m, nil
		}
		m.scr = screenRunning
		return m, tea.Batch(m.spinner.Tick, m.runActionCmd(action, targets))
	}
	return m, nil
}

// restoreLookupCmd asks the restore planner, off the UI goroutine, which of
// the candidates have a backup and what the confirm screen says about them:
// the planner may spawn the backend, and quit/back stay available however long
// it takes. Without a planner nothing can be vouched for, so nothing is
// offered.
//
// The targets in the answer are the SAME backup-bearing subset the confirm
// text names. They are the pendingTargets of the confirm screen and what the
// real invocation runs against, never recomputed from the selection (see
// pendingTargets).
func (m model) restoreLookupCmd(req int, action domain.Action, candidates []domain.Target) tea.Cmd {
	return func() tea.Msg {
		if m.restore == nil {
			return restoreLookupMsg{req: req, action: action}
		}
		info, targets, _ := m.restore.Confirmation(action, candidates)
		action.ConfirmMessage = info
		return restoreLookupMsg{req: req, action: action, targets: targets}
	}
}

func (m model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		m.scr = screenRunning
		return m, tea.Batch(m.spinner.Tick, m.runActionCmd(m.pendingAction, m.pendingTargets))
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
