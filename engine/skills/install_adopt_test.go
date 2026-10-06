package skills

// `skills adopt` is how a skill that was installed before install kept records, or by
// hand, becomes one install may replace. It is explicit, and it only records: it
// writes the project lock and no skill file. It takes ownership of a directory only
// when the directory is exactly the current source, and when it is not, it says which
// file differs.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// preexisting lays down the source's files in the given runtime directories with no
// record of them, as an install from before records existed, or a copy by hand, would.
func (f *ownFixture) preexisting(id string, files map[string]string, runtimes ...string) {
	f.t.Helper()
	for _, runtime := range runtimes {
		for path, content := range files {
			f.write(runtime+"/skills/"+id+"/"+path, content)
		}
	}
}

func (f *ownFixture) adoptPlan(skills ...InstallSkill) (InstallPlan, []string) {
	f.t.Helper()
	return PlanAdopt(f.input(skills...))
}

func (f *ownFixture) adopt(skills ...InstallSkill) InstallPlan {
	f.t.Helper()
	plan, refusals := f.adoptPlan(skills...)
	if len(refusals) != 0 {
		f.t.Fatalf("adopt refused: %v", refusals)
	}
	var errOut bytes.Buffer
	if err := ExecuteInstallPlan(plan, f.root, testProjectFS(), &errOut); err != nil {
		f.t.Fatalf("adopt failed: %v (stderr %q)", err, errOut.String())
	}
	return plan
}

func TestAdopt_RecordsAnExistingInstallThatIsExactlyTheSource(t *testing.T) {
	f := newOwnFixture(t)
	f.preexisting("pdf", pdfV1, ".claude", ".agents")
	before := f.snapshot()

	plan := f.adopt(skill("pdf", pdfV1))

	if got := statusOf(plan, "pdf"); got != InstallAdopted {
		t.Errorf("status = %q, want %q", got, InstallAdopted)
	}
	if len(plan.Writes) != 0 || len(plan.Deletes) != 0 {
		t.Errorf("adopt planned %d writes and %d deletes; it only records", len(plan.Writes), len(plan.Deletes))
	}
	after := f.snapshot()
	for path, content := range before {
		if after[path] != content {
			t.Errorf("adopt changed %s", path)
		}
	}
	if got := installIDs(f.lock()); strings.Join(got, ",") != "pdf" {
		t.Errorf("recorded = %v, want pdf", got)
	}
	// From now on it is ours: the same source is unchanged, a newer one is replaced.
	again, refusals := f.plan(skill("pdf", pdfV1))
	if len(refusals) != 0 || statusOf(again, "pdf") != InstallUnchanged {
		t.Errorf("install after adopt: status %q, refusals %v, want unchanged", statusOf(again, "pdf"), refusals)
	}
	f.install(skill("pdf", map[string]string{"SKILL.md": "# pdf v2\n", "references/guide.md": "guide v1\n"}))
	if got := f.read(".agents/skills/pdf/SKILL.md"); got != "# pdf v2\n" {
		t.Errorf("install after adopt did not replace the file: %q", got)
	}
}

func TestAdopt_IsIdempotent(t *testing.T) {
	f := newOwnFixture(t)
	f.preexisting("pdf", pdfV1, ".claude", ".agents")
	f.adopt(skill("pdf", pdfV1))
	before := f.snapshot()

	plan, refusals := f.adoptPlan(skill("pdf", pdfV1))

	if len(refusals) != 0 {
		t.Fatalf("refused: %v", refusals)
	}
	if got := statusOf(plan, "pdf"); got != InstallUnchanged {
		t.Errorf("status = %q, want %q", got, InstallUnchanged)
	}
	if plan.Lock.Rel != "" {
		t.Error("a second adopt rewrites the lock")
	}
	assertSameTree(t, before, f.snapshot())
}

func TestAdopt_OneRuntimeIsEnoughAndTheOtherIsLeftForInstall(t *testing.T) {
	f := newOwnFixture(t)
	f.preexisting("pdf", pdfV1, ".claude")

	plan := f.adopt(skill("pdf", pdfV1))

	if f.exists(".agents") {
		t.Error("adopt created the other runtime's directory; it only records")
	}
	joined := strings.Join(plan.Notes, "\n")
	if !strings.Contains(joined, ".agents/skills/pdf") || !strings.Contains(joined, "skills install") {
		t.Errorf("notes %q do not tell the user the other runtime is not installed", joined)
	}
	again := f.install(skill("pdf", pdfV1))
	if got := statusOf(again, "pdf"); got != InstallUpdated {
		t.Errorf("install after adopt: status %q, want %q (it adds the missing runtime copy)", got, InstallUpdated)
	}
	if got := f.read(".agents/skills/pdf/SKILL.md"); got != "# pdf v1\n" {
		t.Errorf(".agents copy = %q", got)
	}
}

