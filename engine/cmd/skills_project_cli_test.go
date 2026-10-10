package main

// Tests of the adapter of the verbs that keep the skills the agent registers in a project
// (skills_project_cli.go): which command lines are refused before anything is read or locked, which
// lock each verb takes and when, which ports must be wired, and the words each verb tells. What the
// use cases decide is tested in engine/skills/app, and what the program prints and leaves on disk
// by the golden files.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

const (
	projectTidyID = "tidy-worktree"
	projectCand   = "procedural/candidates/repeated-success/tidy-worktree"
)

// projectCLIWorld is an empty project, a draft outside it, a readable registry that matches nothing,
// and the overlay's own lock file in the project, which no verb of this tier touches.
type projectCLIWorld struct {
	t                     *testing.T
	root, draft, registry string
	decoy, decoyRaw       string
}

func newProjectCLIWorld(t *testing.T) projectCLIWorld {
	t.Helper()
	base := t.TempDir()
	w := projectCLIWorld{
		t: t, root: filepath.Join(base, "project"), draft: filepath.Join(base, "drafts", "SKILL.md"),
		registry: filepath.Join(base, "skills.registry.yaml"),
	}
	writeTestFile(t, w.registry, registryOf("unrelated-skill"))
	writeTestFile(t, w.draft, goldenProjectDraft(projectTidyID))
	w.decoy, w.decoyRaw = filepath.Join(w.root, "skills-lock.json"), "{\"decoy\":true}\n"
	writeTestFile(t, w.decoy, w.decoyRaw)
	return w
}

func (w projectCLIWorld) deps(locker skills.Locker) skills.Deps {
	deps := newSkillsDeps(testDeps())
	deps.Locker = locker
	return deps
}

func (w projectCLIWorld) run(verb func(skills.Deps, []string, io.Writer, io.Writer, func(int)), deps skills.Deps, args ...string) verbRun {
	w.t.Helper()
	return runSkillsVerb(verb, deps, args...)
}

// registerArgs is the command line of project-register for the world, with extra flags before the
// draft.
func (w projectCLIWorld) registerArgs(extra ...string) []string {
	args := []string{"project-register", "--project-root", w.root, "--candidate", projectCand, "--registry", w.registry}
	return append(append(args, extra...), w.draft)
}

func (w projectCLIWorld) reviseArgs(extra ...string) []string {
	args := w.registerArgs(extra...)
	args[0] = "project-revise"
	return args
}

func (w projectCLIWorld) statusArgs(id string) []string {
	args := []string{"project-status", "--project-root", w.root, "--registry", w.registry}
	if id != "" {
		args = append(args, id)
	}
	return args
}

func (w projectCLIWorld) retireArgs(id string, extra ...string) []string {
	args := []string{"project-retire", "--project-root", w.root, "--registry", w.registry}
	return append(append(args, extra...), id)
}

func (w projectCLIWorld) target(runtime string) string {
	return filepath.Join(w.root, "."+runtime, "skills", projectTidyID, "SKILL.md")
}

func (w projectCLIWorld) tree() map[string]string { return projectTree(w.t, w.root) }

// registered registers the draft and then rewrites the draft, as a revision needs.
func (w projectCLIWorld) registered() {
	w.t.Helper()
	if r := w.run(skillsProjectRegister, w.deps(noopOverlayLocker{}), w.registerArgs()...); r.code() != 0 {
		w.t.Fatalf("project-register: exit %d, stderr %q", r.code(), r.stderr)
	}
	writeTestFile(w.t, w.draft, strings.Replace(goldenProjectDraft(projectTidyID), "Use when a worktree must be handed over clean.", "Recheck a worktree before handing it to a reviewer.", 1))
}

func (w projectCLIWorld) lockIDs() []string {
	w.t.Helper()
	data, err := os.ReadFile(skills.ProjectLockPath(w.root))
	if err != nil {
		w.t.Fatalf("read the project lock: %v", err)
	}
	lock, err := skills.ParseProjectLock(data)
	if err != nil {
		w.t.Fatalf("the project lock does not parse: %v", err)
	}
	var ids []string
	for _, e := range lock.Skills {
		ids = append(ids, e.ID)
	}
	sort.Strings(ids)
	return ids
}

// untouchedDeps wires ports that fail the test when anything is asked of them, with a locker that
// records, so that a command line that is refused is shown to be refused before everything.
func untouchedDeps(t *testing.T) (skills.Deps, *holdingLocker) {
	t.Helper()
	locker := &holdingLocker{}
	deps := skills.Deps{
		Locker: locker,
		ReadFile: func(name string) ([]byte, error) {
			t.Errorf("a file was read: %s", name)
			return nil, errors.New("not to be read")
		},
		Registries:   untouchedRegistries{t},
		ProjectLocks: untouchedLocks{t},
		Project:      untouchedProject{},
	}
	return deps, locker
}

type untouchedRegistries struct{ t *testing.T }

func (u untouchedRegistries) Load(l string) (skills.Registry, error) {
	u.t.Errorf("a registry was read: %s", l)
	return skills.Registry{}, errors.New("not to be read")
}
func (untouchedRegistries) Decode([]byte) (skills.Registry, error) { return skills.Registry{}, nil }
func (untouchedRegistries) Encode(skills.Registry) ([]byte, error) { return nil, nil }

type untouchedLocks struct{ t *testing.T }

