package reviewverdict_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewverdict"
)

// answered is the stored result every reply below answers: a BLOCKER and a WARNING.
var answered = reviewresult.Result{Findings: []reviewresult.Finding{
	{ID: "F1", Severity: reviewresult.SeverityBlocker, Disposition: reviewresult.DispositionIntroduced},
	{ID: "F2", Severity: reviewresult.SeverityWarning, Disposition: reviewresult.DispositionUnknown},
}}

// entry is one verdict of a reply, written as the verifier writes it.
func entry(id, result, command, output, base string) string {
	return fmt.Sprintf(`{"id":%q,"result":%q,"command":%q,"output":%q,"base_result":%q}`, id, result, command, output, base)
}

// both is the reply that answers F1 and F2 with a check that ran for each.
func both() string {
	return reply(entry("F1", "reproduced", "go test ./a", "FAIL", "not-run"), entry("F2", "not-reproduced", "go vet ./a", "ok", "not-run"))
}

func reply(entries ...string) string { return `{"verdicts":[` + strings.Join(entries, ",") + `]}` }

// allErrors are the reasons Parse refuses a reply. A refusal wraps exactly one of them.
var allErrors = []error{
	reviewverdict.ErrUnparseable, reviewverdict.ErrUnknownMember, reviewverdict.ErrUnknownID,
	reviewverdict.ErrDuplicateID, reviewverdict.ErrUnanswered, reviewverdict.ErrOutsideEnum, reviewverdict.ErrMissingEvidence,
}

// refused requires Parse to refuse data for exactly the reason want, with no verdicts, and returns the message.
func refused(t *testing.T, data string, want error) string {
	t.Helper()
	got, err := reviewverdict.Parse([]byte(data), answered)
	if got != nil {
		t.Errorf("Parse(%q) = %+v, want no verdicts from a refused reply", data, got)
	}
	if err == nil {
		t.Fatalf("Parse(%q) error = nil, want one that is %v", data, want)
	}
	for _, reason := range allErrors {
		if errors.Is(err, reason) != (reason == want) {
			t.Errorf("Parse(%q) error = %v: errors.Is(%v) = %v, want a refusal for %v and no other", data, err, reason, !(reason == want), want)
		}
	}
	return err.Error()
}

func TestParseReturnsTheVerdictOfEachFindingInTheOrderTheVerifierWroteThem(t *testing.T) {
	data := reply(
		entry("F2", "not-run", "", "no go toolchain in the worktree", "not-run"),
		entry("F1", "reproduced", "go test ./engine/ -run TestGuard", "--- FAIL: TestGuard", "not-reproduced"))

	got, err := reviewverdict.Parse([]byte(data), answered)

	want := []reviewverdict.Verdict{
		{ID: "F2", Result: reviewverdict.NotRun, Output: "no go toolchain in the worktree", Base: reviewverdict.NotRun},
		{ID: "F1", Result: reviewverdict.Reproduced, Command: "go test ./engine/ -run TestGuard", Output: "--- FAIL: TestGuard", Base: reviewverdict.NotReproduced},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Parse() = %+v, %v; want %+v", got, err, want)
	}
}

