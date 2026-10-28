package main

// The command line of `review exempt` (own-rdd T4, S5, L16). The tests run the verb over a repository they make in a
// temporary directory, as the tests of the other review verbs do. The rules themselves (the risk grammar, the report,
// the stored file) are tested in review, review/app and reviewverdict, and what `review check` makes of an exemption
// is tested in review_exempt_check_test.go.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

// verifierReport is the report of a verifier that ran one check on the head and found it passing.
const verifierReport = `{"checks":[{"command":"go test ./...","result":"pass","output":"ok  engine 0.4s"}]}`

// exemptArgs is the command line of `review exempt` for the commits of the world, with the risk given.
func (w *reviewWorld) exemptArgs(risk string) []string {
	return []string{"exempt", "--feature", "own-rdd", "--base", w.base, "--head", w.head, "--risk", risk}
}

// exempt exempts the head of the world with the report on stdin and returns what the verb wrote. `review exempt` reads
// the trailer of the head (own-rdd T6b5, S6 f), so a world that exempts is made with newCheckedWorld.
func (w *reviewWorld) exempt(report, risk string) (stdout, stderr string, exits []int) {
	w.t.Helper()
	return w.run(report, w.exemptArgs(risk)...)
}

// A command line the verb cannot read is a usage error: exit 1 with one line on standard error and nothing made,
// before git is asked anything. The report is read from stdin only, and the risk is a flag of this verb alone.
func TestReviewExemptRefusesACommandLineItCannotRead(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no feature", []string{"exempt", "--base", "b", "--head", "h", "--risk", "none"}, "error: review exempt: --feature is required\n"},
		{"no base", []string{"exempt", "--feature", "f", "--head", "h", "--risk", "none"}, "error: review exempt: --base is required\n"},
		{"no head", []string{"exempt", "--feature", "f", "--base", "b", "--risk", "none"}, "error: review exempt: --head is required\n"},
		{"no risk", []string{"exempt", "--feature", "f", "--base", "b", "--head", "h"}, "error: review exempt: --risk is required\n"},
		{"an empty risk", []string{"exempt", "--feature", "f", "--base", "b", "--head", "h", "--risk", ""}, "error: review exempt: --risk is required\n"},
		{"a risk with no value", []string{"exempt", "--feature", "f", "--base", "b", "--head", "h", "--risk"}, "error: review exempt: --risk requires a value\n"},
		{"the risk twice", []string{"exempt", "--feature", "f", "--base", "b", "--head", "h", "--risk", "none", "--risk", "none"}, "error: review exempt: --risk given twice\n"},
		{"the report given as a flag", []string{"exempt", "--feature", "f", "--base", "b", "--head", "h", "--risk", "none", "--report", "r.json"}, "error: review exempt: unknown flag \"--report\"\n"},
		{"the result given to exempt", []string{"exempt", "--feature", "f", "--base", "b", "--head", "h", "--risk", "none", "--result", "r.json"}, "error: review exempt: unknown flag \"--result\"\n"},
		{"the stored result given to exempt", []string{"exempt", "--feature", "f", "--base", "b", "--head", "h", "--risk", "none", "--stored", "s.json"}, "error: review exempt: unknown flag \"--stored\"\n"},
		{"a positional argument", []string{"exempt", "--feature", "f", "--base", "b", "--head", "h", "--risk", "none", "extra"}, "error: review exempt: unexpected argument \"extra\"\n"},
		{"the working directory twice", []string{"exempt", "--feature", "f", "--base", "b", "--head", "h", "--risk", "none", "--cwd", "a", "--cwd", "b"}, "error: review exempt: --cwd given twice\n"},
		{"the risk given to bind", []string{"bind", "--base", "b", "--head", "h", "--risk", "none"}, "error: review bind: unknown flag \"--risk\"\n"},
		{"the risk given to record", []string{"record", "--feature", "f", "--base", "b", "--head", "h", "--risk", "none"}, "error: review record: unknown flag \"--risk\"\n"},
		{"the risk given to verify", []string{"verify", "--feature", "f", "--stored", "s.json", "--risk", "none"}, "error: review verify: unknown flag \"--risk\"\n"},
		{"the risk given to check", []string{"check", "--feature", "f", "--risk", "none"}, "error: review check: unknown flag \"--risk\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newCheckedWorld(t)

			stdout, stderr, exits := w.run(verifierReport, tc.args...)

			if stdout != "" || stderr != tc.want || !equalInts(exits, []int{1}) {
				t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q, [1]", stdout, stderr, exits, tc.want)
			}
			if _, err := os.Stat(w.results("f")); err == nil {
				t.Error("the results directory was made for a command line that was refused")
			}
		})
	}
}

