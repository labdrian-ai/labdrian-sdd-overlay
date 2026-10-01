package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/filelock"
)

// The registry lock is bounded. A propagate that finds it held waits the bound the
// other stores use and then gives up with a busy error, instead of blocking for as
// long as the holder lives: both contract hooks run it on every prompt, so a hung
// holder would otherwise freeze every later prompt.
//
// The call runs in a goroutine and the test gives it far more than the bound, so
// an unbounded wait fails the test instead of hanging it.
func TestAcquireRegistryLockGivesUpAfterTheBound(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the 2 s bound of the registry lock")
	}
	lockPath := filepath.Join(t.TempDir(), "skill-registry.md.lock")
	held, err := filelock.Acquire(lockPath, filelock.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer held()

	type outcome struct {
		err     error
		elapsed time.Duration
	}
	done := make(chan outcome, 1)
	start := time.Now()
	go func() {
		release, err := acquireRegistryLock(lockPath)
		if err == nil {
			release()
		}
		done <- outcome{err: err, elapsed: time.Since(start)}
	}()

	select {
	case got := <-done:
		if !errors.Is(got.err, filelock.ErrBusy) {
			t.Fatalf("acquireRegistryLock on a held lock = %v, want a busy error", got.err)
		}
		if !strings.Contains(got.err.Error(), lockPath) {
			t.Errorf("error %q does not name the lock %s", got.err, lockPath)
		}
		if got.elapsed < filelock.DefaultWait-100*time.Millisecond || got.elapsed > 5*time.Second {
			t.Errorf("gave up after %v, want about the %v bound", got.elapsed, filelock.DefaultWait)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("acquireRegistryLock is still waiting 10 s after a 2 s bound: the wait is not bounded")
	}
}

// A lock that is free is taken at once, and released for the next propagate.
func TestAcquireRegistryLockTakesAFreeLockAndReleasesIt(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "skill-registry.md.lock")

	release, err := acquireRegistryLock(lockPath)
	if err != nil {
		t.Fatalf("acquireRegistryLock on a free lock: %v", err)
	}
	release()

	again, err := acquireRegistryLock(lockPath)
	if err != nil {
		t.Fatalf("acquireRegistryLock after the release: %v", err)
	}
	again()
}
