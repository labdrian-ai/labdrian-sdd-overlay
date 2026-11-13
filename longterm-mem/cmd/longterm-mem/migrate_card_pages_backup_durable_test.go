package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// Which errors of syncing a directory are no failure of the copy: the classification, tested without a file system.
func TestToleratedDirectorySyncError(t *testing.T) {
	other := errors.New("input/output error")
	for _, c := range []struct {
		name string
		goos string
		err  error
		want bool
	}{
		{"no error", "linux", nil, false},
		{"unsupported", "linux", errors.ErrUnsupported, true},
		{"EINVAL", "linux", syscall.EINVAL, true},
		{"ENOTSUP", "darwin", syscall.ENOTSUP, true},
		{"wrapped EINVAL", "linux", &fs.PathError{Op: "sync", Path: "d", Err: syscall.EINVAL}, true},
		{"EIO", "linux", syscall.EIO, false},
		{"another error", "linux", other, false},
		{"another error on windows", "windows", other, true},
	} {
		if got := toleratedDirectorySyncError(c.goos, c.err); got != c.want {
			t.Errorf("%s: toleratedDirectorySyncError(%q, %v) = %v, want %v", c.name, c.goos, c.err, got, c.want)
		}
	}
}

// The removal of a failed copy is made durable too: the directory that held it is synced after the removal, so a crash
// cannot bring the partial copy back.
func TestVaultBackup_TheRemovalOfAFailedCopyIsSynced(t *testing.T) {
	w, _ := migrationWorld(t)
	dir := filepath.Join(w.vaultRoot, filepath.FromSlash(migrationBackupDir))
	parent := filepath.Dir(dir)
	var goneAtSync []bool
	backup := vaultBackup{root: w.vaultRoot, clock: fixedClock{at: migrationInstant},
		write: func(string, []byte, fs.FileMode) error { return errors.New("disk full") },
		syncDir: func(path string) error {
			if path == parent {
				_, err := os.Stat(dir)
				goneAtSync = append(goneAtSync, os.IsNotExist(err))
			}
			return nil
		}}
	if _, err := backup.BackUp([]string{"c-000901"}); err == nil {
		t.Fatal("the failed write answered no error")
	}
	if len(goneAtSync) != 1 || !goneAtSync[0] {
		t.Errorf("%s synced %v time(s) with the removed directory gone at each = %v, want once, after the removal", parent, len(goneAtSync), goneAtSync)
	}
}

// A removal whose sync fails is reported with the failure that caused it.
func TestVaultBackup_AFailedSyncOfTheRemovalIsReportedBesideTheCopyFailure(t *testing.T) {
	w, _ := migrationWorld(t)
	backup := vaultBackup{root: w.vaultRoot, clock: fixedClock{at: migrationInstant},
		write:   func(string, []byte, fs.FileMode) error { return errors.New("disk full") },
		syncDir: func(string) error { return errors.New("input/output error") }}
	_, err := backup.BackUp([]string{"c-000901"})
	if err == nil || !containsAll(err.Error(), "disk full", "input/output error") {
		t.Fatalf("err = %v, want both the write failure and the sync failure of the removal", err)
	}
}

func containsAll(text string, fragments ...string) bool {
	for _, f := range fragments {
		if !strings.Contains(text, f) {
			return false
		}
	}
	return true
}
