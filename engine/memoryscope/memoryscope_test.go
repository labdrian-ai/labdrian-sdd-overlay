package memoryscope

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseDirectiveAcceptsValidRecords(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{
			name: "scope none with empty sources",
			json: `{"version":1,"scope":"none","sources":[],"write":"none"}`,
		},
		{
			name: "scope goal with one source",
			json: `{"version":1,"scope":"goal","sources":["engram"],"write":"none"}`,
		},
		{
			name: "scope project with every source",
			json: `{"version":1,"scope":"project","sources":["engram","longterm-mem","procedural-skills"],"write":"none"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := ParseDirective([]byte(tt.json))
			if err != nil {
				t.Fatalf("ParseDirective(%q) error = %v, want nil", tt.json, err)
			}
			if d.Version != DirectiveVersion {
				t.Errorf("Version = %d, want %d", d.Version, DirectiveVersion)
			}
		})
	}
}

func TestParseDirectiveRejectsInvalidRecords(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr string
	}{
		{
			name:    "unknown top-level field",
			json:    `{"version":1,"scope":"none","sources":[],"write":"none","extra":true}`,
			wantErr: "unknown",
		},
		{
			name:    "case-variant field",
			json:    `{"Version":1,"scope":"none","sources":[],"write":"none"}`,
			wantErr: "unknown",
		},
		{
			name:    "duplicate key",
			json:    `{"version":1,"version":1,"scope":"none","sources":[],"write":"none"}`,
			wantErr: "duplicate",
		},
		{
			name:    "trailing data",
			json:    `{"version":1,"scope":"none","sources":[],"write":"none"}{}`,
			wantErr: "trailing data",
		},
		{
			name:    "invalid utf8",
			json:    "{\"version\":1,\"scope\":\"none\",\"sources\":[],\"write\":\"none\xff\"}",
			wantErr: "UTF-8",
		},
		{
			name:    "wrong version",
			json:    `{"version":2,"scope":"none","sources":[],"write":"none"}`,
			wantErr: "version",
		},
		{
			name:    "unknown scope",
			json:    `{"version":1,"scope":"team","sources":[],"write":"none"}`,
			wantErr: "scope",
		},
		{
			name:    "null sources",
			json:    `{"version":1,"scope":"none","sources":null,"write":"none"}`,
			wantErr: "non-null",
		},
		{
			name:    "unknown source",
			json:    `{"version":1,"scope":"goal","sources":["unknown-source"],"write":"none"}`,
			wantErr: "source",
		},
		{
			name:    "duplicate source",
			json:    `{"version":1,"scope":"goal","sources":["engram","engram"],"write":"none"}`,
			wantErr: "duplicate source",
		},
		{
			name:    "scope none with non-empty sources",
			json:    `{"version":1,"scope":"none","sources":["engram"],"write":"none"}`,
			wantErr: "empty sources",
		},
		{
			name:    "write not none",
			json:    `{"version":1,"scope":"goal","sources":["engram"],"write":"all"}`,
			wantErr: "write",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseDirective([]byte(tt.json))
			if err == nil {
				t.Fatalf("ParseDirective(%q) error = nil, want error containing %q", tt.json, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("ParseDirective(%q) error = %q, want it to contain %q", tt.json, err.Error(), tt.wantErr)
			}
		})
	}
}

// Narrowers can only narrow, so a source or scope absent from every
// profile default is unreachable from any request. Every member of the
// closed set must be granted by at least one default.
func TestEverySourceAndScopeIsReachableFromSomeDefault(t *testing.T) {
	reachedSources := map[Source]bool{}
	reachedScopes := map[Scope]bool{}
	for name, d := range profileDefaults {
		if _, err := DefaultFor(name); err != nil {
			t.Fatalf("DefaultFor(%q) error = %v, want nil", name, err)
		}
		reachedScopes[d.Scope] = true
		for _, s := range d.Sources {
			reachedSources[s] = true
		}
	}
	for s := range knownSources {
		if !reachedSources[s] {
			t.Errorf("source %q is not granted by any profile default, so no request can reach it", s)
		}
	}
	for _, sc := range []Scope{ScopeNone, ScopeGoal, ScopeProject} {
		if !reachedScopes[sc] {
			t.Errorf("scope %q is not granted by any profile default, so no request can reach it", sc)
		}
	}
}

func TestDefaultForKnownProfiles(t *testing.T) {
	tests := []struct {
		profile     string
		wantScope   Scope
		wantSources []Source
	}{
		{"odd", ScopeProject, []Source{SourceEngram, SourceLongtermMem, SourceProceduralSkills}},
		{"sdd", ScopeProject, []Source{SourceEngram, SourceLongtermMem, SourceProceduralSkills}},
		{"standalone-minimal", ScopeNone, nil},
		{"maintenance", ScopeProject, []Source{SourceEngram, SourceLongtermMem, SourceProceduralSkills}},
		{"incident-recovery", ScopeGoal, []Source{SourceEngram, SourceProceduralSkills}},
	}
	for _, tt := range tests {
		t.Run(tt.profile, func(t *testing.T) {
			d, err := DefaultFor(tt.profile)
			if err != nil {
				t.Fatalf("DefaultFor(%q) error = %v, want nil", tt.profile, err)
			}
			if d.Scope != tt.wantScope {
				t.Errorf("DefaultFor(%q).Scope = %q, want %q", tt.profile, d.Scope, tt.wantScope)
			}
			if len(d.Sources) != len(tt.wantSources) {
				t.Fatalf("DefaultFor(%q).Sources = %v, want %v", tt.profile, d.Sources, tt.wantSources)
			}
			for i, s := range tt.wantSources {
				if d.Sources[i] != s {
					t.Errorf("DefaultFor(%q).Sources[%d] = %q, want %q", tt.profile, i, d.Sources[i], s)
				}
			}
			if err := d.Validate(); err != nil {
				t.Errorf("DefaultFor(%q) produced an invalid directive: %v", tt.profile, err)
			}
		})
	}
}

func TestDefaultForUnknownProfileRefuses(t *testing.T) {
	if _, err := DefaultFor("nonexistent-profile"); err == nil {
		t.Fatal("DefaultFor(nonexistent-profile) error = nil, want an error")
	}
}

func TestResolveAppliesNarrowersInOrder(t *testing.T) {
	base := Directive{Version: DirectiveVersion, Scope: ScopeProject, Sources: []Source{SourceEngram, SourceLongtermMem, SourceProceduralSkills}, Write: "none"}
	goalNarrower := Directive{Version: DirectiveVersion, Scope: ScopeGoal, Sources: []Source{SourceEngram, SourceLongtermMem}, Write: "none"}
	handoffNarrower := Directive{Version: DirectiveVersion, Scope: ScopeGoal, Sources: []Source{SourceEngram}, Write: "none"}

	plan, err := Resolve(base, "proj-1", "goal-1", goalNarrower, handoffNarrower)
	if err != nil {
		t.Fatalf("Resolve error = %v, want nil", err)
	}
	if plan.Scope != ScopeGoal {
		t.Errorf("plan.Scope = %q, want %q", plan.Scope, ScopeGoal)
	}
	if len(plan.Sources) != 1 || plan.Sources[0] != SourceEngram {
		t.Errorf("plan.Sources = %v, want [engram]", plan.Sources)
	}
	if plan.Filters.ProjectID != "proj-1" || plan.Filters.GoalID != "goal-1" {
		t.Errorf("plan.Filters = %+v, want project_id=proj-1 goal_id=goal-1", plan.Filters)
	}
	if plan.Write != "none" {
		t.Errorf("plan.Write = %q, want none", plan.Write)
	}
	if strings.TrimSpace(plan.Authority) == "" {
		t.Error("plan.Authority must not be blank")
	}
}

func TestResolveWithNoNarrowersUsesBase(t *testing.T) {
	base := Directive{Version: DirectiveVersion, Scope: ScopeGoal, Sources: []Source{SourceEngram}, Write: "none"}
	plan, err := Resolve(base, "proj-1", "goal-1")
	if err != nil {
		t.Fatalf("Resolve error = %v, want nil", err)
	}
	if plan.Scope != ScopeGoal || len(plan.Sources) != 1 || plan.Sources[0] != SourceEngram {
		t.Errorf("plan = %+v, want the base directive unchanged", plan)
	}
}

func TestResolvePlanSourcesIsNonNilEvenWhenEmpty(t *testing.T) {
	noneScope := Directive{Version: DirectiveVersion, Scope: ScopeNone, Sources: []Source{}, Write: "none"}
	plan, err := Resolve(noneScope, "", "")
	if err != nil {
		t.Fatalf("Resolve error = %v, want nil", err)
	}
	if plan.Sources == nil {
		t.Error("plan.Sources must be a non-nil empty slice so it serializes as [] instead of null")
	}
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("json.Marshal error = %v, want nil", err)
	}
	if strings.Contains(string(data), `"sources":null`) {
		t.Errorf("plan JSON = %s, want sources to serialize as []", data)
	}
}

func TestResolveRefusesWideningScope(t *testing.T) {
	base := Directive{Version: DirectiveVersion, Scope: ScopeGoal, Sources: []Source{SourceEngram}, Write: "none"}
	widerNarrower := Directive{Version: DirectiveVersion, Scope: ScopeProject, Sources: []Source{SourceEngram}, Write: "none"}

	_, err := Resolve(base, "proj-1", "goal-1", widerNarrower)
	if err == nil {
		t.Fatal("Resolve error = nil, want a widening refusal")
	}
	if !strings.Contains(err.Error(), "widens scope from goal to project") {
		t.Errorf("Resolve error = %q, want it to name the widening", err.Error())
	}
}

func TestResolveRefusesAddingSource(t *testing.T) {
	base := Directive{Version: DirectiveVersion, Scope: ScopeGoal, Sources: []Source{SourceEngram}, Write: "none"}
	extraSourceNarrower := Directive{Version: DirectiveVersion, Scope: ScopeGoal, Sources: []Source{SourceEngram, SourceLongtermMem}, Write: "none"}

	_, err := Resolve(base, "proj-1", "goal-1", extraSourceNarrower)
	if err == nil {
		t.Fatal("Resolve error = nil, want a source-widening refusal")
	}
	if !strings.Contains(err.Error(), "adds source longterm-mem") {
		t.Errorf("Resolve error = %q, want it to name the added source", err.Error())
	}
}

func TestResolveRejectsInvalidBaseOrNarrower(t *testing.T) {
	validBase := Directive{Version: DirectiveVersion, Scope: ScopeGoal, Sources: []Source{SourceEngram}, Write: "none"}
	invalid := Directive{Version: 99, Scope: ScopeGoal, Sources: []Source{SourceEngram}, Write: "none"}

	if _, err := Resolve(invalid, "proj-1", "goal-1"); err == nil {
		t.Fatal("Resolve with invalid base error = nil, want an error")
	}
	if _, err := Resolve(validBase, "proj-1", "goal-1", invalid); err == nil {
		t.Fatal("Resolve with invalid narrower error = nil, want an error")
	}
}

// A goal-scoped plan without a goal_id would advertise single-goal reads
// while its filters cover the whole project, so a blank goal_id is refused.
func TestResolveRequiresGoalIDForGoalScope(t *testing.T) {
	goalScope := Directive{Version: DirectiveVersion, Scope: ScopeGoal, Sources: []Source{SourceEngram}, Write: "none"}
	for _, goalID := range []string{"", "   "} {
		if plan, err := Resolve(goalScope, "proj-1", goalID); err == nil {
			t.Errorf("Resolve(scope goal, goal_id %q) = %+v, nil; want an error", goalID, plan)
		}
	}
	projectScope := Directive{Version: DirectiveVersion, Scope: ScopeProject, Sources: []Source{SourceEngram}, Write: "none"}
	narrowToGoal := Directive{Version: DirectiveVersion, Scope: ScopeGoal, Sources: []Source{SourceEngram}, Write: "none"}
	if plan, err := Resolve(projectScope, "proj-1", "", narrowToGoal); err == nil {
		t.Errorf("Resolve narrowed to scope goal with blank goal_id = %+v, nil; want an error", plan)
	}
}

func TestResolveRequiresProjectIDForGoalAndProjectScope(t *testing.T) {
	goalScope := Directive{Version: DirectiveVersion, Scope: ScopeGoal, Sources: []Source{SourceEngram}, Write: "none"}
	if _, err := Resolve(goalScope, "", "goal-1"); err == nil {
		t.Fatal("Resolve with blank project_id and scope goal error = nil, want an error")
	}
	projectScope := Directive{Version: DirectiveVersion, Scope: ScopeProject, Sources: []Source{SourceEngram}, Write: "none"}
	if _, err := Resolve(projectScope, "", ""); err == nil {
		t.Fatal("Resolve with blank project_id and scope project error = nil, want an error")
	}
}

func TestResolveOmitsGoalIDForProjectScope(t *testing.T) {
	projectScope := Directive{Version: DirectiveVersion, Scope: ScopeProject, Sources: []Source{SourceEngram}, Write: "none"}
	plan, err := Resolve(projectScope, "proj-1", "goal-1")
	if err != nil {
		t.Fatalf("Resolve error = %v, want nil", err)
	}
	if plan.Filters.ProjectID != "proj-1" {
		t.Errorf("plan.Filters.ProjectID = %q, want proj-1", plan.Filters.ProjectID)
	}
	if plan.Filters.GoalID != "" {
		t.Errorf("plan.Filters.GoalID = %q, want empty: goal_id is only for scope goal", plan.Filters.GoalID)
	}
}

func TestResolveOmitsFiltersForNoneScopeEvenWhenSupplied(t *testing.T) {
	noneScope := Directive{Version: DirectiveVersion, Scope: ScopeNone, Sources: []Source{}, Write: "none"}
	plan, err := Resolve(noneScope, "proj-1", "goal-1")
	if err != nil {
		t.Fatalf("Resolve(none, proj-1, goal-1) error = %v, want nil: a caller-supplied identifier for an unused scope is not an error", err)
	}
	if plan.Filters.ProjectID != "" || plan.Filters.GoalID != "" {
		t.Errorf("plan.Filters = %+v, want empty for scope none", plan.Filters)
	}

	plan, err = Resolve(noneScope, "", "")
	if err != nil {
		t.Fatalf("Resolve(none, \"\", \"\") error = %v, want nil", err)
	}
	if plan.Filters.ProjectID != "" || plan.Filters.GoalID != "" {
		t.Errorf("plan.Filters = %+v, want empty", plan.Filters)
	}
}
