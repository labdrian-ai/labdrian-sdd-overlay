package app

// The verbs that keep the skills the agent registers in a project, as use cases, over a real project
// in a temporary directory (the file system adapters) and a registry held in memory. What each
// refusal is called, what is left in the project and what is told of each skill is what these tests
// hold; the words a person reads are the adapter's, and the planners and the executors are tested
// in engine/skills.

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/skillsfs"
)

const (
	tidyID              = "tidy-worktree"
	tidyCandidate       = "procedural/candidates/repeated-success/tidy-worktree"
	projectRegistryPath = "overlay.registry.yaml"
)

// validDraft is the smallest draft the planner registers.
func validDraft(name string) string {
	return "---\nname: " + name + "\ndescription: Tidy a git worktree before handing it to a reviewer.\nlicense: Apache-2.0\n" +
		"metadata:\n  author: someone\n  version: 1.0.0\n---\n\n## Activation Contract\n\nUse when a worktree must be handed over clean.\n"
}

// projectWorld is an empty project, a draft outside it, and a registry that matches nothing.
type projectWorld struct {
	t               *testing.T
	root, draft     string
	registry        skills.Registry
	locks           skills.ProjectLockStore
	files           skills.FileReader
	project         skills.ProjectFS
	decoy, decoyRaw string
}

func newProjectWorld(t *testing.T) *projectWorld {
	t.Helper()
	base := t.TempDir()
	w := &projectWorld{
		t: t, root: filepath.Join(base, "project"), draft: filepath.Join(base, "drafts", "SKILL.md"),
		registry: registryOf(entry("unrelated-skill", "custom", "claude")),
		locks:    skillsfs.ProjectLocks{}, files: os.ReadFile, project: skillsfs.Project{},
	}
	put(t, w.draft, validDraft(tidyID))
	// The overlay's own lock file sits in the project and is never touched by this tier.
	w.decoy, w.decoyRaw = filepath.Join(w.root, "skills-lock.json"), "{\"decoy\":\"the overlay's own lock file is never touched\"}\n"
	put(t, w.decoy, w.decoyRaw)
	return w
}

func (w *projectWorld) ports() ProjectPorts {
	return ProjectPorts{Registries: registries{projectRegistryPath: w.registry}, Files: w.files, Locks: w.locks, Project: w.project}
}

func (w *projectWorld) register(dry bool) (ProjectRegisterResult, error) {
	return ProjectRegister(w.ports(), ProjectRegisterInput{ProjectRoot: w.root, Candidate: tidyCandidate, RegistryPath: projectRegistryPath, DraftPath: w.draft, DryRun: dry})
}

func (w *projectWorld) revise(dry bool) (ProjectReviseResult, error) {
	return ProjectRevise(w.ports(), ProjectReviseInput{ProjectRoot: w.root, Candidate: tidyCandidate, DraftPath: w.draft, DryRun: dry})
}

func (w *projectWorld) retire(id string, dry bool) (ProjectRetireResult, error) {
	return ProjectRetire(w.ports(), ProjectRetireInput{ProjectRoot: w.root, RegistryPath: projectRegistryPath, ID: id, Reason: "human-request", DryRun: dry})
}

func (w *projectWorld) status(id string) (ProjectStatusResult, error) {
	return ProjectStatus(w.ports(), ProjectStatusInput{ProjectRoot: w.root, RegistryPath: projectRegistryPath, ID: id})
}

func (w *projectWorld) snapshot() map[string]string { return snapshot(w.t, w.root) }

func (w *projectWorld) target(runtime string) string {
	return filepath.Join(w.root, "."+runtime, "skills", tidyID, "SKILL.md")
}

