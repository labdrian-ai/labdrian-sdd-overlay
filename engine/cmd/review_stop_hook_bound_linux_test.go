//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// SH15 (T4y, `36d5bc62df38-4` F5; T4z, `c9440b24b20f-4` F1): the stop waits for the pipe of the evidence run only up to the
// runner's kill grace. The deployed script holds the stderr pipe through a subshell that waits to open a FIFO
// (`x=$(read -r _ < fifo)`), which only the test frees. At the 2-second timeout `sh` is killed; the stop is back after at
// least those 2 seconds, which load cannot shorten, and before the test freed the FIFO, with one stderr line that says no
// answer within 2s. A stop that waited for the pipe to close would return only after the release, at the 20-second backstop,
// so the flag would be set. The test frees the subshell itself, once, at its end or at the backstop, so no process it
// started outlives it. Linux only: an open of a FIFO for reading and writing never blocks there and frees a blocked reader
// (fifo(7)).
func TestStopHookStatesTheBoundOfTheEvidenceRun(t *testing.T) {
	overlaytest.RequirePrograms(t, "/bin/sh")
	s := newStopWorld(t)
	s.useHome(true)
	fifo := filepath.Join(t.TempDir(), "hold")
	if strings.Contains(fifo, "'") {
		t.Fatalf("the FIFO path %q holds a quote, which the stub cannot quote", fifo)
	}
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatalf("make a FIFO: %v", err)
	}
	var released atomic.Bool
	release := sync.OnceFunc(func() {
		released.Store(true)
		if f, err := os.OpenFile(fifo, os.O_RDWR, 0); err == nil {
			f.Close()
		}
	})
	t.Cleanup(release)
	backstop := time.AfterFunc(20*time.Second, release)
	t.Cleanup(func() { backstop.Stop() })
	overlaytest.WriteFile(t, s.userHome, filepath.Join(".claude", "skills", "_shared", "evidence", "evidence.sh"), "x=$(read -r _ < '"+fifo+"')\n")
	s.recordRangeKept(s.head, s.c4, "introduced")
	start := time.Now()

	stdout, stderr, exits := s.stop("s1", false)

	took, wasReleased := time.Since(start), released.Load()
	if took < 2*time.Second {
		t.Errorf("the stop took %v, want it back no sooner than the 2-second timeout", took)
	}
	if wasReleased {
		t.Errorf("the stop returned only after the test freed the pipe, %v in: it waited for the pipe past the runner's grace", took)
	}
	if stdout != "" || !equalInts(exits, []int{0}) {
		t.Errorf("stdout = %q, exits = %v; want nothing and [0]", stdout, exits)
	}
	requireOneErrorLine(t, stderr, cannotTellPrefix, "gave no answer within 2s")
	if strings.Contains(stderr, "was stopped by a signal") {
		t.Errorf("stderr = %q, want the timeout to be the reason", stderr)
	}
}