func (u untouchedLocks) ReadLock(root string) ([]byte, error) {
	u.t.Errorf("a project lock was read: %s", root)
	return nil, errors.New("not to be read")
}

// untouchedProject has no file system behind it: any call of it panics, and none is made.
type untouchedProject struct{ skills.ProjectFS }

var projectVerbs = []struct {
	name string
	run  func(skills.Deps, []string, io.Writer, io.Writer, func(int))
	spec skillsFlagSpec
}{
	{"project-register", skillsProjectRegister, skillsProjectRegisterSpec},
	{"project-revise", skillsProjectRevise, skillsProjectReviseSpec},
	{"project-retire", skillsProjectRetire, skillsProjectRetireSpec},
	{"project-status", skillsProjectStatus, skillsProjectStatusSpec},
}

// ---- what each verb tells --------------------------------------------------------------------

func TestProjectRegisterTellsEachPathThenTheDigestTheRevisionAndTheTrustNote(t *testing.T) {
	w := newProjectCLIWorld(t)

	r := w.run(skillsProjectRegister, w.deps(noopOverlayLocker{}), w.registerArgs()...)

	if r.code() != 0 || r.stderr != "" {
		t.Fatalf("exit %d, stderr %q", r.code(), r.stderr)
	}
	lines := strings.Split(strings.TrimRight(r.stdout, "\n"), "\n")
	wantRels := []string{".claude/skills/" + projectTidyID + "/SKILL.md", ".agents/skills/" + projectTidyID + "/SKILL.md", skills.ProjectLockRelPath}
	if len(lines) != len(wantRels)+3 {
		t.Fatalf("stdout = %q, want one wrote: line per path, then sha256, revision and the note", r.stdout)
	}
	for i, rel := range wantRels {
		if lines[i] != "wrote: "+rel {
			t.Errorf("line %d = %q, want %q", i, lines[i], "wrote: "+rel)
		}
	}
	written, err := os.ReadFile(w.target("claude"))
	if err != nil {
		t.Fatal(err)
	}
	if lines[3] != "sha256: "+skills.HashSkill(written) || lines[4] != "revision: 1" || lines[5] != skills.PiTrustNote {
		t.Errorf("the closing lines are %q, want the digest of the bytes written, revision 1 and the Pi trust note last", lines[3:])
	}
	if got, _ := os.ReadFile(w.decoy); string(got) != w.decoyRaw {
		t.Errorf("the decoy lock file changed to %q", got)
	}
}

// --dry-run says the paths the agent feeds to `git check-ignore`, and nothing else: no write, no
// digest, no revision, and no Pi trust note, which discloses a consequence only a write creates.
func TestProjectRegisterDryRunTellsThePlanAndNothingElse(t *testing.T) {
	w := newProjectCLIWorld(t)

	r := w.run(skillsProjectRegister, w.deps(noopOverlayLocker{}), w.registerArgs("--dry-run")...)

	want := "plan: .claude/skills/" + projectTidyID + "/SKILL.md\nplan: .agents/skills/" + projectTidyID + "/SKILL.md\nplan: " + skills.ProjectLockRelPath + "\n"
	if r.code() != 0 || r.stdout != want || r.stderr != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 0 and %q", r.code(), r.stdout, r.stderr, want)
	}
	if _, err := os.Stat(filepath.Join(w.root, ".claude")); err == nil {
		t.Error("a dry run wrote")
	}
}

// A refusal prints nothing on stdout and never the trust note: the agent feeds stdout to `git add`,
// so a path beside a refusal would be a pathspec for a file that does not exist.
func TestProjectRegisterRefusalPrintsNothingOnStdoutAndNoNote(t *testing.T) {
	w := newProjectCLIWorld(t)
	inside := filepath.Join(w.root, "draft-SKILL.md")
	writeTestFile(t, inside, goldenProjectDraft(projectTidyID))
	args := w.registerArgs()
	args[len(args)-1] = inside

	r := w.run(skillsProjectRegister, w.deps(noopOverlayLocker{}), args...)

	if r.code() != 1 || r.stdout != "" || strings.Contains(r.stderr, skills.PiTrustNote) || !strings.Contains(r.stderr, "outside the project root") {
		t.Errorf("exit %d, stdout %q, stderr %q, want a refusal that explains itself and prints no note", r.code(), r.stdout, r.stderr)
	}
}

func TestProjectRegisterTellsADraftThatCannotBeReadAndARegistryThatCannotBeUsed(t *testing.T) {
	w := newProjectCLIWorld(t)
	args := w.registerArgs()
	args[len(args)-1] = filepath.Join(w.root, "..", "absent", "SKILL.md")
	r := w.run(skillsProjectRegister, w.deps(noopOverlayLocker{}), args...)
	if r.code() != 1 || r.stdout != "" || !strings.HasPrefix(r.stderr, fmt.Sprintf("error: reading draft %q: ", args[len(args)-1])) {
		t.Errorf("exit %d, stderr %q, want the draft that cannot be read named", r.code(), r.stderr)
	}

	missing := filepath.Join(w.root, "..", "no-such-registry.yaml")
	r = w.run(skillsProjectRegister, w.deps(noopOverlayLocker{}), "project-register", "--project-root", w.root, "--candidate", projectCand, "--registry", missing, w.draft)
	if r.code() != 1 || !strings.HasPrefix(r.stderr, fmt.Sprintf("error: reading registry %q: ", missing)) {
		t.Errorf("exit %d, stderr %q, want the registry that cannot be read named", r.code(), r.stderr)
	}
	writeTestFile(t, w.registry, "version: \"99\"\nskills: []\n")
	r = w.run(skillsProjectRegister, w.deps(noopOverlayLocker{}), w.registerArgs()...)
	if want := fmt.Sprintf("error: parsing registry %q: ", w.registry); r.code() != 1 || !strings.HasPrefix(r.stderr, want) {
		t.Errorf("exit %d, stderr %q, want a registry that is not usable told with its path", r.code(), r.stderr)
	}
}

