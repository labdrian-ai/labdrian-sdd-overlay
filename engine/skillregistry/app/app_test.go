package app

import (
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skillregistry"
)

const registryPath = "/p/.atl/skill-registry.md"

// fakeWorld is the three ports in memory: the files of the registry, the skills of the sources and
// the lock, with a log of what was asked of each.
type fakeWorld struct {
	project  bool
	files    map[string]string
	sources  map[string][]skillregistry.Candidate // by Dir
	readErr  error
	writeErr error
	lockErr  error

	events []string
	writes int
	now    time.Time
}

func newWorld() *fakeWorld {
	return &fakeWorld{
		project: true,
		files:   map[string]string{},
		sources: map[string][]skillregistry.Candidate{},
		now:     time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC),
	}
}

func (w *fakeWorld) Read(path string) ([]byte, error) {
	w.events = append(w.events, "read")
	if w.readErr != nil {
		return nil, w.readErr
	}
	text, ok := w.files[path]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
	}
	return []byte(text), nil
}

func (w *fakeWorld) Write(path string, content []byte) error {
	w.events = append(w.events, "write")
	if w.writeErr != nil {
		return w.writeErr
	}
	w.files[path] = string(content)
	w.writes++
	return nil
}

func (w *fakeWorld) IsProject(dir string) bool {
	w.events = append(w.events, "isproject")
	return w.project
}

func (w *fakeWorld) Scan(source skillregistry.Source) ([]skillregistry.Candidate, bool) {
	c, ok := w.sources[source.Dir]
	return c, ok
}

func (w *fakeWorld) Lock(registry string) (func(), error) {
	w.events = append(w.events, "lock")
	if w.lockErr != nil {
		return nil, w.lockErr
	}
	return func() { w.events = append(w.events, "unlock") }, nil
}

func (w *fakeWorld) service() *Service {
	return New(w, w, w, func() time.Time { return w.now })
}

func skill(scope, dir, name, desc, path string) skillregistry.Candidate {
	return skillregistry.Candidate{Scope: scope, DirName: dir, Path: path, Text: "---\nname: " + name + "\ndescription: " + desc + "\n---\n"}
}

func (w *fakeWorld) withSkills() {
	w.sources["/p/skills"] = []skillregistry.Candidate{skill(skillregistry.ScopeProject, "alpha", "alpha", "first", "/p/skills/alpha/SKILL.md")}
	w.sources["/h/.claude/skills"] = []skillregistry.Candidate{skill(skillregistry.ScopeUser, "beta", "beta", "second", "/h/.claude/skills/beta/SKILL.md")}
}

func TestRefreshSkipsADirectoryThatIsNotAProjectAndTouchesNothing(t *testing.T) {
	w := newWorld()
	w.project = false
	w.withSkills()

	got, err := w.service().Refresh(Request{Cwd: "/p", Home: "/h"})

	if err != nil || got.Outcome != Skipped {
		t.Fatalf("Refresh = %+v, %v, want Skipped", got, err)
	}
	if want := []string{"isproject"}; !reflect.DeepEqual(w.events, want) {
		t.Fatalf("events = %v, want only the project check %v (no lock, read or write)", w.events, want)
	}
}

func TestRefreshWritesTheRegistryOfAProjectThatHasNoneUnderTheLock(t *testing.T) {
	w := newWorld()
	w.withSkills()

	got, err := w.service().Refresh(Request{Cwd: "/p", Home: "/h"})

	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if got != (Result{Outcome: Refreshed, Registry: registryPath, Skills: 2}) {
		t.Fatalf("Refresh = %+v", got)
	}
	if want := []string{"isproject", "lock", "read", "write", "unlock"}; !reflect.DeepEqual(w.events, want) {
		t.Fatalf("events = %v, want %v", w.events, want)
	}
	text := w.files[registryPath]
	for _, want := range []string{
		"# Skill Registry — p\n",
		"Last updated: 2026-11-18\n",
		"- skills\n- /h/.claude/skills\n\n## Contract",
		"| `alpha` | first | project | `/p/skills/alpha/SKILL.md` |\n| `beta` | second | user | `/h/.claude/skills/beta/SKILL.md` |\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("registry lacks %q:\n%s", want, text)
		}
	}
}

func TestRefreshListsOnlyTheSourcesThatExist(t *testing.T) {
	w := newWorld()
	w.sources["/p/.claude/skills"] = nil
	w.sources["/h/.codex/skills"] = nil

	if _, err := w.service().Refresh(Request{Cwd: "/p", Home: "/h"}); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if text := w.files[registryPath]; !strings.Contains(text, "## Sources scanned\n\n- .claude/skills\n- /h/.codex/skills\n\n## Contract") {
		t.Fatalf("sources scanned wrong:\n%s", text)
	}
}

func TestRefreshWithoutAHomeScansTheProjectOnly(t *testing.T) {
	w := newWorld()
	w.withSkills()

	got, err := w.service().Refresh(Request{Cwd: "/p"})

	if err != nil || got.Skills != 1 {
		t.Fatalf("Refresh = %+v, %v, want the project's one skill", got, err)
	}
}

func TestRefreshDatesTheRegistryByTheUTCDay(t *testing.T) {
	w := newWorld()
	w.now = time.Date(2026, 11, 19, 1, 0, 0, 0, time.FixedZone("UTC+14", 14*3600))

	if _, err := w.service().Refresh(Request{Cwd: "/p"}); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if text := w.files[registryPath]; !strings.Contains(text, "Last updated: 2026-11-18\n") {
		t.Fatalf("the day is not UTC's:\n%s", text)
	}
}

