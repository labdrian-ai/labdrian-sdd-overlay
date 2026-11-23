package app_test

// What KeptResults and Stop do for the learning loop (S6, L55 T4h): the stored results of the branch's commits whose kept
// findings (the rules of Findings) are not classified yet are asked for once per session and result, when no review
// question blocks. The Classified port is a fake here. The branch of the fakes is the one of unreviewed_test.go; a result
// of the whole branch (baseSHA to otherSHA) is the review of both commits, so no review question is put.

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// fakeClassified is the Classified port over a map: the review identifiers whose evidence rows exist.
type fakeClassified struct {
	done     map[string]bool
	err      error
	stamp    string
	stampErr error
	calls    []string // "<root> <review>"
	stamps   int      // how many times the state of the page was asked
}

func newFakeClassified() *fakeClassified {
	return &fakeClassified{done: map[string]bool{}, stamp: "stamp-1"}
}

func (c *fakeClassified) Stamp(root string) (string, error) {
	c.stamps++
	if c.stampErr != nil {
		return "", c.stampErr
	}
	return c.stamp, nil
}

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

// memoKey is the key under which a session remembers the answer of the port for review at the page state stamp: the
// prefix and the first 16 hex digits of the SHA-256 of the identifier, a NUL and the stamp, written here without the code
// under test.
func memoKey(prefix, review, stamp string) string {
	return prefix + sha256Of([]byte(review + "\x00" + stamp))[:16]
}

func classifiedKey(review, stamp string) string { return memoKey("classified-", review, stamp) }
func lackingKey(review, stamp string) string    { return memoKey("lacking-", review, stamp) }

// addFailsFor is an Asked store whose Add of one key fails, and of no other.
type addFailsFor struct {
	*fakeAsked
	key string
	err error
}

func (a *addFailsFor) Add(session, key string) error {
	if key == a.key {
		return a.err
	}
	return a.fakeAsked.Add(session, key)
}

// reviewsOf names the results of a ruling.
func reviewsOf(results []app.KeptResult) []string {
	names := []string{}
	for _, r := range results {
		names = append(names, r.Review)
	}
	return names
}

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
	if !asked.put["session-1 "+classifyKey(otherReview)] || !asked.put["session-1 "+lackingKey(otherReview, port.stamp)] || len(asked.put) != 2 {
		t.Errorf("remembered = %v, want only %s and %s of session-1", asked.put, classifyKey(otherReview), lackingKey(otherReview, port.stamp))
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

	if err != nil || got.Block || len(asked.put) != 1 || !asked.put["session-1 "+classifiedKey(otherReview, port.stamp)] {
		t.Errorf("Stop() = %+v, %v with %v remembered; want no block, no error and only %s remembered", got, err, asked.put, classifiedKey(otherReview, port.stamp))
	}
}

// SH4: the question is put once per session; the next stop does not block and does not ask the port again, and a result
// still lacking is named, after a new question of the port, in another session.
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
	if len(port.calls) != 1 {
		t.Errorf("port calls = %q, want one: the session remembers that the page lacks the result", port.calls)
	}
	other, otherErr := service.Stop(evidenceDir, "session-2", false)
	if otherErr != nil || !other.Block || len(other.Classify) != 1 || len(port.calls) != 2 {
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
	if len(asked.put) != 4 || !asked.put["session-1 "+classifyKey(otherReview)] || !asked.put["session-1 "+classifyKey("own-rdd/333333333333-2")] {
		t.Errorf("remembered = %v, want a classify- key for each of the two results, and a lacking- key for each", asked.put)
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

	got, refused, err := app.New(g, store).KeptResults(evidenceDir)

	want := []app.KeptResult{{Review: headReview, Head: headSHA, Kept: 1}, {Review: otherReview, Head: otherSHA, Kept: 1}}
	if err != nil || len(refused) != 0 || !reflect.DeepEqual(got, want) {
		t.Errorf("KeptResults() = %+v, %v, %v; want %+v and no error", got, refused, err, want)
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

			got, refused, err := app.New(g, store).KeptResults(evidenceDir)

			if err != nil || len(refused) != 0 || len(got) != 0 {
				t.Errorf("KeptResults() = %+v, %v, %v; want nothing and no error", got, refused, err)
			}
		})
	}
}