func TestProjectReviseTellsEachPathTheDigestAndTheRevisionAndNoNote(t *testing.T) {
	w := newProjectCLIWorld(t)
	w.registered()

	r := w.run(skillsProjectRevise, w.deps(noopOverlayLocker{}), w.reviseArgs("--dry-run")...)
	if r.code() != 0 || !strings.HasSuffix(r.stdout, "plan: "+skills.ProjectLockRelPath+"\n") || strings.Count(r.stdout, "plan: ") != 3 {
		t.Errorf("dry run: exit %d, stdout %q, stderr %q", r.code(), r.stdout, r.stderr)
	}
	r = w.run(skillsProjectRevise, w.deps(noopOverlayLocker{}), w.reviseArgs()...)
	if r.code() != 0 || strings.Count(r.stdout, "wrote: ") != 3 || !strings.HasSuffix(r.stdout, "revision: 2\n") || strings.Contains(r.stdout, "note:") {
		t.Errorf("revise: exit %d, stdout %q, stderr %q, want three paths, the digest and revision 2, and no note", r.code(), r.stdout, r.stderr)
	}
}

func TestProjectReviseRefusesWhatAPersonOwnsAndPrintsNothingOnStdout(t *testing.T) {
	w := newProjectCLIWorld(t)
	w.registered()
	writeTestFile(t, w.target("claude"), "human edit\n")
	before := w.tree()

	r := w.run(skillsProjectRevise, w.deps(noopOverlayLocker{}), w.reviseArgs()...)

	if r.code() != 1 || r.stdout != "" || !strings.Contains(r.stderr, "hash-mismatch") || !strings.Contains(r.stderr, "human-owned") {
		t.Errorf("exit %d, stdout %q, stderr %q", r.code(), r.stdout, r.stderr)
	}
	if !reflect.DeepEqual(before, w.tree()) {
		t.Error("a refused revision changed the project")
	}
}

func TestProjectRetireTellsEachFileItRemoved(t *testing.T) {
	w := newProjectCLIWorld(t)
	w.registered()

	r := w.run(skillsProjectRetire, w.deps(noopOverlayLocker{}), w.retireArgs(projectTidyID, "--reason", "human-request")...)

	want := "removed: .claude/skills/" + projectTidyID + "/SKILL.md\nremoved: .agents/skills/" + projectTidyID + "/SKILL.md\n"
	if r.code() != 0 || r.stdout != want || r.stderr != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want %q", r.code(), r.stdout, r.stderr, want)
	}
	if got := w.lockIDs(); len(got) != 0 {
		t.Errorf("the lock still lists %v", got)
	}
}

// A retirement that could not be put back tells what the executor could not restore, before the
// error, exits 1, and tells no path as removed.
func TestProjectRetireThatCannotBePutBackExits1AndTellsNoRemovedPath(t *testing.T) {
	w := newProjectCLIWorld(t)
	w.registered()
	deps := w.deps(noopOverlayLocker{})
	first := w.target("claude")
	deps.Project = &brokenProject{ProjectFS: deps.Project, remove: first, rename: first}

	r := w.run(skillsProjectRetire, deps, w.retireArgs(projectTidyID)...)

	report := strings.Index(r.stderr, "error: rollback incomplete: ")
	failure := strings.Index(r.stderr, "error: project-register: rollback incomplete")
	if r.code() != 1 || r.stdout != "" || report < 0 || failure < 0 || report > failure {
		t.Errorf("exit %d, stdout %q, stderr %q, want the report of what was not restored, then the error", r.code(), r.stdout, r.stderr)
	}
}

type brokenProject struct {
	skills.ProjectFS
	remove, rename string
}

func (b *brokenProject) Remove(name string) error {
	if name == b.remove {
		return errors.New("injected")
	}
	return b.ProjectFS.Remove(name)
}

func (b *brokenProject) Rename(oldPath, newPath string) error {
	if newPath == b.rename {
		return errors.New("injected")
	}
	return b.ProjectFS.Rename(oldPath, newPath)
}

