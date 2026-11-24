//go:build unix

package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// SH15: an evidence script whose shell forks a grandchild that holds the pipes is stopped at the hook's 2-second timeout
// with every process it started: the stop happens with one stderr line that says no answer within 2s, the stop is back
// within 3.5 seconds (2 and a margin; about 4 without the group) and the grandchild is gone (T4x, `4b9c3789c2e0-3` F1).
func TestStopHookBoundsTheEvidenceScriptWithEveryProcessItStarted(t *testing.T) {
	s := newStopWorld(t)
	s.useHome(true)
	g := overlaytest.NewGrandchild(t)
	overlaytest.WriteFile(t, s.userHome, filepath.Join(".claude", "skills", "_shared", "evidence", "evidence.sh"), g.Body)
	s.recordRangeKept(s.head, s.c4, "introduced")
	start := time.Now()

	stdout, stderr, exits := s.stop("s1", false)

	if took := time.Since(start); took > 3500*time.Millisecond {
		t.Errorf("the stop took %v, want it back within 3.5s", took)
	}
	if stdout != "" || !equalInts(exits, []int{0}) {
		t.Errorf("stdout = %q, exits = %v; want nothing and [0]", stdout, exits)
	}
	requireOneErrorLine(t, stderr, cannotTellPrefix, "gave no answer within 2s")
	if strings.Contains(stderr, "was stopped by a signal") {
		t.Errorf("stderr = %q, want the timeout to be the reason", stderr)
	}
	g.Started(t)
	g.RequireGone(t, time.Second)
}
