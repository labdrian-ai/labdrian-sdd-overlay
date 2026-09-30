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
	"strings"
	"testing"
)

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
	// Restored explicitly here, so no later test depends on the order of this
	// test's cleanups (newInstallFixture also restores the value it saved).
	fixtureCwd := installCwd
	installCwd = func() (string, error) { return otherProject, nil }
	t.Cleanup(func() { installCwd = fixtureCwd })
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
	if dir, err := installCwd(); err != nil || dir != f.project {
		t.Fatalf("installCwd() = %q, %v; want this test's project %q (a seam leaked from another test)", dir, err, f.project)
	}
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

// Two installs of different skills into one project read the project lock and write
// it back. Without the project lock both read "no lock yet" and the later write
// drops the earlier record, so the first skill's files sit in the project with nothing
// recording that install owns them, and the next install would call them foreign.
func TestConcurrentInstallsIntoOneProjectKeepBothRecords(t *testing.T) {
	dir := t.TempDir()
	reg, root := filepath.Join(dir, "skills.registry.yaml"), filepath.Join(dir, "skills")
	regBytes, err := Serialize(buildRegistry([]struct {
		id              string
		scope           string
		allowedProjects []string
	}{
		{"one", "project", []string{"p1"}},
		{"two", "project", []string{"p2"}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, reg, string(regBytes))
	for _, id := range []string{"one", "two"} {
		writeTestFile(t, filepath.Join(root, id, "SKILL.md"), lintCleanSkillMD(id))
	}
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	saved := installCwd
	installCwd = func() (string, error) { return project, nil }
	t.Cleanup(func() { installCwd = saved })

	gate := newReadGate(t, filepath.Join(project, filepath.FromSlash(ProjectLockRelPath)))
	locker := &exclusionLocker{blocked: gate.release}
	install := func(projectID string) func() coreRun {
		return func() coreRun {
			return runAt("install", []string{"--registry", reg, "--source-root", root, "--project-id", projectID}, gate.readFile, nil, locker)
		}
	}

	results := runConcurrently(install("p1"), install("p2"))

	for i, r := range results {
		if r.code != 0 {
			t.Errorf("install #%d: exit %d, stderr %q", i, r.code, r.stderr)
		}
	}
	data, err := os.ReadFile(filepath.Join(project, filepath.FromSlash(ProjectLockRelPath)))
	if err != nil {
		t.Fatal(err)
	}
	lock, err := ParseProjectLock(data)
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
