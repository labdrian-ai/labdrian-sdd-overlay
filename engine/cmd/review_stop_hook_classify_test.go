package main

// The classification question of the stop hook (S6, T4w, L84): once no review is asked for, the hook asks, once for each
// session and result, for the stored results of the branch whose kept findings docs/wiki/evidence.md does not hold, through
// the evidence.sh deployed under HOME. The script is a copy of the repository's, under a HOME of the test's own, never the
// real one (L54); a test that needs sh and awk skips locally and fails on CI without them.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
	reviewapp "github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// useHome gives the world a HOME of its own, with the evidence store deployed in it when withScript, and makes it the HOME
// of the process too, so nothing of the real one is read.
func (s *stopWorld) useHome(withScript bool) {
	s.t.Helper()
	overlaytest.RequireShAndAwk(s.t)
	s.userHome = s.t.TempDir()
	s.t.Setenv("HOME", s.userHome)
	if !withScript {
		return
	}
	src := filepath.Join(overlaytest.RepoRoot(s.t), "skills", "_shared", "evidence")
	for _, name := range []string{"evidence.sh", "evidence-rules.md"} {
		data, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			s.t.Fatal(err)
		}
		overlaytest.WriteFile(s.t, s.userHome, filepath.Join(".claude", "skills", "_shared", "evidence", name), string(data))
	}
}

// withKept puts one finding in the reply of a reviewer: F1, a WARNING with the given disposition.
func withKept(disposition string) func(map[string]any) {
	return func(o map[string]any) {
		o["findings"] = []any{map[string]any{
			"id": "F1", "location": "a.txt:2", "severity": "WARNING", "claim": "the second line breaks the first",
			"evidence_class": "deterministic", "causal_disposition": disposition, "proof_refs": []any{"a.txt:2"},
		}}
	}
}

