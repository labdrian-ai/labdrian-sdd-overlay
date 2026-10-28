package app_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewverdict"
)

// verified stores the verdict of the reply beside the result, as `review verify` does, with a git of its own so
// that the calls of the service under test stay its own.
func verified(t *testing.T, store *fakeResults, result string, reply []byte) {
	t.Helper()
	if _, err := app.New(newFakeGit(), store).Verify(evidenceDir, "own-rdd", result, reply); err != nil {
		t.Fatalf("setup: Verify() error = %v", err)
	}
}

// A head is cleared by the stored result that binds it. The candidate is recomputed from git, for the commits the
// result names and not from anything the file says about its diff: git is asked what the head is, then for the
// diff of the range from the base the result names to the head, and the stored binding must be equal to what it
// gave. The store is listed and read, the verdict beside the result is looked for, and nothing is stored.
func TestCheckClearsTheHeadOfAStoredCleanResultAndRecomputesTheCandidateFromGit(t *testing.T) {
	g, store := checkable(t)
	reviewed(t, store, baseSHA, headSHA, 1)

	got, err := app.New(g, store).Check(evidenceDir, "own-rdd", "HEAD")

	want := app.Checked{Head: headSHA, Results: []review.Reviewed{{
		Candidate: candidateOf(t, g), Result: wantFirstPath, Decision: reviewverdict.Decision{Outcome: reviewverdict.Clear},
	}}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Check() = %+v, %v; want %+v and no error", got, err, want)
	}
	// The head is read first (its first parent, its message and trailers, and, as it declares Risk: none, its paths
	// and what it deletes: it is not passive), then the sweep that binds it, and then the result.
	bind := []string{"Commit " + baseSHA, "Commit " + headSHA, "Diff " + baseSHA + " " + headSHA, "Paths " + baseSHA + " " + headSHA}
	wantGit := append([]string{
		"Locate " + evidenceDir, "Commit HEAD",
		"Parent " + headSHA, "Message " + headSHA, "Trailers", "Paths " + baseSHA + " " + headSHA, "DeletedOrRenamed " + baseSHA + " " + headSHA,
	}, bind...)
	wantGit = append(append(wantGit, "Parent "+headSHA), bind...)
	if !reflect.DeepEqual(g.calls, wantGit) {
		t.Errorf("git calls = %q, want %q", g.calls, wantGit)
	}
	wantStore := []string{
		"Names " + evidenceDir + " own-rdd",
		"Read " + evidenceDir + " own-rdd 222222222222-7.sweep.json",
		"Read " + evidenceDir + " own-rdd 222222222222-1.json",
		"Read " + evidenceDir + " own-rdd 222222222222-1.verifier.json",
	}
	if !reflect.DeepEqual(store.calls, wantStore) {
		t.Errorf("store calls = %q, want %q", store.calls, wantStore)
	}
}

// When the verifier ran, its verdict is read beside the result, held to the result it answers, and decides: a
// severe finding it did not reproduce does not block, a WARNING is to be fixed and a SUGGESTION carried. The
// answer names the verdict that was used.
func TestCheckDecidesFromTheVerdictStoredBesideTheResult(t *testing.T) {
	g, store := checkable(t)
	name := reviewed(t, store, baseSHA, headSHA, 1, finding("F1", "BLOCKER"), finding("F2", "WARNING"), finding("F3", "SUGGESTION"))
	verified(t, store, name, verdictReply(verdict("F1", "not-reproduced"), verdict("F2", "not-run"), verdict("F3", "not-run")))

	got, err := app.New(g, store).Check(evidenceDir, "own-rdd", "HEAD")

	want := app.Checked{Head: headSHA, Results: []review.Reviewed{{
		Candidate: candidateOf(t, g), Result: wantFirstPath, Verdict: wantVerdict,
		Decision: reviewverdict.Decision{Outcome: reviewverdict.Clear, Fix: []string{"F2"}, Carry: []string{"F3"}},
	}}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Check() = %+v, %v; want %+v and no error", got, err, want)
	}
}

// Every result that binds the head counts, and they are answered in the order of their launches: the tenth after
// the ninth, not after the first, whatever order the store lists them in.
func TestCheckAnswersEveryResultOfTheHeadInTheOrderOfTheirLaunches(t *testing.T) {
	g, store := checkable(t)
	reviewed(t, store, baseSHA, headSHA, 10)
	reviewed(t, store, baseSHA, headSHA, 2)

	got, err := app.New(g, store).Check(evidenceDir, "own-rdd", "HEAD")

	var names []string
	for _, r := range got.Results {
		names = append(names, r.Result)
	}
	want := []string{"odd/review-results/own-rdd/222222222222-2.json", "odd/review-results/own-rdd/222222222222-10.json"}
	if err != nil || !reflect.DeepEqual(names, want) {
		t.Errorf("Check() = results %q, %v; want %q and no error", names, err, want)
	}
}

