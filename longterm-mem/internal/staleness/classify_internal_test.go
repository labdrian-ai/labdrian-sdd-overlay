package staleness

import (
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/repohistory"
)

// A path with history whose last change was neither a delete nor a rename
// -- a merge commit is the ordinary way there -- comes back as unknown WITH
// a date. The date must not be enough to report it: only an observed
// deletion is evidence of removal, and inferring one from "something
// happened, later" is exactly how a live memory gets marked dead.
func TestFindings_UnknownIsNeverAReportedRemovalEvenWhenRecent(t *testing.T) {
	obs := []engram.Observation{{ID: 1, Title: "t", UpdatedAt: "2026-01-01 00:00:00"}}
	facts := map[string]repohistory.PathFact{
		"a/b.go": {
			Path:   "a/b.go",
			State:  repohistory.StateUnknown,
			Commit: "deadbeef",
			At:     time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), // long after
		},
	}

	got := findings(obs, map[int64][]string{1: {"a/b.go"}}, facts)
	if len(got) != 0 {
		t.Fatalf("an unknown path was reported as a removal on a date alone: %+v", got)
	}
}
