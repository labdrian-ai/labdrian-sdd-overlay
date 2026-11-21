package main

// `review frozen-context` (labdrian-orchestrator-opencode T40, S8): the verb that reads a brief on stdin and prints
// the brief followed by the frozen context of its candidate, the bytes `review launch --runtime claude` hands the
// reviewer after its preamble. Every test goes through the command over the real git of a reviewWorld. No test of it
// may leave a stored result.

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/claudeprint"
)

// bindText runs `review bind` for the two commits of the world and returns what it printed, the form a brief names
// the base and the head in.
func (w *reviewWorld) bindText() string {
	w.t.Helper()
	stdout, stderr, exits := w.run("", "bind", "--base", w.base, "--head", w.head)
	if !equalInts(exits, []int{0}) || stderr != "" {
		w.t.Fatalf("setup: review bind exits = %v, stderr = %q; want [0] and nothing", exits, stderr)
	}
	return stdout
}

func TestFrozenContextPrintsWhatLaunchHandsTheReviewerForTheBrief(t *testing.T) {
	w := newLaunchWorld(t, false)
	brief := "Review this candidate.\n\n" + w.bindText()
	b := w.bind()
	w.claude.stdout = reply(t, b, nil)
	args := w.args(b)
	args[1] = "claude"
	if _, stderr, exits := w.launch(brief, args...); !equalInts(exits, []int{0}) {
		t.Fatalf("setup: launch exits = %v, stderr = %q", exits, stderr)
	}
	handed := strings.TrimPrefix(w.claude.stdin, claudeprint.Preamble()+"\n\n")
	if handed == w.claude.stdin {
		t.Fatalf("setup: the reviewer stdin %q does not start with the preamble", w.claude.stdin)
	}
	storedBefore := listNames(t, w.results("f"))

	stdout, stderr, exits := w.run(brief, "frozen-context")

	if !equalInts(exits, []int{0}) || stderr != "" || stdout != handed {
		t.Errorf("frozen-context printed %q / %q, exits %v; want what launch handed the reviewer, %q, and nothing on stderr", stdout, stderr, exits, handed)
	}
	if !strings.HasPrefix(stdout, brief+"\n\n## Frozen context (base "+b.Base+", head "+b.Head+")\n\nchanged paths: ") {
		t.Errorf("frozen-context printed %q; want the brief followed by its frozen context", stdout)
	}
	if names := listNames(t, w.results("f")); !slices.Equal(names, storedBefore) {
		t.Errorf("stored %v, was %v; want no new record", names, storedBefore)
	}
}

func TestFrozenContextAcceptsTheCwdFlag(t *testing.T) {
	w := newReviewWorld(t)
	brief := w.bindText()
	w.cwd = w.parent

	stdout, stderr, exits := w.run(brief, "frozen-context", "--cwd", w.root)

	if !equalInts(exits, []int{0}) || stderr != "" || !strings.HasPrefix(stdout, brief+"\n\n## Frozen context (base ") {
		t.Errorf("frozen-context --cwd printed %q / %q, exits %v; want the brief and its context", stdout, stderr, exits)
	}
}