// Everything that keeps a head from delivery is a refusal that names its reason, answers nothing and stores
// nothing: no result of the head, a result that no longer binds it (the diff, or the paths, are not the ones git
// gives now), a stored file that is no longer a result, a reply that no longer holds to its binding, a verdict that
// no longer holds to its result, a result that blocks, a severe finding nobody checked, and one blocking result
// among several. Git or the store failing is a refusal as well: what cannot be checked is not cleared.
func TestCheckRefusesTheHeadAndStoresNothingWhenNoStoredResultClearsIt(t *testing.T) {
	boom := errors.New("boom")
	collision := "222222222222" + strings.Repeat("3", 28) // another commit whose name begins like the head's
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, g *fakeGit, store *fakeResults)
		want  []error // the reasons the error wraps
		say   []string
	}{
		{"the head was never reviewed", func(*testing.T, *fakeGit, *fakeResults) {}, []error{review.ErrNoResult}, []string{headSHA}},
		{"the stored result is about another head", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			reviewed(t, store, baseSHA, otherSHA, 1)
		}, []error{review.ErrNoResult}, nil},
		{"the name of the result says the head but its binding is another commit", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			c, err := review.NewCandidate(baseSHA, collision, []byte(diffText), changedPaths)
			if err != nil {
				t.Fatalf("setup: %v", err)
			}
			file, err := review.EncodeResult(c, []string{"default"}, replyFor(t, c))
			if err != nil {
				t.Fatalf("setup: %v", err)
			}
			store.put("own-rdd", "222222222222-1.json", file)
		}, []error{review.ErrNoResult}, nil},
		{"the diff of the range is no longer the one the reviewer read", func(t *testing.T, g *fakeGit, store *fakeResults) {
			reviewed(t, store, baseSHA, headSHA, 1)
			g.diff = []byte("a diff the reviewer never read\n")
			resweep(t, g, store) // the sweep binds the diff git gives now; the result does not
		}, []error{review.ErrBindingDiffers}, []string{wantFirstPath, "diff_sha256"}},
		{"git lists other paths for the range", func(t *testing.T, g *fakeGit, store *fakeResults) {
			reviewed(t, store, baseSHA, headSHA, 1)
			g.paths = changedPaths[:1]
		}, []error{review.ErrBindingDiffers}, []string{wantFirstPath, "paths"}},
		{"the stored file is no longer a result", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			store.put("own-rdd", "222222222222-1.json", []byte("{}"))
		}, []error{review.ErrInvalidStored}, []string{wantFirstPath}},
		{"the stored result records no rows (it was stored before the rows were recorded)", func(t *testing.T, g *fakeGit, store *fakeResults) {
			store.put("own-rdd", "222222222222-1.json", storedWithoutRows(t, candidateOf(t, g)))
		}, []error{review.ErrInvalidStored}, []string{wantFirstPath, "rows"}},
		{"the reply of the stored result no longer holds to its binding", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			c, err := review.NewCandidate(baseSHA, headSHA, []byte(diffText), changedPaths)
			if err != nil {
				t.Fatalf("setup: %v", err)
			}
			file, err := review.EncodeResult(c, []string{"default"}, replyWith(t, baseSHA, otherSHA, c.DiffSHA256, "completed", c.Paths))
			if err != nil {
				t.Fatalf("setup: %v", err)
			}
			store.put("own-rdd", "222222222222-1.json", file)
		}, []error{review.ErrInvalidStored, reviewresult.ErrCandidateMismatch}, []string{wantFirstPath}},
		{"the result blocks", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			verified(t, store, reviewed(t, store, baseSHA, headSHA, 1, finding("F1", "BLOCKER")), blocking)
		}, []error{review.ErrBlocked}, []string{wantFirstPath, wantVerdict, "F1"}},
		{"a severe finding nobody checked", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			reviewed(t, store, baseSHA, headSHA, 1, finding("F1", "BLOCKER"))
		}, []error{review.ErrNotClear}, []string{wantFirstPath, "no verdict", "F1"}},
		{"a severe finding the verifier could not check", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			verified(t, store, reviewed(t, store, baseSHA, headSHA, 1, finding("F1", "BLOCKER")), verdictReply(verdict("F1", "not-run")))
		}, []error{review.ErrNotClear}, []string{wantFirstPath, wantVerdict, "F1"}},
		{"the verdict no longer holds to the result", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			reviewed(t, store, baseSHA, headSHA, 1, finding("F1", "BLOCKER"))
			store.put("own-rdd", "222222222222-1.verifier.json", verdictReply(verdict("F9", "reproduced")))
		}, []error{reviewverdict.ErrUnknownID}, []string{wantVerdict}},
		{"the first launch is clear and the second blocks", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			reviewed(t, store, baseSHA, headSHA, 1)
			verified(t, store, reviewed(t, store, baseSHA, headSHA, 2, finding("F1", "BLOCKER")), blocking)
		}, []error{review.ErrBlocked}, []string{wantSecondPath, wantSecondVerdict}},
		{"the first launch blocks and the second is clear", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			verified(t, store, reviewed(t, store, baseSHA, headSHA, 1, finding("F1", "BLOCKER")), blocking)
			reviewed(t, store, baseSHA, headSHA, 2)
		}, []error{review.ErrBlocked}, []string{wantFirstPath, wantVerdict}},
		{"git cannot say what the head is", func(t *testing.T, g *fakeGit, _ *fakeResults) {
			g.fail["Commit HEAD"] = boom
		}, []error{boom}, []string{`resolve head "HEAD"`}},
		{"git cannot give the diff of the range", func(t *testing.T, g *fakeGit, store *fakeResults) {
			reviewed(t, store, baseSHA, headSHA, 1)
			g.fail["Diff"] = boom
		}, []error{boom}, []string{recordPath("222222222222-7.sweep.json"), "diff"}}, // the sweep is read before the result
		{"git does not know the base the result names", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			reviewed(t, store, fourSHA, headSHA, 1)
		}, nil, []string{wantFirstPath, "resolve base"}},
		{"the store cannot list the results of the feature", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			store.fail["Names"] = boom
		}, []error{boom}, []string{"list odd/review-results/own-rdd"}},
		{"the store cannot read the result", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			reviewed(t, store, baseSHA, headSHA, 1)
			store.fail["Read 222222222222-1.json"] = boom
		}, []error{boom}, []string{wantFirstPath}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := checkable(t)
			tc.setup(t, g, store)
			created := store.created()

			got, err := app.New(g, store).Check(evidenceDir, "own-rdd", "HEAD")

			if err == nil {
				t.Fatalf("Check() = %+v and no error, want a refusal", got)
			}
			for _, reason := range tc.want {
				if !errors.Is(err, reason) {
					t.Errorf("Check() error = %v, want one that wraps %v", err, reason)
				}
			}
			for _, s := range tc.say {
				if !strings.Contains(err.Error(), s) {
					t.Errorf("message %q does not say %q", err, s)
				}
			}
			if !reflect.DeepEqual(got, app.Checked{}) || store.created() != created {
				t.Errorf("Check() answered %+v and the store was written %d times, want nothing answered and nothing stored", got, store.created()-created)
			}
		})
	}
}

