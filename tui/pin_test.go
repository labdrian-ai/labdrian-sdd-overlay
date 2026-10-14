package main

// The program as the operator sees it, pinned from the program as it was before it was cut into layers (T3
// slice 1). Each test drives the whole program (the catalog, the lookups, the actions, the probe) against the
// stub backend and compares what it shows and what the backend received with a golden file, so a refactor that
// changes either fails here, whichever package the change is in. Run with -update only to record a change that
// was decided, and read the diff of the goldens: it is the change.

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/muesli/termenv"
)

// ---------------------------------------------------------------------------
// the harness
// ---------------------------------------------------------------------------

// session is a person at the program: it feeds messages to the model and runs, in the order the model returns
// them, the commands the model asks for.
type session struct {
	t    *testing.T
	stub *stubBackend
	m    tea.Model
	held []tea.Cmd
	log  strings.Builder
}

const (
	pinnedWidth  = 100
	pinnedHeight = 40
)

// newSession builds the program over a fresh stub backend and gives it a terminal of the pinned size. The
// program has not started: the catalog and the probe are asked for when the session starts.
func newSession(t *testing.T) *session {
	t.Helper()
	stub := newStubBackend(t)
	s := &session{t: t, stub: stub, m: stackModel(stub.root, nil)}
	s.resize(pinnedWidth, pinnedHeight)
	return s
}

// newSessionOver builds the program over a backend root that is not the stub's (none, or one that is not there).
func newSessionOver(t *testing.T, root string, rootErr error) *session {
	t.Helper()
	s := &session{t: t, stub: newStubBackend(t), m: stackModel(root, rootErr)}
	s.resize(pinnedWidth, pinnedHeight)
	return s
}

func (s *session) resize(width, height int) {
	s.apply(tea.WindowSizeMsg{Width: width, Height: height})
	s.settle()
}

// apply gives msg to the model and holds the command it returns.
func (s *session) apply(msg tea.Msg) {
	var cmd tea.Cmd
	s.m, cmd = s.m.Update(msg)
	if cmd != nil {
		s.held = append(s.held, cmd)
	}
}

// settle runs the held commands, and what they answer is given to the model, until nothing is left to run. A
// spinner tick is not given back: the program would get it a moment later, and the screens pin the first frame.
func (s *session) settle() {
	for len(s.held) > 0 {
		cmd := s.held[0]
		s.held = s.held[1:]
		s.run(cmd)
	}
}

func (s *session) run(cmd tea.Cmd) {
	msg := cmd()
	switch msg := msg.(type) {
	case nil:
		return
	case tea.BatchMsg:
		for _, sub := range msg {
			if sub != nil {
				s.run(sub)
			}
		}
		return
	case spinner.TickMsg:
		return
	default:
		s.apply(msg)
	}
}

// start is the program starting: the commands Init asks for run.
func (s *session) start() *session {
	s.t.Helper()
	if cmd := s.m.Init(); cmd != nil {
		s.held = append(s.held, cmd)
	}
	s.settle()
	return s
}

// press is the operator pressing keys, one after the other, each answered before the next.
func (s *session) press(keys ...string) *session {
	s.t.Helper()
	s.pressHeld(keys...)
	s.settle()
	return s
}

// pressHeld is press without answering: the commands the keys asked for stay unrun, which is how a screen that
// is only there while the backend is working (the lookup, the run) is looked at.
func (s *session) pressHeld(keys ...string) *session {
	s.t.Helper()
	for _, name := range keys {
		s.apply(keyNamed(name))
	}
	return s
}

func keyNamed(name string) tea.KeyMsg {
	switch name {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
}

// times repeats a key.
func times(n int, key string) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = key
	}
	return keys
}

// the rows of the action menu, in the order it lists them
const (
	rowEstado = iota
	rowSyncCheck
	rowCapture
	rowApply
	rowRestore
	rowSelfUpdate
	rowInstallHooks
	rowUninstallHooks
	rowSkills
	rowMemoryStatus
	rowMemoryInstall
	rowMemoryUninstall
)

// toActions is the operator accepting the targets as they are: the actions screen.
func (s *session) toActions() *session {
	s.t.Helper()
	return s.press("enter")
}

// onRow moves the cursor of the actions screen down to row.
func (s *session) onRow(row int) *session {
	s.t.Helper()
	return s.press(times(row, "down")...)
}

// view is what the operator sees, below the hero logo (which has a test of its own, and a golden below).
func (s *session) view() string { return dropLogo(s.m.View()) }

// dropLogo cuts the header's hero logo from a view: everything up to the wordmark's line.
func dropLogo(view string) string {
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		if strings.Contains(line, "overlay sobre gentle-ai") {
			return strings.Join(lines[i+1:], "\n")
		}
	}
	return view
}

// snap adds what the operator sees now to the session's record.
func (s *session) snap(label string) *session {
	fmt.Fprintf(&s.log, "### %s\n%s\n\n", label, s.view())
	return s
}

