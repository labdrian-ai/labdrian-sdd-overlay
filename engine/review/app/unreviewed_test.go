package app_test

// What NextUnreviewed does (own-rdd T6a, S6, L18): of the commits a branch has that the default branch has not, it names
// the next one that no stored review covers, with the commit before it as the base, for the stop hook to ask for.
// A review is a stored result or exemption that is read back and held to git again, whatever it made of the review;
// the reviews of every feature of the tree count; and a tree that keeps no features, a detached HEAD, the default
// branch and a chain that is complete have nothing to ask. The branch of the fakes has two commits, headSHA and
// otherSHA, made on baseSHA.

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// branchWorld is a repository whose branch feat/x has two commits of its own on top of the commit baseSHA, in a tree
// that keeps features, with no review stored.
func branchWorld() (*fakeGit, *fakeResults) {
	g, store := newFakeGit(), newFakeResults()
	g.standing = app.Standing{Branch: "feat/x", Default: "main", Commits: []string{headSHA, otherSHA}}
	g.parents = map[string]string{headSHA: baseSHA, otherSHA: headSHA}
	g.declares(otherSHA, "none")
	store.tasks = true
	return g, store
}

// sweptWorld is branchWorld with a clean sweep record of each of its commits, in the feature own-rdd: a branch whose
// commits lack nothing but a review.
func sweptWorld(t *testing.T) (*fakeGit, *fakeResults) {
	t.Helper()
	g, store := branchWorld()
	sweptBranch(t, store)
	return g, store
}

// same says got asks for the commit that want names: the branch, the base and the head. What the commit lacks is told
// apart in the tests that care for it.
func same(got, want app.Question) bool {
	return got.Branch == want.Branch && got.Base == want.Base && got.Head == want.Head
}

// nothing says got is no question.
func nothing(got app.Question) bool { return reflect.DeepEqual(got, app.Question{}) }

// A branch no review covers is asked for from its first commit, with the commit the branch was made on as the base.
func TestNextUnreviewedAsksForTheFirstCommitOfABranchNoReviewCovers(t *testing.T) {
	g, store := branchWorld()

	got, ask, err := app.New(g, store).NextUnreviewed(evidenceDir)

	want := app.Question{Branch: "feat/x", Base: baseSHA, Head: headSHA}
	if err != nil || !ask || !same(got, want) {
		t.Fatalf("NextUnreviewed() = %+v, %v, %v; want %+v, true and no error", got, ask, err, want)
	}
	if wantGit := []string{"Locate " + evidenceDir, "Standing", "Parent " + headSHA}; len(g.calls) < 3 || !reflect.DeepEqual(g.calls[:3], wantGit) {
		t.Errorf("git calls = %q, want them to begin with %q", g.calls, wantGit)
	}
	if wantStore := []string{"Features " + fakeRepo.Root}; !reflect.DeepEqual(store.calls, wantStore) {
		t.Errorf("store calls = %q, want %q: it reads and stores nothing else when there is no feature", store.calls, wantStore)
	}
}

// What a commit is asked for after is the last commit the chain of reviews reaches: the base is its head. A result,
// an exemption and a result that blocks all count, because the question is whether a commit was reviewed, and
// 'review check' is the judge of how it turned out.
func TestNextUnreviewedAsksForTheCommitAfterTheLastOneReviewed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		store func(t *testing.T, store *fakeResults)
	}{
		{"a clear result", func(t *testing.T, store *fakeResults) { reviewed(t, store, baseSHA, headSHA, 1) }},
		{"an exemption", func(t *testing.T, store *fakeResults) { exempted(t, store, baseSHA, headSHA, 1) }},
		{"a result with a severe finding that nobody checked", func(t *testing.T, store *fakeResults) {
			reviewed(t, store, baseSHA, headSHA, 1, finding("F1", "BLOCKER"))
		}},
		{"a result that the verifier found blocks", func(t *testing.T, store *fakeResults) {
			name := reviewed(t, store, baseSHA, headSHA, 1, finding("F1", "BLOCKER"))
			verdictName, err := review.VerdictName(name)
			if err != nil {
				t.Fatalf("setup: %v", err)
			}
			store.put("own-rdd", verdictName, blocking)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := sweptWorld(t)
			tc.store(t, store)

			got, ask, err := app.New(g, store).NextUnreviewed(evidenceDir)

			want := app.Question{Branch: "feat/x", Base: headSHA, Head: otherSHA}
			if err != nil || !ask || !same(got, want) {
				t.Errorf("NextUnreviewed() = %+v, %v, %v; want %+v, true and no error", got, ask, err, want)
			}
		})
	}
}

