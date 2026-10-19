package memoryscope

import (
	"reflect"
	"testing"
)

// TestParseDirectiveMemberReadsAnOptionalNestedDirective covers the one helper every record that
// nests a MemoryDirective uses (the Goal v3, the shaper handoff v4, the workflow log): an absent or
// null member is no directive, a present one is parsed as strictly as a directive on its own, and
// every refusal names the member.
func TestParseDirectiveMemberReadsAnOptionalNestedDirective(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    *Directive
		wantErr string
	}{
		{name: "absent member", data: `{"other":1}`, want: nil},
		{name: "null member", data: `{"memory_directive": null }`, want: nil},
		{
			name: "valid member",
			data: `{"memory_directive":{"version":1,"scope":"goal","sources":["engram"],"write":"none"}}`,
			want: &Directive{Version: 1, Scope: ScopeGoal, Sources: []Source{SourceEngram}, Write: "none"},
		},
		{
			name:    "case-variant nested key",
			data:    `{"memory_directive":{"version":1,"Scope":"goal","sources":[],"write":"none"}}`,
			wantErr: `memory_directive: parse memory directive: unknown memory directive field "Scope"`,
		},
		{
			name:    "unknown nested key",
			data:    `{"memory_directive":{"version":1,"scope":"none","sources":[],"write":"none","x":1}}`,
			wantErr: `memory_directive: parse memory directive: unknown memory directive field "x"`,
		},
		{
			name:    "invalid nested directive",
			data:    `{"memory_directive":{"version":2,"scope":"none","sources":[],"write":"none"}}`,
			wantErr: "memory_directive: parse memory directive: version must be 1, got 2",
		},
		{name: "not an object", data: `[1]`, wantErr: "json: cannot unmarshal array into Go value of type map[string]json.RawMessage"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDirectiveMember([]byte(tt.data), "memory_directive")
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("ParseDirectiveMember() error = %v, want %q", err, tt.wantErr)
				}
				if got != nil {
					t.Errorf("ParseDirectiveMember() = %+v on error, want nil", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseDirectiveMember() error = %v, want nil", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseDirectiveMember() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestParseDirectiveMemberNamesTheMemberItWasAsked tells the member apart from the key the caller
// chose, so a record with several directives says which one is wrong.
func TestParseDirectiveMemberNamesTheMemberItWasAsked(t *testing.T) {
	_, err := ParseDirectiveMember([]byte(`{"goal_directive":{"version":1,"scope":"none","sources":[],"write":"all"}}`), "goal_directive")
	want := `goal_directive: parse memory directive: write must be exactly "none", got "all"`
	if err == nil || err.Error() != want {
		t.Fatalf("ParseDirectiveMember() error = %v, want %q", err, want)
	}
}

// TestCheckNarrowsSaysWhatWidens covers the step Resolve applies to every narrower, exported so a
// caller that names its narrowers can say which one widened.
func TestCheckNarrowsSaysWhatWidens(t *testing.T) {
	project := Directive{Version: 1, Scope: ScopeProject, Sources: []Source{SourceEngram}, Write: "none"}
	tests := []struct {
		name      string
		narrower  Directive
		wantError string
	}{
		{name: "equal", narrower: project},
		{name: "narrower scope, fewer sources", narrower: Directive{Version: 1, Scope: ScopeGoal, Sources: []Source{}, Write: "none"}},
		{name: "added source", narrower: Directive{Version: 1, Scope: ScopeProject, Sources: []Source{SourceEngram, SourceLongtermMem}, Write: "none"}, wantError: "adds source longterm-mem not present in the current scope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckNarrows(project, tt.narrower)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("CheckNarrows() = %v, want nil", err)
				}
				return
			}
			if err == nil || err.Error() != tt.wantError {
				t.Fatalf("CheckNarrows() = %v, want %q", err, tt.wantError)
			}
		})
	}
	goalScope := Directive{Version: 1, Scope: ScopeGoal, Sources: []Source{SourceEngram}, Write: "none"}
	if err := CheckNarrows(goalScope, project); err == nil || err.Error() != "widens scope from goal to project" {
		t.Fatalf("CheckNarrows(goal, project) = %v, want %q", err, "widens scope from goal to project")
	}
}
