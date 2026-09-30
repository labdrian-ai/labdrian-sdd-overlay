package skills

// The project lock: install, project-register, project-revise, and project-retire
// read the project's lock file (.labdrian/procedural-skills.lock.json), decide, and
// write it back with the skill files, so two of them on one project root lose an
// update unless they are serialized. They take an exclusive lock on the project
// root directory itself, per root. Nothing here creates a file in a project.

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func projectLockFile(root string) string {
	return filepath.Join(root, filepath.FromSlash(ProjectLockRelPath))
}

// secondRegisterArgs are the arguments that register another skill, id, in the
// same project as e.
func secondRegisterArgs(t *testing.T, e projectCLIEnv, id string) []string {
	t.Helper()
	draft := filepath.Join(filepath.Dir(e.draftPath), "second", id, "SKILL.md")
	writeTestFile(t, draft, string(validDraft(id)))
	return []string{"--project-root", e.root, "--candidate", "procedural/candidates/repeated-success/" + id, "--registry", e.registryPath, draft}
}

func projectLockIDs(t *testing.T, root string) []string {
	t.Helper()
	data, err := os.ReadFile(projectLockFile(root))
	if err != nil {
		t.Fatalf("read the project lock: %v", err)
	}
	lock, err := ParseProjectLock(data)
	if err != nil {
		t.Fatalf("the project lock does not parse: %v\n%s", err, data)
	}
	var ids []string
	for _, e := range lock.Skills {
		ids = append(ids, e.ID)
	}
	sort.Strings(ids)
	return ids
}

// ---- which verbs lock which directory -------------------------------------------------

func TestSkillsCoreAt_ProjectVerbsLockTheProjectRootExclusively(t *testing.T) {
	for _, verb := range []string{"project-register", "project-revise", "project-retire"} {
		t.Run(verb, func(t *testing.T) {
			root := t.TempDir()
			locker := &recordingLocker{}
			// The verb itself will refuse the missing draft or id; the lock is taken
			// before it looks at anything, and released whatever it decides.
			runAt(verb, []string{"--project-root", root, "--dry-run"}, os.ReadFile, nil, locker)
			want := []string{"lockdir exclusive " + root, "unlock " + root}
			if got := locker.log(); !reflect.DeepEqual(got, want) {
				t.Errorf("lock events = %v, want %v", got, want)
			}
		})
	}
}

// The verbs refuse a project root that is missing, relative, or not a value, before
// they read or write anything; the lock does not get ahead of that refusal, so a
// mistyped command never locks (or fails to lock) some other directory.
func TestSkillsCoreAt_NoProjectLockWithoutAUsableProjectRoot(t *testing.T) {
	abs := t.TempDir()
	for name, args := range map[string][]string{
		"no flag":                       {"--dry-run"},
		"flag without a value":          {"--project-root"},
		"an empty value":                {"--project-root", ""},
		"a relative root":               {"--project-root", filepath.Join("rel", "dir")},
		"a flag where the value goes":   {"--project-root", "--dry-run"},
		"only after the end of options": {"--", "--project-root", abs},
	} {
		t.Run(name, func(t *testing.T) {
			locker := &recordingLocker{}
			runAt("project-register", args, os.ReadFile, nil, locker)
			if got := locker.log(); len(got) != 0 {
				t.Errorf("took locks %v, want none", got)
			}
		})
	}
}

// Several --project-root flags: the last one is the root the verb uses, so it is
// the one that is locked.
func TestSkillsCoreAt_TheLastProjectRootIsTheOneLocked(t *testing.T) {
	first, last := t.TempDir(), t.TempDir()
	locker := &recordingLocker{}
	runAt("project-register", []string{"--project-root", first, "--project-root", last}, os.ReadFile, nil, locker)
	if got := locker.log(); len(got) == 0 || got[0] != "lockdir exclusive "+last {
		t.Errorf("lock events = %v, want the last root %s locked", got, last)
	}
}

// project-status only reads and reports; it takes no lock.
func TestSkillsCoreAt_ProjectStatusTakesNoLock(t *testing.T) {
	locker := &recordingLocker{}
	runAt("project-status", []string{"--project-root", t.TempDir()}, os.ReadFile, nil, locker)
	if got := locker.log(); len(got) != 0 {
		t.Errorf("project-status took locks %v, want none", got)
	}
}

