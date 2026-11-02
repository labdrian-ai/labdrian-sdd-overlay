package app_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
)

// fakeReviewer is the Reviewer port with a canned answer. It records every call. With replies, call n answers
// replies[n] (the last one again past the end) instead of reply.
type fakeReviewer struct {
	reply   []byte
	replies [][]byte
	err     error
	calls   []reviewerCall
}

type reviewerCall struct {
	worktree, model string
	brief           []byte
}

func (r *fakeReviewer) Review(worktree, model string, brief []byte) ([]byte, error) {
	r.calls = append(r.calls, reviewerCall{worktree: worktree, model: model, brief: brief})
	if len(r.replies) > 0 {
		return r.replies[min(len(r.calls), len(r.replies))-1], r.err
	}
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
	if n := git.called("FunctionContextDiff "); n != 0 {
		t.Errorf("FunctionContextDiff calls = %d, want none without WithFrozenContext", n)
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

// replyWithoutSeverity is a reply of c with one finding that has no severity member: the refusal of the first T7 run
// (phase9-c25-review-hunt L12), a reply refused for its shape.
func replyWithoutSeverity(t *testing.T, c review.Candidate) []byte {
	t.Helper()
	reply := map[string]any{
		"candidate":  map[string]string{"base": c.Base, "head": c.Head, "diff_sha256": c.DiffSHA256},
		"inspection": map[string]any{"status": "completed", "paths": c.Paths, "reason": ""},
		"findings": []any{map[string]any{
			"id": "F1", "location": c.Paths[0] + ":1", "claim": "an empty input panics",
			"evidence_class": "inferential", "causal_disposition": "introduced", "proof_refs": []string{},
		}},
		"evidence": []string{"read every changed path"},
	}
	data, err := json.Marshal(reply)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return data
}

// A reply refused for its shape is asked for again once, with the brief followed by the refusal, and the second
// reply is what is recorded (phase9-c25-review-hunt S7).
func TestLaunchAsksAgainOnceWhenTheReplyIsRefusedForItsShape(t *testing.T) {
	service, _, store, reviewer, c, brief := launchWorld(t)
	reviewer.replies = [][]byte{replyWithoutSeverity(t, c), replyFor(t, c)}
	got, err := service.Launch("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "default", "gpt-5.5", brief)
	if err != nil {
		t.Fatalf("Launch() error = %v, want the second reply recorded", err)
	}
	if len(reviewer.calls) != 2 {
		t.Fatalf("the reviewer was asked %d times, want twice", len(reviewer.calls))
	}
	again := reviewer.calls[1]
	if again.worktree != reviewer.calls[0].worktree || again.model != "gpt-5.5" || !bytes.HasPrefix(again.brief, brief) {
		t.Fatalf("the second call was %+v, want the same worktree and model and a brief that starts with the first", again)
	}
	tail := string(again.brief[len(brief):])
	for _, want := range []string{"## Your previous reply was refused", `findings[0] has no "severity" member`, "Reply again with the whole JSON object"} {
		if !strings.Contains(tail, want) {
			t.Errorf("the re-ask after the brief is %q, want it to hold %q", tail, want)
		}
	}
	name := review.ResultName(c, got.Number)
	if store.created() != 1 || !bytes.Contains(store.files["own-rdd"][name], []byte(`"findings": []`)) {
		t.Errorf("the store holds %q after %d creates, want the second reply stored once", store.files["own-rdd"][name], store.created())
	}
}

// A second refusal is not asked for again: Launch returns it and stores nothing.
func TestLaunchReturnsTheSecondRefusalOfAShape(t *testing.T) {
	service, _, store, reviewer, c, brief := launchWorld(t)
	reviewer.reply = replyWithoutSeverity(t, c)
	_, err := service.Launch("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "default", "gpt-5.5", brief)
	var refused *app.ReplyRefusedError
	if !errors.As(err, &refused) || !strings.Contains(err.Error(), `has no "severity" member`) {
		t.Fatalf("Launch() error = %v, want the *ReplyRefusedError of the second reply", err)
	}
	if len(reviewer.calls) != 2 || store.created() != 0 {
		t.Errorf("the reviewer was asked %d times and %d files created, want twice and nothing written", len(reviewer.calls), store.created())
	}
}

// A refusal that is not of the shape (here an echo of another candidate) is returned at once, as Record refuses it.
func TestLaunchDoesNotAskAgainWhenTheReplyIsRefusedForItsBinding(t *testing.T) {
	service, _, _, reviewer, c, brief := launchWorld(t)
	reviewer.reply = replyWith(t, c.Base, c.Head, strings.Repeat("0", 64), "completed", c.Paths)
	_, err := service.Launch("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "default", "gpt-5.5", brief)
	if !errors.Is(err, reviewresult.ErrCandidateMismatch) || len(reviewer.calls) != 1 {
		t.Fatalf("Launch() error = %v after %d calls, want ErrCandidateMismatch after one", err, len(reviewer.calls))
	}
}

// A re-ask that would take the brief over review.MaxReplyBytes is not sent: the first refusal is returned.
func TestLaunchDoesNotAskAgainOverTheBound(t *testing.T) {
	service, _, store, reviewer, c, brief := launchWorld(t)
	brief = append(brief, bytes.Repeat([]byte("x"), review.MaxReplyBytes-len(brief)-10)...)
	reviewer.replies = [][]byte{replyWithoutSeverity(t, c), replyFor(t, c)}
	_, err := service.Launch("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "default", "gpt-5.5", brief)
	var refused *app.ReplyRefusedError
	if !errors.As(err, &refused) || !errors.Is(err, reviewresult.ErrUnparseable) {
		t.Fatalf("Launch() error = %v, want the first *ReplyRefusedError", err)
	}
	if len(reviewer.calls) != 1 || store.created() != 0 {
		t.Errorf("the reviewer was asked %d times and %d files created, want once and nothing written", len(reviewer.calls), store.created())
	}
}

// frozenWorld is launchWorld with the frozen context on, over a function-context diff of contextDiff.
func frozenWorld(t *testing.T, contextDiff string) (app.Service, *fakeGit, *fakeResults, *fakeReviewer, review.Candidate, []byte) {
	t.Helper()
	service, git, store, reviewer, c, brief := launchWorld(t)
	git.contextDiff = []byte(contextDiff)
	return service.WithFrozenContext(), git, store, reviewer, c, brief
}

func TestLaunchWithFrozenContextAppendsTheFunctionContextDiffAfterTheBrief(t *testing.T) {
	contextDiff := "diff --git a/a.go b/a.go\n@@ -1,3 +1,3 @@\n func f() {\n-\treturn 1\n+\treturn 2\n }\n"
	service, git, store, reviewer, c, brief := frozenWorld(t, contextDiff)
	if _, err := service.Launch("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "default", "gpt-5.5", brief); err != nil {
		t.Fatalf("Launch() error = %v", err)
	}
	want := string(brief) + "\n\n## Frozen context (base " + c.Base + ", head " + c.Head + ")\n\n```diff\n" + contextDiff + "```\n"
	if len(reviewer.calls) != 1 || string(reviewer.calls[0].brief) != want {
		t.Fatalf("the reviewer was asked %d times, the first with\n%q\nwant once with\n%q", len(reviewer.calls), firstBrief(reviewer), want)
	}
	if n := git.count("FunctionContextDiff " + c.Base + " " + c.Head); n != 1 {
		t.Errorf("FunctionContextDiff of the bound base and head asked %d times, want once: %v", n, git.calls)
	}
	if store.created() != 1 {
		t.Errorf("Create calls = %d, want the reply recorded once", store.created())
	}
}

// A diff that holds a fence of its own stays inside the one block: the fence is longer than any run of backticks in it.
func TestLaunchWithFrozenContextFencesADiffThatHoldsAFence(t *testing.T) {
	contextDiff := "diff --git a/README.md b/README.md\n@@ -1,3 +1,3 @@\n ````sh\n-old\n+new\n ````"
	service, _, _, reviewer, c, brief := frozenWorld(t, contextDiff)
	if _, err := service.Launch("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "default", "gpt-5.5", brief); err != nil {
		t.Fatalf("Launch() error = %v", err)
	}
	want := string(brief) + "\n\n## Frozen context (base " + c.Base + ", head " + c.Head + ")\n\n`````diff\n" + contextDiff + "\n`````\n"
	if got := firstBrief(reviewer); got != want {
		t.Fatalf("the reviewer was asked with\n%q\nwant\n%q", got, want)
	}
}

func TestLaunchWithFrozenContextRefusesABriefAndContextOverTheBound(t *testing.T) {
	service, _, store, reviewer, _, brief := frozenWorld(t, strings.Repeat("x", review.MaxReplyBytes))
	_, err := service.Launch("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "default", "gpt-5.5", brief)
	if !errors.Is(err, review.ErrInvalidBrief) {
		t.Fatalf("Launch() error = %v, want one wrapping ErrInvalidBrief", err)
	}
	if len(reviewer.calls) != 0 || store.created() != 0 {
		t.Errorf("the reviewer was asked %d times and %d results created, want neither", len(reviewer.calls), store.created())
	}
}

// The brief is checked as the parent wrote it: the frozen context cannot supply what the brief lacks, and is not asked for.
func TestLaunchWithFrozenContextChecksTheBriefAlone(t *testing.T) {
	_, _, _, _, c, brief := launchWorld(t)
	service, git, _, reviewer, _, _ := frozenWorld(t, c.DiffSHA256)
	_, err := service.Launch("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "default", "gpt-5.5", []byte(strings.Replace(string(brief), c.DiffSHA256, "", 1)))
	if !errors.Is(err, review.ErrInvalidBrief) {
		t.Fatalf("Launch() error = %v, want one wrapping ErrInvalidBrief", err)
	}
	if len(reviewer.calls) != 0 || git.called("FunctionContextDiff ") != 0 {
		t.Errorf("the reviewer was asked %d times and the context %d times, want neither", len(reviewer.calls), git.called("FunctionContextDiff "))
	}
}

func TestLaunchWithFrozenContextReturnsTheErrorOfGit(t *testing.T) {
	service, git, store, reviewer, _, brief := frozenWorld(t, "")
	broken := errors.New("git diff: exit status 128")
	git.fail["FunctionContextDiff"] = broken
	_, err := service.Launch("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "default", "gpt-5.5", brief)
	if !errors.Is(err, broken) {
		t.Fatalf("Launch() error = %v, want one wrapping %v", err, broken)
	}
	if len(reviewer.calls) != 0 || store.created() != 0 {
		t.Errorf("the reviewer was asked %d times and %d results created, want neither", len(reviewer.calls), store.created())
	}
}

// firstBrief is the brief of the first call of the reviewer, or "" when it was never asked.
func firstBrief(r *fakeReviewer) string {
	if len(r.calls) == 0 {
		return ""
	}
	return string(r.calls[0].brief)
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