func TestFrozenContextRefusals(t *testing.T) {
	w := newReviewWorld(t)
	b := w.bind()
	bindText := w.bindText()
	other := strings.Repeat("a", len(b.Head))
	prefix := "error: review frozen-context: "
	for _, tc := range []struct {
		name  string
		stdin string
		want  string
	}{
		{"empty stdin", "", prefix + "review: invalid brief: the brief is empty\n"},
		{"no pair", "Review this candidate.\n",
			prefix + "review: invalid brief: the brief names no base and head pair: it needs one \"base\" and one \"head\" member with a full object name, as review bind prints them\n"},
		{"the base alone", fmt.Sprintf("{\"base\": %q}\n", b.Base),
			prefix + "review: invalid brief: the brief names no base and head pair: it needs one \"base\" and one \"head\" member with a full object name, as review bind prints them\n"},
		{"the form of a range, not of review bind", "Review " + b.Base + ".." + b.Head + "\n",
			prefix + "review: invalid brief: the brief names no base and head pair: it needs one \"base\" and one \"head\" member with a full object name, as review bind prints them\n"},
		{"two different heads", bindText + fmt.Sprintf("{\"base\": %q, \"head\": %q}\n", b.Base, other),
			prefix + "review: invalid brief: the brief names more than one base and head pair, so it is not the brief of one candidate\n"},
		{"two different bases", bindText + fmt.Sprintf("{\"base\": %q, \"head\": %q}\n", other, b.Head),
			prefix + "review: invalid brief: the brief names more than one base and head pair, so it is not the brief of one candidate\n"},
		{"a brief that is not the one of the candidate", fmt.Sprintf("{\"base\": %q, \"head\": %q}\n", b.Base, b.Head),
			prefix + "the brief does not name the diff_sha256 " + b.DiffSHA256 + " of the binding, so it is not the brief of this candidate\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exits := w.run(tc.stdin, "frozen-context")

			if stdout != "" || stderr != tc.want || !equalInts(exits, []int{2}) {
				t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q, [2]", stdout, stderr, exits, tc.want)
			}
			if names := listNames(t, w.results("f")); len(names) != 0 {
				t.Errorf("stored %v; want no record", names)
			}
		})
	}
}

func TestFrozenContextRefusesADiffOverTheLimitOfOneLaunch(t *testing.T) {
	w := newReviewWorld(t)
	overlaytest.WriteFile(t, w.root, "big.txt", strings.Repeat("x", review.DiffLimitBytes+1)+"\n")
	w.git("add", "-A")
	w.git("commit", "-q", "-m", "big")
	w.head = w.name("HEAD")
	b := w.bind()
	if !b.OverLimit {
		t.Fatalf("setup: the candidate is %d bytes, not over the limit", b.DiffBytes)
	}

	stdout, stderr, exits := w.run(w.bindText(), "frozen-context")

	want := fmt.Sprintf("error: review frozen-context: the diff is %d bytes, over the %d-byte limit of one launch; split the candidate into one launch per commit (skills/role-chain/SKILL.md)\n", b.DiffBytes, review.DiffLimitBytes)
	if stdout != "" || stderr != want || !equalInts(exits, []int{2}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q, [2]", stdout, stderr, exits, want)
	}
}

func TestFrozenContextRefusesABriefAndContextOverTheReplyLimit(t *testing.T) {
	w := newReviewWorld(t)
	brief := w.bindText()
	plain, stderr, exits := w.run(brief, "frozen-context")
	if !equalInts(exits, []int{0}) || stderr != "" {
		t.Fatalf("setup: exits = %v, stderr = %q", exits, stderr)
	}
	context := len(plain) - len(brief)
	// The brief alone fits, the brief and its context do not.
	padded := brief + strings.Repeat(" ", review.MaxReplyBytes-len(brief)-context+1)
	if len(padded) > review.MaxReplyBytes {
		t.Fatalf("setup: the padded brief has %d bytes, over the limit", len(padded))
	}

	stdout, stderr, exits := w.run(padded, "frozen-context")

	want := fmt.Sprintf("error: review frozen-context: review: invalid brief: the brief and its frozen context are %d bytes, the maximum is %d\n", len(padded)+context, review.MaxReplyBytes)
	if stdout != "" || stderr != want || !equalInts(exits, []int{2}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q, [2]", stdout, stderr, exits, want)
	}
}

func TestFrozenContextUnknownFlagIsAUsageError(t *testing.T) {
	w := newReviewWorld(t)

	stdout, stderr, exits := w.run("", "frozen-context", "--feature", "f")

	if want := "error: review frozen-context: unknown flag \"--feature\"\n"; stdout != "" || stderr != want || !equalInts(exits, []int{1}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q, [1]", stdout, stderr, exits, want)
	}
}
