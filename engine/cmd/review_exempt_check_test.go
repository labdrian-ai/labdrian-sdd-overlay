package main

// What `review check` makes of an exemption (own-rdd T4, S5, L16): a head that a stored exemption binds is cleared and
// the file used is named, a result that does not clear the head wins over any exemption, and an exemption that no
// longer holds is held to its rules and to git again. The exemptions are made by `review exempt`, whose own tests are
// in review_exempt_test.go; the rules of the clearance are tested in review and review/app.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// A head that a stored exemption binds is cleared: exit 0, and stdout names the head, the exemption used and what the
// candidate was recomputed to be. results is an empty array, not null: the head has none. Nothing is stored.
func TestCheckClearsTheHeadOfAStoredExemptionAndPrintsTheFileItUsed(t *testing.T) {
	w := newReviewWorld(t)
	b := w.bind()
	if _, stderr, exits := w.exempt(verifierReport, "none"); !equalInts(exits, []int{0}) {
		t.Fatalf("setup: review exempt exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	before := listNames(t, w.results("own-rdd"))

	stdout, stderr, exits := w.run("", "check", "--feature", "own-rdd")

	want := fmt.Sprintf(`{
  "head": %q,
  "outcome": "clear",
  "results": [],
  "exemptions": [
    {
      "exemption": "odd/review-results/own-rdd/%s-1.exempt.json",
      "base": %q,
      "diff_sha256": %q
    }
  ]
}
`, w.head, w.head[:12], w.base, b.DiffSHA256)
	if stdout != want || stderr != "" || !equalInts(exits, []int{0}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want %q, nothing, [0]", stdout, stderr, exits, want)
	}
	if got := listNames(t, w.results("own-rdd")); !reflect.DeepEqual(got, before) {
		t.Errorf("results = %q after the check, want %q: it stores nothing", got, before)
	}
}

// A blocking review wins over any exemption, and a result that does not block and is not clear either keeps the head
// from delivery as well: the refusal is the one the results alone give, and the exemption does not appear in it.
func TestCheckLetsAStoredResultThatDoesNotClearTheHeadWinOverAnExemption(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(w *reviewWorld)
		want  []string
	}{
		{"the result blocks", func(w *reviewWorld) {
			w.recordBlocker(w.base, w.head)
			stored := w.head[:12] + "-2.json" // the exemption took number 1 of the head, and the result is the next
			if _, stderr, exits := w.run(verdictOn("reproduced"), "verify", "--feature", "own-rdd", "--stored", stored); !equalInts(exits, []int{0}) {
				w.t.Fatalf("setup: review verify exit calls = %v, stderr = %q; want [0]", exits, stderr)
			}
		}, []string{"a stored result blocks the head", "blocking findings F1"}},
		{"a severe finding nobody checked", func(w *reviewWorld) { w.recordBlocker(w.base, w.head) },
			[]string{"a stored result is not clear", "findings not checked F1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newReviewWorld(t)
			if _, stderr, exits := w.exempt(verifierReport, "none"); !equalInts(exits, []int{0}) {
				t.Fatalf("setup: review exempt exit calls = %v, stderr = %q; want [0]", exits, stderr)
			}
			tc.setup(w)

			stdout, stderr, exits := w.run("", "check", "--feature", "own-rdd")

			if stdout != "" || !equalInts(exits, []int{1}) || !strings.HasPrefix(stderr, "error: review check: ") || strings.Count(stderr, "\n") != 1 {
				t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, one line of error: review check: ..., [1]", stdout, stderr, exits)
			}
			for _, want := range tc.want {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr = %q, want it to say %q", stderr, want)
				}
			}
		})
	}
}

