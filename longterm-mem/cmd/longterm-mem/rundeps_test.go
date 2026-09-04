package main

import (
	"context"
	"errors"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
)

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
// The ineligible case is the trap, and it is why the guard cannot be a
// bare switch on Action.Kind: Writer.Promote reports an ineligible
// observation as a ZERO promote.Result, and a zero Action.Kind is
// ActionCreated, the first iota. Reading the kind alone therefore answers
// "created" for a promotion that touched no file at all.
func TestReindexAfterPromote_KeysOffTheOutcomeNotTheCall(t *testing.T) {
	page := promote.Page{Address: "c-000001", Path: "wiki/memory/c-000001.md"}

	for _, tc := range []struct {
		name        string
		result      promote.Result
		wantRebuild int
	}{
		{
			name:        "a created page rebuilds once",
			result:      promote.Result{Page: page, Action: promote.Action{Kind: promote.ActionCreated}},
			wantRebuild: 1,
		},
		{
			name:        "an updated page rebuilds once",
			result:      promote.Result{Page: page, Action: promote.Action{Kind: promote.ActionUpdated}},
			wantRebuild: 1,
		},
		{
			name:        "a local-edit refusal rebuilds nothing",
			result:      promote.Result{Page: page, Action: promote.Action{Kind: promote.ActionSkippedLocalEdit}},
			wantRebuild: 0,
		},
		{
			name:        "an ineligible no-op rebuilds nothing",
			result:      promote.Result{},
			wantRebuild: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rebuilds := 0
			err := reindexAfterPromote(context.Background(), tc.result, func(context.Context) error {
				rebuilds++
				return nil
			})
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
		promote.Result{
			Page:   promote.Page{Address: "c-000002"},
			Action: promote.Action{Kind: promote.ActionCreated},
		},
		func(context.Context) error { return boom },
	)
	if !errors.Is(err, boom) {
		t.Fatalf("reindexAfterPromote returned %v, want the rebuild's own error", err)
	}
}
