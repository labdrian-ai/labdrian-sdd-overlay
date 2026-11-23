package app_test

// What KeptResults and Stop do for the learning loop (S6, L55 T4h): the stored results of the branch's commits whose kept
// findings (the rules of Findings) are not classified yet are asked for once per session and result, when no review
// question blocks. The Classified port is a fake here. The branch of the fakes is the one of unreviewed_test.go; a result
// of the whole branch (baseSHA to otherSHA) is the review of both commits, so no review question is put.

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// fakeClassified is the Classified port over a map: the review identifiers whose evidence rows exist.
type fakeClassified struct {
	done  map[string]bool
	err   error
	calls []string // "<root> <review>"
}

func newFakeClassified() *fakeClassified { return &fakeClassified{done: map[string]bool{}} }

func (c *fakeClassified) Classified(root, review string) (bool, error) {
	c.calls = append(c.calls, root+" "+review)
	if c.err != nil {
		return false, c.err
	}
	return c.done[review], nil
}

// warning is a finding that stays evidence: a WARNING the diff introduced.
func warning(id string) map[string]any { return finding(id, "WARNING") }

// classifyWorld is a branch whose commits lack nothing but have a stored result of the whole branch with one kept WARNING.
func classifyWorld(t *testing.T) (*fakeGit, *fakeResults, string) {
	t.Helper()
	g, store := sweptWorld(t)
	return g, store, reviewed(t, store, baseSHA, otherSHA, 1, warning("F1"))
}

const (
	otherReview = "own-rdd/333333333333-1" // the identifier of the result of otherSHA
	headReview  = "own-rdd/222222222222-1"
)

// classifyKey is the key of the question that asks for review: classify- and the first 16 hex digits of the SHA-256 of
// the identifier, written here without the code under test.
func classifyKey(review string) string { return "classify-" + sha256Of([]byte(review))[:16] }

// SH1: a result with a kept finding that the page lacks blocks the stop and is named.
func TestStopAsksForTheClassificationOfAResultWithAKeptFinding(t *testing.T) {
	g, store, _ := classifyWorld(t)
	asked, port := newFakeAsked(), newFakeClassified()

	got, err := app.New(g, store).WithAsked(asked).WithClassified(port).Stop(evidenceDir, "session-1", false)

	want := app.Ruling{Block: true, Classify: []app.KeptResult{{Review: otherReview, Head: otherSHA, Kept: 1}}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Stop() = %+v, %v; want %+v and no error", got, err, want)
	}
	if wantCalls := []string{fakeRepo.Root + " " + otherReview}; !reflect.DeepEqual(port.calls, wantCalls) {
		t.Errorf("port calls = %q, want %q", port.calls, wantCalls)
	}
	if !asked.put["session-1 "+classifyKey(otherReview)] || len(asked.put) != 1 {
		t.Errorf("remembered = %v, want only %s of session-1", asked.put, classifyKey(otherReview))
	}
}

// SH2: a finding that is not evidence is no kept finding: nothing is asked and the port is not called.
func TestStopDoesNotAskForAResultWithNoKeptFinding(t *testing.T) {
	for _, tc := range []struct {
		name  string
		store func(t *testing.T, store *fakeResults)
	}{
		{"every finding is pre-existing", func(t *testing.T, store *fakeResults) {
			f := warning("F1")
			f["causal_disposition"] = "pre-existing"
			reviewed(t, store, baseSHA, otherSHA, 1, f)
		}},
		{"a BLOCKER the verifier found not reproduced", func(t *testing.T, store *fakeResults) {
			name := reviewed(t, store, baseSHA, otherSHA, 1, finding("F1", "BLOCKER"))
			verdictName, err := review.VerdictName(name)
			if err != nil {
				t.Fatalf("setup: %v", err)
			}
			store.put("own-rdd", verdictName, verdictReply(verdict("F1", "not-reproduced")))
		}},
		{"no finding", func(t *testing.T, store *fakeResults) { reviewed(t, store, baseSHA, otherSHA, 1) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := sweptWorld(t)
			tc.store(t, store)
			port := newFakeClassified()

			got, err := app.New(g, store).WithAsked(newFakeAsked()).WithClassified(port).Stop(evidenceDir, "session-1", false)

			if err != nil || !reflect.DeepEqual(got, app.Ruling{}) || len(port.calls) != 0 {
				t.Errorf("Stop() = %+v, %v with %q asked of the port; want no block, no error and no call", got, err, port.calls)
			}
		})
	}
}

