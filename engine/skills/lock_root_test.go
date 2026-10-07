package skills

// What the lock layer locks, and what it says about it: install fails closed when it
// cannot name the directory it would write into, a busy message names the directory
// that was really locked, a raw call never leaves a lock file behind for a registry
// that is not there, and the busy-error walk ends on a cyclic chain.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- install needs a project directory it can lock -------------------------------------

// install writes into the working directory. When that directory cannot be resolved,
// or resolves to something that is not absolute, the verb used to run without the
// project lock (or, for a relative path, write relative to the process). It refuses
// instead, before it takes any lock, and says so. Each way of failing gives its own
// reason, so that the person is told which one it was: the error the system gave, or
// the path it returned and why that path cannot be used.
func TestInstallRefusesBeforeLockingWhenItCannotNameItsProjectDirectory(t *testing.T) {
	notAbsolute := func(path string) string { return fmt.Sprintf("%q is not an absolute path", path) }
	cases := []struct {
		name   string
		cwd    func() (string, error)
		reason string
	}{
		{"the working directory cannot be read", func() (string, error) { return "", fmt.Errorf("getwd: permission denied") }, "getwd: permission denied"},
		{"a relative path", func() (string, error) { return filepath.Join("rel", "dir"), nil }, notAbsolute(filepath.Join("rel", "dir"))},
		{"an empty path", func() (string, error) { return "", nil }, notAbsolute("")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newInstallFixture(t)
			locker := &recordingLocker{}
			// No project admits a skill: a run that goes ahead writes nothing, so a
			// failing test cannot leave files behind in the package directory.
			args := []string{"--registry", f.reg, "--source-root", f.root, "--project-id", "nobody"}

			r := runAtIn(tc.cwd, "install", args, os.ReadFile, nil, locker)

			if r.code != 1 || r.stdout != "" {
				t.Errorf("exit %d, stdout %q, want exit 1 and nothing on stdout; stderr %q", r.code, r.stdout, r.stderr)
			}
			for _, want := range []string{"skills install", "nothing was locked", tc.reason} {
				if !strings.Contains(r.stderr, want) {
					t.Errorf("stderr %q does not contain %q", r.stderr, want)
				}
			}
			for _, other := range cases {
				if other.name != tc.name && strings.Contains(r.stderr, other.reason) {
					t.Errorf("stderr %q gives the reason of %q (%q)", r.stderr, other.name, other.reason)
				}
			}
			if got := locker.log(); len(got) != 0 {
				t.Errorf("lock events = %v, want none: the verb must refuse before it locks", got)
			}
		})
	}
}

// The directory install works in is resolved once, and the verb and the lock are
// given the same answer: a second call to the seam can differ.
func TestInstallLocksTheDirectoryItInstallsInto(t *testing.T) {
	f := newInstallFixture(t)
	calls := 0
	first, second := f.project, filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(second, 0o755); err != nil {
		t.Fatal(err)
	}
	cwd := func() (string, error) {
		calls++
		if calls == 1 {
			return first, nil
		}
		return second, nil
	}
	locker := &recordingLocker{}

	r := runAtIn(cwd, "install", f.installArgs(), os.ReadFile, nil, locker)

	if r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	if _, err := os.Stat(filepath.Join(first, ".claude", "skills", "proj", "SKILL.md")); err != nil {
		t.Errorf("the skill was not installed into the directory that was locked: %v", err)
	}
	if _, err := os.Stat(filepath.Join(second, ".claude")); err == nil {
		t.Errorf("install wrote into %s, which it did not lock", second)
	}
	if calls != 1 {
		t.Errorf("the working directory was resolved %d times, want once", calls)
	}
}

// ---- the busy message names the lock that was taken ------------------------------------

// busyAtErr is a busy lock that says which path it tried: engine/filelock's
// BusyError does, and the path is the resolved directory when the caller reached it
// through a symlink.
type busyAtErr struct{ path, at string }

func (e busyAtErr) Error() string    { return "lock " + e.at + " is held by another process" }
func (busyAtErr) Busy() bool         { return true }
func (e busyAtErr) LockPath() string { return e.at }

func TestABusyMessageNamesTheResolvedDirectoryThatWasLocked(t *testing.T) {
	e := newProjectCLIEnv(t, "tidy-worktree", projectCLIRegistry)
	resolved := filepath.Join(t.TempDir(), "the-real-project")
	locker := &recordingLocker{failOn: map[string]error{e.root: busyAtErr{path: e.root, at: resolved}}}

	r := runAt("project-register", registerArgs(e), os.ReadFile, nil, locker)

	if r.code != ExitBusy {
		t.Fatalf("exit %d, stderr %q, want %d", r.code, r.stderr, ExitBusy)
	}
	for _, want := range []string{"the project " + e.root, "lock on the directory " + resolved} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("stderr %q does not contain %q", r.stderr, want)
		}
	}
	if strings.Contains(r.stderr, "lock on the directory "+e.root) {
		t.Errorf("stderr %q names the path that was asked for as the lock, not the one that was taken", r.stderr)
	}
}

// A busy error that does not say which path it tried keeps the old wording.
func TestABusyMessageWithoutAPathNamesTheRequestedLock(t *testing.T) {
	e := newProjectCLIEnv(t, "tidy-worktree", projectCLIRegistry)
	locker := &recordingLocker{failOn: map[string]error{e.root: busyErr{e.root}}}

	r := runAt("project-register", registerArgs(e), os.ReadFile, nil, locker)

	if !strings.Contains(r.stderr, "lock on the directory "+e.root) {
		t.Errorf("stderr %q does not name the lock that was asked for", r.stderr)
	}
}

// ---- a raw call leaves no lock file behind ---------------------------------------------

// chdirToATempDir makes an empty temporary directory the working directory for one
// test, where a raw call's default registry would be looked for. (testing.T.Chdir
// needs Go 1.24; the module is on 1.21.)
func chdirToATempDir(t *testing.T) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Errorf("restoring the working directory: %v", err)
		}
	})
}

// ---- the walk along a chain of wrapped errors ends -------------------------------------

// loopErr is an error whose Unwrap returns itself: a hand-written wrapper can do
// that, and nothing in the language forbids it.
type loopErr struct{}

func (e *loopErr) Error() string { return "wraps itself" }
func (e *loopErr) Unwrap() error { return e }

func TestIsBusyEndsOnAnErrorThatWrapsItself(t *testing.T) {
	if isBusy(&loopErr{}) {
		t.Error("isBusy(self-wrapping error) = true, want false")
	}
	if !isBusy(fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", busyErr{"x"}))) {
		t.Error("isBusy did not see a busy error two wraps down")
	}
}
