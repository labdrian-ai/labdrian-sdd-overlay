package skills

// The ownership rules of `skills install`, from the planner down to the files. Every
// case runs over a real temporary project. The rules, in one table:
//
//	state of a target directory          action
//	-----------------------------------  ------------------------------------------
//	absent, no record                    create every file, record it
//	owned, every file as recorded        replace what the source changed, remove what
//	                                     the source dropped, write nothing if equal
//	owned, a recorded file edited        refuse, naming the file
//	owned, a recorded file missing       write it again
//	owned, an unrecorded file beside     leave it alone; refuse only if the new
//	                                     source wants that very path
//	present, no record (foreign)         refuse, naming the directory and adopt
//
// and nothing is written unless every target of every skill passes.

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// ownFixture is a project directory that install plans and executes against.
type ownFixture struct {
	t    *testing.T
	root string
}

func newOwnFixture(t *testing.T) *ownFixture {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &ownFixture{t: t, root: root}
}

// source builds the files of a skill from a path -> content map.
func source(files map[string]string) []sourceFile {
	var out []sourceFile
	for path, content := range files {
		out = append(out, sourceFile{Rel: path, Data: []byte(content), Mode: 0o644})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rel < out[j].Rel })
	return out
}

func skill(id string, files map[string]string) InstallSkill {
	return InstallSkill{ID: id, Files: source(files)}
}

func (f *ownFixture) input(skills ...InstallSkill) InstallInput {
	f.t.Helper()
	in := InstallInput{
		ProjectRoot: f.root,
		ProjectID:   "proj",
		Skills:      skills,
		ReadFile:    os.ReadFile,
		Stat:        os.Stat,
		ResolvePath: resolvePathKeepingMissing,
		ReadDir:     os.ReadDir,
	}
	data, err := os.ReadFile(filepath.Join(f.root, filepath.FromSlash(ProjectLockRelPath)))
	if err == nil {
		in.LockData, in.LockExists = data, true
	} else if !os.IsNotExist(err) {
		f.t.Fatal(err)
	}
	return in
}

func (f *ownFixture) plan(skills ...InstallSkill) (InstallPlan, []string) {
	f.t.Helper()
	return PlanInstallOwnership(f.input(skills...))
}

// install plans and executes, failing the test on a refusal.
func (f *ownFixture) install(skills ...InstallSkill) InstallPlan {
	f.t.Helper()
	plan, refusals := f.plan(skills...)
	if len(refusals) != 0 {
		f.t.Fatalf("install refused: %v", refusals)
	}
	var errOut bytes.Buffer
	if err := ExecuteInstallPlan(plan, f.root, osProjectFS{}, &errOut); err != nil {
		f.t.Fatalf("install failed: %v (stderr %q)", err, errOut.String())
	}
	return plan
}

func (f *ownFixture) write(rel, content string) {
	f.t.Helper()
	writeTestFile(f.t, filepath.Join(f.root, filepath.FromSlash(rel)), content)
}

func (f *ownFixture) read(rel string) string {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.root, filepath.FromSlash(rel)))
	if err != nil {
		f.t.Fatal(err)
	}
	return string(data)
}

func (f *ownFixture) exists(rel string) bool {
	_, err := os.Lstat(filepath.Join(f.root, filepath.FromSlash(rel)))
	return err == nil
}

func (f *ownFixture) snapshot() map[string]string { return snapshotTree(f.t, f.root) }

func (f *ownFixture) lock() ProjectLock {
	f.t.Helper()
	lock, err := ParseProjectLock([]byte(f.read(ProjectLockRelPath)))
	if err != nil {
		f.t.Fatal(err)
	}
	return lock
}

func statusOf(p InstallPlan, id string) InstallStatus {
	for _, o := range p.Skills {
		if o.ID == id {
			return o.Status
		}
	}
	return "(not in the plan)"
}

func refusedWith(t *testing.T, refusals []string, wants ...string) {
	t.Helper()
	if len(refusals) == 0 {
		t.Fatal("the install was not refused")
	}
	all := strings.Join(refusals, "\n")
	for _, want := range wants {
		if !strings.Contains(all, want) {
			t.Errorf("the refusal %q does not contain %q", all, want)
		}
	}
}

var pdfV1 = map[string]string{"SKILL.md": "# pdf v1\n", "references/guide.md": "guide v1\n"}

// ---- absent: create and record ---------------------------------------------------------

