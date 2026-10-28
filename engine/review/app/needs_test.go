package app_test

// What a commit needs before `review check` clears it (own-rdd T6b5, S6 d and f, L63): a valid Risk trailer, a sweep
// record, the sweeper's answer to a fired sweep, a polisher record for a declared item and the review that covers it,
// except for a passive commit, which needs nothing, and a sweep commit, which needs no sweep of its own. The tests go
// through Check and CheckChain over the fakes, with the records the other verbs store.

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// sweepNo is the number of the clean sweep that checkable stores for the head: the results of the other tests take the
// numbers 1, 2, 3 and 10, so the names they expect do not move.
const sweepNo = 7

func candidateFor(t *testing.T, base, head string) review.Candidate {
	t.Helper()
	c, err := review.NewCandidate(base, head, []byte(diffText), changedPaths)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return c
}

// swept stores the sweep number n of the commit from base to head, and returns its name. It fires exactly when it is given
// lines of the working tree (T3).
func swept(t *testing.T, store *fakeResults, base, head string, n int, t3 ...string) string {
	t.Helper()
	c := candidateFor(t, base, head)
	file, err := review.EncodeSweep(c, review.SweepFindings{T3: t3})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	store.put("own-rdd", review.SweepName(c, n), file)
	return review.SweepName(c, n)
}

// answered stores the sweeper record number n of the commit from base to head, that answers the sweep called sweep and
// names the commit of its change, when it names one.
func answered(t *testing.T, store *fakeResults, base, head string, n int, sweep string, commit *string) string {
	t.Helper()
	c := candidateFor(t, base, head)
	file, err := review.EncodeSweeper(c, sweep, review.SweeperReport{Commit: commit, Candidates: []review.SweeperCandidate{}})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	store.put("own-rdd", review.SweeperName(c, n), file)
	return review.SweeperName(c, n)
}

// polished stores the polisher record number n of the commit from base to head.
func polished(t *testing.T, store *fakeResults, base, head string, n int) string {
	t.Helper()
	c := candidateFor(t, base, head)
	file, err := review.EncodePolisher(c, []byte(`{"findings":[]}`))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	store.put("own-rdd", review.PolisherName(c, n), file)
	return review.PolisherName(c, n)
}

func recordPath(name string) string { return review.ResultPath("own-rdd", name) }

// resweep replaces the clean sweep of checkable with one of the diff and the paths git gives now.
func resweep(t *testing.T, g *fakeGit, store *fakeResults) {
	t.Helper()
	c, err := review.NewCandidate(baseSHA, headSHA, g.diff, g.paths)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	file, err := review.EncodeSweep(c, review.SweepFindings{})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	store.put("own-rdd", review.SweepName(c, sweepNo), file)
}

// checkable is a fake git whose head declares Risk: none and a store that holds a clean sweep of it: a head that lacks
// only its review.
func checkable(t *testing.T) (*fakeGit, *fakeResults) {
	t.Helper()
	g, store := newFakeGit(), newFakeResults()
	g.declares(headSHA, "none")
	swept(t, store, baseSHA, headSHA, sweepNo)
	return g, store
}

// A passive commit needs nothing: its trailer is Risk: none, it changes only task documents and review records, and it
// deletes or renames nothing. Check clears it with no record, answers no file, and stores nothing.
func TestCheckClearsAPassiveHeadWithoutAnyRecord(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	g.declares(headSHA, "none")
	g.paths = []string{"odd/tasks/own-rdd.md", "odd/review-results/own-rdd/x.json", "odd/review-receipts/r.json"}

	got, err := app.New(g, store).Check(evidenceDir, "own-rdd", "HEAD")

	if err != nil || !reflect.DeepEqual(got, app.Checked{Head: headSHA}) {
		t.Errorf("Check() = %+v, %v; want the head cleared with no result and no exemption", got, err)
	}
	if store.created() != 0 {
		t.Errorf("the store was written %d times, want nothing stored", store.created())
	}
}

