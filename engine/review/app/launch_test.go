package app_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// fakeReviewer is the Reviewer port with a canned answer. It records every call.
type fakeReviewer struct {
	reply []byte
	err   error
	calls []reviewerCall
}

type reviewerCall struct {
	worktree, model string
	brief           []byte
}

func (r *fakeReviewer) Review(worktree, model string, brief []byte) ([]byte, error) {
	r.calls = append(r.calls, reviewerCall{worktree: worktree, model: model, brief: brief})
	return r.reply, r.err
}

// launchWorld is a Service over the fakes with a reviewer, and the brief that names the binding of the candidate.
func launchWorld(t *testing.T) (app.Service, *fakeGit, *fakeResults, *fakeReviewer, review.Candidate, []byte) {
	t.Helper()
	git, store := newFakeGit(), newFakeResults()
	c := candidateOf(t, git)
	reviewer := &fakeReviewer{reply: replyFor(t, c)}
	brief := []byte(strings.Join([]string{c.Base, c.Head, c.DiffSHA256, review.WorktreePath(fakeRepo.Main, c)}, "\n"))
	return app.New(git, store).WithReviewer(reviewer), git, store, reviewer, c, brief
}

func TestLaunchRecordsTheReplyOfTheReviewerOverTheBoundCandidate(t *testing.T) {
	service, git, store, reviewer, c, brief := launchWorld(t)
	got, err := service.Launch("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "default", "gpt-5.5", brief)
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}
	want := reviewerCall{worktree: review.WorktreePath(fakeRepo.Main, c), model: "gpt-5.5", brief: brief}
	if len(reviewer.calls) != 1 || reviewer.calls[0].worktree != want.worktree || reviewer.calls[0].model != want.model || !bytes.Equal(reviewer.calls[0].brief, want.brief) {
		t.Fatalf("the reviewer was asked %+v, want once with %+v", reviewer.calls, want)
	}
	if git.called("AddWorktree ") != 1 {
		t.Errorf("AddWorktree calls = %d, want 1: Launch binds as Bind does", git.called("AddWorktree "))
	}
	// What Record of the same reply stores, on an empty store of its own.
	otherStore := newFakeResults()
	recorded, err := app.New(newFakeGit(), otherStore).Record("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "default", reviewer.reply)
	if err != nil {
		t.Fatalf("setup: Record() error = %v", err)
	}
	if got.Path != recorded.Path || got.Number != recorded.Number || got.SHA256 != recorded.SHA256 {
		t.Errorf("Launch() = {%s %d %s}, want what Record stores {%s %d %s}", got.Path, got.Number, got.SHA256, recorded.Path, recorded.Number, recorded.SHA256)
	}
	name := review.ResultName(c, got.Number)
	if !bytes.Equal(store.files["own-rdd"][name], otherStore.files["own-rdd"][name]) || store.created() != 1 {
		t.Errorf("the store holds %q after %d creates, want the bytes Record stores", store.files["own-rdd"][name], store.created())
	}
}

func TestLaunchRefusesBeforeTheReviewerStarts(t *testing.T) {
	_, _, _, _, c, brief := launchWorld(t)
	over := func(g *fakeGit) { g.diff = bytes.Repeat([]byte("x"), review.DiffLimitBytes+1) }
	cases := []struct {
		name    string
		feature string
		rows    string
		model   string
		brief   []byte
		prepare func(*fakeGit)
		want    error
	}{
		{name: "unsafe feature", feature: "../x", rows: "default", model: "gpt-5.5", brief: brief, want: review.ErrInvalidFeature},
		{name: "bogus rows", feature: "own-rdd", rows: "bogus", model: "gpt-5.5", brief: brief, want: review.ErrInvalidRows},
		{name: "model as an option", feature: "own-rdd", rows: "default", model: "-x", brief: brief, want: review.ErrInvalidModel},
		{name: "over the limit", feature: "own-rdd", rows: "default", model: "gpt-5.5", brief: brief, prepare: over, want: review.ErrOverLimit},
		{name: "brief without the diff", feature: "own-rdd", rows: "default", model: "gpt-5.5", brief: []byte(strings.Replace(string(brief), c.DiffSHA256, "", 1)), want: review.ErrInvalidBrief},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, git, store, reviewer, _, _ := launchWorld(t)
			if tc.prepare != nil {
				tc.prepare(git)
			}
			_, err := service.Launch("/work/wt/feature", tc.feature, "HEAD~1", "HEAD", tc.rows, tc.model, tc.brief)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Launch() error = %v, want one wrapping %v", err, tc.want)
			}
			if len(reviewer.calls) != 0 {
				t.Errorf("the reviewer was asked %d times, want never", len(reviewer.calls))
			}
			if store.created() != 0 {
				t.Errorf("Create calls = %d, want nothing written", store.created())
			}
		})
	}
}

func TestLaunchStoresNothingWhenTheReviewerDoesNotAnswer(t *testing.T) {
	service, _, store, reviewer, _, brief := launchWorld(t)
	reviewer.reply, reviewer.err = nil, fmt.Errorf("codex exited 1: %w", app.ErrReviewerDidNotAnswer)
	_, err := service.Launch("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "default", "gpt-5.5", brief)
	if !errors.Is(err, app.ErrReviewerDidNotAnswer) {
		t.Fatalf("Launch() error = %v, want one wrapping ErrReviewerDidNotAnswer", err)
	}
	if store.created() != 0 {
		t.Errorf("Create calls = %d, want nothing written", store.created())
	}
}

func TestLaunchRefusesAReplyAsRecordDoes(t *testing.T) {
	service, _, store, reviewer, c, brief := launchWorld(t)
	reviewer.reply = replyWith(t, c.Base, c.Head, strings.Repeat("0", 64), "completed", c.Paths)
	_, err := service.Launch("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "default", "gpt-5.5", brief)
	var refused *app.ReplyRefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("Launch() error = %v, want a *ReplyRefusedError", err)
	}
	if store.created() != 0 {
		t.Errorf("Create calls = %d, want nothing written", store.created())
	}
}

func TestLaunchWithoutAReviewerRefuses(t *testing.T) {
	git, store := newFakeGit(), newFakeResults()
	_, err := app.New(git, store).Launch("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "default", "gpt-5.5", []byte("brief"))
	if !errors.Is(err, app.ErrNoReviewer) {
		t.Fatalf("Launch() error = %v, want one wrapping ErrNoReviewer", err)
	}
	if len(git.calls) != 0 || store.created() != 0 {
		t.Errorf("git calls %v and %d creates, want none", git.calls, store.created())
	}
}
