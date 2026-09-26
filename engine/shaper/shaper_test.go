package shaper

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const validHandoffJSON = `{"version":1,"project_id":"standalone-shaper-handoff","goal_id":"goal-alpha","architecture":"Layered CLI with a shared jsonstrict wire gate.","stages":["Extract jsonstrict.","Refactor goal.Parse onto jsonstrict."],"acceptance":["go test ./... passes."],"out_of_scope":["Runtime clearance UI."]}`

func documentWith(t *testing.T, changes map[string]any) []byte {
	t.Helper()
	var fields map[string]any
	if err := json.Unmarshal([]byte(validHandoffJSON), &fields); err != nil {
		t.Fatalf("decode valid test document: %v", err)
	}
	for key, value := range changes {
		fields[key] = value
	}
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("encode test document: %v", err)
	}
	return data
}

func documentWithout(t *testing.T, field string) []byte {
	t.Helper()
	var fields map[string]any
	if err := json.Unmarshal([]byte(validHandoffJSON), &fields); err != nil {
		t.Fatalf("decode valid test document: %v", err)
	}
	delete(fields, field)
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("encode test document: %v", err)
	}
	return data
}

func sampleHandoff() Handoff {
	return Handoff{
		Version:      1,
		ProjectID:    "standalone-shaper-handoff",
		GoalID:       "goal-alpha",
		Architecture: "Layered CLI with a shared jsonstrict wire gate.",
		Stages:       []string{"Extract jsonstrict.", "Refactor goal.Parse onto jsonstrict."},
		Acceptance:   []string{"go test ./... passes."},
		OutOfScope:   []string{"Runtime clearance UI."},
	}
}

func TestParseValidHandoffPreservesAuthoredValues(t *testing.T) {
	data := documentWith(t, map[string]any{
		"project_id":   "  project-x  ",
		"architecture": "  Keep this wording unchanged.\n",
		"stages":       []string{"  preserve this item  "},
	})
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse(valid handoff): %v", err)
	}
	if got.ProjectID != "  project-x  " || got.Architecture != "  Keep this wording unchanged.\n" {
		t.Errorf("Parse rewrote authored strings: project_id=%q architecture=%q", got.ProjectID, got.Architecture)
	}
	if !reflect.DeepEqual(got.Stages, []string{"  preserve this item  "}) {
		t.Errorf("Parse rewrote authored array items: %#v", got.Stages)
	}
}

func TestParseRejectsInvalidDocumentsWithoutPartialHandoff(t *testing.T) {
	duplicate := strings.Replace(validHandoffJSON, `"version":1`, `"version":1,"version":1`, 1)
	caseVariantField := documentWith(t, map[string]any{"PROJECT_ID": "case-variant"})
	invalidUTF8 := []byte(validHandoffJSON)
	invalidUTF8[strings.Index(string(invalidUTF8), "Layered")] = 0xff
	cases := []struct {
		name string
		data []byte
	}{
		{name: "malformed", data: []byte(`{"version":`)},
		{name: "invalid UTF-8", data: invalidUTF8},
		{name: "trailing object", data: []byte(validHandoffJSON + ` {}`)},
		{name: "trailing bytes", data: []byte(validHandoffJSON + "\nnot-json")},
		{name: "blank array item", data: documentWith(t, map[string]any{"stages": []string{" \t "}})},
		{name: "duplicate key", data: []byte(duplicate)},
		{name: "unknown field", data: documentWith(t, map[string]any{"unexpected": true})},
		{name: "case-variant field", data: caseVariantField},
		{name: "non-object root", data: []byte(`[]`)},
		{name: "wrong field type", data: documentWith(t, map[string]any{"architecture": 17})},
		{name: "unsupported version", data: documentWith(t, map[string]any{"version": 2})},
		{name: "missing required field", data: documentWithout(t, "project_id")},
		{name: "null array", data: documentWith(t, map[string]any{"stages": nil})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.data)
			if err == nil {
				t.Fatalf("Parse accepted invalid document")
			}
			if !reflect.DeepEqual(got, Handoff{}) {
				t.Errorf("Parse returned partial Handoff on error: %#v", got)
			}
		})
	}
}

func TestParseRejectsDuplicateKeysAtEveryObjectDepth(t *testing.T) {
	nested := strings.TrimSuffix(validHandoffJSON, "}") + `,"unexpected":{"key":1,"key":2}}`
	escapedEquivalent := strings.Replace(validHandoffJSON, `"version":1`, `"version":1,"version":1`, 1)
	cases := []struct {
		name string
		data []byte
		key  string
	}{
		{name: "nested object", data: []byte(nested), key: "key"},
		{name: "escaped equivalent field name", data: []byte(escapedEquivalent), key: "version"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.data)
			if err == nil || !strings.Contains(err.Error(), `duplicate JSON object key "`+tc.key+`"`) {
				t.Errorf("Parse error = %v, want duplicate-key refusal for %q", err, tc.key)
			}
		})
	}
}

