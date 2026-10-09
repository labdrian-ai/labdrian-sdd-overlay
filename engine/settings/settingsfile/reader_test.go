package settingsfile_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings/settingsfile"
)

func TestReaderReadsTheObjectOfAFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	write(t, path, `{"hooks":{"SessionEnd":[]},"model":"x"}`)
	doc, err := settingsfile.Reader{}.Settings(path)
	if err != nil {
		t.Fatalf("Settings() = %v", err)
	}
	if doc.Null() || doc.Root()["model"] != "x" {
		t.Errorf("Settings() = %+v, want the object of the file", doc.Root())
	}
}

func TestReaderTakesAMissingFileAsOneWithNothingInIt(t *testing.T) {
	doc, err := settingsfile.Reader{}.Settings(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("Settings() of a file that is not there = %v, want no error", err)
	}
	if !doc.Null() || doc.Root() != nil {
		t.Errorf("Settings() of a file that is not there = %+v, want the document with no root", doc.Root())
	}
}

func TestReaderTakesTheJSONValueNullAsAFileWithNothingInIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	write(t, path, "null")
	doc, err := settingsfile.Reader{}.Settings(path)
	if err != nil || !doc.Null() {
		t.Errorf("Settings() of null = %+v, %v, want no root and no error", doc.Root(), err)
	}
}

func TestReaderSaysWhenTheContentIsNotAnObject(t *testing.T) {
	for name, content := range map[string]string{
		"not JSON":  "{not json",
		"empty":     "",
		"an array":  "[]",
		"a string":  `"text"`,
		"truncated": `{"hooks": {`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			write(t, path, content)
			_, err := settingsfile.Reader{}.Settings(path)
			if err == nil || !strings.HasPrefix(err.Error(), "invalid JSON: ") {
				t.Errorf("Settings() = %v, want an error that begins with %q", err, "invalid JSON: ")
			}
		})
	}
}

func TestReaderReportsWhatTheSystemSaysOfAFileItCannotRead(t *testing.T) {
	dir := t.TempDir()
	_, err := settingsfile.Reader{}.Settings(dir)
	if err == nil {
		t.Fatal("Settings() of a directory = nil, want the error of the system")
	}
	if strings.Contains(err.Error(), "invalid JSON") || !strings.Contains(err.Error(), dir) {
		t.Errorf("Settings() of a directory = %q, want the error of the read, naming the path, and not a JSON error", err)
	}
}

func TestReaderDoesNotTakeAFileInTheWayOfThePathForAMissingOne(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".claude"), "a file where the directory should be")
	if _, err := (settingsfile.Reader{}).Settings(filepath.Join(dir, ".claude", "settings.json")); err == nil {
		t.Error("Settings() under a file = nil, want the error of the system: the settings are not absent, they cannot be looked for")
	}
}

func TestReaderFollowsASymbolicLink(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "real.json"), `{"model":"linked"}`)
	if err := os.Symlink(filepath.Join(dir, "real.json"), filepath.Join(dir, "settings.json")); err != nil {
		t.Skipf("no symbolic links here: %v", err)
	}
	doc, err := settingsfile.Reader{}.Settings(filepath.Join(dir, "settings.json"))
	if err != nil || doc.Root()["model"] != "linked" {
		t.Errorf("Settings() through a link = %v, %v, want the file it leads to", doc.Root(), err)
	}
}
