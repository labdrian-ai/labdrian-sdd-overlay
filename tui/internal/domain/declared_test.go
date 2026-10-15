package domain

// The fields an action declares in place of a check on its command's name
// (T3): how its result is shown, whether a success refreshes the launch probe,
// and whether it needs a backup to go back to. The menu says which actions
// declare what, and these tests are where that is written down; the program
// reads the fields and never the names.

import "testing"

// topLevelCommands returns the menu's rows keyed by command, in menu order
// within a command (the three longterm-mem rows share one).
func topLevelCommands(t *testing.T) map[string][]Action {
	t.Helper()
	byCommand := map[string][]Action{}
	for _, a := range Actions() {
		byCommand[a.Command] = append(byCommand[a.Command], a)
	}
	return byCommand
}

// TestActions_OnlySyncCheckIsReadAsAReport: sync-check is the one action whose
// output is a report the dashboard is built from; every other row, and every
// action merged into a row, shows its transcript.
func TestActions_OnlySyncCheckIsReadAsAReport(t *testing.T) {
	var check func(a Action)
	check = func(a Action) {
		want := ResultTranscript
		if a.Command == "sync-check" {
			want = ResultSyncDashboard
		}
		if a.ResultKind != want {
			t.Errorf("%s (%v) has ResultKind %d, want %d", a.Command, a.Args, a.ResultKind, want)
		}
		for _, sub := range a.Also {
			check(sub)
		}
	}
	for _, a := range Actions() {
		check(a)
	}
	if got := topLevelCommands(t)["sync-check"]; len(got) != 1 {
		t.Errorf("the menu has %d sync-check rows, want exactly one", len(got))
	}
}

// TestActions_OnlySelfUpdateRefreshesTheProbe: self-update fast-forwards main,
// which is the cached ref the launch-time probe reads, so its success re-runs
// the probe and the banner corrects itself. Nothing else the menu runs moves
// the repository.
func TestActions_OnlySelfUpdateRefreshesTheProbe(t *testing.T) {
	var check func(a Action)
	check = func(a Action) {
		if want := a.Command == "self-update"; a.RefreshProbe != want {
			t.Errorf("%s (%v) has RefreshProbe %t, want %t", a.Command, a.Args, a.RefreshProbe, want)
		}
		for _, sub := range a.Also {
			check(sub)
		}
	}
	for _, a := range Actions() {
		check(a)
	}
}

// TestActions_OnlyRestoreRequiresABackup: restore is never offered for a
// target with nothing to go back to (R-003). It is also, by the backend's own
// rule, a per-file action and a destructive one -- a RequiresBackup row that
// were not CopyTargetsOnly would ask a package target, which keeps no backups.
func TestActions_OnlyRestoreRequiresABackup(t *testing.T) {
	var check func(a Action)
	check = func(a Action) {
		if want := a.Command == "restore"; a.RequiresBackup != want {
			t.Errorf("%s (%v) has RequiresBackup %t, want %t", a.Command, a.Args, a.RequiresBackup, want)
		}
		if a.RequiresBackup && (!a.CopyTargetsOnly || !a.Mutating) {
			t.Errorf("%s requires a backup but is CopyTargetsOnly %t and Mutating %t: a rollback is destructive and only per-file", a.Command, a.CopyTargetsOnly, a.Mutating)
		}
		for _, sub := range a.Also {
			check(sub)
		}
	}
	for _, a := range Actions() {
		check(a)
	}
}
