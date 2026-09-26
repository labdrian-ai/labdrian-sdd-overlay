package shaper

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// validHandoffV3JSON is a structurally valid handoff version 3: version 2
// plus roles, tests, risks, estimates, memory_scope, and delivery_limit.
const validHandoffV3JSON = `{"version":3,"project_id":"standalone-shaper-handoff","goal_id":"goal-alpha",` +
	`"architecture":"Layered CLI with a shared jsonstrict wire gate.",` +
	`"stages":["Extract jsonstrict.","Refactor goal.Parse onto jsonstrict."],` +
	`"acceptance":[` +
	`{"criterion":"go test ./... passes.","verification":{"check":"cd engine && go test -count=1 ./..."}},` +
	`{"criterion":"The CLI help reads clearly.","verification":{"adjudication":"A maintainer judges whether the help text names every verb."}}` +
	`],"out_of_scope":["Runtime clearance UI."],` +
	`"roles":[{"role":"implementer","responsibility":"Writes the parsing and validation code."},` +
	`{"role":"reviewer","responsibility":"Checks the diff for correctness."}],` +
	`"tests":["go test ./... passes.","staticcheck ./... is clean."],` +
	`"risks":[{"risk":"Nested key casing drifts.","mitigation":"Reuse checkAcceptanceV2Fields."}],` +
	`"estimates":[` +
	`{"stage":"Extract jsonstrict.","low_minutes":10,"high_minutes":20},` +
	`{"stage":"Refactor goal.Parse onto jsonstrict.","low_minutes":15,"high_minutes":30}` +
	`],"memory_scope":"Project-scoped.","delivery_limit":"No delivery without human clearance."}`

// v3DocumentWith returns validHandoffV3JSON with top-level fields replaced.
func v3DocumentWith(t *testing.T, changes map[string]any) []byte {
	t.Helper()
	var fields map[string]any
	if err := json.Unmarshal([]byte(validHandoffV3JSON), &fields); err != nil {
		t.Fatalf("unmarshal v3 fixture: %v", err)
	}
	for k, v := range changes {
		fields[k] = v
	}
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("marshal v3 fixture: %v", err)
	}
	return data
}

// v3WithRaw returns validHandoffV3JSON with the raw value of one top-level
// array field replaced verbatim, so tests control key case, nulls, and
// duplicates inside nested objects.
func v3WithRaw(t *testing.T, field, rawValue string) []byte {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(validHandoffV3JSON), &fields); err != nil {
		t.Fatalf("unmarshal v3 fixture: %v", err)
	}
	fields[field] = json.RawMessage(rawValue)
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("marshal v3 fixture: %v", err)
	}
	return data
}

func sampleHandoffV3() Handoff {
	return Handoff{
		Version:      3,
		ProjectID:    "p",
		GoalID:       "g",
		Architecture: "a",
		Stages:       []string{"s1", "s2"},
		AcceptanceItems: []AcceptanceItem{
			{Criterion: "c", Verification: AcceptanceVerification{Check: "go test"}},
		},
		OutOfScope: []string{},
		Roles: []Role{
			{Role: "implementer", Responsibility: "does the work"},
		},
		Tests: []string{"go test ./..."},
		Risks: []Risk{},
		Estimates: []Estimate{
			{Stage: "s1", LowMinutes: 5, HighMinutes: 10},
			{Stage: "s2", LowMinutes: 5, HighMinutes: 10},
		},
		MemoryScope:   "scoped",
		DeliveryLimit: "no delivery",
	}
}

func TestParseV3AcceptsCompleteHandoff(t *testing.T) {
	got, err := Parse([]byte(validHandoffV3JSON))
	if err != nil {
		t.Fatalf("Parse(valid v3): %v", err)
	}
	if got.Version != 3 {
		t.Errorf("Version = %d, want 3", got.Version)
	}
	wantRoles := []Role{
		{Role: "implementer", Responsibility: "Writes the parsing and validation code."},
		{Role: "reviewer", Responsibility: "Checks the diff for correctness."},
	}
	if !reflect.DeepEqual(got.Roles, wantRoles) {
		t.Errorf("Roles = %#v\nwant %#v", got.Roles, wantRoles)
	}
	wantTests := []string{"go test ./... passes.", "staticcheck ./... is clean."}
	if !reflect.DeepEqual(got.Tests, wantTests) {
		t.Errorf("Tests = %#v\nwant %#v", got.Tests, wantTests)
	}
	wantRisks := []Risk{{Risk: "Nested key casing drifts.", Mitigation: "Reuse checkAcceptanceV2Fields."}}
	if !reflect.DeepEqual(got.Risks, wantRisks) {
		t.Errorf("Risks = %#v\nwant %#v", got.Risks, wantRisks)
	}
	wantEstimates := []Estimate{
		{Stage: "Extract jsonstrict.", LowMinutes: 10, HighMinutes: 20},
		{Stage: "Refactor goal.Parse onto jsonstrict.", LowMinutes: 15, HighMinutes: 30},
	}
	if !reflect.DeepEqual(got.Estimates, wantEstimates) {
		t.Errorf("Estimates = %#v\nwant %#v", got.Estimates, wantEstimates)
	}
	if got.MemoryScope != "Project-scoped." {
		t.Errorf("MemoryScope = %q", got.MemoryScope)
	}
	if got.DeliveryLimit != "No delivery without human clearance." {
		t.Errorf("DeliveryLimit = %q", got.DeliveryLimit)
	}
	if got.AcceptanceItems == nil || got.Acceptance != nil {
		t.Errorf("v3 acceptance shape wrong: items=%#v strings=%#v", got.AcceptanceItems, got.Acceptance)
	}
}

