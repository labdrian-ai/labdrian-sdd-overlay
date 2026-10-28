package main

// What `review check` decides about a head (own-rdd T4, S5). The tests run the verb over a repository they make in a
// temporary directory, with the results the other verbs stored, as the tests of the other review verbs do. Its
// command line, its help and the built program are in review_check_cli_test.go.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// declaresRisk ends the message of the head of the world, which must be HEAD, with the trailer "Risk: value" and follows
// the new commit in w.head: the amended commit is another object, so call it before anything is bound or stored for the
// head.
func (w *reviewWorld) declaresRisk(value string) {
	w.t.Helper()
	w.git("commit", "-q", "--amend", "-m", "head", "-m", "Risk: "+value)
	w.head = w.name("HEAD")
}

// newCheckedWorld is the world of a delivery: its head declares its risk in the trailer "Risk: none", as every commit a
// builder makes does (own-rdd T6b5, S6 a), so `review check` and `review exempt` read it.
func newCheckedWorld(t *testing.T) *reviewWorld {
	t.Helper()
	w := newReviewWorld(t)
	w.declaresRisk("none")
	return w
}

// sweepAll runs `review sweep` for every first-parent commit above the base of the world that has no sweep record yet in
// a feature (own-rdd when none is named), so that what `review check` asks of a commit besides its review is there. The
// working tree of the world is clean, so every sweep is clean. It takes the next number of each commit, so call it after
// the results and exemptions whose numbers a test expects.
func (w *reviewWorld) sweepAll(features ...string) {
	w.t.Helper()
	if len(features) == 0 {
		features = []string{"own-rdd"}
	}
	state := w.t.TempDir()
	for _, commit := range strings.Fields(w.git("rev-list", "--first-parent", "--reverse", w.base+"..HEAD")) {
		for _, feature := range features {
			if stored, _ := filepath.Glob(filepath.Join(w.results(feature), commit[:12]+"-*.sweep.json")); len(stored) > 0 {
				continue
			}
			stdout, stderr, exits := w.sweepRun(state, "", "sweep", "--feature", feature, "--base", commit+"^", "--head", commit)
			if !equalInts(exits, []int{0}) || decodeSwept(w.t, stdout).Outcome != "clean" {
				w.t.Fatalf("setup: review sweep of %s exit calls = %v, stdout = %q, stderr = %q; want [0] and a clean sweep", commit, exits, stdout, stderr)
			}
		}
	}
}

