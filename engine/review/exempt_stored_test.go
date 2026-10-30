package review_test

// The stored exemption and the skeleton of the record that closes it (own-rdd T4, S5). The file is written like a
// result: the binding of the candidate it is about, and what the exemption was made on, so that a later reader holds
// it to the declaration and to the report again, and compares the binding with the one it computes from git.

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
)

// reportWithMarkup is a clear report whose output has characters that a JSON encoder escapes for HTML: the file
// holds what the verifier said, and a reader of the file sees the text and not an escape.
const reportWithMarkup = `{"checks":[{"command":"go test ./review/...","result":"pass","output":"ok  a < b && c > d"}]}`

const exemptionRef = "odd/review-results/own-rdd/222222222222-1.exempt.json"

func TestEncodeExemptionStoresTheBindingTheRiskAndTheReport(t *testing.T) {
	data, err := review.EncodeExemption(candidate(t), "none", []byte(reportWithMarkup))
	if err != nil {
		t.Fatalf("EncodeExemption() error = %v, want nil", err)
	}
	var got struct {
		Version int `json:"version"`
		Binding struct {
			Base       string   `json:"base"`
			Head       string   `json:"head"`
			DiffSHA256 string   `json:"diff_sha256"`
			Paths      []string `json:"paths"`
		} `json:"binding"`
		Risk   string          `json:"risk"`
		Report json.RawMessage `json:"report"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("the stored file is not JSON: %v\n%s", err, data)
	}
	if got.Version != 1 || review.StoredVersion != 1 {
		t.Errorf("version = %d (StoredVersion %d), want 1", got.Version, review.StoredVersion)
	}
	if got.Binding.Base != baseSHA || got.Binding.Head != headSHA || got.Binding.DiffSHA256 != diffSHA {
		t.Errorf("binding = %+v, want base %s, head %s and diff_sha256 %s", got.Binding, baseSHA, headSHA, diffSHA)
	}
	if !reflect.DeepEqual(got.Binding.Paths, changedPaths()) {
		t.Errorf("binding paths = %q, want %q", got.Binding.Paths, changedPaths())
	}
	if got.Risk != "none" {
		t.Errorf("risk = %q, want the declaration that was given, none", got.Risk)
	}
	var storedReport, givenReport any
	if err := json.Unmarshal(got.Report, &storedReport); err != nil {
		t.Fatalf("the stored report is not JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(reportWithMarkup), &givenReport); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if !reflect.DeepEqual(storedReport, givenReport) {
		t.Errorf("the stored report is %s, want the report that was given: %s", got.Report, reportWithMarkup)
	}
}

// The file is for people as well as programs: indented, ending in a newline, and with the characters of the
// report as they were written.
func TestEncodeExemptionWritesAReadableFileThatKeepsTheCharactersOfTheReport(t *testing.T) {
	data, err := review.EncodeExemption(candidate(t), "none", []byte(reportWithMarkup))
	if err != nil {
		t.Fatalf("EncodeExemption() error = %v, want nil", err)
	}
	text := string(data)
	if !strings.HasPrefix(text, "{\n  \"version\": 1,\n  \"binding\": {\n") {
		t.Errorf("the file begins %q, want an indented object that starts with the version and the binding", firstLines(text, 3))
	}
	if !strings.HasSuffix(text, "}\n") {
		t.Errorf("the file ends %q, want a closing brace and one newline", text[max(0, len(text)-5):])
	}
	if !strings.Contains(text, `"ok  a < b && c > d"`) {
		t.Errorf("the file does not hold the text of the report as written (< and & unescaped):\n%s", text)
	}
}

func TestEncodeExemptionWritesNoPathAsAnEmptyArray(t *testing.T) {
	c, err := review.NewCandidate(baseSHA, headSHA, nil, nil)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	data, err := review.EncodeExemption(c, "none", []byte(reportWithMarkup))
	if err != nil {
		t.Fatalf("EncodeExemption() error = %v, want nil", err)
	}
	if !strings.Contains(string(data), `"paths": []`) {
		t.Errorf("the file does not say \"paths\": []:\n%s", data)
	}
}

// What is not JSON cannot be embedded in a JSON file; the command never gets here with such a report (it is
// refused first), and the function does not write a file that does not parse.
func TestEncodeExemptionRefusesAReportThatIsNotJSON(t *testing.T) {
	for _, report := range []string{"", "no", `{"checks":`} {
		if data, err := review.EncodeExemption(candidate(t), "none", []byte(report)); !errors.Is(err, review.ErrReplyNotJSON) {
			t.Errorf("EncodeExemption(%q) = %q, %v, want an error wrapping ErrReplyNotJSON", report, data, err)
		}
	}
}

// What was stored is what is read back: the binding as the candidate gives it, the declaration, and the report as it
// was given (its white space aside).
func TestDecodeExemptionReadsBackWhatEncodeExemptionStored(t *testing.T) {
	c := candidate(t)
	data, err := review.EncodeExemption(c, "none", []byte(reportWithMarkup))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := review.DecodeExemption(data)

	if err != nil {
		t.Fatalf("DecodeExemption() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(got.Binding, c.Binding()) || got.Risk != "none" {
		t.Errorf("DecodeExemption() = binding %+v, risk %q; want %+v and none", got.Binding, got.Risk, c.Binding())
	}
	var back, given any
	if err := json.Unmarshal(got.Report, &back); err != nil {
		t.Fatalf("the report read back is not JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(reportWithMarkup), &given); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if !reflect.DeepEqual(back, given) {
		t.Errorf("the report read back is %s, want %s", got.Report, reportWithMarkup)
	}
}

// A file that is not one EncodeExemption writes is refused: it is not one JSON object, it writes a key twice, it has
// a member EncodeExemption does not write at any level, it is of another version, or it lacks the declaration or the
// report the exemption was made on. A file that holds a result is not an exemption.
func TestDecodeExemptionRefusesAFileThatIsNotOneEncodeExemptionWrites(t *testing.T) {
	good := string(mustEncodeExemption(t))
	for name, data := range map[string]string{
		"an empty file":                   "",
		"prose":                           "an exemption",
		"an array":                        "[]",
		"null":                            "null",
		"two objects":                     good + good,
		"a key written twice":             `{"version":1,"version":1,"binding":{},"risk":"none","report":{}}`,
		"a member it does not write":      strings.Replace(good, `"risk"`, `"note": 1, "risk"`, 1),
		"a member of the binding":         strings.Replace(good, `"base"`, `"oops": 1, "base"`, 1),
		"another version":                 strings.Replace(good, `"version": 1`, `"version": 2`, 1),
		"no version":                      strings.Replace(good, `"version": 1,`, ``, 1),
		"no declaration":                  strings.Replace(good, `"risk": "none",`, ``, 1),
		"an empty declaration":            strings.Replace(good, `"risk": "none"`, `"risk": ""`, 1),
		"no report":                       `{"version":1,"binding":{"base":"","head":"","diff_sha256":"","paths":[]},"risk":"none"}`,
		"a null report":                   `{"version":1,"binding":{"base":"","head":"","diff_sha256":"","paths":[]},"risk":"none","report":null}`,
		"the file of a stored result":     string(mustEncodeResult(t)),
		"bytes that are not UTF-8":        good + "\xff",
		"a risk that is not a string":     strings.Replace(good, `"risk": "none"`, `"risk": 1`, 1),
		"a binding that is not an object": `{"version":1,"binding":[],"risk":"none","report":{}}`,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := review.DecodeExemption([]byte(data))

			if !errors.Is(err, review.ErrInvalidStored) {
				t.Errorf("DecodeExemption() = %+v, %v; want an error wrapping ErrInvalidStored", got, err)
			}
			if !reflect.DeepEqual(got, review.StoredExemption{}) {
				t.Errorf("DecodeExemption() = %+v, want the zero value from a refused file", got)
			}
		})
	}
}

// The file of an exemption is not a result: the reader of a stored result refuses it, whatever else it holds.
func TestDecodeResultRefusesTheFileOfAnExemption(t *testing.T) {
	if got, err := review.DecodeResult(mustEncodeExemption(t)); !errors.Is(err, review.ErrInvalidStored) {
		t.Errorf("DecodeResult(an exemption) = %+v, %v; want an error wrapping ErrInvalidStored", got, err)
	}
}

func mustEncodeExemption(t *testing.T) []byte {
	t.Helper()
	data, err := review.EncodeExemption(candidate(t), "none", []byte(reportWithMarkup))
	if err != nil {
		t.Fatalf("setup: EncodeExemption() error = %v", err)
	}
	return data
}

func mustEncodeResult(t *testing.T) []byte {
	t.Helper()
	data, err := review.EncodeResult(candidate(t), sentRows(), []byte(replyText))
	if err != nil {
		t.Fatalf("setup: EncodeResult() error = %v", err)
	}
	return data
}

// The skeleton of the record that closes an exemption fills every member the command knows: the delivery as next,
// the kind of the payload and its digest, the two pieces of evidence that bind the range and the stored file, and
// that it ran. It leaves from_role empty: the exemption skips the reviewer, so the role the chain is at (builder,
// sweeper or polisher) is the parent's to fill.
func TestExemptionHandoffFillsWhatTheCommandKnows(t *testing.T) {
	got := review.ExemptionHandoff(candidate(t), exemptionRef, storedSHA)

	if got.Version != roles.HandoffVersion || got.FromRole != "" || got.ToRole != roles.RoleDelivery || got.Status != roles.StatusCompleted {
		t.Errorf("version %d, from_role %q, to_role %q, status %q; want %d, %q, %q, %q",
			got.Version, got.FromRole, got.ToRole, got.Status, roles.HandoffVersion, "", roles.RoleDelivery, roles.StatusCompleted)
	}
	if got.PayloadKind != "review-exemption" || review.PayloadExemption != roles.PayloadExemption || roles.PayloadExemption != "review-exemption" {
		t.Errorf("payload_kind = %q (PayloadExemption %q, roles.PayloadExemption %q), want review-exemption", got.PayloadKind, review.PayloadExemption, roles.PayloadExemption)
	}
	if got.PayloadSHA256 != storedSHA {
		t.Errorf("payload_sha256 = %q, want the digest of the stored file %q", got.PayloadSHA256, storedSHA)
	}
	wantEvidence := []roles.Evidence{
		{Kind: "review-diff", Ref: baseSHA + ".." + headSHA, SHA256: diffSHA},
		{Kind: "review-exemption", Ref: exemptionRef, SHA256: storedSHA},
	}
	if !reflect.DeepEqual(got.Evidence, wantEvidence) || review.EvidenceExemption != "review-exemption" {
		t.Errorf("evidence = %+v (EvidenceExemption %q), want %+v", got.Evidence, review.EvidenceExemption, wantEvidence)
	}
	if got.ResumeReason != nil {
		t.Errorf("resume_reason = %q, want it absent on a completed record", *got.ResumeReason)
	}
	if got.Context.Decisions == nil || got.Context.OpenQuestions == nil {
		t.Errorf("context = %+v, want decisions and open_questions as empty arrays, not null", got.Context)
	}
}

// completedExemption is the skeleton as the parent completes it: JSON, with the members of values set and every
// other member as the command printed it. from is the role the chain is at, which the parent writes as from_role.
// skip names one member to leave as printed.
func completedExemption(t *testing.T, from roles.Role, skip string) []byte {
	t.Helper()
	data, err := json.Marshal(review.ExemptionHandoff(candidate(t), exemptionRef, storedSHA))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("setup: %v", err)
	}
	for name, value := range parentValues("delivery") {
		if name == skip || name == "to_role" {
			continue
		}
		if name == "context.summary" {
			record["context"].(map[string]any)["summary"] = value
			continue
		}
		record[name] = value
	}
	if skip != "from_role" {
		record["from_role"] = string(from)
	}
	out, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return out
}

// Once the parent fills the members the exemption leaves to it, `roles validate` and `roles append` accept the
// record: the engine has filled to_role, because an exemption always goes to the delivery.
func TestExemptionHandoffIsAcceptedOnceTheParentFillsItsMembers(t *testing.T) {
	for _, from := range []roles.Role{roles.RoleBuilder, roles.RoleSweeper, roles.RolePolisher, roles.RoleReviewer} {
		t.Run(string(from), func(t *testing.T) {
			record := completedExemption(t, from, "")

			h, err := roles.ParseRoleHandoff(record)

			if err != nil {
				t.Fatalf("ParseRoleHandoff() error = %v for\n%s", err, record)
			}
			if h.FromRole != from || h.ToRole != roles.RoleDelivery || h.PayloadKind != "review-exemption" || len(h.Evidence) != 2 {
				t.Errorf("parsed record: from_role %q, to_role %q, payload_kind %q, %d evidence; want %q, delivery, review-exemption, 2",
					h.FromRole, h.ToRole, h.PayloadKind, len(h.Evidence), from)
			}
		})
	}
}

// Each member the parent fills is one the record needs: left as printed, the record is refused and the refusal
// names it. to_role is not among them, because the skeleton prints it; from_role is, because the skeleton leaves
// it empty.
func TestExemptionHandoffIsRefusedWhileAnyMemberOfTheParentIsLeftEmpty(t *testing.T) {
	for _, field := range append([]string{"from_role"}, review.ParentFields()...) {
		if field == "to_role" {
			continue
		}
		t.Run(field, func(t *testing.T) {
			record := completedExemption(t, roles.RoleBuilder, field)

			_, err := roles.ParseRoleHandoff(record)

			if err == nil {
				t.Fatalf("ParseRoleHandoff() accepted a record with %s left empty:\n%s", field, record)
			}
			name := field[strings.LastIndex(field, ".")+1:]
			if !strings.Contains(err.Error(), name) {
				t.Errorf("ParseRoleHandoff() error = %q, want it to name %q", err, name)
			}
		})
	}
}