// The lock order, pinned so that later work on install inherits it: the overlay
// lock first, then the project lock, and released in the opposite order. Two verbs
// that took them the other way round could each hold one and wait for the other;
// with bounded waits that would be exit 2 for both, not a hang, but it would be a
// failure nobody could retry their way out of.
func TestInstallTakesTheOverlayLockBeforeTheProjectLockAndReleasesThemInReverse(t *testing.T) {
	f := newInstallFixture(t)
	locker := &recordingLocker{}

	r := runAt("install", f.installArgs(), os.ReadFile, nil, locker)
	if r.code != 0 {
		t.Fatalf("exit %d, stderr=%q", r.code, r.stderr)
	}

	overlay := RegistryLockPath(f.reg)
	want := []string{
		"lock shared " + overlay,
		"lockdir exclusive " + f.project,
		"unlock " + f.project,
		"unlock " + overlay,
	}
	if got := locker.log(); !reflect.DeepEqual(got, want) {
		t.Errorf("lock events = %v, want %v", got, want)
	}
}

// The order is a property of the planner, not of one verb: whatever a verb asks
// for, every file (overlay) lock comes before every directory (project) lock.
func TestLockRequestsAreAlwaysOverlayBeforeProject(t *testing.T) {
	f := newInstallFixture(t)
	root := t.TempDir()
	cases := map[string][]string{
		"add": nil, "remove": nil, "sync-manifest": nil, "approve": nil, "validate": nil,
		"install":          f.installArgs(),
		"project-register": {"--project-root", root},
		"project-revise":   {"--project-root", root},
		"project-retire":   {"--project-root", root},
		"project-status":   {"--project-root", root},
		"list":             nil, "status": nil, "lint": nil,
	}
	multi := 0
	for verb, args := range cases {
		seenDir := false
		requests := lockRequestsFor(verb, args)
		for _, req := range requests {
			if req.dir {
				seenDir = true
			} else if seenDir {
				t.Errorf("%s asks for the overlay lock %s after a project lock: %v", verb, req.path, requests)
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

// ---- busy and failed project locks ------------------------------------------------------

func TestSkillsCoreAt_ABusyProjectLockExits2AndTheProjectIsUntouched(t *testing.T) {
	e := newProjectCLIEnv(t, "tidy-worktree", projectCLIRegistry)
	locker := &recordingLocker{failOn: map[string]error{e.root: busyErr{e.root}}}

	r := runAt("project-register", registerArgs(e), os.ReadFile, nil, locker)

	if r.code != ExitBusy || r.stdout != "" {
		t.Errorf("exit %d, stdout %q, want exit 2 and nothing on stdout; stderr %q", r.code, r.stdout, r.stderr)
	}
	for _, want := range []string{"skills project-register", "in progress", "retry", "the project " + e.root} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("stderr %q does not contain %q", r.stderr, want)
		}
	}
	e.assertNothingWritten(t)
}

// When install gets the overlay lock and then cannot get the project's, it lets go
// of the overlay's, says the project is busy, and installs nothing.
func TestInstallReleasesTheOverlayLockWhenTheProjectLockIsBusy(t *testing.T) {
	f := newInstallFixture(t)
	overlay := RegistryLockPath(f.reg)
	locker := &recordingLocker{failOn: map[string]error{f.project: busyErr{f.project}}}

	r := runAt("install", f.installArgs(), os.ReadFile, nil, locker)

	if r.code != ExitBusy || !strings.Contains(r.stderr, "the project "+f.project) {
		t.Errorf("exit %d, stderr %q, want exit 2 naming the project", r.code, r.stderr)
	}
	want := []string{"lock shared " + overlay, "refused exclusive " + f.project, "unlock " + overlay}
	if got := locker.log(); !reflect.DeepEqual(got, want) {
		t.Errorf("lock events = %v, want %v: the overlay lock must not be left held", got, want)
	}
	if _, err := os.Stat(filepath.Join(f.project, ".claude")); err == nil {
		t.Error("install wrote into the project without its lock")
	}
}

// A directory that cannot be locked at all (a filesystem that refuses) is a refusal,
// exit 1, not a busy lock and not a verb that runs unlocked.
func TestSkillsCoreAt_AProjectRootThatCannotBeLockedRefusesTheVerb(t *testing.T) {
	e := newProjectCLIEnv(t, "tidy-worktree", projectCLIRegistry)
	cause := "filelock: lock " + e.root + ": operation not supported (this filesystem may not support locking a directory; nothing was locked)"
	locker := &recordingLocker{failOn: map[string]error{e.root: errString(cause)}}

	r := runAt("project-register", registerArgs(e), os.ReadFile, nil, locker)

	if r.code != 1 || r.stdout != "" {
		t.Errorf("exit %d, stdout %q, want exit 1 and nothing on stdout", r.code, r.stdout)
	}
	for _, want := range []string{"cannot take the lock", "may not support locking a directory"} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("stderr %q does not contain %q", r.stderr, want)
		}
	}
	if strings.Contains(r.stderr, "retry") {
		t.Errorf("stderr %q tells the caller to retry a failure that will not clear", r.stderr)
	}
	e.assertNothingWritten(t)
}

