package app

import (
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sddregistry"
)

// fakeRegistry is a registry store in memory. A path that is not in files is not there.
type fakeRegistry struct {
	files    map[string]string
	readErr  error
	writeErr error
	writes   []string
}

func (f *fakeRegistry) Read(path string) ([]byte, error) {
	if f.readErr != nil {
		return nil, f.readErr
	}
	text, ok := f.files[path]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
	}
	return []byte(text), nil
}

func (f *fakeRegistry) Write(path string, content []byte) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	if f.files == nil {
		f.files = map[string]string{}
	}
	f.files[path] = string(content)
	f.writes = append(f.writes, path)
	return nil
}

// fakeSkills is a skill source in memory. A path that is not in files is not a file; one in dirs is a
// directory, which is not either.
type fakeSkills struct {
	files   map[string]string
	readErr map[string]error
}

func (f fakeSkills) IsFile(path string) bool { _, ok := f.files[path]; return ok }

func (f fakeSkills) ReadFile(path string) ([]byte, error) {
	if err := f.readErr[path]; err != nil {
		return nil, err
	}
	return []byte(f.files[path]), nil
}

const (
	registryPath = "/p/proj/.atl/skill-registry.md"
	skillsRoot   = "/h/.codex/skills"
)

func TestRepairStartsARegistryForAProjectThatHasNoneAndPutsTheInstalledExecutorsInIt(t *testing.T) {
	reg := &fakeRegistry{}
	skills := fakeSkills{files: map[string]string{
		skillsRoot + "/sdd-apply/SKILL.md":  "description: Apply\n",
		skillsRoot + "/sdd-verify/SKILL.md": "# no description\n",
	}}

	got, err := New(reg, skills).Repair(Request{Registry: registryPath, SkillsRoot: skillsRoot})
	if err != nil {
		t.Fatal(err)
	}

	wantRows := []sddregistry.Executor{
		sddregistry.NewExecutor("sdd-apply", skillsRoot+"/sdd-apply/SKILL.md", "description: Apply\n"),
		sddregistry.NewExecutor("sdd-verify", skillsRoot+"/sdd-verify/SKILL.md", "# no description\n"),
	}
	want, err := sddregistry.Repair(sddregistry.NewRegistry("proj"), wantRows)
	if err != nil {
		t.Fatal(err)
	}
	if reg.files[registryPath] != want {
		t.Errorf("the registry written =\n%q\nwant\n%q", reg.files[registryPath], want)
	}
	if !reflect.DeepEqual(reg.writes, []string{registryPath}) {
		t.Errorf("the registry was written %v, want once", reg.writes)
	}
	if got.Rows != 2 || len(got.Missing) != 9 || got.Missing[0] != skillsRoot+"/sdd-archive/SKILL.md" {
		t.Errorf("Result = %+v, want 2 rows and the 9 skills that are not installed, in order", got)
	}
}

func TestRepairWorksOnTheRegistryThatIsThereAndKeepsItsOtherLines(t *testing.T) {
	reg := &fakeRegistry{files: map[string]string{registryPath: "# mine\n## Skills\n| `go` |\n## Loading protocol\nx\n"}}

	_, err := New(reg, fakeSkills{}).Repair(Request{Registry: registryPath, SkillsRoot: skillsRoot})
	if err != nil {
		t.Fatal(err)
	}

	want := "# mine\n" + sddregistry.Note + "\n\n## Skills\n| `go` |\n\n## Loading protocol\nx\n"
	if reg.files[registryPath] != want {
		t.Errorf("the registry written =\n%q\nwant\n%q", reg.files[registryPath], want)
	}
}

func TestRepairCountsTheRowsOfTheRegistryItWrote(t *testing.T) {
	reg := &fakeRegistry{files: map[string]string{registryPath: "## Skills\n| `sdd-` | begins like a row |\n## Loading protocol\n"}}
	skills := fakeSkills{files: map[string]string{skillsRoot + "/sdd-spec/SKILL.md": "description: Spec\n"}}

	got, err := New(reg, skills).Repair(Request{Registry: registryPath, SkillsRoot: skillsRoot})
	if err != nil {
		t.Fatal(err)
	}
	if got.Rows != 2 {
		t.Errorf("Rows = %d, want 2: the row written and the line that begins like one", got.Rows)
	}
}

func TestRepairRefusesARegistryWithoutItsHeadingsAndWritesNothing(t *testing.T) {
	reg := &fakeRegistry{files: map[string]string{registryPath: "# nothing here\n"}}

	_, err := New(reg, fakeSkills{}).Repair(Request{Registry: registryPath, SkillsRoot: skillsRoot})

	var missing *sddregistry.MissingSectionError
	if !errors.As(err, &missing) || missing.Heading != "## Skills" {
		t.Errorf("Repair = %v, want a refusal that names the skills heading", err)
	}
	if len(reg.writes) != 0 || reg.files[registryPath] != "# nothing here\n" {
		t.Errorf("the registry was written (%v) or changed: %q", reg.writes, reg.files[registryPath])
	}
}

func TestRepairTellsAFailureToReadFromARegistryThatIsNotThere(t *testing.T) {
	denied := errors.New("permission denied")
	reg := &fakeRegistry{readErr: denied}

	_, err := New(reg, fakeSkills{}).Repair(Request{Registry: registryPath, SkillsRoot: skillsRoot})

	var read *RegistryReadError
	if !errors.As(err, &read) || read.Path != registryPath || !errors.Is(err, denied) {
		t.Errorf("Repair = %v, want a *RegistryReadError for %s that wraps the failure", err, registryPath)
	}
	if len(reg.writes) != 0 {
		t.Errorf("a registry that could not be read was started anew and written: %v", reg.writes)
	}
	if !strings.Contains(err.Error(), "cannot read the registry "+registryPath+": permission denied") {
		t.Errorf("the error says %q", err)
	}
}

