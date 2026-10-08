package app

// `skills install` and `skills adopt` as use cases, over a real overlay and a real project in
// temporary directories (the file system adapters) and a registry held in memory. What each
// refusal is called, and what is left in the project, is what these tests hold; the words a person
// reads are the adapter's, and the ownership rules themselves are tested in engine/skills.

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/skillsfs"
)

const installRegistryPath = "reg.yaml"

// projectEntry is an entry of a registry that admits the skill to the projects given.
func projectEntry(id string, allowed ...string) skills.Entry {
	e := entry(id, "custom", "claude")
	e.Install.DefaultScope = "project"
	e.Install.AllowedProjects = allowed
	return e
}

// namedIdentity is the ProjectIdentity of a test: the id the person gave, and otherwise the name of
// the directory, as the composition root chains them.
type namedIdentity struct{}

func (namedIdentity) Identify(q skills.ProjectQuery) (skills.ProjectID, bool, error) {
	if q.Explicit != "" {
		return q.Explicit, true, nil
	}
	return skills.ProjectID(filepath.Base(q.Dir)), true, nil
}

// fakeIdentity answers what the test says and records what it was asked.
type fakeIdentity struct {
	id    skills.ProjectID
	ok    bool
	err   error
	asked []skills.ProjectQuery
}

func (f *fakeIdentity) Identify(q skills.ProjectQuery) (skills.ProjectID, bool, error) {
	f.asked = append(f.asked, q)
	return f.id, f.ok, f.err
}

// installWorld is an overlay with a registry in memory and the sources of its skills on disk, and an
// empty project.
type installWorld struct {
	t             *testing.T
	overlay, root string
	registry      skills.Registry
	identity      skills.ProjectIdentity
	locks         skills.ProjectLockStore
	tree          skills.SkillTree
	files         skills.FileReader
	project       skills.ProjectFS
}

// newInstallWorld is a project "target-repo" and an overlay whose registry admits my-skill to it,
// with the source files given.
func newInstallWorld(t *testing.T, files map[string]string) *installWorld {
	t.Helper()
	w := &installWorld{
		t: t, overlay: t.TempDir(), root: filepath.Join(t.TempDir(), "target-repo"),
		registry: registryOf(projectEntry("my-skill", "target-repo")),
		identity: namedIdentity{}, locks: skillsfs.ProjectLocks{}, tree: skillsfs.Tree{}, files: os.ReadFile, project: skillsfs.Project{},
	}
	if err := os.MkdirAll(w.root, 0o755); err != nil {
		t.Fatal(err)
	}
	w.setSource("my-skill", files)
	return w
}

func (w *installWorld) setSource(id string, files map[string]string) {
	w.t.Helper()
	if err := os.RemoveAll(filepath.Join(w.overlay, id)); err != nil {
		w.t.Fatal(err)
	}
	for name, content := range files {
		put(w.t, filepath.Join(w.overlay, id, filepath.FromSlash(name)), content)
	}
}

func (w *installWorld) ports() InstallPorts {
	return InstallPorts{
		Registries: registries{installRegistryPath: w.registry}, Identity: w.identity, Tree: w.tree,
		Locks: w.locks, Files: w.files, Project: w.project,
	}
}

func (w *installWorld) input(extra ...func(*InstallInput)) InstallInput {
	in := InstallInput{RegistryPath: installRegistryPath, SourceRoot: w.overlay, ProjectID: "target-repo", ProjectRoot: w.root}
	for _, f := range extra {
		f(&in)
	}
	return in
}

func (w *installWorld) install(extra ...func(*InstallInput)) (InstallResult, error) {
	return InstallProject(w.ports(), w.input(extra...))
}

func (w *installWorld) adopt(extra ...func(*InstallInput)) (InstallResult, error) {
	return AdoptProject(w.ports(), w.input(extra...))
}

func (w *installWorld) snapshot() map[string]string { return snapshot(w.t, w.root) }

// snapshot lists every file and directory under root with its content, to compare before and after.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			out[filepath.ToSlash(rel)+"/"] = ""
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func sameTree(t *testing.T, before, after map[string]string) {
	t.Helper()
	var changed []string
	for path, content := range before {
		if got, ok := after[path]; !ok || got != content {
			changed = append(changed, path)
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			changed = append(changed, path)
		}
	}
	sort.Strings(changed)
	if len(changed) > 0 {
		t.Errorf("the project changed: %v", changed)
	}
}

