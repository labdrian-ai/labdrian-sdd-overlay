package durable

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The bytes a state file holds: two spaces of indentation, one key to a line, and the newline that ends the last
// line. The three files this module keeps for itself (the vault registry, the install-state record and the
// identity ledger) were written exactly so by a copy of the same sequence each.
func TestWriteJSON_WritesTheValueIndentedByTwoSpacesAndEndedByANewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	value := struct {
		Schema int               `json:"schema"`
		Rows   map[string]string `json:"rows"`
	}{Schema: 1, Rows: map[string]string{"b": "2", "a": "1"}}

	if err := WriteJSON(path, value, 0o600); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	want := "{\n  \"schema\": 1,\n  \"rows\": {\n    \"a\": \"1\",\n    \"b\": \"2\"\n  }\n}\n"
	if got := mustReadFile(t, path); got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

// A fresh install has no state directory yet. WriteJSON makes it, for the module alone, and the file in it gets the
// mode the caller asked for.
func TestWriteJSON_MakesTheDirectoryOwnerOnlyAndTheFileWithTheModeItWasGiven(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "state", "nested", "state.json")

	if err := WriteJSON(path, map[string]int{"schema": 1}, 0o640); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	for _, dir := range []string{filepath.Join(root, "state"), filepath.Join(root, "state", "nested")} {
		if got := permOf(t, dir); got != 0o700 {
			t.Errorf("mode of %s = %o, want %o", dir, got, 0o700)
		}
	}
	if got := permOf(t, path); got != 0o640 {
		t.Errorf("mode of the file = %o, want %o (the mode it was given)", got, 0o640)
	}
}

// A file that is there keeps its owner's mode, as WriteFile keeps it: the mode WriteJSON is given applies to a
// file it creates and to nothing else.
func TestWriteJSON_KeepsTheModeOfAFileThatIsThere(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	mustWriteFile(t, path, "{}\n", 0o644)

	if err := WriteJSON(path, map[string]int{"schema": 2}, 0o600); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	if got := mustReadFile(t, path); got != "{\n  \"schema\": 2\n}\n" {
		t.Errorf("content = %q, want the new value", got)
	}
	if got := permOf(t, path); got != 0o644 {
		t.Errorf("mode = %o, want %o (the file's own)", got, 0o644)
	}
}

// A value that cannot be encoded is refused before anything is touched: the file that was there is as it was, and
// no directory is made for a file that will not be written. The error names the file and says nothing of who asked.
func TestWriteJSON_RefusesAValueThatCannotBeEncodedAndTouchesNothing(t *testing.T) {
	root := t.TempDir()
	kept := filepath.Join(root, "kept.json")
	mustWriteFile(t, kept, "kept\n", 0o600)
	absent := filepath.Join(root, "dir", "absent.json")

	for _, path := range []string{kept, absent} {
		err := WriteJSON(path, make(chan int), 0o600)
		if err == nil {
			t.Fatalf("WriteJSON(%s) of a channel = nil, want an error", path)
		}
		if want := "marshal " + path + ": "; !strings.HasPrefix(err.Error(), want) {
			t.Errorf("error = %q, want it to begin %q", err, want)
		}
	}

	if got := mustReadFile(t, kept); got != "kept\n" {
		t.Errorf("the file that was there = %q, want it untouched", got)
	}
	if _, err := os.Stat(filepath.Join(root, "dir")); !os.IsNotExist(err) {
		t.Errorf("the directory of the file that was refused exists (stat error %v), want it not made", err)
	}
}

// A directory that cannot be made is a failure that names the file the caller wanted written.
func TestWriteJSON_NamesTheFileWhenItsDirectoryCannotBeMade(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	mustWriteFile(t, blocker, "a file where a directory should be\n", 0o600)
	path := filepath.Join(blocker, "state.json")

	err := WriteJSON(path, map[string]int{"schema": 1}, 0o600)

	if err == nil {
		t.Fatal("WriteJSON under a file = nil, want an error")
	}
	if want := "create directory for " + path + ": "; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error = %q, want it to begin %q", err, want)
	}
}

// The file is replaced, not rewritten in place: a reader never meets half of it. A second name of the old file (a
// hardlink) is how a rewrite in place shows, because it would change with the file, and a replacement leaves it
// with the old bytes.
func TestWriteJSON_ReplacesTheFileRatherThanRewritingItInPlace(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "state.json")
	mustWriteFile(t, path, "old\n", 0o600)
	other := filepath.Join(root, "other-name.json")
	if err := os.Link(path, other); err != nil {
		t.Skipf("cannot make a hardlink here: %v", err)
	}

	if err := WriteJSON(path, map[string]int{"schema": 4}, 0o600); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	if got := mustReadFile(t, path); got != "{\n  \"schema\": 4\n}\n" {
		t.Errorf("content = %q, want the new value", got)
	}
	if got := mustReadFile(t, other); got != "old\n" {
		t.Errorf("the other name of the old file = %q, want the old bytes: the file was rewritten in place", got)
	}
}

// What is written goes through WriteFile, so it is the same replacement: through a link, onto the file it points to.
func TestWriteJSON_WritesThroughASymlinkLikeWriteFile(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "store", "state.json")
	if err := os.MkdirAll(filepath.Dir(real), 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, real, "old\n", 0o600)
	link := filepath.Join(root, "state.json")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("cannot make a symlink here: %v", err)
	}

	if err := WriteJSON(link, map[string]int{"schema": 3}, 0o600); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	if target, err := os.Readlink(link); err != nil || target != real {
		t.Errorf("the link = (%q, %v), want it still pointing to %q", target, err, real)
	}
	if got := mustReadFile(t, real); got != "{\n  \"schema\": 3\n}\n" {
		t.Errorf("the file the link points to = %q, want the new value", got)
	}
}