// record is the session's whole record: what the operator saw at each snap, and what the backend received.
func (s *session) record() []byte {
	fmt.Fprintf(&s.log, "### the backend received\n%s\n", strings.Join(s.stub.invoked(), "\n"))
	return []byte(s.log.String())
}

// withColors fixes the color profile the styles render in for the test, whatever terminal runs it.
func withColors(t *testing.T, profile termenv.Profile) {
	t.Helper()
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(profile)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
}

// ---------------------------------------------------------------------------
// what the backend says
// ---------------------------------------------------------------------------

// syncCheckReport is a sync-check across the four targets that shows every state the dashboard has.
const syncCheckReport = `
=== sync-check: claude (/stub/.claude) ===
  IN_SYNC: skills/foo
VERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=0 REPO_BEHIND_RELEASE=0 RECORDED_VERSION=v1.2.0 DIGEST_MATCH=yes
ACTION:claude: in sync with gentle-ai (healthy)

=== sync-check: opencode (/stub/.config/opencode) ===
  OVERLAY_NOT_DEPLOYED: skills/bar (live missing)
  OVERLAY_NOT_DEPLOYED: agents/GADU.md (live missing)
  IN_SYNC: agents/SDD.md (same)
VERDICT:opencode:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=2 REPO_BEHIND_ORIGIN=0 REPO_BEHIND_RELEASE=0 RECORDED_VERSION=v1.1.0 DIGEST_MATCH=no
ACTION:opencode: run 'labdrian apply --target opencode'

=== sync-check: codex (/stub/.codex) ===
  UPSTREAM_CHANGED: skills/baz
  UPSTREAM_CHANGED: agents/Orchestrator.md (differs)
VERDICT:codex:UPSTREAM_CHANGED=3 OVERLAY_NOT_DEPLOYED=1 REPO_BEHIND_ORIGIN=5 REPO_BEHIND_RELEASE=2 RECORDED_VERSION=v1.0.0 DIGEST_MATCH=yes
ACTION:codex: gentle-ai sync detected: run 'labdrian capture --target codex' then 'labdrian apply'

=== sync-check: pi (/stub/pi) ===
VERDICT:pi:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=NA REPO_BEHIND_RELEASE=2 RECORDED_VERSION=NA DIGEST_MATCH=NA
ACTION:pi: self-update to catch up with the latest release
`

// probeReport is the first verdict of a sync-check, which is all the launch probe reads.
func probeReport(origin, release string) string {
	return "\n=== sync-check: claude (/stub/.claude) ===\n" +
		"VERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=" + origin +
		" REPO_BEHIND_RELEASE=" + release + "\n" +
		"ACTION:claude: in sync with gentle-ai (healthy)\n"
}

// ---------------------------------------------------------------------------
// the screens
// ---------------------------------------------------------------------------

// pinnedScreen builds the program in the state a screen is looked at in.
type pinnedScreen struct {
	name  string
	build func(t *testing.T) *session
}