func outcomes(res InstallResult) string {
	var parts []string
	for _, o := range res.Skills {
		parts = append(parts, string(o.Status)+": "+o.ID)
	}
	return strings.Join(parts, ",")
}

func projectFile(w *installWorld, rel string) string {
	w.t.Helper()
	return read(w.t, filepath.Join(w.root, filepath.FromSlash(rel)))
}

// ---- install -----------------------------------------------------------------------------

func TestInstallCopiesTheSkillIntoBothRuntimesAndRecordsIt(t *testing.T) {
	w := newInstallWorld(t, map[string]string{"SKILL.md": "# My Skill"})

	res, err := w.install()

	if err != nil || outcomes(res) != "installed: my-skill" || res.ProjectID != "target-repo" || res.NoneAdmitted {
		t.Fatalf("InstallProject = %+v, %v, want my-skill installed for target-repo", res, err)
	}
	for _, runtime := range []string{".claude", ".agents"} {
		if got := projectFile(w, runtime+"/skills/my-skill/SKILL.md"); got != "# My Skill" {
			t.Errorf("%s copy = %q", runtime, got)
		}
	}
	if !strings.Contains(projectFile(w, skills.ProjectLockRelPath), `"my-skill"`) {
		t.Error("the project lock does not record my-skill")
	}
}

func TestAnInstallThatChangesNothingSaysUnchanged(t *testing.T) {
	w := newInstallWorld(t, map[string]string{"SKILL.md": "# Canonical Content"})
	if _, err := w.install(); err != nil {
		t.Fatal(err)
	}
	before := w.snapshot()

	res, err := w.install()

	if err != nil || outcomes(res) != "unchanged: my-skill" {
		t.Errorf("second install = %+v, %v, want unchanged", res, err)
	}
	sameTree(t, before, w.snapshot())
}

func TestASourceThatMovedOnIsUpdated(t *testing.T) {
	w := newInstallWorld(t, map[string]string{"SKILL.md": "v1"})
	if _, err := w.install(); err != nil {
		t.Fatal(err)
	}
	w.setSource("my-skill", map[string]string{"SKILL.md": "v2"})

	res, err := w.install()

	if err != nil || outcomes(res) != "updated: my-skill" || projectFile(w, ".agents/skills/my-skill/SKILL.md") != "v2" {
		t.Errorf("install = %+v, %v, want my-skill updated to v2", res, err)
	}
}

// What a person put in the project is theirs: the run stops, says which file, and writes nothing.
func TestAnInstallOverAHandEditedFileIsRefusedWithEveryReasonAndNothingIsWritten(t *testing.T) {
	w := newInstallWorld(t, map[string]string{"SKILL.md": "# Canonical Content"})
	if _, err := w.install(); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(w.root, ".claude/skills/my-skill/SKILL.md"), "# Stale")
	w.setSource("my-skill", map[string]string{"SKILL.md": "# A newer canonical content"})
	before := w.snapshot()

	_, err := w.install()

	var refusal *PlanRefusal
	if !errors.As(err, &refusal) || refusal.Verb != "install" || len(refusal.Reasons) == 0 {
		t.Fatalf("err = %v, want a *PlanRefusal of install", err)
	}
	for _, want := range []string{".claude/skills/my-skill/SKILL.md", "edited"} {
		if !strings.Contains(strings.Join(refusal.Reasons, "\n"), want) {
			t.Errorf("the reasons %q do not say %q", refusal.Reasons, want)
		}
	}
	sameTree(t, before, w.snapshot())
}

func TestADirectoryThatWasNotInstalledIsRefusedWithAWayForward(t *testing.T) {
	w := newInstallWorld(t, map[string]string{"SKILL.md": "# My Skill"})
	put(t, filepath.Join(w.root, ".claude/skills/my-skill/SKILL.md"), "someone else's\n")
	before := w.snapshot()

	_, err := w.install()

	var refusal *PlanRefusal
	if !errors.As(err, &refusal) {
		t.Fatalf("err = %v, want a *PlanRefusal", err)
	}
	for _, want := range []string{".claude/skills/my-skill", "skills adopt", "--project-id target-repo"} {
		if !strings.Contains(strings.Join(refusal.Reasons, "\n"), want) {
			t.Errorf("the reasons %q do not say %q", refusal.Reasons, want)
		}
	}
	sameTree(t, before, w.snapshot())
}

