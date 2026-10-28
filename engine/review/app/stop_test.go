package app_test

// What Stop does (own-rdd T6a, S6): it blocks the stop of an agent once for each session and each question, when a
// commit of the branch has no review, or when the repository does not tell its default branch; it does not block while
// a stop hook is already making the agent continue, and it does not block what it cannot remember having asked. The
// branch of the fakes is the one of unreviewed_test.go.

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// fakeAsked is the Asked port over a map: the questions put in each session.
type fakeAsked struct {
	put   map[string]bool // session + " " + key
	fail  map[string]error
	calls []string
}

func newFakeAsked() *fakeAsked { return &fakeAsked{put: map[string]bool{}, fail: map[string]error{}} }

func (a *fakeAsked) Has(session, key string) (bool, error) {
	a.calls = append(a.calls, "Has "+session+" "+key)
	if err := a.fail["Has"]; err != nil {
		return false, err
	}
	return a.put[session+" "+key], nil
}

func (a *fakeAsked) Add(session, key string) error {
	a.calls = append(a.calls, "Add "+session+" "+key)
	if err := a.fail["Add"]; err != nil {
		return err
	}
	a.put[session+" "+key] = true
	return nil
}

const (
	firstQuestionKey = "111111111111-222222222222-review" // baseSHA to headSHA, the need of its review
	nextQuestionKey  = "222222222222-333333333333-review" // headSHA to otherSHA, the need of its review
)

// kinds lists what the needs of a question are.
func kinds(q app.Question) []review.Missing { return lacks(q) }

// A need that appears after an earlier block of the same commit blocks once more in the same session, and asks for that
// need alone in the store: the sweeper of a sweep that fired.
func TestStopAsksANeedThatAppearsLaterForTheSameCommitInTheSameSession(t *testing.T) {
	g, store := branchWorld()
	g.declares(headSHA, "none")
	asked := newFakeAsked()
	service := app.New(g, store).WithAsked(asked)

	first, err := service.Stop(evidenceDir, "session-1", false)
	if err != nil || !first.Block || !reflect.DeepEqual(kinds(first.Question), []review.Missing{review.MissingSweep, review.MissingReview}) {
		t.Fatalf("the first Stop() = %+v, %v; want a block for a sweep and a review", first, err)
	}
	wantFirst := []string{
		"Has session-1 111111111111-222222222222-sweep", "Has session-1 111111111111-222222222222-review",
		"Add session-1 111111111111-222222222222-sweep", "Add session-1 111111111111-222222222222-review",
	}
	if !reflect.DeepEqual(asked.calls, wantFirst) {
		t.Fatalf("calls of the first Stop() = %q, want %q", asked.calls, wantFirst)
	}

	fired := swept(t, store, baseSHA, headSHA, 1, "?? stray.txt")
	asked.calls = nil
	second, err := service.Stop(evidenceDir, "session-1", false)
	if err != nil || !second.Block || !reflect.DeepEqual(kinds(second.Question), []review.Missing{review.MissingSweeper, review.MissingReview}) || second.Question.Sweep != review.ResultPath("own-rdd", fired) {
		t.Fatalf("the second Stop() = %+v, %v; want a block for the sweeper and the review, naming %s", second, err, fired)
	}
	wantSecond := []string{
		"Has session-1 111111111111-222222222222-sweeper", "Has session-1 111111111111-222222222222-review",
		"Add session-1 111111111111-222222222222-sweeper",
	}
	if !reflect.DeepEqual(asked.calls, wantSecond) {
		t.Errorf("calls of the second Stop() = %q, want %q: only the sweeper is added", asked.calls, wantSecond)
	}

	answered(t, store, baseSHA, headSHA, 2, fired, nil)
	asked.calls = nil
	third, err := service.Stop(evidenceDir, "session-1", false)
	if err != nil || third.Block {
		t.Errorf("the third Stop() = %+v, %v; want no block", third, err)
	}
	for _, call := range asked.calls {
		if strings.HasPrefix(call, "Add ") {
			t.Errorf("the third Stop() added %q, want nothing added", call)
		}
	}
}