// A commit is passive only when all four conditions hold: each one that fails asks for the rest of S6 (d).
func TestCheckDoesNotTreatAsPassiveACommitThatFailsAnyConditionOfPassive(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(g *fakeGit)
		want  error
	}{
		{"a path outside the passive directories", func(g *fakeGit) { g.paths = []string{"odd/tasks/own-rdd.md", "engine/a.go"} }, review.ErrNoSweep},
		{"a path that only begins like a passive directory", func(g *fakeGit) { g.paths = []string{"odd/tasks-old/own-rdd.md"} }, review.ErrNoSweep},
		{"a deleted or renamed path", func(g *fakeGit) {
			g.paths = []string{"odd/tasks/own-rdd.md"}
			g.deleted = []string{"odd/tasks/old.md"}
		}, review.ErrNoSweep},
		{"a trailer that names an item", func(g *fakeGit) {
			g.paths = []string{"odd/tasks/own-rdd.md"}
			g.declares(headSHA, "3")
		}, review.ErrNoSweep},
		{"a trailer that is missing", func(g *fakeGit) {
			g.paths = []string{"odd/tasks/own-rdd.md"}
			g.messages, g.trailers = map[string]string{}, map[string][]review.Trailer{}
		}, review.ErrRiskTrailer},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := newFakeGit(), newFakeResults()
			g.declares(headSHA, "none")
			tc.setup(g)

			got, err := app.New(g, store).Check(evidenceDir, "own-rdd", "HEAD")

			if !errors.Is(err, tc.want) || !reflect.DeepEqual(got, app.Checked{}) {
				t.Errorf("Check() = %+v, %v; want a refusal that wraps %v", got, err, tc.want)
			}
		})
	}
}

// A passive commit needs no record, but a result or exemption that binds it is still read and held: one that blocks,
// is not clear or does not hold refuses it, as it refuses any commit (S5).
func TestCheckStillHoldsTheStoredReviewOfAPassiveHead(t *testing.T) {
	// The results the helpers store bind the paths of changedPaths, so the commit is made passive by making those paths the
	// ones of the commit.
	saved := changedPaths
	changedPaths = []string{"odd/tasks/own-rdd.md"}
	t.Cleanup(func() { changedPaths = saved })
	passive := func(g *fakeGit) {
		g.declares(headSHA, "none")
		g.paths = changedPaths
	}
	t.Run("a result that blocks refuses it", func(t *testing.T) {
		g, store := newFakeGit(), newFakeResults()
		passive(g)
		verified(t, store, reviewed(t, store, baseSHA, headSHA, 1, finding("F1", "BLOCKER")), blocking)

		got, err := app.New(g, store).Check(evidenceDir, "own-rdd", "HEAD")

		if !errors.Is(err, review.ErrBlocked) || !reflect.DeepEqual(got, app.Checked{}) {
			t.Errorf("Check() = %+v, %v; want a refusal that wraps ErrBlocked", got, err)
		}
	})
	t.Run("a result that does not hold refuses it", func(t *testing.T) {
		g, store := newFakeGit(), newFakeResults()
		passive(g)
		store.put("own-rdd", "222222222222-1.json", []byte("{}"))

		_, err := app.New(g, store).Check(evidenceDir, "own-rdd", "HEAD")

		if !errors.Is(err, review.ErrInvalidStored) {
			t.Errorf("Check() error = %v, want one that wraps ErrInvalidStored", err)
		}
	})
	t.Run("a clear result is answered", func(t *testing.T) {
		g, store := newFakeGit(), newFakeResults()
		passive(g)
		reviewed(t, store, baseSHA, headSHA, 1)

		got, err := app.New(g, store).Check(evidenceDir, "own-rdd", "HEAD")

		if err != nil || len(got.Results) != 1 || got.Results[0].Result != wantFirstPath {
			t.Errorf("Check() = %+v, %v; want the head cleared by %s", got, err, wantFirstPath)
		}
	})
}