func TestProjectStatusTellsOwnerRevisionAndSupersession(t *testing.T) {
	w := newProjectCLIWorld(t)
	w.registered()

	r := w.run(skillsProjectStatus, w.deps(noopOverlayLocker{}), w.statusArgs(projectTidyID)...)
	if want := projectTidyID + " rev:1 owner:agent superseded-by:-\n"; r.code() != 0 || r.stdout != want || r.stderr != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want %q", r.code(), r.stdout, r.stderr, want)
	}

	writeTestFile(t, w.target("claude"), "human edit\n")
	writeTestFile(t, w.registry, registryOf(projectTidyID))
	r = w.run(skillsProjectStatus, w.deps(noopOverlayLocker{}), w.statusArgs("")...)
	if want := projectTidyID + " rev:1 owner:human (hash-mismatch"; r.code() != 0 || !strings.HasPrefix(r.stdout, want) || !strings.HasSuffix(r.stdout, ") superseded-by:"+projectTidyID+"\n") {
		t.Errorf("exit %d, stdout %q, stderr %q, want the reason of the human and the skill that supersedes", r.code(), r.stdout, r.stderr)
	}

	r = w.run(skillsProjectStatus, w.deps(noopOverlayLocker{}), w.statusArgs("other")...)
	if want := "error: project-status: skill \"other\" is not in the project lock\n"; r.code() != 1 || r.stdout != "" || r.stderr != want {
		t.Errorf("exit %d, stdout %q, stderr %q, want %q", r.code(), r.stdout, r.stderr, want)
	}
}

// ---- a command line is refused before anything is read or locked -------------------------------

func TestProjectVerbsRefuseWhatTheyCannotWorkWithoutBeforeReadingOrLocking(t *testing.T) {
	const root = "/abs/project"
	cases := []struct {
		verb string
		name string
		args []string
		want string
	}{
		{"project-register", "no project root", []string{"--candidate", projectCand, "/tmp/d.md"}, "error: skills project-register requires --project-root <abs> (there is no working-directory fallback)"},
		{"project-register", "a relative project root", []string{"--project-root", "relative/project", "--candidate", projectCand, "/tmp/d.md"}, `error: skills project-register: --project-root "relative/project" must be an absolute path`},
		{"project-register", "no candidate", []string{"--project-root", root, "/tmp/d.md"}, "error: skills project-register requires --candidate <key>"},
		{"project-register", "no draft", []string{"--project-root", root, "--candidate", projectCand}, "error: skills project-register requires a <draft-file> argument"},
		{"project-register", "an empty draft", []string{"--project-root", root, "--candidate", projectCand, ""}, "error: skills project-register requires a <draft-file> argument"},
		{"project-register", "a second draft", []string{"--project-root", root, "--candidate", projectCand, "/tmp/a.md", "/tmp/b.md"}, `error: skills project-register: unexpected extra argument "/tmp/b.md" (project-register accepts exactly one <draft-file>)`},
		{"project-register", "a misspelled --dry-run after the draft", []string{"--project-root", root, "--candidate", projectCand, "/tmp/a.md", "--dryrun"}, `error: skills project-register: unknown flag "--dryrun"`},
		{"project-register", "a misspelled --dry-run before it", []string{"--dryrun", "--project-root", root, "--candidate", projectCand, "/tmp/a.md"}, `error: skills project-register: unknown flag "--dryrun"`},
		{"project-register", "a flag of retire", []string{"--project-root", root, "--candidate", projectCand, "--reason", "x", "/tmp/a.md"}, `error: skills project-register: unknown flag "--reason"`},
		{"project-register", "the equals form", []string{"--project-root", root, "--candidate", projectCand, "--dry-run=true", "/tmp/a.md"}, `error: skills project-register: unknown flag "--dry-run=true"`},
		{"project-register", "a candidate that is a flag", []string{"--project-root", root, "--candidate", "--dry-run", "/tmp/a.md"}, `error: skills project-register: flag "--candidate" requires a value; got flag token "--dry-run"`},
		{"project-register", "a manifest with no value cannot swallow --dry-run", []string{"--project-root", root, "--candidate", projectCand, "--registry", "r.yaml", "--manifest", "--dry-run", "/tmp/a.md"}, `error: skills project-register: flag "--manifest" requires a value; got flag token "--dry-run"`},
		{"project-register", "a registry that is the last word", []string{"--project-root", root, "--candidate", projectCand, "/tmp/a.md", "--registry"}, `error: skills project-register: flag "--registry" requires a value`},
		{"project-register", "a source root that is the last word", []string{"--project-root", root, "--candidate", projectCand, "/tmp/a.md", "--source-root"}, `error: skills project-register: flag "--source-root" requires a value`},
		{"project-revise", "no project root", []string{"--candidate", projectCand, "/tmp/d.md"}, "error: skills project-revise requires --project-root <abs> (there is no working-directory fallback)"},
		{"project-revise", "no candidate", []string{"--project-root", root, "/tmp/d.md"}, "error: skills project-revise requires --candidate <key>"},
		{"project-revise", "no draft", []string{"--project-root", root, "--candidate", projectCand}, "error: skills project-revise requires a <draft-file> argument"},
		{"project-revise", "a flag of retire", []string{"--project-root", root, "--candidate", projectCand, "--reason", "x", "/tmp/d.md"}, `error: skills project-revise: unknown flag "--reason"`},
		{"project-revise", "a second draft", []string{"--project-root", root, "--candidate", projectCand, "/tmp/a.md", "/tmp/b.md"}, `error: skills project-revise: unexpected extra argument "/tmp/b.md" (project-revise accepts exactly one <draft-file>)`},
		{"project-retire", "no project root", []string{"x"}, "error: skills project-retire requires --project-root <abs> (there is no working-directory fallback)"},
		{"project-retire", "no id", []string{"--project-root", root, "--reason", "x"}, "error: skills project-retire requires a <id> argument"},
		{"project-retire", "a second id", []string{"--project-root", root, "a", "b"}, `error: skills project-retire: unexpected extra argument "b" (project-retire accepts exactly one <id>)`},
		{"project-retire", "a reason that begins with a dash", []string{"--project-root", root, "--reason", "-x", "a"}, `error: skills project-retire: flag "--reason" requires a value; got flag token "-x"`},
		{"project-retire", "a flag of register", []string{"--project-root", root, "--candidate", "k", "a"}, `error: skills project-retire: unknown flag "--candidate"`},
		{"project-retire", "--absorbed-into followed by a flag", []string{"--project-root", root, "--absorbed-into", "--dry-run", "a"}, `error: skills project-retire: flag "--absorbed-into" requires a value; got flag token "--dry-run"`},
		{"project-status", "no project root", []string{"--registry", "r.yaml"}, "error: skills project-status requires --project-root <abs> (there is no working-directory fallback)"},
		{"project-status", "a flag of register", []string{"--project-root", root, "--dry-run"}, `error: skills project-status: unknown flag "--dry-run"`},
		{"project-status", "a second id", []string{"--project-root", root, "a", "b"}, `error: skills project-status: unexpected extra argument "b"`},
		{"project-status", "a root that is the last word", []string{"--registry", "r.yaml", "--project-root"}, `error: skills project-status: flag "--project-root" requires a value`},
	}
	for _, tc := range cases {
		t.Run(tc.verb+" "+tc.name, func(t *testing.T) {
			run := map[string]func(skills.Deps, []string, io.Writer, io.Writer, func(int)){
				"project-register": skillsProjectRegister, "project-revise": skillsProjectRevise,
				"project-retire": skillsProjectRetire, "project-status": skillsProjectStatus,
			}[tc.verb]
			deps, locker := untouchedDeps(t)

			r := runSkillsVerb(run, deps, append([]string{tc.verb}, tc.args...)...)

			if r.code() != 1 || r.stdout != "" || r.stderr != tc.want+"\n" {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 1 and %q", r.code(), r.stdout, r.stderr, tc.want)
			}
			if got := locker.log(); len(got) != 0 {
				t.Errorf("lock events = %v, want none: the command line is refused before the lock is asked for", got)
			}
		})
	}
}

