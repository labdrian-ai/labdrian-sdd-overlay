package app_test

// What Check does with the exemptions of a head (own-rdd T4, S5, L16): a head is cleared by a stored exemption that
// still holds when no stored result binds it, a result that blocks wins over any exemption, and an exemption is read
// back, held to its rules and to git again, so one that was edited, or is about another head, never clears.

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

const (
	wantFirstExemptionPath  = "odd/review-results/own-rdd/222222222222-1.exempt.json"
	wantSecondExemptionPath = "odd/review-results/own-rdd/222222222222-2.exempt.json"
)

// exempted stores the exemption number n of the candidate from base to head, with a clear report and the risk none,
// as Exempt does, and returns its name.
func exempted(t *testing.T, store *fakeResults, base, head string, n int) string {
	t.Helper()
	c, err := review.NewCandidate(base, head, []byte(diffText), changedPaths)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	file, err := review.EncodeExemption(c, review.NoRisk, []byte(exemptReport))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	store.put("own-rdd", review.ExemptionName(c, n), file)
	return review.ExemptionName(c, n)
}

// A head is cleared by the stored exemption that binds it. The candidate is recomputed from git for the commits the
// exemption names, as it is for a result, and the stored binding must be equal to what git gave. The store is
// listed and the exemption read: no result and no verdict is looked for, and nothing is stored.
func TestCheckClearsTheHeadOfAStoredExemptionAndRecomputesTheCandidateFromGit(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	exempted(t, store, baseSHA, headSHA, 1)

	got, err := app.New(g, store).Check(evidenceDir, "own-rdd", "HEAD")

	want := app.Checked{Head: headSHA, Exemptions: []review.Exempted{{Candidate: candidateOf(t, g), Exemption: wantFirstExemptionPath}}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Check() = %+v, %v; want %+v and no error", got, err, want)
	}
	wantGit := []string{
		"Locate " + evidenceDir, "Commit HEAD",
		"Commit " + baseSHA, "Commit " + headSHA, "Diff " + baseSHA + " " + headSHA, "Paths " + baseSHA + " " + headSHA, "Parent " + headSHA,
	}
	if !reflect.DeepEqual(g.calls, wantGit) {
		t.Errorf("git calls = %q, want %q", g.calls, wantGit)
	}
	wantStore := []string{"Names " + evidenceDir + " own-rdd", "Read " + evidenceDir + " own-rdd 222222222222-1.exempt.json"}
	if !reflect.DeepEqual(store.calls, wantStore) {
		t.Errorf("store calls = %q, want %q", store.calls, wantStore)
	}
}

// What Exempt stores is what Check clears, end to end through the two use cases over the same store.
func TestCheckClearsWhatExemptStored(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	exempt, err := app.New(g, store).Exempt(evidenceDir, "own-rdd", "HEAD~1", "HEAD", "none", []byte(exemptReport))
	if err != nil {
		t.Fatalf("setup: Exempt() error = %v", err)
	}

	got, err := app.New(newFakeGit(), store).Check(evidenceDir, "own-rdd", "HEAD")

	if err != nil || len(got.Exemptions) != 1 || got.Exemptions[0].Exemption != exempt.Path || len(got.Results) != 0 {
		t.Errorf("Check() = %+v, %v; want the head cleared by %s alone", got, err, exempt.Path)
	}
}

// Every exemption that binds the head is answered, in the order of their numbers, whatever order the store lists
// them in: the tenth after the ninth and not after the first.
func TestCheckAnswersEveryExemptionOfTheHeadInTheOrderOfTheirNumbers(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	exempted(t, store, baseSHA, headSHA, 10)
	exempted(t, store, baseSHA, headSHA, 2)

	got, err := app.New(g, store).Check(evidenceDir, "own-rdd", "HEAD")

	var names []string
	for _, e := range got.Exemptions {
		names = append(names, e.Exemption)
	}
	want := []string{wantSecondExemptionPath, "odd/review-results/own-rdd/222222222222-10.exempt.json"}
	if err != nil || !reflect.DeepEqual(names, want) {
		t.Errorf("Check() = exemptions %q, %v; want %q and no error", names, err, want)
	}
}