// recordRangeKept records the reply of a reviewer who found one WARNING in the commits from base to head, and returns the
// identifier of the stored result, own-rdd/<head12>-<n>.
func (s *stopWorld) recordRangeKept(base, head, disposition string) string {
	s.t.Helper()
	b := s.bindRange(base, head)
	_, stderr, exits := s.run(reply(s.t, b, withKept(disposition)), "record", "--feature", "own-rdd", "--base", base, "--head", head, "--rows", "default")
	if !equalInts(exits, []int{0}) {
		s.t.Fatalf("setup: review record exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	return "own-rdd/" + strings.TrimSuffix(storedName(s.t, stderr), ".json")
}

// evidencePageHolding is a valid evidence page with one Evidence row, whose Review is review.
func evidencePageHolding(review string) string {
	return "# Evidence\n\n## Kinds\n\n| Kind | Level | Rule | Applies when | Source | Scope |\n|------|-------|------|--------------|--------|-------|\n" +
		"| alpha-kind | 1 | | | | project |\n" +
		"\n## Evidence\n\n| Kind | Level | Review | Date | Finding |\n|------|-------|--------|------|---------|\n" +
		"| alpha-kind | 1 | " + review + " | 2026-01-01 | x |\n"
}

// classifyLine is the line of the block for the result review of the commit head, with kept kept findings.
func classifyLine(review, head string, kept int) string {
	feature, stored, _ := strings.Cut(review, "/")
	return "- " + review + " (commit " + head[:12] + ", " + string(rune('0'+kept)) + " kept): labdrian review findings --feature " + feature + " --stored " + stored + ".json"
}

// classifyReasonOf is the reason of a block that asks for classification, with the lines given: the words L84 gives,
// written out here and not taken from the code.
func classifyReasonOf(lines ...string) string {
	return "Review results of the branch hold findings that docs/wiki/evidence.md does not hold yet:\n\n" +
		strings.Join(lines, "\n") + "\n\n" +
		"Run the Classify step of the role-chain skill for each. The stop hook names each result once for each session, and names it again beside a result it has not named yet."
}

// requireOneErrorLine requires stderr to be exactly one line that starts with prefix and holds every part.
func requireOneErrorLine(t *testing.T, stderr, prefix string, parts ...string) {
	t.Helper()
	if !strings.HasPrefix(stderr, prefix) || strings.Count(stderr, "\n") != 1 || !strings.HasSuffix(stderr, "\n") {
		t.Errorf("stderr = %q, want exactly one line starting %q", stderr, prefix)
	}
	for _, part := range parts {
		if !strings.Contains(stderr, part) {
			t.Errorf("stderr = %q, want it to hold %q", stderr, part)
		}
	}
}

const (
	cannotTellPrefix = "review stop-hook: tell whether the results with kept findings are classified: "
	incompletePrefix = "review stop-hook: classify the results with kept findings: "
)

// SH8: a result with a kept finding that the page does not hold is asked for once for each session, and not once a row
// holds it.
func TestStopHookAsksForTheClassificationOfAResultWithKeptFindings(t *testing.T) {
	s := newStopWorld(t)
	s.useHome(true)
	review := s.recordRangeKept(s.head, s.c4, "introduced")

	reason := s.blocked("s1")

	if want := classifyReasonOf(classifyLine(review, s.c4, 1)); reason != want {
		t.Errorf("the reason is %q, want %q", reason, want)
	}
	if !strings.Contains(reason, "Classify step") || strings.Contains(reason, "unfinished review steps") {
		t.Errorf("the reason is %q, want the classification question and no review one", reason)
	}
	s.allowed("s1")
	overlaytest.WriteFile(t, s.root, "docs/wiki/evidence.md", evidencePageHolding(review))
	s.allowed("s2")
}

// SH8b: a result with no kept finding is not asked for, and nothing is remembered.
func TestStopHookDoesNotAskForAResultWithNoKeptFinding(t *testing.T) {
	s := newStopWorld(t)
	s.useHome(true)
	s.recordRangeKept(s.head, s.c4, "pre-existing")

	s.allowed("s1")

	s.requireNothingRemembered(t)
}

// SH9: what the script cannot tell is one line on stderr, and the stop happens.
func TestStopHookSaysOnStderrWhatTheEvidenceScriptCannotTell(t *testing.T) {
	t.Run("no script deployed", func(t *testing.T) {
		s := newStopWorld(t)
		s.useHome(false)
		s.recordRangeKept(s.head, s.c4, "introduced")

		stdout, stderr, exits := s.stop("s1", false)

		if stdout != "" || !equalInts(exits, []int{0}) {
			t.Errorf("stdout = %q, exits = %v; want nothing and [0]", stdout, exits)
		}
		requireOneErrorLine(t, stderr, cannotTellPrefix, filepath.Join(s.userHome, ".claude", "skills", "_shared", "evidence", "evidence.sh"))
	})
	t.Run("a page it cannot read", func(t *testing.T) {
		s := newStopWorld(t)
		s.useHome(true)
		s.recordRangeKept(s.head, s.c4, "introduced")
		if err := os.MkdirAll(filepath.Join(s.root, "docs", "wiki", "evidence.md"), 0o755); err != nil {
			t.Fatal(err)
		}

		stdout, stderr, exits := s.stop("s1", false)

		if stdout != "" || !equalInts(exits, []int{0}) {
			t.Errorf("stdout = %q, exits = %v; want nothing and [0]", stdout, exits)
		}
		requireOneErrorLine(t, stderr, cannotTellPrefix, "exited 4", "cannot read")
	})
}

// junkResult stores a file the pass cannot read beside the results of the commit head.
func (s *stopWorld) junkResult(head string) {
	s.t.Helper()
	overlaytest.WriteFile(s.t, s.root, "odd/review-results/own-rdd/"+head[:12]+"-9.json", "not json\nsecond line\n")
}

// SH10: a block and an error of the same stop are both printed: the block on stdout, the error as one line on stderr.
func TestStopHookPrintsTheBlockAndTheErrorTogether(t *testing.T) {
	s := newStopWorld(t)
	s.useHome(true)
	review := s.recordRangeKept(s.head, s.c4, "introduced")
	s.junkResult(s.c4)

	stdout, stderr, exits := s.stop("s1", false)

	if want := blockLine(t, classifyReasonOf(classifyLine(review, s.c4, 1))); stdout != want || !equalInts(exits, []int{0}) {
		t.Errorf("stdout = %q, exits = %v; want %q and [0]", stdout, exits, want)
	}
	requireOneErrorLine(t, stderr, incompletePrefix, "-9.json")
}

// SH13: after a stop that failed, the next stop shows the question and the refusal together, and the one after neither.
func TestStopHookShowsTheQuestionAndTheRefusalAfterAFailedStop(t *testing.T) {
	s := newStopWorld(t)
	s.useHome(false)
	review := s.recordRangeKept(s.head, s.c4, "introduced")
	s.junkResult(s.c4)

	stdout, stderr, exits := s.stop("s1", false)

	if stdout != "" || !equalInts(exits, []int{0}) {
		t.Errorf("stop 1: stdout = %q, exits = %v; want nothing and [0]", stdout, exits)
	}
	requireOneErrorLine(t, stderr, cannotTellPrefix, "-9.json")

	s.useHome(true)
	stdout, stderr, exits = s.stop("s1", false)

	if want := blockLine(t, classifyReasonOf(classifyLine(review, s.c4, 1))); stdout != want || !equalInts(exits, []int{0}) {
		t.Errorf("stop 2: stdout = %q, exits = %v; want %q and [0]", stdout, exits, want)
	}
	requireOneErrorLine(t, stderr, incompletePrefix, "-9.json")
	if strings.Contains(stderr, "tell whether") {
		t.Errorf("stop 2: stderr = %q, want no word of the failed stop", stderr)
	}

	s.allowed("s1")
}

// The reason of a block that only asks for classification names every result in order and no commit to review.
func TestStopReasonOfAClassificationBlockNamesEveryResultAndNoCommit(t *testing.T) {
	head1, head2 := strings.Repeat("a", 40), strings.Repeat("b", 40)
	ruling := reviewapp.Ruling{Block: true, Classify: []reviewapp.KeptResult{
		{Review: "one/" + head1[:12] + "-1", Head: head1, Kept: 2},
		{Review: "two/" + head2[:12] + "-3", Head: head2, Kept: 1},
	}}

	got := stopReason(ruling)

	want := classifyReasonOf(classifyLine(ruling.Classify[0].Review, head1, 2), classifyLine(ruling.Classify[1].Review, head2, 1))
	if got != want {
		t.Errorf("stopReason = %q, want %q", got, want)
	}
	if strings.Contains(got, "unfinished review steps") {
		t.Errorf("stopReason = %q, want no review question", got)
	}
}
