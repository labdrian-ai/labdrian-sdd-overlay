package main

// How the model uses the BackupQuery port to decide whether restore is
// offered and what its confirm screen says. The query itself is a fake here;
// what the real backend answers is pinned in overlaycli_backend_test.go.

import (
	"errors"
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

// TestRestore_EnterNeverQueriesInsideUpdate: the real query spawns the backend,
// so it runs in the returned command, never in Update, where it would freeze the UI.
func TestRestore_EnterNeverQueriesInsideUpdate(t *testing.T) {
	backups := &fakeBackups{gate: make(chan struct{}), byTarget: map[string]Backup{
		"claude": {Timestamp: "20260301T093000Z", Version: "v1.5.0"},
	}}
	m := newLoadedModel(t, &fakeCatalog{targets: threeCopyTargets()}, backups)

	// A query inside Update would wait on the gate until this timer opens it.
	timer := time.AfterFunc(2*time.Second, func() { close(backups.gate) })
	m, cmd := pressRestore(t, m)
	if !timer.Stop() {
		t.Fatal("enter on restore blocked on the BackupQuery inside Update")
	}
	if len(backups.asked) != 0 || m.scr != screenLookup || !strings.Contains(stripANSI(m.View()), "Consultando respaldos") {
		t.Errorf("after enter: asked %q, screen %v; want no query yet and the lookup screen", backups.asked, m.scr)
	}

	close(backups.gate)
	lookupResult(t, cmd) // only the returned command touches the port
	if len(backups.asked) == 0 {
		t.Error("the returned command never queried the port")
	}
}

// TestRestore_AStaleLookupResultIsIgnored: an answer arriving after the operator
// backed out, or after a newer request began, must not open the confirm screen.
func TestRestore_AStaleLookupResultIsIgnored(t *testing.T) {
	backups := &fakeBackups{byTarget: map[string]Backup{"claude": {Timestamp: "20260301T093000Z", Version: "v1.5.0"}}}
	m := newLoadedModel(t, &fakeCatalog{targets: threeCopyTargets()}, backups)

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
	m, _ := pressRestore(t, newLoadedModel(t, &fakeCatalog{targets: threeCopyTargets()}, &fakeBackups{}))

	for _, key := range []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune("q")}, {Type: tea.KeyCtrlC}} {
		if next, cmd := m.Update(key); cmd == nil || !next.(model).quitting || cmd() != tea.Quit() {
			t.Errorf("%v while the lookup runs did not quit", key)
		}
	}
}

// TestRestore_AsksTheBackupQueryForEachSelectedTarget: the latest backup comes
// from the port, once per target restore can run against, not from the state
// directory's layout.
func TestRestore_AsksTheBackupQueryForEachSelectedTarget(t *testing.T) {
	backups := &fakeBackups{byTarget: map[string]Backup{
		"claude":   {Timestamp: "20260301T093000Z", Version: "v1.5.0"},
		"opencode": {Timestamp: "20260302T101500Z", Version: "v1.6.0"},
	}}
	m := enterRestore(t, newLoadedModel(t, &fakeCatalog{targets: threeCopyTargets()}, backups))

	if strings.Join(backups.asked, ",") != "claude,opencode,codex" {
		t.Errorf("backup query asked about %q, want each selected target once, in order", backups.asked)
	}
	if got := targetNames(m.pendingTargets); got != "claude, opencode" {
		t.Errorf("restore will run against %q, want the targets that have a backup", got)
	}
	rendered := stripANSI(m.View())
	for _, want := range []string{"claude: 20260301T093000Z (v1.5.0)", "opencode: 20260302T101500Z (v1.6.0)"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("confirm screen should name %q, got:\n%s", want, rendered)
		}
	}
}

// TestRestore_AnUnknownVersionIsLabelledInSpanish: the backend reports a
// backup whose version it cannot read as unknown; the UI says so in its own
// language rather than printing nothing or a made-up version.
func TestRestore_AnUnknownVersionIsLabelledInSpanish(t *testing.T) {
	backups := &fakeBackups{byTarget: map[string]Backup{
		"claude": {Timestamp: "20260101T000000Z"},
	}}
	m := enterRestore(t, newLoadedModel(t, &fakeCatalog{targets: threeCopyTargets()}, backups))

	if m.scr != screenConfirm {
		t.Fatalf("a backup with an unknown version is still restorable; got screen %v", m.scr)
	}
	if rendered := stripANSI(m.View()); !strings.Contains(rendered, "claude: 20260101T000000Z (desconocida)") {
		t.Errorf("confirm screen should label the version desconocida, got:\n%s", rendered)
	}
}

// TestRestore_ATargetWhoseBackupsCannotBeReadIsNotOffered: fail closed. A
// destructive rollback is never offered on the strength of a backup the
// backend could not confirm, and the targets whose answer is clear still are.
func TestRestore_ATargetWhoseBackupsCannotBeReadIsNotOffered(t *testing.T) {
	backups := &fakeBackups{
		byTarget: map[string]Backup{
			"claude":   {Timestamp: "20260301T093000Z", Version: "v1.5.0"},
			"opencode": {Timestamp: "20260302T101500Z", Version: "v1.6.0"},
		},
		errs: map[string]error{"opencode": errors.New("backend unreachable")},
	}
	m := enterRestore(t, newLoadedModel(t, &fakeCatalog{targets: threeCopyTargets()}, backups))

	if got := targetNames(m.pendingTargets); got != "claude" {
		t.Errorf("restore will run against %q, want only claude: opencode's backup could not be confirmed", got)
	}
}

func TestRestore_WhenNoBackupCanBeConfirmedNothingIsOffered(t *testing.T) {
	backups := &fakeBackups{errs: map[string]error{
		"claude":   errors.New("backend unreachable"),
		"opencode": errors.New("backend unreachable"),
		"codex":    errors.New("backend unreachable"),
	}}
	m := enterRestore(t, newLoadedModel(t, &fakeCatalog{targets: threeCopyTargets()}, backups))

	if m.scr != screenActions || m.pendingAction.Command == "restore" {
		t.Errorf("restore with unreadable backups reached screen %v with pending %q; it must stay a no-op", m.scr, m.pendingAction.Command)
	}
}

// TestRestore_WithoutABackupQueryOffersNothing: a model wired without the port
// (a composition mistake) fails closed instead of panicking.
func TestRestore_WithoutABackupQueryOffersNothing(t *testing.T) {
	catalog := &fakeCatalog{targets: threeCopyTargets()}
	m := newModel(deps{repoRoot: t.TempDir(), catalog: catalog})
	updated, _ := m.Update(loadTargetsCmd(catalog)())

	m = enterRestore(t, updated.(model))
	if m.scr != screenActions {
		t.Errorf("restore without a backup query reached screen %v, want it to stay a no-op", m.scr)
	}
}