// A lock that is there and is not JSON is a refusal of the plan, and the lock is not replaced.
func TestAProjectLockThatIsNotJSONStopsTheInstallAndIsNotReplaced(t *testing.T) {
	w := newInstallWorld(t, map[string]string{"SKILL.md": "# My Skill"})
	put(t, filepath.Join(w.root, filepath.FromSlash(skills.ProjectLockRelPath)), "{ not json")
	before := w.snapshot()

	_, err := w.install()

	var refusal *PlanRefusal
	if !errors.As(err, &refusal) || !strings.Contains(strings.Join(refusal.Reasons, "\n"), skills.ProjectLockRelPath) {
		t.Errorf("err = %v, want a refusal that names the lock", err)
	}
	sameTree(t, before, w.snapshot())
}

// A lock that cannot be read at all is not "no lock yet": an empty one written over it would disown
// everything it records.
func TestAProjectLockThatCannotBeReadStopsTheInstallInsteadOfStartingAnEmptyOne(t *testing.T) {
	w := newInstallWorld(t, map[string]string{"SKILL.md": "# My Skill"})
	if err := os.MkdirAll(skills.ProjectLockPath(w.root), 0o755); err != nil {
		t.Fatal(err)
	}
	before := w.snapshot()

	_, err := w.install()

	var unreadable *ProjectLockReadError
	if !errors.As(err, &unreadable) || !strings.HasPrefix(err.Error(), `reading project lock ".labdrian/procedural-skills.lock.json": `) {
		t.Errorf("err = %v, want a *ProjectLockReadError", err)
	}
	sameTree(t, before, w.snapshot())
}

// The lock is read through the store the use case was given, and not through the file reader: a
// reader that refuses every file does not stop it, and a store that says "no lock" starts one.
func TestTheLockIsReadThroughTheStoreAndNotTheFileReader(t *testing.T) {
	w := newInstallWorld(t, map[string]string{"SKILL.md": "# My Skill"})
	asked := ""
	w.locks = lockStoreFunc(func(root string) ([]byte, error) { asked = root; return nil, fs.ErrNotExist })
	w.files = func(name string) ([]byte, error) {
		if strings.HasSuffix(filepath.ToSlash(name), skills.ProjectLockRelPath) {
			t.Errorf("the lock was read as a file: %s", name)
		}
		return os.ReadFile(name)
	}

	if _, err := w.install(); err != nil {
		t.Fatal(err)
	}
	if asked != w.root {
		t.Errorf("the store was asked for %q, want the project root %q", asked, w.root)
	}
}

type lockStoreFunc func(root string) ([]byte, error)

func (f lockStoreFunc) ReadLock(root string) ([]byte, error) { return f(root) }

func TestEverySourceDirectoryThatIsMissingIsNamedNotTheFirst(t *testing.T) {
	w := newInstallWorld(t, map[string]string{"SKILL.md": "x"})
	w.registry = registryOf(projectEntry("ghost-a", "target-repo"), projectEntry("ghost-b", "target-repo"))

	_, err := w.install()

	var missing *SourcesMissingError
	if !errors.As(err, &missing) || len(missing.Missing) != 2 || missing.Missing[0].SkillID != "ghost-a" ||
		missing.Missing[1].Src != filepath.Join(w.overlay, "ghost-b") || err.Error() != "2 source director(ies) missing" {
		t.Fatalf("err = %v (%+v), want both sources missing", err, missing)
	}
	if entries, _ := os.ReadDir(w.root); len(entries) != 0 {
		t.Errorf("a failed install wrote %d entries into the project", len(entries))
	}
}

type failingTree struct{ skills.SkillTree }

func (failingTree) ReadSkillSource(string) ([]skills.SourceFile, error) {
	return nil, errors.New("disk on fire")
}

func TestASourceThatCannotBeReadIsNamedWithItsCause(t *testing.T) {
	w := newInstallWorld(t, map[string]string{"SKILL.md": "x"})
	w.tree = failingTree{w.tree}

	_, err := w.install()

	var unreadable *SourceReadError
	if !errors.As(err, &unreadable) || unreadable.ID != "my-skill" || unreadable.Dir != filepath.Join(w.overlay, "my-skill") ||
		err.Error() != "skill my-skill: reading its source "+unreadable.Dir+": disk on fire" {
		t.Errorf("err = %v, want the source of my-skill named with the cause", err)
	}
}

