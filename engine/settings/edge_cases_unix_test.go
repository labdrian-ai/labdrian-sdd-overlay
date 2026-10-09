//go:build unix

package settings_test

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

// The cases below are the ones the golden file cannot set up: a file that is not a plain file.
// What each pins is what the program did before H27 (recorded by running it over the same
// layouts), so a change in any of them is a decision and not an accident.

func edgeWorld(t *testing.T) (dir, path string) {
	t.Helper()
	previous := syscall.Umask(0o022)
	t.Cleanup(func() { syscall.Umask(previous) })
	dir = t.TempDir()
	return dir, filepath.Join(dir, "settings.json")
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func modeOfFile(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode()
}

// A settings.json that is a link (a dotfile manager's) is replaced by a plain file: the link is
// gone, its target keeps the old content, and the backup holds what the target held.
func TestInstallReplacesALinkedSettingsFileWithAPlainOne(t *testing.T) {
	dir, path := edgeWorld(t)
	target := filepath.Join(dir, "dotfiles.json")
	if err := os.WriteFile(target, []byte(`{"model":"linked"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}

	if err := settings.NewMerger(path, "/opt/h/bin").Install(); err != nil {
		t.Fatalf("Install() = %v", err)
	}

	if mode := modeOfFile(t, path); !mode.IsRegular() || mode.Perm() != 0o600 {
		t.Errorf("settings.json has mode %v, want a plain file of 0600", mode)
	}
	if got := readText(t, target); got != `{"model":"linked"}` {
		t.Errorf("the target of the old link was changed: %q", got)
	}
	if got := readText(t, path+".bak"); got != `{"model":"linked"}` {
		t.Errorf("the backup holds %q, want what the link pointed at", got)
	}
}

// A link to nothing is a settings.json that does not exist: a plain file is written in its place
// and there is nothing to back up.
func TestInstallWritesAPlainFileOverADanglingLink(t *testing.T) {
	dir, path := edgeWorld(t)
	if err := os.Symlink(filepath.Join(dir, "gone.json"), path); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}

	if err := settings.NewMerger(path, "/opt/h/bin").Install(); err != nil {
		t.Fatalf("Install() = %v", err)
	}

	if !modeOfFile(t, path).IsRegular() {
		t.Error("settings.json is still a link")
	}
	if _, err := os.Lstat(path + ".bak"); !os.IsNotExist(err) {
		t.Errorf("a backup was made of a file that did not exist: %v", err)
	}
}

// A backup that cannot be written stops the install before the file is replaced.
func TestInstallLeavesTheFileAloneWhenTheBackupNameIsADirectory(t *testing.T) {
	_, path := edgeWorld(t)
	if err := os.WriteFile(path, []byte(`{"model":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path+".bak", 0o755); err != nil {
		t.Fatal(err)
	}

	err := settings.NewMerger(path, "/opt/h/bin").Install()

	if err == nil || !strings.HasPrefix(err.Error(), "settings: backup to ") {
		t.Fatalf("Install() = %v, want a backup error", err)
	}
	if got := readText(t, path); got != `{"model":"x"}` {
		t.Errorf("settings.json was changed although its backup failed: %q", got)
	}
}

// A string that is not UTF-8 is read as the replacement character and written back as it, the
// way encoding/json treats it.
func TestInstallWritesTheReplacementCharacterForInvalidUTF8(t *testing.T) {
	_, path := edgeWorld(t)
	if err := os.WriteFile(path, []byte("{\"note\":\"\xff\xfe\"}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := settings.NewMerger(path, "/opt/h/bin").Install(); err != nil {
		t.Fatalf("Install() = %v", err)
	}

	if got := readText(t, path); !strings.Contains(got, "\"note\": \"��\"") {
		t.Errorf("settings.json = %q, want the note written as two replacement characters", got)
	}
	if got := readText(t, path+".bak"); got != "{\"note\":\"\xff\xfe\"}" {
		t.Errorf("the backup is not the original bytes: %q", got)
	}
}

// A settings.json that is a directory is a read error, for install and uninstall alike, and
// nothing is created.
func TestInstallAndUninstallRefuseASettingsPathThatIsADirectory(t *testing.T) {
	_, path := edgeWorld(t)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	m := settings.NewMerger(path, "/opt/h/bin")

	for name, act := range map[string]func() error{"Install": m.Install, "Uninstall": m.Uninstall} {
		err := act()
		if err == nil || !strings.HasPrefix(err.Error(), "settings: read "+path+": ") {
			t.Errorf("%s() = %v, want a read error naming the path", name, err)
		}
	}
	if _, err := os.Lstat(path + ".bak"); !os.IsNotExist(err) {
		t.Errorf("a backup exists: %v", err)
	}
}
