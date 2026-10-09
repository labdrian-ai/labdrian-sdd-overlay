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
	fixedUmask(t)
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

// linkedWorld puts a settings file in dotfiles/, the way a dotfile manager keeps it, and the link
// settings.json to it. It returns the directory of the link, the link and the file it names.
func linkedWorld(t *testing.T, content string) (dir, link, target string) {
	t.Helper()
	dir, link = edgeWorld(t)
	if err := os.Mkdir(filepath.Join(dir, "dotfiles"), 0o755); err != nil {
		t.Fatal(err)
	}
	target = filepath.Join(dir, "dotfiles", "claude-settings.json")
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	return dir, link, target
}

// A settings.json that is a link (a dotfile manager's) is followed: the file it names is read and
// replaced, the link stays a link, and the backup is written next to the file, as it is next to a
// plain one. (Before H27 the link was replaced by a plain file and the file it named was left as
// it was; the owner decided on 2026-10-09 that the link is followed.)
func TestInstallThroughALinkWritesTheFileItNamesAndKeepsTheLink(t *testing.T) {
	dir, link, target := linkedWorld(t, `{"model":"linked"}`)

	if err := (settingsfile.Installer{}).Install(link, binary); err != nil {
		t.Fatalf("Install() = %v", err)
	}

	if lstatMode(t, link)&os.ModeSymlink == 0 {
		t.Fatal("the link was replaced")
	}
	if got, _ := os.Readlink(link); got != target {
		t.Errorf("the link names %q, want it unchanged: %q", got, target)
	}
	if got := readText(t, target); !strings.Contains(got, `"model": "linked"`) || !strings.Contains(got, binary) {
		t.Errorf("the target holds %q, want its own keys and our hooks", got)
	}
	if mode := lstatMode(t, target); !mode.IsRegular() || mode.Perm() != 0o600 {
		t.Errorf("the target has mode %v, want a plain file of 0600", mode)
	}
	if got := readText(t, target+".bak"); got != `{"model":"linked"}` {
		t.Errorf("the backup next to the target holds %q, want what the target held", got)
	}
	if names := namesIn(t, dir); names != "dotfiles settings.json" {
		t.Errorf("the directory of the link holds %q, want only the link: no backup, no temporary file", names)
	}
	if names := namesIn(t, filepath.Join(dir, "dotfiles")); names != "claude-settings.json claude-settings.json.bak" {
		t.Errorf("the directory of the target holds %q, want the file and its backup", names)
	}
	if found, owned, err := (settingsfile.Installer{}).Inspect(link, binary); err != nil || !found || !owned {
		t.Errorf("Inspect() through the link = %v, %v, %v, want found and owned", found, owned, err)
	}
}

// The whole chain is followed, and a link written with a relative name works the same.
func TestInstallFollowsAChainOfLinksIncludingRelativeOnes(t *testing.T) {
	dir, link, target := linkedWorld(t, `{"model":"linked"}`)
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	middle := filepath.Join(dir, "middle.json")
	if err := os.Symlink(filepath.Join("dotfiles", "claude-settings.json"), middle); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	if err := os.Symlink("middle.json", link); err != nil {
		t.Fatal(err)
	}

	if err := (settingsfile.Installer{}).Install(link, binary); err != nil {
		t.Fatalf("Install() = %v", err)
	}

	for _, l := range []string{link, middle} {
		if lstatMode(t, l)&os.ModeSymlink == 0 {
			t.Errorf("%s was replaced", l)
		}
	}
	if got := readText(t, target); !strings.Contains(got, binary) {
		t.Errorf("the end of the chain holds %q, want our hooks", got)
	}
}

