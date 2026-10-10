package fsfiles_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/status/fsfiles"
)

func TestStatAnswersThePermissionBitsOfAFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bin")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o751); err != nil {
		t.Fatal(err)
	}
	mode, err := fsfiles.Files{}.Stat(path)
	if err != nil || mode.Perm() != 0o751 || mode.IsDir() {
		t.Errorf("Stat() = %v, %v, want a regular file with 0751", mode, err)
	}
}

func TestStatSaysADirectoryIsOneAndFollowsALink(t *testing.T) {
	dir := t.TempDir()
	if mode, err := (fsfiles.Files{}).Stat(dir); err != nil || !mode.IsDir() {
		t.Errorf("Stat(dir) = %v, %v, want a directory", mode, err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Skipf("no symbolic links here: %v", err)
	}
	if mode, err := (fsfiles.Files{}).Stat(link); err != nil || !mode.IsDir() {
		t.Errorf("Stat(link to a directory) = %v, %v, want the directory it leads to", mode, err)
	}
	dangling := filepath.Join(t.TempDir(), "dangling")
	if err := os.Symlink(filepath.Join(dir, "nowhere"), dangling); err != nil {
		t.Fatal(err)
	}
	if _, err := (fsfiles.Files{}).Stat(dangling); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat(link to nothing) = %v, want an absence", err)
	}
}

func TestAnAbsentPathIsAnAbsenceAndAFileInTheWayIsNot(t *testing.T) {
	dir := t.TempDir()
	if _, err := (fsfiles.Files{}).Stat(filepath.Join(dir, "none")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat(absent) = %v, want an absence", err)
	}
	if _, err := (fsfiles.Files{}).ReadFile(filepath.Join(dir, "none")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadFile(absent) = %v, want an absence", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (fsfiles.Files{}).Stat(filepath.Join(dir, "file", "below")); err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat(under a file) = %v, want an error that is not an absence", err)
	}
	if _, err := (fsfiles.Files{}).ReadFile(filepath.Join(dir, "file", "below")); err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadFile(under a file) = %v, want an error that is not an absence", err)
	}
}

func TestReadFileAnswersTheBytesAndRefusesADirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.md")
	if err := os.WriteFile(path, []byte("body\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := fsfiles.Files{}.ReadFile(path)
	if err != nil || string(data) != "body\n" {
		t.Errorf("ReadFile() = %q, %v", data, err)
	}
	if _, err := (fsfiles.Files{}).ReadFile(dir); err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadFile(directory) = %v, want the error of the system and not an absence", err)
	}
}
