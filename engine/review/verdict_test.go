package review_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewverdict"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
)

// verdictRef and verdictSHA stand for a verdict that was stored beside the result of handoff_test.go.
const (
	verdictRef = "odd/review-results/own-rdd/222222222222-1.verifier.json"
	verdictSHA = "5555555555555555555555555555555555555555555555555555555555555555"
)

// A verdict is stored beside the result it answers, under the same <head12>-<n>, so that it is found from the
// result and the result from it.
func TestVerdictNameIsTheNameOfTheResultWithVerifierBeforeTheExtension(t *testing.T) {
	for result, want := range map[string]string{
		"222222222222-1.json":  "222222222222-1.verifier.json",
		"abcdef012345-12.json": "abcdef012345-12.verifier.json",
	} {
		if got, err := review.VerdictName(result); err != nil || got != want {
			t.Errorf("VerdictName(%q) = %q, %v; want %q", result, got, err, want)
		}
	}
}

func TestResultOfVerdictIsTheResultTheVerdictIsBeside(t *testing.T) {
	for _, result := range []string{"222222222222-1.json", "abcdef012345-12.json"} {
		verdict, err := review.VerdictName(result)
		if err != nil {
			t.Fatalf("setup: VerdictName(%q) error = %v", result, err)
		}
		if got, ok := review.ResultOfVerdict(verdict); !ok || got != result {
			t.Errorf("ResultOfVerdict(%q) = %q, %v; want %q, true", verdict, got, ok, result)
		}
	}
	for _, name := range []string{
		"222222222222-1.json", "222222222222-1.verifier.md", ".verifier.json", "222222222222-x.verifier.json",
		"222222222222-1.verifier.json.bak", "222222222222-01.verifier.json", "../222222222222-1.verifier.json",
	} {
		if got, ok := review.ResultOfVerdict(name); ok {
			t.Errorf("ResultOfVerdict(%q) = %q, true; want false: it is not the name of a verdict", name, got)
		}
	}
}

// A name is the name of a result when ResultName makes it, and a verdict is made for no other: the name
// is a file of the results directory and is never a path.
func TestCheckResultNameAcceptsWhatResultNameMakes(t *testing.T) {
	for _, n := range []int{1, 2, 10, 999999999} {
		if name := review.ResultName(candidate(t), n); review.CheckResultName(name) != nil {
			t.Errorf("CheckResultName(%q) = %v, want nil", name, review.CheckResultName(name))
		}
	}
}

func TestCheckResultNameRefusesAnyOtherName(t *testing.T) {
	for _, name := range []string{
		"", "222222222222-1", "222222222222-1.md", "222222222222-1.verifier.json", "22222222222-1.json",
		"2222222222222-1.json", "22222222222g-1.json", "22222222222A-1.json", "222222222222-0.json",
		"222222222222-01.json", "222222222222-1000000000.json", "222222222222-.json", "-1.json", "222222222222_1.json",
		"../222222222222-1.json", "a/222222222222-1.json", "222222222222-1.json\n", ".222222222222-1.json",
	} {
		err := review.CheckResultName(name)
		if !errors.Is(err, review.ErrInvalidResultName) {
			t.Errorf("CheckResultName(%q) = %v, want an error wrapping ErrInvalidResultName", name, err)
		}
		if _, verr := review.VerdictName(name); !errors.Is(verr, review.ErrInvalidResultName) {
			t.Errorf("VerdictName(%q) error = %v, want an error wrapping ErrInvalidResultName", name, verr)
		}
	}
}

// The next number of a head counts the verdicts stored beside its results, so that a launch never takes the
// number of a verdict.
func TestNextNumberCountsAVerdictBesideItsResult(t *testing.T) {
	names := []string{"222222222222-1.json", "222222222222-1.verifier.json", "222222222222-2.json", "222222222222-2.verifier.json"}
	if got := review.NextNumber(names, candidate(t)); got != 3 {
		t.Errorf("NextNumber(%q) = %d, want 3", names, got)
	}
	if got := review.NextNumber([]string{"222222222222-4.verifier.json"}, candidate(t)); got != 5 {
		t.Errorf("NextNumber() of a lone verdict = %d, want 5", got)
	}
}

