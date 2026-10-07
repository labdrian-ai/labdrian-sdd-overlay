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
//
// The test does not poll for the descendant to die. The program opens a FIFO for writing and
// hands the open descriptor to the descendant, and the test holds the read end: a read of a FIFO
// ends (EOF) exactly when its last writer is gone, killed or not reaped, so the read returns when
// the program and its descendant are dead and not before. A descendant that outlives the deadline
// keeps the FIFO open, the read does not return, and the bound on the wait turns that into the
// failure it is.
func TestADescendantOfAProgramThatHangsIsKilledAtItsDeadline(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh to stand for a program that forks")
	}
	dir := t.TempDir()
	fifo := filepath.Join(dir, "alive")
	pidFile := filepath.Join(dir, "descendant.pid")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("no FIFO to hold the descendant: %v", err)
	}
	// The read end is open before the program starts, so the program's open for writing does not
	// wait for a reader, and it is read blocking: the open itself must not wait for a writer.
	reader, err := os.OpenFile(fifo, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := syscall.SetNonblock(int(reader.Fd()), false); err != nil {
		t.Fatal(err)
	}

	// fd 3 is the FIFO, open in the program before it forks, so both hold it from the start. The
	// pid file is written after that, so that it exists is the proof that the program got as far as
	// holding the FIFO and starting the descendant before it was killed: without it the read below
	// would end at once because no writer ever existed, and the test would pass having tried
	// nothing. A program that is slow to start (a loaded machine) is given a longer deadline, and
	// one that never starts it fails the test.
	script := `exec 3>"` + fifo + `"; sleep 3600 & echo $! > "` + pidFile + `"; wait`
	started := false
	for _, deadline := range []time.Duration{500 * time.Millisecond, 2 * time.Second, 8 * time.Second} {
		_ = os.Remove(pidFile)
		_, _, _, err = runWithin(deadline, sh, dir, nil, []string{"-c", script})
		if !errors.Is(err, errRunTimedOut) {
			t.Fatalf("runWithin() = %v, want errRunTimedOut", err)
		}
		if _, statErr := os.Stat(pidFile); statErr == nil {
			started = true
			break
		}
	}
	if !started {
		t.Fatal("the program did not start its descendant before its deadline, so nothing was proved about it")
	}

	gone := make(chan error, 1)
	go func() {
		_, err := reader.Read(make([]byte, 1))
		gone <- err
	}()
	select {
	case err := <-gone:
		if err == nil {
			t.Errorf("the program wrote to the FIFO: it was to hold it open and say nothing")
		}
	case <-time.After(2 * time.Minute):
		if data, err := os.ReadFile(pidFile); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
		t.Error("the descendant of the program still held the FIFO two minutes after the program was killed at its deadline")
	}
}
