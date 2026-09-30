package skills

// install and the writers of a skill directory. `skills approve` writes its record
// through a temporary file in the skill directory (writeFileAtomic) and renames it
// into place; an install that copies the directory in that window would put the
// temporary file in the project. Two defences, tested here: install holds the
// overlay lock (shared) for the whole copy, which a writer cannot take until it is
// done, and the copier skips a writer's temporary file whatever the lock did.

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCopyTree_SkipsAWritersTemporaryFile(t *testing.T) {
	src, dst := filepath.Join(t.TempDir(), "src"), filepath.Join(t.TempDir(), "dst")
	for path, content := range map[string]string{
		"SKILL.md":            "the skill",
		"references/guide.md": "a reference",
		".gitkeep":            "",
		// In the fixture on purpose. copyTree never copies the approval record
		// (its `rel == ApprovalRecordName` rule in install.go: the record is
		// repository governance state, not skill content), so it is absent from
		// want below although its name does not carry the writer's temp prefix.
		ApprovalRecordName:                      `{"version":1}`,
		atomicTempPrefix + "123456789":          "half a record",
		"references/" + atomicTempPrefix:        "not a writer's file: a longer name is needed",
		"references/" + atomicTempPrefix + "42": "half of another write",
	} {
		writeTestFile(t, filepath.Join(src, filepath.FromSlash(path)), content)
	}

	if err := copyTree(src, dst); err != nil {
		t.Fatal(err)
	}

	var got []string
	err := filepath.WalkDir(dst, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(dst, p)
			got = append(got, filepath.ToSlash(rel))
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// A name that is exactly the prefix has no unique suffix and is not something
	// writeFileAtomic makes; it is content, and is copied. The approval record
	// and the two writer's temp files are the three fixture entries left out.
	want := []string{".gitkeep", "SKILL.md", "references/.tmp-skills-", "references/guide.md"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("copied %v, want %v", got, want)
	}
	if _, err := os.Lstat(filepath.Join(dst, ApprovalRecordName)); !os.IsNotExist(err) {
		t.Errorf("the approval record was copied (Lstat err = %v); copyTree must skip it", err)
	}
}

// The overlay of the install tests: a global skill approve can target, and a
// project skill install copies.
type installFixture struct {
	dir, reg, man, root, project string
}

func newInstallFixture(t *testing.T) installFixture {
	t.Helper()
	dir := t.TempDir()
	f := installFixture{
		dir:     dir,
		reg:     filepath.Join(dir, "skills.registry.yaml"),
		man:     filepath.Join(dir, "overlay.manifest"),
		root:    filepath.Join(dir, "skills"),
		project: filepath.Join(t.TempDir(), "project"),
	}
	regBytes, err := Serialize(buildRegistry([]struct {
		id              string
		scope           string
		allowedProjects []string
	}{
		{"glob", "global", nil},
		{"proj", "project", []string{"p"}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, f.reg, string(regBytes))
	writeTestFile(t, f.man, minimalManifest("glob", "proj"))
	for _, id := range []string{"glob", "proj"} {
		writeTestFile(t, filepath.Join(f.root, id, "SKILL.md"), lintCleanSkillMD(id))
	}
	writeValidApproval(t, f.root, "glob")
	if err := os.MkdirAll(f.project, 0o755); err != nil {
		t.Fatal(err)
	}
	saved := installCwd
	installCwd = func() (string, error) { return f.project, nil }
	t.Cleanup(func() { installCwd = saved })
	return f
}

func (f installFixture) installArgs() []string {
	return []string{"--registry", f.reg, "--source-root", f.root, "--project-id", "p"}
}

func (f installFixture) approveArgs() []string {
	return []string{"--id", "glob", "--approver", "reviewer", "--source-root", f.root, "--registry", f.reg}
}

// An approve that starts while an install is copying waits for it: the install is
// parked after it read the registry, holding the shared lock, and the approve can
// only be granted once the install has released it.
func TestAnApproveStartedDuringAnInstallWaitsForTheInstallToFinish(t *testing.T) {
	f := newInstallFixture(t)
	gate := newReadGate(t, f.reg)
	locker := &exclusionLocker{blocked: gate.release}

	installDone := make(chan coreRun, 1)
	go func() { installDone <- runAt("install", f.installArgs(), gate.readFile, nil, locker) }()
	<-gate.arrived
	approved := runAt("approve", f.approveArgs(), os.ReadFile, fixedClock(approveFixedNow), locker)
	gate.release()
	installed := <-installDone

	for name, r := range map[string]coreRun{"install": installed, "approve": approved} {
		if r.code != 0 {
			t.Errorf("%s: exit %d, stderr=%q", name, r.code, r.stderr)
		}
	}
	if _, err := os.Stat(filepath.Join(f.project, ".claude", "skills", "proj", "SKILL.md")); err != nil {
		t.Errorf("the project skill was not installed: %v", err)
	}
	events := locker.events()
	released, granted := indexOf(events, "released shared .skills.registry.yaml.lock"), indexOf(events, "granted exclusive .skills.registry.yaml.lock")
	if released < 0 || granted < 0 || released > granted {
		t.Errorf("lock events %v: the approve was granted its exclusive lock before the install released its shared one", events)
	}
}

// Two installs into different projects do not exclude each other (they only read
// the overlay), so a slow one never makes another wait.
func TestTwoInstallsIntoDifferentProjectsShareTheOverlayLock(t *testing.T) {
	f := newInstallFixture(t)
	otherProject := filepath.Join(t.TempDir(), "other")
	if err := os.MkdirAll(otherProject, 0o755); err != nil {
		t.Fatal(err)
	}
	gate := newReadGate(t, f.reg)
	blocked := false
	locker := &exclusionLocker{blocked: func() { blocked = true; gate.release() }}

	first := make(chan coreRun, 1)
	go func() { first <- runAt("install", f.installArgs(), gate.readFile, nil, locker) }()
	<-gate.arrived
	// The first install has resolved its project and is parked; the seam may move.
	installCwd = func() (string, error) { return otherProject, nil }
	second := runAt("install", f.installArgs(), os.ReadFile, nil, locker)
	gate.release()
	<-first

	if second.code != 0 || blocked {
		t.Errorf("the second install: exit %d, blocked %v (stderr %q), want it to run alongside the first", second.code, blocked, second.stderr)
	}
	if !strings.Contains(second.stdout, "installed: proj") {
		t.Errorf("the second install printed %q", second.stdout)
	}
}

// Two installs into the same project are serialized by the project lock: the second
// waits for the first, which is what keeps them from interleaving RemoveAll and the
// copy of one skill directory.
func TestTwoInstallsIntoOneProjectAreSerialized(t *testing.T) {
	f := newInstallFixture(t)
	gate := newReadGate(t, f.reg)
	locker := &exclusionLocker{blocked: gate.release}

	first := make(chan coreRun, 1)
	go func() { first <- runAt("install", f.installArgs(), gate.readFile, nil, locker) }()
	<-gate.arrived
	second := runAt("install", f.installArgs(), os.ReadFile, nil, locker)
	gate.release()
	<-first

	if second.code != 0 {
		t.Fatalf("the second install: exit %d, stderr %q", second.code, second.stderr)
	}
	events := locker.events()
	released, granted := indexOf(events, "released exclusive project"), lastIndexOf(events, "granted exclusive project")
	if released < 0 || granted < 0 || released > granted {
		t.Errorf("lock events %v: the second install was granted the project before the first released it", events)
	}
}

func lastIndexOf(events []string, want string) int {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i] == want {
			return i
		}
	}
	return -1
}

func indexOf(events []string, want string) int {
	for i, e := range events {
		if e == want {
			return i
		}
	}
	return -1
}
