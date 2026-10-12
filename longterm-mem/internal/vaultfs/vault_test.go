package vaultfs

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// Vault answers what is at its root: a directory, a file, or nothing the file system could examine.
func TestTheVaultRootIsExaminedWithoutBeingChanged(t *testing.T) {
	t.Run("a directory", func(t *testing.T) {
		if isDir, err := New(t.TempDir()).RootIsDirectory(); err != nil || !isDir {
			t.Fatalf("RootIsDirectory = (%v, %v), want a directory", isDir, err)
		}
	})
	t.Run("a link to a directory", func(t *testing.T) {
		dir := t.TempDir()
		link := filepath.Join(t.TempDir(), "link")
		symlinkOrSkip(t, dir, link)
		if isDir, err := New(link).RootIsDirectory(); err != nil || !isDir {
			t.Fatalf("RootIsDirectory = (%v, %v), want a directory", isDir, err)
		}
	})
	t.Run("a file", func(t *testing.T) {
		file := writeVaultFile(t, t.TempDir(), "file", "x", 0o644)
		if isDir, err := New(file).RootIsDirectory(); err != nil || isDir {
			t.Fatalf("RootIsDirectory = (%v, %v), want a file: not a directory, and no error", isDir, err)
		}
	})
	t.Run("nothing there", func(t *testing.T) {
		absent := filepath.Join(t.TempDir(), "absent")
		_, err := New(absent).RootIsDirectory()
		if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), absent) {
			t.Fatalf("error = %v, want the file system's, naming %s", err, absent)
		}
		if strings.HasPrefix(err.Error(), "promote:") {
			t.Errorf("error %q carries a consumer's name: the stat returns the file system's own error", err)
		}
	})
	t.Run("a link to nothing", func(t *testing.T) {
		link := filepath.Join(t.TempDir(), "link")
		symlinkOrSkip(t, "absent", link)
		if _, err := New(link).RootIsDirectory(); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("error = %v, want one for which errors.Is(err, fs.ErrNotExist) holds", err)
		}
	})
}
