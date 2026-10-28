package app_test

// The facts of a commit read from every feature of the tree, for the stop hook (own-rdd T6b6a, S6 e, L66): a role record
// of any feature counts, the first feature that has a sweep owns it, and a record that does not read back or hold counts as
// absent. The tests go through FactsOfAnyFeature over the fakes, with the records the other verbs store.

import (
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// putIn stores a record in a feature and returns its name.
func putIn(store *fakeResults, feature, name string, file []byte) string {
	store.put(feature, name, file)
	return name
}

func sweptIn(t *testing.T, store *fakeResults, feature string, n int, t3 ...string) string {
	t.Helper()
	c := candidateFor(t, baseSHA, headSHA)
	file, err := review.EncodeSweep(c, review.SweepFindings{T3: t3})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return putIn(store, feature, review.SweepName(c, n), file)
}

func answeredIn(t *testing.T, store *fakeResults, feature, base, head string, n int, sweep string, commit *string) string {
	t.Helper()
	c := candidateFor(t, base, head)
	file, err := review.EncodeSweeper(c, sweep, review.SweeperReport{Commit: commit, Candidates: []review.SweeperCandidate{}})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return putIn(store, feature, review.SweeperName(c, n), file)
}

func polishedIn(t *testing.T, store *fakeResults, feature string, n int) string {
	t.Helper()
	c := candidateFor(t, baseSHA, headSHA)
	file, err := review.EncodePolisher(c, []byte(`{"findings":[]}`))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return putIn(store, feature, review.PolisherName(c, n), file)
}

// listings is the listing of each feature, in the order given.
// It is generic so that the unexported listing type is never named here.
func listings[T any](t *testing.T, store *fakeResults, listing func(string, []string) T, features ...string) []T {
	t.Helper()
	var all []T
	for _, f := range features {
		names, err := store.Names(fakeRepo.Root, f)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		all = append(all, listing(f, names))
	}
	return all
}

func factsOf(t *testing.T, g *fakeGit, store *fakeResults, commit string, features ...string) (review.Facts, string, error) {
	t.Helper()
	return app.New(g, store).FactsOfAnyFeature(fakeRepo, commit, listings(t, store, app.NewFeatureListing, features...)...)
}

func TestFactsOfAnyFeatureCountsASweepOfTheSecondFeatureWhenTheFirstHasNone(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	g.paths = []string{"engine/a.go"}
	store.put("first", "unrelated.json", []byte("{}"))
	name := sweptIn(t, store, "second", 1)

	facts, parent, err := factsOf(t, g, store, headSHA, "first", "second")

	if err != nil || parent != baseSHA {
		t.Fatalf("FactsOfAnyFeature() = %v, %q, %v; want the first parent and no error", facts, parent, err)
	}
	if want := review.ResultPath("second", name); facts.Sweep != want || facts.SweepFired {
		t.Errorf("Sweep, SweepFired = %q, %v; want %q, false", facts.Sweep, facts.SweepFired, want)
	}
}

func TestFactsOfAnyFeatureTakesTheSweepOfTheFirstFeatureThatHasOne(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	g.paths = []string{"engine/a.go"}
	clean := sweptIn(t, store, "first", 1)
	sweptIn(t, store, "second", 2, "a line of the working tree")

	facts, _, err := factsOf(t, g, store, headSHA, "first", "second")

	if err != nil {
		t.Fatalf("FactsOfAnyFeature() error = %v", err)
	}
	if want := review.ResultPath("first", clean); facts.Sweep != want || facts.SweepFired || facts.Answered {
		t.Errorf("Sweep, SweepFired, Answered = %q, %v, %v; want %q, false, false", facts.Sweep, facts.SweepFired, facts.Answered, want)
	}
}

func TestFactsOfAnyFeatureAnswersAFiredSweepOnlyFromTheFeatureOfTheSweep(t *testing.T) {
	for _, tc := range []struct {
		name         string
		sweeperIn    string
		wantAnswered bool
	}{
		{"a sweeper record of the feature of the sweep", "first", true},
		{"a sweeper record of another feature", "second", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := newFakeGit(), newFakeResults()
			g.paths = []string{"engine/a.go"}
			fired := sweptIn(t, store, "first", 1, "a line of the working tree")
			store.put("second", "x.json", []byte("{}"))
			answeredIn(t, store, tc.sweeperIn, baseSHA, headSHA, 1, fired, nil)

			facts, _, err := factsOf(t, g, store, headSHA, "first", "second")

			if err != nil || !facts.SweepFired || facts.Answered != tc.wantAnswered {
				t.Errorf("SweepFired, Answered, err = %v, %v, %v; want true, %v, nil", facts.SweepFired, facts.Answered, err, tc.wantAnswered)
			}
		})
	}
}

