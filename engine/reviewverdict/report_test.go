package reviewverdict_test

// The report that exempts a head from review (own-rdd T4, S5). The verifier is launched with no finding to answer:
// the prompt names the checks, and the verifier returns the checks it ran, each with its command, whether it
// passed, and what it printed. ParseReport holds the reply to that shape and Clear says whether it is clear.

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewverdict"
)

// checkOf is one check of a report, written as the verifier writes it.
func checkOf(command, result, output string) string {
	return fmt.Sprintf(`{"command":%q,"result":%q,"output":%q}`, command, result, output)
}

func reportOf(checks ...string) string { return `{"checks":[` + strings.Join(checks, ",") + `]}` }

// cleanReport is a report of two checks that passed.
func cleanReport() string {
	return reportOf(checkOf("gofmt -l .", "pass", "(no output)"), checkOf("go test ./review/...", "pass", "ok  review 0.009s"))
}

// reportErrors are the reasons ParseReport refuses a reply. A refusal wraps exactly one of them.
var reportErrors = []error{
	reviewverdict.ErrUnparseable, reviewverdict.ErrUnknownMember, reviewverdict.ErrOutsideEnum, reviewverdict.ErrMissingEvidence,
}

// refusedReport requires ParseReport to refuse data for exactly the reason want, with an empty report, and returns
// the message.
func refusedReport(t *testing.T, data string, want error) string {
	t.Helper()
	got, err := reviewverdict.ParseReport([]byte(data))
	if !reflect.DeepEqual(got, reviewverdict.Report{}) {
		t.Errorf("ParseReport(%q) = %+v, want the empty report from a refused reply", data, got)
	}
	if err == nil {
		t.Fatalf("ParseReport(%q) error = nil, want one that is %v", data, want)
	}
	for _, reason := range reportErrors {
		if errors.Is(err, reason) != (reason == want) {
			t.Errorf("ParseReport(%q) error = %v: errors.Is(%v) = %v, want a refusal for %v and no other", data, err, reason, !(reason == want), want)
		}
	}
	return err.Error()
}

func TestParseReportReturnsTheChecksInTheOrderTheVerifierRanThem(t *testing.T) {
	data := reportOf(
		checkOf("go vet ./...", "fail", "vet: undefined: x"),
		checkOf("go test ./review/...", "not-run", "no go toolchain in the worktree"),
		checkOf("gofmt -l .", "pass", "(no output)"))

	got, err := reviewverdict.ParseReport([]byte(data))

	want := reviewverdict.Report{Checks: []reviewverdict.Check{
		{Command: "go vet ./...", Result: reviewverdict.CheckFailed, Output: "vet: undefined: x"},
		{Command: "go test ./review/...", Result: reviewverdict.CheckNotRun, Output: "no go toolchain in the worktree"},
		{Command: "gofmt -l .", Result: reviewverdict.CheckPassed, Output: "(no output)"},
	}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("ParseReport() = %+v, %v; want %+v", got, err, want)
	}
}

func TestParseReportRefusesAReplyThatIsNotTheObjectOfTheContract(t *testing.T) {
	without := func(member string) string {
		c := map[string]string{"command": `"c"`, "result": `"pass"`, "output": `"o"`}
		delete(c, member)
		var parts []string
		for _, m := range []string{"command", "result", "output"} {
			if v, ok := c[m]; ok {
				parts = append(parts, `"`+m+`":`+v)
			}
		}
		return `{"checks":[{` + strings.Join(parts, ",") + `}]}`
	}
	for name, data := range map[string]string{
		"an empty report":                   "",
		"a report of white space":           " \n\t",
		"prose":                             "Everything passed.",
		"a fence around the object":         "```json\n" + cleanReport() + "\n```",
		"an array":                          "[]",
		"null":                              "null",
		"two objects":                       cleanReport() + cleanReport(),
		"a key written twice":               `{"checks":[],"checks":[]}`,
		"bytes that are not UTF-8":          "{\"checks\":[]}\xff",
		"no checks member":                  `{}`,
		"a null checks member":              `{"checks":null}`,
		"checks that is an object":          `{"checks":{}}`,
		"a check that is not an object":     `{"checks":["go test ./..."]}`,
		"a check with no command":           without("command"),
		"a check with no result":            without("result"),
		"a check with no output":            without("output"),
		"a null member":                     `{"checks":[{"command":"c","result":null,"output":"o"}]}`,
		"a member of the wrong type":        `{"checks":[{"command":"c","result":"pass","output":1}]}`,
		"a null before a value out of enum": `{"checks":[{"command":"c","result":null,"output":"o"},{"command":"c","result":"maybe","output":"o"}]}`,
	} {
		t.Run(name, func(t *testing.T) { refusedReport(t, data, reviewverdict.ErrUnparseable) })
	}
}

// A missing report is the first thing an exemption cannot do without, and the message says so.
func TestParseReportSaysWhenThereIsNoReportAtAll(t *testing.T) {
	for name, data := range map[string]string{"nothing": "", "white space": " \n\t"} {
		t.Run(name, func(t *testing.T) {
			if message := refusedReport(t, data, reviewverdict.ErrUnparseable); !strings.Contains(message, "empty") {
				t.Errorf("error = %q, want it to say the report is empty", message)
			}
		})
	}
}

func TestParseReportRefusesAMemberTheContractDoesNotName(t *testing.T) {
	for name, data := range map[string]string{
		"in the report":               `{"checks":[],"outcome":"clear"}`,
		"in a check":                  `{"checks":[{"command":"c","result":"pass","output":"o","exit_code":0}]}`,
		"by another case":             `{"checks":[{"Command":"c","result":"pass","output":"o"}]}`,
		"the verdicts of a review":    `{"verdicts":[]}`,
		"an unknown member and a gap": `{"checks":[{"command":"c","output":"o","note":"x"}]}`,
	} {
		t.Run(name, func(t *testing.T) { refusedReport(t, data, reviewverdict.ErrUnknownMember) })
	}
}

