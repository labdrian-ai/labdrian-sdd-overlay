package settingsfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

func TestCodexAdapterUsesAtomicBackupsAndRefusesUnsafeBackup(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "hooks.json")
	const original = `{"future" : 9007199254740993}`
	if err := os.WriteFile(path, []byte(original), 0o640); err != nil {
		t.Fatal(err)
	}
	// Backup symlinks must never be followed, even when the target is synthetic.
	target := filepath.Join(home, "foreign")
	if err := os.WriteFile(target, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path+".bak"); err != nil {
		t.Fatal(err)
	}
	if err := (CodexHooks{}).Install(path, "/opt/engine"); err == nil {
		t.Fatal("accepted symlink backup")
	}
	for file, want := range map[string]string{path: original, target: "unchanged"} {
		data, err := os.ReadFile(file)
		if err != nil || string(data) != want {
			t.Fatalf("%s: %s %v", file, data, err)
		}
	}
	// An existing empty file is malformed, not a missing file to provision.
	empty := filepath.Join(home, "empty.json")
	if err := os.WriteFile(empty, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (CodexHooks{}).Install(empty, "/opt/engine"); err == nil {
		t.Fatal("accepted an existing empty hooks.json")
	}
	if stat, err := os.Stat(empty); err != nil || stat.Size() != 0 {
		t.Fatalf("empty file changed: %v %v", stat, err)
	}
	if _, err := os.Stat(empty + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("refused empty file created backup: %v", err)
	}
	// A second independent path proves backup bytes/mode and the published mode.
	path = filepath.Join(home, "safe.json")
	if err := os.WriteFile(path, []byte(original), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := (CodexHooks{}).Install(path, "/opt/engine"); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(path + ".bak")
	if err != nil || string(backup) != original {
		t.Fatalf("backup: %s %v", backup, err)
	}
	for file, want := range map[string]os.FileMode{path: 0o600, path + ".bak": 0o640} {
		stat, err := os.Stat(file)
		if err != nil || stat.Mode().Perm() != want {
			t.Fatalf("%s: %v %v", file, stat, err)
		}
	}
}

// File.Write shares the publication path now: Claude's exact serialized bytes stay unchanged.
func TestClaudeFileWriteStillUsesDocumentBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	doc := settings.Empty()
	if _, err := doc.Merge("/opt/engine"); err != nil {
		t.Fatal(err)
	}
	want, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if err := (File{Path: path}).Write(doc); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(want) {
		t.Fatalf("write: %s %v", data, err)
	}
}
