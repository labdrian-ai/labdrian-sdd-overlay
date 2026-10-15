package fsstore

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sddregistry/app"
)

// The adapter answers the ports the use case owns.
var (
	_ app.RegistryStore = Registry{}
	_ app.SkillSource   = Skills{}
)

func TestRegistryReadReturnsTheFileAndTellsAMissingOneApart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skill-registry.md")
	if err := os.WriteFile(path, []byte("# registry\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Registry{}.Read(path)
	if err != nil || string(got) != "# registry\n" {
		t.Errorf("Read = %q, %v, want the file", got, err)
	}

	_, err = Registry{}.Read(filepath.Join(dir, "absent.md"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Read of a file that is not there = %v, want an error for which errors.Is(err, fs.ErrNotExist) holds", err)
	}
	_, err = Registry{}.Read(dir)
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Read of a directory = %v, want a failure that is not an absence", err)
	}
	// A registry under a path whose parent is a file cannot be there, and is not an absence either:
	// the use case would start a registry that could never be written.
	_, err = Registry{}.Read(filepath.Join(path, "skill-registry.md"))
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Read of a path under a file = %v, want a failure that is not an absence", err)
	}
}

func TestRegistryWriteMakesTheDirectoryOfARegistryThatHasNoneAndLeavesMode0644(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proj", ".atl", "skill-registry.md")

	if err := (Registry{}).Write(path, []byte("# new\n")); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil || string(got) != "# new\n" {
		t.Errorf("the registry = %q, %v, want what was written", got, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %o, want 644: a registry is a file of the repository that other people read", info.Mode().Perm())
	}
	dir, err := os.Stat(filepath.Dir(path))
	if err != nil || !dir.IsDir() {
		t.Errorf("the directory of the registry was not made: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("the directory holds %d entries after the write, want only the registry (no temporary file)", len(entries))
	}
}

func TestRegistryWriteReplacesWholeWhatIsThere(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skill-registry.md")
	if err := os.WriteFile(path, []byte("an old and much longer registry than the new one\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := (Registry{}).Write(path, []byte("new\n")); err != nil {
		t.Fatal(err)
	}

	if got, _ := os.ReadFile(path); string(got) != "new\n" {
		t.Errorf("the registry = %q, want exactly what was written", got)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %o, want 644 whatever the mode of the registry it replaced", info.Mode().Perm())
	}
}

func TestRegistryWriteFailsWhenTheDirectoryCannotBeMade(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	err := (Registry{}).Write(filepath.Join(dir, "file", ".atl", "skill-registry.md"), []byte("x"))

	if err == nil {
		t.Error("Write under a file succeeded, want a failure")
	}
}

func TestRegistryHomeMakesTheDirectoryAndNothingElse(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a", "b", "skill-registry.md")

	if err := (Registry{}).Home(path); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(root, "a", "b")); err != nil || !info.IsDir() {
		t.Errorf("the directory was not made: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Home made the registry itself: %v", err)
	}
	if err := (Registry{}).Home(path); err != nil {
		t.Errorf("Home of a directory that is there = %v, want nothing to do", err)
	}
}

func TestSkillsIsFileOnlyForARegularFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(file, []byte("description: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.md")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	dangling := filepath.Join(dir, "dangling.md")
	if err := os.Symlink(filepath.Join(dir, "nowhere"), dangling); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{
		file:                            true,
		link:                            true, // a link to a file is a file, as `test -f` says
		dangling:                        false,
		dir:                             false,
		filepath.Join(dir, "absent.md"): false,
	} {
		if got := (Skills{}).IsFile(path); got != want {
			t.Errorf("IsFile(%s) = %v, want %v", path, got, want)
		}
	}
	if got, err := (Skills{}).ReadFile(file); err != nil || string(got) != "description: x\n" {
		t.Errorf("ReadFile = %q, %v, want the file", got, err)
	}
}
