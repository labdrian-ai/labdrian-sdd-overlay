package review_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
)

// storedRef and storedSHA stand for a result that was stored: its path in the repository and the
// SHA-256 of the bytes of the file.
const (
	storedRef = "odd/review-results/own-rdd/222222222222-1.json"
	storedSHA = "4444444444444444444444444444444444444444444444444444444444444444"
)

// The skeleton fills every member the command knows from what it stored and recomputed: that a review
// is the reviewer's, its kind, the digest of the stored file, the two pieces of evidence that bind the
// range and the stored bytes, and that the review ran.
func TestHandoffFillsWhatTheCommandKnows(t *testing.T) {
	got := review.Handoff(candidate(t), storedRef, storedSHA)

	if got.Version != roles.HandoffVersion || got.FromRole != roles.RoleReviewer || got.Status != roles.StatusCompleted {
		t.Errorf("version %d, from_role %q, status %q; want %d, %q, %q", got.Version, got.FromRole, got.Status, roles.HandoffVersion, roles.RoleReviewer, roles.StatusCompleted)
	}
	if got.PayloadKind != "review-report" || review.PayloadKind != "review-report" {
		t.Errorf("payload_kind = %q (PayloadKind %q), want review-report", got.PayloadKind, review.PayloadKind)
	}
	if got.PayloadSHA256 != storedSHA {
		t.Errorf("payload_sha256 = %q, want the digest of the stored file %q", got.PayloadSHA256, storedSHA)
	}
	wantEvidence := []roles.Evidence{
		{Kind: "review-diff", Ref: baseSHA + ".." + headSHA, SHA256: diffSHA},
		{Kind: "review-result", Ref: storedRef, SHA256: storedSHA},
	}
	if !reflect.DeepEqual(got.Evidence, wantEvidence) {
		t.Errorf("evidence = %+v, want %+v", got.Evidence, wantEvidence)
	}
	if got.ResumeReason != nil {
		t.Errorf("resume_reason = %q, want it absent on a completed record", *got.ResumeReason)
	}
	if got.Context.Decisions == nil || got.Context.OpenQuestions == nil {
		t.Errorf("context = %+v, want decisions and open_questions as empty arrays, not null", got.Context)
	}
}

// The members only the parent knows are the chain it appends to and what it decides from the review:
// where the record goes, the narrative, and who gets the work next.
func TestParentFieldsAreTheMembersOnlyTheParentKnows(t *testing.T) {
	want := []string{"project_id", "goal_id", "chain_id", "seq", "prev_sha256", "to_role", "context.summary"}
	if got := review.ParentFields(); !reflect.DeepEqual(got, want) {
		t.Errorf("ParentFields() = %v, want %v", got, want)
	}
}

// parentValues are values for the members only the parent knows, as valid as a chain accepts.
func parentValues(toRole string) map[string]any {
	return map[string]any{
		"project_id": "labdrian-sdd-overlay", "goal_id": "own-rdd", "chain_id": "chain-1", "seq": 7,
		"prev_sha256": strings.Repeat("5", 64), "to_role": toRole, "context.summary": "the review of the range ran",
	}
}

// withParentFields is the skeleton as the parent completes it: JSON, with the members of values set and
// every other member as the command printed it. skip names one member to leave as printed.
func withParentFields(t *testing.T, values map[string]any, skip string) []byte {
	t.Helper()
	data, err := json.Marshal(review.Handoff(candidate(t), storedRef, storedSHA))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("setup: %v", err)
	}
	for name, value := range values {
		if name == skip {
			continue
		}
		if name == "context.summary" {
			record["context"].(map[string]any)["summary"] = value
			continue
		}
		record[name] = value
	}
	out, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return out
}

// Once the parent fills the members it names, `roles validate` and `roles append` accept the record,
// for either hop a review ends in.
func TestHandoffIsAcceptedOnceTheParentFillsItsMembers(t *testing.T) {
	for _, toRole := range []string{"delivery", "builder"} {
		t.Run(toRole, func(t *testing.T) {
			record := withParentFields(t, parentValues(toRole), "")
			h, err := roles.ParseRoleHandoff(record)
			if err != nil {
				t.Fatalf("ParseRoleHandoff() error = %v for\n%s", err, record)
			}
			if string(h.ToRole) != toRole || h.PayloadKind != "review-report" || len(h.Evidence) != 2 {
				t.Errorf("parsed record: to_role %q, payload_kind %q, %d evidence; want %q, review-report, 2", h.ToRole, h.PayloadKind, len(h.Evidence), toRole)
			}
		})
	}
}

// Each member ParentFields names is one the record needs, and no other: left as printed, the record is
// refused, and the refusal names that member; and the fields the skeleton fills never need the parent.
func TestHandoffIsRefusedWhileAnyMemberOfTheParentIsLeftEmpty(t *testing.T) {
	fields := review.ParentFields()
	if len(fields) == 0 {
		t.Fatal("ParentFields() is empty, so no member is checked")
	}
	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			record := withParentFields(t, parentValues("delivery"), field)
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

// The skeleton as printed is not a record a chain accepts: the parent has to fill it in.
func TestHandoffAsPrintedIsNotAcceptedByTheChain(t *testing.T) {
	data, err := json.Marshal(review.Handoff(candidate(t), storedRef, storedSHA))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := roles.ParseRoleHandoff(data); err == nil {
		t.Errorf("ParseRoleHandoff() accepted the skeleton with no member filled by the parent:\n%s", data)
	}
}
