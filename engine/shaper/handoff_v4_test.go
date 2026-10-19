package shaper

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
)

// validDirectiveJSON is the memory directive of validHandoffV4JSON.
const validDirectiveJSON = `{"version":1,"scope":"goal","sources":["engram","longterm-mem"],"write":"none"}`

// validHandoffV4JSON is validHandoffV3JSON at version 4, with memory_scope
// replaced by memory_directive.
var validHandoffV4JSON = strings.Replace(
	strings.Replace(validHandoffV3JSON, `{"version":3,`, `{"version":4,`, 1),
	`"memory_scope":"Project-scoped."`, `"memory_directive":`+validDirectiveJSON, 1)

// v4WithRaw returns validHandoffV4JSON with the raw value of one top-level
// field replaced verbatim, or removed when rawValue is "".
func v4WithRaw(t *testing.T, field, rawValue string) []byte {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(validHandoffV4JSON), &fields); err != nil {
		t.Fatalf("unmarshal v4 fixture: %v", err)
	}
	if rawValue == "" {
		delete(fields, field)
	} else {
		fields[field] = json.RawMessage(rawValue)
	}
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("marshal v4 fixture: %v", err)
	}
	return data
}

func sampleDirective() *memoryscope.Directive {
	return &memoryscope.Directive{
		Version: 1,
		Scope:   memoryscope.ScopeGoal,
		Sources: []memoryscope.Source{memoryscope.SourceEngram, memoryscope.SourceLongtermMem},
		Write:   "none",
	}
}

func sampleHandoffV4() Handoff {
	h := sampleHandoffV3()
	h.Version = 4
	h.MemoryScope = ""
	h.MemoryDirective = sampleDirective()
	return h
}

func TestParseV4CarriesTheDirectiveInPlaceOfMemoryScope(t *testing.T) {
	got, err := Parse([]byte(validHandoffV4JSON))
	if err != nil {
		t.Fatalf("Parse(v4): %v", err)
	}
	v3, err := Parse([]byte(validHandoffV3JSON))
	if err != nil {
		t.Fatalf("Parse(v3): %v", err)
	}
	want := v3
	want.Version = 4
	want.MemoryScope = ""
	want.MemoryDirective = sampleDirective()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse(v4) = %#v, want %#v", got, want)
	}
}

func TestParseV4RefusesWithNamedReason(t *testing.T) {
	duplicate := strings.Replace(validHandoffV4JSON, `"scope":"goal",`, `"scope":"goal","scope":"project",`, 1)
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"memory_scope is unknown in version 4", v4WithRaw(t, "memory_scope", `"Project-scoped."`), `parse shaper handoff: unknown Shaper handoff field "memory_scope"`},
		{"missing memory_directive", v4WithRaw(t, "memory_directive", ""), "parse shaper handoff: memory_directive must be a non-null object"},
		{"null memory_directive", v4WithRaw(t, "memory_directive", "null"), "parse shaper handoff: memory_directive must be a non-null object"},
		{"unknown directive field", v4WithRaw(t, "memory_directive", `{"version":1,"scope":"goal","sources":[],"write":"none","extra":true}`), `parse shaper handoff: memory_directive: parse memory directive: unknown memory directive field "extra"`},
		{"case-variant directive field", v4WithRaw(t, "memory_directive", `{"version":1,"Scope":"goal","sources":[],"write":"none"}`), `parse shaper handoff: memory_directive: parse memory directive: unknown memory directive field "Scope"`},
		{"duplicate directive key", []byte(duplicate), `parse shaper handoff: duplicate JSON object key "scope"`},
		{"unknown source", v4WithRaw(t, "memory_directive", `{"version":1,"scope":"goal","sources":["notes"],"write":"none"}`), `parse shaper handoff: memory_directive: parse memory directive: unknown source "notes"`},
		{"write mode", v4WithRaw(t, "memory_directive", `{"version":1,"scope":"project","sources":[],"write":"append"}`), `parse shaper handoff: memory_directive: parse memory directive: write must be exactly "none", got "append"`},
		{"version 3 rules still apply", v4WithRaw(t, "roles", "null"), "parse shaper handoff: roles must be a non-null array"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.data)
			if err == nil {
				t.Fatalf("Parse accepted %s: %#v", tc.name, got)
			}
			if err.Error() != tc.want {
				t.Errorf("Parse error = %q, want %q", err.Error(), tc.want)
			}
			if !reflect.DeepEqual(got, Handoff{}) {
				t.Errorf("Parse returned a partial handoff: %#v", got)
			}
		})
	}
}

// Versions 1 to 3 carry no directive: memory_directive is an unknown field there, and a
// version 3 handoff still parses as before.
func TestParseBeforeV4RefusesAMemoryDirective(t *testing.T) {
	var directive any
	if err := json.Unmarshal([]byte(validDirectiveJSON), &directive); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"version 1", documentWith(t, map[string]any{"memory_directive": directive})},
		{"version 2", v2DocumentWith(t, map[string]any{"memory_directive": directive})},
		{"version 3", v3DocumentWith(t, map[string]any{"memory_directive": directive})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.data)
			if want := `parse shaper handoff: unknown Shaper handoff field "memory_directive"`; err == nil || err.Error() != want {
				t.Errorf("Parse error = %v, want %q", err, want)
			}
		})
	}
	v3, err := Parse([]byte(validHandoffV3JSON))
	if err != nil {
		t.Fatalf("Parse(v3): %v", err)
	}
	if v3.MemoryScope != "Project-scoped." || v3.MemoryDirective != nil {
		t.Errorf("Parse(v3) memory = (%q, %#v), want (%q, nil)", v3.MemoryScope, v3.MemoryDirective, "Project-scoped.")
	}
}

func TestValidateKeepsEachHandoffMemoryFieldToItsVersion(t *testing.T) {
	cases := []struct {
		name string
		h    func() Handoff
		want string
	}{
		{"memory_scope on version 4", func() Handoff { h := sampleHandoffV4(); h.MemoryScope = "Project-scoped."; return h }, "memory_scope is only valid in version 3"},
		{"no directive on version 4", func() Handoff { h := sampleHandoffV4(); h.MemoryDirective = nil; return h }, "memory_directive must be a non-null object"},
		{"invalid directive on version 4", func() Handoff { h := sampleHandoffV4(); h.MemoryDirective.Write = "append"; return h }, `memory_directive: write must be exactly "none", got "append"`},
		{"directive on version 3", func() Handoff { h := sampleHandoffV3(); h.MemoryDirective = sampleDirective(); return h }, "memory_directive is only valid in version 4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.h().Validate()
			if err == nil || err.Error() != tc.want {
				t.Errorf("Validate() = %v, want %q", err, tc.want)
			}
		})
	}
	if err := sampleHandoffV4().Validate(); err != nil {
		t.Errorf("Validate(sample v4) = %v, want nil", err)
	}
}