type errString string

func (e errString) Error() string { return string(e) }

// ---- the interleavings the project lock exists to prevent ---------------------------------

// Two registrations in one project that both read the project lock before either
// wrote it used to leave one skill unregistered (its files written, its entry
// gone) although both exited 0.
func TestConcurrentProjectRegistrationsBothLandInTheProjectLock(t *testing.T) {
	e := newProjectCLIEnv(t, "tidy-worktree", projectCLIRegistry)
	second := secondRegisterArgs(t, e, "tidy-repo")
	gate := newReadGate(t, projectLockFile(e.root))
	locker := &exclusionLocker{blocked: gate.release}

	results := runConcurrently(
		func() coreRun { return runAt("project-register", registerArgs(e), gate.readFile, nil, locker) },
		func() coreRun { return runAt("project-register", second, gate.readFile, nil, locker) },
	)

	for i, r := range results {
		if r.code != 0 {
			t.Errorf("registration #%d: exit %d, stderr=%q", i, r.code, r.stderr)
		}
	}
	if got, want := projectLockIDs(t, e.root), []string{"tidy-repo", "tidy-worktree"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the project lock lists %v, want %v: a registration that exited 0 was lost", got, want)
	}
	for _, id := range []string{"tidy-worktree", "tidy-repo"} {
		for _, dir := range []string{".claude", ".agents"} {
			if _, err := os.Stat(filepath.Join(e.root, dir, "skills", id, "SKILL.md")); err != nil {
				t.Errorf("%s/skills/%s/SKILL.md: %v", dir, id, err)
			}
		}
	}
}

// Two retirements read the same two-entry lock, and each wrote back the lock
// without its own skill: the last write brought the other skill's entry back.
func TestConcurrentProjectRetirementsBothLeaveTheProjectLock(t *testing.T) {
	e := newProjectCLIEnv(t, "tidy-worktree", projectCLIRegistry)
	for _, args := range [][]string{registerArgs(e), secondRegisterArgs(t, e, "tidy-repo")} {
		if r := runAt("project-register", args, os.ReadFile, nil, noopLocker{}); r.code != 0 {
			t.Fatalf("setup registration: exit %d, stderr=%q", r.code, r.stderr)
		}
	}
	gate := newReadGate(t, projectLockFile(e.root))
	locker := &exclusionLocker{blocked: gate.release}
	retire := func(id string) func() coreRun {
		return func() coreRun {
			return runAt("project-retire", projectRetireArgs(e, id, "--reason", "human-request"), gate.readFile, nil, locker)
		}
	}

	results := runConcurrently(retire("tidy-worktree"), retire("tidy-repo"))

	for i, r := range results {
		if r.code != 0 {
			t.Errorf("retirement #%d: exit %d, stderr=%q", i, r.code, r.stderr)
		}
	}
	if got := projectLockIDs(t, e.root); len(got) != 0 {
		t.Errorf("the project lock still lists %v: a retirement that exited 0 was undone", got)
	}
}

// The lock is per project root: two projects do not wait for each other.
func TestProjectLocksOfDifferentRootsDoNotExcludeEachOther(t *testing.T) {
	one := newProjectCLIEnv(t, "tidy-worktree", projectCLIRegistry)
	two := newProjectCLIEnv(t, "tidy-worktree", projectCLIRegistry)
	gate := newReadGate(t, projectLockFile(one.root))
	blocked := false
	locker := &exclusionLocker{blocked: func() { blocked = true; gate.release() }}

	firstDone := make(chan coreRun, 1)
	go func() { firstDone <- runAt("project-register", registerArgs(one), gate.readFile, nil, locker) }()
	<-gate.arrived
	other := runAt("project-register", registerArgs(two), os.ReadFile, nil, locker)
	gate.release()
	first := <-firstDone

	if other.code != 0 || first.code != 0 || blocked {
		t.Errorf("exits %d and %d, blocked %v (stderr %q, %q), want both to run without waiting for each other", first.code, other.code, blocked, first.stderr, other.stderr)
	}
}