// Check refuses with the first thing the head lacks, in the order of S6 (d), and the reason names the commit. Nothing is
// answered and nothing stored.
func TestCheckRefusesWithTheFirstThingTheHeadLacks(t *testing.T) {
	oneName := func(t *testing.T, store *fakeResults, suffix string) string {
		t.Helper()
		for name := range store.files["own-rdd"] {
			if strings.HasSuffix(name, suffix) {
				return name
			}
		}
		t.Fatalf("setup: no %s is stored", suffix)
		return ""
	}
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, g *fakeGit, store *fakeResults)
		want  error
		say   string // the whole reason when it is the text of S6 (f), else a part of it
	}{
		{"no trailer", func(t *testing.T, g *fakeGit, _ *fakeResults) {
			g.messages, g.trailers = map[string]string{}, map[string][]review.Trailer{}
		}, review.ErrRiskTrailer, `commit ` + headSHA + `: it has no "Risk:" trailer`},
		{"a trailer outside the grammar, and no sweep either", func(t *testing.T, g *fakeGit, store *fakeResults) {
			g.declares(headSHA, "7")
			delete(store.files["own-rdd"], oneName(t, store, ".sweep.json"))
		}, review.ErrRiskTrailer, `its "Risk:" trailer "7" is not`},
		{"no sweep record", func(t *testing.T, g *fakeGit, store *fakeResults) {
			delete(store.files["own-rdd"], oneName(t, store, ".sweep.json"))
		}, review.ErrNoSweep, "commit " + headSHA + ": review: no sweep record binds the commit"},
		{"a fired sweep nobody answers", func(t *testing.T, g *fakeGit, store *fakeResults) {
			swept(t, store, baseSHA, headSHA, 8, "?? stray.txt")
		}, review.ErrNoSweeper, "commit " + headSHA + ": review: no sweeper record answers the fired sweep record: " + recordPath("222222222222-8.sweep.json")},
		{"a sweeper that answered an earlier sweep, not the latest", func(t *testing.T, g *fakeGit, store *fakeResults) {
			first := swept(t, store, baseSHA, headSHA, 8, "?? stray.txt")
			swept(t, store, baseSHA, headSHA, 9, "?? stray.txt")
			answered(t, store, baseSHA, headSHA, 10, first, nil)
		}, review.ErrNoSweeper, recordPath("222222222222-9.sweep.json")},
		{"a declared item with no polisher", func(t *testing.T, g *fakeGit, _ *fakeResults) {
			g.declares(headSHA, "3,5")
		}, review.ErrNoPolisher, "commit " + headSHA + ": review: no polisher record binds a commit that declares a high-risk item: Risk: 3,5"},
		{"only the review is missing", func(*testing.T, *fakeGit, *fakeResults) {}, review.ErrNoResult, "head " + headSHA + ": review: no stored result binds the head"},
		{"a fired sweep is answered but the result blocks", func(t *testing.T, g *fakeGit, store *fakeResults) {
			name := swept(t, store, baseSHA, headSHA, 8, "?? stray.txt")
			answered(t, store, baseSHA, headSHA, 9, name, nil)
			verified(t, store, reviewed(t, store, baseSHA, headSHA, 1, finding("F1", "BLOCKER")), blocking)
		}, review.ErrBlocked, "F1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := checkable(t)
			tc.setup(t, g, store)
			created := store.created()

			got, err := app.New(g, store).Check(evidenceDir, "own-rdd", "HEAD")

			if err == nil || !errors.Is(err, tc.want) || !strings.Contains(err.Error(), tc.say) {
				t.Fatalf("Check() = %+v, %v; want a refusal that wraps %v and says %q", got, err, tc.want, tc.say)
			}
			for _, other := range []error{review.ErrNoSweep, review.ErrNoSweeper, review.ErrNoPolisher, review.ErrNoResult} {
				if other != tc.want && errors.Is(err, other) {
					t.Errorf("the reason %q also wraps %v, want exactly one reason", err, other)
				}
			}
			if !reflect.DeepEqual(got, app.Checked{}) || store.created() != created {
				t.Errorf("Check() answered %+v and the store was written %d times, want nothing answered and nothing stored", got, store.created()-created)
			}
		})
	}
}