// recordClean records the reply of a reviewer who found nothing in the commits of the world, under the feature
// own-rdd, and returns the binding of the candidate and the name of the stored result.
func (w *reviewWorld) recordClean() (reviewBinding, string) {
	w.t.Helper()
	b := w.bind()
	if _, stderr, exits := w.run(reply(w.t, b, nil), "record", "--feature", "own-rdd", "--base", w.base, "--head", w.head, "--rows", "default"); !equalInts(exits, []int{0}) {
		w.t.Fatalf("setup: review record exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	return b, w.head[:12] + "-1.json"
}

// withThreeFindings puts three findings in the reply of a reviewer: a BLOCKER, a WARNING and a SUGGESTION.
func withThreeFindings(o map[string]any) {
	finding := func(id, severity string) map[string]any {
		return map[string]any{
			"id": id, "location": "a.txt:2", "severity": severity, "claim": "the second line breaks the first",
			"evidence_class": "deterministic", "causal_disposition": "introduced", "proof_refs": []any{"a.txt:2"},
		}
	}
	o["findings"] = []any{finding("F1", "BLOCKER"), finding("F2", "WARNING"), finding("F3", "SUGGESTION")}
}

// A head that a stored clean result binds is cleared: exit 0, and stdout names the head, the files used and what
// they were recomputed to be. Nothing is stored and nothing is said on stderr.
func TestCheckClearsTheHeadOfAStoredResultAndPrintsTheFilesItUsed(t *testing.T) {
	w := newCheckedWorld(t)
	b, name := w.recordClean()
	w.sweepAll()
	before := listNames(t, w.results("own-rdd"))

	stdout, stderr, exits := w.run("", "check", "--feature", "own-rdd")

	want := fmt.Sprintf(`{
  "head": %q,
  "outcome": "clear",
  "results": [
    {
      "result": "odd/review-results/own-rdd/%s",
      "base": %q,
      "diff_sha256": %q
    }
  ]
}
`, w.head, name, w.base, b.DiffSHA256)
	if stdout != want || stderr != "" || !equalInts(exits, []int{0}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want %q, nothing, [0]", stdout, stderr, exits, want)
	}
	if got := listNames(t, w.results("own-rdd")); strings.Join(got, ",") != strings.Join(before, ",") {
		t.Errorf("results = %q after the check, want %q: it stores nothing", got, before)
	}
}

// When the verifier ran, the verdict beside the result is named, and the findings to fix before delivery and to carry
// are listed: a severe finding it did not reproduce does not block.
func TestCheckNamesTheVerdictAndTheFindingsToFixAndToCarry(t *testing.T) {
	w := newCheckedWorld(t)
	b := w.bind()
	if _, stderr, exits := w.run(reply(t, b, withThreeFindings), "record", "--feature", "own-rdd", "--base", w.base, "--head", w.head, "--rows", "default"); !equalInts(exits, []int{0}) {
		t.Fatalf("setup: review record exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	verdicts := `{"verdicts":[` +
		`{"id":"F1","result":"not-reproduced","command":"go test ./...","output":"ok","base_result":"not-run"},` +
		`{"id":"F2","result":"not-run","command":"","output":"not checked","base_result":"not-run"},` +
		`{"id":"F3","result":"not-run","command":"","output":"not checked","base_result":"not-run"}]}`
	if _, stderr, exits := w.run(verdicts, "verify", "--feature", "own-rdd", "--stored", w.head[:12]+"-1.json"); !equalInts(exits, []int{0}) {
		t.Fatalf("setup: review verify exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}

	w.sweepAll()

	stdout, stderr, exits := w.run("", "check", "--feature", "own-rdd", "--head", w.head)

	want := fmt.Sprintf(`{
  "head": %q,
  "outcome": "clear",
  "results": [
    {
      "result": "odd/review-results/own-rdd/%[2]s-1.json",
      "verdict": "odd/review-results/own-rdd/%[2]s-1.verifier.json",
      "base": %[3]q,
      "diff_sha256": %[4]q,
      "fix": [
        "F2"
      ],
      "carry": [
        "F3"
      ]
    }
  ]
}
`, w.head, w.head[:12], w.base, b.DiffSHA256)
	if stdout != want || stderr != "" || !equalInts(exits, []int{0}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want %q, nothing, [0]", stdout, stderr, exits, want)
	}
}

// Everything that keeps a head from delivery is exit 1 with the reason named on stderr, and nothing on stdout: the
// head was never reviewed (or another head was), the result blocks, a severe finding was never checked, the stored
// result is not the one git binds now, the stored file is not a result any more.
func TestCheckRefusesTheHeadNoStoredResultClearsAndStoresNothing(t *testing.T) {
	zeros := strings.Repeat("0", 64)
	for _, tc := range []struct {
		name  string
		setup func(w *reviewWorld)
		args  []string // the command line after "check"; the feature own-rdd and the head HEAD when nil
		say   func(w *reviewWorld) []string
	}{
		{"the head was never reviewed", func(*reviewWorld) {}, nil, func(w *reviewWorld) []string {
			return []string{"head " + w.head + ": review: no stored result binds the head"}
		}},
		{"another head was reviewed", func(w *reviewWorld) {
			w.recordClean()
			w.commitMore("d.txt") // the head to deliver is the commit after the one that was reviewed
		}, nil, func(w *reviewWorld) []string {
			return []string{"head " + w.name("HEAD") + ": review: no stored result binds the head"}
		}},
		{"another feature was reviewed", func(w *reviewWorld) { w.recordClean() }, []string{"--feature", "other"}, func(w *reviewWorld) []string {
			return []string{"no stored result binds the head"}
		}},
		{"the result blocks", func(w *reviewWorld) {
			_, stored := w.recordBlocker(w.base, w.head)
			if _, stderr, exits := w.run(verdictOn("reproduced"), "verify", "--feature", "own-rdd", "--stored", stored); !equalInts(exits, []int{0}) {
				w.t.Fatalf("setup: review verify exit calls = %v, stderr = %q; want [0]", exits, stderr)
			}
		}, nil, func(w *reviewWorld) []string {
			return []string{"a stored result blocks the head", "odd/review-results/own-rdd/" + w.head[:12] + "-1.json", "odd/review-results/own-rdd/" + w.head[:12] + "-1.verifier.json", "blocking findings F1"}
		}},
		{"a severe finding nobody checked", func(w *reviewWorld) { w.recordBlocker(w.base, w.head) }, nil, func(w *reviewWorld) []string {
			return []string{"a stored result is not clear", "odd/review-results/own-rdd/" + w.head[:12] + "-1.json", "no verdict of the verifier is stored", "findings not checked F1"}
		}},
		{"the stored result is about a diff git does not give now", func(w *reviewWorld) {
			b, name := w.recordClean()
			path := filepath.Join(w.results("own-rdd"), name)
			data, err := os.ReadFile(path)
			if err != nil {
				w.t.Fatal(err)
			}
			if err := os.WriteFile(path, bytes.ReplaceAll(data, []byte(b.DiffSHA256), []byte(zeros)), 0o644); err != nil {
				w.t.Fatal(err)
			}
		}, nil, func(w *reviewWorld) []string {
			return []string{"odd/review-results/own-rdd/" + w.head[:12] + "-1.json", "the stored binding differs from the candidate git gives", "diff_sha256 is " + zeros}
		}},
		{"the stored result records no rows (it was stored before the rows were recorded)", func(w *reviewWorld) {
			_, name := w.recordClean()
			w.dropRows(name)
		}, nil, func(w *reviewWorld) []string {
			return []string{"odd/review-results/own-rdd/" + w.head[:12] + "-1.json", "invalid stored file", "rows"}
		}},
		{"the stored file is no longer a result", func(w *reviewWorld) {
			_, name := w.recordClean()
			if err := os.WriteFile(filepath.Join(w.results("own-rdd"), name), []byte("{}"), 0o644); err != nil {
				w.t.Fatal(err)
			}
		}, nil, func(w *reviewWorld) []string {
			return []string{"odd/review-results/own-rdd/" + w.head[:12] + "-1.json", "invalid stored file"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newCheckedWorld(t)
			tc.setup(w)
			w.sweepAll("own-rdd", "other")
			before := listNames(t, w.results("own-rdd"))
			args := tc.args
			if args == nil {
				args = []string{"--feature", "own-rdd"}
			}

			stdout, stderr, exits := w.run("", append([]string{"check"}, args...)...)

			if stdout != "" || !equalInts(exits, []int{1}) || !strings.HasPrefix(stderr, "error: review check: ") || strings.Count(stderr, "\n") != 1 {
				t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, one line of error: review check: ..., [1]", stdout, stderr, exits)
			}
			for _, want := range tc.say(w) {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr = %q, want it to say %q", stderr, want)
				}
			}
			if got := listNames(t, w.results("own-rdd")); strings.Join(got, ",") != strings.Join(before, ",") {
				t.Errorf("results = %q after a refused check, want %q", got, before)
			}
		})
	}
}

// A head that is not a commit, and a head or a base git would read as an option, are refused before git reads a
// thing: the option writes no file.
func TestCheckRefusesWhatIsNotACommitAndWhatGitWouldReadAsAnOption(t *testing.T) {
	w := newCheckedWorld(t)
	w.recordClean()
	marker := filepath.Join(w.parent, "written-by-an-option")

	for _, tc := range []struct{ name, flag, head, want string }{
		{"a name that is nothing", "--head", "nonesuch", `resolve head "nonesuch"`},
		{"an option", "--head", "--output=" + marker, "starts with '-' and would be read as an option"},
		{"a base that is an option", "--base", "--output=" + marker, "starts with '-' and would be read as an option"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exits := w.run("", "check", "--feature", "own-rdd", tc.flag, tc.head)

			if stdout != "" || !equalInts(exits, []int{1}) || !strings.Contains(stderr, tc.want) {
				t.Errorf("stdout = %q, exits = %v, stderr = %q; want nothing, [1] and %q", stdout, exits, stderr, tc.want)
			}
			if _, err := os.Stat(marker); err == nil {
				t.Error("a file was written by a revision that is an option")
			}
		})
	}
}

// The repository is the one --cwd names, a relative one against the working directory of the process, and the
// head is HEAD of that repository: outside a repository the check is refused, not cleared.
func TestCheckReadsTheRepositoryFromCwdAndResolvesARelativeOneAgainstTheWorkingDirectory(t *testing.T) {
	w := newCheckedWorld(t)
	w.recordClean()
	w.sweepAll()
	w.cwd = w.parent

	stdout, stderr, exits := w.run("", "check", "--feature", "own-rdd", "--cwd", "repo")

	if !equalInts(exits, []int{0}) || stderr != "" || !strings.Contains(stdout, `"head": "`+w.head+`"`) {
		t.Fatalf("exit calls = %v, stderr = %q, stdout = %q; want [0], nothing and the head cleared", exits, stderr, stdout)
	}
	_, stderr, exits = w.run("", "check", "--feature", "own-rdd", "--cwd", t.TempDir())
	if !equalInts(exits, []int{1}) || !strings.HasPrefix(stderr, "error: review check: locate the repository of ") {
		t.Errorf("outside a repository: exits = %v, stderr = %q; want [1] and an error of review check", exits, stderr)
	}
}

// wantCheckRefused fails the test unless `review check` for the head of the world was refused as every refusal of the
// check is: exit 1, nothing on stdout, one line on stderr that is `error: review check: ` and the reason (or contains
// it, when contains), and nothing stored.
func (w *reviewWorld) wantCheckRefused(reason string, contains bool, args ...string) {
	w.t.Helper()
	before := listNames(w.t, w.results("own-rdd"))

	stdout, stderr, exits := w.run("", append([]string{"check", "--feature", "own-rdd"}, args...)...)

	line := strings.TrimSuffix(stderr, "\n")
	matches := line == "error: review check: "+reason
	if contains {
		matches = strings.HasPrefix(line, "error: review check: ") && strings.Contains(line, reason)
	}
	if stdout != "" || !equalInts(exits, []int{1}) || !matches || strings.Count(stderr, "\n") != 1 {
		w.t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, one line of error: review check: %s, [1]", stdout, stderr, exits, reason)
	}
	if got := listNames(w.t, w.results("own-rdd")); strings.Join(got, ",") != strings.Join(before, ",") {
		w.t.Errorf("results = %q after a refused check, want %q", got, before)
	}
}

// A commit needs more than a review before `review check` clears it (own-rdd T6b5, S6 d and f), and the check refuses
// with the first thing it lacks, in this order: its risk trailer, its sweep, the sweeper's answer to a fired sweep, and
// at last its review. Each refusal is exit 1 with the reason on stderr and nothing on stdout.
func TestCheckAsksForWhatTheCommitNeedsInTheOrderOfTheDesign(t *testing.T) {
	t.Run("the risk trailer", func(t *testing.T) {
		w := newReviewWorld(t) // the head's message is one word: it declares no risk

		w.wantCheckRefused("commit "+w.head+`: it has no "Risk:" trailer in the final trailer block of its message`, true)
	})
	t.Run("the sweep, the sweeper and then the review", func(t *testing.T) {
		w := newCheckedWorld(t)
		state := t.TempDir()
		short := w.head[:12]

		w.wantCheckRefused("commit "+w.head+": review: no sweep record binds the commit", false)
		firedSweep(t, w, state) // the sweep takes number 1 and fires on a file left in the working tree
		w.wantCheckRefused("commit "+w.head+": review: no sweeper record answers the fired sweep record: odd/review-results/own-rdd/"+short+"-1.sweep.json", false)
		if _, stderr, exits := w.sweepRun(state, strayReport, "record", "--role", "sweeper", "--feature", "own-rdd", "--base", w.base, "--head", w.head); !equalInts(exits, []int{0}) {
			t.Fatalf("setup: review record --role sweeper exit calls = %v, stderr = %q; want [0]", exits, stderr)
		}
		w.wantCheckRefused("head "+w.head+": review: no stored result binds the head", false)
		w.recordClean() // the result takes number 3

		stdout, stderr, exits := w.run("", "check", "--feature", "own-rdd")

		if !equalInts(exits, []int{0}) || stderr != "" || !strings.Contains(stdout, `"result": "odd/review-results/own-rdd/`+short+`-3.json"`) {
			t.Errorf("stdout = %q, stderr = %q, exits = %v; want the head cleared by result 3, nothing on stderr, [0]", stdout, stderr, exits)
		}
	})
	t.Run("the polisher of a commit that declares an item", func(t *testing.T) {
		w := newCheckedWorld(t)
		w.declaresRisk("3")
		w.sweepAll()

		w.wantCheckRefused("commit "+w.head+": review: no polisher record binds a commit that declares a high-risk item: Risk: 3", false)
		if _, stderr, exits := w.sweepRun(t.TempDir(), `{"findings":[]}`, "record", "--role", "polisher", "--feature", "own-rdd", "--base", w.base, "--head", w.head); !equalInts(exits, []int{0}) {
			t.Fatalf("setup: review record --role polisher exit calls = %v, stderr = %q; want [0]", exits, stderr)
		}
		w.wantCheckRefused("head "+w.head+": review: no stored result binds the head", false)
	})
}

// commitPassive adds a commit on top of the world that changes only a task document and declares Risk: none: a passive
// commit (own-rdd T6b5, S6 d).
func (w *reviewWorld) commitPassive(file string) string {
	w.t.Helper()
	overlaytest.WriteFile(w.t, w.root, "odd/tasks/"+file, "a task\n")
	w.git("add", "-A")
	w.git("commit", "-q", "-m", "docs: "+file, "-m", "Risk: none")
	return w.name("HEAD")
}

// A passive commit needs no record: it is cleared with no result and no sweep, and the answer lists no file, as the
// head of a range of results that holds none: `results` is an empty array and there is no `exemptions`.
func TestCheckClearsAPassiveHeadWithNoRecordAndPrintsNoFile(t *testing.T) {
	w := newCheckedWorld(t)
	passive := w.commitPassive("own-rdd.md")
	before := listNames(t, w.results("own-rdd"))

	stdout, stderr, exits := w.run("", "check", "--feature", "own-rdd")

	want := fmt.Sprintf("{\n  \"head\": %q,\n  \"outcome\": \"clear\",\n  \"results\": []\n}\n", passive)
	if stdout != want || stderr != "" || !equalInts(exits, []int{0}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want %q, nothing, [0]", stdout, stderr, exits, want)
	}
	if got := listNames(t, w.results("own-rdd")); strings.Join(got, ",") != strings.Join(before, ",") {
		t.Errorf("results = %q after the check, want %q: it stores nothing", got, before)
	}
}
