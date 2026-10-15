package app

import (
	"errors"
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

func TestRepairLooksForASkillAtItsRootAndItsNameAndNowhereElse(t *testing.T) {
	reg := &fakeRegistry{}
	asked := map[string]bool{}
	skills := recordingSkills{asked: asked}

	_, err := New(reg, skills).Repair(Request{Registry: registryPath, SkillsRoot: "/root/"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range sddregistry.Executors() {
		if want := "/root//" + name + "/SKILL.md"; !asked[want] {
			t.Errorf("the skill %s was not looked for at %q: %v", name, want, asked)
		}
	}
	if len(asked) != 11 {
		t.Errorf("%d paths were looked for, want 11: %v", len(asked), asked)
	}
}

// recordingSkills notes each path it is asked about and has no file.
type recordingSkills struct{ asked map[string]bool }

func (r recordingSkills) IsFile(path string) bool            { r.asked[path] = true; return false }
func (recordingSkills) ReadFile(path string) ([]byte, error) { return nil, errors.New("unreachable") }
