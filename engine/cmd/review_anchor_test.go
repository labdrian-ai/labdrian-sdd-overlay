package main

// What `review anchor` decides about a merge (own-rdd T7b, S7 a, f, h-j). The tests run the verb over a repository
// they make in a temporary directory, with the records the other review verbs stored, as the tests of `review check`
// do; no record is written by hand. The rules of the decision are in review/landing_test.go and
// review/app/landing_test.go; these tests are the command line, the output, the help and the lists of verbs.

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// recordFeature stores the reply of a reviewer who found nothing in the commits from base to head under the feature.
func (w *reviewWorld) recordFeature(feature, base, head string) {
	w.t.Helper()
	b := w.bindRange(base, head)
	if _, stderr, exits := w.run(reply(w.t, b, nil), "record", "--feature", feature, "--base", base, "--head", head, "--rows", "default"); !equalInts(exits, []int{0}) {
		w.t.Fatalf("setup: review record exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
}

// reviewedBranch makes the branch of a pull request on the commit the world is at, A: a code commit C, and, when
// unreviewed is set, a second code commit E that no record covers; then the review of A..C and the sweeps under the
// feature f, and last a passive commit D that adds odd/tasks/f.md and the records. It returns A and D and leaves the
// world on main, still at A.
func (w *reviewWorld) reviewedBranch(unreviewed bool) (a, d string) {
	w.t.Helper()
	a = w.head
	w.git("checkout", "-q", "-b", "feature")
	c := w.commitMore("d.txt")
	if unreviewed {
		w.commitMore("e.txt")
	}
	w.recordFeature("f", a, c)
	w.sweepAll("f")
	overlaytest.WriteFile(w.t, w.root, "odd/tasks/f.md", "a feature\n")
	w.git("add", "-A")
	w.git("commit", "-q", "-m", "docs: f", "-m", "Risk: none")
	d = w.name("HEAD")
	w.git("checkout", "-q", "main")
	return a, d
}

// mergeFeature merges the branch into main without a fast-forward and returns the merge M, which is HEAD.
func (w *reviewWorld) mergeFeature() string {
	w.t.Helper()
	w.git("merge", "-q", "--no-ff", "feature", "-m", "Merge pull request #1")
	return w.name("HEAD")
}

// A merge of a branch whose chain of reviews clears it, merged onto an unmoved base, lands the tree of the head: exit 0,
// nothing on stderr, and stdout is the object of S7 (f).
func TestReviewAnchorVerifiesTheMergeOfAReviewedBranch(t *testing.T) {
	w := newCheckedWorld(t)
	a, d := w.reviewedBranch(false)
	m := w.mergeFeature()

	stdout, stderr, exits := w.run("", "anchor", "--merge", "HEAD")

	want := fmt.Sprintf(`{
  "merge": %q,
  "base": %q,
  "head": %q,
  "tree": %q,
  "outcome": "verified",
  "features": [
    {
      "feature": "f",
      "outcome": "clear"
    }
  ]
}
`, m, a, d, w.name(d+"^{tree}"))
	if stdout != want || stderr != "" || !equalInts(exits, []int{0}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want %q, nothing, [0]", stdout, stderr, exits, want)
	}
}

// A merge made after the base moved lands another tree than the one reviewed: it is refused with the text of S7 (e).
func TestReviewAnchorRefusesAMergeAfterTheBaseMoved(t *testing.T) {
	w := newCheckedWorld(t)
	_, d := w.reviewedBranch(false)
	moved := w.commitMore("m.txt")
	m := w.mergeFeature()

	stdout, stderr, exits := w.run("", "anchor", "--merge", "HEAD")

	want := fmt.Sprintf("error: review anchor: the merge %s lands the tree %s, not the tree %s of the reviewed head %s: the branch did not hold %s when it was merged\n",
		m, w.name(m+"^{tree}"), w.name(d+"^{tree}"), d, moved)
	if stdout != "" || stderr != want || !equalInts(exits, []int{1}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q, [1]", stdout, stderr, exits, want)
	}
}

// A chain that does not clear refuses the merge, and the refusal says why in the words of `review check` for the same
// range: the commit no record covers.
func TestReviewAnchorNamesWhyTheChainIsRefused(t *testing.T) {
	w := newCheckedWorld(t)
	a, d := w.reviewedBranch(true)
	w.mergeFeature()

	_, checkErr, checkExits := w.run("", "check", "--feature", "f", "--base", a, "--head", d)
	reason, ok := strings.CutPrefix(checkErr, "error: review check: ")
	if !ok || !equalInts(checkExits, []int{1}) {
		t.Fatalf("setup: review check of the range exit calls = %v, stderr = %q; want [1] and its error", checkExits, checkErr)
	}

	stdout, stderr, exits := w.run("", "anchor", "--merge", "HEAD")

	want := "error: review anchor: no feature document the merge changes clears " + a + ".." + d + ": f: " + reason
	if stdout != "" || stderr != want || !equalInts(exits, []int{1}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q, [1]", stdout, stderr, exits, want)
	}
}

// A merge that changes no feature document has no feature: it passes with the outcome no-feature, no tree and an empty
// list of features.
func TestReviewAnchorPassesAMergeThatChangesNoFeatureDocument(t *testing.T) {
	w := newCheckedWorld(t)
	a := w.head
	w.git("checkout", "-q", "-b", "feature")
	d := w.commitMore("d.txt")
	w.git("checkout", "-q", "main")
	m := w.mergeFeature()

	stdout, stderr, exits := w.run("", "anchor", "--merge", "HEAD")

	want := fmt.Sprintf("{\n  \"merge\": %q,\n  \"base\": %q,\n  \"head\": %q,\n  \"outcome\": \"no-feature\",\n  \"features\": []\n}\n", m, a, d)
	if stdout != want || stderr != "" || !equalInts(exits, []int{0}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want %q, nothing, [0]", stdout, stderr, exits, want)
	}
}

// A commit that is not a merge is refused with the text of S7 (b).
func TestReviewAnchorRefusesACommitThatIsNotAMerge(t *testing.T) {
	w := newCheckedWorld(t)

	stdout, stderr, exits := w.run("", "anchor", "--merge", "HEAD")

	want := "error: review anchor: the commit " + w.head + " is not the merge of a pull request (the number of its parents is 1, not 2), so what it lands cannot be told to be reviewed\n"
	if stdout != "" || stderr != want || !equalInts(exits, []int{1}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q, [1]", stdout, stderr, exits, want)
	}
}

// A command line the verb cannot read is a usage error: exit 1, one line on standard error and nothing on standard
// output, before git is asked anything.
func TestReviewAnchorRefusesACommandLineItCannotRead(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no merge", []string{"anchor"}, "error: review anchor: --merge is required\n"},
		{"an empty merge", []string{"anchor", "--merge", ""}, "error: review anchor: --merge is required\n"},
		{"the merge twice", []string{"anchor", "--merge", "a", "--merge", "b"}, "error: review anchor: --merge given twice\n"},
		{"a merge with no value", []string{"anchor", "--merge"}, "error: review anchor: --merge requires a value\n"},
		{"an unknown option", []string{"anchor", "--merge", "HEAD", "--bogus", "x"}, "error: review anchor: unknown flag \"--bogus\"\n"},
		{"a flag of another verb", []string{"anchor", "--merge", "HEAD", "--feature", "f"}, "error: review anchor: unknown flag \"--feature\"\n"},
		{"a positional argument", []string{"anchor", "--merge", "HEAD", "extra"}, "error: review anchor: unexpected argument \"extra\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newCheckedWorld(t)

			stdout, stderr, exits := w.run("", tc.args...)

			if stdout != "" || stderr != tc.want || !equalInts(exits, []int{1}) {
				t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q, [1]", stdout, stderr, exits, tc.want)
			}
		})
	}
}