func TestValidateRejectsInvalidHandoffValues(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Handoff)
	}{
		{name: "unsupported version", mutate: func(h *Handoff) { h.Version = 2 }},
		{name: "blank project id", mutate: func(h *Handoff) { h.ProjectID = " \t " }},
		{name: "blank goal id", mutate: func(h *Handoff) { h.GoalID = "" }},
		{name: "blank architecture", mutate: func(h *Handoff) { h.Architecture = " " }},
		{name: "nil stages", mutate: func(h *Handoff) { h.Stages = nil }},
		{name: "empty stages", mutate: func(h *Handoff) { h.Stages = []string{} }},
		{name: "blank stage item", mutate: func(h *Handoff) { h.Stages = []string{"\t"} }},
		{name: "nil acceptance", mutate: func(h *Handoff) { h.Acceptance = nil }},
		{name: "empty acceptance", mutate: func(h *Handoff) { h.Acceptance = []string{} }},
		{name: "blank acceptance item", mutate: func(h *Handoff) { h.Acceptance = []string{"\n"} }},
		{name: "nil out of scope", mutate: func(h *Handoff) { h.OutOfScope = nil }},
		{name: "blank out of scope item", mutate: func(h *Handoff) { h.OutOfScope = []string{" "} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := sampleHandoff()
			tc.mutate(&h)
			if err := h.Validate(); err == nil {
				t.Errorf("Validate accepted %s", tc.name)
			}
		})
	}
}

func TestValidateAcceptsEmptyOutOfScope(t *testing.T) {
	h := sampleHandoff()
	h.OutOfScope = []string{}
	if err := h.Validate(); err != nil {
		t.Errorf("Validate rejected permitted empty out_of_scope: %v", err)
	}
}

func TestValidateRejectsExactOverlapBetweenStagesAndOutOfScope(t *testing.T) {
	h := sampleHandoff()
	h.Stages = []string{"Ship the CLI.", "Add remote review clearance."}
	h.OutOfScope = []string{"Add remote review clearance."}
	err := h.Validate()
	if err == nil {
		t.Fatal("Validate accepted a stage byte-identical to an out_of_scope item")
	}
	if !strings.Contains(err.Error(), "stages") || !strings.Contains(err.Error(), "2") || !strings.Contains(err.Error(), "Add remote review clearance.") {
		t.Errorf("Validate error = %v, want field name, 1-based index, and conflicting value", err)
	}
}

func TestValidateRejectsExactOverlapBetweenAcceptanceAndOutOfScope(t *testing.T) {
	h := sampleHandoff()
	h.Acceptance = []string{"Tests pass.", "Remote review is granted."}
	h.OutOfScope = []string{"Remote review is granted."}
	err := h.Validate()
	if err == nil {
		t.Fatal("Validate accepted an acceptance item byte-identical to an out_of_scope item")
	}
	if !strings.Contains(err.Error(), "acceptance") || !strings.Contains(err.Error(), "2") || !strings.Contains(err.Error(), "Remote review is granted.") {
		t.Errorf("Validate error = %v, want field name, 1-based index, and conflicting value", err)
	}
}

func TestValidateOverlapRuleAppliesNoNormalization(t *testing.T) {
	cases := []struct {
		name  string
		stage string
		scope string
	}{
		{name: "leading whitespace differs", stage: "a", scope: " a"},
		{name: "case differs", stage: "a", scope: "A"},
		{name: "trailing whitespace differs", stage: "step one", scope: "step one "},
		{name: "substring is not overlap", stage: "deploy to production", scope: "production"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := sampleHandoff()
			h.Stages = []string{tc.stage}
			h.OutOfScope = []string{tc.scope}
			if err := h.Validate(); err != nil {
				t.Errorf("Validate rejected non-identical values %q vs %q: %v", tc.stage, tc.scope, err)
			}
		})
	}
}

func TestVersionOneFixtureParsesToExpectedHandoff(t *testing.T) {
	got, err := Parse([]byte(validHandoffJSON))
	if err != nil {
		t.Fatalf("Parse(valid handoff): %v", err)
	}
	if !reflect.DeepEqual(got, sampleHandoff()) {
		t.Errorf("Parse(valid handoff) = %#v, want %#v", got, sampleHandoff())
	}
}
