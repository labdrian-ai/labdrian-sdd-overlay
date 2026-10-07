//go:build unix

package main

import (
	"bufio"
	"context"
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

	// The test holds a write end of its own until the program says it is ready: a read of a FIFO
	// that has no writer yet ends at once, which is the end the test must not mistake for the
	// death of the descendant.
	holder, err := os.OpenFile(fifo, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()

	// fd 3 is the FIFO, open in the program before it forks, so both hold it from the start. The
	// program then writes the pid of the descendant to a file and a line to the FIFO: that line is
	// the readiness handshake. The test waits for it once, bounded, and only then kills the
	// program, so the kill always reaches a program that holds the FIFO and has a descendant:
	// without that, the read below would end at once because no writer ever existed, and the test
	// would pass having tried nothing. A program that never says it is ready fails the test.
	script := `exec 3>"` + fifo + `"; sleep 3600 & echo $! > "` + pidFile + `"; echo ready >&3; wait`
	ctx, kill := context.WithCancel(context.Background())
	defer kill()
	ran := make(chan error, 1)
	go func() {
		_, _, _, err := runUnder(ctx, sh, dir, nil, []string{"-c", script})
		ran <- err
	}()
	ready := make(chan error, 1)
	go func() {
		_, err := bufio.NewReader(reader).ReadString('\n')
		ready <- err
	}()
	select {
	case err := <-ready:
		if err != nil {
			t.Fatalf("the program closed the FIFO before it said it was ready: %v", err)
		}
	case <-time.After(readyWait):
		kill()
		<-ran
		t.Fatal("the program did not say it held the FIFO and had started its descendant, so nothing was proved about it")
	}
	holder.Close()
	// Whatever happens next, no descendant is left running.
	pid := descendantPid(t, pidFile)
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })

	kill()
	if err := <-ran; !errors.Is(err, errRunTimedOut) {
		t.Fatalf("runUnder() = %v, want errRunTimedOut", err)
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
	case <-time.After(readyWait):
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Error("the descendant of the program still held the FIFO after the program was killed")
	}
}

// readyWait bounds a wait that a pass never spends: the program saying it is ready, and the
// descendant being gone once the program was killed. It is long so that a loaded machine does not
// fail a test that is right, and it is only ever waited out by a failure.
const readyWait = time.Minute

// descendantPid is the pid the program wrote before it said it was ready.
func descendantPid(t *testing.T, pidFile string) int {
	t.Helper()
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("the program said it was ready but wrote no pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	return pid
}