// SH3: a result the page holds is not asked for, and is remembered only as classified.
func TestStopDoesNotAskForAClassifiedResult(t *testing.T) {
	g, store, _ := classifyWorld(t)
	asked, port := newFakeAsked(), newFakeClassified()
	port.done[otherReview] = true

	got, err := app.New(g, store).WithAsked(asked).WithClassified(port).Stop(evidenceDir, "session-1", false)

	if err != nil || got.Block || len(asked.put) != 1 || !asked.put["session-1 "+classifiedKey(otherReview)] {
		t.Errorf("Stop() = %+v, %v with %v remembered; want no block, no error and only %s remembered", got, err, asked.put, classifiedKey(otherReview))
	}
}

// SH4: the question is put once per session; the port is asked again at the next stop, which does not block, and a
// result still lacking is named in another session.
func TestStopAsksForAResultOnceForEachSession(t *testing.T) {
	g, store, _ := classifyWorld(t)
	asked, port := newFakeAsked(), newFakeClassified()
	service := app.New(g, store).WithAsked(asked).WithClassified(port)
	if r, err := service.Stop(evidenceDir, "session-1", false); err != nil || !r.Block {
		t.Fatalf("setup: Stop() = %+v, %v; want a block", r, err)
	}

	again, againErr := service.Stop(evidenceDir, "session-1", false)

	if againErr != nil || again.Block {
		t.Errorf("the second Stop() of the session = %+v, %v; want no block and no error", again, againErr)
	}
	if len(port.calls) != 2 {
		t.Errorf("port calls = %q, want two: a result still lacking is asked of the port at every stop", port.calls)
	}
	other, otherErr := service.Stop(evidenceDir, "session-2", false)
	if otherErr != nil || !other.Block || len(other.Classify) != 1 || len(port.calls) != 3 {
		t.Errorf("Stop() of another session = %+v, %v with %q asked of the port; want a block naming the result and a second call", other, otherErr, port.calls)
	}
}

// SH5: what the port cannot tell is an error and no block, and nothing is remembered: a block that could not be
// justified would be a trap.
func TestStopDoesNotBlockWhenTheClassifiedPortCannotTell(t *testing.T) {
	g, store, _ := classifyWorld(t)
	asked, port := newFakeAsked(), newFakeClassified()
	port.err = errors.New("boom")

	got, err := app.New(g, store).WithAsked(asked).WithClassified(port).Stop(evidenceDir, "session-1", false)

	if err == nil || !strings.Contains(err.Error(), "boom") || got.Block || len(asked.put) != 0 {
		t.Errorf("Stop() = %+v, %v with %v remembered; want no block, an error that says boom and nothing remembered", got, err, asked.put)
	}
}

// SH6: a commit that also lacks its review: the review question is the block, and nothing is asked of the port. The
// classification is asked for at the stop after, once the review question was put.
func TestStopPutsTheReviewQuestionBeforeTheClassificationQuestion(t *testing.T) {
	g, store := sweptWorld(t)
	reviewed(t, store, baseSHA, headSHA, 1, warning("F1")) // otherSHA has no review
	asked, port := newFakeAsked(), newFakeClassified()
	service := app.New(g, store).WithAsked(asked).WithClassified(port)

	first, err := service.Stop(evidenceDir, "session-1", false)

	if err != nil || !first.Block || len(first.Classify) != 0 || len(port.calls) != 0 {
		t.Fatalf("the first Stop() = %+v, %v with %q asked of the port; want the review question and no classification", first, err, port.calls)
	}
	second, err := service.Stop(evidenceDir, "session-1", false)
	want := app.Ruling{Block: true, Classify: []app.KeptResult{{Review: headReview, Head: headSHA, Kept: 1}}}
	if err != nil || !reflect.DeepEqual(second, want) {
		t.Errorf("the second Stop() = %+v, %v; want %+v", second, err, want)
	}
}

