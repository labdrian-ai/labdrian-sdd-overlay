package main

// How the model uses the BackupQuery port to decide whether restore is
// offered and what its confirm screen says. The query itself is a fake here;
// what the real backend answers is pinned in overlaycli_backend_test.go.

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// enterRestore selects the restore action on the actions screen and presses
// enter, returning the resulting model.
func enterRestore(t *testing.T, m model) model {
	t.Helper()
	m.scr = screenActions
	m.aCursor = findAction(t, m, "restore")
	updated, _ := m.updateActions(tea.KeyMsg{Type: tea.KeyEnter})
	return updated.(model)
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