// After `--` every word is a word: a draft that begins with a dash can be named.
func TestProjectRegisterEndOfOptionsBindsADashPrefixedDraft(t *testing.T) {
	w := newProjectCLIWorld(t)
	r := w.run(skillsProjectRegister, w.deps(noopOverlayLocker{}),
		"project-register", "--project-root", w.root, "--candidate", projectCand, "--registry", w.registry, "--", "-weird-draft.md")
	if want := `error: reading draft "-weird-draft.md": `; r.code() != 1 || !strings.HasPrefix(r.stderr, want) {
		t.Errorf("exit %d, stderr %q, want the draft -weird-draft.md read verbatim", r.code(), r.stderr)
	}
}

// The `labdrian` wrapper appends --registry, --manifest and --source-root after the verb's own
// arguments: the manifest and the source root are taken and not read, so that their values are not
// mistaken for the draft.
func TestProjectRegisterTakesTheFlagsTheWrapperAppends(t *testing.T) {
	w := newProjectCLIWorld(t)
	args := []string{"project-register", "--project-root", w.root, "--candidate", projectCand, "--dry-run", w.draft,
		"--registry", w.registry, "--manifest", "overlay.manifest", "--source-root", "skills"}

	r := w.run(skillsProjectRegister, w.deps(noopOverlayLocker{}), args...)

	if r.code() != 0 || !strings.Contains(r.stdout, "plan: .claude/skills/"+projectTidyID+"/SKILL.md") {
		t.Errorf("exit %d, stdout %q, stderr %q", r.code(), r.stdout, r.stderr)
	}
}

// The registry the verb is given is the one it reads, not merely a flag it takes: a registry that
// already holds the id of the draft refuses the registration.
func TestProjectRegisterReadsTheRegistryItIsGiven(t *testing.T) {
	w := newProjectCLIWorld(t)
	writeTestFile(t, w.registry, registryOf(projectTidyID))
	r := w.run(skillsProjectRegister, w.deps(noopOverlayLocker{}), w.registerArgs()...)
	if r.code() != 1 || r.stdout != "" || !strings.Contains(r.stderr, projectTidyID) {
		t.Errorf("exit %d, stdout %q, stderr %q, want a refusal that names the id", r.code(), r.stdout, r.stderr)
	}
}

// ---- which lock, and which root ---------------------------------------------------------------------

// The policy of which lock each verb takes on the project is said once, by skills.ProjectLocks, and
// each adapter takes exactly that: exclusive for the verbs that write, shared for status.
func TestProjectVerbsTakeExactlyTheLockThePolicySaysOnTheirRoot(t *testing.T) {
	for _, v := range projectVerbs {
		t.Run(v.name, func(t *testing.T) {
			w := newProjectCLIWorld(t)
			if v.name != "project-register" {
				w.registered()
			}
			locker := &holdingLocker{}
			args := map[string][]string{
				"project-register": w.registerArgs("--dry-run"), "project-revise": w.reviseArgs("--dry-run"),
				"project-retire": w.retireArgs(projectTidyID, "--dry-run"), "project-status": w.statusArgs(""),
			}[v.name]

			r := w.run(v.run, w.deps(locker), args...)

			if r.code() != 0 {
				t.Fatalf("exit %d, stderr %q", r.code(), r.stderr)
			}
			req := skills.ProjectLocks(v.name, w.root)
			if len(req) != 1 || !req[0].Dir {
				t.Fatalf("the policy asks %+v", req)
			}
			kind := "lockdir exclusive "
			if req[0].Mode == skills.LockShared {
				kind = "lockdir shared "
			}
			if want := []string{kind + w.root, "unlock " + w.root}; !reflect.DeepEqual(locker.log(), want) {
				t.Errorf("lock events = %v, want %v", locker.log(), want)
			}
			if (v.name == "project-status") != (req[0].Mode == skills.LockShared) {
				t.Errorf("%s: the project is locked in mode %v", v.name, req[0].Mode)
			}
		})
	}
}