// A write that fails part way leaves the project as it was, and the use case says what the executor
// reported on the way.
func TestAWriteThatFailsLeavesTheProjectAsItWasAndTheErrorIsAnExecutionError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: directory permissions are not enforced")
	}
	w := newInstallWorld(t, map[string]string{"SKILL.md": "content"})
	skillsDir := filepath.Join(w.root, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(skillsDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(skillsDir, 0o755) })
	before := w.snapshot()

	res, err := w.install()

	var failed *ExecutionError
	if !errors.As(err, &failed) || failed.Err == nil || len(res.Skills) != 0 {
		t.Fatalf("InstallProject = %+v, %v, want an *ExecutionError and no outcome", res, err)
	}
	sameTree(t, before, w.snapshot())
}

// The executor names what it could not put back in the error it returns; the use case does not
// print it, it hands it over typed, in the order the rollback found it.
func TestThePathsARollbackCouldNotRestoreTravelWithTheError(t *testing.T) {
	w := newInstallWorld(t, map[string]string{"SKILL.md": "content"})
	w.project = &failingProject{ProjectFS: skillsfs.Project{}, failRename: true, failRemoveRollback: true}

	_, err := w.install()

	var failed *ExecutionError
	if !errors.As(err, &failed) || !errors.Is(err, skills.ErrRollbackIncomplete) {
		t.Fatalf("err = %v, want an *ExecutionError that carries ErrRollbackIncomplete", err)
	}
	if len(failed.Unrestored) == 0 {
		t.Errorf("Unrestored is empty, want the repo-relative paths the rollback could not restore")
	}
	for _, rel := range failed.Unrestored {
		if filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") {
			t.Errorf("Unrestored holds %q, want repo-relative paths", rel)
		}
	}
}

// failingProject is a ProjectFS whose renames fail, and whose removals fail once they began to.
type failingProject struct {
	skills.ProjectFS
	failRename, failRemoveRollback bool
	renames                        int
}

func (f *failingProject) Rename(oldPath, newPath string) error {
	f.renames++
	if f.failRename && f.renames > 1 {
		return errors.New("rename refused")
	}
	return f.ProjectFS.Rename(oldPath, newPath)
}

func (f *failingProject) Remove(name string) error {
	if f.failRemoveRollback && f.renames > 1 {
		return errors.New("remove refused")
	}
	return f.ProjectFS.Remove(name)
}

// install writes the skill into the two runtime directories and records it in the project lock,
// and into nothing else.
func TestInstallWritesOnlyTheRuntimeDirectoriesAndTheLock(t *testing.T) {
	w := newInstallWorld(t, map[string]string{"SKILL.md": "# My Skill", "references/guide.md": "g"})
	if _, err := w.install(); err != nil {
		t.Fatal(err)
	}
	for path := range w.snapshot() {
		if strings.HasSuffix(path, "/") {
			continue
		}
		if !strings.HasPrefix(path, ".claude/skills/my-skill/") && !strings.HasPrefix(path, ".agents/skills/my-skill/") && path != skills.ProjectLockRelPath {
			t.Errorf("install wrote %s, which is neither a runtime copy of the skill nor the lock", path)
		}
	}
}

// ---- which skills ------------------------------------------------------------------------

func TestOnlyTheSkillsAdmittedToTheProjectAreInstalled(t *testing.T) {
	w := newInstallWorld(t, nil)
	global := entry("global-skill", "custom", "claude")
	w.registry = registryOf(projectEntry("skill-a", "other-repo"), projectEntry("skill-b", "target-repo"), global)
	w.setSource("skill-b", map[string]string{"SKILL.md": "B"})

	res, err := w.install()

	if err != nil || outcomes(res) != "installed: skill-b" {
		t.Fatalf("InstallProject = %+v, %v, want only skill-b installed", res, err)
	}
	for _, id := range []string{"skill-a", "global-skill"} {
		if _, err := os.Stat(filepath.Join(w.root, ".claude", "skills", id)); err == nil {
			t.Errorf("%s was installed", id)
		}
	}
}

