package goal

import "testing"

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
