package goal

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
)

// versionThreeFixtureDigest was made with sha256sum over testdata/v3.json, not with this package.
const versionThreeFixtureDigest = "6857513123c3bf5f7a7f2f4cfc1e23414f9bca496be69c0616e808513dbca172"

func sampleDirective() *memoryscope.Directive {
	return &memoryscope.Directive{
		Version: 1,
		Scope:   memoryscope.ScopeGoal,
		Sources: []memoryscope.Source{memoryscope.SourceEngram, memoryscope.SourceLongtermMem},
		Write:   "none",
	}
}

func sampleGoalV3() Goal {
	g := sampleGoalV2()
	g.Version = 3
	g.MemoryScope = ""
	g.MemoryDirective = sampleDirective()
	return g
}

// documentV3With returns the version 3 form of validGoalJSON (goal_id added, memory_scope
// replaced by the sample directive), then applies changes; a nil change removes the key.
func documentV3With(t *testing.T, changes map[string]any) []byte {
	t.Helper()
	var fields map[string]any
	if err := json.Unmarshal([]byte(validGoalJSON), &fields); err != nil {
		t.Fatalf("decode valid test document: %v", err)
	}
	fields["version"] = 3
	fields["goal_id"] = "goal-alpha"
	delete(fields, "memory_scope")
	fields["memory_directive"] = map[string]any{"version": 1, "scope": "goal", "sources": []string{"engram", "longterm-mem"}, "write": "none"}
	for key, value := range changes {
		if value == nil {
			delete(fields, key)
			continue
		}
		fields[key] = value
	}
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("encode test document: %v", err)
	}
	return data
}

func TestVersionThreeFixtureParsesAndMarshalsCanonically(t *testing.T) {
	fixture, err := os.ReadFile("testdata/v3.json")
	if err != nil {
		t.Fatalf("read v3 fixture: %v", err)
	}
	got, err := Parse(fixture)
	if err != nil {
		t.Fatalf("Parse(v3 fixture): %v", err)
	}
	if !reflect.DeepEqual(got, sampleGoalV3()) {
		t.Errorf("Parse(v3 fixture) = %#v, want %#v", got, sampleGoalV3())
	}
	if got.MemoryScope != "" {
		t.Errorf("Parse(v3 fixture).MemoryScope = %q, want empty", got.MemoryScope)
	}
	encoded, err := got.Marshal()
	if err != nil {
		t.Fatalf("Marshal(parsed v3 fixture): %v", err)
	}
	if !reflect.DeepEqual(encoded, fixture) {
		t.Errorf("v3 fixture did not round-trip byte-for-byte:\ngot:\n%s\nwant:\n%s", encoded, fixture)
	}
	digest, err := got.Digest()
	if err != nil {
		t.Fatalf("Digest(v3): %v", err)
	}
	if digest != versionThreeFixtureDigest {
		t.Errorf("Digest(v3) = %q, want %q", digest, versionThreeFixtureDigest)
	}
}

func TestVersionThreeRefusesWithNamedReason(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"memory_scope is unknown in version 3", documentV3With(t, map[string]any{"memory_scope": "Project-scoped."}), `parse goal: unknown Goal field "memory_scope"`},
		{"missing memory_directive", documentV3With(t, map[string]any{"memory_directive": nil}), "parse goal: memory_directive must be a non-null object"},
		{"null memory_directive", []byte(strings.Replace(string(documentV3With(t, nil)), `"memory_directive":{"scope":"goal","sources":["engram","longterm-mem"],"version":1,"write":"none"}`, `"memory_directive":null`, 1)), "parse goal: memory_directive must be a non-null object"},
		{"memory_directive not an object", documentV3With(t, map[string]any{"memory_directive": "project"}), "parse goal: memory_directive: parse memory directive: json: cannot unmarshal string into Go value of type map[string]json.RawMessage"},
		{"unknown directive field", documentV3With(t, map[string]any{"memory_directive": map[string]any{"version": 1, "scope": "goal", "sources": []string{}, "write": "none", "extra": true}}), `parse goal: memory_directive: parse memory directive: unknown memory directive field "extra"`},
		{"case-variant directive field", documentV3With(t, map[string]any{"memory_directive": map[string]any{"version": 1, "SCOPE": "goal", "sources": []string{}, "write": "none"}}), `parse goal: memory_directive: parse memory directive: unknown memory directive field "SCOPE"`},
		{"widening scope name", documentV3With(t, map[string]any{"memory_directive": map[string]any{"version": 1, "scope": "everything", "sources": []string{}, "write": "none"}}), `parse goal: memory_directive: parse memory directive: scope must be "none", "goal", or "project", got "everything"`},
		{"unknown source", documentV3With(t, map[string]any{"memory_directive": map[string]any{"version": 1, "scope": "goal", "sources": []string{"notes"}, "write": "none"}}), `parse goal: memory_directive: parse memory directive: unknown source "notes"`},
		{"write mode", documentV3With(t, map[string]any{"memory_directive": map[string]any{"version": 1, "scope": "goal", "sources": []string{}, "write": "append"}}), `parse goal: memory_directive: parse memory directive: write must be exactly "none", got "append"`},
		{"missing goal_id", documentV3With(t, map[string]any{"goal_id": nil}), "parse goal: goal_id must be a non-blank string"},
		{"version 4 with a directive", documentV3With(t, map[string]any{"version": 4}), `parse goal: unknown Goal field "memory_directive"`},
		{"unsupported version 4", documentWith(t, map[string]any{"version": 4, "goal_id": "goal-alpha"}), "parse goal: unsupported version 4, want 1, 2, or 3"},
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
			if !reflect.DeepEqual(got, Goal{}) {
				t.Errorf("Parse returned partial Goal on error: %#v", got)
			}
		})
	}
}