func TestARegistryThatAdmitsNothingToTheProjectIsSaidAndWritesNothing(t *testing.T) {
	w := newInstallWorld(t, nil)
	w.registry = registryOf(projectEntry("some-skill", "other-repo"))
	before := w.snapshot()

	res, err := w.install()

	if err != nil || !res.NoneAdmitted || res.ProjectID != "target-repo" || len(res.Skills) != 0 {
		t.Errorf("InstallProject = %+v, %v, want nothing admitted for target-repo", res, err)
	}
	sameTree(t, before, w.snapshot())
}

// With no source root there is nowhere to read the admitted skills from, and that is what is
// said: a missing input, not a registry path that escapes a root. A registry that admits nothing
// needs no source and is told so, as it was.
func TestAnInstallWithNoSourceRootSaysTheSourceRootIsMissingWhenASkillIsAdmitted(t *testing.T) {
	for _, verb := range []string{"install", "adopt"} {
		t.Run(verb, func(t *testing.T) {
			w := newInstallWorld(t, map[string]string{"SKILL.md": "x"})
			before := w.snapshot()
			noRoot := func(in *InstallInput) { in.SourceRoot = "" }
			run := map[string]func(...func(*InstallInput)) (InstallResult, error){"install": w.install, "adopt": w.adopt}[verb]

			res, err := run(noRoot)

			var missing *SourceRootRequiredError
			if !errors.As(err, &missing) || missing.Verb != verb {
				t.Fatalf("%s with no source root = %v, want a *SourceRootRequiredError for %s", verb, err, verb)
			}
			var planning *PlanError
			if errors.As(err, &planning) {
				t.Errorf("%v is a *PlanError: no registry path was planned", err)
			}
			if res.ProjectID != "target-repo" {
				t.Errorf("the result names project %q, want the one that was resolved", res.ProjectID)
			}
			sameTree(t, before, w.snapshot())
		})
	}

	w := newInstallWorld(t, nil)
	w.registry = registryOf(projectEntry("some-skill", "other-repo"))
	if res, err := w.install(func(in *InstallInput) { in.SourceRoot = "" }); err != nil || !res.NoneAdmitted {
		t.Errorf("InstallProject with no source root and nothing admitted = %+v, %v, want nothing admitted", res, err)
	}
}

func TestWhatTheReaderLeftOutOfTheRegistryIsHandedBackEvenWhenTheVerbRefuses(t *testing.T) {
	w := newInstallWorld(t, map[string]string{"SKILL.md": "x"})
	w.registry.Unread = []string{`line 3: unknown key "color" in skill entry`}
	put(t, filepath.Join(w.root, ".claude/skills/my-skill/SKILL.md"), "someone else's\n")

	res, err := w.install()

	var refusal *PlanRefusal
	if !errors.As(err, &refusal) || res.UnreadWarning != w.registry.UnreadWarning() || res.UnreadWarning == "" {
		t.Errorf("InstallProject = %+v, %v, want the refusal and the warning", res, err)
	}
}

func TestARegistryThatCannotBeUsedIsARegistryErrorAndTheStoreIsAskedOnce(t *testing.T) {
	w := newInstallWorld(t, nil)
	in := w.input(func(in *InstallInput) { in.RegistryPath = "absent.yaml" })

	_, err := InstallProject(w.ports(), in)

	var refusal *RegistryError
	if !errors.As(err, &refusal) || !refusal.Unreadable() || refusal.Path != "absent.yaml" {
		t.Errorf("err = %v, want an unreadable *RegistryError naming absent.yaml", err)
	}
}

// ---- who the project is ------------------------------------------------------------------

// The use case asks the port once, with the directory it works in and the id it was given, and
// plans for the id the port answers: it derives nothing itself.
func TestInstallAsksTheIdentityPortForTheProjectAndPlansForItsAnswer(t *testing.T) {
	w := newInstallWorld(t, map[string]string{"SKILL.md": "S"})
	w.registry = registryOf(projectEntry("my-skill", "named-by-the-port"))
	port := &fakeIdentity{id: "named-by-the-port", ok: true}
	w.identity = port

	res, err := w.install(func(in *InstallInput) { in.ProjectID = "" })

	if err != nil || outcomes(res) != "installed: my-skill" || res.ProjectID != "named-by-the-port" {
		t.Fatalf("InstallProject = %+v, %v, want the skill admitted to the id the port named", res, err)
	}
	if want := (skills.ProjectQuery{Dir: w.root}); len(port.asked) != 1 || port.asked[0] != want {
		t.Errorf("the port was asked %+v, want once, %+v", port.asked, want)
	}
}

