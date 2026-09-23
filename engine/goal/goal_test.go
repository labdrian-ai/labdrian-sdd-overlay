package goal

import (
	"encoding/json"
	"testing"
)

const validGoalJSON = `{"version":1,"project_id":"standalone-goal-contract","objective":"Represent user intent before workflow shaping.","scope":"One explicitly identified project.","constraints":["Use only structural deterministic validation."],"non_goals":["Grant execution authority."],"acceptance_criteria":["The contract is versioned and can be parsed strictly."],"memory_scope":"Project-scoped memory may be consulted later.","runtime_scope":"Runtime integration is decided later.","delivery_boundary":"No delivery operation is authorized."}`

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