// The new file is staged beside the file it replaces, so a link to another file system works: a
// rename from the directory of the link would fail across the two.
func TestInstallThroughALinkToAnotherFileSystemStagesBesideTheTarget(t *testing.T) {
	_, link := edgeWorld(t)
	other, err := os.MkdirTemp("/dev/shm", "settingsfile-test-*")
	if err != nil {
		t.Skipf("no second file system to test with: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(other) })
	var here, there syscall.Stat_t
	if err := syscall.Stat(filepath.Dir(link), &here); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Stat(other, &there); err != nil {
		t.Fatal(err)
	}
	if here.Dev == there.Dev {
		t.Skip("/dev/shm is on the same file system as the temporary directory")
	}
	target := filepath.Join(other, "settings.json")
	if err := os.WriteFile(target, []byte(`{"model":"elsewhere"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}

	if err := (settingsfile.Installer{}).Install(link, binary); err != nil {
		t.Fatalf("Install() = %v", err)
	}
	if got := readText(t, target); !strings.Contains(got, binary) {
		t.Errorf("the target holds %q, want our hooks", got)
	}
}

// A link to nothing and a loop of links are refused, naming the path, and nothing is created or
// changed: there is no file to read, so there is nothing to merge into and nowhere to write.
func TestALinkToNothingAndALoopAreRefusedAndNothingChanges(t *testing.T) {
	cases := map[string]func(dir, path string) error{
		"dangling": func(dir, path string) error { return os.Symlink(filepath.Join(dir, "gone.json"), path) },
		"loop": func(dir, path string) error {
			other := filepath.Join(dir, "other.json")
			if err := os.Symlink(path, other); err != nil {
				return err
			}
			return os.Symlink(other, path)
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			dir, path := edgeWorld(t)
			if err := setup(dir, path); err != nil {
				t.Skipf("no symlinks here: %v", err)
			}
			before := namesIn(t, dir)
			installer := settingsfile.Installer{}

			for verb, act := range map[string]func(string, string) error{"Install": installer.Install, "Uninstall": installer.Uninstall} {
				err := act(path, binary)
				if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "nothing was changed") {
					t.Errorf("%s() = %v, want an error naming %s and saying nothing was changed", verb, err, path)
				}
			}
			if lstatMode(t, path)&os.ModeSymlink == 0 {
				t.Error("the link was replaced")
			}
			if after := namesIn(t, dir); after != before {
				t.Errorf("the directory held %q and holds %q", before, after)
			}
		})
	}
}

// Uninstall through a link that holds nothing of ours writes nothing: no backup, no new file.
func TestUninstallThroughALinkThatHoldsNothingOfOursChangesNothing(t *testing.T) {
	dir, link, target := linkedWorld(t, `{"model":"linked"}`)

	if err := (settingsfile.Installer{}).Uninstall(link, binary); err != nil {
		t.Fatalf("Uninstall() = %v", err)
	}
	if got := readText(t, target); got != `{"model":"linked"}` {
		t.Errorf("the target was changed: %q", got)
	}
	if names := namesIn(t, filepath.Join(dir, "dotfiles")); names != "claude-settings.json" {
		t.Errorf("the directory of the target holds %q", names)
	}
}

// Uninstall through a link that does hold our hooks rewrites the file it names and keeps the link.
func TestUninstallThroughALinkThatHoldsOurHooksRewritesTheTarget(t *testing.T) {
	_, link, target := linkedWorld(t, `{"model":"linked"}`)
	if err := (settingsfile.Installer{}).Install(link, binary); err != nil {
		t.Fatal(err)
	}

	if err := (settingsfile.Installer{}).Uninstall(link, binary); err != nil {
		t.Fatalf("Uninstall() = %v", err)
	}

	if lstatMode(t, link)&os.ModeSymlink == 0 {
		t.Error("the link was replaced")
	}
	if got := readText(t, target); strings.Contains(got, binary) || !strings.Contains(got, `"model": "linked"`) {
		t.Errorf("the target holds %q, want our hooks gone and its own keys kept", got)
	}
}

// OWNER DECISION 2 OF 2026-10-09. The backup is written by atomicfile: whole or not at all, and at
// the mode of the file it keeps. Before, it was a copy written in place at 0644 (less the umask),
// so the backup of a 0600 settings.json was readable by everyone.
func TestBackupHasTheModeOfTheFileItKeeps(t *testing.T) {
	for _, mode := range []os.FileMode{0o600, 0o640, 0o644} {
		t.Run(mode.String(), func(t *testing.T) {
			_, path := edgeWorld(t)
			if err := os.WriteFile(path, []byte(`{"model":"x"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}

			if err := (settingsfile.Installer{}).Install(path, binary); err != nil {
				t.Fatalf("Install() = %v", err)
			}

			if got := readText(t, path+".bak"); got != `{"model":"x"}` {
				t.Errorf("the backup holds %q, want the original", got)
			}
			if got := lstatMode(t, path+".bak").Perm(); got != mode {
				t.Errorf("the backup has mode %v, want the original's %v", got, mode)
			}
			if got := lstatMode(t, path).Perm(); got != 0o600 {
				t.Errorf("the new file has mode %v, want 0600", got)
			}
		})
	}
}

// A backup name that is a link is refused: writing through it would put the old settings wherever
// the link points. Nothing is changed.
func TestABackupNameThatIsALinkIsRefusedAndNothingIsWrittenThroughIt(t *testing.T) {
	dir, path := edgeWorld(t)
	if err := os.WriteFile(path, []byte(`{"model":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	elsewhere := filepath.Join(dir, "elsewhere")
	if err := os.WriteFile(elsewhere, []byte("keep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, path+".bak"); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}

	err := settingsfile.Installer{}.Install(path, binary)

	if !errors.Is(err, atomicfile.ErrSymlink) {
		t.Fatalf("Install() = %v, want atomicfile.ErrSymlink", err)
	}
	if got := readText(t, elsewhere); got != "keep me\n" {
		t.Errorf("the file the backup name links to was written through: %q", got)
	}
	if got := readText(t, path); got != `{"model":"x"}` {
		t.Errorf("settings.json was changed although its backup was refused: %q", got)
	}
	if names := namesIn(t, dir); names != "elsewhere settings.json settings.json.bak" {
		t.Errorf("the directory holds %q, want no temporary file", names)
	}
}

// A read-only backup is replaced, not refused: the old backup is swapped out whole, as the file
// itself is. (Before, the in-place write failed with permission denied.)
func TestAReadOnlyBackupIsReplacedAtTheModeOfTheOriginal(t *testing.T) {
	_, path := edgeWorld(t)
	if err := os.WriteFile(path, []byte(`{"model":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".bak", []byte("old backup\n"), 0o444); err != nil {
		t.Fatal(err)
	}

	if err := (settingsfile.Installer{}).Install(path, binary); err != nil {
		t.Fatalf("Install() = %v", err)
	}

	if got := readText(t, path+".bak"); got != `{"model":"x"}` {
		t.Errorf("the backup holds %q, want the original", got)
	}
	if got := lstatMode(t, path+".bak").Perm(); got != 0o600 {
		t.Errorf("the backup has mode %v, want 0600", got)
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

	if err == nil || !strings.HasPrefix(err.Error(), "settings: replace ") {
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
