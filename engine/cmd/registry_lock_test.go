package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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

// The same promise as seen from outside, which is the way the contract hooks meet
// it (spec R-005, "A lock that stays held is refused after the bound"): the built
// binary, run as its own process against a registry whose lock another holder
// keeps, exits 1 after about the bound with the busy refusal naming the lock, and
// leaves the registry as it was. The process runs under a deadline well past the
// bound, so an unbounded wait fails the test instead of hanging it.
//
// Safety: the binary runs with HOME and XDG_STATE_HOME on this test's own
// temporary directories and a registry inside another; nothing real is touched.
func TestPropagateRefusesAHeldRegistryLockAfterTheBound(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the engine binary and waits out the 2 s bound of the registry lock")
	}
	home, stateHome := phase6IsolatedHome(t)
	binary := phase6BuildEngineBinary(t)

	project := t.TempDir()
	registry := filepath.Join(project, "skill-registry.md")
	lockPath := registry + ".lock"
	const original = "# Skill Registry\n\nnothing to propagate into\n"
	if err := os.WriteFile(registry, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	held, err := filelock.Acquire(lockPath, filelock.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer held()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "propagate", "--registry", registry)
	cmd.Dir = project
	cmd.Env = []string{"HOME=" + home, "XDG_STATE_HOME=" + stateHome, "PATH=" + os.Getenv("PATH")}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	start := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(start)

	if ctx.Err() != nil {
		t.Fatalf("propagate was still waiting 15 s after a 2 s bound: the wait is not bounded\nstderr: %s", stderr.String())
	}
	var exit *exec.ExitError
	if !errors.As(runErr, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("propagate on a held lock: err = %v, want exit code 1\nstderr: %s", runErr, stderr.String())
	}
	if elapsed < filelock.DefaultWait-100*time.Millisecond || elapsed > 8*time.Second {
		t.Errorf("gave up after %v, want about the %v bound", elapsed, filelock.DefaultWait)
	}
	for _, want := range []string{"acquiring registry lock", lockPath, "held by another process"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr %q does not contain %q", stderr.String(), want)
		}
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing: the refusal happens before any work", stdout.String())
	}
	if got, err := os.ReadFile(registry); err != nil || string(got) != original {
		t.Errorf("the registry changed under a lock it did not hold: %q (%v)", got, err)
	}
	entries, err := os.ReadDir(project)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if want := []string{"skill-registry.md", "skill-registry.md.lock"}; !reflect.DeepEqual(names, want) {
		t.Errorf("project directory holds %v, want exactly %v", names, want)
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
