package promote

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// SyncProject runs the two passes of a sync, promotion and propagation, and hands back what each did.
func TestSyncProject_RunsBothPassesAndHandsBackBothReports(t *testing.T) {
	w := readyWriter(t)
	mem := &fakeMemory{
		live: []memory.Observation{{ID: 7, Type: "decision", Title: "Pinned", Content: "Body.", Project: pagesProject, RevisionCount: 1, Pinned: true}},
		all:  []memory.Observation{{ID: 7, Type: "decision", Title: "Pinned", Content: "Body.", Project: pagesProject, RevisionCount: 1, Pinned: true}},
	}

	report, err := SyncProject(t.Context(), Deps{Memory: mem, Writer: w}, pagesProject)

	if err != nil {
		t.Fatalf("SyncProject: %v", err)
	}
	if len(report.Sync.Promoted) != 1 {
		t.Errorf("Sync.Promoted = %v, want the pinned observation promoted", report.Sync.Promoted)
	}
	if len(mem.history) != 1 {
		t.Errorf("the history was read %d times, want once: the second pass ran", len(mem.history))
	}
	if report.Failure != NotFailed {
		t.Errorf("Failure = %v, want NotFailed", report.Failure)
	}
}

// Sync's failures are per observation, by design: it steps over a broken one rather than letting it wedge the
// run, so returning at its error would wedge the second pass behind it, and the statuses it carries would never
// land again for the project, identically on every retry. Both passes always run, and the error of the run is the
// errors of the two together.
func TestSyncProject_TheSecondPassRunsWhenTheFirstFails(t *testing.T) {
	boom := errors.New("scripts offline")
	w := readyWriter(t)
	mem := &fakeMemory{}

	report, err := SyncProject(t.Context(), Deps{Memory: mem, Writer: w, Index: IndexRebuilderFunc(func(context.Context) error { return boom })}, pagesProject)

	if !errors.Is(err, boom) {
		t.Fatalf("SyncProject = %v, want the rebuild's failure", err)
	}
	if len(mem.history) != 1 {
		t.Errorf("the history was read %d times, want once: the second pass ran after the first failed", len(mem.history))
	}
	if report.Failure != IndexRebuildFailed {
		t.Errorf("Failure = %v, want IndexRebuildFailed", report.Failure)
	}
}

func TestSyncProject_TheFailuresOfBothPassesAreOneError(t *testing.T) {
	syncBoom, propagateBoom := errors.New("state offline"), errors.New("history offline")
	w := readyWriter(t)
	_, _, syncState := registration(w)
	syncState.err = syncBoom
	mem := &fakeMemory{historyErr: propagateBoom}

	_, err := SyncProject(t.Context(), Deps{Memory: mem, Writer: w}, pagesProject)

	if !errors.Is(err, syncBoom) || !errors.Is(err, propagateBoom) {
		t.Fatalf("SyncProject = %v, want the failures of both passes", err)
	}
	lines := strings.Split(err.Error(), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "promote: sync: ") || !strings.HasPrefix(lines[1], "promote: propagate: ") {
		t.Errorf("the error reads %q, want the sync's line and then the propagation's", err)
	}
}