func TestAdopt_RefusesADirectoryThatIsNotExactlyTheSourceAndNamesTheFile(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(f *ownFixture)
		want   []string
	}{
		"a file with other bytes": {
			func(f *ownFixture) { f.write(".claude/skills/pdf/SKILL.md", "my version\n") },
			[]string{".claude/skills/pdf", "SKILL.md", "other bytes"},
		},
		"a file the source has is missing": {
			func(f *ownFixture) { _ = os.Remove(filepath.Join(f.root, ".agents/skills/pdf/references/guide.md")) },
			[]string{".agents/skills/pdf", "references/guide.md", "missing"},
		},
		"a file the source does not have": {
			func(f *ownFixture) { f.write(".claude/skills/pdf/MY-NOTES.md", "mine\n") },
			[]string{".claude/skills/pdf", "MY-NOTES.md", "not in the source"},
		},
		"a symlink inside": {
			func(f *ownFixture) {
				_ = os.Symlink("SKILL.md", filepath.Join(f.root, ".claude/skills/pdf/alias.md"))
			},
			[]string{".claude/skills/pdf", "alias.md is not a regular file"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newOwnFixture(t)
			f.preexisting("pdf", pdfV1, ".claude", ".agents")
			tc.mutate(f)
			before := f.snapshot()

			plan, refusals := f.adoptPlan(skill("pdf", pdfV1))

			refusedWith(t, refusals, tc.want...)
			if plan.Lock.Rel != "" {
				t.Error("a refused adopt still carries a lock write")
			}
			assertSameTree(t, before, f.snapshot())
		})
	}
}

func TestAdopt_NamesEveryDifferingFileInEveryRuntime(t *testing.T) {
	f := newOwnFixture(t)
	f.preexisting("pdf", pdfV1, ".claude", ".agents")
	f.write(".claude/skills/pdf/SKILL.md", "claude edit\n")
	f.write(".agents/skills/pdf/references/guide.md", "agents edit\n")

	_, refusals := f.adoptPlan(skill("pdf", pdfV1))

	refusedWith(t, refusals, ".claude/skills/pdf", "SKILL.md", ".agents/skills/pdf", "references/guide.md")
}

func TestAdopt_RefusesASkillThatIsNotInstalledAnywhere(t *testing.T) {
	f := newOwnFixture(t)

	_, refusals := f.adoptPlan(skill("pdf", pdfV1))

	refusedWith(t, refusals, "pdf", "not installed", "skills install")
}

// All or nothing, like install: one skill that cannot be adopted stops the rest.
func TestAdopt_OneSkillThatCannotBeAdoptedStopsTheOthers(t *testing.T) {
	f := newOwnFixture(t)
	f.preexisting("good", map[string]string{"SKILL.md": "good\n"}, ".claude")
	f.preexisting("bad", map[string]string{"SKILL.md": "edited\n"}, ".claude")
	before := f.snapshot()

	plan, refusals := f.adoptPlan(skill("good", map[string]string{"SKILL.md": "good\n"}), skill("bad", map[string]string{"SKILL.md": "as shipped\n"}))

	refusedWith(t, refusals, ".claude/skills/bad")
	if plan.Lock.Rel != "" || len(plan.Skills) != 0 {
		t.Errorf("a refused adopt still adopted: %+v", plan)
	}
	assertSameTree(t, before, f.snapshot())
}

func TestAdopt_ASkillInstallAlreadyOwnsIsUnchangedOrPointsAtInstall(t *testing.T) {
	t.Run("current", func(t *testing.T) {
		f := newOwnFixture(t)
		f.install(skill("pdf", pdfV1))

		plan, refusals := f.adoptPlan(skill("pdf", pdfV1))

		if len(refusals) != 0 || statusOf(plan, "pdf") != InstallUnchanged {
			t.Errorf("status %q, refusals %v, want unchanged", statusOf(plan, "pdf"), refusals)
		}
	})
	t.Run("the source moved on", func(t *testing.T) {
		f := newOwnFixture(t)
		f.install(skill("pdf", pdfV1))

		_, refusals := f.adoptPlan(skill("pdf", pdfV2))

		refusedWith(t, refusals, "pdf", "already installed", "skills install")
	})
	t.Run("a recorded file was edited", func(t *testing.T) {
		f := newOwnFixture(t)
		f.install(skill("pdf", pdfV1))
		f.write(".claude/skills/pdf/SKILL.md", "edited\n")

		_, refusals := f.adoptPlan(skill("pdf", pdfV1))

		refusedWith(t, refusals, ".claude/skills/pdf/SKILL.md", "edited")
	})
}

