package main

// Tests of the adapter of `install` and `adopt` (skills_install_cli.go): which locks each takes and
// when, what a refused command line or a project directory that cannot be named does, which ports
// must be wired, and the words each refusal of a use case is told in. What the use cases decide is
// tested in engine/skills/app, and what the program prints and leaves on disk by the golden files.

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// installRegistryEntries is the registry text of the entries of an overlay: a global skill, which
// install never copies, and project skills, which it copies to the projects they admit.
func installRegistryEntries(id, scope, project string) string {
	entry := "  - id: " + id + "\n    path: " + id + "\n    source:\n      type: custom\n    install:\n      defaultScope: " + scope +
		"\n      targets:\n        - claude\n"
	if project != "" {
		entry += "      allowedProjects:\n        - " + project + "\n"
	}
	return entry + "    lifecycle:\n      updateStrategy: overlay-only\n"
}

func installRegistryYAML(entries ...string) string {
	return "version: \"1\"\nskills:\n" + strings.Join(entries, "")
}

// installCLIWorld is an overlay with a global skill and a project skill admitted to the project p,
// and an empty project.
type installCLIWorld struct {
	t                       *testing.T
	dir, reg, root, project string
	lck                     string
}

func newInstallCLIWorld(t *testing.T) installCLIWorld {
	t.Helper()
	dir := t.TempDir()
	w := installCLIWorld{
		t: t, dir: dir, reg: filepath.Join(dir, "skills.registry.yaml"), root: filepath.Join(dir, "skills"),
		project: filepath.Join(t.TempDir(), "project"),
	}
	w.lck = skills.RegistryLockPath(w.reg)
	writeTestFile(t, w.reg, installRegistryYAML(installRegistryEntries("glob", "global", ""), installRegistryEntries("proj", "project", "p")))
	for _, id := range []string{"glob", "proj"} {
		writeTestFile(t, filepath.Join(w.root, id, "SKILL.md"), overlaySkillMD(id))
	}
	if err := os.MkdirAll(w.project, 0o755); err != nil {
		t.Fatal(err)
	}
	return w
}

// args is the command line the wrapper would give: the registry, the tree and the project.
func (w installCLIWorld) args(verb string, extra ...string) []string {
	return append([]string{verb, "--registry", w.reg, "--source-root", w.root, "--project-id", "p"}, extra...)
}

// deps wires the real adapters over the files of the test, installing into the project of the
// world and locking through locker.
func (w installCLIWorld) deps(locker skills.Locker) skills.Deps {
	return w.depsAt(locker, func() (string, error) { return w.project, nil })
}

func (w installCLIWorld) depsAt(locker skills.Locker, cwd func() (string, error)) skills.Deps {
	deps := newSkillsDeps(testDeps())
	deps.Locker, deps.Cwd = locker, cwd
	return deps
}

func (w installCLIWorld) run(verb func(skills.Deps, []string, io.Writer, io.Writer, func(int)), deps skills.Deps, args ...string) verbRun {
	w.t.Helper()
	return runSkillsVerb(verb, deps, args...)
}

func (w installCLIWorld) tree() map[string]string { return projectTree(w.t, w.project) }