// --cwd names the repository the verb runs in, as for the other review verbs.
func TestReviewAnchorReadsTheRepositoryFromCwd(t *testing.T) {
	w := newCheckedWorld(t)
	elsewhere := t.TempDir()
	w.cwd = elsewhere

	_, _, outside := w.run("", "anchor", "--merge", "HEAD")
	stdout, stderr, inside := w.run("", "anchor", "--merge", "HEAD", "--cwd", w.root)

	if !equalInts(outside, []int{1}) {
		t.Errorf("exits outside the repository = %v, want [1]", outside)
	}
	if want := "error: review anchor: the commit " + w.head + " is not the merge"; !strings.HasPrefix(stderr, want) || stdout != "" || !equalInts(inside, []int{1}) {
		t.Errorf("with --cwd the repository is read: stdout = %q, stderr = %q, exits = %v; want nothing, %q..., [1]", stdout, stderr, inside, want)
	}
}

// The other verbs take the flags they took and refuse --merge as any flag they do not know: the dispatch of the new
// verb changed nothing for them.
func TestReviewAnchorLeavesTheOtherVerbsAsTheyWere(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"check", []string{"check", "--feature", "f", "--merge", "HEAD"}, "error: review check: unknown flag \"--merge\"\n"},
		{"check without a feature", []string{"check"}, "error: review check: --feature is required\n"},
		{"bind", []string{"bind", "--merge", "HEAD"}, "error: review bind: unknown flag \"--merge\"\n"},
		{"sweep", []string{"sweep", "--merge", "HEAD"}, "error: review sweep: unknown flag \"--merge\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newCheckedWorld(t)

			stdout, stderr, exits := w.run("", tc.args...)

			if stdout != "" || stderr != tc.want || !equalInts(exits, []int{1}) {
				t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q, [1]", stdout, stderr, exits, tc.want)
			}
		})
	}
}