// A verdict that cannot be read is not the same as no verdict: only a verdict that is not there means the verifier
// did not run. If an unreadable verdict were read as no verdict, a result whose block the verifier reproduced
// would be decided again without it.
func TestCheckRefusesAVerdictItCannotReadInsteadOfTakingItForNoVerdict(t *testing.T) {
	g, store := checkable(t)
	verified(t, store, reviewed(t, store, baseSHA, headSHA, 1), verdictReply())
	boom := errors.New("boom")

	got, err := app.New(g, unreadable{store, "222222222222-1.verifier.json", boom}).Check(evidenceDir, "own-rdd", "HEAD")

	if !errors.Is(err, boom) || !strings.Contains(err.Error(), wantVerdict) || !reflect.DeepEqual(got, app.Checked{}) {
		t.Errorf("Check() = %+v, %v; want a refusal that wraps the failure and names %s", got, err, wantVerdict)
	}
}

// unreadable is a Results that fails to read one name with err, whatever the store holds under it.
type unreadable struct {
	*fakeResults
	name string
	err  error
}

func (u unreadable) Read(root, feature, name string) ([]byte, error) {
	if name == u.name {
		return nil, u.err
	}
	return u.fakeResults.Read(root, feature, name)
}

// The feature and the head are checked before git or the store is asked anything: a feature that is not one path
// segment and a head git would read as an option never reach them.
func TestCheckRefusesWhatGitAndThePathsMustNotReceiveBeforeAskingAnything(t *testing.T) {
	for _, tc := range []struct {
		name, feature, head string
		want                error
	}{
		{"a feature that leads out of the results directory", "../own-rdd", "HEAD", review.ErrInvalidFeature},
		{"a head git would read as an option", "own-rdd", "--output=x", review.ErrInvalidRev},
		{"an empty head", "own-rdd", "", review.ErrInvalidRev},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := checkable(t)

			_, err := app.New(g, store).Check(evidenceDir, tc.feature, tc.head)

			if !errors.Is(err, tc.want) || len(g.calls) != 0 || len(store.calls) != 0 {
				t.Errorf("Check() error = %v with git calls %q and store calls %q, want one that wraps %v and no call", err, g.calls, store.calls, tc.want)
			}
		})
	}
}

// A result forged with a base git would read as an option never reaches git: the stored binding is held to the
// contract (a full object name) before it is used, so the base is not passed to git, and the file is refused.
func TestCheckDoesNotPassAForgedBaseToGit(t *testing.T) {
	g, store := checkable(t)
	name := reviewed(t, store, baseSHA, headSHA, 1)
	store.files["own-rdd"][name] = []byte(strings.Replace(string(store.files["own-rdd"][name]), baseSHA, "--output=forged", 1))

	got, err := app.New(g, store).Check(evidenceDir, "own-rdd", "HEAD")

	if !errors.Is(err, review.ErrInvalidStored) || g.called("Commit --") != 0 || !reflect.DeepEqual(got, app.Checked{}) {
		t.Errorf("Check() = %+v, %v with git calls %q; want a refusal that wraps ErrInvalidStored and no revision like an option given to git", got, err, g.calls)
	}
}
