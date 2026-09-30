package main

// The production wiring of the overlay lock: 'skills <verb>' reaches
// engine/skills with a locker built on engine/filelock. The lock rules themselves
// (which verb, which mode, what a busy lock does) are tested in engine/skills with
// fakes; these tests prove the real file locks are what the verbs get, in a
// temporary directory, with the wait shortened so a busy lock costs milliseconds.

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/filelock"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

const lockTestSkill = "---\n" +
	"name: one\n" +
	"description: A skill for the overlay lock wiring tests.\n" +
	"license: MIT\n" +
	"metadata:\n" +
	"  author: tester\n" +
	"  version: \"1.0\"\n" +
	"---\n" +
	"\n" +
	"## Activation Contract\n" +
	"\n" +
	"Use only in the lock wiring tests.\n"

// lockWorld is an overlay in a temporary directory: a registry with one skill, its
// manifest, and the approved skill tree.
type lockWorld struct {
	dir, registry, manifest, root string
}

func newLockWorld(t *testing.T) lockWorld {
	t.Helper()
	dir := t.TempDir()
	w := lockWorld{
		dir:      dir,
		registry: filepath.Join(dir, "skills.registry.yaml"),
		manifest: filepath.Join(dir, "overlay.manifest"),
		root:     filepath.Join(dir, "skills"),
	}
	writeFixtureFile(t, w.registry, "version: \"1\"\nskills:\n  - id: one\n    path: one\n    source:\n      type: custom\n"+
		"    install:\n      defaultScope: global\n      targets:\n        - claude\n    lifecycle:\n      updateStrategy: overlay-only\n")
	writeFixtureFile(t, w.manifest, "one/SKILL.md custom\n")
	writeFixtureFile(t, filepath.Join(w.root, "one", "SKILL.md"), lockTestSkill)
	if r := w.run("approve", "--id", "one", "--approver", "tester"); r.code != 0 {
		t.Fatalf("approve fixture: exit %d, stderr %q", r.code, r.stderr)
	}
	return w
}

func (w lockWorld) flags() []string {
	return []string{"--registry", w.registry, "--manifest", w.manifest, "--source-root", w.root}
}

type skillsRun struct {
	code           int
	stdout, stderr string
}

// run drives the production entry point for one verb. A verb that succeeds may
// return without calling exit, as a process falling off the end of main does.
func (w lockWorld) run(verb string, extra ...string) skillsRun {
	var out, errBuf bytes.Buffer
	code := 0
	runSkillsCore(verb, append(append([]string{verb}, extra...), w.flags()...), &out, &errBuf, func(c int) { code = c })
	return skillsRun{code, out.String(), errBuf.String()}
}

// shortWait shortens the wait for a taken lock for one test.
func shortWait(t *testing.T) {
	t.Helper()
	saved := skillsLockWait
	skillsLockWait = 30 * time.Millisecond
	t.Cleanup(func() { skillsLockWait = saved })
}

func TestSkillsLock_TheProductionEntryPointCreatesAndHonoursTheRegistryLock(t *testing.T) {
	shortWait(t)
	w := newLockWorld(t)
	lockFile := skills.RegistryLockPath(w.registry)

	// A writer creates the lock file beside the registry...
	if r := w.run("sync-manifest"); r.code != 0 {
		t.Fatalf("sync-manifest: exit %d, stderr %q", r.code, r.stderr)
	}
	if info, err := os.Stat(lockFile); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("no lock file at %s after a writer ran (%v)", lockFile, err)
	}

	// ...and while another process holds it, every locking verb is refused with
	// exit 2 and a retry message, and nothing is changed.
	held, err := filelock.Acquire(lockFile, filelock.Options{})
	if err != nil {
		t.Fatal(err)
	}
	before := snapshotBytes(t, w.registry, w.manifest)
	for _, verb := range []string{"sync-manifest", "remove", "validate"} {
		args := []string{}
		if verb == "remove" {
			args = []string{"one"}
		}
		start := time.Now()
		r := w.run(verb, args...)
		if waited := time.Since(start); waited > time.Second {
			t.Errorf("%s waited %v for a lock with a 30ms wait configured: the wait is not the one the entry point sets", verb, waited)
		}
		if r.code != skills.ExitBusy || r.stdout != "" || !strings.Contains(r.stderr, "in progress") || !strings.Contains(r.stderr, "retry") {
			t.Errorf("%s while the lock is held: exit %d, stdout %q, stderr %q, want exit 2, no stdout, and a retry message", verb, r.code, r.stdout, r.stderr)
		}
	}
	if after := snapshotBytes(t, w.registry, w.manifest); !reflect.DeepEqual(before, after) {
		t.Errorf("a refused verb changed files:\nbefore %v\nafter  %v", before, after)
	}

	// Released, the same verbs run again.
	held()
	if r := w.run("validate"); r.code != 0 {
		t.Errorf("validate after the lock was released: exit %d, stderr %q", r.code, r.stderr)
	}
}

