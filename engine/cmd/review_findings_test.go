package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// findingsListing is the JSON object `review findings` prints.
type findingsListing struct {
	Review  string `json:"review"`
	Verdict string `json:"verdict"`
	Kept    []struct {
		ID          string   `json:"id"`
		Location    string   `json:"location"`
		Severity    string   `json:"severity"`
		Claim       string   `json:"claim"`
		Class       string   `json:"evidence_class"`
		Disposition string   `json:"causal_disposition"`
		ProofRefs   []string `json:"proof_refs"`
	} `json:"kept"`
	LeftOut []struct {
		ID       string `json:"id"`
		Severity string `json:"severity"`
		Reason   string `json:"reason"`
	} `json:"left_out"`
}

// withBlockerAndOldWarning is withBlocker plus F2, a WARNING that the diff did not cause.
func withBlockerAndOldWarning(o map[string]any) {
	withBlocker(o)
	o["findings"] = append(o["findings"].([]any), map[string]any{
		"id": "F2", "location": "dir/b.txt:1", "severity": "WARNING", "claim": "b was already odd",
		"evidence_class": "inferential", "causal_disposition": "pre-existing", "proof_refs": []any{},
	})
}

func (w *reviewWorld) recordWith(edit func(map[string]any)) string {
	w.t.Helper()
	b := w.bind()
	_, stderr, exits := w.run(reply(w.t, b, edit), "record", "--feature", "own-rdd", "--base", w.base, "--head", w.head, "--rows", "default")
	if !equalInts(exits, []int{0}) {
		w.t.Fatalf("setup: review record exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	return storedName(w.t, stderr)
}

// `review findings` prints the kept and the left-out findings of a stored result as one JSON object under the
// identifier <feature>/<head12>-<n>, the same bytes on every run, and stores nothing.
func TestReviewFindingsListsKeptAndLeftOutFindingsDeterministically(t *testing.T) {
	w := newReviewWorld(t)
	name := w.recordWith(withBlockerAndOldWarning)
	before := listNames(t, w.results("own-rdd"))

	stdout, stderr, exits := w.run("", "findings", "--feature", "own-rdd", "--stored", name)
	again, _, _ := w.run("", "findings", "--feature", "own-rdd", "--stored", name)

	var got findingsListing
	if err := json.Unmarshal([]byte(stdout), &got); err != nil || !equalInts(exits, []int{0}) || stderr != "" {
		t.Fatalf("exit calls %v, stderr %q, decode error %v; want [0], nothing and one JSON object:\n%s", exits, stderr, err, stdout)
	}
	if want := "own-rdd/" + strings.TrimSuffix(name, ".json"); got.Review != want || got.Verdict != "" {
		t.Errorf("review = %q, verdict = %q; want %q and no verdict", got.Review, got.Verdict, want)
	}
	if len(got.Kept) != 1 || got.Kept[0].ID != "F1" || got.Kept[0].Severity != "BLOCKER" || got.Kept[0].Location != "a.txt:2" || got.Kept[0].Disposition != "introduced" || len(got.Kept[0].ProofRefs) != 1 {
		t.Errorf("kept = %+v, want F1 with the fields of the reply", got.Kept)
	}
	if len(got.LeftOut) != 1 || got.LeftOut[0].ID != "F2" || got.LeftOut[0].Reason != "causal_disposition pre-existing" || got.LeftOut[0].Severity != "WARNING" {
		t.Errorf("left_out = %+v, want F2 with its reason", got.LeftOut)
	}
	if again != stdout {
		t.Errorf("a second run printed other bytes:\n%s\nvs\n%s", stdout, again)
	}
	if after := listNames(t, w.results("own-rdd")); strings.Join(after, ",") != strings.Join(before, ",") {
		t.Errorf("the results directory changed: %v then %v", before, after)
	}
}

// The verdict stored beside the result is read: a severe finding it did not reproduce is left out, and the stored
// verdict is named.
func TestReviewFindingsReadsTheVerdictStoredBesideTheResult(t *testing.T) {
	w := newReviewWorld(t)
	_, name := w.recordBlocker(w.base, w.head)
	if _, stderr, exits := w.run(verdictOn("not-reproduced"), "verify", "--feature", "own-rdd", "--stored", name); !equalInts(exits, []int{0}) {
		t.Fatalf("setup: review verify exit calls = %v, stderr = %q", exits, stderr)
	}

	stdout, stderr, exits := w.run("", "findings", "--feature", "own-rdd", "--stored", name)

	var got findingsListing
	if err := json.Unmarshal([]byte(stdout), &got); err != nil || !equalInts(exits, []int{0}) || stderr != "" {
		t.Fatalf("exit calls %v, stderr %q, decode error %v", exits, stderr, err)
	}
	if !strings.HasSuffix(got.Verdict, ".verifier.json") || len(got.Kept) != 0 || len(got.LeftOut) != 1 || got.LeftOut[0].Reason != "verdict not-reproduced" {
		t.Errorf("listing = %+v, want the verdict named, nothing kept and F1 left out as not reproduced", got)
	}
	if !strings.Contains(stdout, `"kept": []`) {
		t.Errorf("stdout = %s, want an empty list written as [] and not null", stdout)
	}
}

// A result with no finding prints both lists as [] and never as null (F1 of the T1a review).
func TestReviewFindingsPrintsEmptyListsForAResultWithNoFinding(t *testing.T) {
	w := newReviewWorld(t)
	name := w.recordWith(func(o map[string]any) { o["findings"] = []any{} })

	stdout, stderr, exits := w.run("", "findings", "--feature", "own-rdd", "--stored", name)

	if !equalInts(exits, []int{0}) || stderr != "" {
		t.Fatalf("exit calls %v, stderr %q; want [0] and nothing", exits, stderr)
	}
	if !strings.Contains(stdout, `"kept": []`) || !strings.Contains(stdout, `"left_out": []`) || strings.Contains(stdout, "null") {
		t.Errorf("stdout = %s, want kept and left_out as [] and no null", stdout)
	}
}

// A result that cannot be read is refused with exit 1 and a line on stderr, and nothing on stdout: never an empty list.
func TestReviewFindingsRefusesAResultItCannotRead(t *testing.T) {
	w := newReviewWorld(t)
	w.plant("222222222222-1.json", []byte("not a result"))

	for _, tc := range []struct{ name, stored, want string }{
		{"a file that is not a result", "222222222222-1.json", "error: review findings: "},
		{"a result that is not stored", "333333333333-1.json", "error: review findings: "},
		{"a name that is not a result name", "222222222222-1.verifier.json", "error: review findings: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exits := w.run("", "findings", "--feature", "own-rdd", "--stored", tc.stored)

			if !equalInts(exits, []int{1}) || stdout != "" || !strings.HasPrefix(stderr, tc.want) || !strings.Contains(stderr, tc.stored[:15]) {
				t.Errorf("exit calls %v, stdout %q, stderr %q; want [1], nothing and a line that starts %q and names the file", exits, stdout, stderr, tc.want)
			}
		})
	}
}

// A command line the verb cannot read is a usage error: exit 1, one line, before git is asked anything.
func TestReviewFindingsRefusesACommandLineItCannotRead(t *testing.T) {
	w := newReviewWorld(t)
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no feature", []string{"findings", "--stored", "s.json"}, "error: review findings: --feature is required\n"},
		{"no stored result", []string{"findings", "--feature", "f"}, "error: review findings: --stored is required\n"},
		{"a flag of verify", []string{"findings", "--feature", "f", "--stored", "s.json", "--verdicts", "v"}, "error: review findings: unknown flag \"--verdicts\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exits := w.run("", tc.args...)

			if !equalInts(exits, []int{1}) || stdout != "" || stderr != tc.want {
				t.Errorf("exit calls %v, stdout %q, stderr %q; want [1], nothing and %q", exits, stdout, stderr, tc.want)
			}
		})
	}
}

func TestUsageDocumentsReviewFindings(t *testing.T) {
	var out bytes.Buffer
	usage(&out)
	text := out.String()

	for _, want := range []string{
		"engine review findings --feature <name> --stored <result> [--cwd <repo>]",
		"<feature>/<head12>-<n>", "causal_disposition is pre-existing or base-only", "not-reproduced",
		"never an empty list", "exit 0 listed, 1 usage error, a result that cannot be read or any other failure",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("usage lacks %q", want)
		}
	}
}
