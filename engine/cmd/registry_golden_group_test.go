//go:build unix

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// What the program started is killed with it at the deadline: a program that forks a process that
// outlives it (the golden files show it forking binaries) would otherwise leave that process
// holding files and the output pipes after the run reported it was done.
func TestADescendantOfAProgramThatHangsIsKilledAtItsDeadline(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh to stand for a program that forks")
	}
	if _, err := exec.LookPath("ps"); err != nil {
		t.Skip("no ps to tell whether the descendant is alive")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "descendant.pid")
	_, _, _, err = runWithin(500*time.Millisecond, sh, dir, nil, []string{"-c", "sleep 60 & echo $! > " + pidFile + "; wait"})
	if !errors.Is(err, errRunTimedOut) {
		t.Fatalf("runWithin() = %v, want errRunTimedOut", err)
	}
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("the program did not write the pid of its descendant: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	// Killed is not reaped: a zombie waits for its parent (init, in a container that has none) to
	// collect it, and is dead all the same, so it does not count as alive.
	alive := func() bool {
		out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
		state := strings.TrimSpace(string(out))
		return err == nil && state != "" && !strings.HasPrefix(state, "Z")
	}
	for deadline := time.Now().Add(5 * time.Second); alive() && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
	}
	if alive() {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Errorf("the descendant (pid %d) of the program was still alive after the program was killed at its deadline", pid)
	}
}