func snapshotBytes(t *testing.T, paths ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		out[p] = string(b)
	}
	return out
}

// Readers share: validate runs while another reader holds the lock, and a writer
// does not.
func TestSkillsLock_ValidateSharesTheLockWithReadersAndExcludesWriters(t *testing.T) {
	shortWait(t)
	w := newLockWorld(t)
	lockFile := skills.RegistryLockPath(w.registry)
	if r := w.run("sync-manifest"); r.code != 0 {
		t.Fatalf("sync-manifest: exit %d, stderr %q", r.code, r.stderr)
	}

	reader, err := filelock.Acquire(lockFile, filelock.Options{Mode: filelock.Shared})
	if err != nil {
		t.Fatal(err)
	}
	defer reader()
	if r := w.run("validate"); r.code != 0 {
		t.Errorf("validate while a reader holds the lock: exit %d, stderr %q", r.code, r.stderr)
	}
	if r := w.run("sync-manifest"); r.code != skills.ExitBusy {
		t.Errorf("sync-manifest while a reader holds the lock: exit %d, want %d", r.code, skills.ExitBusy)
	}
}

// validate is a reader: on an overlay nobody has written to it creates no file,
// and on one it cannot write to it still runs. install is the other reader.
func TestSkillsLock_ReadersNeverCreateTheLockFileAndWorkOnAReadOnlyOverlay(t *testing.T) {
	w := newLockWorld(t)
	// The fixture's own approve was a writer and created the lock file; a tree
	// nobody has ever written to has none.
	if err := os.Remove(skills.RegistryLockPath(w.registry)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(w.dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(w.dir, 0o755) })
	if f, err := os.Create(filepath.Join(w.dir, ".probe")); err == nil {
		f.Close()
		t.Skip("directory permissions are not enforced for this process (root or an equivalent); the read-only overlay cannot be simulated")
	}

	for _, verb := range []string{"validate", "install"} {
		extra := []string{}
		if verb == "install" {
			extra = []string{"--project-id", "p"}
		}
		if r := w.run(verb, extra...); r.code != 0 {
			t.Errorf("%s on a read-only overlay: exit %d, stderr %q", verb, r.code, r.stderr)
		}
	}
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".lock") {
			t.Errorf("a reader created %s", e.Name())
		}
	}
}

// ---- the project lock ------------------------------------------------------------------
//
// project-register, project-revise, project-retire and install lock the project
// root directory itself. These tests use real directory locks in a temporary
// project: nothing is created in it for the lock, a symlinked path to it is the
// same lock, and a root that cannot be locked refuses the verb.

const projectDraft = "---\n" +
	"name: tidy-worktree\n" +
	"description: Tidy a git worktree before handing it to a reviewer.\n" +
	"license: Apache-2.0\n" +
	"metadata:\n" +
	"  author: someone\n" +
	"  version: 1.0.0\n" +
	"---\n" +
	"\n" +
	"## Activation Contract\n" +
	"\n" +
	"Use when a worktree must be handed over clean.\n"

func (w lockWorld) registerArgs(t *testing.T, root string) []string {
	t.Helper()
	draft := filepath.Join(w.dir, "drafts", "SKILL.md")
	writeFixtureFile(t, draft, projectDraft)
	return []string{"--project-root", root, "--candidate", "procedural/candidates/repeated-success/tidy-worktree", draft}
}

