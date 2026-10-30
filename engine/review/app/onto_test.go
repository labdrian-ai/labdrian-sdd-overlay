package app_test

// CheckOnto (own-rdd S11 a): the delivery names the integration branch, and the engine starts the chain where the branch
// was made, from the one merge base of that branch and the head, exactly as CheckChain does from a base given by hand.

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

func TestCheckOntoChecksTheChainFromTheOneMergeBase(t *testing.T) {
	g, store := landed()
	g.paths = codePaths
	reviewedFor(t, g, store, "b")
	want, err := app.New(g, store).CheckChain(evidenceDir, "b", baseSHA, headSHA)
	if err != nil {
		t.Fatalf("setup: the chain should clear: %v", err)
	}
	g.calls = nil

	got, err := app.New(g, store).CheckOnto(evidenceDir, "b", "HEAD~1", "HEAD")

	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("CheckOnto() = %+v, %v; want %+v, nil (what CheckChain answers from the merge base)", got, err, want)
	}
	prefix := []string{"Locate " + evidenceDir, "Commit HEAD~1", "Commit HEAD", "MergeBases " + baseSHA + " " + headSHA}
	if len(g.calls) < len(prefix) || !slices.Equal(g.calls[:len(prefix)], prefix) {
		t.Errorf("git calls %q; want them to start %q", g.calls, prefix)
	}
}

func TestCheckOntoRefusesNoneOrSeveralMergeBasesAndReadsNoResult(t *testing.T) {
	for _, bases := range [][]string{nil, {baseSHA, otherSHA}} {
		g, store := landed()
		g.mergeBases = map[string][]string{baseSHA + " " + headSHA: bases}

		_, err := app.New(g, store).CheckOnto(evidenceDir, "b", "HEAD~1", "HEAD")

		want := fmt.Sprintf("%s and %s do not have exactly one merge base (they have %d), "+
			"so the commits the pull request adds cannot be told", baseSHA, headSHA, len(bases))
		if !errors.Is(err, review.ErrMergeBase) || err.Error() != want {
			t.Errorf("%d merge bases: error = %v; want one that wraps review.ErrMergeBase with the text %q", len(bases), err, want)
		}
		if store.callsTo("Names ") != 0 || len(store.calls) != 0 {
			t.Errorf("%d merge bases: store calls %q; want none", len(bases), store.calls)
		}
	}
}

func TestCheckOntoSaysWhenGitCannotTellTheMergeBase(t *testing.T) {
	g, store := landed()
	cause := errors.New("git is gone")
	g.fail["MergeBases"] = cause

	_, err := app.New(g, store).CheckOnto(evidenceDir, "b", "HEAD~1", "HEAD")

	want := fmt.Sprintf("find the merge base of %s and %s: %v", baseSHA, headSHA, cause)
	if err == nil || err.Error() != want || !errors.Is(err, cause) || errors.Is(err, review.ErrMergeBase) {
		t.Errorf("error = %v; want %q wrapping the cause and not review.ErrMergeBase", err, want)
	}
	if len(store.calls) != 0 {
		t.Errorf("store calls %q; want none", store.calls)
	}
}
