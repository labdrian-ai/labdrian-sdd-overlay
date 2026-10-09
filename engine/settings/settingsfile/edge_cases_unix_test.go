//go:build unix

package settingsfile_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/atomicfile"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings/settingsfile"
)

// The cases below are the ones the golden file cannot set up: a file that is not a plain file.
// Each pins what the program did before H27, except where it says it does not.

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

func lstatMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode()
}

func namesIn(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return strings.Join(names, " ")
}

// BEHAVIOR CHANGE OF H27. A settings.json that is a link (a dotfile manager's) used to be replaced
// by a plain file: the link was gone, its target kept the old content, and the backup held what
// the target held. atomicfile, the one write the engine now has, never replaces a link, so the
// install is refused before it stages, copies or renames anything.
func TestInstallRefusesALinkedSettingsFileAndChangesNothing(t *testing.T) {
	dir, path := edgeWorld(t)
	target := filepath.Join(dir, "dotfiles.json")
	if err := os.WriteFile(target, []byte(`{"model":"linked"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}

	err := settingsfile.Installer{}.Install(path, binary)

	if !errors.Is(err, atomicfile.ErrSymlink) {
		t.Fatalf("Install() = %v, want atomicfile.ErrSymlink", err)
	}
	if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "nothing was changed") {
		t.Errorf("Install() = %q, want it to name the path and say nothing was changed", err)
	}
	if lstatMode(t, path)&os.ModeSymlink == 0 {
		t.Error("the link was replaced")
	}
	if got := readText(t, target); got != `{"model":"linked"}` {
		t.Errorf("the target of the link was changed: %q", got)
	}
	if names := namesIn(t, dir); names != "dotfiles.json settings.json" {
		t.Errorf("the directory holds %q, want only the target and the link: no backup, no temporary file", names)
	}
}

// A link to nothing is refused as well (it used to be replaced by a plain file, with no backup).
func TestInstallRefusesADanglingLink(t *testing.T) {
	dir, path := edgeWorld(t)
	if err := os.Symlink(filepath.Join(dir, "gone.json"), path); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}

	if err := (settingsfile.Installer{}).Install(path, binary); !errors.Is(err, atomicfile.ErrSymlink) {
		t.Fatalf("Install() = %v, want atomicfile.ErrSymlink", err)
	}
	if lstatMode(t, path)&os.ModeSymlink == 0 {
		t.Error("the link was replaced")
	}
}

// Uninstall through a link that holds nothing of ours writes nothing, so it is not refused.
func TestUninstallThroughALinkThatHoldsNothingOfOursChangesNothing(t *testing.T) {
	dir, path := edgeWorld(t)
	target := filepath.Join(dir, "dotfiles.json")
	if err := os.WriteFile(target, []byte(`{"model":"linked"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}

	if err := (settingsfile.Installer{}).Uninstall(path, binary); err != nil {
		t.Fatalf("Uninstall() = %v", err)
	}
	if lstatMode(t, path)&os.ModeSymlink == 0 {
		t.Error("the link was replaced")
	}
}

// Uninstall through a link that does hold our hooks has a file to write, and is refused like install.
func TestUninstallThroughALinkThatHoldsOurHooksIsRefused(t *testing.T) {
	dir, path := edgeWorld(t)
	target := filepath.Join(dir, "dotfiles.json")
	if err := (settingsfile.Installer{}).Install(target, binary); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	before := readText(t, target)

	if err := (settingsfile.Installer{}).Uninstall(path, binary); !errors.Is(err, atomicfile.ErrSymlink) {
		t.Fatalf("Uninstall() = %v, want atomicfile.ErrSymlink", err)
	}
	if got := readText(t, target); got != before {
		t.Error("the target of the link was changed")
	}
}

// A backup that cannot be written stops the install before the file is replaced, and the staged
// temporary file does not stay behind.
func TestInstallLeavesTheFileAloneWhenTheBackupNameIsADirectory(t *testing.T) {
	dir, path := edgeWorld(t)
	if err := os.WriteFile(path, []byte(`{"model":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path+".bak", 0o755); err != nil {
		t.Fatal(err)
	}

	err := settingsfile.Installer{}.Install(path, binary)

	if err == nil || !strings.HasPrefix(err.Error(), "settings: backup to ") {
		t.Fatalf("Install() = %v, want a backup error", err)
	}
	if got := readText(t, path); got != `{"model":"x"}` {
		t.Errorf("settings.json was changed although its backup failed: %q", got)
	}
	if names := namesIn(t, dir); names != "settings.json settings.json.bak" {
		t.Errorf("the directory holds %q, want no temporary file", names)
	}
}

// A string that is not UTF-8 is read as the replacement character and written back as it, the
// way encoding/json treats it; the backup keeps the original bytes.
func TestInstallWritesTheReplacementCharacterForInvalidUTF8(t *testing.T) {
	_, path := edgeWorld(t)
	if err := os.WriteFile(path, []byte("{\"note\":\"\xff\xfe\"}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := (settingsfile.Installer{}).Install(path, binary); err != nil {
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
	installer := settingsfile.Installer{}

	for name, act := range map[string]func(string, string) error{"Install": installer.Install, "Uninstall": installer.Uninstall} {
		err := act(path, binary)
		if err == nil || !strings.HasPrefix(err.Error(), "settings: read "+path+": ") {
			t.Errorf("%s() = %v, want a read error naming the path", name, err)
		}
	}
	if _, err := os.Lstat(path + ".bak"); !os.IsNotExist(err) {
		t.Errorf("a backup exists: %v", err)
	}
}

// A directory that does not exist is a failure to stage, in the words install has always used.
func TestInstallIntoAMissingDirectoryFailsToCreateTheTemporaryFile(t *testing.T) {
	dir, _ := edgeWorld(t)
	path := filepath.Join(dir, "no-such-dir", "settings.json")

	err := settingsfile.Installer{}.Install(path, binary)

	if err == nil || !strings.HasPrefix(err.Error(), "settings: create temp: ") {
		t.Fatalf("Install() = %v, want a create temp error", err)
	}
}

// A settings.json with no write permission for its directory is left as it was.
func TestInstallIntoAnUnwritableDirectoryChangesNothing(t *testing.T) {
	dir, path := edgeWorld(t)
	if os.Geteuid() == 0 {
		t.Skip("root writes into any directory")
	}
	if err := os.WriteFile(path, []byte(`{"model":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err := settingsfile.Installer{}.Install(path, binary)

	if err == nil || !strings.HasPrefix(err.Error(), "settings: create temp: ") {
		t.Fatalf("Install() = %v, want a create temp error", err)
	}
	if got := readText(t, path); got != `{"model":"x"}` {
		t.Errorf("settings.json was changed: %q", got)
	}
}