var pinnedScreens = []pinnedScreen{
	{"targets_loading", func(t *testing.T) *session { return newSession(t) }},
	{"targets_loaded", func(t *testing.T) *session { return newSession(t).start() }},
	{"targets_one_deselected", func(t *testing.T) *session {
		return newSession(t).start().press("down", "space")
	}},
	{"targets_none_selected", func(t *testing.T) *session {
		return newSession(t).start().press("a")
	}},
	{"targets_catalog_unreadable", func(t *testing.T) *session {
		s := newSession(t)
		s.stub.exit("targets", 3)
		s.stub.complain("targets", "labdrian-overlay targets: catalog unreadable\n")
		return s.start()
	}},
	{"targets_no_backend_root", func(t *testing.T) *session {
		return newSessionOver(t, "", errors.New("could not locate overlay repo root (bin/labdrian-overlay not found); set OVERLAY_DIR")).start()
	}},
	{"targets_narrow_terminal", func(t *testing.T) *session {
		s := newSession(t).start()
		s.resize(50, 20)
		return s
	}},
	{"actions_first_row", func(t *testing.T) *session { return newSession(t).start().toActions() }},
	{"actions_on_self_update", func(t *testing.T) *session {
		return newSession(t).start().toActions().onRow(rowSelfUpdate)
	}},
	{"actions_last_row", func(t *testing.T) *session {
		return newSession(t).start().toActions().onRow(rowMemoryUninstall)
	}},
	{"actions_narrow_terminal", func(t *testing.T) *session {
		s := newSession(t).start().toActions()
		s.resize(50, 20)
		return s
	}},
	{"confirm_apply", func(t *testing.T) *session {
		return newSession(t).start().toActions().onRow(rowApply).press("enter")
	}},
	{"confirm_capture_names_copy_targets_only", func(t *testing.T) *session {
		return newSession(t).start().toActions().onRow(rowCapture).press("enter")
	}},
	{"confirm_self_update", func(t *testing.T) *session {
		return newSession(t).start().toActions().onRow(rowSelfUpdate).press("enter")
	}},
	{"confirm_install_hooks", func(t *testing.T) *session {
		return newSession(t).start().toActions().onRow(rowInstallHooks).press("enter")
	}},
	{"confirm_memory_install", func(t *testing.T) *session {
		return newSession(t).start().toActions().onRow(rowMemoryInstall).press("enter")
	}},
	{"lookup_in_flight", func(t *testing.T) *session {
		s := newSession(t).start().toActions().onRow(rowRestore)
		s.stub.backups("claude", "20260301T093000Z (v1.5.0)")
		return s.pressHeld("enter")
	}},
	{"confirm_restore_names_the_latest_backup", func(t *testing.T) *session {
		s := newSession(t).start().toActions().onRow(rowRestore)
		s.stub.backups("claude", "20260228T101500Z (v1.4.0)", "20260301T093000Z (v1.5.0)")
		s.stub.backups("opencode", "20260302T080000Z (unknown)")
		s.stub.noBackups("codex")
		return s.press("enter")
	}},
	{"restore_with_no_backup_stays_on_the_menu", func(t *testing.T) *session {
		s := newSession(t).start().toActions().onRow(rowRestore)
		for _, name := range []string{"claude", "opencode", "codex"} {
			s.stub.noBackups(name)
		}
		return s.press("enter")
	}},
	{"running_in_flight", func(t *testing.T) *session {
		return newSession(t).start().toActions().onRow(rowApply).press("enter").pressHeld("y")
	}},
	{"result_success_status", func(t *testing.T) *session {
		s := newSession(t)
		s.stub.say("status", "claude: deployed\nopencode: deployed\ncodex: deployed\npi: deployed\n")
		s.stub.say("status-hooks", "hooks: installed\n")
		s.stub.say("version", "claude v1.2.0\n")
		return s.start().toActions().press("enter")
	}},
	{"result_sync_check_dashboard", func(t *testing.T) *session {
		s := newSession(t)
		s.stub.say("sync-check", syncCheckReport)
		return s.start().toActions().onRow(rowSyncCheck).press("enter")
	}},
	{"result_sync_check_without_verdicts", func(t *testing.T) *session {
		s := newSession(t)
		s.stub.say("sync-check", "SYNC_CHECK:claude: target dir not found -- skipping\n")
		return s.start().toActions().onRow(rowSyncCheck).press("enter")
	}},
	{"result_hard_failure", func(t *testing.T) *session {
		s := newSession(t)
		s.stub.say("apply", "applying...\nboom\n")
		s.stub.exit("apply", 1)
		return s.start().toActions().onRow(rowApply).press("enter", "y")
	}},
	{"result_degraded_exit_two", func(t *testing.T) *session {
		s := newSession(t)
		s.stub.say("status", "engine: DEGRADED\n")
		s.stub.exit("status", 2)
		return s.start().toActions().press("enter")
	}},
	{"result_long_output_scrolled", func(t *testing.T) *session {
		s := newSession(t)
		var lines []string
		for i := 1; i <= 60; i++ {
			lines = append(lines, fmt.Sprintf("line %02d of the status report", i))
		}
		s.stub.say("status", strings.Join(lines, "\n")+"\n")
		return s.start().toActions().press("enter").press("down", "down", "down", "down", "down")
	}},
	{"result_back_to_the_menu", func(t *testing.T) *session {
		return newSession(t).start().toActions().press("enter").press("esc")
	}},
	{"banner_behind_origin_before_any_release", func(t *testing.T) *session {
		s := newSession(t)
		s.stub.say("sync-check", probeReport("3", "NA"))
		return s.start()
	}},
	{"banner_behind_release", func(t *testing.T) *session {
		s := newSession(t)
		s.stub.say("sync-check", probeReport("3", "2"))
		return s.start()
	}},
	{"banner_dismissed", func(t *testing.T) *session {
		s := newSession(t)
		s.stub.say("sync-check", probeReport("3", "2"))
		return s.start().press("x")
	}},
	{"repo_up_to_date_before_any_release", func(t *testing.T) *session {
		s := newSession(t)
		s.stub.say("sync-check", probeReport("0", "NA"))
		return s.start()
	}},
	{"repo_up_to_date_with_untagged_commits", func(t *testing.T) *session {
		s := newSession(t)
		s.stub.say("sync-check", probeReport("4", "0"))
		return s.start()
	}},
	{"repo_state_unknown", func(t *testing.T) *session { return newSession(t).start() }},
}

// TestPinned_Screens pins every screen of the program, as text.
func TestPinned_Screens(t *testing.T) {
	for _, tc := range pinnedScreens {
		t.Run(tc.name, func(t *testing.T) {
			withColors(t, termenv.Ascii)
			golden.RequireEqual(t, []byte(tc.build(t).view()))
		})
	}
}