// SH7: two results of one commit, both with kept findings: both are named, in the order of their numbers, each with a key
// of its own.
func TestStopNamesEveryResultOfACommitWithItsOwnKey(t *testing.T) {
	g, store, _ := classifyWorld(t)
	reviewed(t, store, baseSHA, otherSHA, 2, warning("F1"), warning("F2"))
	asked, port := newFakeAsked(), newFakeClassified()

	got, err := app.New(g, store).WithAsked(asked).WithClassified(port).Stop(evidenceDir, "session-1", false)

	want := []app.KeptResult{{Review: otherReview, Head: otherSHA, Kept: 1}, {Review: "own-rdd/333333333333-2", Head: otherSHA, Kept: 2}}
	if err != nil || !got.Block || !reflect.DeepEqual(got.Classify, want) {
		t.Fatalf("Stop() = %+v, %v; want a block naming %+v", got, err, want)
	}
	if len(asked.put) != 2 || !asked.put["session-1 "+classifyKey(otherReview)] || !asked.put["session-1 "+classifyKey("own-rdd/333333333333-2")] {
		t.Errorf("remembered = %v, want one key for each of the two results", asked.put)
	}
}

// A Service made without a Classified port asks for no classification, as before T4h.
func TestStopWithNoClassifiedPortAsksForNoClassification(t *testing.T) {
	g, store, _ := classifyWorld(t)

	got, err := app.New(g, store).WithAsked(newFakeAsked()).Stop(evidenceDir, "session-1", false)

	if err != nil || !reflect.DeepEqual(got, app.Ruling{}) {
		t.Errorf("Stop() = %+v, %v; want no block and no error", got, err)
	}
}

// KeptResults names the results of the commits of the branch, oldest commit first, and nothing for a repository that
// cannot tell which commits are the branch's own.
func TestKeptResultsNamesTheResultsOfTheBranchCommitsOldestFirst(t *testing.T) {
	g, store, _ := classifyWorld(t)
	reviewed(t, store, baseSHA, headSHA, 1, warning("F1"))

	got, err := app.New(g, store).KeptResults(evidenceDir)

	want := []app.KeptResult{{Review: headReview, Head: headSHA, Kept: 1}, {Review: otherReview, Head: otherSHA, Kept: 1}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("KeptResults() = %+v, %v; want %+v", got, err, want)
	}
}

func TestKeptResultsIsEmptyWhereThereAreNoBranchCommitsToRead(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(g *fakeGit, store *fakeResults)
	}{
		{"a tree that keeps no features", func(g *fakeGit, store *fakeResults) { store.tasks, store.files = false, map[string]map[string][]byte{} }},
		{"a detached HEAD", func(g *fakeGit, store *fakeResults) { g.standing = app.Standing{} }},
		{"no default branch", func(g *fakeGit, store *fakeResults) { g.standing = app.Standing{Branch: "feat/x"} }},
		{"no commit of its own", func(g *fakeGit, store *fakeResults) {
			g.standing = app.Standing{Branch: "main", Default: "main"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store, _ := classifyWorld(t)
			tc.setup(g, store)

			got, err := app.New(g, store).KeptResults(evidenceDir)

			if err != nil || len(got) != 0 {
				t.Errorf("KeptResults() = %+v, %v; want nothing and no error", got, err)
			}
		})
	}
}