func TestAdopt_RefusesAnIDThatTheProceduralVerbsOwn(t *testing.T) {
	f := newOwnFixture(t)
	f.write(ProjectLockRelPath, string(installLockProcedural(t, "pdf")))
	f.preexisting("pdf", pdfV1, ".claude")

	_, refusals := f.adoptPlan(skill("pdf", pdfV1))

	refusedWith(t, refusals, "pdf", "procedural", "project-register")
}

func TestAdopt_RefusesADirectoryThatLeavesTheProject(t *testing.T) {
	f := newOwnFixture(t)
	outside := t.TempDir()
	writeTestFile(t, filepath.Join(outside, "skills", "pdf", "SKILL.md"), "# pdf v1\n")
	writeTestFile(t, filepath.Join(outside, "skills", "pdf", "references", "guide.md"), "guide v1\n")
	if err := os.Symlink(outside, filepath.Join(f.root, ".claude")); err != nil {
		t.Fatal(err)
	}

	_, refusals := f.adoptPlan(skill("pdf", pdfV1))

	refusedWith(t, refusals, ".claude/skills/pdf", "project root")
	if f.exists(ProjectLockRelPath) {
		t.Error("adopt recorded a directory outside the project")
	}
}

func TestAdopt_RefusesAnUnreadableLockAndAnEmptySource(t *testing.T) {
	f := newOwnFixture(t)
	f.write(ProjectLockRelPath, "{ nope")
	_, refusals := f.adoptPlan(skill("pdf", pdfV1))
	refusedWith(t, refusals, ProjectLockRelPath)

	g := newOwnFixture(t)
	_, refusals = g.adoptPlan(InstallSkill{ID: "pdf"})
	// The refusal is shared with install and names the verb that was asked.
	refusedWith(t, refusals, "skills adopt: skill pdf has no files to adopt")
	for _, r := range refusals {
		if strings.Contains(r, "no files to install") {
			t.Errorf("refusal %q tells adopt it has nothing to install", r)
		}
	}
}

// Two runtime directories that are one directory cannot both be adopted: the skill
// would be recorded twice for a single copy on disk. install refuses the same shape
// for the same reason (TestInstall_RefusesWhenTheTwoRuntimeDirectoriesAreTheSameDirectory).
func TestAdopt_RefusesWhenTheTwoRuntimeDirectoriesAreTheSameDirectory(t *testing.T) {
	f := newOwnFixture(t)
	f.preexisting("pdf", pdfV1, ".claude")
	if err := os.MkdirAll(filepath.Join(f.root, ".agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	// The premise of the test is the link: it skips only where links cannot be made
	// (makeSymlink), and fails on Linux, where CI runs, on any other error.
	makeSymlink(t, filepath.Join(f.root, ".claude", "skills"), filepath.Join(f.root, ".agents", "skills"))

	plan, refusals := f.adoptPlan(skill("pdf", pdfV1))

	refusedWith(t, refusals, "skills adopt:", ".claude/skills/pdf", ".agents/skills/pdf", "resolve to the same directory", "cannot both be adopted")
	if len(plan.Skills) != 0 || plan.Lock.Rel != "" {
		t.Errorf("a refused adopt returned a plan: %+v", plan)
	}
}

// ---- the verb ----------------------------------------------------------------------------

// copyByHand puts the fixture's project skill in the project the way a person, or an
// install from before records, would: both runtimes, no record.
func copyByHand(t *testing.T, f installFixture, content string) {
	t.Helper()
	for _, runtime := range []string{".claude", ".agents"} {
		writeTestFile(t, filepath.Join(f.project, runtime, "skills", "proj", "SKILL.md"), content)
	}
}

func TestAdoptVerb_TakesOwnershipOfAnExistingInstallAndThenInstallLeavesItBe(t *testing.T) {
	f := newInstallFixture(t)
	copyByHand(t, f, lintCleanSkillMD("proj"))

	refused := f.runAt("install", f.installArgs(), os.ReadFile, nil, noopLocker{})
	if refused.code != 1 || !strings.Contains(refused.stderr, "skills adopt") {
		t.Fatalf("install before adopt: exit %d, stderr %q, want a refusal that points at adopt", refused.code, refused.stderr)
	}

	adopted := f.runAt("adopt", f.installArgs(), os.ReadFile, nil, noopLocker{})
	if adopted.code != 0 || !strings.Contains(adopted.stdout, "adopted: proj") {
		t.Fatalf("adopt: exit %d, stdout %q, stderr %q", adopted.code, adopted.stdout, adopted.stderr)
	}

	again := f.runAt("install", f.installArgs(), os.ReadFile, nil, noopLocker{})
	if again.code != 0 || !strings.Contains(again.stdout, "unchanged: proj") {
		t.Errorf("install after adopt: exit %d, stdout %q, stderr %q, want unchanged", again.code, again.stdout, again.stderr)
	}
	second := f.runAt("adopt", f.installArgs(), os.ReadFile, nil, noopLocker{})
	if second.code != 0 || !strings.Contains(second.stdout, "unchanged: proj") {
		t.Errorf("a second adopt: exit %d, stdout %q, stderr %q, want unchanged", second.code, second.stdout, second.stderr)
	}
}

// A skill that is in one runtime only is adopted there, and the plan says what is left
// for install. The note is part of the verb's output: it goes to stdout, after the
// outcomes, through the same dispatch the command line uses.
func TestAdoptVerb_PrintsTheNoteForTheRuntimeThatDoesNotHaveTheSkill(t *testing.T) {
	f := newInstallFixture(t)
	writeTestFile(t, filepath.Join(f.project, ".claude", "skills", "proj", "SKILL.md"), lintCleanSkillMD("proj"))

	r := f.runAt("adopt", f.installArgs(), os.ReadFile, nil, noopLocker{})

	want := "adopted: proj\nnote: .agents/skills/proj is not installed; run `labdrian skills install --project-id p` to add it\n"
	if r.code != 0 || r.stdout != want || r.stderr != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 0, stdout %q, and nothing on stderr", r.code, r.stdout, r.stderr, want)
	}
}

func TestAdoptVerb_RefusesADifferingDirectoryNamingTheFileAndWritesNothing(t *testing.T) {
	f := newInstallFixture(t)
	copyByHand(t, f, "my own version\n")
	before := snapshotTree(t, f.project)

	r := f.runAt("adopt", f.installArgs(), os.ReadFile, nil, noopLocker{})

	if r.code != 1 || r.stdout != "" {
		t.Fatalf("exit %d, stdout %q, want exit 1 and nothing on stdout", r.code, r.stdout)
	}
	for _, want := range []string{".claude/skills/proj", "SKILL.md has other bytes", "nothing was adopted"} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("stderr %q does not contain %q", r.stderr, want)
		}
	}
	assertSameTree(t, before, snapshotTree(t, f.project))
}