// A head with a clear result and an exemption is cleared, and both are answered: the exemption is not what clears
// it, and it is not hidden either.
func TestCheckAnswersTheResultsAndTheExemptionsOfAHeadThatHasBoth(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	reviewed(t, store, baseSHA, headSHA, 1)
	exempted(t, store, baseSHA, headSHA, 2)

	got, err := app.New(g, store).Check(evidenceDir, "own-rdd", "HEAD")

	if err != nil || len(got.Results) != 1 || got.Results[0].Result != wantFirstPath || len(got.Exemptions) != 1 || got.Exemptions[0].Exemption != wantSecondExemptionPath {
		t.Errorf("Check() = %+v, %v; want the result %s and the exemption %s", got, err, wantFirstPath, wantSecondExemptionPath)
	}
}

// A blocking review wins over any exemption, and a result that does not block and is not clear either keeps the head
// from delivery as well: an exemption is for work that takes no review, not a way around one that left a severe
// finding unchecked. The refusal is the one the results alone give, and it answers nothing and stores nothing.
func TestCheckLetsAStoredResultThatDoesNotClearTheHeadWinOverAnExemption(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, store *fakeResults)
		want  error
		say   []string
	}{
		{"the result blocks and the exemption was made after it", func(t *testing.T, store *fakeResults) {
			verified(t, store, reviewed(t, store, baseSHA, headSHA, 1, finding("F1", "BLOCKER")), blocking)
			exempted(t, store, baseSHA, headSHA, 2)
		}, review.ErrBlocked, []string{wantFirstPath, wantVerdict, "F1"}},
		{"the result blocks and the exemption was made before it", func(t *testing.T, store *fakeResults) {
			exempted(t, store, baseSHA, headSHA, 1)
			verified(t, store, reviewed(t, store, baseSHA, headSHA, 2, finding("F1", "BLOCKER")), blocking)
		}, review.ErrBlocked, []string{wantSecondPath, wantSecondVerdict, "F1"}},
		{"a clear result, then one that blocks, and an exemption", func(t *testing.T, store *fakeResults) {
			reviewed(t, store, baseSHA, headSHA, 1)
			verified(t, store, reviewed(t, store, baseSHA, headSHA, 2, finding("F1", "BLOCKER")), blocking)
			exempted(t, store, baseSHA, headSHA, 3)
		}, review.ErrBlocked, []string{wantSecondPath, wantSecondVerdict, "F1"}},
		{"a severe finding nobody checked, and an exemption", func(t *testing.T, store *fakeResults) {
			reviewed(t, store, baseSHA, headSHA, 1, finding("F1", "BLOCKER"))
			exempted(t, store, baseSHA, headSHA, 2)
		}, review.ErrNotClear, []string{wantFirstPath, "no verdict", "F1"}},
		{"a severe finding the verifier could not check, and an exemption", func(t *testing.T, store *fakeResults) {
			verified(t, store, reviewed(t, store, baseSHA, headSHA, 1, finding("F1", "BLOCKER")), verdictReply(verdict("F1", "not-run")))
			exempted(t, store, baseSHA, headSHA, 2)
		}, review.ErrNotClear, []string{wantFirstPath, wantVerdict, "F1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := newFakeGit(), newFakeResults()
			tc.setup(t, store)
			created := store.created()

			got, err := app.New(g, store).Check(evidenceDir, "own-rdd", "HEAD")

			if !errors.Is(err, tc.want) {
				t.Fatalf("Check() = %+v, %v; want a refusal that wraps %v", got, err, tc.want)
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

// An exemption that no longer holds never clears the head, and is a refusal that names the file and the reason, and
// stores nothing: the file is read back and held to its rules and to git again, and nothing is taken from what an
// earlier command said. A file that was edited to name no risk, to say a check passed that did not, or to be about a
// diff the head no longer has is refused; so is what cannot be read, because what cannot be checked is not cleared.
func TestCheckRefusesAnExemptionThatNoLongerHoldsAndStoresNothing(t *testing.T) {
	boom := errors.New("boom")
	edit := func(t *testing.T, store *fakeResults, name, from, to string) {
		t.Helper()
		text := string(store.files["own-rdd"][name])
		if !strings.Contains(text, from) {
			t.Fatalf("setup: %s does not hold %q:\n%s", name, from, text)
		}
		store.files["own-rdd"][name] = []byte(strings.Replace(text, from, to, 1))
	}
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, g *fakeGit, store *fakeResults)
		want  []error // the reasons the error wraps
		say   []string
	}{
		{"the stored file is no longer an exemption", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			store.put("own-rdd", "222222222222-1.exempt.json", []byte("{}"))
		}, []error{review.ErrInvalidStored}, []string{wantFirstExemptionPath}},
		{"the stored file is a result under the name of an exemption", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			name := reviewed(t, store, baseSHA, headSHA, 1)
			store.put("own-rdd", "222222222222-1.exempt.json", store.files["own-rdd"][name])
			delete(store.files["own-rdd"], name)
		}, []error{review.ErrInvalidStored}, []string{wantFirstExemptionPath}},
		{"the risk was edited to name a high-risk item", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			edit(t, store, exempted(t, store, baseSHA, headSHA, 1), `"risk": "none"`, `"risk": "2"`)
		}, []error{review.ErrHighRisk}, []string{wantFirstExemptionPath, "2 (security)"}},
		{"the risk was edited to something nobody can read", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			edit(t, store, exempted(t, store, baseSHA, headSHA, 1), `"risk": "none"`, `"risk": "no"`)
		}, []error{review.ErrInvalidRisk}, []string{wantFirstExemptionPath}},
		{"the report was edited so that a check failed", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			edit(t, store, exempted(t, store, baseSHA, headSHA, 1), `"result": "pass"`, `"result": "fail"`)
		}, []error{reviewverdict.ErrNotClear}, []string{wantFirstExemptionPath, "fail"}},
		{"the report was edited so that a check did not run", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			edit(t, store, exempted(t, store, baseSHA, headSHA, 1), `"result": "pass"`, `"result": "not-run"`)
		}, []error{reviewverdict.ErrNotClear}, []string{wantFirstExemptionPath, "not-run"}},
		{"the report was edited to a member the contract does not name", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			edit(t, store, exempted(t, store, baseSHA, headSHA, 1), `"checks": [`, `"outcome": "clear", "checks": [`)
		}, []error{reviewverdict.ErrUnknownMember}, []string{wantFirstExemptionPath}},
		{"the diff of the range is no longer the one the exemption was made on", func(t *testing.T, g *fakeGit, store *fakeResults) {
			exempted(t, store, baseSHA, headSHA, 1)
			g.diff = []byte("a diff nobody checked\n")
		}, []error{review.ErrBindingDiffers}, []string{wantFirstExemptionPath, "diff_sha256"}},
		{"git lists other paths for the range", func(t *testing.T, g *fakeGit, store *fakeResults) {
			exempted(t, store, baseSHA, headSHA, 1)
			g.paths = changedPaths[:1]
		}, []error{review.ErrBindingDiffers}, []string{wantFirstExemptionPath, "paths"}},
		{"git does not know the base the exemption names", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			exempted(t, store, fourSHA, headSHA, 1)
		}, nil, []string{wantFirstExemptionPath, "resolve base"}},
		{"git cannot give the diff of the range", func(t *testing.T, g *fakeGit, store *fakeResults) {
			exempted(t, store, baseSHA, headSHA, 1)
			g.fail["Diff"] = boom
		}, []error{boom}, []string{wantFirstExemptionPath, "diff"}},
		{"the store cannot read the exemption", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			exempted(t, store, baseSHA, headSHA, 1)
			store.fail["Read"] = boom
		}, []error{boom}, []string{wantFirstExemptionPath}},
		{"one exemption holds and another does not", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			exempted(t, store, baseSHA, headSHA, 1)
			edit(t, store, exempted(t, store, baseSHA, headSHA, 2), `"risk": "none"`, `"risk": "1"`)
		}, []error{review.ErrHighRisk}, []string{wantSecondExemptionPath}},
		{"a clear result and an exemption that does not hold", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			reviewed(t, store, baseSHA, headSHA, 1)
			edit(t, store, exempted(t, store, baseSHA, headSHA, 2), `"result": "pass"`, `"result": "fail"`)
		}, []error{reviewverdict.ErrNotClear}, []string{wantSecondExemptionPath}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := newFakeGit(), newFakeResults()
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

