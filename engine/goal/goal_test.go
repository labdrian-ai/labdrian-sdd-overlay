package goal

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const validGoalJSON = `{"version":1,"project_id":"standalone-goal-contract","objective":"Represent user intent before workflow shaping.","scope":"One explicitly identified project.","constraints":["Use only structural deterministic validation."],"non_goals":["Grant execution authority."],"acceptance_criteria":["The contract is versioned and can be parsed strictly."],"memory_scope":"Project-scoped memory may be consulted later.","runtime_scope":"Runtime integration is decided later.","delivery_boundary":"No delivery operation is authorized."}`

func documentWith(t *testing.T, changes map[string]any) []byte {
	t.Helper()
	var fields map[string]any
	if err := json.Unmarshal([]byte(validGoalJSON), &fields); err != nil {
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
	if err := json.Unmarshal([]byte(validGoalJSON), &fields); err != nil {
		t.Fatalf("decode valid test document: %v", err)
	}
	delete(fields, field)
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("encode test document: %v", err)
	}
	return data
}

func sampleGoal() Goal {
	return Goal{
		Version:            1,
		ProjectID:          "standalone-goal-contract",
		Objective:          "Represent user intent before workflow shaping.",
		Scope:              "One explicitly identified project.",
		Constraints:        []string{"Use only structural deterministic validation."},
		NonGoals:           []string{"Grant execution authority."},
		AcceptanceCriteria: []string{"The contract is versioned and can be parsed strictly."},
		MemoryScope:        "Project-scoped memory may be consulted later.",
		RuntimeScope:       "Runtime integration is decided later.",
		DeliveryBoundary:   "No delivery operation is authorized.",
	}
}

func TestParseValidGoalPreservesAuthoredValues(t *testing.T) {
	data := documentWith(t, map[string]any{
		"project_id":  "  project-x  ",
		"objective":   "  Keep this wording unchanged.\n",
		"constraints": []string{"  preserve this item  "},
	})
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse(valid goal): %v", err)
	}
	if got.ProjectID != "  project-x  " || got.Objective != "  Keep this wording unchanged.\n" {
		t.Errorf("Parse rewrote authored strings: project_id=%q objective=%q", got.ProjectID, got.Objective)
	}
	if !reflect.DeepEqual(got.Constraints, []string{"  preserve this item  "}) {
		t.Errorf("Parse rewrote authored array items: %#v", got.Constraints)
	}
}

func TestParseRejectsInvalidDocumentsWithoutPartialGoal(t *testing.T) {
	duplicate := strings.Replace(validGoalJSON, `"version":1`, `"version":1,"version":1`, 1)
	caseVariantField := documentWith(t, map[string]any{"PROJECT_ID": "case-variant"})
	invalidUTF8 := []byte(validGoalJSON)
	invalidUTF8[strings.Index(string(invalidUTF8), "Represent")] = 0xff
	cases := []struct {
		name string
		data []byte
	}{
		{name: "malformed", data: []byte(`{"version":`)},
		{name: "invalid UTF-8", data: invalidUTF8},
		{name: "trailing object", data: []byte(validGoalJSON + ` {}`)},
		{name: "trailing bytes", data: []byte(validGoalJSON + "\nnot-json")},
		{name: "blank array item", data: documentWith(t, map[string]any{"constraints": []string{" \t "}})},
		{name: "duplicate key", data: []byte(duplicate)},
		{name: "unknown field", data: documentWith(t, map[string]any{"unexpected": true})},
		{name: "case-variant field", data: caseVariantField},
		{name: "non-object root", data: []byte(`[]`)},
		{name: "wrong field type", data: documentWith(t, map[string]any{"objective": 17})},
		{name: "unsupported version", data: documentWith(t, map[string]any{"version": 2})},
		{name: "missing required field", data: documentWithout(t, "project_id")},
		{name: "null array", data: documentWith(t, map[string]any{"constraints": nil})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.data)
			if err == nil {
				t.Fatalf("Parse accepted invalid document")
			}
			if !reflect.DeepEqual(got, Goal{}) {
				t.Errorf("Parse returned partial Goal on error: %#v", got)
			}
		})
	}
}

func TestParseRejectsDuplicateKeysAtEveryObjectDepth(t *testing.T) {
	nested := strings.TrimSuffix(validGoalJSON, "}") + `,"unexpected":{"key":1,"key":2}}`
	escapedEquivalent := strings.Replace(validGoalJSON, `"version":1`, `"version":1,"vers\u0069on":1`, 1)
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

func TestValidateRejectsInvalidGoalValues(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Goal)
	}{
		{name: "unsupported version", mutate: func(g *Goal) { g.Version = 2 }},
		{name: "blank project id", mutate: func(g *Goal) { g.ProjectID = " \t " }},
		{name: "blank objective", mutate: func(g *Goal) { g.Objective = "" }},
		{name: "blank scope", mutate: func(g *Goal) { g.Scope = " " }},
		{name: "blank memory scope", mutate: func(g *Goal) { g.MemoryScope = "\n" }},
		{name: "blank runtime scope", mutate: func(g *Goal) { g.RuntimeScope = "\t" }},
		{name: "blank delivery boundary", mutate: func(g *Goal) { g.DeliveryBoundary = " " }},
		{name: "nil constraints", mutate: func(g *Goal) { g.Constraints = nil }},
		{name: "nil non-goals", mutate: func(g *Goal) { g.NonGoals = nil }},
		{name: "nil acceptance criteria", mutate: func(g *Goal) { g.AcceptanceCriteria = nil }},
		{name: "empty acceptance criteria", mutate: func(g *Goal) { g.AcceptanceCriteria = []string{} }},
		{name: "blank constraint item", mutate: func(g *Goal) { g.Constraints = []string{"\t"} }},
		{name: "blank non-goal item", mutate: func(g *Goal) { g.NonGoals = []string{" "} }},
		{name: "blank acceptance item", mutate: func(g *Goal) { g.AcceptanceCriteria = []string{"\n"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := sampleGoal()
			tc.mutate(&g)
			if err := g.Validate(); err == nil {
				t.Errorf("Validate accepted %s", tc.name)
			}
		})
	}
}

func TestValidateAcceptsEmptyConstraintsAndNonGoals(t *testing.T) {
	g := sampleGoal()
	g.Constraints = []string{}
	g.NonGoals = []string{}
	if err := g.Validate(); err != nil {
		t.Errorf("Validate rejected permitted empty arrays: %v", err)
	}
}