// M1: a result that cannot be read, hold or bind is refused, and the results that hold are still listed beside it, as
// 'review check' refuses the same file; it is never skipped.
func TestKeptResultsListsWhatHoldsBesideWhatItRefuses(t *testing.T) {
	cause := errors.New("boom")
	for _, tc := range []struct {
		name    string
		setup   func(t *testing.T, g *fakeGit, store *fakeResults)
		want    []string // the results that hold
		refused int
		wraps   error
		names   string
	}{
		{"a file that is not a result", func(t *testing.T, g *fakeGit, store *fakeResults) {
			store.put("own-rdd", "333333333333-9.json", []byte("not a result"))
		}, []string{otherReview}, 1, review.ErrInvalidStored, "333333333333-9.json"},
		{"the verdict cannot be read", func(t *testing.T, g *fakeGit, store *fakeResults) {
			reviewed(t, store, baseSHA, otherSHA, 2, warning("F2"))
			store.fail["Read 333333333333-2.verifier.json"] = cause
		}, []string{otherReview}, 1, cause, "333333333333-2.verifier.json"},
		{"a result for a diff that is not the one that was reviewed", func(t *testing.T, g *fakeGit, store *fakeResults) {
			name := reviewed(t, store, baseSHA, otherSHA, 2, warning("F2"))
			sum := sha256Of([]byte(diffText))
			store.put("own-rdd", name, bytes.ReplaceAll(store.files["own-rdd"][name], []byte(sum), []byte(strings.Repeat("0", len(sum)))))
		}, []string{otherReview}, 1, review.ErrBindingDiffers, "333333333333-2.json"},
		{"git cannot give the diff", func(t *testing.T, g *fakeGit, store *fakeResults) {
			reviewed(t, store, baseSHA, otherSHA, 2, warning("F2"))
			g.fail["Diff"] = cause
		}, nil, 2, cause, "333333333333-2.json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store, _ := classifyWorld(t)
			tc.setup(t, g, store)

			got, refused, err := app.New(g, store).KeptResults(evidenceDir)

			if err != nil || len(refused) != tc.refused || !reflect.DeepEqual(reviewsOf(got), append([]string{}, tc.want...)) {
				t.Fatalf("KeptResults() = %+v, %v, %v; want %v, %d refusals and no error", got, refused, err, tc.want, tc.refused)
			}
			last := refused[len(refused)-1]
			if !errors.Is(last, tc.wraps) || !strings.Contains(last.Error(), tc.names) {
				t.Errorf("the last refusal = %v; want an error that wraps %v and names %q", last, tc.wraps, tc.names)
			}
		})
	}
}

