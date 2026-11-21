package statestore

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendLineKeepsOneLineForEachCallAndLastLineReadsTheLast(t *testing.T) {
	parts := []string{t.TempDir(), "labdrian", "x", "y"}
	path := filepath.Join(filepath.Join(parts...), "log")
	if _, err := LastLine(path, 64); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("LastLine of a missing file = %v, want not exist", err)
	}
	for _, line := range []string{"one", "two", "three"} {
		if err := AppendLine(parts, "log", line); err != nil {
			t.Fatal(err)
		}
	}
	if data, _ := os.ReadFile(path); string(data) != "one\ntwo\nthree\n" {
		t.Errorf("file = %q, want one line for each call", data)
	}
	if got, err := LastLine(path, 64); err != nil || got != "three" {
		t.Errorf("LastLine() = %q, %v, want three", got, err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v, want 0600", info.Mode().Perm())
	}
	if info, _ := os.Stat(filepath.Dir(path)); info.Mode().Perm() != 0o700 {
		t.Errorf("directory mode = %v, want 0700", info.Mode().Perm())
	}
}

func TestLastLineReadsOnlyTheWindowAndIgnoresAHalfWrittenLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	long := strings.Repeat("a", 5000)
	if err := os.WriteFile(path, []byte(long+"\nlast\npartial"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := LastLine(path, 16); err != nil || got != "last" {
		t.Errorf("LastLine() = %q, %v, want last: the half written line is not the last", got, err)
	}
	if err := os.WriteFile(path, []byte("x\n"+long+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := LastLine(path, 16); err != nil || got != strings.Repeat("a", 15) {
		t.Errorf("LastLine() over a longer line = %q, %v, want its last 15 bytes", got, err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := LastLine(path, 16); err != nil || got != "" {
		t.Errorf("LastLine() of an empty file = %q, %v, want none", got, err)
	}
	if _, err := LastLine(path, 1); err == nil {
		t.Errorf("LastLine() with a window of 1 = nil error")
	}
}

func TestAppendLineAndLastLineRefuseLinksAndDirectoriesThatAreNotDirectories(t *testing.T) {
	home := t.TempDir()
	parts := []string{home, "labdrian", "d"}
	if err := AppendLine(parts, "log", "x"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(filepath.Join(parts...), "log")
	target := filepath.Join(home, "elsewhere")
	_ = os.WriteFile(target, nil, 0o600)
	_ = os.Remove(path)
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := LastLine(path, 64); !errors.Is(err, ErrSymlink) {
		t.Errorf("LastLine() over a symlink = %v, want ErrSymlink", err)
	}
	if err := AppendLine(parts, "log", "y"); err == nil {
		t.Errorf("AppendLine() over a symlink = nil error")
	}
	if data, _ := os.ReadFile(target); len(data) != 0 {
		t.Errorf("a symlink was followed and %q written", data)
	}
	blocked := []string{home, "labdrian", "d", "log", "below"}
	_ = os.Remove(path)
	_ = os.WriteFile(path, nil, 0o600)
	if err := AppendLine(blocked, "log", "z"); err == nil {
		t.Errorf("AppendLine() under a file = nil error")
	}
}
