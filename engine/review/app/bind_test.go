package app_test

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// The worktree of a review of the candidate of the fakes: beside the MAIN checkout of the repository,
// whichever worktree the command ran in (the fake repository is a linked worktree of /work/main).
var wantWorktree = filepath.FromSlash("/work/main-worktrees/review-222222222222")

// Bind turns the two revisions a person types into the candidate: full names, from git, and the digest
// and the paths of the diff between those names. What it asks git for is the full names, never the
// revisions, so the diff cannot move under it.
func TestBindResolvesTheRevisionsAndBindsTheDiff(t *testing.T) {
	g := newFakeGit()
	svc := app.New(g, newFakeResults())

	got, err := svc.Bind("/work/wt/feature/sub", "HEAD~1", "HEAD")
	if err != nil {
		t.Fatalf("Bind() error = %v, want nil", err)
	}

	want := candidateOf(t, g)
	if got.Candidate.Base != baseSHA || got.Candidate.Head != headSHA {
		t.Errorf("Candidate names %s and %s, want the full names %s and %s", got.Candidate.Base, got.Candidate.Head, baseSHA, headSHA)
	}
	if got.Candidate.DiffSHA256 != want.DiffSHA256 || got.Candidate.DiffBytes != len(diffText) {
		t.Errorf("Candidate digest %s over %d bytes, want %s over %d", got.Candidate.DiffSHA256, got.Candidate.DiffBytes, want.DiffSHA256, len(diffText))
	}
	if strings.Join(got.Candidate.Paths, ",") != strings.Join(changedPaths, ",") {
		t.Errorf("Candidate paths %q, want %q", got.Candidate.Paths, changedPaths)
	}
	for _, call := range []string{"Locate /work/wt/feature/sub", "Commit HEAD~1", "Commit HEAD", "Diff " + baseSHA + " " + headSHA, "Paths " + baseSHA + " " + headSHA} {
		if g.count(call) != 1 {
			t.Errorf("git calls = %q, want exactly one %q", g.calls, call)
		}
	}
}

func TestBindCreatesTheDetachedWorktreeBesideTheMainCheckout(t *testing.T) {
	g := newFakeGit()
	svc := app.New(g, newFakeResults())

	got, err := svc.Bind("/work/wt/feature", "HEAD~1", "HEAD")
	if err != nil {
		t.Fatalf("Bind() error = %v, want nil", err)
	}

	if got.Worktree != wantWorktree || !got.Created {
		t.Errorf("Worktree = %q (created %v), want %q created", got.Worktree, got.Created, wantWorktree)
	}
	if call := "AddWorktree " + wantWorktree + " " + headSHA; g.count(call) != 1 {
		t.Errorf("git calls = %q, want exactly one %q", g.calls, call)
	}
}

// A second bind of the same head finds the worktree where the first put it, at the head, and changes
// nothing: the binding is the same and the worktree is not created again.
func TestBindReusesTheWorktreeThatIsAtTheHead(t *testing.T) {
	g := newFakeGit()
	g.worktree = app.WorktreeState{Present: true, OfRepository: true, Head: headSHA}
	svc := app.New(g, newFakeResults())

	got, err := svc.Bind("/work/wt/feature", "HEAD~1", "HEAD")
	if err != nil {
		t.Fatalf("Bind() error = %v, want nil", err)
	}

	if got.Worktree != wantWorktree || got.Created {
		t.Errorf("Worktree = %q (created %v), want %q reused", got.Worktree, got.Created, wantWorktree)
	}
	if n := g.called("AddWorktree"); n != 0 {
		t.Errorf("git calls = %q, want no AddWorktree for a worktree that is already at the head", g.calls)
	}
}

// A worktree checked out at another commit would show the reviewer files that are not the candidate's.
func TestBindRefusesAWorktreeAtAnotherCommit(t *testing.T) {
	g := newFakeGit()
	g.worktree = app.WorktreeState{Present: true, OfRepository: true, Head: otherSHA}
	svc := app.New(g, newFakeResults())

	_, err := svc.Bind("/work/wt/feature", "HEAD~1", "HEAD")

	if !errors.Is(err, app.ErrWorktreeElsewhere) {
		t.Fatalf("Bind() error = %v, want one that is %v", err, app.ErrWorktreeElsewhere)
	}
	for _, want := range []string{wantWorktree, otherSHA, headSHA} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Bind() error = %q, want it to name %q", err, want)
		}
	}
	if n := g.called("AddWorktree"); n != 0 {
		t.Errorf("git calls = %q, want no AddWorktree", g.calls)
	}
}