func TestInstall_AbsentTargetsAreCreatedInBothRuntimesAndRecorded(t *testing.T) {
	f := newOwnFixture(t)

	plan := f.install(skill("pdf", pdfV1))

	if got := statusOf(plan, "pdf"); got != InstallCreated {
		t.Errorf("status = %q, want %q", got, InstallCreated)
	}
	for _, dir := range []string{".claude/skills/pdf", ".agents/skills/pdf"} {
		if got := f.read(dir + "/SKILL.md"); got != "# pdf v1\n" {
			t.Errorf("%s/SKILL.md = %q", dir, got)
		}
		if got := f.read(dir + "/references/guide.md"); got != "guide v1\n" {
			t.Errorf("%s/references/guide.md = %q", dir, got)
		}
	}
	lock := f.lock()
	if len(lock.Installs) != 1 || lock.Installs[0].ID != "pdf" {
		t.Fatalf("installs = %+v", lock.Installs)
	}
	want := []ProjectInstallFile{
		{Path: "SKILL.md", SHA256: HashSkill([]byte("# pdf v1\n"))},
		{Path: "references/guide.md", SHA256: HashSkill([]byte("guide v1\n"))},
	}
	if !reflect.DeepEqual(lock.Installs[0].Files, want) {
		t.Errorf("recorded files = %+v, want %+v", lock.Installs[0].Files, want)
	}
}

func TestInstall_AFileKeepsItsSourceModeAndNothingElseIsCreated(t *testing.T) {
	f := newOwnFixture(t)
	script := sourceFile{Rel: "scripts/run.sh", Data: []byte("#!/bin/sh\n"), Mode: 0o755}
	doc := sourceFile{Rel: "SKILL.md", Data: []byte("doc"), Mode: 0o600}

	f.install(InstallSkill{ID: "pdf", Files: []sourceFile{doc, script}})

	for path, want := range map[string]fs.FileMode{
		".claude/skills/pdf/scripts/run.sh": 0o755,
		".agents/skills/pdf/SKILL.md":       0o600,
	} {
		info, err := os.Stat(filepath.Join(f.root, filepath.FromSlash(path)))
		if err != nil || info.Mode().Perm() != want {
			t.Errorf("%s: mode %v (err %v), want %04o", path, info.Mode().Perm(), err, want)
		}
	}
	var got []string
	for k := range f.snapshot() {
		got = append(got, k)
	}
	sort.Strings(got)
	for _, path := range got {
		if strings.HasPrefix(path, ".tmp") || strings.Contains(path, atomicTempPrefix) {
			t.Errorf("a temporary file was left behind: %s", path)
		}
	}
}

// ---- owned and unchanged: nothing to do ------------------------------------------------

func TestInstall_ASecondInstallOfTheSameSourceWritesNothingAndSaysSo(t *testing.T) {
	f := newOwnFixture(t)
	f.install(skill("pdf", pdfV1))
	before := f.snapshot()

	plan, refusals := f.plan(skill("pdf", pdfV1))

	if len(refusals) != 0 {
		t.Fatalf("refused: %v", refusals)
	}
	if got := statusOf(plan, "pdf"); got != InstallUnchanged {
		t.Errorf("status = %q, want %q", got, InstallUnchanged)
	}
	if len(plan.Writes) != 0 || len(plan.Deletes) != 0 || plan.Lock.Rel != "" {
		t.Errorf("the plan has %d writes, %d deletes, lock %q; want none", len(plan.Writes), len(plan.Deletes), plan.Lock.Rel)
	}
	var errOut bytes.Buffer
	if err := ExecuteInstallPlan(plan, f.root, osProjectFS{}, &errOut); err != nil {
		t.Fatal(err)
	}
	assertSameTree(t, before, f.snapshot())
}

// ---- owned and the source changed: replace what it owns -------------------------------