// An exemption is for a commit that declares no risk (own-rdd T6b5, S6 f): a head whose trailer fails or names an item is
// refused before anything is stored, as plain errors with exit 1, as a --risk that names an item is. A report that is not
// clear is still refused first, with exit 2.
func TestExemptRefusesAHeadWhoseTrailerFailsOrNamesAnItem(t *testing.T) {
	for _, tc := range []struct {
		name  string
		world func(t *testing.T) *reviewWorld
		want  func(w *reviewWorld) string
	}{
		{"no trailer", func(t *testing.T) *reviewWorld { return newReviewWorld(t) }, func(w *reviewWorld) string {
			return "error: review exempt: review: the commit does not declare its risk in a Risk trailer: commit " + w.head +
				": it has no \"Risk:\" trailer in the final trailer block of its message\n"
		}},
		{"a trailer that names items", func(t *testing.T) *reviewWorld {
			w := newCheckedWorld(t)
			w.declaresRisk("3,5")
			return w
		}, func(w *reviewWorld) string {
			return "error: review exempt: commit " + w.head + ": review: a change that names a high-risk item is never exempt from review: " +
				"the declared risk names 3 (contracts others consume), 5 (delivery or environment)\n"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := tc.world(t)

			stdout, stderr, exits := w.run(verifierReport, w.exemptArgs("none")...) // not w.exempt, which would declare the risk

			if want := tc.want(w); stdout != "" || stderr != want || !equalInts(exits, []int{1}) {
				t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q, [1]", stdout, stderr, exits, want)
			}
			if _, err := os.Stat(w.results("own-rdd")); err == nil {
				t.Error("the results directory was made for an exemption that was refused")
			}
			if _, stderr, exits := w.run(`{"checks":[]}`, w.exemptArgs("none")...); !equalInts(exits, []int{2}) || !strings.Contains(stderr, "the report is not clear") {
				t.Errorf("a report that is not clear: stderr = %q, exits = %v; want it refused first, as before, with exit 2", stderr, exits)
			}
		})
	}
}