// What the use case takes for "there is no registry" is an error for which errors.Is(err,
// fs.ErrNotExist) holds, however the store wrapped it, and nothing else: any other failure of the
// read is a registry it could not read, said as such, and it is not started anew. (The use case takes
// no lock; that a repair and a propagate of the same registry take turns is held where the lock is
// taken, in engine/cmd/registry_test.go, and is not repeated here.)
func TestRepairTakesAWrappedAbsenceForNoRegistry(t *testing.T) {
	reg := &fakeRegistry{readErr: fmt.Errorf("the store of the project: %w", &fs.PathError{Op: "open", Path: registryPath, Err: fs.ErrNotExist})}

	_, err := New(reg, fakeSkills{}).Repair(Request{Registry: registryPath, SkillsRoot: skillsRoot})

	if err != nil {
		t.Fatalf("Repair = %v, want a registry started for a project whose store says (wrapped) that there is none", err)
	}
	if len(reg.writes) != 1 || !strings.HasPrefix(reg.files[registryPath], "# Skill Registry — proj\n") {
		t.Errorf("writes = %v, registry = %q, want one registry written, started for proj", reg.writes, reg.files[registryPath])
	}
}

func TestRepairDoesNotTakeAnyOtherFailureToReadForNoRegistry(t *testing.T) {
	for _, tc := range []struct{ name, says string }{
		{"a path that is a directory", "is a directory"},
		{"a parent that is a file", "not a directory"},
		{"no permission", "permission denied"},
		{"a name that reads like an absence", "no such file or directory"}, // the words are not the type
	} {
		t.Run(tc.name, func(t *testing.T) {
			cause := &fs.PathError{Op: "read", Path: registryPath, Err: errors.New(tc.says)}
			reg := &fakeRegistry{readErr: cause}

			_, err := New(reg, fakeSkills{}).Repair(Request{Registry: registryPath, SkillsRoot: skillsRoot})

			var read *RegistryReadError
			if !errors.As(err, &read) || read.Path != registryPath || !errors.Is(err, cause) {
				t.Errorf("Repair = %v, want a *RegistryReadError for %s that wraps the failure", err, registryPath)
			}
			if errors.Is(err, fs.ErrNotExist) {
				t.Errorf("Repair = %v, which reads as an absence", err)
			}
			if len(reg.writes) != 0 {
				t.Errorf("a registry that could not be read was started anew and written: %v", reg.writes)
			}
		})
	}
}

func TestRepairStopsAtASkillItCannotReadAndWritesNothing(t *testing.T) {
	denied := errors.New("permission denied")
	path := skillsRoot + "/sdd-apply/SKILL.md"
	reg := &fakeRegistry{}
	skills := fakeSkills{files: map[string]string{path: "x"}, readErr: map[string]error{path: denied}}

	_, err := New(reg, skills).Repair(Request{Registry: registryPath, SkillsRoot: skillsRoot})

	var read *SkillReadError
	if !errors.As(err, &read) || read.Path != path || !errors.Is(err, denied) {
		t.Errorf("Repair = %v, want a *SkillReadError for %s that wraps the failure", err, path)
	}
	if len(reg.writes) != 0 {
		t.Errorf("the registry was written although a skill could not be read: %v", reg.writes)
	}
}

func TestRepairTellsAFailureToWrite(t *testing.T) {
	disk := errors.New("no space left on device")
	reg := &fakeRegistry{writeErr: disk}

	_, err := New(reg, fakeSkills{}).Repair(Request{Registry: registryPath, SkillsRoot: skillsRoot})

	var write *RegistryWriteError
	if !errors.As(err, &write) || write.Path != registryPath || !errors.Is(err, disk) {
		t.Errorf("Repair = %v, want a *RegistryWriteError for %s that wraps the failure", err, registryPath)
	}
}

// The file of a skill is its root, one slash, its name and SKILL.md: a root that ends with a slash
// does not put a second one at the join. The slashes the root has inside are the root's own and stay
// as they are: the script joins by concatenation, so a HOME that ends with a slash gave the rows of
// the registry `$HOME//.codex/skills/...` (registry_pins.golden holds it,
// repair/home-with-a-trailing-slash), and a path cleaned here would write another row for the same
// file. The root the script hands over never ends with a slash; one handed by a person can.
func TestRepairLooksForASkillAtItsRootAndItsNameAndNowhereElse(t *testing.T) {
	for _, tc := range []struct{ name, root, join string }{
		{"a root as the script hands it over", "/h/.codex/skills", "/h/.codex/skills/"},
		{"a root that ends with a slash", "/root/", "/root/"},
		{"a root that ends with two", "/root//", "/root/"},
		{"the root of the file system", "/", "/"},
		{"slashes inside the root are its own", "/h//.codex/skills", "/h//.codex/skills/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			asked := map[string]bool{}

			_, err := New(&fakeRegistry{}, recordingSkills{asked: asked}).Repair(Request{Registry: registryPath, SkillsRoot: tc.root})
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range sddregistry.Executors() {
				if want := tc.join + name + "/SKILL.md"; !asked[want] {
					t.Errorf("the skill %s was not looked for at %q: %v", name, want, asked)
				}
			}
			if len(asked) != 11 {
				t.Errorf("%d paths were looked for, want 11: %v", len(asked), asked)
			}
		})
	}
}

// recordingSkills notes each path it is asked about and has no file.
type recordingSkills struct{ asked map[string]bool }

func (r recordingSkills) IsFile(path string) bool            { r.asked[path] = true; return false }
func (recordingSkills) ReadFile(path string) ([]byte, error) { return nil, errors.New("unreachable") }