// A need already asked is not asked again while the commit misses it, even when the other needs are gone.
func TestStopDoesNotAskAgainANeedAlreadyAsked(t *testing.T) {
	g, store := branchWorld()
	g.declares(headSHA, "none")
	asked := newFakeAsked()
	service := app.New(g, store).WithAsked(asked)
	if r, err := service.Stop(evidenceDir, "session-1", false); err != nil || !r.Block {
		t.Fatalf("setup: Stop() = %+v, %v; want a block", r, err)
	}

	swept(t, store, baseSHA, headSHA, 1)
	asked.calls = nil
	got, err := service.Stop(evidenceDir, "session-1", false)

	if err != nil || got.Block {
		t.Errorf("Stop() = %+v, %v; want no block: the review was asked already", got, err)
	}
	for _, call := range asked.calls {
		if strings.HasPrefix(call, "Add ") {
			t.Errorf("Stop() added %q, want nothing added", call)
		}
	}
}

// The first stop of a session with a commit no review covers is blocked with the question, and is remembered: the same
// question in the same session is not asked again.
func TestStopBlocksOncePerSessionAndQuestion(t *testing.T) {
	g, store := sweptWorld(t)
	asked := newFakeAsked()
	service := app.New(g, store).WithAsked(asked)

	first, err := service.Stop(evidenceDir, "session-1", false)
	again, againErr := service.Stop(evidenceDir, "session-1", false)

	want := app.Ruling{Block: true, Question: reviewMissing(t, baseSHA, headSHA)}
	if err != nil || !reflect.DeepEqual(first, want) {
		t.Fatalf("the first Stop() = %+v, %v; want %+v and no error", first, err, want)
	}
	if againErr != nil || again.Block {
		t.Errorf("the second Stop() of the session = %+v, %v; want no block and no error", again, againErr)
	}
	if !asked.put["session-1 "+firstQuestionKey] || len(asked.put) != 1 {
		t.Errorf("remembered = %v, want only the question %s of session-1", asked.put, firstQuestionKey)
	}
}

// Another session is asked the same question; and in the same session a new commit to review is a new question.
func TestStopAsksAgainForAnotherSessionAndForTheNextCommit(t *testing.T) {
	g, store := sweptWorld(t)
	asked := newFakeAsked()
	service := app.New(g, store).WithAsked(asked)
	if r, err := service.Stop(evidenceDir, "session-1", false); err != nil || !r.Block {
		t.Fatalf("setup: Stop() = %+v, %v; want a block", r, err)
	}

	other, otherErr := service.Stop(evidenceDir, "session-2", false)
	reviewed(t, store, baseSHA, headSHA, 1)
	next, nextErr := service.Stop(evidenceDir, "session-1", false)

	if otherErr != nil || !other.Block || other.Question.Head != headSHA {
		t.Errorf("Stop() of another session = %+v, %v; want a block for the first commit", other, otherErr)
	}
	wantNext := app.Ruling{Block: true, Question: reviewMissing(t, headSHA, otherSHA)}
	if nextErr != nil || !reflect.DeepEqual(next, wantNext) {
		t.Errorf("Stop() after the first commit was reviewed = %+v, %v; want %+v", next, nextErr, wantNext)
	}
	if !asked.put["session-2 "+firstQuestionKey] || !asked.put["session-1 "+nextQuestionKey] {
		t.Errorf("remembered = %v, want session-2 %s and session-1 %s", asked.put, firstQuestionKey, nextQuestionKey)
	}
}