// Whatever the spelling, the directory that is locked is the one the use case works in: both come
// from the one parse of the command line.
func TestTheProjectLockedIsTheProjectTheVerbReadsForEverySpellingOfTheCommandLine(t *testing.T) {
	w := newProjectCLIWorld(t)
	w.registered()
	other := t.TempDir()
	reg := []string{"--registry", w.registry}
	cat := func(parts ...[]string) []string {
		var out []string
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}
	for _, tc := range []struct {
		name  string
		args  []string
		locks bool
	}{
		{"one root", cat([]string{"--project-root", w.root}, reg, []string{"--dry-run", projectTidyID}), true},
		{"the last of two roots is the one used", cat([]string{"--project-root", other, "--project-root", w.root}, reg, []string{"--dry-run", projectTidyID}), true},
		{"an unclean spelling of the root", cat([]string{"--project-root", w.root + string(filepath.Separator) + ".." + string(filepath.Separator) + filepath.Base(w.root)}, reg, []string{"--dry-run", projectTidyID}), true},
		{"the end of options after the root", cat([]string{"--project-root", w.root}, reg, []string{"--dry-run", "--", projectTidyID}), true},
		{"a flag that swallows the next one", cat([]string{"--reason", "--project-root", w.root}, reg, []string{"--dry-run", projectTidyID}), false},
		{"a root only after the end of options", cat([]string{"--", "--project-root", w.root}, reg, []string{projectTidyID}), false},
		{"a relative root", cat([]string{"--project-root", "relative/dir"}, reg, []string{"--dry-run", projectTidyID}), false},
		{"a root spelled with an equals sign", cat([]string{"--project-root=" + w.root}, reg, []string{"--dry-run", projectTidyID}), false},
		{"a root given as a flag's value", cat([]string{"--project-root", "--dry-run"}, reg, []string{projectTidyID}), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			locker := &holdingLocker{}
			r := w.run(skillsProjectRetire, w.deps(locker), append([]string{"project-retire"}, tc.args...)...)
			var locked []string
			for _, e := range locker.log() {
				if dir, ok := strings.CutPrefix(e, "lockdir exclusive "); ok {
					locked = append(locked, dir)
				}
			}
			if tc.locks {
				if r.code() != 0 || !reflect.DeepEqual(locked, []string{filepath.Clean(w.root)}) {
					t.Errorf("exit %d, locked %v, stderr %q, want exactly the project the verb read, %s", r.code(), locked, r.stderr, w.root)
				}
			} else if len(locked) != 0 || r.code() != 1 {
				t.Errorf("exit %d, locked %v, want a refusal and no lock", r.code(), locked)
			}
		})
	}
}

// The verbs lock before they look at the project, and let go whatever they decide.
func TestProjectVerbsLetGoOfTheLockWhenTheyRefuse(t *testing.T) {
	w := newProjectCLIWorld(t)
	locker := &holdingLocker{}
	// The project has no lock file, so revise, retire and status refuse after the lock is taken.
	for _, v := range []struct {
		run  func(skills.Deps, []string, io.Writer, io.Writer, func(int))
		args []string
	}{
		{skillsProjectRevise, w.reviseArgs()}, {skillsProjectRetire, w.retireArgs("x")}, {skillsProjectStatus, w.statusArgs("")},
	} {
		locker.events = nil
		r := w.run(v.run, w.deps(locker), v.args...)
		if r.code() != 1 || len(locker.log()) != 2 || !strings.HasPrefix(locker.log()[1], "unlock ") {
			t.Errorf("%s: exit %d, lock events %v, want the lock taken and let go of", v.args[0], r.code(), locker.log())
		}
	}
}

// A busy project lock is reported with exit 2 and a retry message, nothing is written, and the lock
// that the locker says it tried is the one the message names.
func TestAProjectLockThatStaysTakenExits2AndNamesTheDirectoryThatWasLocked(t *testing.T) {
	w := newProjectCLIWorld(t)
	resolved := filepath.Join(t.TempDir(), "the-real-project")
	before := w.tree()
	for name, tc := range map[string]struct {
		err   error
		wants []string
		not   string
	}{
		"a lock that says which path it tried": {busyAtErr{at: resolved}, []string{"the project " + w.root, "lock on the directory " + resolved}, "lock on the directory " + w.root},
		"a lock that says nothing of its path": {busyFailure{}, []string{"the project " + w.root, "lock on the directory " + w.root}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			locker := &holdingLocker{failOn: map[string]error{w.root: tc.err}}

			r := w.run(skillsProjectRegister, w.deps(locker), w.registerArgs()...)

			if r.code() != skills.ExitBusy || r.stdout != "" {
				t.Errorf("exit %d, stdout %q, want exit 2 and nothing on stdout; stderr %q", r.code(), r.stdout, r.stderr)
			}
			for _, want := range append([]string{"skills project-register", "in progress", "retry"}, tc.wants...) {
				if !strings.Contains(r.stderr, want) {
					t.Errorf("stderr %q does not contain %q", r.stderr, want)
				}
			}
			if tc.not != "" && strings.Contains(r.stderr, tc.not) {
				t.Errorf("stderr %q names the path that was asked for, not the one that was taken", r.stderr)
			}
			if !reflect.DeepEqual(before, w.tree()) {
				t.Error("a busy lock changed the project")
			}
		})
	}
}