// Something else at that path (a directory somebody made, a file, a worktree of another repository) is
// not ours to use or to remove.
func TestBindRefusesAPathThatIsNotAWorktreeOfTheRepository(t *testing.T) {
	g := newFakeGit()
	g.worktree = app.WorktreeState{Present: true}
	svc := app.New(g, newFakeResults())

	_, err := svc.Bind("/work/wt/feature", "HEAD~1", "HEAD")

	if !errors.Is(err, app.ErrWorktreeForeign) {
		t.Fatalf("Bind() error = %v, want one that is %v", err, app.ErrWorktreeForeign)
	}
	if !strings.Contains(err.Error(), wantWorktree) {
		t.Errorf("Bind() error = %q, want it to name %q", err, wantWorktree)
	}
	if n := g.called("AddWorktree"); n != 0 {
		t.Errorf("git calls = %q, want no AddWorktree", g.calls)
	}
}

// A diff that is over the limit is bound all the same, and the binding says so: the split into one
// launch per commit is the parent's job.
func TestBindSaysSoWhenTheDiffIsOverTheLimitAndDoesNotFail(t *testing.T) {
	g := newFakeGit()
	g.diff = bytes.Repeat([]byte("x"), review.DiffLimitBytes+1)
	svc := app.New(g, newFakeResults())

	got, err := svc.Bind("/work/wt/feature", "HEAD~1", "HEAD")
	if err != nil {
		t.Fatalf("Bind() error = %v, want nil for a diff over the limit", err)
	}

	if !got.Candidate.OverLimit() || got.Candidate.DiffBytes != review.DiffLimitBytes+1 {
		t.Errorf("Candidate: over limit %v with %d bytes, want over the limit with %d", got.Candidate.OverLimit(), got.Candidate.DiffBytes, review.DiffLimitBytes+1)
	}
	if !got.Created {
		t.Errorf("Created = false, want the worktree created for a diff over the limit too")
	}
}

// The worktree is made last: a candidate that cannot be bound leaves no directory behind.
func TestBindCreatesNoWorktreeForACandidateThatCannotBeBound(t *testing.T) {
	boom := errors.New("git: boom")
	for _, tc := range []struct {
		name  string
		setup func(g *fakeGit)
		want  error
	}{
		{"the diff cannot be read", func(g *fakeGit) { g.fail["Diff"] = boom }, boom},
		{"the paths cannot be read", func(g *fakeGit) { g.fail["Paths"] = boom }, boom},
		{"a path cannot be bound", func(g *fakeGit) { g.paths = []string{"caf\xe9.txt"} }, review.ErrInvalidCandidate},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newFakeGit()
			tc.setup(g)
			svc := app.New(g, newFakeResults())

			_, err := svc.Bind("/work/wt/feature", "HEAD~1", "HEAD")

			if !errors.Is(err, tc.want) {
				t.Fatalf("Bind() error = %v, want one that is %v", err, tc.want)
			}
			if g.called("Worktree") != 0 || g.called("AddWorktree") != 0 {
				t.Errorf("git calls = %q, want no call about the worktree", g.calls)
			}
		})
	}
}

// What git cannot answer is the error of the command, wrapped so that the cause is still there.
func TestBindReturnsWhatGitCouldNotAnswer(t *testing.T) {
	boom := errors.New("git: boom")
	for _, key := range []string{"Locate", "Commit HEAD~1", "Commit HEAD", "Diff", "Paths", "Worktree", "AddWorktree"} {
		t.Run(key, func(t *testing.T) {
			g := newFakeGit()
			g.fail[key] = boom
			svc := app.New(g, newFakeResults())

			_, err := svc.Bind("/work/wt/feature", "HEAD~1", "HEAD")

			if !errors.Is(err, boom) {
				t.Errorf("Bind() error = %v, want one that wraps %v", err, boom)
			}
		})
	}
}

// A revision that git would read as an option never reaches git: not even to locate the repository.
func TestBindRefusesARevisionThatIsAnOptionBeforeAskingGit(t *testing.T) {
	for _, tc := range []struct{ name, base, head string }{
		{"the base", "--output=/tmp/x", "HEAD"},
		{"the head", "HEAD~1", "-x"},
		{"an empty base", "", "HEAD"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newFakeGit()
			svc := app.New(g, newFakeResults())

			_, err := svc.Bind("/work/wt/feature", tc.base, tc.head)

			if !errors.Is(err, review.ErrInvalidRev) {
				t.Errorf("Bind() error = %v, want one that is %v", err, review.ErrInvalidRev)
			}
			if len(g.calls) != 0 {
				t.Errorf("git calls = %q, want none", g.calls)
			}
		})
	}
}
