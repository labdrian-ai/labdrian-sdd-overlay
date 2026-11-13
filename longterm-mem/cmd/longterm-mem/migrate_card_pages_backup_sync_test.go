package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The copy is only a way back if it survives a crash: its directories are synced with it, those that hold it
// included, and a directory that cannot be synced fails the copy and takes it away.
func TestVaultBackup_SyncsTheCopyAndTheDirectoriesThatHoldIt(t *testing.T) {
	w, _ := migrationWorld(t)
	if _, err := os.Stat(filepath.Join(w.vaultRoot, ".vault-meta")); !os.IsNotExist(err) {
		t.Fatalf(".vault-meta must be absent before the copy, which makes it (stat error: %v)", err)
	}
	var synced []string
	backup := vaultBackup{root: w.vaultRoot, clock: fixedClock{at: migrationInstant},
		syncDir: func(path string) error { synced = append(synced, path); return nil }}
	dir, err := backup.BackUp([]string{"c-000901"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{dir, filepath.Join(dir, "wiki"), filepath.Join(dir, "wiki", "memory"), filepath.Join(w.vaultRoot, ".vault-meta"), w.vaultRoot} {
		found := false
		for _, got := range synced {
			found = found || got == want
		}
		if !found {
			t.Errorf("%s was not synced (synced: %v)", want, synced)
		}
	}
}

func TestVaultBackup_ADirectoryThatCannotBeSyncedFailsTheCopyAndRemovesIt(t *testing.T) {
	w, _ := migrationWorld(t)
	backup := vaultBackup{root: w.vaultRoot, clock: fixedClock{at: migrationInstant},
		syncDir: func(string) error { return errors.New("input/output error") }}
	dir := filepath.Join(w.vaultRoot, filepath.FromSlash(migrationBackupDir))
	if _, err := backup.BackUp([]string{"c-000901"}); err == nil || !strings.Contains(err.Error(), "input/output error") {
		t.Fatalf("err = %v, want the sync failure", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the directory of the failed copy is still there (stat error: %v)", err)
	}
}