// Adopt takes the locks install takes, in the same order: the overlay's shared lock,
// then the project's exclusive one.
func TestAdoptVerb_TakesTheSameLocksAsInstallInTheSameOrder(t *testing.T) {
	f := newInstallFixture(t)
	copyByHand(t, f, lintCleanSkillMD("proj"))
	locker := &recordingLocker{}

	r := f.runAt("adopt", f.installArgs(), os.ReadFile, nil, locker)

	if r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	overlay := RegistryLockPath(f.reg)
	want := []string{"lock shared " + overlay, "lockdir exclusive " + f.project, "unlock " + f.project, "unlock " + overlay}
	if got := locker.log(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("lock events = %v, want %v", got, want)
	}
}

func TestAdoptVerb_RefusesBeforeLockingWhenItCannotNameItsProjectDirectory(t *testing.T) {
	f := newInstallFixture(t)
	locker := &recordingLocker{}

	r := runAtIn(inDir("relative/dir"), "adopt", f.installArgs(), os.ReadFile, nil, locker)

	if r.code != 1 || !strings.Contains(r.stderr, "skills adopt") || !strings.Contains(r.stderr, "nothing was locked") {
		t.Errorf("exit %d, stderr %q, want a refusal from skills adopt that says nothing was locked", r.code, r.stderr)
	}
	if got := locker.log(); len(got) != 0 {
		t.Errorf("lock events = %v, want none", got)
	}
}

func TestAdoptVerb_IsListedWithTheOtherVerbs(t *testing.T) {
	r := runAt("nuke", nil, os.ReadFile, nil, noopLocker{})
	if !strings.Contains(r.stderr, "adopt") {
		t.Errorf("the supported-verb list %q does not include adopt", r.stderr)
	}
}

func TestAdopt_KeepsTheOtherRecordsOfTheLock(t *testing.T) {
	f := newOwnFixture(t)
	f.install(skill("one", map[string]string{"SKILL.md": "one\n"}))
	f.preexisting("two", map[string]string{"SKILL.md": "two\n"}, ".claude", ".agents")

	f.adopt(skill("one", map[string]string{"SKILL.md": "one\n"}), skill("two", map[string]string{"SKILL.md": "two\n"}))

	if got := strings.Join(installIDs(f.lock()), ","); got != "one,two" {
		t.Errorf("recorded = %s, want one,two", got)
	}
}