func TestParseV3PreservesAuthoredStrings(t *testing.T) {
	data := v3DocumentWith(t, map[string]any{"memory_scope": "  keep  ", "delivery_limit": "  also keep  "})
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.MemoryScope != "  keep  " || got.DeliveryLimit != "  also keep  " {
		t.Errorf("Parse rewrote authored v3 strings: %#v", got)
	}
}

func TestParseV1AndV2LeaveV3FieldsZero(t *testing.T) {
	v1, err := Parse([]byte(validHandoffJSON))
	if err != nil {
		t.Fatalf("Parse(v1): %v", err)
	}
	if v1.Roles != nil || v1.Tests != nil || v1.Risks != nil || v1.Estimates != nil || v1.MemoryScope != "" || v1.DeliveryLimit != "" {
		t.Errorf("v1 handoff carries v3 fields: %#v", v1)
	}
	v2, err := Parse([]byte(validHandoffV2JSON))
	if err != nil {
		t.Fatalf("Parse(v2): %v", err)
	}
	if v2.Roles != nil || v2.Tests != nil || v2.Risks != nil || v2.Estimates != nil || v2.MemoryScope != "" || v2.DeliveryLimit != "" {
		t.Errorf("v2 handoff carries v3 fields: %#v", v2)
	}
}

func TestParseRejectsV3FieldsOnV1AndV2Documents(t *testing.T) {
	v1WithRoles := strings.Replace(validHandoffJSON, `"out_of_scope"`, `"roles":[],"out_of_scope"`, 1)
	v2WithRoles := strings.Replace(validHandoffV2JSON, `"out_of_scope"`, `"roles":[],"out_of_scope"`, 1)
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{name: "v1 with roles", data: []byte(v1WithRoles)},
		{name: "v2 with roles", data: []byte(v2WithRoles)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.data)
			if err == nil || !strings.Contains(err.Error(), "unknown") {
				t.Errorf("Parse error = %v, want an unknown-field refusal", err)
			}
		})
	}
}

