package skills

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewverdict"
)

// exemptionOverride starts the line of the delta of the verifier that tells it what to reply with when the prompt asks
// for the report of an exemption (own-rdd T4, S5, L16) and lists no finding id.
const exemptionOverride = "When the prompt asks for the report of an exemption"

// exemptionReportObject is the object that line of the delta of roles/verifier/body.md spells out, decoded, and the one
// check it shows. The line is found by how it starts, not by its place.
func exemptionReportObject(t *testing.T) (line string, spelled, check map[string]any) {
	t.Helper()
	delta := roleDelta(t, skillsRepoRoot(t), "verifier")
	at := slices.IndexFunc(delta, func(l string) bool { return strings.HasPrefix(l, exemptionOverride) })
	if at < 0 {
		t.Fatalf("no line of the verifier delta starts %q; got:\n%s", exemptionOverride, strings.Join(delta, "\n"))
	}
	line = delta[at]
	start := strings.Index(line, `{"checks"`)
	if start < 0 {
		t.Fatalf("the line of the verifier delta for an exemption spells out no JSON object that starts with \"checks\"; got:\n%s", line)
	}
	if err := json.NewDecoder(strings.NewReader(line[start:])).Decode(&spelled); err != nil {
		t.Fatalf("the object the verifier delta spells out for an exemption is not JSON: %v", err)
	}
	if checks, _ := spelled["checks"].([]any); len(checks) == 1 {
		check, _ = checks[0].(map[string]any)
	}
	if check == nil {
		t.Fatalf("the verifier delta must spell out one check, as an object; got %v", spelled["checks"])
	}
	return line, spelled, check
}

// The report of an exemption is one JSON object, and the agent file of the verifier is the only place it can read the
// shape of that object: the prompt of the parent names the checks and the worktree, not the format. The line of its
// delta for an exemption says when it applies (the prompt lists no finding id), that it replaces the return envelope
// in the way the verdicts do, and spells the object out with the members engine/reviewverdict requires, the values it
// lists and the rules it refuses a report for.
func TestVerifierDeltaSpellsOutTheExemptionReportItReturns(t *testing.T) {
	line, spelled, check := exemptionReportObject(t)

	if len(spelled) != 1 {
		t.Errorf("the object has %d members, want only checks: a report is evidence, never a schema, a receipt or an authority", len(spelled))
	}
	var members []string
	for member := range check {
		members = append(members, member)
	}
	sort.Strings(members)
	if want := []string{"command", "output", "result"}; !reflect.DeepEqual(members, want) {
		t.Errorf("a check has the members %v, want %v", members, want)
	}
	if check["result"] != "pass|fail|not-run" {
		t.Errorf("result = %v, want the three values of the contract", check["result"])
	}
	for _, rule := range []string{
		"it lists no finding id and names the checks",
		"this replaces the return envelope in the same way",
		"your whole reply is one JSON object and no prose",
		"one entry for each check the prompt names, in the order you ran them",
		"command and output are never empty",
		"no member is omitted or null",
		"not-run is never a pass",
	} {
		if !strings.Contains(line, rule) {
			t.Errorf("the line of the verifier delta for an exemption must say %q; got:\n%s", rule, line)
		}
	}
}

// The object the delta spells out for an exemption is the object the engine parses, and a drift between the two is an
// exemption that never happens: the agent would reply with a member the parser refuses as unknown, or without one it
// requires. The test fills the object with a legal value for each member and requires reviewverdict.ParseReport to
// accept it for each value the delta lists for result, and requires the report to be clear only when every check
// passed. What the delta forbids, the parser refuses.
func TestVerifierDeltaExemptionObjectIsTheOneTheEngineParses(t *testing.T) {
	_, _, check := exemptionReportObject(t)
	legal := map[string]any{"command": "go test ./engine/review/...", "output": "ok  review 0.009s"}
	list, ok := check["result"].(string)
	if !ok {
		t.Fatalf("the verifier delta must list the alternatives of result as one string; got %v", check["result"])
	}
	alternatives := strings.Split(list, "|")
	want := []string{string(reviewverdict.CheckPassed), string(reviewverdict.CheckFailed), string(reviewverdict.CheckNotRun)}
	if !reflect.DeepEqual(alternatives, want) {
		t.Fatalf("the verifier delta lists %v for result, want the values of the contract %v", alternatives, want)
	}
	instance := func(result string) []byte {
		filled := map[string]any{}
		for member := range check {
			value, ok := legal[member]
			if member == "result" {
				value, ok = result, true
			}
			if !ok {
				t.Fatalf("the verifier delta names the member %q, and this test has no legal value for it", member)
			}
			filled[member] = value
		}
		text, err := json.Marshal(map[string]any{"checks": []any{filled}})
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		return text
	}
	for _, result := range alternatives {
		report, err := reviewverdict.ParseReport(instance(result))
		if err != nil {
			t.Errorf("reviewverdict.ParseReport refuses result %q, which the verifier delta lists: %v\n%s", result, err, instance(result))
			continue
		}
		clearness := report.Clear()
		switch {
		case result == string(reviewverdict.CheckPassed) && clearness != nil:
			t.Errorf("the report of one check that passed is not clear: %v", clearness)
		case result != string(reviewverdict.CheckPassed) && !errors.Is(clearness, reviewverdict.ErrNotClear):
			t.Errorf("the report of one check that is %q is %v, want %v: only a check that passed clears a report", result, clearness, reviewverdict.ErrNotClear)
		}
	}
	// What the delta forbids, the parser refuses: an empty output, and an empty command.
	for _, member := range []string{"output", "command"} {
		saved := legal[member]
		legal[member] = ""
		if _, err := reviewverdict.ParseReport(instance(string(reviewverdict.CheckPassed))); !errors.Is(err, reviewverdict.ErrMissingEvidence) {
			t.Errorf("the verifier delta says command and output are never empty, and reviewverdict.ParseReport of a check with an empty %s returns %v, want %v", member, err, reviewverdict.ErrMissingEvidence)
		}
		legal[member] = saved
	}
}