func TestTheExplicitProjectIdGoesToThePortAndTheAnswerIsTheirs(t *testing.T) {
	w := newInstallWorld(t, map[string]string{"SKILL.md": "S"})
	port := &fakeIdentity{id: "named-by-the-port", ok: true}
	w.identity = port

	res, _ := w.install(func(in *InstallInput) { in.ProjectID = "given" })

	if want := (skills.ProjectQuery{Dir: w.root, Explicit: "given"}); len(port.asked) != 1 || port.asked[0] != want {
		t.Errorf("the port was asked %+v, want once, %+v", port.asked, want)
	}
	if !res.NoneAdmitted || res.ProjectID != "named-by-the-port" {
		t.Errorf("InstallProject = %+v, want the plan made for the id the port answered, not the one given", res)
	}
}

func TestAdoptAsksTheSameIdentityPortTheSameWay(t *testing.T) {
	w := newInstallWorld(t, map[string]string{"SKILL.md": "S"})
	port := &fakeIdentity{id: "target-repo", ok: true}
	w.identity = port
	for _, runtime := range []string{".claude", ".agents"} {
		put(t, filepath.Join(w.root, runtime, "skills", "my-skill", "SKILL.md"), "S")
	}

	res, err := w.adopt(func(in *InstallInput) { in.ProjectID = "given" })

	if err != nil || outcomes(res) != "adopted: my-skill" {
		t.Fatalf("AdoptProject = %+v, %v", res, err)
	}
	if want := (skills.ProjectQuery{Dir: w.root, Explicit: "given"}); len(port.asked) != 1 || port.asked[0] != want {
		t.Errorf("the port was asked %+v, want once, %+v", port.asked, want)
	}
}

func TestAProjectThatNoSourceCanNameIsRefusedWithTheWayForward(t *testing.T) {
	w := newInstallWorld(t, map[string]string{"SKILL.md": "S"})
	w.identity = &fakeIdentity{ok: false}
	before := w.snapshot()

	_, err := w.install(func(in *InstallInput) { in.ProjectID = "" })

	var identity *IdentityError
	want := "skills install: no source of project identity could name the project in " + w.root + "; give --project-id"
	if !errors.As(err, &identity) || identity.Failure != IdentityNoAnswer || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
	sameTree(t, before, w.snapshot())
}

func TestAnIdentityThatCannotBeToldIsRefusedWithTheReason(t *testing.T) {
	w := newInstallWorld(t, map[string]string{"SKILL.md": "S"})
	cause := errors.New("cannot look for a git repository in /x (permission denied)")
	w.identity = &fakeIdentity{err: cause}

	_, err := w.adopt(func(in *InstallInput) { in.ProjectID = "" })

	var identity *IdentityError
	if !errors.As(err, &identity) || identity.Failure != IdentityUnknown || !errors.Is(err, cause) ||
		err.Error() != "skills adopt: resolving project identity: "+cause.Error() {
		t.Errorf("err = %v, want the reason of the port", err)
	}
}

func TestWithoutAnIdentityPortNothingIsInstalled(t *testing.T) {
	w := newInstallWorld(t, map[string]string{"SKILL.md": "S"})
	w.identity = nil

	_, err := w.install()

	var identity *IdentityError
	if !errors.As(err, &identity) || identity.Failure != IdentityNotWired || err.Error() != "skills install: no project identity is wired" {
		t.Errorf("err = %v, want the identity not wired", err)
	}
	if entries, _ := os.ReadDir(w.root); len(entries) != 0 {
		t.Errorf("%d entries were written", len(entries))
	}
}

// ---- the approval record ---------------------------------------------------------------