// A head that lacks nothing is cleared: the roles it needs left their records and a review covers it.
func TestCheckClearsAHeadWhoseRolesAllLeftTheirRecords(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, g *fakeGit, store *fakeResults)
	}{
		{"a clean sweep and a review", func(*testing.T, *fakeGit, *fakeResults) {}},
		{"the latest sweep is clean, an earlier one fired and nobody answered it", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			swept(t, store, baseSHA, headSHA, 5, "?? stray.txt")
		}},
		{"a fired sweep and the sweeper that answers it", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			answered(t, store, baseSHA, headSHA, 9, swept(t, store, baseSHA, headSHA, 8, "?? stray.txt"), nil)
		}},
		{"a declared item and a polisher", func(t *testing.T, g *fakeGit, store *fakeResults) {
			g.declares(headSHA, "3,5")
			polished(t, store, baseSHA, headSHA, 4)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := checkable(t)
			tc.setup(t, g, store)
			reviewed(t, store, baseSHA, headSHA, 1)

			got, err := app.New(g, store).Check(evidenceDir, "own-rdd", "HEAD")

			if err != nil || got.Head != headSHA || len(got.Results) != 1 {
				t.Errorf("Check() = %+v, %v; want the head cleared by %s", got, err, wantFirstPath)
			}
		})
	}
}

// A sweep commit, whose subject starts chore(sweep) and which a sweeper record of its first parent names, needs no sweep
// of its own. One whose subject does not, or that the sweeper record names no more, does.
func TestCheckClearsASweepCommitWithoutASweepOfItsOwn(t *testing.T) {
	sweepCommit := func(t *testing.T, subject string, names *string) (*fakeGit, *fakeResults) {
		g, store := newFakeGit(), newFakeResults()
		message := subject + "\n\nRisk: none"
		g.messages[headSHA] = message
		g.trailers[message] = []review.Trailer{{Key: "Risk", Value: "none"}}
		g.parents[baseSHA] = otherSHA
		first := swept(t, store, otherSHA, baseSHA, 1, "?? stray.txt")
		answered(t, store, otherSHA, baseSHA, 2, first, names)
		reviewed(t, store, baseSHA, headSHA, 1)
		return g, store
	}
	head := headSHA
	other := otherSHA
	for _, tc := range []struct {
		name    string
		subject string
		names   *string
		want    error
	}{
		{"named by the sweeper record of its parent", "chore(sweep): remove the dead helper", &head, nil},
		{"the subject is not a sweep", "fix: something else", &head, review.ErrNoSweep},
		{"the sweeper record names another commit", "chore(sweep): remove the dead helper", &other, review.ErrNoSweep},
		{"the sweeper record names no commit", "chore(sweep): remove the dead helper", nil, review.ErrNoSweep},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := sweepCommit(t, tc.subject, tc.names)

			got, err := app.New(g, store).Check(evidenceDir, "own-rdd", "HEAD")

			if tc.want == nil && (err != nil || got.Head != headSHA) {
				t.Errorf("Check() = %+v, %v; want the sweep commit cleared", got, err)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Errorf("Check() = %+v, %v; want a refusal that wraps %v", got, err, tc.want)
			}
		})
	}
}