// A stop hook that made the agent continue is not asked to block again, whatever is left to review: the agent would
// never stop. Nothing is asked of git or the store, and nothing is remembered.
func TestStopDoesNotBlockWhileAStopHookIsContinuing(t *testing.T) {
	g, store := sweptWorld(t)
	asked := newFakeAsked()

	got, err := app.New(g, store).WithAsked(asked).Stop(evidenceDir, "session-1", true)

	if err != nil || !reflect.DeepEqual(got, app.Ruling{}) {
		t.Errorf("Stop() = %+v, %v; want no block and no error", got, err)
	}
	if len(g.calls) != 0 || len(store.calls) != 0 || len(asked.calls) != 0 {
		t.Errorf("git calls = %q, store calls = %q, asked calls = %q; want none", g.calls, store.calls, asked.calls)
	}
}

// A branch whose commits are all reviewed has nothing to ask, and nothing is remembered.
func TestStopDoesNotBlockWhenEveryCommitIsReviewed(t *testing.T) {
	g, store := sweptWorld(t)
	reviewed(t, store, baseSHA, otherSHA, 1)
	asked := newFakeAsked()

	got, err := app.New(g, store).WithAsked(asked).Stop(evidenceDir, "session-1", false)

	if err != nil || !reflect.DeepEqual(got, app.Ruling{}) || len(asked.put) != 0 {
		t.Errorf("Stop() = %+v, %v with %v remembered; want no block, no error and nothing remembered", got, err, asked.put)
	}
}

// A repository that does not tell its default branch is told once per session, as a question of its own.
func TestStopSaysOncePerSessionThatTheDefaultBranchCannotBeFound(t *testing.T) {
	g, store := sweptWorld(t)
	g.standing = app.Standing{Branch: "feat/x"}
	asked := newFakeAsked()
	service := app.New(g, store).WithAsked(asked)

	first, err := service.Stop(evidenceDir, "session-1", false)
	again, againErr := service.Stop(evidenceDir, "session-1", false)

	want := app.Ruling{Block: true, NoDefault: true, Question: app.Question{Branch: "feat/x"}}
	if err != nil || !reflect.DeepEqual(first, want) {
		t.Errorf("the first Stop() = %+v, %v; want %+v and no error", first, err, want)
	}
	if againErr != nil || again.Block {
		t.Errorf("the second Stop() = %+v, %v; want no block and no error", again, againErr)
	}
}

// A stop is never blocked on what cannot be told or remembered: a hook that blocked without being able to say it did
// so once would trap the agent. The error is returned and the stop is not blocked.
func TestStopDoesNotBlockWhatItCannotTellOrRemember(t *testing.T) {
	boom := errors.New("boom")
	for _, tc := range []struct {
		name  string
		setup func(g *fakeGit, asked *fakeAsked)
		want  string
	}{
		{"git cannot say where HEAD stands", func(g *fakeGit, asked *fakeAsked) { g.fail["Standing"] = boom }, "where HEAD stands"},
		{"the store of the questions cannot be read", func(g *fakeGit, asked *fakeAsked) { asked.fail["Has"] = boom }, "boom"},
		{"the store of the questions cannot be written", func(g *fakeGit, asked *fakeAsked) { asked.fail["Add"] = boom }, "boom"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := sweptWorld(t)
			asked := newFakeAsked()
			tc.setup(g, asked)

			got, err := app.New(g, store).WithAsked(asked).Stop(evidenceDir, "session-1", false)

			if err == nil || !strings.Contains(err.Error(), tc.want) || got.Block {
				t.Errorf("Stop() = %+v, %v; want no block and an error that says %q", got, err, tc.want)
			}
		})
	}
}

// Without a store of its questions it could not ask each once, so it asks none.
func TestStopRefusesToRunWithoutAStoreOfItsQuestions(t *testing.T) {
	g, store := sweptWorld(t)

	got, err := app.New(g, store).Stop(evidenceDir, "session-1", false)

	if err == nil || got.Block || len(g.calls) != 0 {
		t.Errorf("Stop() = %+v, %v with %q asked of git; want no block, an error and git not asked", got, err, g.calls)
	}
}