// The reviews of every feature of the tree count, and the chain may cross from one feature to another.
func TestNextUnreviewedReadsTheReviewsOfEveryFeature(t *testing.T) {
	g, store := sweptWorld(t)
	reviewed(t, store, baseSHA, headSHA, 1)
	second := reviewed(t, store, headSHA, otherSHA, 1)
	store.put("another", second, store.files["own-rdd"][second])
	delete(store.files["own-rdd"], second)

	got, ask, err := app.New(g, store).NextUnreviewed(evidenceDir)

	if err != nil || ask || !nothing(got) {
		t.Errorf("NextUnreviewed() = %+v, %v, %v; want nothing to ask and no error", got, ask, err)
	}
}

// A record counts only when it is read back and holds: a file that is not a result or an exemption, and a result whose
// binding is not what git gives now, are no review of the commit.
func TestNextUnreviewedDoesNotCountARecordThatDoesNotHold(t *testing.T) {
	for _, tc := range []struct {
		name  string
		store func(t *testing.T, g *fakeGit, store *fakeResults)
	}{
		{"a file that is not a result", func(t *testing.T, g *fakeGit, store *fakeResults) {
			store.put("own-rdd", "222222222222-1.json", []byte("not a result"))
		}},
		{"a file that is not an exemption", func(t *testing.T, g *fakeGit, store *fakeResults) {
			store.put("own-rdd", "222222222222-1.exempt.json", []byte("not an exemption"))
		}},
		{"a result for a diff that is not the one git gives now", func(t *testing.T, g *fakeGit, store *fakeResults) {
			reviewed(t, store, baseSHA, headSHA, 1)
			g.diff = []byte("diff --git a/other.go b/other.go\n")
		}},
		{"an exemption for a diff that is not the one git gives now", func(t *testing.T, g *fakeGit, store *fakeResults) {
			exempted(t, store, baseSHA, headSHA, 1)
			g.diff = []byte("diff --git a/other.go b/other.go\n")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := sweptWorld(t)
			tc.store(t, g, store)

			got, ask, err := app.New(g, store).NextUnreviewed(evidenceDir)

			want := app.Question{Branch: "feat/x", Base: baseSHA, Head: headSHA}
			if err != nil || !ask || !same(got, want) {
				t.Errorf("NextUnreviewed() = %+v, %v, %v; want %+v, true and no error", got, ask, err, want)
			}
		})
	}
}

// thirdSHA is a commit after otherSHA, for the branches of three commits.
const thirdSHA = "5555555555555555555555555555555555555555"

// sweepPath is the repository-relative path of the first sweep record of the commit after base, in the feature own-rdd.
func sweepPath(t *testing.T, base, head string) string {
	t.Helper()
	return review.ResultPath("own-rdd", review.SweepName(candidateFor(t, base, head), 1))
}

// reviewMissing is the question for a commit that has everything but the review that covers it.
func reviewMissing(t *testing.T, base, head string) app.Question {
	t.Helper()
	return app.Question{
		Branch: "feat/x", Base: base, Head: head, Risk: "none", Sweep: sweepPath(t, base, head),
		Needs: []review.Need{{Missing: review.MissingReview, Err: review.ErrNoResult}},
	}
}

// lacks lists what a question says its commit lacks.
func lacks(q app.Question) []review.Missing {
	var kinds []review.Missing
	for _, need := range q.Needs {
		kinds = append(kinds, need.Missing)
	}
	return kinds
}

// A commit that only changes passive paths is never asked: it is a link of the chain, from its first parent to itself, so
// a review that starts at it is reached, and a branch of such commits has nothing to ask.
func TestNextUnreviewedNeverAsksForAPassiveCommit(t *testing.T) {
	passive := []string{"odd/tasks/own-rdd.md"}
	t.Run("a passive commit between two reviewed commits", func(t *testing.T) {
		g, store := newFakeGit(), newFakeResults()
		g.standing = app.Standing{Branch: "feat/x", Default: "main", Commits: []string{headSHA, otherSHA, thirdSHA}}
		g.parents = map[string]string{headSHA: baseSHA, otherSHA: headSHA, thirdSHA: otherSHA}
		g.declares(otherSHA, "none")
		g.declares(thirdSHA, "none")
		g.pathsOf = map[string][]string{otherSHA: passive}
		store.tasks = true
		sweptFor(t, store, "own-rdd", baseSHA, headSHA, 1)
		sweptFor(t, store, "own-rdd", otherSHA, thirdSHA, 1)
		reviewed(t, store, baseSHA, headSHA, 1)
		reviewed(t, store, otherSHA, thirdSHA, 1)

		got, ask, err := app.New(g, store).NextUnreviewed(evidenceDir)

		if err != nil || ask || !nothing(got) {
			t.Errorf("NextUnreviewed() = %+v, %v, %v; want nothing to ask and no error", got, ask, err)
		}
	})
	t.Run("a branch of passive commits only", func(t *testing.T) {
		g, store := branchWorld()
		g.paths = passive

		got, ask, err := app.New(g, store).NextUnreviewed(evidenceDir)

		if err != nil || ask || !nothing(got) {
			t.Errorf("NextUnreviewed() = %+v, %v, %v; want nothing to ask and no error", got, ask, err)
		}
	})
}