func TestParseReportRefusesAResultOutsideItsEnumeration(t *testing.T) {
	for name, result := range map[string]string{"another word": "passed", "another case": "PASS", "ok": "ok", "an empty result": ""} {
		t.Run(name, func(t *testing.T) {
			message := refusedReport(t, reportOf(checkOf("c", "pass", "o"), checkOf("go test ./...", result, "o")), reviewverdict.ErrOutsideEnum)
			if !strings.Contains(message, "checks[1]") || !strings.Contains(message, fmt.Sprintf("%q", result)) {
				t.Errorf("error = %q, want it to name checks[1] and the result %q", message, result)
			}
		})
	}
}

// Every check carries its evidence, whether it passed or not: the command that was run, and what it printed or why
// it did not run. A check that did not run is reported, never inferred, and says why.
func TestParseReportRefusesACheckWithoutEvidence(t *testing.T) {
	for _, result := range []string{"pass", "fail", "not-run"} {
		for name, tc := range map[string]struct{ command, output, want string }{
			"an empty command":      {"", "o", "no command"},
			"a blank command":       {"  \t", "o", "no command"},
			"an empty output":       {"c", "", "no output"},
			"an output of newlines": {"c", "\n \n", "no output"},
		} {
			t.Run(result+" with "+name, func(t *testing.T) {
				message := refusedReport(t, reportOf(checkOf("go vet ./...", "pass", "ok"), checkOf(tc.command, result, tc.output)), reviewverdict.ErrMissingEvidence)
				if !strings.Contains(message, "checks[1]") || !strings.Contains(message, tc.want) {
					t.Errorf("error = %q, want it to name checks[1] and say %q", message, tc.want)
				}
			})
		}
	}
}

// The rules are applied in one order and the first broken is the one reported, so a reply is always refused for
// the same reason: the shape, then each check in turn with its enumeration before its evidence.
func TestParseReportRefusesForTheFirstRuleABrokenReplyBreaks(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		want       error
	}{
		{"a member of the wrong type before a member the contract does not name",
			`{"checks":[{"command":"c","result":"pass","output":1,"note":"x"}]}`, reviewverdict.ErrUnknownMember},
		{"a check with a member missing before a later check with a member the contract does not name",
			`{"checks":[{"command":"c","output":"o"},{"command":"c","result":"pass","output":"o","note":"x"}]}`, reviewverdict.ErrUnparseable},
		{"a result outside its enumeration before a check without evidence",
			reportOf(checkOf("", "maybe", "")), reviewverdict.ErrOutsideEnum},
		{"the evidence of the first check before the enumeration of the second",
			reportOf(checkOf("c", "pass", ""), checkOf("c", "maybe", "o")), reviewverdict.ErrMissingEvidence},
	} {
		t.Run(tc.name, func(t *testing.T) { refusedReport(t, tc.data, tc.want) })
	}
}

// A report is clear when it names at least one check and every check passed. A check that failed is not clear, and
// neither is one that did not run: a check that did not run never passes. The refusal names the first check that
// keeps the report from being clear, and no other reason.
func TestClearIsAReportOfAtLeastOneCheckEveryCheckOfWhichPassed(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		wantIn     []string // what the message says; none when the report is clear
	}{
		{"one check that passed", reportOf(checkOf("c", "pass", "o")), nil},
		{"two checks that passed", cleanReport(), nil},
		{"no check at all", reportOf(), []string{"names no check"}},
		{"a check that failed", reportOf(checkOf("go vet ./...", "fail", "o")), []string{"checks[0]", "go vet ./...", "fail"}},
		{"a check that did not run", reportOf(checkOf("go test ./...", "not-run", "no toolchain")), []string{"checks[0]", "go test ./...", "not-run"}},
		{"a check that failed after one that passed", reportOf(checkOf("c1", "pass", "o"), checkOf("c2", "fail", "o")), []string{"checks[1]", "c2"}},
		{"the first of two checks that are not clear", reportOf(checkOf("c1", "not-run", "o"), checkOf("c2", "fail", "o")), []string{"checks[0]", "c1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report, err := reviewverdict.ParseReport([]byte(tc.data))
			if err != nil {
				t.Fatalf("setup: ParseReport() error = %v", err)
			}

			err = report.Clear()

			if tc.wantIn == nil {
				if err != nil {
					t.Errorf("Clear() error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, reviewverdict.ErrNotClear) {
				t.Fatalf("Clear() error = %v, want one that wraps ErrNotClear", err)
			}
			for _, reason := range reportErrors {
				if errors.Is(err, reason) {
					t.Errorf("Clear() error = %v also wraps %v, want the refusal of a clear report to say only ErrNotClear", err, reason)
				}
			}
			for _, w := range tc.wantIn {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("message %q does not say %q", err, w)
				}
			}
		})
	}
}

// The report is not the object of the verdicts: the verifier's reply about the findings of a result is refused by
// ParseReport as it is, and a report is refused by Parse, so neither can stand in for the other.
func TestAReportAndTheVerdictsOfAResultAreNotEachOther(t *testing.T) {
	if _, err := reviewverdict.Parse([]byte(cleanReport()), answered); !errors.Is(err, reviewverdict.ErrUnknownMember) {
		t.Errorf("Parse(a report) error = %v, want one that wraps ErrUnknownMember", err)
	}
	refusedReport(t, both(), reviewverdict.ErrUnknownMember)
}
