package skills

// The rules of `skills uninstall`, over a real temporary project. It is install's
// ownership rule run the other way: a skill that install wrote, and left as it wrote
// it, is the program's to remove; anything else is someone's.
//
//	state of the installed skill           action
//	------------------------------------  ------------------------------------------
//	every recorded file as recorded        remove each, the record, and every
//	                                       directory that is then empty
//	a recorded file edited                 refuse, naming the file; nothing is removed
//	a recorded file already gone           nothing to remove for it
//	an unrecorded file beside              leave it, and the directory that holds it
//	no install record                      refuse
//	a procedural skill                     refuse, pointing at project-retire

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (f *ownFixture) planUninstall(id string) (InstallPlan, []string) {
	f.t.Helper()
	in := f.input()
	in.Verb = "uninstall"
	return PlanUninstall(in, id)
}

// uninstall plans and executes, failing the test on a refusal, and returns the skill directories
// that were kept.
func (f *ownFixture) uninstall(id string) (InstallPlan, []string) {
	f.t.Helper()
	plan, refusals := f.planUninstall(id)
	if len(refusals) != 0 {
		f.t.Fatalf("uninstall refused: %v", refusals)
	}
	if err := ExecuteInstallPlan(plan, f.root, testProjectFS()); err != nil {
		f.t.Fatalf("uninstall failed: %v", err)
	}
	return plan, RemoveEmptySkillDirs(testProjectFS(), f.root, plan)
}

func TestUninstall_RemovesEveryRecordedFileTheRecordAndTheSkillDirectories(t *testing.T) {
	f := newOwnFixture(t)
	f.install(skill("pdf", pdfV1), skill("csv", map[string]string{"SKILL.md": "# csv\n"}))

	plan, kept := f.uninstall("pdf")

	for _, target := range projectTargets {
		if f.exists(target.Dir + "/pdf") {
			t.Errorf("%s/pdf is still there", target.Dir)
		}
		if !f.exists(target.Dir + "/csv/SKILL.md") {
			t.Errorf("%s/csv/SKILL.md was removed; only pdf was asked for", target.Dir)
		}
	}
	if len(kept) != 0 {
		t.Errorf("kept = %v, want none", kept)
	}
	lock := f.lock()
	if len(lock.Installs) != 1 || lock.Installs[0].ID != "csv" {
		t.Errorf("install records = %+v, want only csv's", lock.Installs)
	}
	if got := statusOf(plan, "pdf"); got != InstallUninstalled {
		t.Errorf("status = %q, want %q", got, InstallUninstalled)
	}
	if len(plan.Deletes) != 4 {
		t.Errorf("the plan removes %d files, want the 4 recorded ones (2 files in 2 runtimes)", len(plan.Deletes))
	}
}

// The last install record removed leaves the lock file in place, empty of records, as
// project-retire leaves it, so a later revert can restore it.
func TestUninstall_TheLastRecordLeavesTheLockFile(t *testing.T) {
	f := newOwnFixture(t)
	f.install(skill("pdf", pdfV1))

	f.uninstall("pdf")

	if !f.exists(ProjectLockRelPath) {
		t.Fatal("the project lock was removed")
	}
	if lock := f.lock(); len(lock.Installs) != 0 {
		t.Errorf("install records = %+v, want none", lock.Installs)
	}
}

func TestUninstall_RefusesAnEditedFileAndRemovesNothing(t *testing.T) {
	f := newOwnFixture(t)
	f.install(skill("pdf", pdfV1))
	f.write(".agents/skills/pdf/references/guide.md", "my notes\n")
	before := f.snapshot()

	_, refusals := f.planUninstall("pdf")

	refusedWith(t, refusals, "skills uninstall", ".agents/skills/pdf/references/guide.md", "was edited")
	assertSameTree(t, before, f.snapshot())
}

func TestUninstall_RefusesAnIDWithNoInstallRecord(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(f *ownFixture)
	}{
		{"no lock at all", func(*ownFixture) {}},
		{"a lock that records other skills", func(f *ownFixture) { f.install(skill("csv", map[string]string{"SKILL.md": "# csv\n"})) }},
		{"a directory install never recorded", func(f *ownFixture) { f.write(".claude/skills/pdf/SKILL.md", "# mine\n") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newOwnFixture(t)
			tc.setup(f)
			before := f.snapshot()

			_, refusals := f.planUninstall("pdf")

			refusedWith(t, refusals, "skills uninstall", "pdf has no install record in "+ProjectLockRelPath)
			assertSameTree(t, before, f.snapshot())
		})
	}
}

func TestUninstall_RefusesAProceduralSkillAndPointsAtProjectRetire(t *testing.T) {
	f := newOwnFixture(t)
	f.write(ProjectLockRelPath, string(installLockProcedural(t, "pdf")))
	before := f.snapshot()

	_, refusals := f.planUninstall("pdf")

	refusedWith(t, refusals, "pdf", "procedural", "project-retire")
	assertSameTree(t, before, f.snapshot())
}

// A file the record does not list is someone's: it is left, and so is the directory that holds
// it, which RemoveEmptySkillDirs reports as kept.
func TestUninstall_LeavesAnUnrecordedFileAndTheDirectoryThatHoldsIt(t *testing.T) {
	f := newOwnFixture(t)
	f.install(skill("pdf", pdfV1))
	f.write(".claude/skills/pdf/references/mine.md", "mine\n")

	_, kept := f.uninstall("pdf")

	if got := f.read(".claude/skills/pdf/references/mine.md"); got != "mine\n" {
		t.Errorf("the unrecorded file holds %q", got)
	}
	if f.exists(".claude/skills/pdf/SKILL.md") || f.exists(".claude/skills/pdf/references/guide.md") {
		t.Error("a recorded file was left")
	}
	if f.exists(".agents/skills/pdf") {
		t.Error(".agents/skills/pdf is still there, although it held only recorded files")
	}
	if want := []string{".claude/skills/pdf"}; strings.Join(kept, ",") != strings.Join(want, ",") {
		t.Errorf("kept = %v, want %v", kept, want)
	}
	if len(f.lock().Installs) != 0 {
		t.Error("the record was kept")
	}
}

// A runtime directory that is gone, or a recorded file someone already deleted, leaves nothing to
// remove there; the rest is removed as usual.
func TestUninstall_ToleratesARuntimeDirectoryOrAFileThatIsAlreadyGone(t *testing.T) {
	f := newOwnFixture(t)
	f.install(skill("pdf", pdfV1))
	if err := os.RemoveAll(filepath.Join(f.root, ".agents", "skills", "pdf")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(f.root, ".claude", "skills", "pdf", "references", "guide.md")); err != nil {
		t.Fatal(err)
	}

	plan, kept := f.uninstall("pdf")

	if len(plan.Deletes) != 1 {
		t.Errorf("the plan removes %d files, want the one still there", len(plan.Deletes))
	}
	if f.exists(".claude/skills/pdf") || f.exists(".agents/skills/pdf") {
		t.Error("a skill directory is still there")
	}
	if len(kept) != 0 || len(f.lock().Installs) != 0 {
		t.Errorf("kept = %v, records = %+v; want none of either", kept, f.lock().Installs)
	}
}