// nothingWritten says a refusal left the project as the fixture made it, the decoy included.
func (w *projectWorld) nothingWritten() {
	w.t.Helper()
	for _, rel := range []string{".claude", ".agents", ".labdrian", ".pi"} {
		if _, err := os.Stat(filepath.Join(w.root, rel)); !errors.Is(err, fs.ErrNotExist) {
			w.t.Errorf("%s must not exist after a refusal (stat err = %v)", rel, err)
		}
	}
	if got := read(w.t, w.decoy); got != w.decoyRaw {
		w.t.Errorf("the decoy lock file changed to %q", got)
	}
}

// registered is a world in which the draft was registered and then revised on disk, so that revise,
// status and retire have a skill of the agent to work on.
func newRegisteredWorld(t *testing.T) *projectWorld {
	t.Helper()
	w := newProjectWorld(t)
	if _, err := w.register(false); err != nil {
		t.Fatalf("the first registration: %v", err)
	}
	put(t, w.draft, strings.Replace(validDraft(tidyID), "Use when a worktree must be handed over clean.", "Recheck a worktree before handing it to a reviewer.", 1))
	return w
}

// ---- project-register ------------------------------------------------------------------------

// A real run writes one SKILL.md per fixed target and the lock last, and says each path, the digest
// of the bytes it wrote and the revision.
func TestProjectRegisterWritesEveryTargetAndTheLockLast(t *testing.T) {
	w := newProjectWorld(t)

	res, err := w.register(false)

	if err != nil {
		t.Fatal(err)
	}
	wantPaths := []string{".claude/skills/" + tidyID + "/SKILL.md", ".agents/skills/" + tidyID + "/SKILL.md", skills.ProjectLockRelPath}
	if strings.Join(res.Wrote, "|") != strings.Join(wantPaths, "|") || len(res.Planned) != 0 {
		t.Errorf("Wrote = %v, Planned = %v, want %v written", res.Wrote, res.Planned, wantPaths)
	}
	written := read(t, w.target("claude"))
	if res.SHA256 != skills.HashSkill([]byte(written)) || res.Revision != 1 {
		t.Errorf("SHA256 = %q, Revision = %d, want the digest of what was written and revision 1", res.SHA256, res.Revision)
	}
	if read(t, w.target("agents")) != written {
		t.Error("the two runtimes do not hold the same bytes")
	}
	if _, err := os.Stat(filepath.Join(w.root, ".pi")); err == nil {
		t.Error(".pi/ must never be created")
	}
	if got := read(t, w.decoy); got != w.decoyRaw {
		t.Errorf("the decoy lock file changed to %q", got)
	}
}

// A dry run says the paths of the plan, the lock last, and writes nothing at all.
func TestProjectRegisterDryRunSaysThePlanAndWritesNothing(t *testing.T) {
	w := newProjectWorld(t)

	res, err := w.register(true)

	want := []string{".claude/skills/" + tidyID + "/SKILL.md", ".agents/skills/" + tidyID + "/SKILL.md", skills.ProjectLockRelPath}
	if err != nil || strings.Join(res.Planned, "|") != strings.Join(want, "|") || len(res.Wrote) != 0 || res.SHA256 != "" || res.Revision != 0 {
		t.Fatalf("ProjectRegister = %+v, %v, want only the plan %v", res, err, want)
	}
	w.nothingWritten()
}

