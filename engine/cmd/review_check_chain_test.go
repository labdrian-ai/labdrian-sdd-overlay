package main

// What `review check --base` decides about a chain of commits (own-rdd T4, S5, L18). Reviews are per commit: a head is
// cleared from a delivery base only when clear results or exemptions cover base..head as an unbroken chain, each range
// starting where the one before ended. The tests run the verb over a repository they make in a temporary directory,
// with the results the other verbs stored, as review_check_test.go does.

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// commitMore adds a commit on top of the world, so that it holds a chain of commits, and returns its full name. The
// commit declares its risk, "Risk: none", as every commit a builder makes does.
func (w *reviewWorld) commitMore(file string) string {
	w.t.Helper()
	overlaytest.WriteFile(w.t, w.root, file, file+"\n")
	w.git("add", "-A")
	w.git("commit", "-q", "-m", file, "-m", "Risk: none")
	return w.name("HEAD")
}

// bindRange binds the commits from base to head, as `review bind` does, and returns the binding.
func (w *reviewWorld) bindRange(base, head string) reviewBinding {
	w.t.Helper()
	stdout, stderr, exits := w.run("", "bind", "--base", base, "--head", head)
	if !equalInts(exits, []int{0}) {
		w.t.Fatalf("setup: review bind exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	return decodeBinding(w.t, stdout)
}

// recordRange stores the reply of a reviewer who found nothing in the commits from base to head and returns the
// binding of the candidate.
func (w *reviewWorld) recordRange(base, head string) reviewBinding {
	w.t.Helper()
	b := w.bindRange(base, head)
	if _, stderr, exits := w.run(reply(w.t, b, nil), "record", "--feature", "own-rdd", "--base", base, "--head", head, "--rows", "default"); !equalInts(exits, []int{0}) {
		w.t.Fatalf("setup: review record exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	return b
}

// checkChain runs `review check --base base` for HEAD of the world.
func (w *reviewWorld) checkChain(base string) (stdout, stderr string, exits []int) {
	w.t.Helper()
	return w.run("", "check", "--feature", "own-rdd", "--base", base)
}

// chainAnswer is what `review check --base` prints for a chain that clears head from base, with the ranges given.
func chainAnswer(head, base string, ranges ...string) string {
	return fmt.Sprintf("{\n  \"head\": %q,\n  \"base\": %q,\n  \"outcome\": \"clear\",\n  \"ranges\": [\n%s\n  ]\n}\n", head, base, strings.Join(ranges, ",\n"))
}

// resultRange is a range of the answer that one clean result clears, and exemptedRange one that an exemption clears.
func resultRange(base, head, digest string) string {
	return fmt.Sprintf(`    {
      "base": %[1]q,
      "head": %[2]q,
      "results": [
        {
          "result": "odd/review-results/own-rdd/%[3]s-1.json",
          "base": %[1]q,
          "diff_sha256": %[4]q
        }
      ]
    }`, base, head, head[:12], digest)
}

func exemptedRange(base, head, digest string) string {
	return fmt.Sprintf(`    {
      "base": %[1]q,
      "head": %[2]q,
      "results": [],
      "exemptions": [
        {
          "exemption": "odd/review-results/own-rdd/%[3]s-1.exempt.json",
          "base": %[1]q,
          "diff_sha256": %[4]q
        }
      ]
    }`, base, head, head[:12], digest)
}

// wantNoChain fails the test unless `review check --base base` was refused as every refusal of the check is: exit 1,
// nothing on stdout, one line on stderr that begins with the chain from base to head and says each of say, and
// nothing stored.
func (w *reviewWorld) wantNoChain(base, head string, say ...string) {
	w.t.Helper()
	w.sweepAll()
	before := listNames(w.t, w.results("own-rdd"))

	stdout, stderr, exits := w.checkChain(base)

	prefix := "error: review check: the chain from " + base + " to " + head + ": "
	if stdout != "" || !equalInts(exits, []int{1}) || !strings.HasPrefix(stderr, prefix) || strings.Count(stderr, "\n") != 1 {
		w.t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, one line that begins %q, [1]", stdout, stderr, exits, prefix)
	}
	for _, want := range say {
		if !strings.Contains(stderr, want) {
			w.t.Errorf("stderr = %q, want it to say %q", stderr, want)
		}
	}
	if got := listNames(w.t, w.results("own-rdd")); !reflect.DeepEqual(got, before) {
		w.t.Errorf("results = %q after a refused check, want %q", got, before)
	}
}

// A chain of clear ranges that begins at the base and ends at the head clears the head: exit 0, and stdout names the
// head, the base and the ranges from the one that starts at the base to the one that ends at the head, each with the
// file it used and what the candidate was recomputed to be. The walk stops at the base: the result of the range below
// it is not part of the chain. Nothing is stored and nothing is said on stderr.
func TestCheckBaseClearsAnUnbrokenChainOfClearRangesAndListsThemFromTheBase(t *testing.T) {
	w := newCheckedWorld(t)
	mid, tip := w.commitMore("d.txt"), w.commitMore("e.txt")
	w.recordRange(w.base, w.head)
	first, second := w.recordRange(w.head, mid), w.recordRange(mid, tip)
	w.sweepAll()
	before := listNames(t, w.results("own-rdd"))

	stdout, stderr, exits := w.checkChain(w.head)

	want := chainAnswer(tip, w.head, resultRange(w.head, mid, first.DiffSHA256), resultRange(mid, tip, second.DiffSHA256))
	if stdout != want || stderr != "" || !equalInts(exits, []int{0}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want %q, nothing, [0]", stdout, stderr, exits, want)
	}
	if got := listNames(t, w.results("own-rdd")); !reflect.DeepEqual(got, before) {
		t.Errorf("results = %q after the check, want %q: it stores nothing", got, before)
	}
}

// A range that an exemption clears is a range of the chain like one that a result clears, and the chain may end in it:
// the walk reaches the base through the exemption.
func TestCheckBaseClearsAChainThatEndsInAnExemption(t *testing.T) {
	w := newCheckedWorld(t)
	tip := w.commitMore("d.txt")
	exempted := w.bindRange(w.base, w.head)
	if _, stderr, exits := w.exempt(verifierReport, "none"); !equalInts(exits, []int{0}) {
		t.Fatalf("setup: review exempt exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	reviewed := w.recordRange(w.head, tip)
	w.sweepAll()

	stdout, stderr, exits := w.checkChain(w.base)

	want := chainAnswer(tip, w.base, exemptedRange(w.base, w.head, exempted.DiffSHA256), resultRange(w.head, tip, reviewed.DiffSHA256))
	if stdout != want || stderr != "" || !equalInts(exits, []int{0}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want %q, nothing, [0]", stdout, stderr, exits, want)
	}
}

// A commit that no stored result or exemption ends at breaks the chain: the check stops there and says so.
func TestCheckBaseRefusesAChainWithAGap(t *testing.T) {
	w := newCheckedWorld(t)
	mid, tip := w.commitMore("d.txt"), w.commitMore("e.txt")
	w.recordRange(w.base, w.head)
	w.recordRange(mid, tip) // nothing covers the commits from head to mid

	w.wantNoChain(w.base, tip, "head "+mid+": review: no stored result binds the head")
}

// A range that is not clear refuses the chain even when the ranges on both sides of it are clear.
func TestCheckBaseRefusesARangeThatIsNotClear(t *testing.T) {
	w := newCheckedWorld(t)
	mid, tip := w.commitMore("d.txt"), w.commitMore("e.txt")
	w.recordRange(w.base, w.head)
	w.recordBlocker(w.head, mid) // a severe finding nobody checked
	w.recordRange(mid, tip)

	w.wantNoChain(w.base, tip, "head "+mid+": review: a stored result is not clear",
		"odd/review-results/own-rdd/"+mid[:12]+"-1.json", "findings not checked F1")
}

// The walk must reach the base. One that passes it, because a stored range begins before it, and one that comes back
// to a commit it already passed, never reach it: both are refused, and the second does not loop.
func TestCheckBaseRefusesAChainThatDoesNotReachTheBase(t *testing.T) {
	t.Run("a range that begins before the base", func(t *testing.T) {
		w := newCheckedWorld(t)
		mid, tip := w.commitMore("d.txt"), w.commitMore("e.txt")
		w.recordRange(w.head, tip) // one review of both commits, and the delivery base is the commit in the middle

		w.wantNoChain(mid, tip, "head "+w.head+": review: no stored result binds the head")
	})
	t.Run("ranges that lead back to a commit the chain already passed", func(t *testing.T) {
		w := newCheckedWorld(t)
		tip := w.commitMore("d.txt")
		w.recordRange(w.head, tip)
		w.recordRange(tip, w.head) // the same commits reviewed the other way round

		w.wantNoChain(w.base, tip, "lead back to a commit the chain already passed", tip)
	})
}

// A passive commit is a link of the chain from its first parent with no record: the range it makes has no result and no
// exemption, and the chain goes through it to the review below (own-rdd T6b5, S6 d and f).
func TestCheckBaseTakesAPassiveCommitAsALinkOfTheChainWithNoRecord(t *testing.T) {
	w := newCheckedWorld(t)
	reviewed := w.recordRange(w.base, w.head)
	passive := w.commitPassive("own-rdd.md")
	w.sweepAll()

	stdout, stderr, exits := w.checkChain(w.base)

	link := fmt.Sprintf("    {\n      \"base\": %q,\n      \"head\": %q,\n      \"results\": []\n    }", w.head, passive)
	want := chainAnswer(passive, w.base, resultRange(w.base, w.head, reviewed.DiffSHA256), link)
	if stdout != want || stderr != "" || !equalInts(exits, []int{0}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want %q, nothing, [0]", stdout, stderr, exits, want)
	}
}

// A commit inside a range a review covers needs what a commit needs but its review: the check names it.
func TestCheckBaseHoldsTheCommitsInsideAReviewedRangeToWhatTheyNeed(t *testing.T) {
	w := newCheckedWorld(t)
	mid, tip := w.commitMore("d.txt"), w.commitMore("e.txt")
	w.recordRange(w.head, tip) // one review of both commits
	w.sweepAll()
	if err := os.Remove(filepath.Join(w.results("own-rdd"), mid[:12]+"-1.sweep.json")); err != nil { // mid has no other record, so its sweep is number 1
		t.Fatalf("setup: %v", err)
	}

	stdout, stderr, exits := w.checkChain(w.head)

	want := "error: review check: the chain from " + w.head + " to " + tip + ": commit " + mid + ": review: no sweep record binds the commit\n"
	if stdout != "" || stderr != want || !equalInts(exits, []int{1}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q, [1]", stdout, stderr, exits, want)
	}
}

// Without --base the check is by head alone, as it was: the ranges before the head are not looked at, and the answer
// has neither a base nor ranges.
func TestCheckWithoutBaseStaysTheCheckOfOneHeadAndAnswersAsBefore(t *testing.T) {
	w := newCheckedWorld(t)
	tip := w.commitMore("d.txt")
	reviewed := w.recordRange(w.head, tip) // the range from base to head was never reviewed
	w.sweepAll()

	stdout, stderr, exits := w.run("", "check", "--feature", "own-rdd")

	want := fmt.Sprintf(`{
  "head": %q,
  "outcome": "clear",
  "results": [
    {
      "result": "odd/review-results/own-rdd/%s-1.json",
      "base": %q,
      "diff_sha256": %q
    }
  ]
}
`, tip, tip[:12], w.head, reviewed.DiffSHA256)
	if stdout != want || stderr != "" || !equalInts(exits, []int{0}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want %q, nothing, [0]", stdout, stderr, exits, want)
	}
}

// topicOfTwoReviewedCommits makes the branch topic from the head of main, with two commits above it that are reviewed and
// swept one by one, leaves topic checked out and main where it was, and returns the fork point and the two commits.
func (w *reviewWorld) topicOfTwoReviewedCommits() (fork, mid, tip string) {
	w.t.Helper()
	fork = w.head
	w.git("checkout", "-q", "-b", "topic")
	mid, tip = w.commitMore("d.txt"), w.commitMore("e.txt")
	w.recordRange(fork, mid)
	w.recordRange(mid, tip)
	w.sweepAll()
	return fork, mid, tip
}

// moveMain adds a commit to main on top of fork, which topic does not have. topic stays checked out: the commit is made
// with plumbing, so the working tree and the results stored in it are not touched.
func (w *reviewWorld) moveMain(fork string) {
	w.t.Helper()
	moved := strings.TrimSpace(w.git("commit-tree", fork+"^{tree}", "-p", fork, "-m", "main moved on", "-m", "Risk: none"))
	w.git("branch", "-f", "main", moved)
}

// --onto names the integration branch of the delivery and the engine starts the chain where the branch was made (own-rdd
// T9a, S11 a). When main moved on after topic was made, --base main walks to a commit the chain never reaches and is
// refused, as it always was, while --onto main starts at the fork point and clears, printing what --base <fork point> does.
func TestCheckOntoClearsTheChainFromWhereTheBranchWasMade(t *testing.T) {
	w := newCheckedWorld(t)
	fork, mid, tip := w.topicOfTwoReviewedCommits()
	w.moveMain(fork)

	_, refusal, exits := w.checkChain("main")
	if want := "error: review check: the chain from " + w.name("main") + " to " + tip + ": head " + fork + ": review: no stored result binds the head\n"; refusal != want || !equalInts(exits, []int{1}) {
		t.Fatalf("setup: --base main: stderr = %q, exits = %v; want %q, [1]", refusal, exits, want)
	}
	wantOut, wantErr, wantExits := w.checkChain(fork)
	if wantErr != "" || !equalInts(wantExits, []int{0}) || !strings.Contains(wantOut, mid) {
		t.Fatalf("setup: --base <fork point>: stdout = %q, stderr = %q, exits = %v; want a cleared chain", wantOut, wantErr, wantExits)
	}

	stdout, stderr, exits := w.run("", "check", "--feature", "own-rdd", "--onto", "main")

	if stdout != wantOut || stderr != "" || !equalInts(exits, []int{0}) {
		t.Errorf("--onto main: stdout = %q, stderr = %q, exits = %v; want the stdout of --base %s (%q), nothing, [0]", stdout, stderr, exits, fork, wantOut)
	}
}

// When the branch did not move, the merge base is the branch itself and --onto is --base: same stdout, stderr and exit
// code, for a chain that clears and for one that is refused.
func TestCheckOntoOfABranchThatDidNotMoveIsCheckBase(t *testing.T) {
	for _, reviewed := range []bool{true, false} {
		w := newCheckedWorld(t)
		if reviewed {
			w.topicOfTwoReviewedCommits()
		} else {
			w.git("checkout", "-q", "-b", "topic")
			w.commitMore("d.txt")
			w.sweepAll()
		}
		baseOut, baseErr, baseExits := w.checkChain("main")

		ontoOut, ontoErr, ontoExits := w.run("", "check", "--feature", "own-rdd", "--onto", "main")

		if ontoOut != baseOut || ontoErr != baseErr || !reflect.DeepEqual(ontoExits, baseExits) {
			t.Errorf("reviewed %v: --onto main: stdout = %q, stderr = %q, exits = %v; want those of --base main: %q, %q, %v",
				reviewed, ontoOut, ontoErr, ontoExits, baseOut, baseErr, baseExits)
		}
		if reviewed != equalInts(baseExits, []int{0}) {
			t.Errorf("reviewed %v: --base main exits = %v; want 0 exactly when the chain is reviewed", reviewed, baseExits)
		}
	}
}

// A branch whose history shares no commit with the head has no merge base: the check says so, stores nothing and
// prints nothing.
func TestCheckOntoRefusesHistoriesWithNoMergeBase(t *testing.T) {
	w := newCheckedWorld(t)
	w.git("checkout", "-q", "--orphan", "island")
	w.git("commit", "-q", "--allow-empty", "-m", "island", "-m", "Risk: none")
	island := w.name("HEAD")
	w.git("checkout", "-q", "main")

	stdout, stderr, exits := w.run("", "check", "--feature", "own-rdd", "--onto", "island")

	want := "error: review check: " + island + " and " + w.head + " do not have exactly one merge base (they have 0), " +
		"so the commits the pull request adds cannot be told\n"
	if stdout != "" || stderr != want || !equalInts(exits, []int{1}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q, [1]", stdout, stderr, exits, want)
	}
}