// The approval record sits inside the skill directory it approves, so install leaves it behind: it
// is repository governance state, not skill content, and a runtime that loaded it would treat it as
// part of the skill.
func TestInstallDoesNotProjectTheApprovalRecord(t *testing.T) {
	w := newInstallWorld(t, map[string]string{
		"SKILL.md":                                "body\n",
		"references/notes.md":                     "notes\n",
		skills.ApprovalRecordName:                 `{"version": 1}`,
		"references/" + skills.ApprovalRecordName: "nested, so content",
	})
	if _, err := w.install(); err != nil {
		t.Fatal(err)
	}
	for _, runtime := range []string{".claude", ".agents"} {
		dir := filepath.Join(w.root, runtime, "skills", "my-skill")
		if _, err := os.Stat(filepath.Join(dir, skills.ApprovalRecordName)); err == nil {
			t.Errorf("%s: the approval record at the root of the skill was installed", runtime)
		}
		for _, want := range []string{"SKILL.md", filepath.Join("references", "notes.md"), filepath.Join("references", skills.ApprovalRecordName)} {
			if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
				t.Errorf("%s: %s was not installed: %v", runtime, want, err)
			}
		}
	}
}

func TestADirectoryInThePlaceOfTheRecordIsNotProjectedEither(t *testing.T) {
	w := newInstallWorld(t, map[string]string{"SKILL.md": "body\n"})
	if err := os.MkdirAll(filepath.Join(w.overlay, "my-skill", skills.ApprovalRecordName), 0o755); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(w.overlay, "my-skill", skills.ApprovalRecordName, "inner.txt"), "x")

	if _, err := w.install(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(w.root, ".claude", "skills", "my-skill", skills.ApprovalRecordName)); err == nil {
		t.Error("a directory named like the record was installed")
	}
}

// ---- adopt -------------------------------------------------------------------------------

// handCopy puts the source of my-skill in the runtimes given with no record of it, as an install
// from before records existed, or a copy by hand, would.
func (w *installWorld) handCopy(content string, runtimes ...string) {
	w.t.Helper()
	for _, runtime := range runtimes {
		put(w.t, filepath.Join(w.root, runtime, "skills", "my-skill", "SKILL.md"), content)
	}
}

func TestAdoptTakesOwnershipOfAnExistingInstallAndThenInstallLeavesItBe(t *testing.T) {
	w := newInstallWorld(t, map[string]string{"SKILL.md": "# my-skill\n"})
	w.handCopy("# my-skill\n", ".claude", ".agents")

	_, err := w.install()
	var refusal *PlanRefusal
	if !errors.As(err, &refusal) || !strings.Contains(strings.Join(refusal.Reasons, "\n"), "skills adopt") {
		t.Fatalf("install before adopt: %v, want a refusal that points at adopt", err)
	}

	adopted, err := w.adopt()
	if err != nil || outcomes(adopted) != "adopted: my-skill" {
		t.Fatalf("AdoptProject = %+v, %v", adopted, err)
	}
	if again, err := w.install(); err != nil || outcomes(again) != "unchanged: my-skill" {
		t.Errorf("install after adopt = %+v, %v, want unchanged", again, err)
	}
	if second, err := w.adopt(); err != nil || outcomes(second) != "unchanged: my-skill" {
		t.Errorf("a second adopt = %+v, %v, want unchanged", second, err)
	}
}

// A skill that is in one runtime only is adopted there, and the note says what is left for install.
func TestAdoptNotesTheRuntimeThatDoesNotHaveTheSkill(t *testing.T) {
	w := newInstallWorld(t, map[string]string{"SKILL.md": "# my-skill\n"})
	w.handCopy("# my-skill\n", ".claude")

	res, err := w.adopt()

	want := []string{".agents/skills/my-skill is not installed; run `labdrian skills install --project-id target-repo` to add it"}
	if err != nil || outcomes(res) != "adopted: my-skill" || strings.Join(res.Notes, "|") != want[0] {
		t.Errorf("AdoptProject = %+v, %v, want my-skill adopted with the note %q", res, err, want)
	}
}

func TestAdoptRefusesADifferingDirectoryNamingTheFileAndWritesNothing(t *testing.T) {
	w := newInstallWorld(t, map[string]string{"SKILL.md": "# my-skill\n"})
	w.handCopy("my own version\n", ".claude", ".agents")
	before := w.snapshot()

	_, err := w.adopt()

	var refusal *PlanRefusal
	if !errors.As(err, &refusal) || refusal.Verb != "adopt" {
		t.Fatalf("err = %v, want a *PlanRefusal of adopt", err)
	}
	for _, want := range []string{".claude/skills/my-skill", "SKILL.md has other bytes"} {
		if !strings.Contains(strings.Join(refusal.Reasons, "\n"), want) {
			t.Errorf("the reasons %q do not say %q", refusal.Reasons, want)
		}
	}
	sameTree(t, before, w.snapshot())
}

