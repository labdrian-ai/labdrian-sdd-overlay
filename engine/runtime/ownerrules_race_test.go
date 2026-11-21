package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var raceBlock = MarkedBlock{Open: "<!-- t:x -->", Close: "<!-- /t:x -->"}

// changeAtRename makes the file path hold text at the moment the writer is about to rename, and
// puts the hook back when the test ends.
func changeAtRename(t *testing.T, path, text string) {
	t.Helper()
	old := beforeBlockRename
	t.Cleanup(func() { beforeBlockRename = old })
	beforeBlockRename = func(string) {
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func entriesOf(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}

func TestWriteFileRefusesWhenTheFileChangedAfterItWasRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(path, []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changeAtRename(t, path, "mine\nedited meanwhile\n")
	if _, err := raceBlock.WriteFile(path, "body"); err == nil || !strings.Contains(err.Error(), "changed while it was being written") {
		t.Errorf("WriteFile error = %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "mine\nedited meanwhile\n" {
		t.Errorf("the edit was lost: %q", got)
	}
	if n := entriesOf(t, dir); n != 1 {
		t.Errorf("%d entries, want only CLAUDE.md (no temporary file)", n)
	}
}

func TestWriteFileRefusesWhenAFileAppearedAfterItWasFoundAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CLAUDE.md")
	changeAtRename(t, path, "created meanwhile\n")
	if _, err := raceBlock.WriteFile(path, "body"); err == nil {
		t.Error("WriteFile did not refuse")
	}
	if got, _ := os.ReadFile(path); string(got) != "created meanwhile\n" {
		t.Errorf("the file = %q", got)
	}
}

func TestRemoveFileRefusesWhenTheFileChangedAfterItWasRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CLAUDE.md")
	if _, err := raceBlock.WriteFile(path, "body"); err != nil {
		t.Fatal(err)
	}
	changeAtRename(t, path, "mine now\n")
	if _, err := raceBlock.RemoveFile(path); err == nil {
		t.Error("RemoveFile did not refuse")
	}
	if got, _ := os.ReadFile(path); string(got) != "mine now\n" {
		t.Errorf("the file = %q", got)
	}
}