func TestInstall_ReplacesOnlyTheFilesTheSourceChangedAndRecordsThem(t *testing.T) {
	f := newOwnFixture(t)
	f.install(skill("pdf", pdfV1))
	guideBefore := f.snapshot()[".claude/skills/pdf/references/guide.md"]

	v2 := map[string]string{"SKILL.md": "# pdf v2\n", "references/guide.md": pdfV1["references/guide.md"]}
	plan := f.install(skill("pdf", v2))

	if got := statusOf(plan, "pdf"); got != InstallUpdated {
		t.Errorf("status = %q, want %q", got, InstallUpdated)
	}
	if len(plan.Writes) != 2 {
		t.Errorf("planned %d writes, want SKILL.md in both targets only", len(plan.Writes))
	}
	for _, dir := range []string{".claude/skills/pdf", ".agents/skills/pdf"} {
		if got := f.read(dir + "/SKILL.md"); got != "# pdf v2\n" {
			t.Errorf("%s/SKILL.md = %q, want v2", dir, got)
		}
	}
	if got := f.snapshot()[".claude/skills/pdf/references/guide.md"]; got != guideBefore {
		t.Errorf("an unchanged file was rewritten: %q -> %q", guideBefore, got)
	}
	recorded := map[string]string{}
	for _, file := range f.lock().Installs[0].Files {
		recorded[file.Path] = file.SHA256
	}
	if recorded["SKILL.md"] != HashSkill([]byte("# pdf v2\n")) {
		t.Errorf("the record holds %s for SKILL.md, want the digest of v2", recorded["SKILL.md"])
	}
}

func TestInstall_RemovesAnOwnedFileTheSourceDroppedAndTheDirectoryItEmptied(t *testing.T) {
	f := newOwnFixture(t)
	f.install(skill("pdf", pdfV1))

	plan := f.install(skill("pdf", map[string]string{"SKILL.md": "# pdf v1\n", "NOTES.md": "new\n"}))

	if len(plan.Deletes) != 2 {
		t.Errorf("planned %d deletes, want references/guide.md in both targets", len(plan.Deletes))
	}
	for _, dir := range []string{".claude/skills/pdf", ".agents/skills/pdf"} {
		if f.exists(dir + "/references/guide.md") {
			t.Errorf("%s/references/guide.md was not removed", dir)
		}
		if f.exists(dir + "/references") {
			t.Errorf("%s/references is left behind empty", dir)
		}
		if got := f.read(dir + "/NOTES.md"); got != "new\n" {
			t.Errorf("%s/NOTES.md = %q", dir, got)
		}
	}
	for _, in := range f.lock().Installs[0].Files {
		if in.Path == "references/guide.md" {
			t.Error("the record still lists the dropped file")
		}
	}
}

// A file someone else keeps in an installed directory is not ours to touch, and does
// not block an update of ours.
func TestInstall_LeavesAnUnrecordedFileBesideTheSkillAlone(t *testing.T) {
	f := newOwnFixture(t)
	f.install(skill("pdf", pdfV1))
	f.write(".claude/skills/pdf/MY-NOTES.md", "mine\n")
	f.write(".claude/skills/pdf/local/tool.sh", "mine too\n")

	f.install(skill("pdf", map[string]string{"SKILL.md": "# pdf v2\n"}))

	if got := f.read(".claude/skills/pdf/MY-NOTES.md"); got != "mine\n" {
		t.Errorf("MY-NOTES.md = %q", got)
	}
	if got := f.read(".claude/skills/pdf/local/tool.sh"); got != "mine too\n" {
		t.Errorf("local/tool.sh = %q", got)
	}
	if got := f.read(".claude/skills/pdf/SKILL.md"); got != "# pdf v2\n" {
		t.Errorf("SKILL.md = %q", got)
	}
}

// ---- refusals ----------------------------------------------------------------------------

func TestInstall_RefusesAHandEditedFileNamingItAndWritesNothing(t *testing.T) {
	for _, target := range []string{".claude", ".agents"} {
		t.Run(target, func(t *testing.T) {
			f := newOwnFixture(t)
			f.install(skill("pdf", pdfV1))
			f.write(target+"/skills/pdf/SKILL.md", "my local tweak\n")
			before := f.snapshot()

			plan, refusals := f.plan(skill("pdf", map[string]string{"SKILL.md": "# pdf v2\n", "references/guide.md": "guide v1\n"}))

			refusedWith(t, refusals, target+"/skills/pdf/SKILL.md", "edited")
			if len(plan.Writes) != 0 || len(plan.Deletes) != 0 || plan.Lock.Rel != "" {
				t.Errorf("a refused plan still carries work: %d writes, %d deletes, lock %q", len(plan.Writes), len(plan.Deletes), plan.Lock.Rel)
			}
			assertSameTree(t, before, f.snapshot())
		})
	}
}

func TestInstall_RefusesADirectoryItDidNotInstallAndPointsAtAdopt(t *testing.T) {
	f := newOwnFixture(t)
	f.write(".claude/skills/pdf/SKILL.md", "someone else's skill\n")
	before := f.snapshot()

	plan, refusals := f.plan(skill("pdf", pdfV1))

	refusedWith(t, refusals, ".claude/skills/pdf", "not installed by", "skills adopt", "--project-id proj")
	if len(plan.Writes) != 0 || plan.Lock.Rel != "" {
		t.Error("a refused plan still carries work")
	}
	assertSameTree(t, before, f.snapshot())
}

