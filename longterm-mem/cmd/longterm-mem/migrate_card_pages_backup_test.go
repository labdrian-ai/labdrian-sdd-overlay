package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The backup of the migration is the only way back, so its own failures are tested: two backups in one second, a copy
// that fails halfway, and putting the copy back over the vault.

func TestVaultBackup_TwoBackupsAtOneInstantDoNotCollide(t *testing.T) {
	w, _ := migrationWorld(t)
	backup := vaultBackup{root: w.vaultRoot, clock: fixedClock{at: migrationInstant}}
	first, err := backup.BackUp([]string{"c-000901"})
	if err != nil {
		t.Fatalf("first backup: %v", err)
	}
	second, err := backup.BackUp([]string{"c-000901"})
	if err != nil {
		t.Fatalf("second backup at the same instant: %v", err)
	}
	if first == second {
		t.Fatalf("both backups are in %s", first)
	}
	for _, dir := range []string{first, second} {
		if _, err := os.Stat(filepath.Join(dir, "wiki", "memory", "c-000901.md")); err != nil {
			t.Errorf("the backup %s lacks the page: %v", dir, err)
		}
	}
}

func TestVaultBackup_AFailedCopyLeavesNoPartialDirectory(t *testing.T) {
	w, _ := migrationWorld(t)
	backup := vaultBackup{root: w.vaultRoot, clock: fixedClock{at: migrationInstant}}
	if _, err := backup.BackUp([]string{"c-000901", "c-no-such-page"}); err == nil {
		t.Fatal("a backup of a page that is not there answered no error")
	}
	if _, err := os.Stat(filepath.Join(w.vaultRoot, filepath.FromSlash(migrationBackupDir))); !os.IsNotExist(err) {
		t.Errorf("the failed backup left its directory behind (stat error: %v)", err)
	}
	// And the retry in the same second is not blocked by it.
	if _, err := backup.BackUp([]string{"c-000901"}); err != nil {
		t.Errorf("the retry: %v", err)
	}
}

// Putting the copy back over the vault returns every rewritten file to the bytes it had before the migration.
func TestMigrateCardPages_RestoringTheBackupReturnsTheVaultToItsBytes(t *testing.T) {
	w, _ := migrationWorld(t)
	before := cmdVaultSnapshot(t, w.vaultRoot)
	if code, so, se := runMigrate(t, "--apply"); code != exitRegistrationConflict {
		t.Fatalf("exit %d: %s %s", code, so, se)
	}
	backupDir := filepath.Join(w.vaultRoot, filepath.FromSlash(migrationBackupDir))
	err := filepath.WalkDir(backupDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(backupDir, path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(w.vaultRoot, rel), data, 0o644)
	})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	after := map[string]string{}
	for path, content := range cmdVaultSnapshot(t, w.vaultRoot) {
		if !strings.HasPrefix(filepath.ToSlash(path), migrationBackupDir+"/") {
			after[path] = content
		}
	}
	if changed := changedPaths(before, after); len(changed) != 0 {
		t.Errorf("after restoring the backup the vault differs from before the migration at %v", changed)
	}
}