func TestFactsOfAnyFeatureCountsAPolisherRecordOfAFeatureThatHasNoSweep(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	g.paths = []string{"engine/a.go"}
	g.declares(headSHA, "3")
	sweptIn(t, store, "first", 1)
	polishedIn(t, store, "second", 1)

	facts, _, err := factsOf(t, g, store, headSHA, "first", "second")

	if err != nil || !facts.Polished {
		t.Errorf("Polished, err = %v, %v; want true, nil", facts.Polished, err)
	}
}

func TestFactsOfAnyFeatureMakesACommitASweepCommitWhenASweeperRecordOfAnyFeatureNamesIt(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	g.paths = []string{"engine/a.go"}
	g.parents[otherSHA] = headSHA
	g.messages[otherSHA] = "chore(sweep): remove dead code\n\nRisk: none"
	g.trailers[g.messages[otherSHA]] = []review.Trailer{{Key: "Risk", Value: "none"}}
	store.put("first", "x.json", []byte("{}"))
	sweep := sweptIn(t, store, "second", 1)
	commit := otherSHA
	answeredIn(t, store, "second", baseSHA, headSHA, 1, sweep, &commit)

	facts, parent, err := factsOf(t, g, store, otherSHA, "first", "second")

	if err != nil || parent != headSHA || !facts.SweepCommit {
		t.Errorf("SweepCommit, parent, err = %v, %q, %v; want true, %q, nil", facts.SweepCommit, parent, err, headSHA)
	}
}

func TestFactsOfAnyFeatureCountsARecordThatDoesNotDecodeOrHoldAsAbsent(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	g.paths = []string{"engine/a.go"}
	c := candidateFor(t, baseSHA, headSHA)
	store.put("first", review.SweepName(c, 1), []byte("not json"))
	other, err := review.NewCandidate(baseSHA, headSHA, []byte("diff --git a/b.go b/b.go\n"), g.paths)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	file, err := review.EncodeSweep(other, review.SweepFindings{})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	store.put("second", review.SweepName(c, 2), file)

	facts, _, err := factsOf(t, g, store, headSHA, "first", "second")

	if err != nil || facts.Sweep != "" || facts.SweepFired {
		t.Errorf("Sweep, SweepFired, err = %q, %v, %v; want no sweep and a nil error", facts.Sweep, facts.SweepFired, err)
	}
}

func TestFactsOfAnyFeatureReadsNoRecordOfAPassiveCommit(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	g.paths = []string{"odd/tasks/own-rdd.md"}
	sweptIn(t, store, "first", 1)

	facts, _, err := factsOf(t, g, store, headSHA, "first")

	if err != nil || !facts.Passive {
		t.Fatalf("Passive, err = %v, %v; want true, nil", facts.Passive, err)
	}
	if n := store.callsTo("Read "); n != 0 {
		t.Errorf("the store was read %d times, want no record read", n)
	}
}

func TestFactsOfAnyFeatureFailsWhenGitCannotReadTheMessage(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	g.fail["Message"] = errors.New("boom")
	store.put("first", "x.json", []byte("{}"))

	_, _, err := factsOf(t, g, store, headSHA, "first")

	if err == nil || !strings.Contains(err.Error(), "read the message of "+headSHA) {
		t.Errorf("FactsOfAnyFeature() error = %v, want one naming the message of the commit", err)
	}
}
