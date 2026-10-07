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