// The stored file of a result reads back as the binding it was held to, the rows its launch was sent and the reply
// as it was given.
func TestDecodeResultReadsWhatEncodeResultWrote(t *testing.T) {
	c := candidate(t)
	data, err := review.EncodeResult(c, sentRows(), []byte(replyText))
	if err != nil {
		t.Fatalf("setup: EncodeResult() error = %v", err)
	}

	got, err := review.DecodeResult(data)

	if err != nil {
		t.Fatalf("DecodeResult() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(got.Binding, c.Binding()) {
		t.Errorf("Binding = %+v, want %+v", got.Binding, c.Binding())
	}
	if !reflect.DeepEqual(got.Rows, sentRows()) {
		t.Errorf("Rows = %q, want the rows that were stored: %q", got.Rows, sentRows())
	}
	var read, given any
	if err := json.Unmarshal(got.Reply, &read); err != nil {
		t.Fatalf("Reply is not JSON: %v\n%s", err, got.Reply)
	}
	if err := json.Unmarshal([]byte(replyText), &given); err != nil || !reflect.DeepEqual(read, given) {
		t.Errorf("Reply = %s, want the reply that was stored: %s", got.Reply, replyText)
	}
}

func TestDecodeResultRefusesWhatEncodeResultDoesNotWrite(t *testing.T) {
	good, err := review.EncodeResult(candidate(t), sentRows(), []byte(replyText))
	if err != nil {
		t.Fatalf("setup: EncodeResult() error = %v", err)
	}
	// edited is the file good with its members edited by edit, written as JSON again.
	edited := func(edit func(members map[string]any)) string {
		var members map[string]any
		if err := json.Unmarshal(good, &members); err != nil {
			t.Fatalf("setup: %v", err)
		}
		edit(members)
		data, err := json.Marshal(members)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		return string(data)
	}
	cases := map[string]string{
		"nothing":             "",
		"text":                "a result",
		"an array":            "[]",
		"another version":     strings.Replace(string(good), `"version": 1`, `"version": 2`, 1),
		"no version":          strings.Replace(string(good), `"version": 1,`, ``, 1),
		"a member of no kind": strings.Replace(string(good), `"version": 1,`, `"version": 1, "note": "x",`, 1),
		"no reply":            `{"version": 1, "binding": {}}`,
		"a null reply":        `{"version": 1, "binding": {}, "reply": null}`,
		"two objects":         string(good) + string(good),
	}
	// A file with no rows is the shape of a result stored before the rows were recorded: it is refused, and not read
	// as the result of a launch that was sent no row.
	cases["no rows"] = edited(func(m map[string]any) { delete(m, "rows") })
	cases["null rows"] = edited(func(m map[string]any) { m["rows"] = nil })
	cases["an empty list of rows"] = edited(func(m map[string]any) { m["rows"] = []any{} })
	cases["a row nobody wrote"] = edited(func(m map[string]any) { m["rows"] = []any{"default", "nope"} })
	cases["a row written twice"] = edited(func(m map[string]any) { m["rows"] = []any{"default", "default"} })
	cases["rows that are not a list"] = edited(func(m map[string]any) { m["rows"] = "default" })
	cases["a row that is not a string"] = edited(func(m map[string]any) { m["rows"] = []any{1} })
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if got, err := review.DecodeResult([]byte(data)); !errors.Is(err, review.ErrInvalidStored) {
				t.Errorf("DecodeResult() = %+v, %v; want an error wrapping ErrInvalidStored", got, err)
			}
		})
	}
}

// The file of a verdict is for people as well as programs: the reply of the verifier, indented, ending in a
// newline, with the characters it was written with.
func TestEncodeVerdictWritesTheReplyIndentedAndKeepsItsCharacters(t *testing.T) {
	reply := ` {"verdicts":[{"id":"F1","result":"reproduced","command":"go test ./...","output":"a < b && c > d","base_result":"not-run"}]}` + "\n\n"

	data, err := review.EncodeVerdict([]byte(reply))

	if err != nil {
		t.Fatalf("EncodeVerdict() error = %v, want nil", err)
	}
	text := string(data)
	if !strings.HasPrefix(text, "{\n  \"verdicts\": [\n") || !strings.HasSuffix(text, "}\n") || strings.HasSuffix(text, "\n\n") {
		t.Errorf("the file is %q, want an indented object ending in one newline", text)
	}
	if !strings.Contains(text, `"a < b && c > d"`) {
		t.Errorf("the file does not hold the text of the reply as written (< and & unescaped):\n%s", text)
	}
	var read, given any
	if err := json.Unmarshal(data, &read); err != nil || json.Unmarshal([]byte(reply), &given) != nil || !reflect.DeepEqual(read, given) {
		t.Errorf("the file is not the reply that was given:\n%s", text)
	}
}

