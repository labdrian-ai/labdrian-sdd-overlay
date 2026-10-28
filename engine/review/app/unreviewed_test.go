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
	store.tasks = true
	return g, store
}

// A branch no review covers is asked for from its first commit, with the commit the branch was made on as the base.
func TestNextUnreviewedAsksForTheFirstCommitOfABranchNoReviewCovers(t *testing.T) {
	g, store := branchWorld()

	got, ask, err := app.New(g, store).NextUnreviewed(evidenceDir)

	want := app.Question{Branch: "feat/x", Base: baseSHA, Head: headSHA}
	if err != nil || !ask || got != want {
		t.Fatalf("NextUnreviewed() = %+v, %v, %v; want %+v, true and no error", got, ask, err, want)
	}
	if wantGit := []string{"Locate " + evidenceDir, "Standing", "Parent " + headSHA}; !reflect.DeepEqual(g.calls, wantGit) {
		t.Errorf("git calls = %q, want %q", g.calls, wantGit)
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
			g, store := branchWorld()
			tc.store(t, store)

			got, ask, err := app.New(g, store).NextUnreviewed(evidenceDir)

			want := app.Question{Branch: "feat/x", Base: headSHA, Head: otherSHA}
			if err != nil || !ask || got != want {
				t.Errorf("NextUnreviewed() = %+v, %v, %v; want %+v, true and no error", got, ask, err, want)
			}
		})
	}
}

// The reviews of every feature of the tree count, and the chain may cross from one feature to another.
func TestNextUnreviewedReadsTheReviewsOfEveryFeature(t *testing.T) {
	g, store := branchWorld()
	reviewed(t, store, baseSHA, headSHA, 1)
	second := reviewed(t, store, headSHA, otherSHA, 1)
	store.put("another", second, store.files["own-rdd"][second])
	delete(store.files["own-rdd"], second)

	got, ask, err := app.New(g, store).NextUnreviewed(evidenceDir)

	if err != nil || ask || got != (app.Question{}) {
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
			g, store := branchWorld()
			tc.store(t, g, store)

			got, ask, err := app.New(g, store).NextUnreviewed(evidenceDir)

			want := app.Question{Branch: "feat/x", Base: baseSHA, Head: headSHA}
			if err != nil || !ask || got != want {
				t.Errorf("NextUnreviewed() = %+v, %v, %v; want %+v, true and no error", got, ask, err, want)
			}
		})
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
		}, nil},
		{"one review covers both commits", func(t *testing.T, g *fakeGit, store *fakeResults) { reviewed(t, store, baseSHA, otherSHA, 1) }, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := branchWorld()
			tc.setup(t, g, store)

			got, ask, err := app.New(g, store).NextUnreviewed(evidenceDir)

			if err != nil || ask || got != (app.Question{}) {
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

	if !errors.Is(err, app.ErrNoDefaultBranch) || !strings.Contains(err.Error(), "feat/x") || ask || got != (app.Question{}) {
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

			if err == nil || !strings.Contains(err.Error(), tc.want) || ask || got != (app.Question{}) {
				t.Errorf("NextUnreviewed() = %+v, %v, %v; want nothing to ask and an error that says %q", got, ask, err, tc.want)
			}
		})
	}
}
