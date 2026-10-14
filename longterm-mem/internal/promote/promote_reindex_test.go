package promote

import (
	"context"
	"errors"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// lookupOf is an ObservationLookup over a fixed set of observations by id.
func lookupOf(observations ...memory.Observation) ObservationLookup {
	return func(id int64) (memory.Observation, bool, error) {
		for _, obs := range observations {
			if obs.ID == id {
				return obs, true, nil
			}
		}
		return memory.Observation{}, false, nil
	}
}

// A promotion that wrote a page rebuilds the index once before it returns, so the page is found by the next query
// and not only after somebody syncs out of band; the outcome carries the result of the promotion.
func TestPromoteAndReindex_RebuildsTheIndexAfterAPageWasWritten(t *testing.T) {
	rebuilder := &countingRebuilder{}

	outcome, err := PromoteAndReindex(t.Context(), readyWriter(t), lookupOf(pageObservation(501)), 501, rebuilder)

	if err != nil {
		t.Fatalf("PromoteAndReindex: %v", err)
	}
	if outcome.Result.Action.Kind != ActionCreated {
		t.Errorf("action = %v, want ActionCreated", outcome.Result.Action.Kind)
	}
	if rebuilder.calls != 1 || outcome.IndexRebuildErr != nil {
		t.Errorf("the index was rebuilt %d times with failure %v, want once and none", rebuilder.calls, outcome.IndexRebuildErr)
	}
}

// The page is written and durable by the time the rebuild runs, so the rebuild's failure is not the promotion's:
// the call succeeds, the page is in the result, and the failure sits beside it.
func TestPromoteAndReindex_ARebuildFailureIsBesideASuccessfulPromotion(t *testing.T) {
	boom := errors.New("scripts offline")

	outcome, err := PromoteAndReindex(t.Context(), readyWriter(t), lookupOf(pageObservation(502)), 502, &countingRebuilder{err: boom})

	if err != nil {
		t.Fatalf("PromoteAndReindex = %v, want the promotion to have succeeded", err)
	}
	if outcome.Result.Action.Kind != ActionCreated || outcome.Result.Page.Address == "" {
		t.Errorf("result = %+v, want the page that was written", outcome.Result)
	}
	if !errors.Is(outcome.IndexRebuildErr, boom) {
		t.Errorf("IndexRebuildErr = %v, want the rebuild's own failure", outcome.IndexRebuildErr)
	}
}

// A promotion that did not happen is an error and rebuilds nothing: not found, and a failure of the lookup.
func TestPromoteAndReindex_APromotionThatDidNotHappenRebuildsNothing(t *testing.T) {
	broken := errors.New("memory offline")
	for name, tc := range map[string]struct {
		lookup ObservationLookup
		want   error
	}{
		"an observation that does not exist": {lookupOf(), ErrObservationNotFound},
		"a lookup that fails":                {func(int64) (memory.Observation, bool, error) { return memory.Observation{}, false, broken }, broken},
	} {
		t.Run(name, func(t *testing.T) {
			rebuilder := &countingRebuilder{}

			_, err := PromoteAndReindex(t.Context(), readyWriter(t), tc.lookup, 9, rebuilder)

			if !errors.Is(err, tc.want) {
				t.Errorf("PromoteAndReindex = %v, want %v", err, tc.want)
			}
			if rebuilder.calls != 0 {
				t.Errorf("the index was rebuilt %d times for a promotion that did not happen", rebuilder.calls)
			}
		})
	}
}

// With no rebuilder wired, or one that holds a nil pointer, the promotion is made and nothing is rebuilt, as Sync
// does.
func TestPromoteAndReindex_WithoutARebuilderPromotesAndRebuildsNothing(t *testing.T) {
	var typedNil *countingRebuilder
	for name, rebuilder := range map[string]IndexRebuilder{"none": nil, "a nil pointer": typedNil} {
		t.Run(name, func(t *testing.T) {
			outcome, err := PromoteAndReindex(t.Context(), readyWriter(t), lookupOf(pageObservation(503)), 503, rebuilder)

			if err != nil || outcome.IndexRebuildErr != nil || outcome.Result.Action.Kind != ActionCreated {
				t.Errorf("PromoteAndReindex = %+v, %v; want the page created and no failure", outcome, err)
			}
		})
	}
}

// TestReindexAfterPromote_KeysOffTheOutcomeNotTheCall pins the limit the
// MCP promote rebuild was given: reindexing is keyed off what the
// promotion DID, never off "promote was called".
//
// A rebuild walks the whole vault and shells out to the vault's own
// tooling. Paying that for a promotion that wrote nothing -- a refusal
// under local-edit precedence, or an ineligible observation that is a true
// no-op -- is the entire cost for none of the benefit, on a surface (MCP)
// a client can call in a loop.
//
// The ineligible case is the one to watch: Writer.Promote reports an
// ineligible observation as a ZERO promote.Result. The zero Action.Kind is
// ActionNone, so reading the kind alone answers "nothing was written" for a
// promotion that touched no file. It used to be ActionCreated, the first
// iota, and a guard on the page address stood in front of the switch.
func TestReindexAfterPromote_KeysOffTheOutcomeNotTheCall(t *testing.T) {
	page := Page{Address: "c-000001", Path: "wiki/memory/c-000001.md"}

	for _, tc := range []struct {
		name        string
		result      Result
		wantRebuild int
	}{
		{
			name:        "a created page rebuilds once",
			result:      Result{Page: page, Action: Action{Kind: ActionCreated}},
			wantRebuild: 1,
		},
		{
			name:        "an updated page rebuilds once",
			result:      Result{Page: page, Action: Action{Kind: ActionUpdated}},
			wantRebuild: 1,
		},
		{
			name:        "a local-edit refusal rebuilds nothing",
			result:      Result{Page: page, Action: Action{Kind: ActionSkippedLocalEdit}},
			wantRebuild: 0,
		},
		{
			name:        "an observation left alone for an unreadable page rebuilds nothing",
			result:      Result{Action: Action{Kind: ActionSkippedUnreadablePages}},
			wantRebuild: 0,
		},
		{
			name:        "an ineligible no-op rebuilds nothing",
			result:      Result{},
			wantRebuild: 0,
		},
		{
			// The kind says nothing was done; a page address beside it does not turn that into a write.
			name:        "an unset action rebuilds nothing, whatever page it sits beside",
			result:      Result{Page: page},
			wantRebuild: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rebuilds := 0
			err := reindexAfterPromote(context.Background(), tc.result, IndexRebuilderFunc(func(context.Context) error {
				rebuilds++
				return nil
			}))
			if err != nil {
				t.Fatalf("reindexAfterPromote: %v", err)
			}
			if rebuilds != tc.wantRebuild {
				t.Fatalf("rebuild ran %d time(s), want %d", rebuilds, tc.wantRebuild)
			}
		})
	}
}

// TestReindexAfterPromote_ReportsARebuildFailureWithoutHidingIt: the page
// is already written and durable when the rebuild runs, so a rebuild
// failure is not a promotion failure -- but it is not nothing either, and
// swallowing it would leave a caller believing a page it cannot find by
// query was never written.
func TestReindexAfterPromote_ReportsARebuildFailureWithoutHidingIt(t *testing.T) {
	boom := errors.New("vault rebuild script exited 1")
	err := reindexAfterPromote(context.Background(),
		Result{
			Page:   Page{Address: "c-000002"},
			Action: Action{Kind: ActionCreated},
		},
		IndexRebuilderFunc(func(context.Context) error { return boom }),
	)
	if !errors.Is(err, boom) {
		t.Fatalf("reindexAfterPromote returned %v, want the rebuild's own error", err)
	}
}