func TestEncodeVerdictRefusesAReplyThatIsNotJSON(t *testing.T) {
	for _, reply := range []string{"", "no", `{"a":`} {
		if data, err := review.EncodeVerdict([]byte(reply)); !errors.Is(err, review.ErrReplyNotJSON) {
			t.Errorf("EncodeVerdict(%q) = %q, %v, want an error wrapping ErrReplyNotJSON", reply, data, err)
		}
	}
}

// A correction is the same candidate with another head: the builder fixed what the review found and the fix
// is reviewed against the same base. A second launch on the same head is no correction, and neither is a
// range from another base.
func TestIsCorrectionIsAnotherHeadOfTheSameBase(t *testing.T) {
	binding := func(base, head string) reviewresult.Binding { return reviewresult.Binding{Base: base, Head: head} }
	for _, tc := range []struct {
		name             string
		earlier, current reviewresult.Binding
		want             bool
	}{
		{"another head of the same base", binding("b1", "h1"), binding("b1", "h2"), true},
		{"the same head: a second launch", binding("b1", "h1"), binding("b1", "h1"), false},
		{"another base, the same head", binding("b1", "h1"), binding("b2", "h1"), false},
		{"another base and another head", binding("b1", "h1"), binding("b2", "h2"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := review.IsCorrection(tc.earlier, tc.current); got != tc.want {
				t.Errorf("IsCorrection(%+v, %+v) = %v, want %v", tc.earlier, tc.current, got, tc.want)
			}
		})
	}
}

// fill completes the skeleton as the parent does with the members it names, except to_role when the skeleton
// already says who is next.
func fill(t *testing.T, h roles.RoleHandoff) []byte {
	t.Helper()
	data, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("setup: %v", err)
	}
	for name, value := range parentValues("") {
		switch {
		case name == "context.summary":
			record["context"].(map[string]any)["summary"] = value
		case name != "to_role":
			record[name] = value
		}
	}
	out, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return out
}

// The skeleton of a verified review is the one of the reviewed range with the verdict bound as one more piece
// of evidence, and it says who gets the work next: the builder when something blocks, the delivery when the
// review is clear, and nobody yet when a check did not run.
func TestVerifiedHandoffBindsTheVerdictAndNamesWhoIsNext(t *testing.T) {
	for outcome, want := range map[reviewverdict.Outcome]roles.Role{
		reviewverdict.Blocks: roles.RoleBuilder, reviewverdict.Clear: roles.RoleDelivery, reviewverdict.Unverified: "",
	} {
		t.Run(string(outcome), func(t *testing.T) {
			got := review.VerifiedHandoff(candidate(t).Binding(), review.FileRef{Ref: storedRef, SHA256: storedSHA}, review.FileRef{Ref: verdictRef, SHA256: verdictSHA}, outcome)

			wantEvidence := []roles.Evidence{
				{Kind: "review-diff", Ref: baseSHA + ".." + headSHA, SHA256: diffSHA},
				{Kind: "review-result", Ref: storedRef, SHA256: storedSHA},
				{Kind: "verifier-report", Ref: verdictRef, SHA256: verdictSHA},
			}
			if !reflect.DeepEqual(got.Evidence, wantEvidence) || review.EvidenceVerifier != "verifier-report" {
				t.Errorf("evidence = %+v, want %+v", got.Evidence, wantEvidence)
			}
			if got.ToRole != want || got.FromRole != roles.RoleReviewer || got.PayloadKind != "review-report" || got.PayloadSHA256 != storedSHA || got.Status != roles.StatusCompleted {
				t.Errorf("to_role %q, from_role %q, payload_kind %q, payload_sha256 %q, status %q; want to_role %q, the reviewer's review-report of the result", got.ToRole, got.FromRole, got.PayloadKind, got.PayloadSHA256, got.Status, want)
			}
			record := fill(t, got)
			h, err := roles.ParseRoleHandoff(record)
			switch {
			case want == "" && (err == nil || !strings.Contains(err.Error(), "to_role")):
				t.Errorf("ParseRoleHandoff() = %v, want a refusal that names to_role while nobody is next:\n%s", err, record)
			case want != "" && err != nil:
				t.Errorf("ParseRoleHandoff() error = %v for\n%s", err, record)
			case want != "" && h.ToRole != want:
				t.Errorf("parsed to_role = %q, want %q", h.ToRole, want)
			}
		})
	}
}
