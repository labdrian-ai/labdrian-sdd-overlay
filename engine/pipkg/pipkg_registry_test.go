package pipkg_test

// Tests for how pipkg reads the skills registry: through the port the skills domain owns
// (skills.RegistryRepository), never by parsing a file itself. What a repository says is what
// pipkg builds from, and what the domain refuses is refused here with the words the domain uses.

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// fakeRegistries is a repository whose registry is the one the test gives it, wherever it is asked
// for, and which records what it was asked.
type fakeRegistries struct {
	reg    skills.Registry
	err    error
	loaded []string
}

func (f *fakeRegistries) Load(location string) (skills.Registry, error) {
	f.loaded = append(f.loaded, location)
	return f.reg, f.err
}
func (f *fakeRegistries) Decode([]byte) (skills.Registry, error) { return f.reg, f.err }
func (f *fakeRegistries) Encode(skills.Registry) ([]byte, error) { return nil, errors.New("not used") }

// pipkg builds from the registry the repository returns, not from the file at the path: the path
// is only what the repository is asked for. A registry with the one pi skill, from a fake, builds
// that skill's directory from the overlay's source.
func TestBuildReadsTheRegistryThroughTheRepositoryItIsGiven(t *testing.T) {
	overlayRoot, _ := fixtureOverlay(t)
	fake := &fakeRegistries{reg: skills.Registry{Version: "1", Skills: []skills.Entry{{
		ID: "pi-skill", Path: "pi-skill", Source: skills.Source{Type: skills.SourceCustom},
		Install:   skills.Install{DefaultScope: skills.ScopeGlobal, Targets: []string{"pi"}},
		Lifecycle: skills.Lifecycle{UpdateStrategy: "overlay-only"},
	}}}}
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	if err := pipkg.Build(fake, overlayRoot, "no/such/file.yaml", destDir); err != nil {
		t.Fatalf("Build() = %v, want it to build from what the repository returned", err)
	}
	if len(fake.loaded) != 1 || fake.loaded[0] != "no/such/file.yaml" {
		t.Errorf("the repository was asked for %v, want exactly the registry path Build was given", fake.loaded)
	}
	if _, err := os.Stat(filepath.Join(destDir, "skills", "pi-skill", "SKILL.md")); err != nil {
		t.Errorf("the skill of the registry was not built: %v", err)
	}
}

// Only the domain's rule decides whether a registry may be built from. The registry the fake
// returns has a path that climbs out of the overlay; Build refuses it with the domain's words,
// whichever adapter read it.
func TestBuildRefusesWhatTheDomainRefusesWhateverTheAdapterRead(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	fake := &fakeRegistries{reg: skills.Registry{Version: "1", Skills: []skills.Entry{{
		ID: "x", Path: "../outside", Source: skills.Source{Type: skills.SourceCustom},
		Install:   skills.Install{DefaultScope: skills.ScopeGlobal, Targets: []string{"pi"}},
		Lifecycle: skills.Lifecycle{UpdateStrategy: "overlay-only"},
	}}}}
	err := pipkg.Build(fake, overlayRoot, registryPath, filepath.Join(t.TempDir(), "labdrian-pi"))
	if err == nil || !strings.Contains(err.Error(), `pipkg: parsing registry: skills: entry "x": path "../outside" must not contain a ".." component`) {
		t.Errorf("Build() = %v, want the refusal of the domain's rule, worded as a parse failure of the registry", err)
	}
}

// A store that cannot be read is told as one that could not be opened, with the reader's own words.
func TestBuildTellsAStoreThatCannotBeReadAsOneThatCouldNotBeOpened(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	cause := &fs.PathError{Op: "open", Path: registryPath, Err: fs.ErrNotExist}
	fake := &fakeRegistries{err: &skills.RegistryReadError{Err: cause}}
	err := pipkg.Build(fake, overlayRoot, registryPath, filepath.Join(t.TempDir(), "labdrian-pi"))
	if err == nil || err.Error() != "pipkg: opening registry: "+cause.Error() {
		t.Errorf("Build() = %v, want %q", err, "pipkg: opening registry: "+cause.Error())
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the refusal does not carry the cause: errors.Is(err, fs.ErrNotExist) = false")
	}
}

// Check reads the registry through the same repository as Build and tells a fault in the same
// words: the registry the fake returns is refused by the domain's rule (a path that climbs out of
// the overlay), or its store cannot be read, and the check stops with the sentence Build uses for
// each, over a package that was built from the real registry.
func TestCheckTellsWhatTheRepositoryItIsGivenCannotBeBuiltFromInTheWordsBuildUses(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	built := filepath.Join(t.TempDir(), "built", "labdrian-pi")
	if err := pipkg.Build(fileRegistries, overlayRoot, registryPath, built); err != nil {
		t.Fatal(err)
	}
	cause := &fs.PathError{Op: "open", Path: registryPath, Err: fs.ErrNotExist}
	for name, tc := range map[string]struct {
		fake *fakeRegistries
		want string
	}{
		"a registry the domain refuses": {&fakeRegistries{reg: skills.Registry{Version: "1", Skills: []skills.Entry{{
			ID: "x", Path: "../outside", Source: skills.Source{Type: skills.SourceCustom},
			Install:   skills.Install{DefaultScope: skills.ScopeGlobal, Targets: []string{"pi"}},
			Lifecycle: skills.Lifecycle{UpdateStrategy: "overlay-only"},
		}}}}, `pipkg: parsing registry: skills: entry "x": path "../outside" must not contain a ".." component`},
		"a store that cannot be read": {&fakeRegistries{err: &skills.RegistryReadError{Err: cause}}, "pipkg: opening registry: " + cause.Error()},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := pipkg.Check(tc.fake, overlayRoot, registryPath, built)
			if err == nil || err.Error() != tc.want {
				t.Errorf("Check() = %v, want %q", err, tc.want)
			}
			if len(tc.fake.loaded) == 0 {
				t.Error("Check did not ask the repository it was given for the registry")
			}
		})
	}
}

