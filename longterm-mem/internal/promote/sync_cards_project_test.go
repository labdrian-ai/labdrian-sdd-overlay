package promote

import (
	"errors"
	"fmt"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// PlanCards names what SyncCards would promote and writes nothing, and its counts are the run's own.
func TestPlanCards_PredictsTheRunAndWritesNothing(t *testing.T) {
	vaultRoot := t.TempDir()
	w := syncCardsWriter(vaultRoot)
	bridged := eligibleCard(13, 1)
	bridged.SameContentEngramIDs = []int64{99}
	engramPageIn(t, vaultRoot, PrecedenceStore{}, 99, "c-000050")
	ineligible := memory.CardMemory{ID: 14, Project: "memorycard", Title: "Plain", Body: "Not curated."}
	cards := &fakeCards{memories: []memory.CardMemory{eligibleCard(11, 1), eligibleCard(12, 1), bridged, ineligible}}

	plan, err := PlanCards(t.Context(), cards, w, "memorycard")
	if err != nil {
		t.Fatalf("PlanCards: %v", err)
	}
	if plan.WouldPromote != 2 || len(plan.Titles) != 2 || plan.Titles[0] != "Card 11" || plan.Titles[1] != "Card 12" || plan.Bridged != 1 || plan.Skipped != 1 {
		t.Fatalf("plan = %+v, want two titled, one bridged, one skipped", plan)
	}
	before := pageFiles(t, vaultRoot)
	if len(before) != 1 {
		t.Fatalf("PlanCards wrote pages: %v", before)
	}

	report, err := SyncCards(t.Context(), cards, w, "memorycard")
	if err != nil {
		t.Fatalf("SyncCards: %v", err)
	}
	if len(report.Promoted) != plan.WouldPromote || report.Bridged != plan.Bridged || report.Skipped != plan.Skipped {
		t.Errorf("run %+v disagrees with plan %+v", report, plan)
	}
}

// A listing that fails is no plan.
func TestPlanCards_AListingFailureIsAnError(t *testing.T) {
	cards := &fakeCards{err: errors.New("store gone")}
	if _, err := PlanCards(t.Context(), cards, syncCardsWriter(t.TempDir()), "memorycard"); err == nil {
		t.Fatal("PlanCards answered no error for a listing that failed")
	}
}

// SyncCardsProject rebuilds the index once and records the sync after its pass, as Sync does.
func TestSyncCardsProject_RebuildsTheIndexAndRecordsTheSync(t *testing.T) {
	vaultRoot := t.TempDir()
	w := syncCardsWriter(vaultRoot)
	state := &memSyncState{}
	w.SyncState = state
	index := &countingRebuilder{}
	cards := &fakeCards{memories: []memory.CardMemory{eligibleCard(11, 1)}}

	report, err := SyncCardsProject(t.Context(), cards, w, index, "memorycard")
	if err != nil || report.Failure != NotFailed {
		t.Fatalf("SyncCardsProject = %+v, %v, want a clean run", report, err)
	}
	if len(report.Promoted) != 1 || index.calls != 1 || len(state.recorded) != 1 {
		t.Errorf("promoted %d, rebuilds %d, records %d, want 1 of each", len(report.Promoted), index.calls, len(state.recorded))
	}
}

// A memorycard that cannot list is an unavailable memory, and neither the index nor the sync-state is touched.
func TestSyncCardsProject_AnUnavailableStoreStopsBeforeTheRebuild(t *testing.T) {
	w := syncCardsWriter(t.TempDir())
	state := &memSyncState{}
	w.SyncState = state
	index := &countingRebuilder{}

	report, err := SyncCardsProject(t.Context(), &fakeCards{err: errors.New("store gone")}, w, index, "memorycard")
	if err == nil || report.Failure != MemoryUnavailable {
		t.Fatalf("err = %v, failure = %v, want MemoryUnavailable", err, report.Failure)
	}
	if index.calls != 0 || len(state.recorded) != 0 {
		t.Errorf("rebuilds %d, records %d, want none", index.calls, len(state.recorded))
	}
}

// An index that cannot be rebuilt is the one failure that names the vault's tooling.
func TestSyncCardsProject_AFailedRebuildIsNamed(t *testing.T) {
	w := syncCardsWriter(t.TempDir())
	index := &countingRebuilder{err: errors.New("script crashed")}
	report, err := SyncCardsProject(t.Context(), &fakeCards{memories: []memory.CardMemory{eligibleCard(11, 1)}}, w, index, "memorycard")
	var rebuild *IndexRebuildError
	if !errors.As(err, &rebuild) || report.Failure != IndexRebuildFailed {
		t.Fatalf("err = %v, failure = %v, want IndexRebuildFailed", err, report.Failure)
	}
	if len(report.Promoted) != 1 {
		t.Errorf("what landed is still reported: %+v", report)
	}
}

// One memory that fails does not stop the rebuild or the record: the run did everything else.
func TestSyncCardsProject_AFailedMemoryStillRebuildsAndRecords(t *testing.T) {
	vaultRoot := t.TempDir()
	w := syncCardsWriter(vaultRoot)
	w.Pages = failingPages{PageRepository: w.Pages}
	state := &memSyncState{}
	w.SyncState = state
	index := &countingRebuilder{}

	report, err := SyncCardsProject(t.Context(), &fakeCards{memories: []memory.CardMemory{eligibleCard(11, 1)}}, w, index, "memorycard")
	if err == nil || report.Failure != OtherFailure || len(report.Failed) != 1 {
		t.Fatalf("err = %v, report = %+v, want OtherFailure with one failed memory", err, report)
	}
	if index.calls != 1 || len(state.recorded) != 1 {
		t.Errorf("rebuilds %d, records %d, want 1 of each", index.calls, len(state.recorded))
	}
}

// CardRunFailure is the one classification of a pass that could not run, shared by the run (SyncCardsProject) and
// the command's dry run: an unlistable memorycard, wrapped or not, is MemoryUnavailable; anything else is OtherFailure.
func TestCardRunFailure_ClassifiesAPassThatCouldNotRun(t *testing.T) {
	listed := &CardListError{Project: "p", Err: errors.New("no such table")}
	for name, tc := range map[string]struct {
		err  error
		want Failure
	}{
		"unlistable":         {listed, MemoryUnavailable},
		"unlistable wrapped": {fmt.Errorf("outer: %w", listed), MemoryUnavailable},
		"anything else":      {errors.New("writer cannot be built"), OtherFailure},
	} {
		if got := CardRunFailure(tc.err); got != tc.want {
			t.Errorf("%s: CardRunFailure = %v, want %v", name, got, tc.want)
		}
	}
}
