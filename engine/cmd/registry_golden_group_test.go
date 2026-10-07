//go:build unix

package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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
//
// Every way out of the test, a pass, a failed wait, a reader error or a t.Fatal, ends the same way,
// in the cleanups registered right after each resource: nothing is left running, and no goroutine.
func TestADescendantOfAProgramThatHangsIsKilledAtItsDeadline(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh to stand for a program that forks")
	}
	dir := t.TempDir()
	fifo, witnessFifo := filepath.Join(dir, "alive"), filepath.Join(dir, "witness")
	pidFile, groupFile := filepath.Join(dir, "descendant.pid"), filepath.Join(dir, "program.pid")
	for _, path := range []string{fifo, witnessFifo} {
		if err := syscall.Mkfifo(path, 0o600); err != nil {
			t.Skipf("no FIFO to hold the descendant: %v", err)
		}
	}

	// The goroutines of the test, waited for last.
	var goroutines sync.WaitGroup
	t.Cleanup(func() {
		finished := make(chan struct{})
		go func() { goroutines.Wait(); close(finished) }()
		select {
		case <-finished:
		case <-time.After(descendantGoneWait):
			t.Error("a goroutine of the test was still running at the end of it")
		}
	})

	// The read end is open before the program starts, so the program's open for writing does not
	// wait for a reader, and it is read blocking: the open itself must not wait for a writer.
	// Closing it ends a read that is still waiting, so the goroutine that reads it returns.
	reader := openFIFOForReading(t, fifo)
	t.Cleanup(func() { reader.Close() })

	// The witness is a second FIFO that only the cleanup reads, held open by the program and its
	// descendant: it ends when they are gone whatever the test did on the way, which is the proof
	// that nothing was left running even when the reader above failed.
	witness := openFIFOForReading(t, witnessFifo)
	// The goroutine of the cleanup is counted here, in the same step as the cleanup is registered,
	// and not inside it, so that the wait for the goroutines needs no particular place in the order
	// of the cleanups to see it.
	goroutines.Add(1)
	t.Cleanup(func() {
		gone := make(chan struct{})
		go func() { defer goroutines.Done(); _, _ = witness.Read(make([]byte, 1)); close(gone) }()
		select {
		case <-gone:
		case <-time.After(descendantGoneWait):
			t.Error("the program or its descendant was still running at the end of the test")
		}
		// The one close: it is also what ends a read that is still waiting, so the goroutine returns.
		witness.Close()
	})

	// Kill what the program started: the descendant by its pid when the program recorded it, and
	// the program's own process group, which is the one the program was started in.
	t.Cleanup(func() {
		if pid := recordedPid(pidFile); pid > 0 {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
		if group := recordedPid(groupFile); group > 0 {
			_ = syscall.Kill(-group, syscall.SIGKILL)
		}
	})

	// The test holds a write end of its own until the program says it is ready: a read of a FIFO
	// that has no writer yet ends at once, which is the end the test must not mistake for the
	// death of the descendant.
	holder, err := os.OpenFile(fifo, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { holder.Close() })

	ctx, kill := context.WithCancel(context.Background())
	t.Cleanup(kill)

	// fd 3 is the FIFO and fd 4 the witness, open in the program before it forks, so both hold them
	// from the start. The program then writes its own pid and the descendant's to files and a line to
	// the FIFO: that line is the readiness handshake. The test waits for it once, bounded, and only
	// then kills the program, so the kill always reaches a program that holds the FIFO and has a
	// descendant: without that, the read below would end at once because no writer ever existed, and
	// the test would pass having tried nothing. A program that never says it is ready fails the test.
	script := `exec 3>"` + fifo + `" 4>"` + witnessFifo + `"; echo $$ > "` + groupFile + `"; ` +
		`sleep 3600 & echo $! > "` + pidFile + `"; echo ready >&3; wait`
	ran := make(chan error, 1)
	goroutines.Add(1)
	go func() {
		defer goroutines.Done()
		_, _, _, err := runUnder(ctx, sh, dir, nil, []string{"-c", script})
		ran <- err
	}()
	ready, ended := make(chan error, 1), make(chan error, 1)
	goroutines.Add(1)
	go func() {
		defer goroutines.Done()
		lines := bufio.NewReader(reader)
		_, err := lines.ReadString('\n')
		ready <- err
		if err == nil {
			_, err = lines.ReadByte() // ends when the last writer is gone
			ended <- err
		}
	}()

	select {
	case err := <-ready:
		if err != nil {
			t.Fatalf("the program closed the FIFO before it said it was ready: %v", err)
		}
	case <-time.After(programReadyWait):
		t.Fatal("the program did not say it held the FIFO and had started its descendant, so nothing was proved about it")
	}
	holder.Close()

	kill()
	if err := <-ran; !errors.Is(err, errRunTimedOut) {
		t.Fatalf("runUnder() = %v, want errRunTimedOut", err)
	}
	select {
	case err := <-ended:
		if err != io.EOF {
			t.Errorf("the FIFO ended with %v, want EOF: the program was to hold it open and say nothing", err)
		}
	case <-time.After(descendantGoneWait):
		t.Error("the descendant of the program still held the FIFO after the program was killed")
	}
}

// programReadyWait bounds the wait for the program to say it holds the FIFO and has started its
// descendant. A pass spends as long as the program takes to start; the bound is long so that a loaded
// machine does not fail a test that is right, and only a program that never says it is ready waits
// it out.
const programReadyWait = time.Minute

// descendantGoneWait bounds the waits for what the kill was to end: the descendant letting go of
// the FIFO, and, at the end of the test, everything it started being gone. A pass spends as long as
// the kill takes; only a descendant that survives it waits this out.
const descendantGoneWait = time.Minute

// openFIFOForReading opens the read end of a FIFO that has no writer yet, without waiting for one,
// and makes it blocking, so that a read waits for data or for the last writer to go.
func openFIFOForReading(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.SetNonblock(int(f.Fd()), false); err != nil {
		f.Close()
		t.Fatal(err)
	}
	return f
}

// recordedPid is the pid in a file the program wrote, or 0 when it wrote none.
func recordedPid(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return pid
}
