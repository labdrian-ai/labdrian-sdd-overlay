package goal

import (
	"encoding/json"
	"os"
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

func sampleGoalV2() Goal {
	g := sampleGoal()
	g.Version = 2
	g.GoalID = "goal-alpha"
	return g
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
		{name: "unsupported version", mutate: func(g *Goal) { g.Version = 3 }},
		{name: "goal id forbidden in version one", mutate: func(g *Goal) { g.GoalID = "goal-alpha" }},
		{name: "blank version two goal id", mutate: func(g *Goal) { g.Version = 2 }},
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

func TestMarshalCanonicalBytesMatchVersionOneFixture(t *testing.T) {
	want, err := os.ReadFile("testdata/v1.json")
	if err != nil {
		t.Fatalf("read v1 fixture: %v", err)
	}
	got, err := sampleGoal().Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Marshal bytes differ from v1 fixture:\ngot:\n%s\nwant:\n%s", got, want)
	}
	if len(got) == 0 || got[len(got)-1] != '\n' || (len(got) > 1 && got[len(got)-2] == '\n') {
		t.Errorf("Marshal must end in exactly one newline; got %q", got)
	}
	orderedFields := []string{`"version"`, `"project_id"`, `"objective"`, `"scope"`, `"constraints"`, `"non_goals"`, `"acceptance_criteria"`, `"memory_scope"`, `"runtime_scope"`, `"delivery_boundary"`}
	previous := -1
	for _, field := range orderedFields {
		index := strings.Index(string(got), field)
		if index <= previous {
			t.Errorf("canonical output field %s is missing or out of order in:\n%s", field, got)
		}
		previous = index
	}
}

func TestVersionOneGoldenFixtureParsesAndRoundTrips(t *testing.T) {
	fixture, err := os.ReadFile("testdata/v1.json")
	if err != nil {
		t.Fatalf("read v1 fixture: %v", err)
	}
	got, err := Parse(fixture)
	if err != nil {
		t.Fatalf("Parse(v1 fixture): %v", err)
	}
	if !reflect.DeepEqual(got, sampleGoal()) {
		t.Errorf("Parse(v1 fixture) = %#v, want %#v", got, sampleGoal())
	}
	encoded, err := got.Marshal()
	if err != nil {
		t.Fatalf("Marshal(parsed fixture): %v", err)
	}
	if !reflect.DeepEqual(encoded, fixture) {
		t.Errorf("v1 fixture did not round-trip byte-for-byte:\ngot:\n%s\nwant:\n%s", encoded, fixture)
	}
}

func TestMarshalRejectsInvalidGoal(t *testing.T) {
	g := sampleGoal()
	g.Version = 0
	if data, err := g.Marshal(); err == nil || data != nil {
		t.Errorf("Marshal(invalid goal) = (%q, %v), want (nil, error)", data, err)
	}
}

func TestVersionTwoFixtureParsesAndMarshalsCanonically(t *testing.T) {
	fixture, err := os.ReadFile("testdata/v2.json")
	if err != nil {
		t.Fatalf("read v2 fixture: %v", err)
	}
	got, err := Parse(fixture)
	if err != nil {
		t.Fatalf("Parse(v2 fixture): %v", err)
	}
	if !reflect.DeepEqual(got, sampleGoalV2()) {
		t.Errorf("Parse(v2 fixture) = %#v, want %#v", got, sampleGoalV2())
	}
	encoded, err := got.Marshal()
	if err != nil {
		t.Fatalf("Marshal(parsed v2 fixture): %v", err)
	}
	if !reflect.DeepEqual(encoded, fixture) {
		t.Errorf("v2 fixture did not round-trip byte-for-byte:\ngot:\n%s\nwant:\n%s", encoded, fixture)
	}
}

func TestVersionTwoPreservesAuthoredGoalID(t *testing.T) {
	data := documentWith(t, map[string]any{"version": 2, "goal_id": "  goal-alpha  "})
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse(v2 goal): %v", err)
	}
	if got.GoalID != "  goal-alpha  " {
		t.Errorf("Parse rewrote authored goal_id: %q", got.GoalID)
	}
}

func TestVersionTwoRequiresCallerSuppliedGoalID(t *testing.T) {
	cases := []struct {
		name    string
		changes map[string]any
		remove  string
	}{
		{name: "missing", remove: "goal_id"},
		{name: "null", changes: map[string]any{"goal_id": nil}},
		{name: "blank", changes: map[string]any{"goal_id": " \t "}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := documentWith(t, map[string]any{"version": 2, "goal_id": "goal-alpha"})
			var fields map[string]any
			if err := json.Unmarshal(data, &fields); err != nil {
				t.Fatal(err)
			}
			if tc.remove != "" {
				delete(fields, tc.remove)
			}
			for key, value := range tc.changes {
				fields[key] = value
			}
			data, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Parse(data); err == nil {
				t.Errorf("Parse accepted %s goal_id", tc.name)
			}
		})
	}
}