func TestParseRefusesAReplyThatIsNotTheObjectOfTheContract(t *testing.T) {
	without := func(member string) string {
		e := map[string]string{"id": `"F1"`, "result": `"reproduced"`, "command": `"c"`, "output": `"o"`, "base_result": `"not-run"`}
		delete(e, member)
		var parts []string
		for _, m := range []string{"id", "result", "command", "output", "base_result"} {
			if v, ok := e[m]; ok {
				parts = append(parts, `"`+m+`":`+v)
			}
		}
		return `{"verdicts":[{` + strings.Join(parts, ",") + `},` + entry("F2", "not-run", "", "x", "not-run") + `]}`
	}
	for name, data := range map[string]string{
		"an empty reply":                                "",
		"prose":                                         "Both findings reproduce.",
		"a fence around the object":                     "```json\n" + both() + "\n```",
		"an array":                                      "[]",
		"null":                                          "null",
		"two objects":                                   both() + both(),
		"a key written twice":                           `{"verdicts":[],"verdicts":[]}`,
		"bytes that are not UTF-8":                      "{\"verdicts\":[]}\xff",
		"no verdicts member":                            `{}`,
		"a null verdicts member":                        `{"verdicts":null}`,
		"verdicts that is an object":                    `{"verdicts":{}}`,
		"a verdict that is not an object":               `{"verdicts":["F1"]}`,
		"a verdict with no id":                          without("id"),
		"a verdict with no result":                      without("result"),
		"a verdict with no command":                     without("command"),
		"a verdict with no output":                      without("output"),
		"a verdict with no base_result":                 without("base_result"),
		"a null member":                                 `{"verdicts":[{"id":"F1","result":null,"command":"c","output":"o","base_result":"not-run"}]}`,
		"a member of the wrong type":                    `{"verdicts":[{"id":"F1","result":"reproduced","command":1,"output":"o","base_result":"not-run"}]}`,
		"a null before a value outside its enumeration": `{"verdicts":[{"id":"F1","result":null,"command":"c","output":"o","base_result":"x"}]}`,
	} {
		t.Run(name, func(t *testing.T) { refused(t, data, reviewverdict.ErrUnparseable) })
	}
}

func TestParseRefusesAMemberTheContractDoesNotName(t *testing.T) {
	for name, data := range map[string]string{
		"in the reply":    `{"verdicts":[],"schema":"v1"}`,
		"in a verdict":    `{"verdicts":[{"id":"F1","result":"reproduced","command":"c","output":"o","base_result":"not-run","note":"x"}]}`,
		"by another case": `{"verdicts":[{"id":"F1","Result":"reproduced","command":"c","output":"o","base_result":"not-run"}]}`,
	} {
		t.Run(name, func(t *testing.T) { refused(t, data, reviewverdict.ErrUnknownMember) })
	}
}

func TestParseRefusesAnIDNoFindingHas(t *testing.T) {
	for name, id := range map[string]string{"an id of no finding": "F9", "a blank id": ""} {
		t.Run(name, func(t *testing.T) {
			message := refused(t, reply(entry("F1", "reproduced", "c", "o", "not-run"), entry(id, "reproduced", "c", "o", "not-run")), reviewverdict.ErrUnknownID)
			if !strings.Contains(message, fmt.Sprintf("%q", id)) {
				t.Errorf("error = %q, want it to name the id %q", message, id)
			}
		})
	}
}

func TestParseRefusesAFindingAnsweredTwice(t *testing.T) {
	message := refused(t, reply(entry("F1", "reproduced", "c", "o", "not-run"), entry("F2", "not-run", "", "o", "not-run"), entry("F1", "not-reproduced", "c", "o", "not-run")), reviewverdict.ErrDuplicateID)
	if !strings.Contains(message, `"F1"`) {
		t.Errorf("error = %q, want it to name F1", message)
	}
}

func TestParseRefusesAFindingNobodyAnswered(t *testing.T) {
	for name, tc := range map[string]struct{ data, want string }{
		"one of two": {reply(entry("F2", "not-run", "", "o", "not-run")), `"F1"`},
		"none":       {reply(), `"F1", "F2"`},
	} {
		t.Run(name, func(t *testing.T) {
			message := refused(t, tc.data, reviewverdict.ErrUnanswered)
			if !strings.Contains(message, tc.want) {
				t.Errorf("error = %q, want it to name %s", message, tc.want)
			}
		})
	}
	t.Run("a result with no finding has nothing to answer", func(t *testing.T) {
		got, err := reviewverdict.Parse([]byte(reply()), reviewresult.Result{})
		if err != nil || len(got) != 0 {
			t.Errorf("Parse() = %+v, %v; want no verdicts and no error", got, err)
		}
	})
}