// An exemption that no longer holds never clears the head: the file is held to its rules and to git again. One edited
// to name a high-risk item, to say a check passed that failed, or that is about a diff the head no longer has, is
// exit 1 with the file and the reason on stderr, and nothing is stored. So is a head that was never exempted.
func TestCheckRefusesAnExemptionThatNoLongerHoldsAndAHeadThatWasNeverExempted(t *testing.T) {
	zeros := strings.Repeat("0", 64)
	edit := func(w *reviewWorld, from, to string) {
		w.t.Helper()
		path := filepath.Join(w.results("own-rdd"), w.head[:12]+"-1.exempt.json")
		data, err := os.ReadFile(path)
		if err != nil || !bytes.Contains(data, []byte(from)) {
			w.t.Fatalf("setup: %s does not hold %q (%v)", path, from, err)
		}
		if err := os.WriteFile(path, bytes.Replace(data, []byte(from), []byte(to), 1), 0o644); err != nil {
			w.t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name  string
		setup func(w *reviewWorld)
		args  []string // the command line after "check"; the feature own-rdd when nil
		say   func(w *reviewWorld) []string
	}{
		{"the head was never exempted", func(*reviewWorld) {}, nil, func(w *reviewWorld) []string {
			return []string{"head " + w.head + ": review: no stored result binds the head"}
		}},
		{"another head was exempted", func(w *reviewWorld) { w.exempt(verifierReport, "none") }, []string{"--feature", "own-rdd", "--head", "HEAD~1"}, func(w *reviewWorld) []string {
			return []string{"head " + w.base + ": review: no stored result binds the head"}
		}},
		{"the risk was edited to name an item", func(w *reviewWorld) {
			w.exempt(verifierReport, "none")
			edit(w, `"risk": "none"`, `"risk": "2"`)
		}, nil, func(w *reviewWorld) []string {
			return []string{"odd/review-results/own-rdd/" + w.head[:12] + "-1.exempt.json", "never exempt from review", "2 (security)"}
		}},
		{"the report was edited so that a check failed", func(w *reviewWorld) {
			w.exempt(verifierReport, "none")
			edit(w, `"result": "pass"`, `"result": "fail"`)
		}, nil, func(w *reviewWorld) []string {
			return []string{"odd/review-results/own-rdd/" + w.head[:12] + "-1.exempt.json", "the report is not clear", "is fail"}
		}},
		{"the diff is not the one the exemption was made on", func(w *reviewWorld) {
			w.exempt(verifierReport, "none")
			edit(w, w.bind().DiffSHA256, zeros)
		}, nil, func(w *reviewWorld) []string {
			return []string{"odd/review-results/own-rdd/" + w.head[:12] + "-1.exempt.json", "the stored binding differs from the candidate git gives", "diff_sha256 is " + zeros}
		}},
		{"the stored file is no longer an exemption", func(w *reviewWorld) {
			w.exempt(verifierReport, "none")
			path := filepath.Join(w.results("own-rdd"), w.head[:12]+"-1.exempt.json")
			if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
				w.t.Fatal(err)
			}
		}, nil, func(w *reviewWorld) []string {
			return []string{"odd/review-results/own-rdd/" + w.head[:12] + "-1.exempt.json", "invalid stored file"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newReviewWorld(t)
			tc.setup(w)
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
			if got := listNames(t, w.results("own-rdd")); !reflect.DeepEqual(got, before) {
				t.Errorf("results = %q after a refused check, want %q", got, before)
			}
		})
	}
}

// The help of the delivery check says what an exemption changes in it, and the wrapper says the same.
func TestUsageDocumentsWhatReviewCheckMakesOfAnExemption(t *testing.T) {
	wrapper, err := os.ReadFile("../../bin/lib/usage.sh")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	usage(&out)
	engineText, wrapperText := strings.Join(strings.Fields(out.String()), " "), strings.Join(strings.Fields(string(wrapper)), " ")
	for _, want := range []string{
		"decides whether the head being delivered (--head, default HEAD) was reviewed, or exempted from review",
		"clears it when at least one result binds the head and every one is clear, or when an exemption binds it and no result does",
		"no exemption clears a head that has such a result",
		"stdout is one JSON object: head, outcome clear, the results used and the exemptions used",
		"(exemptions is left out when there is none)",
	} {
		if !strings.Contains(engineText, want) {
			t.Errorf("the engine usage of review check lacks %q", want)
		}
	}
	if want := "an exemption that still holds clears a head that has no result"; !strings.Contains(wrapperText, want) {
		t.Errorf("bin/lib/usage.sh lacks %q, which the engine usage says as the clearance by an exemption", want)
	}
}

// The built program: a head is refused until it is exempted, cleared by the exemption, and refused again once the
// exemption is edited to name a high-risk item. The streams and the exit codes are the ones the delivery role reads.
func TestReviewExemptOfTheBuiltProgramClearsAnExemptedHeadAndRefusesAnEditedOne(t *testing.T) {
	w := newReviewWorld(t)
	bin := engineBinary(t)
	run := func(stdin string, args ...string) (stdout, stderr string, code int) {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Dir = w.root
		cmd.Env = goldenEnvironment()
		cmd.Stdin = strings.NewReader(stdin)
		var o, e bytes.Buffer
		cmd.Stdout, cmd.Stderr = &o, &e
		err := cmd.Run()
		code = 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatalf("run %v: %v", args, err)
		}
		return o.String(), e.String(), code
	}

	if out, errText, code := run("", "review", "check", "--feature", "own-rdd"); code != 1 || out != "" || !strings.Contains(errText, "no stored result binds the head") {
		t.Fatalf("check of a head never exempted: exit %d, stdout %q, stderr %q; want 1, nothing and the reason", code, out, errText)
	}
	if out, errText, code := run(verifierReport, "review", "exempt", "--feature", "own-rdd", "--base", "HEAD~1", "--head", "HEAD", "--risk", "5"); code != 1 || out != "" || !strings.Contains(errText, "5 (delivery or environment)") {
		t.Fatalf("exempt of a head with a declared risk: exit %d, stdout %q, stderr %q; want 1, nothing and the item", code, out, errText)
	}
	if out, errText, code := run(`{"checks":[]}`, "review", "exempt", "--feature", "own-rdd", "--base", "HEAD~1", "--head", "HEAD", "--risk", "none"); code != 2 || out != "" || !strings.Contains(errText, "the report is not clear") {
		t.Fatalf("exempt with a report that names no check: exit %d, stdout %q, stderr %q; want 2, nothing and the rule", code, out, errText)
	}
	if _, err := os.Stat(w.results("own-rdd")); err == nil {
		t.Fatal("the refusals stored something")
	}
	out, errText, code := run(verifierReport, "review", "exempt", "--feature", "own-rdd", "--base", "HEAD~1", "--head", "HEAD", "--risk", "none")
	if code != 0 || !strings.Contains(out, `"payload_kind": "review-exemption"`) || !strings.Contains(errText, w.head[:12]+"-1.exempt.json") {
		t.Fatalf("exempt: exit %d, stdout %q, stderr %q; want 0, the skeleton and the file stored", code, out, errText)
	}

	out, errText, code = run("", "review", "check", "--feature", "own-rdd")
	var cleared struct {
		Head       string `json:"head"`
		Outcome    string `json:"outcome"`
		Results    []any  `json:"results"`
		Exemptions []struct {
			Exemption string `json:"exemption"`
			Base      string `json:"base"`
		} `json:"exemptions"`
	}
	if err := json.Unmarshal([]byte(out), &cleared); err != nil || code != 0 || errText != "" {
		t.Fatalf("check of an exempted head: exit %d, stderr %q, stdout %q (%v); want 0, nothing on stderr and a JSON object", code, errText, out, err)
	}
	if cleared.Head != w.head || cleared.Outcome != "clear" || cleared.Results == nil || len(cleared.Results) != 0 || len(cleared.Exemptions) != 1 ||
		cleared.Exemptions[0].Exemption != "odd/review-results/own-rdd/"+w.head[:12]+"-1.exempt.json" || cleared.Exemptions[0].Base != w.base {
		t.Errorf("check = %+v, want the head %s cleared by exemption 1 of the base %s and no result", cleared, w.head, w.base)
	}

	path := filepath.Join(w.results("own-rdd"), w.head[:12]+"-1.exempt.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes.Replace(data, []byte(`"risk": "none"`), []byte(`"risk": "1,3"`), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, errText, code := run("", "review", "check", "--feature", "own-rdd"); code != 1 || out != "" || !strings.Contains(errText, "1 (data or irreversible effects), 3 (contracts others consume)") {
		t.Errorf("check with an edited exemption: exit %d, stdout %q, stderr %q; want 1, nothing and the items", code, out, errText)
	}
}