// projectTree lists every file and directory under root with its content.
func projectTree(t *testing.T, root string) map[string]string {
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

// copyByHand puts the project skill in the project the way a person, or an install from before
// records, would: both runtimes unless it is told which, and no record.
func (w installCLIWorld) copyByHand(content string, runtimes ...string) {
	w.t.Helper()
	if len(runtimes) == 0 {
		runtimes = []string{".claude", ".agents"}
	}
	for _, runtime := range runtimes {
		writeTestFile(w.t, filepath.Join(w.project, runtime, "skills", "proj", "SKILL.md"), content)
	}
}

var installVerbs = []struct {
	name string
	run  func(skills.Deps, []string, io.Writer, io.Writer, func(int))
	did  string
}{
	{"install", skillsInstall, "installed"},
	{"adopt", skillsAdopt, "adopted"},
}

// ---- what each verb tells ----------------------------------------------------------------

func TestInstallTellsWhatItDidToEachSkill(t *testing.T) {
	w := newInstallCLIWorld(t)
	deps := w.deps(noopOverlayLocker{})

	r := w.run(skillsInstall, deps, w.args("install")...)
	if r.stdout != "installed: proj\n" || r.stderr != "" || !reflect.DeepEqual(r.exits, []int{0}) {
		t.Errorf("install = %q, %q, %v", r.stdout, r.stderr, r.exits)
	}
	if data, err := os.ReadFile(filepath.Join(w.project, ".agents", "skills", "proj", "SKILL.md")); err != nil || string(data) != overlaySkillMD("proj") {
		t.Errorf("the .agents copy = %q, %v", data, err)
	}
	r = w.run(skillsInstall, deps, w.args("install")...)
	if r.stdout != "unchanged: proj\n" || !reflect.DeepEqual(r.exits, []int{0}) {
		t.Errorf("a second install = %q, %v, want unchanged", r.stdout, r.exits)
	}
	writeTestFile(t, filepath.Join(w.root, "proj", "SKILL.md"), overlaySkillMD("proj")+"\nMore.\n")
	r = w.run(skillsInstall, deps, w.args("install")...)
	if r.stdout != "updated: proj\n" || !reflect.DeepEqual(r.exits, []int{0}) {
		t.Errorf("install of a source that moved on = %q, %v, want updated", r.stdout, r.exits)
	}
}

func TestInstallSaysWhenTheRegistryAdmitsNothingToTheProject(t *testing.T) {
	w := newInstallCLIWorld(t)
	r := w.run(skillsInstall, w.deps(noopOverlayLocker{}), "install", "--registry", w.reg, "--source-root", w.root, "--project-id", "nobody")
	if r.stdout != "no project-scoped skills admitted for project \"nobody\"\n" || r.stderr != "" || !reflect.DeepEqual(r.exits, []int{0}) {
		t.Errorf("install = %q, %q, %v", r.stdout, r.stderr, r.exits)
	}
}

func TestAdoptTellsTheNoteForTheRuntimeThatDoesNotHaveTheSkill(t *testing.T) {
	w := newInstallCLIWorld(t)
	w.copyByHand(overlaySkillMD("proj"), ".claude")

	r := w.run(skillsAdopt, w.deps(noopOverlayLocker{}), w.args("adopt")...)

	want := "adopted: proj\nnote: .agents/skills/proj is not installed; run `labdrian skills install --project-id p` to add it\n"
	if r.stdout != want || r.stderr != "" || !reflect.DeepEqual(r.exits, []int{0}) {
		t.Errorf("exit %v, stdout %q, stderr %q, want stdout %q and nothing on stderr", r.exits, r.stdout, r.stderr, want)
	}
}

func TestAdoptTakesOwnershipOfAnExistingInstallAndThenInstallLeavesItBe(t *testing.T) {
	w := newInstallCLIWorld(t)
	w.copyByHand(overlaySkillMD("proj"))
	deps := w.deps(noopOverlayLocker{})

	refused := w.run(skillsInstall, deps, w.args("install")...)
	if refused.code() != 1 || !strings.Contains(refused.stderr, "skills adopt") {
		t.Fatalf("install before adopt: exit %d, stderr %q, want a refusal that points at adopt", refused.code(), refused.stderr)
	}
	adopted := w.run(skillsAdopt, deps, w.args("adopt")...)
	if adopted.code() != 0 || adopted.stdout != "adopted: proj\n" {
		t.Fatalf("adopt: exit %d, stdout %q, stderr %q", adopted.code(), adopted.stdout, adopted.stderr)
	}
	if again := w.run(skillsInstall, deps, w.args("install")...); again.code() != 0 || again.stdout != "unchanged: proj\n" {
		t.Errorf("install after adopt: exit %d, stdout %q, stderr %q, want unchanged", again.code(), again.stdout, again.stderr)
	}
	if second := w.run(skillsAdopt, deps, w.args("adopt")...); second.code() != 0 || second.stdout != "unchanged: proj\n" {
		t.Errorf("a second adopt: exit %d, stdout %q, stderr %q, want unchanged", second.code(), second.stdout, second.stderr)
	}
}

// ---- the refusals, in the words of each -----------------------------------------------------

func TestInstallAndAdoptTellEveryReasonOfARefusalAndThatNothingWasDone(t *testing.T) {
	for _, v := range installVerbs {
		t.Run(v.name, func(t *testing.T) {
			w := newInstallCLIWorld(t)
			w.copyByHand("my own version\n")
			before := w.tree()

			r := w.run(v.run, w.deps(noopOverlayLocker{}), w.args(v.name)...)

			if r.code() != 1 || r.stdout != "" {
				t.Fatalf("exit %d, stdout %q, want exit 1 and nothing on stdout", r.code(), r.stdout)
			}
			lines := strings.Split(strings.TrimSuffix(r.stderr, "\n"), "\n")
			if len(lines) < 2 || !strings.HasPrefix(lines[0], "error: skills "+v.name+": ") ||
				lines[len(lines)-1] != "error: skills "+v.name+": refused, so nothing was "+v.did {
				t.Errorf("stderr = %q, want each reason as an error line and the verdict last", r.stderr)
			}
			if !reflect.DeepEqual(before, w.tree()) {
				t.Error("a refused verb changed the project")
			}
		})
	}
}

func TestInstallNamesEverySourceThatIsMissingAndHowManyThereAre(t *testing.T) {
	w := newInstallCLIWorld(t)
	writeTestFile(t, w.reg, installRegistryYAML(installRegistryEntries("ghost-a", "project", "p"), installRegistryEntries("ghost-b", "project", "p")))
	r := w.run(skillsInstall, w.deps(noopOverlayLocker{}), w.args("install")...)
	want := "error: skill ghost-a: source dir not found: " + filepath.Join(w.root, "ghost-a") + "\n" +
		"error: skill ghost-b: source dir not found: " + filepath.Join(w.root, "ghost-b") + "\n" +
		"error: 2 source director(ies) missing\n"
	if r.code() != 1 || r.stdout != "" || r.stderr != want {
		t.Errorf("exit %d, stdout %q, stderr %q, want %q", r.code(), r.stdout, r.stderr, want)
	}
}

func TestInstallTellsAProjectLockThatCannotBeReadAndWritesNothing(t *testing.T) {
	w := newInstallCLIWorld(t)
	if err := os.MkdirAll(skills.ProjectLockPath(w.project), 0o755); err != nil {
		t.Fatal(err)
	}
	before := w.tree()
	r := w.run(skillsInstall, w.deps(noopOverlayLocker{}), w.args("install")...)
	if r.code() != 1 || r.stdout != "" || !strings.HasPrefix(r.stderr, `error: reading project lock ".labdrian/procedural-skills.lock.json": `) {
		t.Errorf("exit %d, stdout %q, stderr %q", r.code(), r.stdout, r.stderr)
	}
	if !reflect.DeepEqual(before, w.tree()) {
		t.Error("the project changed")
	}
}

func TestInstallTellsTheRegistryItCannotUseInTheWordsOfEveryVerb(t *testing.T) {
	w := newInstallCLIWorld(t)
	absent := filepath.Join(w.dir, "absent.yaml")
	r := w.run(skillsInstall, w.deps(noopOverlayLocker{}), "install", "--registry", absent, "--source-root", w.root, "--project-id", "p")
	if r.code() != 1 || !strings.HasPrefix(r.stderr, fmt.Sprintf("error: reading registry %q: ", absent)) {
		t.Errorf("exit %d, stderr %q, want the registry that cannot be read named", r.code(), r.stderr)
	}
	writeTestFile(t, w.reg, "version: \"99\"\nskills: []\n")
	r = w.run(skillsInstall, w.deps(noopOverlayLocker{}), w.args("install")...)
	if r.code() != 1 || !strings.HasPrefix(r.stderr, "error: parsing registry: ") {
		t.Errorf("exit %d, stderr %q, want a registry that is not usable told as such", r.code(), r.stderr)
	}
}

func TestInstallTellsWhatTheReaderLeftOutOfTheRegistry(t *testing.T) {
	w := newInstallCLIWorld(t)
	writeTestFile(t, w.reg, installRegistryYAML(installRegistryEntries("proj", "project", "p"))+"    color: red\n")
	r := w.run(skillsInstall, w.deps(noopOverlayLocker{}), w.args("install")...)
	if r.code() != 0 || !strings.Contains(r.stderr, "color") {
		t.Errorf("exit %d, stderr %q, want the unread field told and the install done", r.code(), r.stderr)
	}
}

// What the executor says while it puts back what it could not is told before the error that
// follows it, as the executor always said it.
func TestInstallTellsWhatTheExecutorReportedBeforeTheErrorItFailedWith(t *testing.T) {
	w := newInstallCLIWorld(t)
	deps := w.deps(noopOverlayLocker{})
	deps.Project = &refusingRenames{ProjectFS: deps.Project}
	r := w.run(skillsInstall, deps, w.args("install")...)
	if r.code() != 1 || r.stdout != "" {
		t.Fatalf("exit %d, stdout %q", r.code(), r.stdout)
	}
	report := strings.Index(r.stderr, "error: rollback incomplete: ")
	failure := strings.LastIndex(r.stderr, "error: skills install: rollback incomplete: ")
	if report < 0 || failure < 0 || report > failure {
		t.Errorf("stderr = %q, want the paths the executor could not restore first and the error after them", r.stderr)
	}
}

// refusingRenames is a project file system whose renames fail from the second on, and whose
// removals fail once they did, so that the executor cannot put everything back.
type refusingRenames struct {
	skills.ProjectFS
	renames int
}

func (f *refusingRenames) Rename(oldPath, newPath string) error {
	f.renames++
	if f.renames > 1 {
		return errors.New("rename refused")
	}
	return f.ProjectFS.Rename(oldPath, newPath)
}

func (f *refusingRenames) Remove(name string) error {
	if f.renames > 1 {
		return errors.New("remove refused")
	}
	return f.ProjectFS.Remove(name)
}

// ---- the command line -----------------------------------------------------------------------

// A flag the verb does not read is refused, before a lock is asked for and before anything is read
// or written (decision D4); install and adopt used to ignore it.
func TestInstallAndAdoptRefuseAFlagTheyDoNotReadBeforeTakingALock(t *testing.T) {
	for _, v := range installVerbs {
		for _, flag := range []string{"--frobnicate", "-v", "--project-id=other", "--repo", "--ref", "--candidate"} {
			t.Run(v.name+" "+flag, func(t *testing.T) {
				w := newInstallCLIWorld(t)
				locker := &holdingLocker{}
				before := w.tree()

				r := w.run(v.run, w.deps(locker), w.args(v.name, flag)...)

				want := fmt.Sprintf("error: skills %s: unknown flag %q\n", v.name, flag)
				if r.code() != 1 || r.stdout != "" || r.stderr != want {
					t.Errorf("exit %d, stdout %q, stderr %q, want exit 1 and %q", r.code(), r.stdout, r.stderr, want)
				}
				if got := locker.log(); len(got) != 0 {
					t.Errorf("lock events = %v, want none: a refused command line asks for no lock", got)
				}
				if !reflect.DeepEqual(before, w.tree()) {
					t.Error("the project changed")
				}
			})
		}
	}
}

// What the verbs read: the registry, the source root and the project id, the manifest of the
// wrapper taken and not read, a word that is no flag ignored, a "--" dropped, and the flags after
// it still flags.
func TestInstallReadsItsFlagsAndIgnoresWhatIsNoFlag(t *testing.T) {
	w := newInstallCLIWorld(t)
	for name, args := range map[string][]string{
		"the manifest of the wrapper":       {"--manifest", filepath.Join(w.dir, "overlay.manifest")},
		"a manifest that looks like a flag": {"--manifest", "--registry"},
		"a word that is no flag":            {"extra"},
		"--, a word, and the flags after":   {"--", "extra", "--manifest", "m"},
	} {
		t.Run(name, func(t *testing.T) {
			project := filepath.Join(t.TempDir(), "project")
			if err := os.MkdirAll(project, 0o755); err != nil {
				t.Fatal(err)
			}
			deps := w.depsAt(noopOverlayLocker{}, func() (string, error) { return project, nil })
			r := w.run(skillsInstall, deps, w.args("install", args...)...)
			if r.code() != 0 || r.stdout != "installed: proj\n" || r.stderr != "" {
				t.Errorf("exit %d, stdout %q, stderr %q", r.code(), r.stdout, r.stderr)
			}
		})
	}
}

func TestInstallTakesTheValueOfAFlagWhateverItLooksLike(t *testing.T) {
	w := newInstallCLIWorld(t)
	// --project-id before another flag: that flag is its value, and the registry is the default.
	r := w.run(skillsInstall, w.deps(noopOverlayLocker{}), "install", "--project-id", "--registry", w.reg, "--source-root", w.root)
	if r.code() != 1 || !strings.Contains(r.stderr, `reading registry "skills.registry.yaml"`) {
		t.Errorf("exit %d, stderr %q, want the registry of the working directory read, since --registry was taken as a value", r.code(), r.stderr)
	}
	// --project-id as the last word has no value: the identity port is asked with none.
	asked := &recordedIdentity{}
	deps := w.deps(noopOverlayLocker{})
	deps.Identity = asked
	w.run(skillsInstall, deps, "install", "--registry", w.reg, "--source-root", w.root, "--project-id")
	if len(asked.queries) != 1 || asked.queries[0].Explicit != "" {
		t.Errorf("the identity port was asked %+v, want one question with no explicit id", asked.queries)
	}
}

type recordedIdentity struct{ queries []skills.ProjectQuery }

func (r *recordedIdentity) Identify(q skills.ProjectQuery) (skills.ProjectID, bool, error) {
	r.queries = append(r.queries, q)
	return "p", true, nil
}

// ---- the project directory ------------------------------------------------------------------

// install writes into the working directory. When that directory cannot be resolved, or resolves to
// something that is not absolute, the verb refuses before it takes any lock, and says so. Each way
// of failing gives its own reason.
func TestInstallAndAdoptRefuseBeforeLockingWhenTheyCannotNameTheirProjectDirectory(t *testing.T) {
	notAbsolute := func(path string) string { return fmt.Sprintf("%q is not an absolute path", path) }
	cases := []struct {
		name   string
		cwd    func() (string, error)
		reason string
	}{
		{"the working directory cannot be read", func() (string, error) { return "", fmt.Errorf("getwd: permission denied") }, "getwd: permission denied"},
		{"a relative path", func() (string, error) { return filepath.Join("rel", "dir"), nil }, notAbsolute(filepath.Join("rel", "dir"))},
		{"an empty path", func() (string, error) { return "", nil }, notAbsolute("")},
		{"no working directory wired", nil, "no working directory is wired"},
	}
	for _, v := range installVerbs {
		for _, tc := range cases {
			t.Run(v.name+" "+tc.name, func(t *testing.T) {
				w := newInstallCLIWorld(t)
				locker := &holdingLocker{}
				// No project admits a skill: a run that goes ahead writes nothing.
				r := w.run(v.run, w.depsAt(locker, tc.cwd), v.name, "--registry", w.reg, "--source-root", w.root, "--project-id", "nobody")

				want := fmt.Sprintf("error: skills %s: cannot resolve the project directory it works in (%s); nothing was locked and nothing was %s\n", v.name, tc.reason, v.did)
				if r.code() != 1 || r.stdout != "" || r.stderr != want {
					t.Errorf("exit %d, stdout %q, stderr %q, want exit 1 and %q", r.code(), r.stdout, r.stderr, want)
				}
				if got := locker.log(); len(got) != 0 {
					t.Errorf("lock events = %v, want none: the verb must refuse before it locks", got)
				}
			})
		}
	}
}

// The directory install works in is resolved once, and the verb and the lock are given the same
// answer: a second call can differ.
func TestInstallLocksTheDirectoryItInstallsInto(t *testing.T) {
	w := newInstallCLIWorld(t)
	calls := 0
	second := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(second, 0o755); err != nil {
		t.Fatal(err)
	}
	cwd := func() (string, error) {
		calls++
		if calls == 1 {
			return w.project, nil
		}
		return second, nil
	}
	locker := &holdingLocker{}

	r := w.run(skillsInstall, w.depsAt(locker, cwd), w.args("install")...)

	if r.code() != 0 {
		t.Fatalf("exit %d, stderr %q", r.code(), r.stderr)
	}
	if _, err := os.Stat(filepath.Join(w.project, ".claude", "skills", "proj", "SKILL.md")); err != nil {
		t.Errorf("the skill was not installed into the directory that was locked: %v", err)
	}
	if _, err := os.Stat(filepath.Join(second, ".claude")); err == nil {
		t.Errorf("install wrote into %s, which it did not lock", second)
	}
	if calls != 1 {
		t.Errorf("the working directory was resolved %d times, want once", calls)
	}
	if got := locker.log(); len(got) < 2 || got[1] != "lockdir exclusive "+w.project {
		t.Errorf("lock events = %v, want the project that was written locked", got)
	}
}

// ---- which locks, and in which order ------------------------------------------------------------

// The lock order, pinned so that later work on install inherits it: the overlay lock first, then
// the project lock, and released in the opposite order. Two verbs that took them the other way round
// could each hold one and wait for the other.
func TestInstallAndAdoptTakeTheOverlayLockBeforeTheProjectLockAndReleaseThemInReverse(t *testing.T) {
	for _, v := range installVerbs {
		t.Run(v.name, func(t *testing.T) {
			w := newInstallCLIWorld(t)
			if v.name == "adopt" {
				w.copyByHand(overlaySkillMD("proj"))
			}
			locker := &holdingLocker{}

			if r := w.run(v.run, w.deps(locker), w.args(v.name)...); r.code() != 0 {
				t.Fatalf("exit %d, stderr=%q", r.code(), r.stderr)
			}

			want := []string{"lock shared " + w.lck, "lockdir exclusive " + w.project, "unlock " + w.project, "unlock " + w.lck}
			if got := locker.log(); !reflect.DeepEqual(got, want) {
				t.Errorf("lock events = %v, want %v", got, want)
			}
		})
	}
}

// The order is a property of the policy, not of one verb: whatever a verb asks for, every file
// (overlay) lock comes before every directory (project) lock.
func TestTheLocksOfEveryVerbAreAlwaysOverlayBeforeProject(t *testing.T) {
	root := t.TempDir()
	multi := 0
	for _, verb := range []string{"add", "remove", "sync-manifest", "approve", "validate", "install", "adopt",
		"project-register", "project-revise", "project-retire", "project-status", "list", "status", "lint"} {
		requests := append(skills.OverlayLocks(verb, "r.yaml"), skills.ProjectLocks(verb, root)...)
		seenDir := false
		for _, req := range requests {
			if req.Dir {
				seenDir = true
			} else if seenDir {
				t.Errorf("%s asks for the overlay lock %s after a project lock: %v", verb, req.Path, requests)
			}
		}
		if len(requests) > 1 {
			multi++
		}
	}
	if multi == 0 {
		t.Error("no verb asks for two locks, so this test checks nothing")
	}
}

// When install gets the overlay lock and then cannot get the project's, it lets go of the overlay's,
// says the project is busy, and installs nothing.
func TestInstallReleasesTheOverlayLockWhenTheProjectLockIsBusy(t *testing.T) {
	w := newInstallCLIWorld(t)
	locker := &holdingLocker{failOn: map[string]error{w.project: busyFailure{}}}

	r := w.run(skillsInstall, w.deps(locker), w.args("install")...)

	if r.code() != skills.ExitBusy || r.stdout != "" || !strings.Contains(r.stderr, "the project "+w.project) || !strings.Contains(r.stderr, "retry") {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 2 naming the project and telling to retry", r.code(), r.stdout, r.stderr)
	}
	want := []string{"lock shared " + w.lck, "refused exclusive " + w.project, "unlock " + w.lck}
	if got := locker.log(); !reflect.DeepEqual(got, want) {
		t.Errorf("lock events = %v, want %v: the overlay lock must not be left held", got, want)
	}
	if _, err := os.Stat(filepath.Join(w.project, ".claude")); err == nil {
		t.Error("install wrote into the project without its lock")
	}
}

func TestInstallAndAdoptWithoutALockerFailClosed(t *testing.T) {
	for _, v := range installVerbs {
		t.Run(v.name, func(t *testing.T) {
			w := newInstallCLIWorld(t)
			deps := w.deps(noopOverlayLocker{})
			deps.Locker = nil
			r := w.run(v.run, deps, w.args(v.name)...)
			if want := "error: skills " + v.name + ": no lock is configured, so it will not run unserialized with the other skills commands\n"; r.code() != 1 || r.stdout != "" || r.stderr != want {
				t.Errorf("exit %d, stdout %q, stderr %q, want %q", r.code(), r.stdout, r.stderr, want)
			}
			if _, err := os.Stat(filepath.Join(w.project, ".claude")); err == nil {
				t.Error("the verb wrote without a lock")
			}
		})
	}
}

// A lock that stays taken past the bound is reported with exit 2 and a retry message, and the verb
// does not run: the files are byte for byte as they were. A busy error that a caller wrapped is
// still a busy error.
func TestInstallAndAdoptWithABusyOverlayLockExit2WithARetryMessageAndChangeNothing(t *testing.T) {
	for _, v := range installVerbs {
		for name, busy := range map[string]error{"busy": busyFailure{}, "a wrapped busy error": fmt.Errorf("acquire: %w", busyFailure{})} {
			t.Run(v.name+" "+name, func(t *testing.T) {
				w := newInstallCLIWorld(t)
				w.copyByHand(overlaySkillMD("proj"))
				before := w.tree()
				regBefore, _ := os.ReadFile(w.reg)
				locker := &holdingLocker{failOn: map[string]error{w.lck: busy}}

				r := w.run(v.run, w.deps(locker), w.args(v.name)...)

				if r.code() != skills.ExitBusy || skills.ExitBusy != 2 || r.stdout != "" {
					t.Errorf("exit %d (ExitBusy %d), stdout %q, want 2 and nothing on stdout", r.code(), skills.ExitBusy, r.stdout)
				}
				for _, want := range []string{"skills " + v.name, "in progress", "retry", w.lck} {
					if !strings.Contains(r.stderr, want) {
						t.Errorf("stderr %q does not contain %q", r.stderr, want)
					}
				}
				if got, _ := os.ReadFile(w.reg); string(got) != string(regBefore) || !reflect.DeepEqual(before, w.tree()) {
					t.Error("a busy lock changed files")
				}
			})
		}
	}
}

// A lock that cannot be taken for any other reason is a refusal, exit 1: waiting would not help, so
// the message must not tell the caller to retry.
func TestInstallAndAdoptWithAnOverlayLockThatCannotBeTakenExit1AndDoNotSayRetry(t *testing.T) {
	for _, v := range installVerbs {
		t.Run(v.name, func(t *testing.T) {
			w := newInstallCLIWorld(t)
			locker := &holdingLocker{fail: fmt.Errorf("open %s: permission denied", w.lck)}

			r := w.run(v.run, w.deps(locker), w.args(v.name)...)

			if r.code() != 1 || r.stdout != "" {
				t.Fatalf("exit %d, stdout %q, want exit 1 and nothing on stdout", r.code(), r.stdout)
			}
			for _, want := range []string{"skills " + v.name, "cannot take the lock", "permission denied", w.lck} {
				if !strings.Contains(r.stderr, want) {
					t.Errorf("stderr %q does not contain %q", r.stderr, want)
				}
			}
			if strings.Contains(r.stderr, "retry") {
				t.Errorf("stderr %q tells the caller to retry a failure that will not clear", r.stderr)
			}
		})
	}
}

// The locks are let go of when the verb refuses, whatever the reason.
func TestInstallAndAdoptLetGoOfTheLocksWhenTheyRefuse(t *testing.T) {
	for _, v := range installVerbs {
		t.Run(v.name, func(t *testing.T) {
			w := newInstallCLIWorld(t)
			writeTestFile(t, w.reg, "version: \"99\"\nskills: []\n") // a registry the verb refuses, under the lock
			locker := &holdingLocker{}

			r := w.run(v.run, w.deps(locker), w.args(v.name)...)

			if r.code() != 1 {
				t.Fatalf("exit %d, want 1; stderr=%q", r.code(), r.stderr)
			}
			want := []string{"lock shared " + w.lck, "lockdir exclusive " + w.project, "unlock " + w.project, "unlock " + w.lck}
			if got := locker.log(); !reflect.DeepEqual(got, want) {
				t.Errorf("lock events = %v, want %v", got, want)
			}
		})
	}
}

// State is read after the locks are taken and before they are let go of, never outside them: a read
// made before can be stale by the time the lock is granted. Every port the use case reads through
// is held to it.
func TestInstallAndAdoptReadEverythingUnderTheLocks(t *testing.T) {
	for _, v := range installVerbs {
		t.Run(v.name, func(t *testing.T) {
			w := newInstallCLIWorld(t)
			if v.name == "adopt" {
				w.copyByHand(overlaySkillMD("proj"))
			}
			locker := &holdingLocker{}
			var outside []string
			check := func(what string) {
				if !locker.isHeld(w.lck) || !locker.isHeld(w.project) {
					outside = append(outside, what)
				}
			}
			deps := w.deps(locker)
			deps.Registries = checkedRegistries{deps.Registries, check}
			deps.ProjectLocks = checkedProjectLocks{deps.ProjectLocks, check}
			deps.Tree = checkedTree{deps.Tree, check}
			read := deps.ReadFile
			deps.ReadFile = func(name string) ([]byte, error) { check("file " + name); return read(name) }

			r := w.run(v.run, deps, w.args(v.name)...)

			if r.code() != 0 {
				t.Fatalf("exit %d; stderr=%q", r.code(), r.stderr)
			}
			if len(outside) != 0 {
				t.Errorf("read %v before the locks were taken or after they were released", outside)
			}
		})
	}
}

type checkedRegistries struct {
	skills.RegistryRepository
	check func(string)
}

func (c checkedRegistries) Load(location string) (skills.Registry, error) {
	c.check("registry " + location)
	return c.RegistryRepository.Load(location)
}

type checkedProjectLocks struct {
	skills.ProjectLockStore
	check func(string)
}

func (c checkedProjectLocks) ReadLock(root string) ([]byte, error) {
	c.check("project lock " + root)
	return c.ProjectLockStore.ReadLock(root)
}

type checkedTree struct {
	skills.SkillTree
	check func(string)
}

func (c checkedTree) ReadSkillSource(dir string) ([]skills.SourceFile, error) {
	c.check("source " + dir)
	return c.SkillTree.ReadSkillSource(dir)
}

// A composition root that forgot a port gets a refusal and nothing written, after the locks.
func TestInstallAndAdoptRefuseAPortThatWasNotWired(t *testing.T) {
	for _, tc := range []struct {
		name  string
		unset func(*skills.Deps)
		want  string
	}{
		{"the skill tree", func(d *skills.Deps) { d.Tree = nil }, "no skill tree is wired, so it cannot read the skills of the overlay"},
		{"the project file system", func(d *skills.Deps) { d.Project = nil }, "no project file system is wired, so it cannot read or write files"},
		{"the project lock store", func(d *skills.Deps) { d.ProjectLocks = nil }, "no project lock store is wired, so it cannot read the lock of the project"},
	} {
		for _, v := range installVerbs {
			t.Run(v.name+" without "+tc.name, func(t *testing.T) {
				w := newInstallCLIWorld(t)
				deps := w.deps(&holdingLocker{})
				tc.unset(&deps)
				r := w.run(v.run, deps, w.args(v.name)...)
				if want := "error: skills " + v.name + ": " + tc.want + "\n"; r.code() != 1 || r.stderr != want {
					t.Errorf("exit %d, stderr %q, want %q", r.code(), r.stderr, want)
				}
			})
		}
	}
}

// ---- the identity port ------------------------------------------------------------------------

func TestInstallAndAdoptTellAProjectNoSourceCouldName(t *testing.T) {
	w := newInstallCLIWorld(t)
	deps := w.deps(noopOverlayLocker{})
	deps.Identity = noIdentity{}
	r := w.run(skillsInstall, deps, "install", "--registry", w.reg, "--source-root", w.root)
	want := "error: skills install: no source of project identity could name the project in " + w.project + "; give --project-id\n"
	if r.code() != 1 || r.stderr != want {
		t.Errorf("exit %d, stderr %q, want %q", r.code(), r.stderr, want)
	}
	deps.Identity = nil
	r = w.run(skillsAdopt, deps, "adopt", "--registry", w.reg, "--source-root", w.root)
	if want := "error: skills adopt: no project identity is wired\n"; r.code() != 1 || r.stderr != want {
		t.Errorf("exit %d, stderr %q, want %q", r.code(), r.stderr, want)
	}
}

type noIdentity struct{}

func (noIdentity) Identify(skills.ProjectQuery) (skills.ProjectID, bool, error) {
	return "", false, nil
}

// ---- the interleavings the locks exist to prevent ----------------------------------------------------

// parkedLocks is a project lock store whose first read parks, after the bytes are read, until a
// second reader arrives or the locker says somebody waits for the first one's lock.
type parkedLocks struct {
	skills.ProjectLockStore
	gate *readGate
}

func (g parkedLocks) ReadLock(root string) ([]byte, error) {
	return g.gate.readFile(skills.ProjectLockPath(root))
}

// parkedRegistries is a registry repository whose first load of a path parks, as readGate says.
type parkedRegistries struct {
	skills.RegistryRepository
	gate *readGate
}

func (g parkedRegistries) Load(location string) (skills.Registry, error) {
	if _, err := g.gate.readFile(location); err != nil {
		return skills.Registry{}, &skills.RegistryReadError{Err: err}
	}
	return g.RegistryRepository.Load(location)
}

// Two installs into different projects do not exclude each other (they only read the overlay), so a
// slow one never makes another wait.
func TestTwoInstallsIntoDifferentProjectsShareTheOverlayLock(t *testing.T) {
	w := newInstallCLIWorld(t)
	other := filepath.Join(t.TempDir(), "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	gate := newReadGate(t, w.reg)
	blocked := false
	locker := &exclusionLocker{blocked: func() { blocked = true; gate.release() }}
	deps := w.deps(locker)
	deps.Registries = parkedRegistries{deps.Registries, gate}

	first := make(chan verbRun, 1)
	go func() { first <- w.run(skillsInstall, deps, w.args("install")...) }()
	<-gate.arrived
	// The first install has read the registry and is parked; the second works in another project.
	second := w.run(skillsInstall, w.depsAt(locker, func() (string, error) { return other, nil }), w.args("install")...)
	gate.release()
	<-first

	if second.code() != 0 || blocked {
		t.Errorf("the second install: exit %d, blocked %v (stderr %q), want it to run alongside the first", second.code(), blocked, second.stderr)
	}
	if second.stdout != "installed: proj\n" {
		t.Errorf("the second install printed %q", second.stdout)
	}
}

// Two installs into the same project are serialized by the project lock: the second waits for the
// first.
func TestTwoInstallsIntoOneProjectAreSerialized(t *testing.T) {
	w := newInstallCLIWorld(t)
	gate := newReadGate(t, w.reg)
	locker := &exclusionLocker{blocked: gate.release}
	deps := w.deps(locker)
	deps.Registries = parkedRegistries{deps.Registries, gate}

	first := make(chan verbRun, 1)
	go func() { first <- w.run(skillsInstall, deps, w.args("install")...) }()
	<-gate.arrived
	second := w.run(skillsInstall, w.deps(locker), w.args("install")...)
	gate.release()
	<-first

	if second.code() != 0 {
		t.Fatalf("the second install: exit %d, stderr %q", second.code(), second.stderr)
	}
	events := locker.events()
	released, granted := indexOfEvent(events, "released exclusive project"), lastIndexOfEvent(events, "granted exclusive project")
	if released < 0 || granted < 0 || released > granted {
		t.Errorf("lock events %v: the second install was granted the project before the first released it", events)
	}
}

// Two installs of different skills into one project read the project lock and write it back.
// Without the project lock both read "no lock yet" and the later write drops the earlier record, so
// the first skill's files sit in the project with nothing recording that install owns them.
func TestConcurrentInstallsIntoOneProjectKeepBothRecords(t *testing.T) {
	dir := t.TempDir()
	reg, root := filepath.Join(dir, "skills.registry.yaml"), filepath.Join(dir, "skills")
	writeTestFile(t, reg, installRegistryYAML(installRegistryEntries("one", "project", "p1"), installRegistryEntries("two", "project", "p2")))
	for _, id := range []string{"one", "two"} {
		writeTestFile(t, filepath.Join(root, id, "SKILL.md"), overlaySkillMD(id))
	}
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	gate := newReadGate(t, skills.ProjectLockPath(project))
	locker := &exclusionLocker{blocked: gate.release}
	deps := newSkillsDeps(testDeps())
	deps.Locker, deps.Cwd = locker, func() (string, error) { return project, nil }
	deps.ProjectLocks = parkedLocks{deps.ProjectLocks, gate}
	install := func(projectID string) func() verbRun {
		return func() verbRun {
			return runSkillsVerb(skillsInstall, deps, "install", "--registry", reg, "--source-root", root, "--project-id", projectID)
		}
	}

	results := concurrently(install("p1"), install("p2"))

	for i, r := range results {
		if r.code() != 0 {
			t.Errorf("install #%d: exit %d, stderr %q", i, r.code(), r.stderr)
		}
	}
	data, err := os.ReadFile(skills.ProjectLockPath(project))
	if err != nil {
		t.Fatal(err)
	}
	lock, err := skills.ParseProjectLock(data)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, in := range lock.Installs {
		ids = append(ids, in.ID)
	}
	if strings.Join(ids, ",") != "one,two" {
		t.Errorf("the project lock records %v, want one and two: an install that exited 0 was lost", ids)
	}
}

func lastIndexOfEvent(events []string, want string) int {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i] == want {
			return i
		}
	}
	return -1
}

func indexOfEvent(events []string, want string) int {
	for i, e := range events {
		if e == want {
			return i
		}
	}
	return -1
}

// install and adopt are run by the program through the table of the adapter: the verb is reached,
// takes the locks of the real locker on the registry and the directory, and reads the registry, which
// admits nothing to this project.
func TestInstallAndAdoptAreReachedThroughTheProgram(t *testing.T) {
	w := newInstallCLIWorld(t)
	for _, verb := range []string{"install", "adopt"} {
		t.Run(verb, func(t *testing.T) {
			var out, errOut strings.Builder
			code := -1
			// The working directory of the program is the one of the test, which holds no registry
			// for this project, so the project named admits nothing and nothing is written.
			runSkillsCore(testDeps(), verb, []string{verb, "--registry", w.reg, "--source-root", w.root, "--project-id", "nobody"}, &out, &errOut, func(c int) { code = c })
			if code != 0 || out.String() != "no project-scoped skills admitted for project \"nobody\"\n" || errOut.String() != "" {
				t.Errorf("exit %d, stdout %q, stderr %q", code, out.String(), errOut.String())
			}
		})
	}
}