// A role record is held to git again as a result is, and a file that does not hold refuses the commit with its path and
// the reason: one that cannot be read, is not that kind of record, binds another diff, or is not for one commit.
func TestCheckRefusesARoleRecordThatDoesNotHold(t *testing.T) {
	boom := errors.New("boom")
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, g *fakeGit, store *fakeResults) (read func(app.Results) app.Results)
		want  error
		say   []string
	}{
		{"the sweep is no longer a sweep", func(t *testing.T, _ *fakeGit, store *fakeResults) func(app.Results) app.Results {
			store.put("own-rdd", "222222222222-7.sweep.json", []byte("{}"))
			return nil
		}, review.ErrInvalidStored, []string{recordPath("222222222222-7.sweep.json")}},
		{"the diff of the sweep is not the one git gives now", func(t *testing.T, g *fakeGit, _ *fakeResults) func(app.Results) app.Results {
			g.diff = []byte("a diff the sweep never saw\n")
			return nil
		}, review.ErrBindingDiffers, []string{recordPath("222222222222-7.sweep.json"), "diff_sha256"}},
		{"the base of the sweep is not the first parent of the head", func(t *testing.T, _ *fakeGit, store *fakeResults) func(app.Results) app.Results {
			delete(store.files["own-rdd"], "222222222222-7.sweep.json")
			swept(t, store, otherSHA, headSHA, 7)
			return nil
		}, review.ErrNotOneCommit, []string{recordPath("222222222222-7.sweep.json"), "first parent"}},
		{"the sweep cannot be read", func(t *testing.T, _ *fakeGit, store *fakeResults) func(app.Results) app.Results {
			return func(r app.Results) app.Results { return unreadable{store, "222222222222-7.sweep.json", boom} }
		}, boom, []string{"read " + recordPath("222222222222-7.sweep.json")}},
		{"the sweeper record is no longer one", func(t *testing.T, _ *fakeGit, store *fakeResults) func(app.Results) app.Results {
			swept(t, store, baseSHA, headSHA, 8, "?? stray.txt")
			store.put("own-rdd", "222222222222-9.sweeper.json", []byte("{}"))
			return nil
		}, review.ErrInvalidStored, []string{recordPath("222222222222-9.sweeper.json")}},
		{"the polisher record is no longer one", func(t *testing.T, g *fakeGit, store *fakeResults) func(app.Results) app.Results {
			g.declares(headSHA, "2")
			store.put("own-rdd", "222222222222-9.polisher.json", []byte("{}"))
			return nil
		}, review.ErrInvalidStored, []string{recordPath("222222222222-9.polisher.json")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := checkable(t)
			wrap := tc.setup(t, g, store)
			var results app.Results = store
			if wrap != nil {
				results = wrap(store)
			}

			got, err := app.New(g, results).Check(evidenceDir, "own-rdd", "HEAD")

			if !errors.Is(err, tc.want) || !reflect.DeepEqual(got, app.Checked{}) {
				t.Fatalf("Check() = %+v, %v; want a refusal that wraps %v", got, err, tc.want)
			}
			for _, s := range tc.say {
				if !strings.Contains(err.Error(), s) {
					t.Errorf("message %q does not say %q", err, s)
				}
			}
		})
	}
}

// A role record whose name is of the commit but whose stored head is another commit is not the commit's, as a result's is
// not: the commit still lacks its sweep.
func TestCheckDoesNotCountTheRoleRecordOfAnotherCommit(t *testing.T) {
	collision := "222222222222" + strings.Repeat("3", 28) // another commit whose name begins like the head's
	g, store := newFakeGit(), newFakeResults()
	g.declares(headSHA, "none")
	c := candidateFor(t, baseSHA, collision)
	file, err := review.EncodeSweep(c, review.SweepFindings{})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	store.put("own-rdd", "222222222222-7.sweep.json", file)

	_, err = app.New(g, store).Check(evidenceDir, "own-rdd", "HEAD")

	if !errors.Is(err, review.ErrNoSweep) {
		t.Errorf("Check() error = %v, want one that wraps ErrNoSweep", err)
	}
}

// An exemption is for work that names no item: of a commit whose trailer names one it refuses the head with the error
// `review exempt --risk` gives, even when a clear result also binds it, as an exemption that does not hold does.
func TestCheckRefusesAnExemptionOfACommitWhoseTrailerNamesAnItem(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, store *fakeResults)
	}{
		{"it alone", func(t *testing.T, store *fakeResults) { exempted(t, store, baseSHA, headSHA, 1) }},
		{"a clear result binds the commit too", func(t *testing.T, store *fakeResults) {
			reviewed(t, store, baseSHA, headSHA, 1)
			exempted(t, store, baseSHA, headSHA, 2)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := checkable(t)
			g.declares(headSHA, "3,5")
			polished(t, store, baseSHA, headSHA, 4)
			tc.setup(t, store)
			exemption := wantFirstExemptionPath
			if len(store.files["own-rdd"]) > 0 && store.files["own-rdd"]["222222222222-2.exempt.json"] != nil {
				exemption = wantSecondExemptionPath
			}

			got, err := app.New(g, store).Check(evidenceDir, "own-rdd", "HEAD")

			want := exemption + ": commit " + headSHA + ": " + review.CheckRisk("3,5").Error()
			if !errors.Is(err, review.ErrHighRisk) || err.Error() != want || !reflect.DeepEqual(got, app.Checked{}) {
				t.Errorf("Check() = %+v, %v; want the refusal %q", got, err, want)
			}
		})
	}
}