// A commit a review covers is asked again when it lacks a sweep record, with the commit before it as the base.
func TestNextUnreviewedAsksAgainForAReviewedCommitWithNoSweepRecord(t *testing.T) {
	g, store := branchWorld()
	reviewed(t, store, baseSHA, headSHA, 1)
	sweptFor(t, store, "own-rdd", headSHA, otherSHA, 1)

	got, ask, err := app.New(g, store).NextUnreviewed(evidenceDir)

	if err != nil || !ask || got.Base != baseSHA || got.Head != headSHA || got.Sweep != "" || got.Risk != "none" || !reflect.DeepEqual(lacks(got), []review.Missing{review.MissingSweep}) {
		t.Errorf("NextUnreviewed() = %+v, %v, %v; want the reviewed commit asked again for a sweep alone, Risk none and no sweep", got, ask, err)
	}
}

// A commit with no trailer lacks the risk and the sweep, and its risk is not told.
func TestNextUnreviewedAsksForACommitWithNoTrailer(t *testing.T) {
	g, store := branchWorld()
	g.messages[headSHA] = "subject with no trailer"

	got, ask, err := app.New(g, store).NextUnreviewed(evidenceDir)

	if err != nil || !ask || got.Head != headSHA || got.Risk != "" || !reflect.DeepEqual(lacks(got), []review.Missing{review.MissingRisk, review.MissingSweep}) {
		t.Errorf("NextUnreviewed() = %+v, %v, %v; want the commit asked for the risk and the sweep, with no Risk", got, ask, err)
	}
}

// The first unreviewed commit that has its sweep lacks the review alone, and the question names the sweep record.
func TestNextUnreviewedAsksForTheReviewOfACommitThatHasItsSweep(t *testing.T) {
	g, store := sweptWorld(t)

	got, ask, err := app.New(g, store).NextUnreviewed(evidenceDir)

	if want := reviewMissing(t, baseSHA, headSHA); err != nil || !ask || !reflect.DeepEqual(got, want) {
		t.Errorf("NextUnreviewed() = %+v, %v, %v; want %+v, true and no error", got, ask, err, want)
	}
}

// A reviewed commit whose sweep fired and that has no sweeper record is asked before a later commit nothing reviewed.
func TestNextUnreviewedAsksForTheSweeperOfAReviewedCommitBeforeALaterOne(t *testing.T) {
	g, store := branchWorld()
	reviewed(t, store, baseSHA, headSHA, 1)
	fired := sweptFor(t, store, "own-rdd", baseSHA, headSHA, 1, "a line of the working tree")
	sweptFor(t, store, "own-rdd", headSHA, otherSHA, 1)

	got, ask, err := app.New(g, store).NextUnreviewed(evidenceDir)

	if err != nil || !ask || got.Base != baseSHA || got.Head != headSHA || got.Sweep != review.ResultPath("own-rdd", fired) || !reflect.DeepEqual(lacks(got), []review.Missing{review.MissingSweeper}) {
		t.Errorf("NextUnreviewed() = %+v, %v, %v; want the reviewed commit asked for the sweeper of %s", got, ask, err, fired)
	}
}

// A sweep record that does not decode is not counted: the commit lacks its sweep.
func TestNextUnreviewedDoesNotCountASweepRecordThatDoesNotDecode(t *testing.T) {
	g, store := sweptWorld(t)
	store.put("own-rdd", review.SweepName(candidateFor(t, baseSHA, headSHA), 1), []byte("not json"))

	got, ask, err := app.New(g, store).NextUnreviewed(evidenceDir)

	if err != nil || !ask || got.Head != headSHA || got.Sweep != "" || !reflect.DeepEqual(lacks(got), []review.Missing{review.MissingSweep, review.MissingReview}) {
		t.Errorf("NextUnreviewed() = %+v, %v, %v; want the commit asked for a sweep and a review", got, ask, err)
	}
}

// A commit that declares an item lacks the polisher record, listed before the review.
func TestNextUnreviewedAsksForThePolisherOfACommitThatDeclaresAnItem(t *testing.T) {
	g, store := sweptWorld(t)
	g.declares(headSHA, "3,5")

	got, ask, err := app.New(g, store).NextUnreviewed(evidenceDir)

	if err != nil || !ask || got.Head != headSHA || got.Risk != "3,5" || !reflect.DeepEqual(lacks(got), []review.Missing{review.MissingPolisher, review.MissingReview}) {
		t.Errorf("NextUnreviewed() = %+v, %v, %v; want the commit asked for a polisher and a review, with Risk 3,5", got, ask, err)
	}
}