// The verb stores the exemption in the next free number of the head and prints the skeleton of the record of the
// chain: the stored file has the binding git gives, the declared risk and the report of the verifier, stdout is the
// record of the reviewer to the delivery with the exemption bound as evidence, and stderr says what was stored.
func TestExemptStoresTheExemptionAndPrintsTheSkeletonOfTheRecord(t *testing.T) {
	w := newCheckedWorld(t)
	b := w.bind()

	stdout, stderr, exits := w.exempt(verifierReport, "none")

	if !equalInts(exits, []int{0}) {
		t.Fatalf("exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	name := w.head[:12] + "-1.exempt.json"
	storedPath := filepath.Join(w.results("own-rdd"), name)
	file, err := os.ReadFile(storedPath)
	if err != nil {
		t.Fatalf("the exemption is not stored at %s: %v", storedPath, err)
	}
	if info, _ := os.Stat(storedPath); info == nil || info.Mode().Perm() != 0o644 {
		t.Errorf("the stored file has mode %v, want 0644", info)
	}
	stored, err := review.DecodeExemption(file)
	if err != nil {
		t.Fatalf("the stored file is not an exemption: %v\n%s", err, file)
	}
	if stored.Binding.Base != b.Base || stored.Binding.Head != b.Head || stored.Binding.DiffSHA256 != b.DiffSHA256 || !reflect.DeepEqual(stored.Binding.Paths, b.Paths) {
		t.Errorf("stored binding = %+v, want the binding git gives: %+v", stored.Binding, b)
	}
	if stored.Risk != "none" || review.CheckReport(stored.Report) != nil {
		t.Errorf("stored risk = %q and report = %s, want none and a clear report", stored.Risk, stored.Report)
	}
	storedRef := "odd/review-results/own-rdd/" + name
	var record struct {
		Version       int    `json:"version"`
		FromRole      string `json:"from_role"`
		ToRole        string `json:"to_role"`
		PayloadKind   string `json:"payload_kind"`
		PayloadSHA256 string `json:"payload_sha256"`
		Status        string `json:"status"`
		Evidence      []struct {
			Kind   string `json:"kind"`
			Ref    string `json:"ref"`
			SHA256 string `json:"sha256"`
		} `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(stdout), &record); err != nil {
		t.Fatalf("stdout is not the JSON skeleton: %v\n%s", err, stdout)
	}
	if record.FromRole != "reviewer" || record.ToRole != "delivery" || record.PayloadKind != "review-exemption" || record.Status != "completed" || record.PayloadSHA256 != sum(file) {
		t.Errorf("skeleton = %+v, want the reviewer's completed record to the delivery of kind review-exemption with the digest %s", record, sum(file))
	}
	if len(record.Evidence) != 2 ||
		record.Evidence[0].Kind != "review-diff" || record.Evidence[0].Ref != w.base+".."+w.head || record.Evidence[0].SHA256 != b.DiffSHA256 ||
		record.Evidence[1].Kind != "review-exemption" || record.Evidence[1].Ref != storedRef || record.Evidence[1].SHA256 != sum(file) {
		t.Errorf("evidence = %+v, want review-diff of %s..%s and review-exemption of %s", record.Evidence, w.base, w.head, storedRef)
	}
	wantErr := fmt.Sprintf("review exempt: stored %s (sha256 %s)\n"+
		"review exempt: the head is exempted from review: the declared risk is none and the report of the verifier is clear; 'review check' clears it unless a stored result blocks it\n"+
		"review exempt: stdout is the skeleton of the record for 'roles append --stdin'; the parent of the chain fills project_id, goal_id, chain_id, seq, prev_sha256, context.summary\n",
		storedRef, sum(file))
	if stderr != wantErr {
		t.Errorf("stderr = %q, want %q", stderr, wantErr)
	}
}

// Every item of the high-risk list keeps a head from being exempt, alone or in a list: exit 1, the items named, and
// nothing stored. A declaration the engine cannot read is refused too.
func TestExemptRefusesADeclaredRiskThatIsNotNoneAndStoresNothing(t *testing.T) {
	for _, tc := range []struct{ risk, want string }{
		{"1", "1 (data or irreversible effects)"}, {"2", "2 (security)"}, {"3", "3 (contracts others consume)"},
		{"4", "4 (concurrency)"}, {"5", "5 (delivery or environment)"}, {"6", "6 (no test would catch a regression)"},
		{"2,4", "2 (security), 4 (concurrency)"},
		{"None", `"None" is not "none" or a list of the numbers 1 to 6`}, {"7", `"7" is not "none" or a list of the numbers 1 to 6`},
	} {
		t.Run(tc.risk, func(t *testing.T) {
			w := newCheckedWorld(t)

			stdout, stderr, exits := w.exempt(verifierReport, tc.risk)

			if stdout != "" || !equalInts(exits, []int{1}) || !strings.HasPrefix(stderr, "error: review exempt: review: ") || strings.Count(stderr, "\n") != 1 {
				t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, one line of error: review exempt: review: ..., [1]", stdout, stderr, exits)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Errorf("stderr = %q, want it to say %q", stderr, tc.want)
			}
			if _, err := os.Stat(w.results("own-rdd")); err == nil {
				t.Error("the results directory was made for a risk that was refused")
			}
		})
	}
}

// A report that is missing, is not the object of the contract, or is not clear is exit 2 with the rule it breaks, as
// a reply that is refused is, and nothing is stored: a check that did not run never passes.
func TestExemptRefusesAReportThatIsMissingOrNotClearAndStoresNothing(t *testing.T) {
	for name, report := range map[string]string{
		"no report":                   "",
		"prose":                       "All checks passed.",
		"a check that failed":         `{"checks":[{"command":"go test ./...","result":"fail","output":"FAIL engine"}]}`,
		"a check that did not run":    `{"checks":[{"command":"go test ./...","result":"not-run","output":"no toolchain"}]}`,
		"no check":                    `{"checks":[]}`,
		"a check without output":      `{"checks":[{"command":"go test ./...","result":"pass","output":""}]}`,
		"a member nobody defined":     `{"checks":[],"outcome":"clear"}`,
		"a report that says it is ok": `{"outcome":"clear"}`,
	} {
		t.Run(name, func(t *testing.T) {
			w := newCheckedWorld(t)

			stdout, stderr, exits := w.exempt(report, "none")

			if stdout != "" || !equalInts(exits, []int{2}) || !strings.HasPrefix(stderr, "error: review exempt: the reply is refused and nothing was stored: reviewverdict: ") {
				t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, the refusal of the report by a named rule, [2]", stdout, stderr, exits)
			}
			if _, err := os.Stat(w.results("own-rdd")); err == nil {
				t.Error("the results directory was made for a report that was refused")
			}
		})
	}
}

// The numbering is the head's, shared with its results: the second exemption is number 2, a result recorded after
// an exemption is the next number, and nothing that is stored is ever replaced.
func TestExemptTakesTheNextNumberOfTheHeadAndReplacesNothing(t *testing.T) {
	w := newCheckedWorld(t)
	w.exempt(verifierReport, "none")
	first, err := os.ReadFile(filepath.Join(w.results("own-rdd"), w.head[:12]+"-1.exempt.json"))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, stderr, exits := w.exempt(verifierReport, "none")
	b := w.bind()
	_, recordErr, recordExits := w.run(reply(t, b, nil), "record", "--feature", "own-rdd", "--base", w.base, "--head", w.head, "--rows", "default")

	if !equalInts(exits, []int{0}) || !strings.Contains(stderr, "odd/review-results/own-rdd/"+w.head[:12]+"-2.exempt.json") {
		t.Errorf("the second exemption: exits = %v, stderr = %q; want [0] and number 2", exits, stderr)
	}
	if !equalInts(recordExits, []int{0}) || !strings.Contains(recordErr, "odd/review-results/own-rdd/"+w.head[:12]+"-3.json") {
		t.Errorf("the result after them: exits = %v, stderr = %q; want [0] and number 3 of its own kind", recordExits, recordErr)
	}
	want := []string{w.head[:12] + "-1.exempt.json", w.head[:12] + "-2.exempt.json", w.head[:12] + "-3.json"}
	if got := listNames(t, w.results("own-rdd")); !reflect.DeepEqual(got, want) {
		t.Errorf("results = %q, want %q", got, want)
	}
	if again, _ := os.ReadFile(filepath.Join(w.results("own-rdd"), w.head[:12]+"-1.exempt.json")); !bytes.Equal(again, first) {
		t.Error("the first exemption was changed")
	}
}

// Standard output is where the verb hands over the skeleton of the record. When it cannot be written the parent never
// received it, and exit 0 would tell it that it did. The exemption stays stored, and the message says where.
func TestExemptFailsWithExit1WhenTheSkeletonCannotBeWritten(t *testing.T) {
	w := newCheckedWorld(t)
	p := newCapturedProcess()
	p.stdin = strings.NewReader(verifierReport)
	p.stdout = brokenStdout{errors.New("broken pipe")}

	w.runOn(p, w.exemptArgs("none")...)

	stored := "odd/review-results/own-rdd/" + w.head[:12] + "-1.exempt.json"
	want := "error: review exempt: the exemption is stored at " + stored + ", but the handoff could not be written: write to stdout: broken pipe\n"
	if !equalInts(p.exits, []int{1}) || !strings.HasSuffix(p.err.String(), want) || p.out.Len() != 0 {
		t.Errorf("exits = %v, stderr = %q, stdout = %q; want [1], a message that ends in %q and nothing on stdout", p.exits, p.err.String(), p.out.String(), want)
	}
	if _, err := os.Stat(filepath.Join(w.results("own-rdd"), w.head[:12]+"-1.exempt.json")); err != nil {
		t.Errorf("the exemption the message names is not there: %v", err)
	}
}

// A revision that is not a commit, and one git would read as an option, are refused before the exemption is stored:
// the option writes no file.
func TestExemptRefusesWhatIsNotACommitAndWhatGitWouldReadAsAnOption(t *testing.T) {
	w := newCheckedWorld(t)
	marker := filepath.Join(w.parent, "written-by-an-option")
	for _, tc := range []struct{ name, base, want string }{
		{"a name that is nothing", "nonesuch", `resolve base "nonesuch"`},
		{"an option", "--output=" + marker, "starts with '-' and would be read as an option"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exits := w.run(verifierReport, "exempt", "--feature", "own-rdd", "--base", tc.base, "--head", w.head, "--risk", "none")

			if stdout != "" || !equalInts(exits, []int{1}) || !strings.Contains(stderr, tc.want) {
				t.Errorf("stdout = %q, exits = %v, stderr = %q; want nothing, [1] and %q", stdout, exits, stderr, tc.want)
			}
			if _, err := os.Stat(marker); err == nil {
				t.Error("a file was written by a revision that is an option")
			}
			if _, err := os.Stat(w.results("own-rdd")); err == nil {
				t.Error("the results directory was made for a candidate that could not be bound")
			}
		})
	}
}

// The sentence that tells the parent of the chain what it still fills is shared by the verbs that hand over a skeleton
// (parentFills). record leaves the next role to the parent, and verify names it, as exempt does, so it is not among
// the members the parent fills. The sentence of the verbs that existed before is what it always was.
func TestRecordAndVerifySayWhatTheParentOfTheChainStillFillsAsBefore(t *testing.T) {
	const (
		sentence = "stdout is the skeleton of the record for 'roles append --stdin'; the parent of the chain fills "
		withRole = "project_id, goal_id, chain_id, seq, prev_sha256, to_role, context.summary\n"
		noRole   = "project_id, goal_id, chain_id, seq, prev_sha256, context.summary\n"
	)
	w := newCheckedWorld(t)
	b := w.bind()

	_, recorded, recordExits := w.run(reply(t, b, withBlocker), "record", "--feature", "own-rdd", "--base", w.base, "--head", w.head, "--rows", "default")
	_, verified, verifyExits := w.run(verdictOn("reproduced"), "verify", "--feature", "own-rdd", "--stored", w.head[:12]+"-1.json")

	if !equalInts(recordExits, []int{0}) || !strings.HasSuffix(recorded, "review record: "+sentence+withRole) {
		t.Errorf("record: exits = %v, stderr = %q; want [0] and a last line that says %q", recordExits, recorded, sentence+withRole)
	}
	if !equalInts(verifyExits, []int{0}) || !strings.HasSuffix(verified, "review verify: "+sentence+noRole) {
		t.Errorf("verify: exits = %v, stderr = %q; want [0] and a last line that says %q", verifyExits, verified, sentence+noRole)
	}
}

// The words a caller reads when it gives the wrong verb name every verb, the new one included.
func TestReviewNamesExemptAmongItsVerbs(t *testing.T) {
	w := newCheckedWorld(t)

	_, noVerb, _ := w.run("")
	_, unknown, _ := w.run("", "bogus")

	if want := "error: review requires a verb: bind, record, verify, check, exempt, sweep\n"; noVerb != want {
		t.Errorf("review with no verb says %q, want %q", noVerb, want)
	}
	if want := "error: review: unknown verb \"bogus\" (expected bind, record, verify, check, exempt or sweep)\n"; unknown != want {
		t.Errorf("review with an unknown verb says %q, want %q", unknown, want)
	}
}

// The verb is in the help of the engine, with what a caller acts on, and the wrapper says the same.
func TestUsageDocumentsReviewExempt(t *testing.T) {
	wrapper, err := os.ReadFile("../../bin/lib/usage.sh")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	usage(&out)
	engineText, wrapperText := strings.Join(strings.Fields(out.String()), " "), strings.Join(strings.Fields(string(wrapper)), " ")
	if want := "The head's own Risk trailer must be 'Risk: none'"; !strings.Contains(engineText, want) {
		t.Errorf("the engine usage lacks %q", want)
	}
	for _, pair := range []struct{ engine, inWrapper string }{
		{"engine review exempt --feature <name> --base <rev> --head <rev> --risk <none|items> [--cwd <repo>]", "exempt --feature NAME --base REV --head REV --risk none|ITEMS [--cwd REPO]"},
		{"reads the verifier's report from stdin", "reads the verifier's report from stdin"},
		{"{\"checks\": [{\"command\": ..., \"result\": pass, fail or not-run, \"output\": ...}]}", "{\"checks\": [{\"command\": ..., \"result\": pass, fail or not-run, \"output\": ...}]}"},
		{"--risk is none or the numbers 1 to 6 of the high-risk items separated by commas", "--risk is none or the numbers 1 to 6 of the high-risk items"},
		{"a head that names any item is never exempt", "a head that names any item is never exempt"},
		{"is for one commit: --base must be the first parent of --head", "is for one commit: --base must be the first parent of --head"},
		{"and its base to the first parent of the head (it is for one commit)", "an exemption whose base is not the first parent of the head refuses"},
		{"a range whose base is its head covers no commit and is refused", "a range whose base is its head covers no commit and is refused"},
		{"<head12>-<n>.exempt.json", "HEAD12-N.exempt.json"},
		{"never over an existing file", "never over a file"},
		{"payload_kind review-exemption", "payload_kind review-exemption"},
		{"a result that blocks wins over any exemption", "a result that blocks wins over any exemption"},
		{"exit 0 stored, 2 report refused, 1 usage error, a risk that is refused or any other failure", "exit 0 stored, 2 report refused, 1 error"},
	} {
		if !strings.Contains(engineText, pair.engine) {
			t.Errorf("the engine usage lacks %q", pair.engine)
		}
		if !strings.Contains(wrapperText, pair.inWrapper) {
			t.Errorf("bin/lib/usage.sh lacks %q, which the engine usage says as %q", pair.inWrapper, pair.engine)
		}
	}
}