// chain makes the fakes for the commits base, head and tip: tip's parent is head, and both declare Risk: none and have a
// clean sweep. The paths of every commit are the engine's, so none is passive.
func chain(t *testing.T) (*fakeGit, *fakeResults) {
	t.Helper()
	g, store := newFakeGit(), newFakeResults()
	g.parents[otherSHA] = headSHA
	g.declares(headSHA, "none")
	g.declares(otherSHA, "none")
	swept(t, store, baseSHA, headSHA, sweepNo)
	swept(t, store, headSHA, otherSHA, sweepNo)
	return g, store
}

// A passive commit is a link of the unbroken chain from its first parent, with no record: the range starts where the
// commit's parent is and ends at the commit, with no result and no exemption.
func TestCheckChainTakesAPassiveCommitAsALinkWithNoRecord(t *testing.T) {
	g, store := chain(t)
	g.pathsOf = map[string][]string{headSHA: {"odd/tasks/own-rdd.md"}}
	reviewed(t, store, headSHA, otherSHA, 1)

	got, err := app.New(g, store).CheckChain(evidenceDir, "own-rdd", baseSHA, otherSHA)

	if err != nil || len(got.Ranges) != 2 {
		t.Fatalf("CheckChain() = %+v, %v; want a chain of two ranges", got, err)
	}
	first, second := got.Ranges[0], got.Ranges[1]
	if first.Base != baseSHA || first.Head != headSHA || len(first.Results)+len(first.Exemptions) != 0 {
		t.Errorf("first range = %+v, want the passive commit from %s with no record", first, baseSHA)
	}
	if second.Base != headSHA || second.Head != otherSHA || len(second.Results) != 1 {
		t.Errorf("second range = %+v, want the result from %s to %s", second, headSHA, otherSHA)
	}
}

// A commit that is inside a range a review covers needs everything but the review: its trailer, its sweep, the sweeper
// and the polisher. The refusal names the commit.
func TestCheckChainHoldsTheCommitsInsideAReviewedRangeToWhatTheyNeed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, g *fakeGit, store *fakeResults)
		want  error
		say   string
	}{
		{"the commit inside has everything", func(*testing.T, *fakeGit, *fakeResults) {}, nil, ""},
		{"the commit inside is passive", func(_ *testing.T, g *fakeGit, store *fakeResults) {
			g.pathsOf = map[string][]string{headSHA: {"odd/tasks/own-rdd.md"}}
			delete(store.files["own-rdd"], "222222222222-7.sweep.json")
		}, nil, ""},
		{"the commit inside has no trailer", func(_ *testing.T, g *fakeGit, _ *fakeResults) {
			delete(g.messages, headSHA)
		}, review.ErrRiskTrailer, headSHA},
		{"the commit inside has no sweep", func(_ *testing.T, _ *fakeGit, store *fakeResults) {
			delete(store.files["own-rdd"], "222222222222-7.sweep.json")
		}, review.ErrNoSweep, "commit " + headSHA + ": review: no sweep record binds the commit"},
		{"the commit inside has a fired sweep nobody answers", func(t *testing.T, _ *fakeGit, store *fakeResults) {
			swept(t, store, baseSHA, headSHA, 8, "?? stray.txt")
		}, review.ErrNoSweeper, "commit " + headSHA},
		{"the commit inside declares an item and has no polisher", func(_ *testing.T, g *fakeGit, _ *fakeResults) {
			g.declares(headSHA, "4")
		}, review.ErrNoPolisher, "commit " + headSHA},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := chain(t)
			tc.setup(t, g, store)
			reviewed(t, store, baseSHA, otherSHA, 1) // one review of both commits

			got, err := app.New(g, store).CheckChain(evidenceDir, "own-rdd", baseSHA, otherSHA)

			if tc.want == nil {
				if err != nil || len(got.Ranges) != 1 || got.Ranges[0].Base != baseSHA {
					t.Errorf("CheckChain() = %+v, %v; want the one range from %s", got, err, baseSHA)
				}
				return
			}
			prefix := "the chain from " + baseSHA + " to " + otherSHA + ": "
			if !errors.Is(err, tc.want) || !strings.HasPrefix(err.Error(), prefix) || !strings.Contains(err.Error(), tc.say) {
				t.Errorf("CheckChain() = %+v, %v; want a refusal after %q that wraps %v and says %q", got, err, prefix, tc.want, tc.say)
			}
		})
	}
}