// J1: what the pass as a whole cannot tell is an error and no result, and no refusal.
func TestKeptResultsRefusesWhatItCannotReadHoldOrBind(t *testing.T) {
	cause := errors.New("boom")
	for _, tc := range []struct {
		name  string
		setup func(g *fakeGit, store *fakeResults)
	}{
		{"the store cannot give the features", func(g *fakeGit, store *fakeResults) { store.fail["Features"] = cause }},
		{"git cannot tell where HEAD stands", func(g *fakeGit, store *fakeResults) { g.fail["Standing"] = cause }},
		{"the store cannot list the results", func(g *fakeGit, store *fakeResults) { store.fail["Names"] = cause }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store, _ := classifyWorld(t)
			tc.setup(g, store)

			got, refused, err := app.New(g, store).KeptResults(evidenceDir)

			if got != nil || refused != nil || !errors.Is(err, cause) {
				t.Errorf("KeptResults() = %+v, %v, %v; want nothing, no refusal and an error that wraps the cause", got, refused, err)
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

	got, refused, err := app.New(g, store).KeptResults(evidenceDir)

	want := []app.KeptResult{{Review: otherReview, Head: otherSHA, Kept: 1}}
	if err != nil || len(refused) != 0 || !reflect.DeepEqual(got, want) {
		t.Errorf("KeptResults() = %+v, %v, %v; want %+v and no error", got, refused, err, want)
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

// J5: a question that cannot be recorded is an error and no block, for a result the page lacks and for one it holds; for
// the one it holds the failed record is only the memory, named in the error.
func TestStopDoesNotBlockWhenAQuestionCannotBeRecorded(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(port *fakeClassified)
		names string
	}{
		{"an unclassified result", func(port *fakeClassified) {}, ""},
		{"a classified result", func(port *fakeClassified) { port.done[otherReview] = true }, "record that " + otherReview + " is classified"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store, _ := classifyWorld(t)
			asked, port := newFakeAsked(), newFakeClassified()
			tc.setup(port)
			cause := errors.New("boom")
			asked.fail["Add"] = cause

			got, err := app.New(g, store).WithAsked(asked).WithClassified(port).Stop(evidenceDir, "session-1", false)

			if !errors.Is(err, cause) || got.Block || !strings.Contains(err.Error(), tc.names) {
				t.Errorf("Stop() = %+v, %v; want no block and an error that wraps the cause and names %q", got, err, tc.names)
			}
		})
	}
}

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
	if !asked.put["session-1 "+classifiedKey(otherReview, port.stamp)] || asked.put["session-1 "+classifyKey(otherReview)] {
		t.Errorf("remembered = %v, want %s of session-1 and no classify- key", asked.put, classifiedKey(otherReview, port.stamp))
	}
}

// secondAddFails is an Asked store whose second Add of a classify- key fails, once.
type secondAddFails struct {
	*fakeAsked
	adds int
}

func (a *secondAddFails) Add(session, key string) error {
	if strings.HasPrefix(key, "classify-") {
		a.adds++
		if a.adds == 2 {
			return errors.New("disk full")
		}
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

const secondReview = "own-rdd/333333333333-2"

// M2: a result the pass refuses does not hide the ones that hold: the stop asks for them, and the refusal is an error
// beside the ruling at every stop of the session, whether the stop blocks or not.
func TestStopAsksForTheResultsThatHoldBesideARefusedOne(t *testing.T) {
	g, store, _ := classifyWorld(t)
	store.put("own-rdd", "333333333333-9.json", []byte("not a result"))
	port := newFakeClassified()
	service := app.New(g, store).WithAsked(newFakeAsked()).WithClassified(port)

	first, firstErr := service.Stop(evidenceDir, "session-1", false)
	second, secondErr := service.Stop(evidenceDir, "session-1", false)

	if !first.Block || !reflect.DeepEqual(reviewsOf(first.Classify), []string{otherReview}) {
		t.Errorf("stop 1 = %+v; want a block that names %s", first, otherReview)
	}
	if !errors.Is(firstErr, review.ErrInvalidStored) || !strings.Contains(firstErr.Error(), "333333333333-9.json") {
		t.Errorf("stop 1 error = %v; want one that wraps %v and names the refused file", firstErr, review.ErrInvalidStored)
	}
	if second.Block || secondErr == nil || secondErr.Error() != firstErr.Error() {
		t.Errorf("stop 2 = %+v, %v; want no block and the same error", second, secondErr)
	}
	if len(port.calls) != 1 {
		t.Errorf("port calls = %q, want one", port.calls)
	}
}

// M3: every problem of the pass is on one line, in order, so the hook's stderr stays one line.
func TestStopNamesEveryProblemOfThePassOnOneLine(t *testing.T) {
	g, store, _ := classifyWorld(t)
	store.put("own-rdd", "333333333333-8.json", []byte("not a result"))
	store.put("own-rdd", "333333333333-9.json", []byte("not a result"))
	port := newFakeClassified()
	port.done[otherReview] = true
	asked := &addFailsFor{fakeAsked: newFakeAsked(), key: classifiedKey(otherReview, port.stamp), err: errors.New("disk full")}

	_, err := app.New(g, store).WithAsked(asked).WithClassified(port).Stop(evidenceDir, "session-1", false)

	if err == nil {
		t.Fatal("Stop() = no error; want one that names the three problems")
	}
	text := err.Error()
	first, second, third := strings.Index(text, "-8.json"), strings.Index(text, "-9.json"), strings.Index(text, "disk full")
	if strings.Contains(text, "\n") || !strings.HasPrefix(text, "classify the results with kept findings: ") || !strings.Contains(text, "; ") || first < 0 || second < first || third < second {
		t.Errorf("Stop() error = %q; want one line that names -8.json, -9.json and the failed record in order, joined by '; '", text)
	}
}

// M4: the port is asked once for each result and page state in a session, whatever it answers; a new page state asks
// again, and so does another session.
func TestStopAsksThePortOnceForEachResultAndPageState(t *testing.T) {
	g, store, _ := classifyWorld(t)
	port := newFakeClassified()
	service := app.New(g, store).WithAsked(newFakeAsked()).WithClassified(port)

	for i, step := range []struct {
		session, stamp string
		calls          int
		block          bool
	}{
		{"session-1", "stamp-1", 1, true},
		{"session-1", "stamp-1", 1, false},
		{"session-1", "stamp-1", 1, false},
		{"session-1", "stamp-2", 2, false}, // its classify- key is held
		{"session-2", "stamp-2", 3, true},
	} {
		port.stamp = step.stamp
		got, err := service.Stop(evidenceDir, step.session, false)
		if err != nil || got.Block != step.block || len(port.calls) != step.calls {
			t.Errorf("stop %d of %s at %s = %+v, %v with %d port calls; want block=%v, no error and %d calls", i+1, step.session, step.stamp, got, err, len(port.calls), step.block, step.calls)
		}
	}
}

// M5: a row removed from the page later in the session is seen at the next stop, because the stamp changed.
func TestStopSeesARowRemovedLaterInTheSession(t *testing.T) {
	t.Run("the page state changed", func(t *testing.T) {
		g, store, _ := classifyWorld(t)
		port := newFakeClassified()
		port.done[otherReview] = true
		service := app.New(g, store).WithAsked(newFakeAsked()).WithClassified(port)
		if first, err := service.Stop(evidenceDir, "session-1", false); err != nil || first.Block {
			t.Fatalf("setup: stop 1 = %+v, %v; want no block", first, err)
		}
		port.done[otherReview], port.stamp = false, "stamp-2"

		got, err := service.Stop(evidenceDir, "session-1", false)

		if err != nil || !got.Block || !reflect.DeepEqual(reviewsOf(got.Classify), []string{otherReview}) || len(port.calls) != 2 {
			t.Errorf("stop 2 = %+v, %v with %d port calls; want a block that names %s and two calls", got, err, len(port.calls), otherReview)
		}
	})
	t.Run("the page state is the same", func(t *testing.T) {
		g, store, _ := classifyWorld(t)
		port := newFakeClassified()
		port.done[otherReview] = true
		service := app.New(g, store).WithAsked(newFakeAsked()).WithClassified(port)
		if first, err := service.Stop(evidenceDir, "session-1", false); err != nil || first.Block {
			t.Fatalf("setup: stop 1 = %+v, %v; want no block", first, err)
		}
		port.done[otherReview] = false

		got, err := service.Stop(evidenceDir, "session-1", false)

		if err != nil || got.Block || len(port.calls) != 1 {
			t.Errorf("stop 2 = %+v, %v with %d port calls; want no block and one call: equal stamps mean equal answers", got, err, len(port.calls))
		}
	})
}

// M6: a memory that cannot be recorded costs no answer: the pass goes on to the next result, and the failure is an error
// beside the ruling.
func TestStopGoesOnWhenItsMemoryCannotBeRecorded(t *testing.T) {
	cause := errors.New("disk full")
	t.Run("classified results", func(t *testing.T) {
		g, store, _ := classifyWorld(t)
		reviewed(t, store, baseSHA, otherSHA, 2, warning("F2"))
		port := newFakeClassified()
		port.done[otherReview], port.done[secondReview] = true, true
		asked := &addFailsFor{fakeAsked: newFakeAsked(), key: classifiedKey(otherReview, port.stamp), err: cause}

		got, err := app.New(g, store).WithAsked(asked).WithClassified(port).Stop(evidenceDir, "session-1", false)

		wantCalls := []string{fakeRepo.Root + " " + otherReview, fakeRepo.Root + " " + secondReview}
		if got.Block || !errors.Is(err, cause) || !strings.Contains(err.Error(), otherReview) || !reflect.DeepEqual(port.calls, wantCalls) {
			t.Errorf("Stop() = %+v, %v with port calls %q; want no block, an error that wraps the cause and names %s, and calls %q", got, err, port.calls, otherReview, wantCalls)
		}
	})
	t.Run("lacking results", func(t *testing.T) {
		g, store, _ := classifyWorld(t)
		reviewed(t, store, baseSHA, otherSHA, 2, warning("F2"))
		port := newFakeClassified()
		asked := &addFailsFor{fakeAsked: newFakeAsked(), key: lackingKey(otherReview, port.stamp), err: cause}

		got, err := app.New(g, store).WithAsked(asked).WithClassified(port).Stop(evidenceDir, "session-1", false)

		if !got.Block || !reflect.DeepEqual(reviewsOf(got.Classify), []string{otherReview, secondReview}) || !errors.Is(err, cause) {
			t.Errorf("Stop() = %+v, %v; want a block that names both results and an error that wraps the cause", got, err)
		}
	})
}

// M7: a page whose state cannot be read is an error and no block, and the port is not asked.
func TestStopDoesNotBlockWhenThePageStateCannotBeRead(t *testing.T) {
	g, store, _ := classifyWorld(t)
	port := newFakeClassified()
	cause := errors.New("boom")
	port.stampErr = cause

	got, err := app.New(g, store).WithAsked(newFakeAsked()).WithClassified(port).Stop(evidenceDir, "session-1", false)

	if !errors.Is(err, cause) || got.Block || len(port.calls) != 0 {
		t.Errorf("Stop() = %+v, %v with %q asked of the port; want no block, an error that wraps the cause and no call", got, err, port.calls)
	}
}

// M8: with no kept result the state of the page is not read.
func TestStopReadsNoPageStateWithNoKeptResult(t *testing.T) {
	g, store := sweptWorld(t)
	reviewed(t, store, baseSHA, otherSHA, 1)
	port := newFakeClassified()

	got, err := app.New(g, store).WithAsked(newFakeAsked()).WithClassified(port).Stop(evidenceDir, "session-1", false)

	if err != nil || got.Block || port.stamps != 0 {
		t.Errorf("Stop() = %+v, %v with %d reads of the page state; want no block, no error and none", got, err, port.stamps)
	}
}