// An exemption that is about another head never clears this one, whatever its name says: the file is about a commit by
// its binding and not by its name. A name that does not begin with the head is not looked at, a name that does begin
// with it and holds the binding of another commit is read and left out, and either way the head was never exempted.
func TestCheckNeverClearsAHeadByTheExemptionOfAnotherHead(t *testing.T) {
	collision := "222222222222" + strings.Repeat("3", 28) // another commit whose name begins like the head's
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, store *fakeResults)
	}{
		{"the exemption is of another head", func(t *testing.T, store *fakeResults) {
			exempted(t, store, baseSHA, otherSHA, 1)
		}},
		{"the exemption of another head is stored under the name of this one", func(t *testing.T, store *fakeResults) {
			c, err := review.NewCandidate(baseSHA, otherSHA, []byte(diffText), changedPaths)
			if err != nil {
				t.Fatalf("setup: %v", err)
			}
			file, err := review.EncodeExemption(c, review.NoRisk, []byte(exemptReport))
			if err != nil {
				t.Fatalf("setup: %v", err)
			}
			store.put("own-rdd", "222222222222-1.exempt.json", file)
		}},
		{"the name says the head but the binding is a commit that begins like it", func(t *testing.T, store *fakeResults) {
			c, err := review.NewCandidate(baseSHA, collision, []byte(diffText), changedPaths)
			if err != nil {
				t.Fatalf("setup: %v", err)
			}
			file, err := review.EncodeExemption(c, review.NoRisk, []byte(exemptReport))
			if err != nil {
				t.Fatalf("setup: %v", err)
			}
			store.put("own-rdd", "222222222222-1.exempt.json", file)
		}},
		{"the exemption of this head is stored under the name of another", func(t *testing.T, store *fakeResults) {
			name := exempted(t, store, baseSHA, headSHA, 1)
			store.put("own-rdd", "333333333333-1.exempt.json", store.files["own-rdd"][name])
			delete(store.files["own-rdd"], name)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := newFakeGit(), newFakeResults()
			tc.setup(t, store)

			got, err := app.New(g, store).Check(evidenceDir, "own-rdd", "HEAD")

			if !errors.Is(err, review.ErrNoResult) || !reflect.DeepEqual(got, app.Checked{}) {
				t.Errorf("Check() = %+v, %v; want a refusal that wraps ErrNoResult: the head was never exempted", got, err)
			}
		})
	}
}

// An exemption forged with a base git would read as an option never reaches git: the stored binding is held to the
// contract (a full object name) before it is used, as the binding of a stored result is, so the base is not passed
// to git, and the file is refused.
func TestCheckDoesNotPassTheForgedBaseOfAnExemptionToGit(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	name := exempted(t, store, baseSHA, headSHA, 1)
	store.files["own-rdd"][name] = []byte(strings.Replace(string(store.files["own-rdd"][name]), baseSHA, "--output=forged", 1))

	got, err := app.New(g, store).Check(evidenceDir, "own-rdd", "HEAD")

	if !errors.Is(err, reviewresult.ErrInvalidBinding) || g.called("Commit --") != 0 || !reflect.DeepEqual(got, app.Checked{}) {
		t.Errorf("Check() = %+v, %v with git calls %q; want a refusal that wraps ErrInvalidBinding and no revision like an option given to git", got, err, g.calls)
	}
}