// A build refuses a registry the reader left fields out of and says what it left out in the
// refusal; the repository that warns would say it once more before the refusal. A check only warns,
// so it keeps the warning. Each is told once.
func TestBuildTellsWhatTheReaderLeftOutOnlyInItsRefusalAndCheckOnlyInItsWarning(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	built := filepath.Join(t.TempDir(), "built", "labdrian-pi")
	if err := pipkg.Build(fileRegistries, overlayRoot, registryPath, built); err != nil {
		t.Fatal(err)
	}
	reg, err := skills.ReadRegistry(fileRegistries, registryPath)
	if err != nil {
		t.Fatal(err)
	}
	reg.Unread = []string{`line 3: unknown key "color" in skill entry`}
	var stderr strings.Builder
	repo := skills.WarnOfUnread(&fakeRegistries{reg: reg}, &stderr)

	err = pipkg.Build(repo, overlayRoot, registryPath, built)
	if err == nil || !strings.Contains(err.Error(), `unknown key "color"`) {
		t.Errorf("Build() = %v, want the refusal to say what was left out", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("Build warned %q before refusing with the same words", stderr.String())
	}

	if _, err := pipkg.Check(repo, overlayRoot, registryPath, built); err != nil {
		t.Fatal(err)
	}
	if want := "warning: registry fields left unread: line 3: unknown key \"color\" in skill entry\n"; stderr.String() != want {
		t.Errorf("Check warned %q, want %q", stderr.String(), want)
	}
}

// With the real adapter, a registry path that is a directory is a store that cannot be read, as it
// is for every skills verb. (Before the port pipkg opened the file and read it as a stream, so a
// directory was a failure to parse, in the words "skills: read error"; no verb ever said that.)
func TestARegistryThatIsADirectoryIsAStoreThatCannotBeRead(t *testing.T) {
	overlayRoot, _ := fixtureOverlay(t)
	dir := t.TempDir()
	err := pipkg.Build(fileRegistries, overlayRoot, dir, filepath.Join(t.TempDir(), "labdrian-pi"))
	if err == nil || !strings.HasPrefix(err.Error(), "pipkg: opening registry: ") || !strings.Contains(err.Error(), "is a directory") {
		t.Errorf("Build() = %v, want pipkg: opening registry: ... is a directory", err)
	}
}

// Decision 4 of the owner (2026-10-05): a package is an artifact others consume, so it is not built
// from a registry the reader did not read whole, in the words of the one rule the domain has for
// it (Registry.CheckBuildable), and nothing is written when it refuses: not the package, and not a
// package that was already there. Checking a package against such a registry only warns (the
// warning is the repository's: skills.WarnOfUnread), as it did.
func TestBuildRefusesARegistryTheReaderLeftFieldsOutOfAndWritesNothing(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	built := filepath.Join(t.TempDir(), "built", "labdrian-pi")
	if err := pipkg.Build(fileRegistries, overlayRoot, registryPath, built); err != nil {
		t.Fatal(err)
	}
	reg, err := skills.ReadRegistry(fileRegistries, registryPath)
	if err != nil {
		t.Fatal(err)
	}
	reg.Unread = []string{`line 3: unknown key "color" in skill entry`}
	partial := &fakeRegistries{reg: reg}

	if _, err := pipkg.Check(partial, overlayRoot, registryPath, built); err != nil {
		t.Errorf("Check() = %v, want a check over a registry read in part to go on as it did", err)
	}

	// A rebuild would clear this file: it is how the test sees whether the package was touched.
	writeFile(t, filepath.Join(built, "marker.txt"), "left by the last build\n")

	const want = `pipkg: skills: the registry has fields this program does not read, and a package built from it would be built from a partial read: line 3: unknown key "color" in skill entry`
	before := listTree(t, built)

	if err := pipkg.Build(partial, overlayRoot, registryPath, built); err == nil || err.Error() != want {
		t.Errorf("Build() over a package that is there = %v, want %q", err, want)
	}
	if after := listTree(t, built); after != before {
		t.Errorf("the package that was there changed when Build refused:\nbefore %s\nafter  %s", before, after)
	}

	fresh := filepath.Join(t.TempDir(), "parent", "labdrian-pi")
	if err := pipkg.Build(partial, overlayRoot, registryPath, fresh); err == nil || err.Error() != want {
		t.Errorf("Build() into a new place = %v, want %q", err, want)
	}
	if _, err := os.Stat(filepath.Dir(fresh)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Build() made %s while refusing: Stat = %v, want it absent", filepath.Dir(fresh), err)
	}
}

// listTree is the names, sizes and contents of the files under dir, in one string.
func listTree(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		b.WriteString(p + "=" + string(data) + "\n")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}