// The verb is in the help of the engine, in the help of the wrapper, in the README and in the notes of the results
// directory, each saying what S7 (b)-(f) say.
func TestUsageReadmeAndWrapperDocumentReviewAnchor(t *testing.T) {
	var out strings.Builder
	usage(&out)
	flat := func(text string) string { return strings.Join(strings.Fields(text), " ") }
	file := func(path string) string {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return flat(string(raw))
	}
	for _, place := range []struct {
		name, text string
		holds      []string
	}{
		{"the engine usage", flat(out.String()), []string{
			"engine review anchor --merge <rev> [--cwd <repo>]",
			"lands a tree that stored reviews cleared",
			"exactly two parents",
			"exactly one merge base",
			"at least one feature's chain clears",
			"the tree of the merge must equal the tree of the head",
			"no-feature",
			"exit 0 verified or no-feature, 1 usage error, refusal or any other failure",
		}},
		{"bin/lib/usage.sh", file("../../bin/lib/usage.sh"), []string{
			"anchor --merge REV [--cwd REPO]",
			"exactly two parents",
			"exactly one merge base",
			"one feature's chain clears",
			"the tree of the merge must equal the tree of the head",
			"exit 0 verified or no-feature, 1 refused or error",
		}},
		{"README.md", file("../../README.md"), []string{
			"anchor --merge <rev> [--cwd <repo>]",
			"exactly two parents",
			"exactly one merge base",
			"at least one feature's chain clears",
			"the tree of the merge must equal the tree of the head",
			"Exit 0 verified or no-feature, 1 usage error, refusal or any other failure",
		}},
		{"odd/review-results/README.md", file("../../odd/review-results/README.md"), []string{
			"CI holds the merge of a pull request to the stored reviews of the feature documents it changes",
			"review anchor --merge HEAD",
		}},
	} {
		for _, want := range place.holds {
			if !strings.Contains(place.text, want) {
				t.Errorf("%s lacks %q", place.name, want)
			}
		}
	}
}