func TestVersionTwoGoalIDWhitespaceBoundary(t *testing.T) {
	accepted := []struct {
		name  string
		value string
	}{
		{name: "single ascii rune", value: "g"},
		{name: "non-ascii rune", value: "g\u03a9"},
		{name: "relative path-like", value: "../x"},
		{name: "slash separated", value: "a/b"},
		{name: "surrounding whitespace kept verbatim", value: " g\t"},
	}
	for _, tc := range accepted {
		t.Run("accepts "+tc.name, func(t *testing.T) {
			got, err := Parse(documentWith(t, map[string]any{"version": 2, "goal_id": tc.value}))
			if err != nil {
				t.Fatalf("Parse(goal_id %q): %v", tc.value, err)
			}
			if got.GoalID != tc.value {
				t.Fatalf("Parse rewrote goal_id: got %q, want %q", got.GoalID, tc.value)
			}
			encoded, err := got.Marshal()
			if err != nil {
				t.Fatalf("Marshal(goal_id %q): %v", tc.value, err)
			}
			reparsed, err := Parse(encoded)
			if err != nil {
				t.Fatalf("Parse(Marshal(goal_id %q)): %v", tc.value, err)
			}
			if reparsed.GoalID != tc.value {
				t.Errorf("goal_id did not round-trip: got %q, want %q", reparsed.GoalID, tc.value)
			}
		})
	}

	rejected := []struct {
		name  string
		value string
	}{
		{name: "no-break space", value: "\u00a0"},
		{name: "ideographic space", value: "\u3000"},
		{name: "line separator", value: "\u2028"},
		{name: "next line", value: "\u0085"},
		{name: "ascii spaces and tab", value: "  \t"},
	}
	for _, tc := range rejected {
		t.Run("Parse rejects "+tc.name, func(t *testing.T) {
			if _, err := Parse(documentWith(t, map[string]any{"version": 2, "goal_id": tc.value})); err == nil {
				t.Errorf("Parse accepted whitespace-only goal_id %q", tc.value)
			}
		})
		t.Run("Validate rejects "+tc.name, func(t *testing.T) {
			g := sampleGoalV2()
			g.GoalID = tc.value
			if err := g.Validate(); err == nil {
				t.Errorf("Validate accepted whitespace-only goal_id %q", tc.value)
			}
		})
	}
}

func TestVersionTwoIdentityAndStrictFieldValidation(t *testing.T) {
	duplicateGoalID := strings.Replace(validGoalJSON, `"version":1`, `"version":2`, 1)
	duplicateGoalID = strings.TrimSuffix(duplicateGoalID, "}") + `,"goal_id":"goal-alpha","goal_id":"goal-beta"}`
	cases := []struct {
		name string
		data []byte
	}{
		{name: "unknown field", data: documentWith(t, map[string]any{"version": 2, "goal_id": "goal-alpha", "unexpected": true})},
		{name: "duplicate goal_id", data: []byte(duplicateGoalID)},
		{name: "malformed goal_id", data: documentWith(t, map[string]any{"version": 2, "goal_id": 17})},
		{name: "wrong-case goal field", data: documentWith(t, map[string]any{"version": 2, "goal_id": "goal-alpha", "GOAL_ID": "goal-beta"})},
		{name: "unsupported version", data: documentWith(t, map[string]any{"version": 3, "goal_id": "goal-alpha"})},
		{name: "v1 rejects goal_id", data: documentWith(t, map[string]any{"goal_id": "goal-alpha"})},
		{name: "v1 rejects empty goal_id", data: documentWith(t, map[string]any{"goal_id": ""})},
		{name: "v1 rejects null goal_id", data: documentWith(t, map[string]any{"goal_id": nil})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse(tc.data); err == nil {
				t.Errorf("Parse accepted %s", tc.name)
			}
		})
	}

	first := sampleGoalV2()
	second := sampleGoalV2()
	second.GoalID = "goal-beta"
	firstJSON, err := first.Marshal()
	if err != nil {
		t.Fatalf("Marshal(first): %v", err)
	}
	secondJSON, err := second.Marshal()
	if err != nil {
		t.Fatalf("Marshal(second): %v", err)
	}
	parsedFirst, err := Parse(firstJSON)
	if err != nil {
		t.Fatalf("Parse(first): %v", err)
	}
	parsedSecond, err := Parse(secondJSON)
	if err != nil {
		t.Fatalf("Parse(second): %v", err)
	}
	if parsedFirst.ProjectID != parsedSecond.ProjectID || parsedFirst.GoalID == parsedSecond.GoalID {
		t.Errorf("same-project Goals are not distinctly identified: first=%#v second=%#v", parsedFirst, parsedSecond)
	}
}

func TestVersionOneDirectGoalAPIRemainsValid(t *testing.T) {
	g := sampleGoal()
	if err := g.Validate(); err != nil {
		t.Fatalf("Validate(v1 Goal): %v", err)
	}
	data, err := g.Marshal()
	if err != nil {
		t.Fatalf("Marshal(v1 Goal): %v", err)
	}
	if strings.Contains(string(data), "goal_id") {
		t.Errorf("v1 Marshal unexpectedly includes goal_id: %s", data)
	}
}
