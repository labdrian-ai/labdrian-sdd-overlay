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
		r := w.run(verb, args...)
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
