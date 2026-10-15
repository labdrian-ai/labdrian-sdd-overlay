package app

// How the planner uses the BackupQuery port to decide whether restore is
// offered and what its confirm text says. The query is a fake here; what the
// real backend answers is pinned in the adapter's contract tests.

import (
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/domain"
)

func restoreAction() domain.Action {
	return domain.Action{Name: "Restaurar respaldo", Command: "restore", Mutating: true, CopyTargetsOnly: true,
		ConfirmMessage: "Restaura el respaldo más reciente de cada destino seleccionado."}
}

// TestRestorePlanner_AsksTheBackupQueryForEachCandidate: the latest backup
// comes from the port, once per candidate and in order, not from the state
// directory's layout; the text names the ones that have one.
func TestRestorePlanner_AsksTheBackupQueryForEachCandidate(t *testing.T) {
	backups := &fakeBackups{byTarget: map[string]domain.Backup{
		"claude":   {Timestamp: "20260301T093000Z", Version: "v1.5.0"},
		"opencode": {Timestamp: "20260302T101500Z", Version: "v1.6.0"},
	}}

	message, targets, ok := NewRestorePlanner(backups).Confirmation(restoreAction(), threeCopyTargets())

	if strings.Join(backups.asked, ",") != "claude,opencode,codex" {
		t.Errorf("backup query asked about %q, want each candidate once, in order", backups.asked)
	}
	if !ok || targetNames(targets) != "claude, opencode" {
		t.Errorf("targets = %q (ok %t), want the targets that have a backup", targetNames(targets), ok)
	}
	want := restoreAction().ConfirmMessage + "\n\nRespaldo a restaurar:\n  claude: 20260301T093000Z (v1.5.0)\n  opencode: 20260302T101500Z (v1.6.0)"
	if message != want {
		t.Errorf("message =\n%q\nwant\n%q", message, want)
	}
}

// TestRestorePlanner_AnUnknownVersionIsLabelledInSpanish: the backend reports
// a backup whose version it cannot read as unknown; the UI says so in its own
// language rather than printing nothing or a made-up version.
func TestRestorePlanner_AnUnknownVersionIsLabelledInSpanish(t *testing.T) {
	backups := &fakeBackups{byTarget: map[string]domain.Backup{
		"claude": {Timestamp: "20260101T000000Z"},
	}}

	message, targets, ok := NewRestorePlanner(backups).Confirmation(restoreAction(), threeCopyTargets())

	if !ok || targetNames(targets) != "claude" {
		t.Fatalf("a backup with an unknown version is still restorable; got targets %q (ok %t)", targetNames(targets), ok)
	}
	if !strings.Contains(message, "claude: 20260101T000000Z (desconocida)") {
		t.Errorf("the text should label the version desconocida, got:\n%s", message)
	}
}

// TestRestorePlanner_ATargetWhoseBackupsCannotBeReadIsNotOffered: fail closed.
// A destructive rollback is never offered on the strength of a backup the
// backend could not confirm, and the targets whose answer is clear still are.
func TestRestorePlanner_ATargetWhoseBackupsCannotBeReadIsNotOffered(t *testing.T) {
	backups := &fakeBackups{
		byTarget: map[string]domain.Backup{
			"claude":   {Timestamp: "20260301T093000Z", Version: "v1.5.0"},
			"opencode": {Timestamp: "20260302T101500Z", Version: "v1.6.0"},
		},
		errs: map[string]error{"opencode": errors.New("backend unreachable")},
	}

	message, targets, ok := NewRestorePlanner(backups).Confirmation(restoreAction(), threeCopyTargets())

	if !ok || targetNames(targets) != "claude" {
		t.Errorf("targets = %q (ok %t), want only claude: opencode's backup could not be confirmed", targetNames(targets), ok)
	}
	if strings.Contains(message, "opencode") {
		t.Errorf("the text names opencode although its backup could not be confirmed:\n%s", message)
	}
}

func TestRestorePlanner_WhenNoBackupCanBeConfirmedNothingIsOffered(t *testing.T) {
	tests := []struct {
		name    string
		backups *fakeBackups
	}{
		{"none of the candidates has a backup", &fakeBackups{}},
		{"none of the answers can be read", &fakeBackups{errs: map[string]error{
			"claude": errors.New("backend unreachable"), "opencode": errors.New("backend unreachable"), "codex": errors.New("backend unreachable"),
		}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			message, targets, ok := NewRestorePlanner(tc.backups).Confirmation(restoreAction(), threeCopyTargets())
			if ok || len(targets) != 0 || message != "" {
				t.Errorf("Confirmation = (%q, %q, %t), want nothing offered", message, targetNames(targets), ok)
			}
		})
	}
}

// TestRestorePlanner_WithoutABackupQueryOffersNothing: a planner built without
// the port (a composition mistake) fails closed instead of panicking.
func TestRestorePlanner_WithoutABackupQueryOffersNothing(t *testing.T) {
	message, targets, ok := NewRestorePlanner(nil).Confirmation(restoreAction(), threeCopyTargets())
	if ok || len(targets) != 0 || message != "" {
		t.Errorf("Confirmation = (%q, %q, %t), want nothing offered", message, targetNames(targets), ok)
	}
}

// Restore only ever runs against the candidates it is given: the planner asks
// about no other target, which is how a package target is never queried.
func TestRestorePlanner_NeverAsksAboutATargetThatIsNotACandidate(t *testing.T) {
	backups := &fakeBackups{byTarget: map[string]domain.Backup{
		"claude": {Timestamp: "20260301T093000Z", Version: "v1.5.0"},
		"pi":     {Timestamp: "20260301T093000Z", Version: "v1.5.0"},
	}}

	_, targets, _ := NewRestorePlanner(backups).Confirmation(restoreAction(), []domain.Target{{Name: "claude", Kind: domain.KindCopy}})

	if targetNames(targets) != "claude" || strings.Join(backups.asked, ",") != "claude" {
		t.Errorf("targets %q after asking about %q, want claude alone", targetNames(targets), backups.asked)
	}
}