func TestSkillsLock_ProjectVerbsHonourARealDirectoryLockAndCreateNoFileForIt(t *testing.T) {
	shortWait(t)
	w := newLockWorld(t)
	root := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	args := w.registerArgs(t, root)

	held, err := filelock.AcquireDir(root, filelock.Options{})
	if err != nil {
		t.Fatal(err)
	}
	r := w.run("project-register", args...)
	if r.code != skills.ExitBusy || r.stdout != "" || !strings.Contains(r.stderr, "in progress") || !strings.Contains(r.stderr, "the project "+root) {
		t.Errorf("project-register while the root is locked: exit %d, stdout %q, stderr %q, want exit 2 naming the project", r.code, r.stdout, r.stderr)
	}
	if entries, _ := os.ReadDir(root); len(entries) != 0 {
		t.Errorf("a refused project-register left %d entries in the project", len(entries))
	}

	held()
	if r := w.run("project-register", args...); r.code != 0 {
		t.Fatalf("project-register after the lock was released: exit %d, stderr %q", r.code, r.stderr)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	// What the verb wrote, and nothing for the lock: the lock is on the directory.
	if want := []string{".agents", ".claude", ".labdrian"}; !reflect.DeepEqual(names, want) {
		t.Errorf("the project holds %v after project-register, want exactly %v", names, want)
	}
}

// project-status is a reader of the project: it runs while another reader holds the
// directory, and it is refused, exit 2, while a verb that writes the project holds it.
func TestSkillsLock_ProjectStatusSharesTheProjectWithReadersAndWaitsForWriters(t *testing.T) {
	shortWait(t)
	w := newLockWorld(t)
	root := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if r := w.run("project-register", w.registerArgs(t, root)...); r.code != 0 {
		t.Fatalf("project-register: exit %d, stderr %q", r.code, r.stderr)
	}
	status := []string{"--project-root", root, "tidy-worktree"}

	reader, err := filelock.AcquireDir(root, filelock.Options{Mode: filelock.Shared})
	if err != nil {
		t.Fatal(err)
	}
	if r := w.run("project-status", status...); r.code != 0 || !strings.Contains(r.stdout, "owner:agent") {
		t.Errorf("project-status beside another reader: exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
	reader()

	writer, err := filelock.AcquireDir(root, filelock.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer writer()
	if r := w.run("project-status", status...); r.code != skills.ExitBusy || r.stdout != "" || !strings.Contains(r.stderr, "in progress") {
		t.Errorf("project-status while a writer holds the project: exit %d, stdout %q, stderr %q, want exit 2 and nothing on stdout", r.code, r.stdout, r.stderr)
	}
}

// A working directory reached through a symlink is the same project: the lock is
// on the directory, whichever way it is named.
func TestSkillsLock_AProjectRootReachedThroughASymlinkIsTheSameLock(t *testing.T) {
	shortWait(t)
	w := newLockWorld(t)
	base := t.TempDir()
	root := filepath.Join(base, "project")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}

	held, err := filelock.AcquireDir(root, filelock.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer held()
	r := w.run("project-register", w.registerArgs(t, link)...)
	if r.code != skills.ExitBusy {
		t.Errorf("project-register through the symlink while the real directory is locked: exit %d, stderr %q, want exit 2", r.code, r.stderr)
	}
	// The message names the path the caller gave and the directory that is really
	// locked: the second is the one another process holds.
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"the project " + link, "lock on the directory " + real} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("stderr %q does not contain %q", r.stderr, want)
		}
	}
}

// A raw engine call with no --registry in a directory that has none must not leave
// a lock file behind in that directory: the writer refuses, and nothing was locked.
func TestSkillsLock_ARawWriterWithoutARegistryCreatesNoLockFile(t *testing.T) {
	w := newLockWorld(t)
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	here := t.TempDir()
	if err := os.Chdir(here); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	var out, errBuf bytes.Buffer
	code := 0
	// No --registry: the default is ./skills.registry.yaml, which is not here.
	runSkillsCore("sync-manifest", []string{"sync-manifest", "--manifest", w.manifest}, &out, &errBuf, func(c int) { code = c })

	if code != 1 || !strings.Contains(errBuf.String(), "nothing was locked") {
		t.Errorf("exit %d, stderr %q, want exit 1 and 'nothing was locked'", code, errBuf.String())
	}
	entries, err := os.ReadDir(here)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("the working directory holds %d entries after a refused raw call, want none (first: %s)", len(entries), entries[0].Name())
	}
}

// A root that cannot be locked (here, one that does not exist) refuses the verb:
// it is not skipped, and nothing is created to make it lockable.
func TestSkillsLock_AProjectRootThatCannotBeLockedRefusesTheVerb(t *testing.T) {
	w := newLockWorld(t)
	root := filepath.Join(t.TempDir(), "no-such-project")

	r := w.run("project-register", w.registerArgs(t, root)...)

	if r.code != 1 || strings.Contains(r.stderr, "retry") || !strings.Contains(r.stderr, "cannot take the lock on the directory") {
		t.Errorf("exit %d, stderr %q, want exit 1, 'cannot take the lock on the directory', and no retry", r.code, r.stderr)
	}
	if _, err := os.Lstat(root); err == nil {
		t.Errorf("the verb created %s", root)
	}
}

// A writer that cannot create the lock file refuses, as a failure and not as a
// busy lock: retrying would not help.
func TestSkillsLock_AWriterThatCannotCreateTheLockRefusesWithoutSayingRetry(t *testing.T) {
	w := newLockWorld(t)
	if err := os.Remove(skills.RegistryLockPath(w.registry)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(w.dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(w.dir, 0o755) })
	if f, err := os.Create(filepath.Join(w.dir, ".probe")); err == nil {
		f.Close()
		t.Skip("directory permissions are not enforced for this process (root or an equivalent)")
	}
	r := w.run("sync-manifest")
	if r.code != 1 || strings.Contains(r.stderr, "retry") || !strings.Contains(r.stderr, "cannot take the lock") {
		t.Errorf("exit %d, stderr %q, want exit 1 and 'cannot take the lock' without a retry", r.code, r.stderr)
	}
}