// J1: what the pass cannot read, hold or bind is an error and no result, as Check refuses the same file; it is never
// skipped.
func TestKeptResultsRefusesWhatItCannotReadHoldOrBind(t *testing.T) {
	cause := errors.New("boom")
	verdictName := "333333333333-1.verifier.json"
	for _, tc := range []struct {
		name  string
		setup func(g *fakeGit, store *fakeResults)
		wraps error
		names string
	}{
		{"a file that is not a result", func(g *fakeGit, store *fakeResults) {
			store.put("own-rdd", "333333333333-2.json", []byte("not a result"))
		}, review.ErrInvalidStored, "333333333333-2.json"},
		{"a result for a diff that is not the one git gives now", func(g *fakeGit, store *fakeResults) {
			g.diff = []byte("diff --git a/other.go b/other.go\n")
		}, review.ErrBindingDiffers, ""},
		{"git cannot give the diff", func(g *fakeGit, store *fakeResults) { g.fail["Diff"] = cause }, cause, ""},
		{"the verdict cannot be read", func(g *fakeGit, store *fakeResults) { store.fail["Read "+verdictName] = cause }, cause, verdictName},
		{"the store cannot give the features", func(g *fakeGit, store *fakeResults) { store.fail["Features"] = cause }, cause, ""},
		{"git cannot tell where HEAD stands", func(g *fakeGit, store *fakeResults) { g.fail["Standing"] = cause }, cause, ""},
		{"the store cannot list the results", func(g *fakeGit, store *fakeResults) { store.fail["Names"] = cause }, cause, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store, _ := classifyWorld(t)
			tc.setup(g, store)

			got, err := app.New(g, store).KeptResults(evidenceDir)

			if got != nil || err == nil || !errors.Is(err, tc.wraps) || !strings.Contains(err.Error(), tc.names) {
				t.Errorf("KeptResults() = %+v, %v; want nothing and an error that wraps %v and names %q", got, err, tc.wraps, tc.names)
			}
		})
	}
}

// J2: a result whose file is named for one commit and whose binding is about another is skipped with no error: only the
// first 12 characters of its name said it was about otherSHA.
func TestKeptResultsSkipsAResultAboutAnotherCommit(t *testing.T) {
	g, store, _ := classifyWorld(t)
	name := reviewed(t, store, baseSHA, headSHA, 1, warning("F1")) // 222222222222-1.json, bound to headSHA
	store.put("own-rdd", "333333333333-2.json", store.files["own-rdd"][name])
	delete(store.files["own-rdd"], name)

	got, err := app.New(g, store).KeptResults(evidenceDir)

	want := []app.KeptResult{{Review: otherReview, Head: otherSHA, Kept: 1}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("KeptResults() = %+v, %v; want %+v and no error", got, err, want)
	}
}

// flakyGit asks nothing different of git until Standing was asked twice: the review part of Stop binds fine and the
// classification part is the second to ask.
type flakyGit struct {
	*fakeGit
	standings int
}

func (f *flakyGit) Standing(repo app.Repository) (app.Standing, error) {
	f.standings++
	return f.fakeGit.Standing(repo)
}

// flakyResults cannot read one file once the classification part of Stop runs.
type flakyResults struct {
	*fakeResults
	git  *flakyGit
	name string
}

func (r *flakyResults) Read(root, feature, name string) ([]byte, error) {
	if r.git.standings >= 2 && name == r.name {
		return nil, errors.New("permission denied")
	}
	return r.fakeResults.Read(root, feature, name)
}

// J3: a result the classification pass cannot read is an error and no block, and the port is not asked.
func TestStopDoesNotBlockWhenTheClassificationPassCannotRead(t *testing.T) {
	g, store, _ := classifyWorld(t)
	git := &flakyGit{fakeGit: g}
	results := &flakyResults{fakeResults: store, git: git, name: "333333333333-1.verifier.json"}
	port := newFakeClassified()

	got, err := app.New(git, results).WithAsked(newFakeAsked()).WithClassified(port).Stop(evidenceDir, "session-1", false)

	if err == nil || !strings.Contains(err.Error(), "333333333333-1.verifier.json") || got.Block || len(port.calls) != 0 {
		t.Errorf("Stop() = %+v, %v with %q asked of the port; want no block, an error naming the verdict and no call", got, err, port.calls)
	}
}

// J4: the questions of the session that cannot be read are an error and no block, and the port is not asked.
func TestStopDoesNotBlockWhenTheQuestionsOfTheSessionCannotBeRead(t *testing.T) {
	g, store, _ := classifyWorld(t)
	asked, port := newFakeAsked(), newFakeClassified()
	cause := errors.New("boom")
	asked.fail["Has"] = cause

	got, err := app.New(g, store).WithAsked(asked).WithClassified(port).Stop(evidenceDir, "session-1", false)

	if !errors.Is(err, cause) || got.Block || len(port.calls) != 0 {
		t.Errorf("Stop() = %+v, %v with %q asked of the port; want no block, an error that wraps the cause and no call", got, err, port.calls)
	}
}