func TestRefreshWritesNothingWhenOnlyTheDateWouldChange(t *testing.T) {
	w := newWorld()
	w.withSkills()
	if _, err := w.service().Refresh(Request{Cwd: "/p", Home: "/h"}); err != nil {
		t.Fatal(err)
	}
	before := w.files[registryPath]
	w.now = w.now.Add(72 * time.Hour)
	w.events = nil

	got, err := w.service().Refresh(Request{Cwd: "/p", Home: "/h"})

	if err != nil || got.Outcome != UpToDate || got.Skills != 2 {
		t.Fatalf("Refresh = %+v, %v, want UpToDate with 2 skills", got, err)
	}
	if w.writes != 1 || w.files[registryPath] != before {
		t.Fatalf("the registry was written again (writes %d)", w.writes)
	}
	if want := []string{"isproject", "lock", "read", "unlock"}; !reflect.DeepEqual(w.events, want) {
		t.Fatalf("events = %v, want %v", w.events, want)
	}
}

func TestRefreshWithForceWritesEvenWhenOnlyTheDateWouldChange(t *testing.T) {
	w := newWorld()
	w.withSkills()
	if _, err := w.service().Refresh(Request{Cwd: "/p", Home: "/h"}); err != nil {
		t.Fatal(err)
	}
	w.now = w.now.Add(72 * time.Hour)

	got, err := w.service().Refresh(Request{Cwd: "/p", Home: "/h", Force: true})

	if err != nil || got.Outcome != Refreshed {
		t.Fatalf("Refresh = %+v, %v, want Refreshed", got, err)
	}
	if text := w.files[registryPath]; !strings.Contains(text, "Last updated: 2026-11-21\n") || w.writes != 2 {
		t.Fatalf("force did not write the new date (writes %d):\n%s", w.writes, text)
	}
}

func TestRefreshWritesWhenASkillChangedAndKeepsWhatTheTableDoesNotOwn(t *testing.T) {
	w := newWorld()
	w.withSkills()
	if _, err := w.service().Refresh(Request{Cwd: "/p", Home: "/h"}); err != nil {
		t.Fatal(err)
	}
	const executor = "| `mine` | my own row | custom | `/m` |\n"
	w.files[registryPath] = strings.Replace(w.files[registryPath], "\n## Loading protocol", "\n"+executor+"\n## Loading protocol", 1)
	w.sources["/p/skills"] = []skillregistry.Candidate{skill(skillregistry.ScopeProject, "alpha", "alpha", "changed", "/p/skills/alpha/SKILL.md")}
	w.now = w.now.Add(24 * time.Hour)

	got, err := w.service().Refresh(Request{Cwd: "/p", Home: "/h"})

	if err != nil || got.Outcome != Refreshed {
		t.Fatalf("Refresh = %+v, %v, want Refreshed", got, err)
	}
	text := w.files[registryPath]
	if !strings.Contains(text, "| `alpha` | changed | project |") || !strings.Contains(text, "\n"+executor+"\n## Loading protocol") {
		t.Fatalf("the changed row or the foreign row is wrong:\n%s", text)
	}
}

func TestRefreshLetsGoOfTheLockWhateverHappens(t *testing.T) {
	read := errors.New("boom read")
	write := errors.New("boom write")
	for _, tc := range []struct {
		name  string
		setup func(w *fakeWorld)
		want  string
	}{
		{"a read that fails", func(w *fakeWorld) { w.readErr = read }, "cannot read the registry " + registryPath + ": boom read"},
		{"a write that fails", func(w *fakeWorld) { w.writeErr = write }, "cannot write the registry " + registryPath + ": boom write"},
		{"a registry that is not one", func(w *fakeWorld) { w.files[registryPath] = "just words\n" }, "registry not recognized"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld()
			tc.setup(w)
			before := w.files[registryPath]

			_, err := w.service().Refresh(Request{Cwd: "/p"})

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			if w.events[len(w.events)-1] != "unlock" || w.files[registryPath] != before {
				t.Fatalf("events %v, registry changed: %v", w.events, w.files[registryPath] != before)
			}
		})
	}
}

func TestRefreshReadsNothingWhenTheLockCannotBeTaken(t *testing.T) {
	w := newWorld()
	w.lockErr = errors.New("held")

	_, err := w.service().Refresh(Request{Cwd: "/p"})

	var lock *LockError
	if !errors.As(err, &lock) || !strings.Contains(err.Error(), "acquiring registry lock: held") {
		t.Fatalf("err = %v, want *LockError", err)
	}
	if want := []string{"isproject", "lock"}; !reflect.DeepEqual(w.events, want) {
		t.Fatalf("events = %v, want %v", w.events, want)
	}
}

func TestRefreshWrapsTheErrorsOfTheStoreForTheCommandToTell(t *testing.T) {
	w := newWorld()
	w.writeErr = errors.New("disk full")
	_, err := w.service().Refresh(Request{Cwd: "/p"})
	var wrote *RegistryWriteError
	if !errors.As(err, &wrote) || wrote.Path != registryPath {
		t.Fatalf("err = %v, want *RegistryWriteError for %s", err, registryPath)
	}

	w = newWorld()
	w.readErr = errors.New("denied")
	_, err = w.service().Refresh(Request{Cwd: "/p"})
	var read *RegistryReadError
	if !errors.As(err, &read) || read.Path != registryPath {
		t.Fatalf("err = %v, want *RegistryReadError for %s", err, registryPath)
	}
}