// One foreign target stops the whole install: the other runtime's directory is not
// written either, so a refused install never leaves half of a skill.
func TestInstall_AForeignDirectoryInOneRuntimeStopsBothAndEverySkill(t *testing.T) {
	f := newOwnFixture(t)
	f.write(".agents/skills/pdf/SKILL.md", "foreign\n")
	before := f.snapshot()

	_, refusals := f.plan(skill("other", map[string]string{"SKILL.md": "fine\n"}), skill("pdf", pdfV1))

	refusedWith(t, refusals, ".agents/skills/pdf")
	assertSameTree(t, before, f.snapshot())
	if f.exists(".claude/skills/other") || f.exists(".claude/skills/pdf") {
		t.Error("something was written although an install was refused")
	}
}

func TestInstall_ReportsEveryRefusalNotOnlyTheFirst(t *testing.T) {
	f := newOwnFixture(t)
	f.write(".claude/skills/one/SKILL.md", "foreign one\n")
	f.write(".claude/skills/two/SKILL.md", "foreign two\n")

	_, refusals := f.plan(skill("one", pdfV1), skill("two", pdfV1))

	refusedWith(t, refusals, ".claude/skills/one", ".claude/skills/two")
}

func TestInstall_RefusesAnUnrecordedFileThatTheNewSourceWantsToWrite(t *testing.T) {
	f := newOwnFixture(t)
	f.install(skill("pdf", map[string]string{"SKILL.md": "# pdf v1\n"}))
	f.write(".claude/skills/pdf/EXTRA.md", "mine, before the skill grew one\n")
	before := f.snapshot()

	_, refusals := f.plan(skill("pdf", map[string]string{"SKILL.md": "# pdf v1\n", "EXTRA.md": "the skill's own\n"}))

	refusedWith(t, refusals, ".claude/skills/pdf/EXTRA.md", "not recorded")
	assertSameTree(t, before, f.snapshot())
}

func TestInstall_RefusesWhenAPathItNeedsIsTheWrongKindOfThing(t *testing.T) {
	t.Run("a recorded file became a directory", func(t *testing.T) {
		f := newOwnFixture(t)
		f.install(skill("pdf", pdfV1))
		if err := os.RemoveAll(filepath.Join(f.root, ".claude/skills/pdf/references/guide.md")); err != nil {
			t.Fatal(err)
		}
		f.write(".claude/skills/pdf/references/guide.md/inner", "x")

		_, refusals := f.plan(skill("pdf", pdfV1))

		refusedWith(t, refusals, ".claude/skills/pdf/references/guide.md", "not a regular file")
	})
	t.Run("a directory of the skill is a file", func(t *testing.T) {
		f := newOwnFixture(t)
		f.install(skill("pdf", map[string]string{"SKILL.md": "x"}))
		f.write(".claude/skills/pdf/references", "a file where the source wants a directory")

		_, refusals := f.plan(skill("pdf", pdfV1))

		refusedWith(t, refusals, ".claude/skills/pdf/references")
	})
	t.Run("the target is a file", func(t *testing.T) {
		f := newOwnFixture(t)
		f.write(".claude/skills/pdf", "a file, not a directory")

		_, refusals := f.plan(skill("pdf", pdfV1))

		refusedWith(t, refusals, ".claude/skills/pdf")
	})
}

func TestInstall_RefusesAnIDThatTheProceduralVerbsOwn(t *testing.T) {
	f := newOwnFixture(t)
	f.write(ProjectLockRelPath, string(installLockProcedural(t, "pdf")))
	f.write(".claude/skills/pdf/SKILL.md", "---\nname: pdf\n---\n")
	f.write(".agents/skills/pdf/SKILL.md", "---\nname: pdf\n---\n")
	before := f.snapshot()

	_, refusals := f.plan(skill("pdf", pdfV1))

	refusedWith(t, refusals, "pdf", "procedural", "project-register")
	assertSameTree(t, before, f.snapshot())
}

