package app_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// launched stores, under feature, the result of the n-th launch on head with the rows it was sent, the way
// `review record` stores it, and returns the name of the file.
func launched(t *testing.T, store *fakeResults, feature, head string, n int, rows ...string) string {
	t.Helper()
	c, err := review.NewCandidate(baseSHA, head, []byte(diffText), changedPaths)
	if err != nil {
		t.Fatalf("setup: NewCandidate() error = %v", err)
	}
	file, err := review.EncodeResult(c, rows, replyFor(t, c))
	if err != nil {
		t.Fatalf("setup: EncodeResult() error = %v", err)
	}
	name := review.ResultName(c, n)
	store.put(feature, name, file)
	return name
}

// The number of lenses a review ran is the number of rows its stored results record, one for each row of each
// launch: a second blind launch of one head and the launches of a corrected candidate add theirs. The verdict of the
// verifier, the exemption of a head and the results of another feature are not launches of this one.
func TestCountLensesAddsTheRowsOfEveryLaunchOfTheFeature(t *testing.T) {
	store := newFakeResults()
	first := launched(t, store, "own-rdd", headSHA, 1, "default", "readability")
	second := launched(t, store, "own-rdd", headSHA, 2, "risk-line")
	corrected := launched(t, store, "own-rdd", otherSHA, 1, "default", "readability", "risk-line")
	store.put("own-rdd", headSHA[:12]+"-1.verifier.json", []byte("the verdict is not a result"))
	store.put("own-rdd", headSHA[:12]+"-3.exempt.json", []byte("the exemption is not a result"))
	launched(t, store, "another-feature", headSHA, 1, "default")

	got, err := app.CountLenses(store, "/work/wt/feature", "own-rdd")

	if want := (app.LensCount{Results: 3, Lenses: 6}); err != nil || got != want {
		t.Fatalf("CountLenses() = %+v, %v; want %+v: 2 + 1 + 3 rows in 3 stored results", got, err, want)
	}
	var reads []string
	for _, call := range store.calls {
		if strings.HasPrefix(call, "Read ") {
			reads = append(reads, call)
		}
	}
	wantReads := []string{
		"Read /work/wt/feature own-rdd " + first,
		"Read /work/wt/feature own-rdd " + second,
		"Read /work/wt/feature own-rdd " + corrected,
	}
	if strings.Join(reads, "\n") != strings.Join(wantReads, "\n") {
		t.Errorf("CountLenses() read\n%s\nwant only the results of the feature, in the order of the listing:\n%s", strings.Join(reads, "\n"), strings.Join(wantReads, "\n"))
	}
	if store.created() != 0 {
		t.Errorf("CountLenses() created %d files, want 0: counting stores nothing", store.created())
	}
}

// A feature that has no stored result has no rows, and the count says so with a zero and no error: whether that is
// right for the record being checked is for its reader to say.
func TestCountLensesOfAFeatureWithNoStoredResultIsZero(t *testing.T) {
	verdictAndExemption := newFakeResults()
	verdictAndExemption.put("own-rdd", headSHA[:12]+"-1.verifier.json", []byte("not a result"))
	verdictAndExemption.put("own-rdd", headSHA[:12]+"-2.exempt.json", []byte("not a result"))
	cases := map[string]*fakeResults{
		"no directory":               newFakeResults(),
		"a verdict and an exemption": verdictAndExemption,
	}
	for name, store := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := app.CountLenses(store, "/work/wt/feature", "own-rdd")

			if err != nil || got != (app.LensCount{}) {
				t.Fatalf("CountLenses() = %+v, %v; want a count of 0 results and 0 rows and no error", got, err)
			}
			if store.callsTo("Read ") != 0 {
				t.Errorf("CountLenses() read %d files, want 0: none is a result", store.callsTo("Read "))
			}
		})
	}
}

// failingRead is a Results whose Read fails for one name and is the wrapped store for every other.
type failingRead struct {
	*fakeResults
	name string
	err  error
}

func (f failingRead) Read(root, feature, name string) ([]byte, error) {
	if name == f.name {
		return nil, f.err
	}
	return f.fakeResults.Read(root, feature, name)
}

// What cannot be counted is not counted: a result that is not one `review record` writes, a store that cannot list or
// read, and a feature that is not one safe path segment refuse the whole count, and no partial number is handed back,
// though a good result was read before the refusal.
func TestCountLensesRefusesWhatItCannotCountAndHandsBackNoNumber(t *testing.T) {
	boom := errors.New("the disk is gone")
	c, err := review.NewCandidate(baseSHA, otherSHA, []byte(diffText), changedPaths)
	if err != nil {
		t.Fatalf("setup: NewCandidate() error = %v", err)
	}
	notHolding, err := review.EncodeResult(c, []string{"default"}, replyWith(t, c.Base, c.Head, strings.Repeat("0", 64), "completed", c.Paths))
	if err != nil {
		t.Fatalf("setup: EncodeResult() error = %v", err)
	}
	good := "odd/review-results/own-rdd/" + headSHA[:12] + "-1.json"
	bad := review.ResultName(c, 1) // it sorts after the good one, so the refusal comes after a result was counted
	badPath := "odd/review-results/own-rdd/" + bad
	cases := []struct {
		name    string
		prepare func(*fakeResults)
		via     func(*fakeResults) app.Results // the store CountLenses is given, when it is not the fake itself
		feature string
		wantIs  error
		wantMsg string
	}{
		{"a result stored without rows", func(s *fakeResults) { s.put("own-rdd", bad, storedWithoutRows(t, c)) }, nil, "own-rdd", review.ErrInvalidStored, badPath},
		{"a result that is not JSON", func(s *fakeResults) { s.put("own-rdd", bad, []byte("not json")) }, nil, "own-rdd", review.ErrInvalidStored, badPath},
		{"a result whose reply does not hold to its binding", func(s *fakeResults) { s.put("own-rdd", bad, notHolding) }, nil, "own-rdd", review.ErrInvalidStored, badPath},
		{"a store that cannot list", func(s *fakeResults) { s.fail["Names"] = boom }, nil, "own-rdd", boom, "list odd/review-results/own-rdd"},
		{"a store that cannot read the first result", func(s *fakeResults) { s.fail["Read"] = boom }, nil, "own-rdd", boom, good},
		{
			"a store that cannot read a later result",
			func(s *fakeResults) { s.put("own-rdd", bad, notHolding) },
			func(s *fakeResults) app.Results { return failingRead{s, bad, boom} },
			"own-rdd", boom, badPath,
		},
		{"a feature that is a path", func(*fakeResults) {}, nil, "../own-rdd", review.ErrInvalidFeature, "../own-rdd"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeResults()
			launched(t, store, "own-rdd", headSHA, 1, "default", "readability")
			tt.prepare(store)
			store.calls = nil
			var results app.Results = store
			if tt.via != nil {
				results = tt.via(store)
			}

			got, err := app.CountLenses(results, "/work/wt/feature", tt.feature)

			if !errors.Is(err, tt.wantIs) {
				t.Fatalf("CountLenses() error = %v, want it to wrap %v", err, tt.wantIs)
			}
			if err != nil && !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("CountLenses() error = %q, want it to name %q", err, tt.wantMsg)
			}
			if got != (app.LensCount{}) {
				t.Errorf("CountLenses() = %+v, want the zero count with a refusal: a partial number is a wrong number", got)
			}
			if tt.wantIs == review.ErrInvalidFeature && len(store.calls) != 0 {
				t.Errorf("CountLenses() asked the store %v, want nothing asked before the feature is checked", store.calls)
			}
		})
	}
}
