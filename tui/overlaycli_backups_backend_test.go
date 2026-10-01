package main

// Contract tests between the overlayCLI backup query and the REAL
// `labdrian-overlay restore --target <t> --list`: what the TUI assumes about
// which backup is the latest, and about whose state directory it reads. They
// run the actual script under scratch HOME and STATE_DIR (see
// hermeticBackendEnv in overlaycli_backend_test.go).

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOverlayCLIContract_LatestBackup_NoBackups(t *testing.T) {
	cli, _, _ := realOverlayCLI(t)

	b, ok, err := cli.LatestBackup("claude")
	if err != nil || ok || b != (Backup{}) {
		t.Errorf("LatestBackup = %+v, %v, %v; want no backup and no error for a target without any", b, ok, err)
	}
}

// TestOverlayCLIContract_LatestBackup_PicksTheNewest: the TUI always targets
// the most recent backup, and relies on the backend listing it last.
func TestOverlayCLIContract_LatestBackup_PicksTheNewest(t *testing.T) {
	cli, _, stateDir := realOverlayCLI(t)
	writeBackupFixture(t, stateDir, "claude", "20260101T000000Z", "v1.2.0\tabc123\t2026-01-01T00:00:00Z")
	writeBackupFixture(t, stateDir, "claude", "20260215T120000Z", "v1.3.0\tdef456\t2026-02-15T12:00:00Z")

	b, ok, err := cli.LatestBackup("claude")
	if err != nil || !ok {
		t.Fatalf("LatestBackup: ok=%v err=%v", ok, err)
	}
	if b != (Backup{Timestamp: "20260215T120000Z", Version: "v1.3.0"}) {
		t.Errorf("backup = %+v, want the chronologically last one with its .meta version", b)
	}
}

// TestOverlayCLIContract_LatestBackup_SameSecondSuffixIsNewer: two backups
// taken in the same second get a "-N" suffix on the later one, and the backend
// must list it after the first, or the TUI would offer the older of the two.
func TestOverlayCLIContract_LatestBackup_SameSecondSuffixIsNewer(t *testing.T) {
	cli, _, stateDir := realOverlayCLI(t)
	writeBackupFixture(t, stateDir, "claude", "20260215T120000Z", "v1.3.0\tdef456\t2026-02-15T12:00:00Z")
	writeBackupFixture(t, stateDir, "claude", "20260215T120000Z-1", "v1.4.0\tghi789\t2026-02-15T12:00:00Z")

	b, ok, err := cli.LatestBackup("claude")
	if err != nil || !ok || b.Timestamp != "20260215T120000Z-1" || b.Version != "v1.4.0" {
		t.Errorf("LatestBackup = %+v, %v, %v; want the -1 backup (v1.4.0)", b, ok, err)
	}
}

// TestOverlayCLIContract_LatestBackup_FollowsTheBackendsStateDir is the drift
// T2 removes: the old lookup read $HOME/.labdrian-overlay/backups itself, so
// it disagreed with the backend whenever STATE_DIR pointed elsewhere. A decoy
// newer backup sits where the old lookup would have looked.
func TestOverlayCLIContract_LatestBackup_FollowsTheBackendsStateDir(t *testing.T) {
	cli, home, stateDir := realOverlayCLI(t)
	if stateDir == filepath.Join(home, ".labdrian-overlay") {
		t.Fatal("test setup: STATE_DIR must differ from the default location for this test to mean anything")
	}
	writeBackupFixture(t, stateDir, "claude", "20260101T000000Z", "v1.2.0\tabc123\t2026-01-01T00:00:00Z")
	writeBackupFixture(t, filepath.Join(home, ".labdrian-overlay"), "claude", "20990101T000000Z", "v9.9.9\tdecoy\t2099-01-01T00:00:00Z")

	b, ok, err := cli.LatestBackup("claude")
	if err != nil || !ok {
		t.Fatalf("LatestBackup: ok=%v err=%v", ok, err)
	}
	if b.Timestamp != "20260101T000000Z" {
		t.Errorf("backup = %+v; the TUI read a state directory other than the one the backend uses", b)
	}
}

// TestOverlayCLIContract_LatestBackup_MetaThatNamesNoVersion: a backup taken
// before any version was recorded, or whose .meta is gone, is still
// restorable; only its version label is unknown.
func TestOverlayCLIContract_LatestBackup_MetaThatNamesNoVersion(t *testing.T) {
	for name, meta := range map[string]string{
		"NEVER_DEPLOYED": "NEVER_DEPLOYED",
		"no .meta file":  noMeta,
		"empty .meta":    "",
	} {
		t.Run(name, func(t *testing.T) {
			cli, _, stateDir := realOverlayCLI(t)
			writeBackupFixture(t, stateDir, "claude", "20260101T000000Z", meta)

			b, ok, err := cli.LatestBackup("claude")
			if err != nil || !ok {
				t.Fatalf("LatestBackup: ok=%v err=%v; the backup is restorable", ok, err)
			}
			if b.Timestamp != "20260101T000000Z" || b.Version != "" {
				t.Errorf("backup = %+v, want the timestamp and an empty (unknown) version", b)
			}
		})
	}
}

// TestOverlayCLIContract_LatestBackup_TargetIsolation: one target's backups
// never answer for another's.
func TestOverlayCLIContract_LatestBackup_TargetIsolation(t *testing.T) {
	cli, _, stateDir := realOverlayCLI(t)
	writeBackupFixture(t, stateDir, "claude", "20260101T000000Z", "v1.0.0\tabc\t2026-01-01T00:00:00Z")

	if b, ok, err := cli.LatestBackup("opencode"); err != nil || ok {
		t.Errorf("LatestBackup(opencode) = %+v, %v, %v; want no backup, since only claude has any", b, ok, err)
	}
}

// TestOverlayCLIContract_LatestBackup_APackageTargetIsAnError: the backend
// refuses restore for a package target, so asking about one is an error rather
// than an empty answer. The model never asks (restore is CopyTargetsOnly), and
// this pins why that guard exists.
func TestOverlayCLIContract_LatestBackup_APackageTargetIsAnError(t *testing.T) {
	cli, _, stateDir := realOverlayCLI(t)
	// Even a leftover directory must not make a package target restorable.
	if err := os.MkdirAll(filepath.Join(stateDir, "backups", "pi", "20260101T000000Z"), 0o755); err != nil {
		t.Fatal(err)
	}

	if b, ok, err := cli.LatestBackup("pi"); err == nil || ok {
		t.Errorf("LatestBackup(pi) = %+v, %v, %v; want an error", b, ok, err)
	}
}

// TestOverlayCLIContract_RestoreListIsReadOnly: asking never changes anything
// -- the listing is a query, so the scratch state stays as it was.
func TestOverlayCLIContract_RestoreListIsReadOnly(t *testing.T) {
	cli, home, stateDir := realOverlayCLI(t)
	writeBackupFixture(t, stateDir, "claude", "20260101T000000Z", "v1.2.0\tabc123\t2026-01-01T00:00:00Z")

	before := listTree(t, home)
	if _, _, err := cli.LatestBackup("claude"); err != nil {
		t.Fatalf("LatestBackup: %v", err)
	}
	after := listTree(t, home)

	if len(before) != len(after) {
		t.Errorf("the listing changed the scratch state: before %v, after %v", before, after)
	}
}

// listTree returns every path under root, for before/after comparison.
func listTree(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.Walk(root, func(path string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return paths
}
