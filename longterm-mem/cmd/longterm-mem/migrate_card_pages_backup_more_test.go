package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVaultBackup_AThirdBackupAtOneInstantTakesTheNextNumber(t *testing.T) {
	w, _ := migrationWorld(t)
	backup := vaultBackup{root: w.vaultRoot, clock: fixedClock{at: migrationInstant}}
	var dirs []string
	for range 3 {
		dir, err := backup.BackUp([]string{"c-000901"})
		if err != nil {
			t.Fatal(err)
		}
		dirs = append(dirs, filepath.Base(dir))
	}
	want := []string{"card-page-migration-20310203T040506Z", "card-page-migration-20310203T040506Z-2", "card-page-migration-20310203T040506Z-3"}
	if strings.Join(dirs, " ") != strings.Join(want, " ") {
		t.Errorf("the backups are %v, want %v", dirs, want)
	}
}

// The copy keeps the mode of each file it copies, so restoring it does not change who can read a page.
func TestVaultBackup_TheCopyKeepsTheModeOfTheSource(t *testing.T) {
	w, _ := migrationWorld(t)
	page := filepath.Join(w.vaultRoot, "wiki", "memory", "c-000901.md")
	sidecar := filepath.Join(w.vaultRoot, ".raw", ".longterm-mem-manifest.json")
	if err := os.Chmod(page, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sidecar, 0o604); err != nil {
		t.Fatal(err)
	}
	dir, err := vaultBackup{root: w.vaultRoot, clock: fixedClock{at: migrationInstant}}.BackUp([]string{"c-000901"})
	if err != nil {
		t.Fatal(err)
	}
	for copied, want := range map[string]fs.FileMode{
		filepath.Join(dir, "wiki", "memory", "c-000901.md"):       0o640,
		filepath.Join(dir, ".raw", ".longterm-mem-manifest.json"): 0o604,
	} {
		info, err := os.Stat(copied)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s has mode %o, want %o", copied, got, want)
		}
	}
}

// A write that fails halfway removes the directory, and the error says what failed.
func TestVaultBackup_AWriteThatFailsMidCopyLeavesNoDirectory(t *testing.T) {
	w, _ := migrationWorld(t)
	writes := 0
	backup := vaultBackup{root: w.vaultRoot, clock: fixedClock{at: migrationInstant},
		write: func(path string, data []byte, perm fs.FileMode) error {
			writes++
			if writes == 2 {
				return errors.New("disk full")
			}
			return os.WriteFile(path, data, perm)
		}}
	if _, err := backup.BackUp([]string{"c-000901", "c-000902"}); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("err = %v, want the write failure", err)
	}
	if _, err := os.Stat(filepath.Join(w.vaultRoot, filepath.FromSlash(migrationBackupDir))); !os.IsNotExist(err) {
		t.Errorf("the directory of the failed copy is still there (stat error: %v)", err)
	}
}

// When the cleanup fails too, both failures are in the error, with the directory that could not be removed.
func TestVaultBackup_AFailedCleanupIsReportedBesideTheCopyFailure(t *testing.T) {
	w, _ := migrationWorld(t)
	backup := vaultBackup{root: w.vaultRoot, clock: fixedClock{at: migrationInstant},
		write:     func(string, []byte, fs.FileMode) error { return errors.New("disk full") },
		removeAll: func(string) error { return errors.New("permission denied") }}
	_, err := backup.BackUp([]string{"c-000901"})
	if err == nil || !strings.Contains(err.Error(), "disk full") || !strings.Contains(err.Error(), "permission denied") || !strings.Contains(err.Error(), migrationBackupDir) {
		t.Fatalf("err = %v, want the copy failure, the cleanup failure and the directory", err)
	}
}
