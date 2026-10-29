package main

// What an exemption may cover and what a range may be (own-rdd T4, S5, L18, L21). An exemption binds exactly one
// commit, so the base of its range is the first parent of its head and a chain walk from a delivery base meets every
// commit: no exemption steps over a review that blocks a commit in the middle. A range whose base is its head covers
// no commit and clears nothing. The tests run the verbs over a repository they make in a temporary directory, as
// review_check_chain_test.go does.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
)

// exemptRange exempts the commits from base to head on a clear report and no risk, as the verb is run.
func (w *reviewWorld) exemptRange(base, head string) (stdout, stderr string, exits []int) {
	w.t.Helper()
	return w.run(verifierReport, "exempt", "--feature", "own-rdd", "--base", base, "--head", head, "--risk", "none")
}

// plant stores file under name in the results directory of the feature, as a file no verb makes any more would be.
func (w *reviewWorld) plant(name string, file []byte) {
	w.t.Helper()
	if err := os.MkdirAll(w.results("own-rdd"), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(w.results("own-rdd"), name), file, 0o644); err != nil {
		w.t.Fatal(err)
	}
}

// blockedAtThirdCommit makes the commits c0..c4 of the verifier's repro: clean results for c0..c1 and c1..c2, and a
// result for c2..c3 whose BLOCKER the verifier reproduced. It returns the five commits.
func (w *reviewWorld) blockedAtThirdCommit() [5]string {
	w.t.Helper()
	c := [5]string{w.base, w.head, w.commitMore("c2.txt"), w.commitMore("c3.txt"), w.commitMore("c4.txt")}
	w.recordRange(c[0], c[1])
	w.recordRange(c[1], c[2])
	_, stored := w.recordBlocker(c[2], c[3])
	if _, stderr, exits := w.run(verdictOn("reproduced"), "verify", "--feature", "own-rdd", "--stored", stored); !equalInts(exits, []int{0}) {
		w.t.Fatalf("setup: review verify exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	return c
}

// An exemption of more than one commit is refused with exit 1 and one line that names the rule, the base and the first
// parent of the head, and nothing is stored. So is the exemption of a head that has no parent: no base is its first
// parent.
func TestExemptRefusesARangeThatIsNotOneCommitAndStoresNothing(t *testing.T) {
	refused := func(t *testing.T, w *reviewWorld, base, head string, say ...string) {
		t.Helper()
		stdout, stderr, exits := w.exemptRange(base, head)
		if stdout != "" || !equalInts(exits, []int{1}) || !strings.HasPrefix(stderr, "error: review exempt: ") || strings.Count(stderr, "\n") != 1 {
			t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, one line of error: review exempt: ..., [1]", stdout, stderr, exits)
		}
		for _, want := range append([]string{"an exemption is for one commit"}, say...) {
			if !strings.Contains(stderr, want) {
				t.Errorf("stderr = %q, want it to say %q", stderr, want)
			}
		}
		if _, err := os.Stat(w.results("own-rdd")); err == nil {
			t.Error("the results directory was made for an exemption that was refused")
		}
	}
	t.Run("the base is further back than the first parent of the head", func(t *testing.T) {
		w := newCheckedWorld(t)
		tip := w.commitMore("d.txt")
		refused(t, w, w.base, tip, "the base is "+w.base, "the first parent of "+tip+" is "+w.head)
	})
	t.Run("the head has no parent", func(t *testing.T) {
		w := newCheckedWorld(t)
		refused(t, w, w.head, w.base, w.base+" has no parent")
	})
}

// The repro of the independent verifier: clean results for c0..c1 and c1..c2, a result for c2..c3 whose BLOCKER the
// verifier reproduced, and an exemption of c2..c4 that would step over c3. The exemption is refused, so c4 is cleared by
// nothing; and when c4 is exempted as the one commit it is, the walk from c0 meets c3 and its blocking result.
func TestAnExemptionCannotStepOverAReviewThatBlocksACommitInTheMiddleOfTheChain(t *testing.T) {
	w := newCheckedWorld(t)
	c := w.blockedAtThirdCommit()

	if _, _, exits := w.exemptRange(c[2], c[4]); !equalInts(exits, []int{1}) {
		t.Errorf("exempt c2..c4 exit calls = %v, want [1]: it would step over c3", exits)
	}
	w.wantNoChain(c[0], c[4], "head "+c[4]+": review: no stored result binds the head")

	if _, stderr, exits := w.exemptRange(c[3], c[4]); !equalInts(exits, []int{0}) {
		t.Fatalf("setup: exempt c3..c4 exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	w.wantNoChain(c[0], c[4], "head "+c[3]+": review: a stored result blocks the head", "odd/review-results/own-rdd/"+c[3][:12]+"-1.json", "blocking findings F1")
}

// Reviews are not held to one commit (only exemptions are): the fix of a commit whose review blocked is reviewed against
// the same base, which is the correction round of the verifier, so a clean result over two commits still clears the head
// and the chain goes through it. The result that blocked the commit in between is about a head the chain no longer passes.
func TestACleanResultOverTwoCommitsAfterACorrectionRoundStillClears(t *testing.T) {
	w := newCheckedWorld(t)
	first := w.recordRange(w.base, w.head)
	broken := w.commitMore("c2.txt")
	_, stored := w.recordBlocker(w.head, broken)
	if _, stderr, exits := w.run(verdictOn("reproduced"), "verify", "--feature", "own-rdd", "--stored", stored); !equalInts(exits, []int{0}) {
		t.Fatalf("setup: review verify exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	fixed := w.commitMore("c3.txt")
	corrected := w.recordRange(w.head, fixed)
	w.sweepAll()

	stdout, stderr, exits := w.checkChain(w.base)

	want := chainAnswer(fixed, w.base, resultRange(w.base, w.head, first.DiffSHA256), resultRange(w.head, fixed, corrected.DiffSHA256))
	if stdout != want || stderr != "" || !equalInts(exits, []int{0}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want %q, nothing, [0]", stdout, stderr, exits, want)
	}
}

// A stored exemption of more than one commit, such as one an older engine made or a hand wrote, never clears: the check
// holds the base of each exemption it reads to the first parent git gives for its head, for the head alone and for a
// chain, and the file is named.
func TestCheckRefusesAStoredExemptionThatCoversMoreThanOneCommit(t *testing.T) {
	w := newCheckedWorld(t)
	tip := w.commitMore("d.txt")
	b := w.bindRange(w.base, tip)
	file, err := review.EncodeExemption(review.Candidate{Base: b.Base, Head: b.Head, DiffSHA256: b.DiffSHA256, Paths: b.Paths}, review.NoRisk, []byte(verifierReport))
	if err != nil {
		t.Fatal(err)
	}
	name := tip[:12] + "-1.exempt.json"
	w.plant(name, file)

	w.wantNoChain(w.base, tip, "odd/review-results/own-rdd/"+name, "an exemption is for one commit")
	stdout, stderr, exits := w.run("", "check", "--feature", "own-rdd")
	if stdout != "" || !equalInts(exits, []int{1}) || !strings.Contains(stderr, "odd/review-results/own-rdd/"+name+": review: an exemption is for one commit") {
		t.Errorf("check of the head alone: stdout = %q, stderr = %q, exits = %v; want nothing, the file and the rule, [1]", stdout, stderr, exits)
	}
}

// A range whose base is its head covers no commit and clears nothing. The verbs that bind a range refuse it and store
// nothing, and a stored result over such a range, which no verb makes any more, never clears the head: not by itself and
// not as the end of a chain.
func TestARangeWhoseBaseIsItsHeadCoversNoCommitAndClearsNothing(t *testing.T) {
	const said = "base and head are the same commit"
	t.Run("bind, record and exempt refuse it", func(t *testing.T) {
		w := newCheckedWorld(t)
		for _, args := range [][]string{
			{"bind", "--base", w.head, "--head", w.head},
			{"record", "--feature", "own-rdd", "--base", w.head, "--head", w.head, "--rows", "default"},
			{"exempt", "--feature", "own-rdd", "--base", w.head, "--head", w.head, "--risk", "none"},
		} {
			stdout, stderr, exits := w.run(verifierReport, args...)
			if stdout != "" || !equalInts(exits, []int{1}) || !strings.Contains(stderr, said) {
				t.Errorf("%s: stdout = %q, stderr = %q, exits = %v; want nothing, %q, [1]", args[0], stdout, stderr, exits, said)
			}
		}
		for _, made := range []string{w.results("own-rdd"), w.worktree(w.head)} {
			if _, err := os.Stat(made); err == nil {
				t.Errorf("%s was made for a range that was refused", made)
			}
		}
	})
	t.Run("a stored result over it does not clear the head", func(t *testing.T) {
		w := newCheckedWorld(t)
		// A result of the empty diff of the head with itself, whole and consistent: its reply holds to its binding and the
		// binding is what git gives for the range, so only the range is wrong.
		empty := reviewBinding{Base: w.head, Head: w.head, DiffSHA256: sum(nil), Paths: []string{}}
		file, err := review.EncodeResult(review.Candidate{Base: empty.Base, Head: empty.Head, DiffSHA256: empty.DiffSHA256, Paths: empty.Paths}, []string{"default"}, []byte(reply(t, empty, nil)))
		if err != nil {
			t.Fatal(err)
		}
		w.plant(w.head[:12]+"-1.json", file)
		w.sweepAll()

		stdout, stderr, exits := w.run("", "check", "--feature", "own-rdd")

		if stdout != "" || !equalInts(exits, []int{1}) || !strings.Contains(stderr, "odd/review-results/own-rdd/"+w.head[:12]+"-1.json") || !strings.Contains(stderr, said) {
			t.Errorf("check of the head alone: stdout = %q, stderr = %q, exits = %v; want nothing, the file and %q, [1]", stdout, stderr, exits, said)
		}
		w.wantNoChain(w.head, w.head, said)
	})
}

// A review-diff that is not the range or the digest of the exemption the record names is refused saying the exemption
// it differs from, and not a result: the range and the diff are the exemption's.
func TestRolesAppendNamesTheExemptionWhenItsReviewDiffIsNotTheOneItStored(t *testing.T) {
	for _, tc := range []struct {
		name   string
		damage func(*roles.Evidence)
		say    string
	}{
		{"the ref is another range", func(e *roles.Evidence) { e.Ref = "main..HEAD" }, "the stored exemption is about the range "},
		{"the digest is another diff's", func(e *roles.Evidence) { e.SHA256 = strings.Repeat("e", 64) }, "the diff_sha256 stored with the exemption is "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rolesTestStateHome(t)
			w := newCheckedWorld(t)
			check := reviewEvidenceCheck(w.rolesDeps())
			record := w.exemptedSkeleton()
			for i := range record.Evidence {
				if record.Evidence[i].Kind == review.EvidenceDiff {
					tc.damage(&record.Evidence[i])
				}
			}
			prev := reviewerChain(t, check, "chain-1")

			r := runRolesChecked(check, rolesAppendArgs("chain-1"), rolesRecordJSON(t, fillSkeleton(record, "chain-1", prev)))

			if r.code != 1 || r.stdout != "" || !strings.Contains(r.stderr, "evidence review-diff") || !strings.Contains(r.stderr, tc.say) || strings.Contains(r.stderr, "stored result") {
				t.Errorf("code = %d, stdout = %q, stderr = %q; want exit 1, nothing on stdout, and the review-diff differing from %q without a stored result", r.code, r.stdout, r.stderr, tc.say)
			}
		})
	}
}