// busyAtErr is a busy lock that says which path it tried, as engine/filelock's BusyError does: it is
// the resolved directory when the caller reached it through a symlink.
type busyAtErr struct{ at string }

func (e busyAtErr) Error() string    { return "lock " + e.at + " is held by another process" }
func (busyAtErr) Busy() bool         { return true }
func (e busyAtErr) LockPath() string { return e.at }

// A directory that cannot be locked at all is a refusal, exit 1, not a busy lock and not a verb that
// runs unlocked; the message must not tell the caller to retry what will not clear.
func TestAProjectRootThatCannotBeLockedRefusesTheVerbAndDoesNotSayRetry(t *testing.T) {
	w := newProjectCLIWorld(t)
	cause := "filelock: lock " + w.root + ": operation not supported (this filesystem may not support locking a directory; nothing was locked)"
	locker := &holdingLocker{failOn: map[string]error{w.root: errors.New(cause)}}
	before := w.tree()

	r := w.run(skillsProjectRegister, w.deps(locker), w.registerArgs()...)

	if r.code() != 1 || r.stdout != "" {
		t.Errorf("exit %d, stdout %q, want exit 1 and nothing on stdout", r.code(), r.stdout)
	}
	for _, want := range []string{"cannot take the lock", "may not support locking a directory"} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("stderr %q does not contain %q", r.stderr, want)
		}
	}
	if strings.Contains(r.stderr, "retry") || !reflect.DeepEqual(before, w.tree()) {
		t.Errorf("stderr %q, tree changed %v", r.stderr, !reflect.DeepEqual(before, w.tree()))
	}
}

// Without a locker the project verbs refuse to run unserialized: the three that write would lose
// updates if two interleaved, and status reads the lock and then the files a revision's renames leave
// disagreeing for a moment.
func TestProjectVerbsWithoutALockerFailClosed(t *testing.T) {
	for _, v := range projectVerbs {
		t.Run(v.name, func(t *testing.T) {
			w := newProjectCLIWorld(t)
			deps := w.deps(noopOverlayLocker{})
			deps.Locker = nil
			args := map[string][]string{
				"project-register": w.registerArgs(), "project-revise": w.reviseArgs(),
				"project-retire": w.retireArgs("x"), "project-status": w.statusArgs(""),
			}[v.name]

			r := w.run(v.run, deps, args...)

			want := "error: skills " + v.name + ": no lock is configured, so it will not run unserialized with the other skills commands\n"
			if r.code() != 1 || r.stdout != "" || r.stderr != want {
				t.Errorf("exit %d, stdout %q, stderr %q, want %q", r.code(), r.stdout, r.stderr, want)
			}
			if _, err := os.Stat(filepath.Join(w.root, ".claude")); err == nil {
				t.Error("a verb without a lock touched the project")
			}
		})
	}
}

// A composition root that forgot a port gets a refusal, after the lock.
func TestProjectVerbsRefuseAPortThatWasNotWired(t *testing.T) {
	for _, v := range projectVerbs {
		for _, tc := range []struct {
			name  string
			unset func(*skills.Deps)
			want  string
		}{
			{"the project file system", func(d *skills.Deps) { d.Project = nil }, "no project file system is wired, so it cannot read or write files"},
			{"the project lock store", func(d *skills.Deps) { d.ProjectLocks = nil }, "no project lock store is wired, so it cannot read the lock of the project"},
		} {
			t.Run(v.name+" without "+tc.name, func(t *testing.T) {
				w := newProjectCLIWorld(t)
				locker := &holdingLocker{}
				deps := w.deps(locker)
				tc.unset(&deps)
				args := map[string][]string{
					"project-register": w.registerArgs(), "project-revise": w.reviseArgs(),
					"project-retire": w.retireArgs("x"), "project-status": w.statusArgs(""),
				}[v.name]

				r := w.run(v.run, deps, args...)

				if want := "error: skills " + v.name + ": " + tc.want + "\n"; r.code() != 1 || r.stdout != "" || r.stderr != want {
					t.Errorf("exit %d, stdout %q, stderr %q, want %q", r.code(), r.stdout, r.stderr, want)
				}
				if got := locker.log(); len(got) != 2 {
					t.Errorf("lock events = %v, want the lock taken and let go of", got)
				}
			})
		}
	}
}

// ---- the interleavings the project lock exists to prevent --------------------------------------------

// secondRegisterArgs registers another skill, id, in the project of the world.
func (w projectCLIWorld) secondRegisterArgs(id string) []string {
	draft := filepath.Join(filepath.Dir(w.draft), "second", id, "SKILL.md")
	writeTestFile(w.t, draft, goldenProjectDraft(id))
	return []string{"project-register", "--project-root", w.root, "--candidate", "procedural/candidates/repeated-success/" + id, "--registry", w.registry, draft}
}