func TestParseV3RejectsInvalidNestedFieldsWithoutPartialHandoff(t *testing.T) {
	cases := []struct {
		name  string
		data  []byte
		match string
	}{
		{name: "null roles", data: v3WithRaw(t, "roles", `null`), match: "roles"},
		{name: "empty roles", data: v3WithRaw(t, "roles", `[]`), match: "roles"},
		{name: "null role item", data: v3WithRaw(t, "roles", `[null]`), match: "roles"},
		{name: "blank role", data: v3WithRaw(t, "roles", `[{"role":" ","responsibility":"x"}]`), match: "role"},
		{name: "blank responsibility", data: v3WithRaw(t, "roles", `[{"role":"a","responsibility":" "}]`), match: "responsibility"},
		{name: "duplicate role", data: v3WithRaw(t, "roles", `[{"role":"a","responsibility":"x"},{"role":"a","responsibility":"y"}]`), match: "duplicate"},
		{name: "unknown role key", data: v3WithRaw(t, "roles", `[{"role":"a","responsibility":"x","seniority":"senior"}]`), match: "unknown"},
		{name: "case-variant role key", data: v3WithRaw(t, "roles", `[{"Role":"a","responsibility":"x"}]`), match: "unknown"},

		{name: "null tests", data: v3WithRaw(t, "tests", `null`), match: "tests"},
		{name: "empty tests", data: v3WithRaw(t, "tests", `[]`), match: "tests"},
		{name: "blank test", data: v3WithRaw(t, "tests", `[" "]`), match: "tests"},

		{name: "null risks", data: v3WithRaw(t, "risks", `null`), match: "risks"},
		{name: "null risk item", data: v3WithRaw(t, "risks", `[null]`), match: "risks"},
		{name: "blank risk", data: v3WithRaw(t, "risks", `[{"risk":" ","mitigation":"x"}]`), match: "risk"},
		{name: "blank mitigation", data: v3WithRaw(t, "risks", `[{"risk":"x","mitigation":" "}]`), match: "mitigation"},
		{name: "unknown risk key", data: v3WithRaw(t, "risks", `[{"risk":"x","mitigation":"y","owner":"z"}]`), match: "unknown"},

		{name: "null estimates", data: v3WithRaw(t, "estimates", `null`), match: "estimates"},
		{name: "missing estimate", data: v3WithRaw(t, "estimates", `[{"stage":"Extract jsonstrict.","low_minutes":1,"high_minutes":2}]`), match: "estimates"},
		{name: "duplicate stage estimate", data: v3WithRaw(t, "estimates", `[{"stage":"Extract jsonstrict.","low_minutes":1,"high_minutes":2},{"stage":"Extract jsonstrict.","low_minutes":1,"high_minutes":2}]`), match: "duplicat"},
		{name: "unknown stage estimate", data: v3WithRaw(t, "estimates", `[{"stage":"Extract jsonstrict.","low_minutes":1,"high_minutes":2},{"stage":"Unknown stage.","low_minutes":1,"high_minutes":2}]`), match: "does not match"},
		{name: "low greater than high", data: v3WithRaw(t, "estimates", `[{"stage":"Extract jsonstrict.","low_minutes":20,"high_minutes":10},{"stage":"Refactor goal.Parse onto jsonstrict.","low_minutes":1,"high_minutes":2}]`), match: "<="},
		{name: "zero low_minutes", data: v3WithRaw(t, "estimates", `[{"stage":"Extract jsonstrict.","low_minutes":0,"high_minutes":10},{"stage":"Refactor goal.Parse onto jsonstrict.","low_minutes":1,"high_minutes":2}]`), match: "positive"},
		{name: "negative high_minutes", data: v3WithRaw(t, "estimates", `[{"stage":"Extract jsonstrict.","low_minutes":1,"high_minutes":-5},{"stage":"Refactor goal.Parse onto jsonstrict.","low_minutes":1,"high_minutes":2}]`), match: "positive"},
		{name: "unknown estimate key", data: v3WithRaw(t, "estimates", `[{"stage":"Extract jsonstrict.","low_minutes":1,"high_minutes":2,"confidence":"high"},{"stage":"Refactor goal.Parse onto jsonstrict.","low_minutes":1,"high_minutes":2}]`), match: "unknown"},

		{name: "blank memory_scope", data: v3DocumentWith(t, map[string]any{"memory_scope": " "}), match: "memory_scope"},
		{name: "blank delivery_limit", data: v3DocumentWith(t, map[string]any{"delivery_limit": " "}), match: "delivery_limit"},

		{name: "test overlaps out_of_scope", data: v3DocumentWith(t, map[string]any{"out_of_scope": []string{"go test ./... passes."}}), match: "byte-identical"},
		{name: "role responsibility overlaps out_of_scope", data: v3DocumentWith(t, map[string]any{"out_of_scope": []string{"Writes the parsing and validation code."}}), match: "byte-identical"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.data)
			if err == nil {
				t.Fatalf("Parse accepted invalid v3 document %s", tc.data)
			}
			if !reflect.DeepEqual(got, Handoff{}) {
				t.Errorf("Parse returned partial Handoff on error: %#v", got)
			}
			if tc.match != "" && !strings.Contains(err.Error(), tc.match) {
				t.Errorf("error %q does not name %q", err, tc.match)
			}
		})
	}
}

func TestValidateV3(t *testing.T) {
	if err := sampleHandoffV3().Validate(); err != nil {
		t.Fatalf("Validate(valid v3): %v", err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Handoff)
	}{
		{name: "nil roles", mutate: func(h *Handoff) { h.Roles = nil }},
		{name: "empty roles", mutate: func(h *Handoff) { h.Roles = []Role{} }},
		{name: "duplicate role", mutate: func(h *Handoff) { h.Roles = append(h.Roles, h.Roles[0]) }},
		{name: "nil tests", mutate: func(h *Handoff) { h.Tests = nil }},
		{name: "empty tests", mutate: func(h *Handoff) { h.Tests = []string{} }},
		{name: "nil risks", mutate: func(h *Handoff) { h.Risks = nil }},
		{name: "nil estimates", mutate: func(h *Handoff) { h.Estimates = nil }},
		{name: "estimate count mismatch", mutate: func(h *Handoff) { h.Estimates = h.Estimates[:1] }},
		{name: "estimate unknown stage", mutate: func(h *Handoff) { h.Estimates[0].Stage = "unknown" }},
		{name: "estimate low > high", mutate: func(h *Handoff) { h.Estimates[0].LowMinutes = 999 }},
		{name: "blank memory_scope", mutate: func(h *Handoff) { h.MemoryScope = " " }},
		{name: "blank delivery_limit", mutate: func(h *Handoff) { h.DeliveryLimit = " " }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := sampleHandoffV3()
			h.Roles = append([]Role(nil), h.Roles...)
			h.Estimates = append([]Estimate(nil), h.Estimates...)
			tc.mutate(&h)
			if err := h.Validate(); err == nil {
				t.Errorf("Validate accepted %s", tc.name)
			}
		})
	}
}

func TestValidateV3RiskEntriesMayBeEmptyButNotNull(t *testing.T) {
	h := sampleHandoffV3()
	h.Risks = []Risk{}
	if err := h.Validate(); err != nil {
		t.Errorf("Validate rejected empty risks: %v", err)
	}
	h.Risks = nil
	if err := h.Validate(); err == nil {
		t.Errorf("Validate accepted nil risks")
	}
}