// What failed decides the code a command answers with, and it is told by what the run found out, not by the words
// of an error: the index rebuild first (it is the one step that runs the vault's own scripts), then a memory that
// can no longer answer, then anything else.
func TestSyncProject_SaysWhyARunFailed(t *testing.T) {
	stateBoom := errors.New("state offline")
	rebuildBoom := errors.New("scripts offline")
	tests := []struct {
		name    string
		memory  *fakeMemory
		rebuild error
		state   error
		want    Failure
	}{
		{"nothing failed", &fakeMemory{}, nil, nil, NotFailed},
		{"the index could not be rebuilt", &fakeMemory{}, rebuildBoom, nil, IndexRebuildFailed},
		{"the index could not be rebuilt and the memory cannot answer its relations either: the rebuild is named", &fakeMemory{edgesErr: errors.New("gone")}, rebuildBoom, nil, IndexRebuildFailed},
		{"the memory cannot list", &fakeMemory{listErr: errors.New("no such table: observations")}, nil, nil, MemoryUnavailable},
		{"the memory cannot read its relations, and a step after the listing failed", &fakeMemory{edgesErr: errors.New("no such table: memory_relations")}, nil, stateBoom, MemoryUnavailable},
		{"the memory answers and a step failed that is not the index", &fakeMemory{}, nil, stateBoom, OtherFailure},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := readyWriter(t)
			_, _, syncState := registration(w)
			syncState.err = tc.state
			deps := Deps{Memory: tc.memory, Writer: w}
			if tc.rebuild != nil {
				deps.Index = IndexRebuilderFunc(func(context.Context) error { return tc.rebuild })
			}

			report, err := SyncProject(t.Context(), deps, pagesProject)

			if (err != nil) != (tc.want != NotFailed) {
				t.Fatalf("SyncProject error = %v, want a failure only when the run failed", err)
			}
			if report.Failure != tc.want {
				t.Errorf("Failure = %v, want %v (error: %v)", report.Failure, tc.want, err)
			}
		})
	}
}

// A run that went well pays nothing for the question of whether the memory can answer: it is asked only after
// a failure, of the two things a sync reads, and a relation id no row can carry costs nothing.
func TestSyncProject_AHealthyRunDoesNotProbeTheMemory(t *testing.T) {
	mem := &fakeMemory{}

	if _, err := SyncProject(t.Context(), Deps{Memory: mem, Writer: readyWriter(t)}, pagesProject); err != nil {
		t.Fatalf("SyncProject: %v", err)
	}

	if len(mem.listed) != 1 || len(mem.edgesFor) != 0 {
		t.Errorf("the memory was listed %d times and asked for relations of %v, want one listing (the sync's) and no probe", len(mem.listed), mem.edgesFor)
	}
}

// The probe asks both questions, because a sync reads both: the listing the promotion pass makes and the relations
// the propagation pass reads. One of them answering is not the memory answering.
func TestSyncProject_TheProbeAsksTheListingAndTheRelations(t *testing.T) {
	mem := &fakeMemory{}
	w := readyWriter(t)
	_, _, syncState := registration(w)
	syncState.err = errors.New("state offline")

	if _, err := SyncProject(t.Context(), Deps{Memory: mem, Writer: w}, pagesProject); err == nil {
		t.Fatal("SyncProject succeeded, want the failure that triggers the probe")
	}

	if len(mem.listed) != 2 {
		t.Errorf("the memory was listed %d times, want the sync's listing and the probe's", len(mem.listed))
	}
	if len(mem.edgesFor) != 1 || mem.edgesFor[0] != 0 {
		t.Errorf("relations were asked for %v, want one probe for an observation id no row can carry", mem.edgesFor)
	}
}

// The failure of the rebuild is a type of its own, found without reading its words; its words stay what they have
// always been, because they are what an operator reads.
func TestSync_TheRebuildFailureIsATypeOfItsOwnWithTheWordsItAlwaysHad(t *testing.T) {
	boom := errors.New("scripts offline")

	_, err := Sync(t.Context(), Deps{Memory: &fakeMemory{}, Writer: readyWriter(t), Index: IndexRebuilderFunc(func(context.Context) error { return boom })}, pagesProject)

	var rebuild *IndexRebuildError
	if !errors.As(err, &rebuild) {
		t.Fatalf("Sync = %v, want an *IndexRebuildError", err)
	}
	if !errors.Is(err, boom) {
		t.Errorf("Sync = %v, want the rebuild's own failure inside", err)
	}
	if want := "promote: sync: rebuild index: scripts offline"; err.Error() != want {
		t.Errorf("the error reads %q, want %q", err, want)
	}
}