// J5: a question that cannot be recorded is an error and no block, for a result the page lacks and for one it holds.
func TestStopDoesNotBlockWhenAQuestionCannotBeRecorded(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(port *fakeClassified)
	}{
		{"an unclassified result", func(port *fakeClassified) {}},
		{"a classified result", func(port *fakeClassified) { port.done[otherReview] = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store, _ := classifyWorld(t)
			asked, port := newFakeAsked(), newFakeClassified()
			tc.setup(port)
			cause := errors.New("boom")
			asked.fail["Add"] = cause

			got, err := app.New(g, store).WithAsked(asked).WithClassified(port).Stop(evidenceDir, "session-1", false)

			if !errors.Is(err, cause) || got.Block {
				t.Errorf("Stop() = %+v, %v; want no block and an error that wraps the cause", got, err)
			}
		})
	}
}

// classifiedKey is the key under which a session remembers that review is classified, written here without the code
// under test.
func classifiedKey(review string) string { return "classified-" + sha256Of([]byte(review))[:16] }

// J6: a result found classified is asked of the port once for each session, and remembered under its own key.
func TestStopAsksThePortAboutAClassifiedResultOnceForEachSession(t *testing.T) {
	g, store, _ := classifyWorld(t)
	asked, port := newFakeAsked(), newFakeClassified()
	port.done[otherReview] = true
	service := app.New(g, store).WithAsked(asked).WithClassified(port)

	for i, step := range []struct {
		session string
		calls   int
	}{{"session-1", 1}, {"session-1", 1}, {"session-2", 2}} {
		got, err := service.Stop(evidenceDir, step.session, false)
		if err != nil || got.Block || len(port.calls) != step.calls {
			t.Errorf("stop %d of %s = %+v, %v with %d port calls; want no block, no error and %d calls", i+1, step.session, got, err, len(port.calls), step.calls)
		}
	}
	if !asked.put["session-1 "+classifiedKey(otherReview)] || asked.put["session-1 "+classifyKey(otherReview)] {
		t.Errorf("remembered = %v, want %s of session-1 and no classify- key", asked.put, classifiedKey(otherReview))
	}
}

// secondAddFails is an Asked store whose second Add fails, once.
type secondAddFails struct {
	*fakeAsked
	adds int
}

func (a *secondAddFails) Add(session, key string) error {
	a.adds++
	if a.adds == 2 {
		return errors.New("disk full")
	}
	return a.fakeAsked.Add(session, key)
}

// J7: when the record of a block was lost half way, the next block names every result still lacking, the one asked
// before included, and the stop after is not blocked.
func TestStopNamesAgainEveryResultThatALostRecordLeftUnasked(t *testing.T) {
	g, store, _ := classifyWorld(t)
	reviewed(t, store, baseSHA, otherSHA, 2, warning("F1"), warning("F2"))
	asked, port := &secondAddFails{fakeAsked: newFakeAsked()}, newFakeClassified()
	service := app.New(g, store).WithAsked(asked).WithClassified(port)

	first, firstErr := service.Stop(evidenceDir, "session-1", false)
	second, secondErr := service.Stop(evidenceDir, "session-1", false)
	third, thirdErr := service.Stop(evidenceDir, "session-1", false)

	if firstErr == nil || first.Block {
		t.Errorf("stop 1 = %+v, %v; want no block and an error", first, firstErr)
	}
	reviews := []string{}
	for _, r := range second.Classify {
		reviews = append(reviews, r.Review)
	}
	if secondErr != nil || !second.Block || !reflect.DeepEqual(reviews, []string{otherReview, "own-rdd/333333333333-2"}) {
		t.Errorf("stop 2 = %+v, %v; want a block that names both results and no error", second, secondErr)
	}
	if thirdErr != nil || third.Block {
		t.Errorf("stop 3 = %+v, %v; want no block and no error", third, thirdErr)
	}
}