func TestAdoptWritesTheLockAndNoSkillFile(t *testing.T) {
	w := newInstallWorld(t, map[string]string{"SKILL.md": "# my-skill\n"})
	w.handCopy("# my-skill\n", ".claude", ".agents")
	before := w.snapshot()

	if _, err := w.adopt(); err != nil {
		t.Fatal(err)
	}
	after := w.snapshot()
	for path, content := range before {
		if after[path] != content {
			t.Errorf("adopt changed %s", path)
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok && path != skills.ProjectLockRelPath && !strings.HasPrefix(path, ".labdrian") {
			t.Errorf("adopt wrote %s, which is not the lock", path)
		}
	}
}

// ---- through the ports ---------------------------------------------------------------------

// sourceTree is a SkillTree that answers what the test says and records what it was asked.
type sourceTree struct {
	skills.SkillTree
	files []skills.SourceFile
	asked []string
}

func (r *sourceTree) ReadSkillSource(dir string) ([]skills.SourceFile, error) {
	r.asked = append(r.asked, dir)
	return r.files, nil
}

// install copies the files the SkillTree says a skill has, whatever the directory holds.
func TestInstallCopiesTheSourceTheTreeReads(t *testing.T) {
	w := newInstallWorld(t, map[string]string{"SKILL.md": "on disk, which the tree does not read"})
	tree := &sourceTree{files: []skills.SourceFile{{Rel: "SKILL.md", Data: []byte("what the tree read"), Mode: 0o644}}}
	w.tree = tree

	if _, err := w.install(); err != nil {
		t.Fatal(err)
	}
	if got := projectFile(w, ".claude/skills/my-skill/SKILL.md"); got != "what the tree read" {
		t.Errorf("installed SKILL.md = %q, want the bytes the tree read", got)
	}
	if want := filepath.Join(w.overlay, "my-skill"); len(tree.asked) != 1 || tree.asked[0] != want {
		t.Errorf("the tree was asked %v, want [%q]", tree.asked, want)
	}
}

// stagingProject is a ProjectFS that counts its calls and fails to stage a file in one directory.
type stagingProject struct {
	skills.ProjectFS
	failDir string
	n       map[string]int
}

func (s *stagingProject) WriteTemp(dir string, data []byte, perm fs.FileMode) (string, error) {
	s.n["writetemp"]++
	if dir == s.failDir {
		return "", errors.New("create temp: no room")
	}
	return s.ProjectFS.WriteTemp(dir, data, perm)
}

func (s *stagingProject) Rename(oldPath, newPath string) error {
	s.n["rename"]++
	return s.ProjectFS.Rename(oldPath, newPath)
}

func (s *stagingProject) MkdirAll(dir string, perm fs.FileMode) error {
	s.n["mkdirall"]++
	return s.ProjectFS.MkdirAll(dir, perm)
}

// install writes through the ProjectFS it is given and words a failure to stage a file as it always
// has: the verb's words, then 'writeProjectTemp: ', then the step the port says and the cause.
func TestInstallWritesThroughTheProjectFileSystemAndWordsItsFailure(t *testing.T) {
	w := newInstallWorld(t, map[string]string{"SKILL.md": "B"})
	failing := &stagingProject{ProjectFS: w.project, failDir: filepath.Join(w.root, ".claude", "skills", "my-skill"), n: map[string]int{}}
	w.project = failing

	_, err := w.install()

	var failed *ExecutionError
	want := "skills install: staging \".claude/skills/my-skill/SKILL.md\": writeProjectTemp: create temp: no room"
	if !errors.As(err, &failed) || err.Error() != want {
		t.Errorf("install with a file system that fails: %v, want an *ExecutionError saying %q", err, want)
	}
	if _, err := os.Stat(failing.failDir); err == nil {
		t.Error("something was installed although staging failed")
	}

	working := &stagingProject{ProjectFS: skillsfs.Project{}, n: map[string]int{}}
	w.project = working
	if _, err := w.install(); err != nil {
		t.Fatal(err)
	}
	if working.n["writetemp"] == 0 || working.n["rename"] == 0 || working.n["mkdirall"] == 0 {
		t.Errorf("install did not write through the file system it was given: %v", working.n)
	}
}