func TestParseRefusesAValueOutsideItsEnumeration(t *testing.T) {
	for _, value := range []string{"maybe", "", "Reproduced", "reproduced ", "not_run"} {
		t.Run("result "+value, func(t *testing.T) {
			message := refused(t, reply(entry("F1", value, "c", "o", "not-run"), entry("F2", "not-run", "", "o", "not-run")), reviewverdict.ErrOutsideEnum)
			if !strings.Contains(message, "result") || !strings.Contains(message, fmt.Sprintf("%q", value)) {
				t.Errorf("error = %q, want it to name result and %q", message, value)
			}
		})
		t.Run("base_result "+value, func(t *testing.T) {
			message := refused(t, reply(entry("F1", "reproduced", "c", "o", value), entry("F2", "not-run", "", "o", "not-run")), reviewverdict.ErrOutsideEnum)
			if !strings.Contains(message, "base_result") || !strings.Contains(message, fmt.Sprintf("%q", value)) {
				t.Errorf("error = %q, want it to name base_result and %q", message, value)
			}
		})
	}
	for _, value := range []reviewverdict.Reproduction{reviewverdict.Reproduced, reviewverdict.NotReproduced, reviewverdict.NotRun} {
		t.Run("accepts "+string(value), func(t *testing.T) {
			got, err := reviewverdict.Parse([]byte(reply(entry("F1", string(value), "c", "o", string(value)), entry("F2", "not-run", "", "o", "not-run"))), answered)
			if err != nil || got[0].Result != value || got[0].Base != value {
				t.Errorf("Parse() = %+v, %v; want result and base_result %q", got, err, value)
			}
		})
	}
}

func TestParseRefusesAVerdictWithoutEvidence(t *testing.T) {
	for name, e := range map[string]string{
		"no output for a reproduction":      entry("F1", "reproduced", "c", " ", "not-run"),
		"no output for a failed one":        entry("F1", "not-reproduced", "c", "", "not-run"),
		"no output for a check not run":     entry("F1", "not-run", "", "", "not-run"),
		"no command for a reproduction":     entry("F1", "reproduced", "", "o", "not-run"),
		"no command for a failed one":       entry("F1", "not-reproduced", "  ", "o", "not-run"),
		"no command for a run only on base": entry("F1", "not-run", "", "o", "reproduced"),
	} {
		t.Run(name, func(t *testing.T) {
			refused(t, reply(e, entry("F2", "not-run", "", "o", "not-run")), reviewverdict.ErrMissingEvidence)
		})
	}
	t.Run("a check not run needs no command", func(t *testing.T) {
		if _, err := reviewverdict.Parse([]byte(reply(entry("F1", "not-run", "", "no toolchain", "not-run"), entry("F2", "not-run", "", "o", "not-run"))), answered); err != nil {
			t.Errorf("Parse() error = %v, want nil", err)
		}
	})
}

// A reply that breaks several rules is refused for the first one in the order of the package comment, so the
// same reply is always refused for the same reason.
func TestParseRefusesForTheFirstRuleABrokenReplyBreaks(t *testing.T) {
	for name, tc := range map[string]struct {
		data string
		want error
	}{
		"the shape before the ids":               {`{"verdicts":[{"id":"F9"}]}`, reviewverdict.ErrUnparseable},
		"an unknown member before the shape":     {`{"verdicts":[{"id":"F9","oops":1}]}`, reviewverdict.ErrUnknownMember},
		"an unknown id before the enum":          {reply(entry("F9", "maybe", "c", "o", "not-run")), reviewverdict.ErrUnknownID},
		"an enum before the evidence":            {reply(entry("F1", "maybe", "", "", "not-run")), reviewverdict.ErrOutsideEnum},
		"a verdict at a time":                    {reply(entry("F1", "maybe", "c", "o", "not-run"), entry("F9", "reproduced", "c", "o", "not-run")), reviewverdict.ErrOutsideEnum},
		"a repeated id before an unanswered one": {reply(entry("F1", "reproduced", "c", "o", "not-run"), entry("F1", "reproduced", "c", "o", "not-run")), reviewverdict.ErrDuplicateID},
	} {
		t.Run(name, func(t *testing.T) { refused(t, tc.data, tc.want) })
	}
}
