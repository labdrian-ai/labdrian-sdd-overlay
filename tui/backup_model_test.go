package main

// How the model uses the RestorePlanner to decide whether restore is offered
// and what its confirm screen says. The planner is a fake here; what it vouches
// for, and how it words it, is tested over the BackupQuery port in package app,
// and what the real backend answers in the adapter's contract tests.

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// lookupResult runs a command and picks out the lookup's message.
func lookupResult(t *testing.T, cmd tea.Cmd) restoreLookupMsg {
	t.Helper()
	for _, msg := range collectMsgs(cmd) {
		if r, ok := msg.(restoreLookupMsg); ok {
			return r
		}
	}
	t.Fatal("the command produced no restoreLookupMsg")
	return restoreLookupMsg{}
}

// pressRestore presses enter on restore; the answer is in the returned command.
func pressRestore(t *testing.T, m model) (model, tea.Cmd) {
	t.Helper()
	m.scr, m.aCursor = screenActions, findAction(t, m, "restore")
	updated, cmd := m.updateActions(tea.KeyMsg{Type: tea.KeyEnter})
	return updated.(model), cmd
}

// deliver hands msg to Update, as the program does when a command finishes.
func deliver(m model, msg tea.Msg) model {
	updated, _ := m.Update(msg)
	return updated.(model)
}

// enterRestore presses enter on restore and delivers the lookup's answer.
func enterRestore(t *testing.T, m model) model {
	t.Helper()
	m, cmd := pressRestore(t, m)
	return deliver(m, lookupResult(t, cmd))
}

// TestRestore_EnterNeverAsksThePlannerInsideUpdate: the real planner spawns the
// backend, so it is asked in the returned command, never in Update, where it
// would freeze the UI.
func TestRestore_EnterNeverAsksThePlannerInsideUpdate(t *testing.T) {
	restore := &fakeRestore{gate: make(chan struct{}), backed: []string{"claude"}}
	m := newLoadedModel(t, &fakeCatalog{targets: threeCopyTargets()}, restore)

	// A question inside Update would wait on the gate until this timer opens it.
	timer := time.AfterFunc(2*time.Second, func() { close(restore.gate) })
	m, cmd := pressRestore(t, m)
	if !timer.Stop() {
		t.Fatal("enter on restore blocked on the RestorePlanner inside Update")
	}
	if len(restore.asked) != 0 || m.scr != screenLookup || !strings.Contains(stripANSI(m.View()), "Consultando respaldos") {
		t.Errorf("after enter: asked %q, screen %v; want no question yet and the lookup screen", restore.asked, m.scr)
	}

	close(restore.gate)
	lookupResult(t, cmd) // only the returned command touches the planner
	if len(restore.asked) == 0 {
		t.Error("the returned command never asked the planner")
	}
}

// TestRestore_AStaleLookupResultIsIgnored: an answer arriving after the operator
// backed out, or after a newer request began, must not open the confirm screen.
func TestRestore_AStaleLookupResultIsIgnored(t *testing.T) {
	m := newLoadedModel(t, &fakeCatalog{targets: threeCopyTargets()}, &fakeRestore{backed: []string{"claude"}})

	m, cmd := pressRestore(t, m)
	first := lookupResult(t, cmd)
	m = deliver(pressKey(t, m, tea.KeyMsg{Type: tea.KeyEsc}), first) // answer after backing out
	if m.scr != screenActions || m.pendingAction.Command == "restore" {
		t.Fatalf("an answer after backing out reached screen %v (pending %q)", m.scr, m.pendingAction.Command)
	}
	m, cmd = pressRestore(t, m) // a newer request is in flight
	second := lookupResult(t, cmd)
	if m = deliver(m, first); m.scr != screenLookup {
		t.Fatalf("an earlier request's answer completed the current lookup: screen %v", m.scr)
	}
	if m = deliver(m, second); m.scr != screenConfirm {
		t.Errorf("the current request's answer should open the confirm screen, got %v", m.scr)
	}
}

// TestRestore_QuitWorksWhileTheLookupIsInFlight: its command is never run here.
func TestRestore_QuitWorksWhileTheLookupIsInFlight(t *testing.T) {
	m, _ := pressRestore(t, newLoadedModel(t, &fakeCatalog{targets: threeCopyTargets()}, &fakeRestore{}))

	for _, key := range []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune("q")}, {Type: tea.KeyCtrlC}} {
		if next, cmd := m.Update(key); cmd == nil || !next.(model).quitting || cmd() != tea.Quit() {
			t.Errorf("%v while the lookup runs did not quit", key)
		}
	}
}

// TestRestore_PutsTheSelectedCopyTargetsToThePlannerAndShowsWhatItSays: the
// planner is asked once, about the selected targets restore can run against and
// in order; the confirm screen carries its text, and its targets are the ones
// the run goes against.
func TestRestore_PutsTheSelectedCopyTargetsToThePlannerAndShowsWhatItSays(t *testing.T) {
	restore := &fakeRestore{backed: []string{"claude", "opencode"}}
	m := enterRestore(t, newLoadedModel(t, &fakeCatalog{targets: threeCopyTargets()}, restore))

	if len(restore.asked) != 1 || strings.Join(restore.asked[0], ",") != "claude,opencode,codex" {
		t.Errorf("the planner was asked %q, want one question about each selected target, in order", restore.asked)
	}
	if m.scr != screenConfirm {
		t.Fatalf("restore with a backup available should reach the confirm screen, got %v", m.scr)
	}
	if got := targetNames(m.pendingTargets); got != "claude, opencode" {
		t.Errorf("restore will run against %q, want the targets the planner vouched for", got)
	}
	if rendered := stripANSI(m.View()); !strings.Contains(rendered, "BACKUPS FOR: claude, opencode") {
		t.Errorf("the confirm screen should show the planner's text, got:\n%s", rendered)
	}
}

// TestRestore_WhenThePlannerOffersNothingTheMenuStays: fail closed. A planner
// that vouches for no backup leaves the operator on the menu with nothing
// pending.
func TestRestore_WhenThePlannerOffersNothingTheMenuStays(t *testing.T) {
	m := enterRestore(t, newLoadedModel(t, &fakeCatalog{targets: threeCopyTargets()}, &fakeRestore{}))

	if m.scr != screenActions || m.pendingAction.Command == "restore" {
		t.Errorf("restore with nothing vouched for reached screen %v with pending %q; it must stay a no-op", m.scr, m.pendingAction.Command)
	}
}

// TestRestore_WithoutAPlannerOffersNothing: a model wired without one (a
// composition mistake) fails closed instead of panicking.
func TestRestore_WithoutAPlannerOffersNothing(t *testing.T) {
	catalog := &fakeCatalog{targets: threeCopyTargets()}
	d := testDeps(catalog)
	d.restore = nil
	updated, _ := newModel(d).Update(loadTargetsCmd(catalog)())

	m := enterRestore(t, updated.(model))
	if m.scr != screenActions {
		t.Errorf("restore without a planner reached screen %v, want it to stay a no-op", m.scr)
	}
}