// Two registrations in one project that both read the project lock before either wrote it used to
// leave one skill unregistered (its files written, its entry gone) although both exited 0.
func TestConcurrentProjectRegistrationsBothLandInTheProjectLock(t *testing.T) {
	w := newProjectCLIWorld(t)
	gate := newReadGate(t, skills.ProjectLockPath(w.root))
	locker := &exclusionLocker{blocked: gate.release}
	deps := w.deps(locker)
	deps.ProjectLocks = parkedLocks{deps.ProjectLocks, gate}
	register := func(args []string) func() verbRun {
		return func() verbRun { return runSkillsVerb(skillsProjectRegister, deps, args...) }
	}

	results := concurrently(register(w.registerArgs()), register(w.secondRegisterArgs("tidy-repo")))

	for i, r := range results {
		if r.code() != 0 {
			t.Errorf("registration #%d: exit %d, stderr=%q", i, r.code(), r.stderr)
		}
	}
	if got, want := w.lockIDs(), []string{"tidy-repo", projectTidyID}; !reflect.DeepEqual(got, want) {
		t.Errorf("the project lock lists %v, want %v: a registration that exited 0 was lost", got, want)
	}
}

// Two retirements in one project, the same way: neither may undo the other.
func TestConcurrentProjectRetirementsBothLeaveTheProjectLock(t *testing.T) {
	w := newProjectCLIWorld(t)
	w.registered()
	if r := w.run(skillsProjectRegister, w.deps(noopOverlayLocker{}), w.secondRegisterArgs("tidy-repo")...); r.code() != 0 {
		t.Fatalf("second registration: exit %d, stderr %q", r.code(), r.stderr)
	}
	gate := newReadGate(t, skills.ProjectLockPath(w.root))
	locker := &exclusionLocker{blocked: gate.release}
	deps := w.deps(locker)
	deps.ProjectLocks = parkedLocks{deps.ProjectLocks, gate}
	retire := func(id string) func() verbRun {
		return func() verbRun {
			return runSkillsVerb(skillsProjectRetire, deps, w.retireArgs(id, "--reason", "human-request")...)
		}
	}

	results := concurrently(retire(projectTidyID), retire("tidy-repo"))

	for i, r := range results {
		if r.code() != 0 {
			t.Errorf("retirement #%d: exit %d, stderr=%q", i, r.code(), r.stderr)
		}
	}
	if got := w.lockIDs(); len(got) != 0 {
		t.Errorf("the project lock still lists %v: a retirement that exited 0 was undone", got)
	}
}

// A status that starts while a revision is in progress waits for it, and reports the revised skill
// as the agent's: the writer is parked after it read the project lock, holding the exclusive lock,
// and the status can only be granted its shared one once the writer has let go.
func TestAProjectStatusStartedDuringARevisionWaitsForItAndSeesTheRevision(t *testing.T) {
	w := newProjectCLIWorld(t)
	w.registered()
	gate := newReadGate(t, skills.ProjectLockPath(w.root))
	locker := &exclusionLocker{blocked: gate.release}
	parked := w.deps(locker)
	parked.ProjectLocks = parkedLocks{parked.ProjectLocks, gate}

	revised := make(chan verbRun, 1)
	go func() { revised <- runSkillsVerb(skillsProjectRevise, parked, w.reviseArgs()...) }()
	<-gate.arrived
	status := w.run(skillsProjectStatus, w.deps(locker), w.statusArgs(projectTidyID)...)
	gate.release()
	rev := <-revised

	if rev.code() != 0 {
		t.Fatalf("project-revise: exit %d, stderr %q", rev.code(), rev.stderr)
	}
	if status.code() != 0 || !strings.Contains(status.stdout, "rev:2 owner:agent") {
		t.Errorf("project-status: exit %d, stdout %q, stderr %q, want revision 2 owned by the agent", status.code(), status.stdout, status.stderr)
	}
	events := locker.events()
	released, granted := indexOfEvent(events, "released exclusive project"), indexOfEvent(events, "granted shared project")
	if released < 0 || granted < 0 || released > granted {
		t.Errorf("lock events %v: the status was granted its shared lock before the revision released the exclusive one", events)
	}
}

// The lock is per project root: two projects do not wait for each other.
func TestProjectLocksOfDifferentRootsDoNotExcludeEachOther(t *testing.T) {
	one, two := newProjectCLIWorld(t), newProjectCLIWorld(t)
	gate := newReadGate(t, skills.ProjectLockPath(one.root))
	blocked := false
	locker := &exclusionLocker{blocked: func() { blocked = true; gate.release() }}
	parked := one.deps(locker)
	parked.ProjectLocks = parkedLocks{parked.ProjectLocks, gate}

	firstDone := make(chan verbRun, 1)
	go func() { firstDone <- runSkillsVerb(skillsProjectRegister, parked, one.registerArgs()...) }()
	<-gate.arrived
	other := two.run(skillsProjectRegister, two.deps(locker), two.registerArgs()...)
	gate.release()
	first := <-firstDone

	if other.code() != 0 || first.code() != 0 || blocked {
		t.Errorf("exits %d and %d, blocked %v (stderr %q, %q), want both to run without waiting for each other", first.code(), other.code(), blocked, first.stderr, other.stderr)
	}
}