// What is told as a path joined onto the root names a real file after a write, which is what
// `git -C root add -- path` stages: the plan and the write agree on every path.
func TestProjectRegisterPlanAndWriteSayTheSamePathsAndEachIsRepoRelative(t *testing.T) {
	w := newProjectWorld(t)
	planned, err := w.register(true)
	if err != nil {
		t.Fatal(err)
	}
	wrote, err := w.register(false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(planned.Planned, "|") != strings.Join(wrote.Wrote, "|") {
		t.Fatalf("planned %v, wrote %v: they must be one set of pathspecs", planned.Planned, wrote.Wrote)
	}
	for _, p := range wrote.Wrote {
		if filepath.IsAbs(p) || strings.Contains(p, `\`) || strings.Contains(p, w.root) {
			t.Errorf("path %q must be repo-relative with forward slashes", p)
		}
		if _, err := os.Stat(filepath.Join(w.root, filepath.FromSlash(p))); err != nil {
			t.Errorf("path %q must name a file under the project root: %v", p, err)
		}
	}
}

// The registry is read, not only taken: a registry that already holds the identity of the draft
// refuses the registration, since a global skill owns it and the project must not shadow it.
func TestProjectRegisterReadsTheRegistryAndRefusesAnIdentityItOwns(t *testing.T) {
	w := newProjectWorld(t)
	w.registry = registryOf(entry(tidyID, "custom", "claude"))

	_, err := w.register(false)

	var refusal *PlanFailure
	if !errors.As(err, &refusal) || !strings.Contains(err.Error(), tidyID) {
		t.Fatalf("err = %v, want a *PlanFailure that names %s", err, tidyID)
	}
	w.nothingWritten()
}

// An unreadable registry is a refusal: registering while the identity check cannot run is exactly
// the shadowing the check exists to prevent.
func TestProjectRegisterRefusesARegistryItCannotRead(t *testing.T) {
	w := newProjectWorld(t)

	_, err := ProjectRegister(w.ports(), ProjectRegisterInput{ProjectRoot: w.root, Candidate: tidyCandidate, RegistryPath: "no-such-registry.yaml", DraftPath: w.draft})

	var refusal *RegistryError
	if !errors.As(err, &refusal) || !refusal.Unreadable() || refusal.Path != "no-such-registry.yaml" {
		t.Errorf("err = %v, want an unreadable *RegistryError naming the registry", err)
	}
	w.nothingWritten()
}

func TestProjectRegisterSaysWhatTheReaderLeftOutOfTheRegistryEvenWhenItRefuses(t *testing.T) {
	w := newProjectWorld(t)
	w.registry.Unread = []string{`line 3: unknown key "color" in skill entry`}
	w.draft = filepath.Join(w.root, "draft-SKILL.md") // inside the project: refused by the planner
	put(t, w.draft, validDraft(tidyID))

	res, err := w.register(false)

	var refusal *PlanFailure
	if !errors.As(err, &refusal) || res.UnreadWarning != w.registry.UnreadWarning() || res.UnreadWarning == "" {
		t.Errorf("ProjectRegister = %+v, %v, want the refusal and the warning", res, err)
	}
}

func TestProjectRegisterSaysADraftThatCannotBeRead(t *testing.T) {
	w := newProjectWorld(t)
	w.draft = filepath.Join(t.TempDir(), "absent", "SKILL.md")

	_, err := w.register(false)

	var unreadable *DraftReadError
	if !errors.As(err, &unreadable) || unreadable.Path != w.draft || !errors.Is(err, fs.ErrNotExist) || !strings.HasPrefix(err.Error(), "reading draft ") {
		t.Errorf("err = %v, want a *DraftReadError for %s", err, w.draft)
	}
	w.nothingWritten()
}

// A refusal of the planner writes nothing and carries the words of the planner.
func TestProjectRegisterRefusalOfThePlannerWritesNothing(t *testing.T) {
	w := newProjectWorld(t)
	w.draft = filepath.Join(w.root, "draft-SKILL.md")
	put(t, w.draft, validDraft(tidyID))

	res, err := w.register(false)

	var refusal *PlanFailure
	if !errors.As(err, &refusal) || !strings.Contains(err.Error(), "outside the project root") || len(res.Wrote) != 0 {
		t.Fatalf("ProjectRegister = %+v, %v, want the refusal of a draft inside the project", res, err)
	}
	if _, statErr := os.Stat(w.target("claude")); statErr == nil {
		t.Error("a skill was written although the plan was refused")
	}
}

// The project lock is read through the store, and one that cannot be read stops the run instead of
// being replaced by an empty lock that would drop every skill already registered.
func TestProjectRegisterStopsOnAProjectLockItCannotRead(t *testing.T) {
	w := newProjectWorld(t)
	if err := os.MkdirAll(skills.ProjectLockPath(w.root), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := w.register(false)

	var unreadable *ProjectLockReadError
	if !errors.As(err, &unreadable) {
		t.Fatalf("err = %v, want a *ProjectLockReadError", err)
	}
	if _, statErr := os.Stat(w.target("claude")); statErr == nil {
		t.Error("a skill was written although the lock could not be read")
	}
}

func TestProjectRegisterReadsTheLockThroughTheStoreAndNotTheFileReader(t *testing.T) {
	w := newProjectWorld(t)
	asked := ""
	w.locks = lockStoreFunc(func(root string) ([]byte, error) { asked = root; return nil, fs.ErrNotExist })
	w.files = func(name string) ([]byte, error) {
		if strings.HasSuffix(filepath.ToSlash(name), skills.ProjectLockRelPath) {
			t.Errorf("the lock was read as a file: %s", name)
		}
		return os.ReadFile(name)
	}

	if _, err := w.register(false); err != nil {
		t.Fatal(err)
	}
	if asked != w.root {
		t.Errorf("the store was asked for %q, want the project root %q", asked, w.root)
	}
}

// ---- project-revise --------------------------------------------------------------------------

func TestProjectReviseReplacesTheSkillOfTheAgentAndBumpsItsRevision(t *testing.T) {
	w := newRegisteredWorld(t)

	res, err := w.revise(false)

	if err != nil || res.Revision != 2 || len(res.Wrote) != 3 || res.SHA256 != skills.HashSkill([]byte(read(t, w.target("claude")))) {
		t.Fatalf("ProjectRevise = %+v, %v, want revision 2, three paths and the digest of the new bytes", res, err)
	}
	if !strings.Contains(read(t, w.target("agents")), "Recheck a worktree") {
		t.Error("the revised bytes are not in the second runtime")
	}
}

func TestProjectReviseDryRunSaysThePlanAndWritesNothing(t *testing.T) {
	w := newRegisteredWorld(t)
	before := w.snapshot()

	res, err := w.revise(true)

	if err != nil || len(res.Planned) != 3 || res.Planned[2] != skills.ProjectLockRelPath || len(res.Wrote) != 0 {
		t.Fatalf("ProjectRevise = %+v, %v, want the plan, the lock last", res, err)
	}
	sameTree(t, before, w.snapshot())
}

// A skill a person edited, deleted or added a file to is theirs: the revision is refused, says why
// and writes nothing.
func TestProjectReviseRefusesWhatAPersonOwnsAndWritesNothing(t *testing.T) {
	for _, tc := range []struct {
		reason string
		mutate func(w *projectWorld)
	}{
		{"hash-mismatch", func(w *projectWorld) { put(t, w.target("claude"), read(t, w.target("claude"))+"human edit\n") }},
		{"missing", func(w *projectWorld) {
			if err := os.Remove(w.target("claude")); err != nil {
				t.Fatal(err)
			}
		}},
		{"extra-entry", func(w *projectWorld) {
			put(t, filepath.Join(filepath.Dir(w.target("claude")), "README.md"), "human note\n")
		}},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			w := newRegisteredWorld(t)
			tc.mutate(w)
			before := w.snapshot()

			_, err := w.revise(false)

			var refusal *PlanFailure
			if !errors.As(err, &refusal) || !strings.Contains(err.Error(), tc.reason) || !strings.Contains(err.Error(), "human-owned") {
				t.Errorf("err = %v, want a refusal naming %s and human ownership", err, tc.reason)
			}
			sameTree(t, before, w.snapshot())
		})
	}
}

// Revision works on a lock that is there: a project with none is refused like one whose lock cannot be
// read, and nothing is written.
func TestProjectReviseNeedsTheLockOfTheProject(t *testing.T) {
	w := newProjectWorld(t)

	_, err := w.revise(false)

	var unreadable *ProjectLockReadError
	if !errors.As(err, &unreadable) || !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want a *ProjectLockReadError that is fs.ErrNotExist", err)
	}
	w.nothingWritten()
}

func TestProjectReviseNeedsNoRegistry(t *testing.T) {
	w := newRegisteredWorld(t)
	ports := w.ports()
	ports.Registries = nil // would panic if it were asked

	if _, err := ProjectRevise(ports, ProjectReviseInput{ProjectRoot: w.root, Candidate: tidyCandidate, DraftPath: w.draft, DryRun: true}); err != nil {
		t.Fatal(err)
	}
}

// ---- project-retire --------------------------------------------------------------------------

func TestProjectRetireRemovesTheSkillAndItsLockEntry(t *testing.T) {
	w := newRegisteredWorld(t)

	res, err := w.retire(tidyID, false)

	want := []string{".claude/skills/" + tidyID + "/SKILL.md", ".agents/skills/" + tidyID + "/SKILL.md"}
	if err != nil || strings.Join(res.Removed, "|") != strings.Join(want, "|") {
		t.Fatalf("ProjectRetire = %+v, %v, want %v removed", res, err, want)
	}
	for _, runtime := range []string{"claude", "agents"} {
		if _, err := os.Stat(w.target(runtime)); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s: the skill must be removed, stat err = %v", runtime, err)
		}
	}
	lock, err := skills.ParseProjectLock([]byte(read(t, skills.ProjectLockPath(w.root))))
	if err != nil || len(lock.Skills) != 0 {
		t.Errorf("the lock holds %d skills (%v), want none", len(lock.Skills), err)
	}
	if got := read(t, w.decoy); got != w.decoyRaw {
		t.Errorf("the decoy lock file changed to %q", got)
	}
}

func TestProjectRetireDryRunSaysThePlanAndWritesNothing(t *testing.T) {
	w := newRegisteredWorld(t)
	before := w.snapshot()

	res, err := w.retire(tidyID, true)

	if err != nil || len(res.Planned) != 3 || res.Planned[2] != skills.ProjectLockRelPath || len(res.Removed) != 0 {
		t.Fatalf("ProjectRetire = %+v, %v, want the two deletions and the lock", res, err)
	}
	sameTree(t, before, w.snapshot())
}

func TestProjectRetireRefusesASkillThePersonEditedAndASkillThatIsNotThere(t *testing.T) {
	w := newRegisteredWorld(t)
	put(t, w.target("claude"), read(t, w.target("claude"))+"human edit\n")
	before := w.snapshot()

	_, err := w.retire(tidyID, false)

	var refusal *PlanFailure
	if !errors.As(err, &refusal) || !strings.Contains(err.Error(), "human-owned") {
		t.Errorf("err = %v, want a *PlanFailure naming human ownership", err)
	}
	sameTree(t, before, w.snapshot())

	if _, err := w.retire("no-such-skill", false); !errors.As(err, &refusal) {
		t.Errorf("err = %v, want a refusal for a skill the lock does not record", err)
	}
}

// A retirement that cannot be put back says so in the error, which names what could not be restored
// and says nothing was removed.
func TestProjectRetireThatCannotBePutBackNamesWhatItCouldNotRestoreAndIsRollbackIncomplete(t *testing.T) {
	w := newRegisteredWorld(t)
	first := w.target("claude")
	w.project = &breakingProject{ProjectFS: skillsfs.Project{}, failRemove: first, failRename: first}

	res, err := w.retire(tidyID, false)

	var failed *ExecutionError
	if !errors.As(err, &failed) || !errors.Is(err, skills.ErrRollbackIncomplete) || len(res.Removed) != 0 {
		t.Fatalf("ProjectRetire = %+v, %v, want an *ExecutionError that is rollback incomplete", res, err)
	}
	if want := []string{".claude/skills/" + tidyID + "/SKILL.md"}; !slices.Equal(failed.Unrestored, want) {
		t.Errorf("Unrestored = %q, want the repo-relative path %q that could not be put back", failed.Unrestored, want)
	}
}

// breakingProject is a ProjectFS that fails to remove, or to rename onto, one path.
type breakingProject struct {
	skills.ProjectFS
	failRemove, failRename string
}

func (b *breakingProject) Remove(name string) error {
	if name == b.failRemove {
		return errors.New("injected")
	}
	return b.ProjectFS.Remove(name)
}

func (b *breakingProject) Rename(oldPath, newPath string) error {
	if newPath == b.failRename {
		return errors.New("injected")
	}
	return b.ProjectFS.Rename(oldPath, newPath)
}

// ---- project-status --------------------------------------------------------------------------

func TestProjectStatusSaysWhoOwnsTheSkillAndWhy(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reason string
		agent  bool
		mutate func(w *projectWorld)
	}{
		{"agent", "", true, func(*projectWorld) {}},
		{"hash-mismatch", "hash-mismatch", false, func(w *projectWorld) { put(t, w.target("claude"), read(t, w.target("claude"))+"human edit\n") }},
		{"missing", "missing", false, func(w *projectWorld) {
			if err := os.Remove(w.target("claude")); err != nil {
				t.Fatal(err)
			}
		}},
		{"extra-entry", "extra-entry", false, func(w *projectWorld) {
			put(t, filepath.Join(filepath.Dir(w.target("claude")), "README.md"), "human note\n")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newRegisteredWorld(t)
			tc.mutate(w)

			res, err := w.status(tidyID)

			if err != nil || len(res.Skills) != 1 {
				t.Fatalf("ProjectStatus = %+v, %v", res, err)
			}
			got := res.Skills[0]
			if got.ID != tidyID || got.Revision != 1 || got.AgentOwned != tc.agent || !strings.HasPrefix(got.Reason, tc.reason) || got.SupersededBy != "" {
				t.Errorf("status = %+v, want %s owned by the agent=%v with reason %q", got, tidyID, tc.agent, tc.reason)
			}
		})
	}
}

func TestProjectStatusAnswersForEverySkillOfTheLockAndForOneAskedAbout(t *testing.T) {
	w := newRegisteredWorld(t)

	all, err := w.status("")
	if err != nil || len(all.Skills) != 1 || all.Skills[0].ID != tidyID {
		t.Fatalf("ProjectStatus of every skill = %+v, %v", all, err)
	}
	_, err = w.status("other")
	var missing *SkillNotInLockError
	if !errors.As(err, &missing) || missing.ID != "other" || err.Error() != `project-status: skill "other" is not in the project lock` {
		t.Errorf("err = %v, want a *SkillNotInLockError for other", err)
	}
}

// A global skill supersedes a project one by the id of the skill or by the last slug of its candidate
// key: MatchCandidate is an existence and identity lookup, not a claim about content. The key names
// the skill in Engram, and its last slug is not always the id the draft was registered under.
func TestProjectStatusSaysWhichGlobalSkillSupersedesBothByIDAndByCandidateSlug(t *testing.T) {
	global := func(id, path string) skills.Entry {
		e := entry(id, "custom", "claude")
		e.Path = path
		return e
	}
	for name, tc := range map[string]struct {
		entry     skills.Entry
		candidate string
		want      string
	}{
		"the id of the registry matches the id of the project skill":           {global(tidyID, "skills/replacement"), tidyCandidate, "skills/replacement"},
		"the last slug of the path matches the id of the project skill":        {global("replacement", "skills/tidy-worktree"), tidyCandidate, "skills/tidy-worktree"},
		"the last slug of the path matches the last slug of the candidate key": {global("replacement", "skills/tidy-up"), "procedural/candidates/repeated-success/tidy-up", "skills/tidy-up"},
	} {
		t.Run(name, func(t *testing.T) {
			w := newProjectWorld(t)
			if _, err := ProjectRegister(w.ports(), ProjectRegisterInput{ProjectRoot: w.root, Candidate: tc.candidate, RegistryPath: projectRegistryPath, DraftPath: w.draft}); err != nil {
				t.Fatal(err)
			}
			w.registry = registryOf(tc.entry)

			res, err := w.status(tidyID)

			if err != nil || len(res.Skills) != 1 || res.Skills[0].SupersededBy != tc.want {
				t.Errorf("ProjectStatus = %+v, %v, want superseded by %s", res, err, tc.want)
			}
		})
	}
}

// project-status also works on an unclean spelling of the root: the paths it checks are those of
// the clean one.
func TestProjectStatusWorksOnAnUncleanSpellingOfTheRoot(t *testing.T) {
	w := newRegisteredWorld(t)
	unclean := w.root + string(filepath.Separator) + "." + string(filepath.Separator)

	res, err := ProjectStatus(w.ports(), ProjectStatusInput{ProjectRoot: unclean, RegistryPath: projectRegistryPath})

	if err != nil || len(res.Skills) != 1 || !res.Skills[0].AgentOwned {
		t.Errorf("ProjectStatus = %+v, %v, want the skill owned by the agent", res, err)
	}
}

func TestProjectStatusRefusesALockItCannotUse(t *testing.T) {
	w := newRegisteredWorld(t)
	put(t, skills.ProjectLockPath(w.root), "{ not json")

	_, err := w.status("")

	var parse *ProjectLockParseError
	if !errors.As(err, &parse) {
		t.Errorf("err = %v, want a *ProjectLockParseError", err)
	}
	w2 := newProjectWorld(t)
	_, err = w2.status("")
	var unreadable *ProjectLockReadError
	if !errors.As(err, &unreadable) {
		t.Errorf("err = %v, want a *ProjectLockReadError for a project with no lock", err)
	}
}

func TestProjectStatusWritesNothing(t *testing.T) {
	w := newRegisteredWorld(t)
	before := w.snapshot()
	ports := w.ports()
	ports.Project = readOnlyProject{w.project}

	if _, err := ProjectStatus(ports, ProjectStatusInput{ProjectRoot: w.root, RegistryPath: projectRegistryPath}); err != nil {
		t.Fatal(err)
	}
	sameTree(t, before, w.snapshot())
}

// readOnlyProject is a ProjectFS that fails the test if anything is written through it.
type readOnlyProject struct{ skills.ProjectFS }

func (readOnlyProject) WriteTemp(string, []byte, fs.FileMode) (string, error) {
	panic("project-status wrote a file")
}
func (readOnlyProject) Rename(string, string) error { panic("project-status renamed a file") }
func (readOnlyProject) Remove(string) error         { panic("project-status removed a file") }
func (readOnlyProject) MkdirAll(string, fs.FileMode) error {
	panic("project-status made a directory")
}

// The project root is cleaned before a retirement is planned, so that a spelling with dots in it
// plans and removes the same files the clean one does: the executor finds the root again by the
// paths the plan carries.
func TestProjectRetireWorksOnAnUncleanSpellingOfTheRoot(t *testing.T) {
	w := newRegisteredWorld(t)
	unclean := w.root + string(filepath.Separator) + ".." + string(filepath.Separator) + filepath.Base(w.root)

	res, err := ProjectRetire(w.ports(), ProjectRetireInput{ProjectRoot: unclean, RegistryPath: projectRegistryPath, ID: tidyID, Reason: "human-request"})

	if err != nil || len(res.Removed) != 2 {
		t.Fatalf("ProjectRetire = %+v, %v, want both files removed", res, err)
	}
	if _, err := os.Stat(w.target("claude")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the skill is still there: %v", err)
	}
}