func installLockProcedural(t *testing.T, id string) []byte {
	t.Helper()
	data, err := SerializeProjectLock(ProjectLock{Version: 1, Skills: []ProjectLockEntry{{
		ID: id, Provenance: "procedural", Candidate: "procedural/candidates/repeated-success/" + id,
		SHA256: HashSkill([]byte("---\nname: pdf\n---\n")), Revision: 1,
		Targets: []string{".claude/skills/" + id + "/SKILL.md", ".agents/skills/" + id + "/SKILL.md"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestInstall_RefusesAnUnreadableLockWithoutReplacingIt(t *testing.T) {
	f := newOwnFixture(t)
	f.write(ProjectLockRelPath, "{ this is not json")
	before := f.snapshot()

	_, refusals := f.plan(skill("pdf", pdfV1))

	refusedWith(t, refusals, ProjectLockRelPath)
	assertSameTree(t, before, f.snapshot())
}

func TestInstall_RefusesASkillWithNothingToInstall(t *testing.T) {
	f := newOwnFixture(t)

	_, refusals := f.plan(InstallSkill{ID: "pdf"})

	refusedWith(t, refusals, "pdf", "no files to install")
}

func TestInstall_RefusesWhenTheTwoRuntimeDirectoriesAreTheSameDirectory(t *testing.T) {
	f := newOwnFixture(t)
	if err := os.MkdirAll(filepath.Join(f.root, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(f.root, ".agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(f.root, ".claude", "skills"), filepath.Join(f.root, ".agents", "skills")); err != nil {
		t.Fatal(err)
	}

	_, refusals := f.plan(skill("pdf", pdfV1))

	refusedWith(t, refusals, ".claude/skills/pdf", ".agents/skills/pdf", "same")
}

func TestInstall_RefusesADestinationThatLeavesTheProject(t *testing.T) {
	f := newOwnFixture(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(f.root, ".claude")); err != nil {
		t.Fatal(err)
	}

	_, refusals := f.plan(skill("pdf", pdfV1))

	refusedWith(t, refusals, ".claude/skills/pdf", "project root")
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Errorf("something was written outside the project: %v", entries)
	}
}

// A file that was recorded and is gone is written again; so is a whole directory.
func TestInstall_WritesAgainWhatWasRemovedByHand(t *testing.T) {
	f := newOwnFixture(t)
	f.install(skill("pdf", pdfV1))
	if err := os.Remove(filepath.Join(f.root, ".claude/skills/pdf/references/guide.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(f.root, ".agents/skills/pdf")); err != nil {
		t.Fatal(err)
	}

	plan := f.install(skill("pdf", pdfV1))

	if got := statusOf(plan, "pdf"); got != InstallUpdated {
		t.Errorf("status = %q, want %q", got, InstallUpdated)
	}
	for _, dir := range []string{".claude/skills/pdf", ".agents/skills/pdf"} {
		if got := f.read(dir + "/references/guide.md"); got != "guide v1\n" {
			t.Errorf("%s/references/guide.md = %q", dir, got)
		}
	}
}

// Other skills and other installs are not disturbed by one install.
func TestInstall_KeepsTheRecordsAndFilesOfOtherSkills(t *testing.T) {
	f := newOwnFixture(t)
	f.install(skill("one", map[string]string{"SKILL.md": "one\n"}))
	oneBefore := f.snapshot()[".claude/skills/one/SKILL.md"]

	f.install(skill("two", map[string]string{"SKILL.md": "two\n"}))

	if got := f.snapshot()[".claude/skills/one/SKILL.md"]; got != oneBefore {
		t.Errorf("skill one changed: %q -> %q", oneBefore, got)
	}
	if ids := installIDs(f.lock()); !reflect.DeepEqual(ids, []string{"one", "two"}) {
		t.Errorf("recorded skills = %v, want one and two", ids)
	}
}

func installIDs(l ProjectLock) []string {
	var ids []string
	for _, in := range l.Installs {
		ids = append(ids, in.ID)
	}
	return ids
}

// A procedural skill in the same lock is not disturbed by an install, and the install
// record survives the procedural verbs rewriting the lock.
func TestInstall_KeepsTheProceduralEntriesOfTheSameLock(t *testing.T) {
	f := newOwnFixture(t)
	f.write(ProjectLockRelPath, string(installLockProcedural(t, "tidy")))

	f.install(skill("pdf", pdfV1))

	lock := f.lock()
	if len(lock.Skills) != 1 || lock.Skills[0].ID != "tidy" || !reflect.DeepEqual(installIDs(lock), []string{"pdf"}) {
		t.Errorf("lock = %+v, want the procedural entry and the install record", lock)
	}
}
