package app_test

// What CheckLanding decides about the merge of a pull request (own-rdd T7a, S7 b-e): the facts come through the Git port,
// the chain of each feature the merge changes is judged by CheckChain, and the tree that landed is held to the tree of
// the reviewed head. The tests go over the fakes.

import (
	"errors"
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// mergeSHA is the merge the fakes land: the first parent is baseSHA, the second headSHA, and the one merge base is baseSHA.
const mergeSHA = thirdSHA

// landed makes the fakes of a merge of a branch of one passive commit, headSHA, that adds the feature document f onto an
// unmoved baseSHA: a passive commit is a link of a chain with no record, so feature f clears, and the trees are equal.
func landed() (*fakeGit, *fakeResults) {
	g, store := newFakeGit(), newFakeResults()
	g.allParents = map[string][]string{mergeSHA: {baseSHA, headSHA}}
	g.mergeBases = map[string][]string{baseSHA + " " + headSHA: {baseSHA}}
	g.trees = map[string]string{mergeSHA: "tree-of-head", headSHA: "tree-of-head"}
	g.paths = []string{"odd/tasks/f.md"}
	return g, store
}

// reviewedFor stores a clean sweep and a clear result of the commit baseSHA..headSHA as they are bound to the paths and
// the diff the fake git gives, under feature.
func reviewedFor(t *testing.T, g *fakeGit, store *fakeResults, feature string) {
	t.Helper()
	resweep(t, g, store)
	c := candidateOf(t, g)
	file, err := review.EncodeResult(c, []string{"default"}, replyFor(t, c))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	store.put("own-rdd", review.ResultName(c, 1), file)
	store.files[feature] = store.files["own-rdd"]
	delete(store.files, "own-rdd")
}

// codePaths are the paths of a branch that is not passive and changes the documents of the features a and b.
var codePaths = []string{"engine/guard.go", "odd/tasks/a.md", "odd/tasks/b.md"}

func TestCheckLandingVerifiesAMergeWhoseFeatureChainClearsAndWhoseTreeIsTheHeadsTree(t *testing.T) {
	g, store := landed()

	got, err := app.New(g, store).CheckLanding(evidenceDir, mergeSHA)

	want := app.Landing{
		Merge: mergeSHA, Base: baseSHA, Head: headSHA, Tree: "tree-of-head", Outcome: app.OutcomeVerified,
		Features: []app.LandedFeature{{Feature: "f", Outcome: app.FeatureClear}},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("CheckLanding() = %+v, %v; want %+v", got, err, want)
	}
	if store.created() != 0 {
		t.Errorf("the store was written %d times, want nothing stored", store.created())
	}
}

func TestCheckLandingRefusesACommitThatIsNotAMergeBeforeItReadsAnyPathOrStore(t *testing.T) {
	g, store := landed()
	g.allParents[mergeSHA] = []string{baseSHA}

	got, err := app.New(g, store).CheckLanding(evidenceDir, mergeSHA)

	if !errors.Is(err, review.ErrNotAMerge) || !reflect.DeepEqual(got, app.Landing{}) {
		t.Errorf("CheckLanding() = %+v, %v; want no landing and an error that wraps %v", got, err, review.ErrNotAMerge)
	}
	if g.called("Paths") != 0 || g.called("MergeBases") != 0 || g.called("Tree") != 0 || len(store.calls) != 0 {
		t.Errorf("git calls %q, store calls %q; want no merge base, path, tree or store call", g.calls, store.calls)
	}
}

func TestCheckLandingRefusesWhenThePairOfParentsDoesNotHaveExactlyOneMergeBase(t *testing.T) {
	for name, bases := range map[string][]string{"none": nil, "two": {baseSHA, otherSHA}} {
		t.Run(name, func(t *testing.T) {
			g, store := landed()
			g.mergeBases[baseSHA+" "+headSHA] = bases

			got, err := app.New(g, store).CheckLanding(evidenceDir, mergeSHA)

			if !errors.Is(err, review.ErrMergeBase) || !reflect.DeepEqual(got, app.Landing{}) || g.called("Paths") != 0 {
				t.Errorf("CheckLanding() = %+v, %v; want no landing, %v and no Paths call", got, err, review.ErrMergeBase)
			}
		})
	}
}

func TestCheckLandingPassesAMergeThatChangesNoFeatureDocument(t *testing.T) {
	g, store := landed()
	g.paths = []string{"README.md", "odd/review-results/a/x.json"}

	got, err := app.New(g, store).CheckLanding(evidenceDir, mergeSHA)

	want := app.Landing{Merge: mergeSHA, Base: baseSHA, Head: headSHA, Outcome: app.OutcomeNoFeature, Features: []app.LandedFeature{}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("CheckLanding() = %+v, %v; want %+v", got, err, want)
	}
	if len(store.calls) != 0 || g.called("Tree") != 0 {
		t.Errorf("store calls %q, git calls %q; want no store call and no tree read", store.calls, g.calls)
	}
}

func TestCheckLandingChecksEveryFeatureAndClearsWhenAtLeastOneChainClears(t *testing.T) {
	g, store := landed()
	g.paths = codePaths
	reviewedFor(t, g, store, "b")
	_, refusal := app.New(g, store).CheckChain(evidenceDir, "a", baseSHA, headSHA)
	if refusal == nil {
		t.Fatal("setup: the chain of the feature a should not clear")
	}

	got, err := app.New(g, store).CheckLanding(evidenceDir, mergeSHA)

	want := []app.LandedFeature{
		{Feature: "a", Outcome: app.FeatureRefused, Reason: refusal.Error()},
		{Feature: "b", Outcome: app.FeatureClear},
	}
	if err != nil || got.Outcome != app.OutcomeVerified || !reflect.DeepEqual(got.Features, want) {
		t.Errorf("CheckLanding() = %+v, %v; want verified with the features %+v", got, err, want)
	}
}

func TestCheckLandingRefusesWithEveryReasonWhenNoFeatureChainClears(t *testing.T) {
	g, store := landed()
	g.paths = codePaths
	service := app.New(g, store)
	_, reasonA := service.CheckChain(evidenceDir, "a", baseSHA, headSHA)
	_, reasonB := service.CheckChain(evidenceDir, "b", baseSHA, headSHA)
	if reasonA == nil || reasonB == nil {
		t.Fatal("setup: no chain should clear")
	}

	got, err := service.CheckLanding(evidenceDir, mergeSHA)

	want := review.NoFeatureClears(baseSHA, headSHA, []string{"a", "b"}, []error{reasonA, reasonB})
	if !errors.Is(err, review.ErrNoFeatureClears) || err.Error() != want.Error() || !reflect.DeepEqual(got, app.Landing{}) {
		t.Errorf("CheckLanding() = %+v, %v; want no landing and the refusal %v", got, err, want)
	}
	if g.called("Tree") != 0 {
		t.Errorf("git calls %q; want no tree read once no chain clears", g.calls)
	}
}

func TestCheckLandingRefusesATreeOtherThanTheReviewedHeads(t *testing.T) {
	g, store := landed()
	g.trees[mergeSHA] = "tree-of-merge"

	got, err := app.New(g, store).CheckLanding(evidenceDir, mergeSHA)

	want := review.CheckLandedTree(mergeSHA, "tree-of-merge", headSHA, "tree-of-head", baseSHA)
	if !errors.Is(err, review.ErrTreeMoved) || err.Error() != want.Error() || !reflect.DeepEqual(got, app.Landing{}) {
		t.Errorf("CheckLanding() = %+v, %v; want no landing and the refusal %v", got, err, want)
	}
}

func TestCheckLandingNeverPassesWhenGitCannotAnswer(t *testing.T) {
	for _, step := range []string{"Locate", "Commit " + mergeSHA, "Parents", "MergeBases", "Paths", "Tree"} {
		t.Run(step, func(t *testing.T) {
			g, store := landed()
			boom := errors.New("git failed: " + step)
			g.fail[step] = boom

			got, err := app.New(g, store).CheckLanding(evidenceDir, mergeSHA)

			if !errors.Is(err, boom) || !reflect.DeepEqual(got, app.Landing{}) {
				t.Errorf("CheckLanding() = %+v, %v; want no landing and an error that wraps %v", got, err, boom)
			}
		})
	}
}

func TestCheckLandingRefusesARevisionGitMustNotBeGiven(t *testing.T) {
	g, store := landed()

	_, err := app.New(g, store).CheckLanding(evidenceDir, "--output=x")

	if !errors.Is(err, review.ErrInvalidRev) || len(g.calls) != 0 {
		t.Errorf("CheckLanding() error = %v, git calls %q; want %v and no git call", err, g.calls, review.ErrInvalidRev)
	}
}

// The port gained three methods (Parents, MergeBases, Tree) and Check does not use them: a head is still cleared as it was
// before, with the calls it made.
func TestCheckStillClearsAPassiveHeadWithoutAskingForParentsMergeBasesOrTrees(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	g.paths = []string{"odd/tasks/own-rdd.md"}

	got, err := app.New(g, store).Check(evidenceDir, "own-rdd", "HEAD")

	if err != nil || !reflect.DeepEqual(got, app.Checked{Head: headSHA}) {
		t.Errorf("Check() = %+v, %v; want the head cleared with no record", got, err)
	}
	if g.called("Parents") != 0 || g.called("MergeBases") != 0 || g.called("Tree") != 0 {
		t.Errorf("git calls %q; want none of Parents, MergeBases or Tree", g.calls)
	}
}