// The commit that ends a range is held to what it needs and to the review that covers it, and the refusal is the first
// of them: a commit with no sweep is refused for its sweep and not for the missing review, and one that lacks only the
// review is refused with the text it always had.
func TestCheckChainRefusesTheCommitThatEndsARangeWithTheFirstThingItLacks(t *testing.T) {
	t.Run("no sweep and no review", func(t *testing.T) {
		g, store := chain(t)
		delete(store.files["own-rdd"], "333333333333-7.sweep.json")
		reviewed(t, store, baseSHA, headSHA, 1)

		_, err := app.New(g, store).CheckChain(evidenceDir, "own-rdd", baseSHA, otherSHA)

		want := "the chain from " + baseSHA + " to " + otherSHA + ": commit " + otherSHA + ": review: no sweep record binds the commit"
		if !errors.Is(err, review.ErrNoSweep) || err.Error() != want {
			t.Errorf("CheckChain() error = %v, want %q", err, want)
		}
	})
	t.Run("only the review", func(t *testing.T) {
		g, store := chain(t)
		reviewed(t, store, baseSHA, headSHA, 1)

		_, err := app.New(g, store).CheckChain(evidenceDir, "own-rdd", baseSHA, otherSHA)

		want := "the chain from " + baseSHA + " to " + otherSHA + ": head " + otherSHA + ": review: no stored result binds the head"
		if !errors.Is(err, review.ErrNoResult) || err.Error() != want {
			t.Errorf("CheckChain() error = %v, want %q", err, want)
		}
	})
}

// While the trailer of a commit fails, a role record that does not read back or hold counts as absent: the commit is
// refused with the error of its trailer, which review.Needs lists first, and the record's own refusal is met once the
// trailer holds (own-rdd T6b5a, S6 f, L66).
func TestCheckRefusesAFailingTrailerBeforeARoleRecordThatDoesNotHold(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(g *fakeGit, store *fakeResults)
		path  string
	}{
		{"a sweep record that does not decode", func(g *fakeGit, store *fakeResults) {
			delete(g.messages, headSHA)
			store.put("own-rdd", "222222222222-1.sweep.json", []byte("not json"))
		}, recordPath("222222222222-1.sweep.json")},
		{"a sweeper record of the first parent of a sweep commit that does not decode", func(g *fakeGit, store *fakeResults) {
			g.messages[headSHA] = "chore(sweep): remove the dead helper"
			store.put("own-rdd", "111111111111-1.sweeper.json", []byte("not json"))
		}, recordPath("111111111111-1.sweeper.json")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := newFakeGit(), newFakeResults()
			tc.setup(g, store)

			got, err := app.New(g, store).Check(evidenceDir, "own-rdd", "HEAD")

			if !errors.Is(err, review.ErrRiskTrailer) || !reflect.DeepEqual(got, app.Checked{}) {
				t.Fatalf("Check() = %+v, %v; want a refusal that wraps %v", got, err, review.ErrRiskTrailer)
			}
			if strings.Contains(err.Error(), tc.path) {
				t.Errorf("message %q names the record %s, want the trailer's error alone", err, tc.path)
			}
		})
	}
}
