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
