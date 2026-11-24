//go:build unix

package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// SH15 (T4y, `36d5bc62df38-4` F5): the bound of the evidence run is the one the runner has. The deployed script holds the
// stderr pipe through a process it started (`x=$(/bin/sleep 10)`), which ends by itself. At the 2-second timeout `sh` is
// killed, and the stop waits for the pipe up to the runner's kill grace, 2 seconds more, and no longer: it is back after
// at least 2 seconds and within 6 (the 4 of the bound and a margin of 2, twice the worst overhead measured under load),
// with one stderr line that says no answer within 2s. The sleep of 10 seconds is past the bound, so a stop that waited for
// the pipe to close would be caught. The sleep is not killed and ends by itself, up to about 6 seconds after the test.
func TestStopHookStatesTheBoundOfTheEvidenceRun(t *testing.T) {
	overlaytest.RequirePrograms(t, "/bin/sh", "/bin/sleep")
	s := newStopWorld(t)
	s.useHome(true)
	overlaytest.WriteFile(t, s.userHome, filepath.Join(".claude", "skills", "_shared", "evidence", "evidence.sh"), "x=$(/bin/sleep 10)\n")
	s.recordRangeKept(s.head, s.c4, "introduced")
	start := time.Now()

	stdout, stderr, exits := s.stop("s1", false)

	if took := time.Since(start); took < 2*time.Second || took > 6*time.Second {
		t.Errorf("the stop took %v, want it back after the 2-second timeout and within 6 seconds", took)
	}
	if stdout != "" || !equalInts(exits, []int{0}) {
		t.Errorf("stdout = %q, exits = %v; want nothing and [0]", stdout, exits)
	}
	requireOneErrorLine(t, stderr, cannotTellPrefix, "gave no answer within 2s")
	if strings.Contains(stderr, "was stopped by a signal") {
		t.Errorf("stderr = %q, want the timeout to be the reason", stderr)
	}
}
