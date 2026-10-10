package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// newReviewReceiptService is the one place the review receipt capture learns which git to ask
// and which environment to ask it in. The adapter and gitprov are proved against fakes and
// real repositories where they live; what is pinned here is the wiring, from a throwaway
// directory and never from the repository the tests run in: the real git binary is the
// runner, and the process environment is the environment git is judged against.
func TestNewReviewReceiptServiceAsksTheRealGitInTheProcessEnvironment(t *testing.T) {
	t.Run("the runner is the real git binary", func(t *testing.T) {
		if _, err := exec.LookPath("git"); err != nil {
			t.Skipf("git unavailable: %v", err)
		}
		dir := t.TempDir()
		gitInit(t, dir)
		svc, err := newReviewReceiptService(dir, os.Environ())
		if err != nil {
			t.Fatal(err)
		}
		// Nothing is waiting in the repository's transaction stores, so the capture finds
		// nothing: it can only say so after git answered where the stores are.
		if captured, err := svc.Capture("a-change"); err != nil || len(captured) != 0 {
			t.Errorf("Capture in a fresh repository = %v, %v, want nothing captured and no error", captured, err)
		}
	})

	t.Run("a directory that is not a repository is git's to refuse", func(t *testing.T) {
		if _, err := exec.LookPath("git"); err != nil {
			t.Skipf("git unavailable: %v", err)
		}
		svc, err := newReviewReceiptService(t.TempDir(), os.Environ())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Capture("a-change"); err == nil || !strings.Contains(err.Error(), "git rev-parse") {
			t.Errorf("Capture outside a repository = %v, want the refusal of the git question", err)
		}
	})

	t.Run("the environment is the process's own", func(t *testing.T) {
		// gitprov refuses to judge a repository while the environment redirects git to
		// another one, so the refusal can only come from the environment the service was
		// wired with: a service wired with an empty environment would not see it.
		t.Setenv("GIT_DIR", t.TempDir())
		svc, err := newReviewReceiptService(t.TempDir(), os.Environ())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Capture("a-change"); err == nil || !strings.Contains(err.Error(), "GIT_DIR") {
			t.Errorf("Capture with GIT_DIR set = %v, want the refusal naming GIT_DIR", err)
		}
	})
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "init", "-q")
	cmd.Env = goldenEnvironment()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
}

// Both verbs that run on the service build it the same way and, when it cannot be built, say
// why in the verb's own words and exit with the code its contract gives a failure: 1 for the
// capture a person runs, 2 for the hook, whose exit code 2 denies the acknowledgement. The
// words tell a set-up failure apart from a failure of the capture itself.
func TestBuildReviewReceiptServiceFailsInTheWordsAndWithTheCodeOfTheVerb(t *testing.T) {
	for _, tc := range []struct {
		name   string
		prefix string
		code   int
		want   string
	}{
		{"the capture verb", "error: review-receipt capture", 1,
			"error: review-receipt capture: set up failed: review receipt store: the project root is required\n"},
		{"the hook verb", "review-receipt hook", 2,
			"review-receipt hook: set up failed: review receipt store: the project root is required\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			var exits []int
			svc := buildReviewReceiptService("", os.Environ(), &stderr, func(code int) { exits = append(exits, code) }, tc.prefix, tc.code)
			if svc != nil {
				t.Errorf("service = %v, want none", svc)
			}
			if stderr.String() != tc.want {
				t.Errorf("stderr = %q, want %q", stderr.String(), tc.want)
			}
			if len(exits) != 1 || exits[0] != tc.code {
				t.Errorf("exits = %v, want exactly one exit with code %d", exits, tc.code)
			}
		})
	}

	t.Run("a service that can be built is returned in silence", func(t *testing.T) {
		var stderr bytes.Buffer
		exited := false
		svc := buildReviewReceiptService(t.TempDir(), os.Environ(), &stderr, func(int) { exited = true }, "p", 1)
		if svc == nil || stderr.Len() != 0 || exited {
			t.Errorf("service = %v, stderr = %q, exited = %v, want a service and silence", svc, stderr.String(), exited)
		}
	})
}