func TestVersionThreeRefusesADuplicateKeyInsideTheDirective(t *testing.T) {
	data := strings.Replace(string(documentV3With(t, nil)), `"scope":"goal",`, `"scope":"goal","scope":"project",`, 1)
	_, err := Parse([]byte(data))
	if err == nil || err.Error() != `parse goal: duplicate JSON object key "scope"` {
		t.Errorf("Parse error = %v, want the duplicate-key refusal for the directive's scope", err)
	}
}

// Versions 1 and 2 carry no directive: memory_directive is an unknown field there, and their
// own refusals keep their wording.
func TestVersionsOneAndTwoRefuseAMemoryDirective(t *testing.T) {
	directive := map[string]any{"version": 1, "scope": "goal", "sources": []string{}, "write": "none"}
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"version 1 with memory_directive", documentWith(t, map[string]any{"memory_directive": directive}), `parse goal: unknown Goal field "memory_directive"`},
		{"version 2 with memory_directive", documentWith(t, map[string]any{"version": 2, "goal_id": "goal-alpha", "memory_directive": directive}), `parse goal: unknown Goal field "memory_directive"`},
		{"version 2 without memory_scope", documentWithout(t, "memory_scope"), "parse goal: memory_scope must be a non-blank string"},
		{"version 2 without goal_id", documentWith(t, map[string]any{"version": 2}), "parse goal: goal_id must be a non-blank string"},
		{"version 1 with goal_id", documentWith(t, map[string]any{"goal_id": "goal-alpha"}), "parse goal: goal_id is only valid in version 2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.data)
			if err == nil || err.Error() != tc.want {
				t.Errorf("Parse error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestVersionTwoParsesWithNoDirective(t *testing.T) {
	got, err := Parse(documentWith(t, map[string]any{"version": 2, "goal_id": "goal-alpha"}))
	if err != nil {
		t.Fatalf("Parse(v2): %v", err)
	}
	if got.MemoryDirective != nil {
		t.Errorf("Parse(v2).MemoryDirective = %#v, want nil: a version 2 Goal narrows nothing", got.MemoryDirective)
	}
	if got.MemoryScope != "Project-scoped memory may be consulted later." {
		t.Errorf("Parse(v2).MemoryScope = %q, want the authored text", got.MemoryScope)
	}
}

func TestValidateKeepsEachMemoryFieldToItsVersion(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Goal)
		want   string
	}{
		{"memory_scope on version 3", func(g *Goal) { g.MemoryScope = "Project-scoped." }, "memory_scope is only valid in version 1 or 2"},
		{"no directive on version 3", func(g *Goal) { g.MemoryDirective = nil }, "memory_directive must be a non-null object"},
		{"invalid directive on version 3", func(g *Goal) { g.MemoryDirective.Sources = nil }, "memory_directive: sources must be a non-null array"},
		{"directive on version 2", func(g *Goal) { g.Version = 2; g.MemoryScope = "Project-scoped." }, "memory_directive is only valid in version 3"},
		{"unsupported version 4", func(g *Goal) { g.Version = 4 }, "unsupported version 4, want 1, 2, or 3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := sampleGoalV3()
			tc.mutate(&g)
			err := g.Validate()
			if err == nil || err.Error() != tc.want {
				t.Errorf("Validate() = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestVersionThreeDigestChangesWithTheDirective(t *testing.T) {
	base, err := sampleGoalV3().Digest()
	if err != nil {
		t.Fatalf("Digest() = %v, want nil", err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*memoryscope.Directive)
	}{
		{"scope", func(d *memoryscope.Directive) { d.Scope = memoryscope.ScopeProject }},
		{"sources", func(d *memoryscope.Directive) { d.Sources = d.Sources[:1] }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := sampleGoalV3()
			tc.mutate(g.MemoryDirective)
			got, err := g.Digest()
			if err != nil {
				t.Fatalf("Digest() = %v, want nil", err)
			}
			if got == base {
				t.Errorf("Digest() after changing the directive's %s is still %q", tc.name, got)
			}
		})
	}
}