// Nothing to ask is no question and no error.
func TestNextUnreviewedHasNothingToAskWhenThereIsNothingToReview(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, g *fakeGit, store *fakeResults)
		git   []string // the calls of git: the ones that decide it and no more
	}{
		{"the tree keeps no features", func(t *testing.T, g *fakeGit, store *fakeResults) { store.tasks = false }, []string{"Locate " + evidenceDir}},
		{"HEAD is detached", func(t *testing.T, g *fakeGit, store *fakeResults) { g.standing = app.Standing{} }, []string{"Locate " + evidenceDir, "Standing"}},
		{"HEAD is on the default branch", func(t *testing.T, g *fakeGit, store *fakeResults) {
			g.standing = app.Standing{Branch: "main", Default: "main"}
		}, []string{"Locate " + evidenceDir, "Standing"}},
		{"every commit is reviewed one by one", func(t *testing.T, g *fakeGit, store *fakeResults) {
			reviewed(t, store, baseSHA, headSHA, 1)
			exempted(t, store, headSHA, otherSHA, 1)
			sweptBranch(t, store)
		}, nil},
		{"one review covers both commits", func(t *testing.T, g *fakeGit, store *fakeResults) {
			reviewed(t, store, baseSHA, otherSHA, 1)
			sweptBranch(t, store)
		}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := branchWorld()
			tc.setup(t, g, store)

			got, ask, err := app.New(g, store).NextUnreviewed(evidenceDir)

			if err != nil || ask || !nothing(got) {
				t.Errorf("NextUnreviewed() = %+v, %v, %v; want nothing to ask and no error", got, ask, err)
			}
			if tc.git != nil && !reflect.DeepEqual(g.calls, tc.git) {
				t.Errorf("git calls = %q, want %q", g.calls, tc.git)
			}
			if n := store.created(); n != 0 {
				t.Errorf("%d files were stored, want none", n)
			}
		})
	}
}

// A repository that does not tell its default branch cannot say which commits are the branch's own: that is told, and
// is not the same as having nothing to ask.
func TestNextUnreviewedSaysWhenTheDefaultBranchCannotBeFound(t *testing.T) {
	g, store := branchWorld()
	g.standing = app.Standing{Branch: "feat/x"}

	got, ask, err := app.New(g, store).NextUnreviewed(evidenceDir)

	if !errors.Is(err, app.ErrNoDefaultBranch) || !strings.Contains(err.Error(), "feat/x") || ask || !nothing(got) {
		t.Errorf("NextUnreviewed() = %+v, %v, %v; want nothing to ask and an error that is ErrNoDefaultBranch and names the branch", got, ask, err)
	}
}

// What cannot be told is an error and not a branch with nothing to ask: git or the store failing, and a first commit
// that has no parent, which leaves no base to name.
func TestNextUnreviewedReportsWhatItCannotTell(t *testing.T) {
	boom := errors.New("boom")
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, g *fakeGit, store *fakeResults)
		want  string
	}{
		{"git cannot find the repository", func(t *testing.T, g *fakeGit, store *fakeResults) { g.fail["Locate"] = boom }, "locate the repository"},
		{"git cannot say where HEAD stands", func(t *testing.T, g *fakeGit, store *fakeResults) { g.fail["Standing"] = boom }, "where HEAD stands"},
		{"git cannot name the parent of the first commit", func(t *testing.T, g *fakeGit, store *fakeResults) { g.fail["Parent"] = boom }, "parent of " + headSHA},
		{"the store cannot say what the tree keeps", func(t *testing.T, g *fakeGit, store *fakeResults) { store.fail["Features"] = boom }, "features"},
		{"the store cannot list a feature", func(t *testing.T, g *fakeGit, store *fakeResults) {
			reviewed(t, store, baseSHA, headSHA, 1)
			store.fail["Names"] = boom
		}, "own-rdd"},
		{"the first commit has no parent", func(t *testing.T, g *fakeGit, store *fakeResults) { g.parents = map[string]string{} }, "no parent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := branchWorld()
			tc.setup(t, g, store)

			got, ask, err := app.New(g, store).NextUnreviewed(evidenceDir)

			if err == nil || !strings.Contains(err.Error(), tc.want) || ask || !nothing(got) {
				t.Errorf("NextUnreviewed() = %+v, %v, %v; want nothing to ask and an error that says %q", got, ask, err, tc.want)
			}
		})
	}
}
