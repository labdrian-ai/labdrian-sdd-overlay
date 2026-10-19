package roles

import (
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

// goalEngramDirective is a valid MemoryDirective v1 object as it appears inside a v2 record.
const goalEngramDirective = `{"version": 1, "scope": "goal", "sources": ["engram"], "write": "none"}`

// v2HandoffJSON is validHandoffJSON as a version 2 record; directive, when not empty, is the raw
// value of its memory_directive member.
func v2HandoffJSON(directive string) string {
	data := strings.Replace(validHandoffJSON(), `"version": 1`, `"version": 2`, 1)
	if directive != "" {
		data = strings.Replace(data, `"status": "completed"`, `"status": "completed",
  "memory_directive": `+directive, 1)
	}
	return data
}

func TestParseRoleHandoffV2CarriesItsMemoryDirective(t *testing.T) {
	h, err := ParseRoleHandoff([]byte(v2HandoffJSON(goalEngramDirective)))
	if err != nil {
		t.Fatalf("ParseRoleHandoff() = %v, want nil", err)
	}
	if h.Version != HandoffVersionV2 {
		t.Errorf("Version = %d, want %d", h.Version, HandoffVersionV2)
	}
	want := &memoryscope.Directive{Version: 1, Scope: memoryscope.ScopeGoal, Sources: []memoryscope.Source{memoryscope.SourceEngram}, Write: "none"}
	if !reflect.DeepEqual(h.MemoryDirective, want) {
		t.Errorf("MemoryDirective = %+v, want %+v", h.MemoryDirective, want)
	}
	if h.FromRole != RoleBuilder || h.ToRole != RoleReviewer || h.Status != StatusCompleted {
		t.Errorf("FromRole/ToRole/Status = %q/%q/%q, want builder/reviewer/completed", h.FromRole, h.ToRole, h.Status)
	}
}

func TestParseRoleHandoffV2WithoutADirectiveHasNone(t *testing.T) {
	h, err := ParseRoleHandoff([]byte(v2HandoffJSON("")))
	if err != nil {
		t.Fatalf("ParseRoleHandoff() = %v, want nil", err)
	}
	if h.Version != HandoffVersionV2 {
		t.Errorf("Version = %d, want %d", h.Version, HandoffVersionV2)
	}
	if h.MemoryDirective != nil {
		t.Errorf("MemoryDirective = %+v, want nil: an absent directive is no directive", h.MemoryDirective)
	}
}

func TestParseRoleHandoffV1RefusesAMemoryDirective(t *testing.T) {
	data := strings.Replace(validHandoffJSON(), `"status": "completed"`, `"status": "completed", "memory_directive": `+goalEngramDirective, 1)
	_, err := ParseRoleHandoff([]byte(data))
	want := `parse role handoff: unknown role handoff field "memory_directive"`
	if err == nil || err.Error() != want {
		t.Fatalf("ParseRoleHandoff() = %v, want %q", err, want)
	}
}

// TestParseRoleHandoffV1IsUnchanged pins that a v1 record still parses to the same value, with no
// directive, and that its errors still read as they always did.
func TestParseRoleHandoffV1IsUnchanged(t *testing.T) {
	h, err := ParseRoleHandoff([]byte(validHandoffJSON()))
	if err != nil {
		t.Fatalf("ParseRoleHandoff() = %v, want nil", err)
	}
	if h.Version != HandoffVersion || h.MemoryDirective != nil {
		t.Errorf("Version/MemoryDirective = %d/%v, want %d/nil", h.Version, h.MemoryDirective, HandoffVersion)
	}
	for _, tc := range []struct{ name, data, want string }{
		{"unknown field", strings.Replace(validHandoffJSON(), `"status": "completed"`, `"status": "completed", "extra_field": true`, 1), `parse role handoff: unknown role handoff field "extra_field"`},
		{"nested unknown field", strings.Replace(validHandoffJSON(), `"kind": "test"`, `"kind": "test", "bogus": 1`, 1), `parse role handoff: evidence[0]: unknown role handoff evidence item field "bogus"`},
		{"blank summary", strings.Replace(validHandoffJSON(), `"implemented the feature"`, `" "`, 1), "parse role handoff: context.summary must not be blank"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseRoleHandoff([]byte(tc.data))
			if err == nil || err.Error() != tc.want {
				t.Errorf("ParseRoleHandoff() = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestParseRoleHandoffV2RefusesABadDirective(t *testing.T) {
	for _, tc := range []struct{ name, directive, want string }{
		{"unknown nested key", `{"version": 1, "scope": "goal", "sources": [], "write": "none", "extra": 1}`,
			`parse role handoff: memory_directive: parse memory directive: unknown memory directive field "extra"`},
		{"case-variant nested key", `{"version": 1, "Scope": "goal", "sources": [], "write": "none"}`,
			`parse role handoff: memory_directive: parse memory directive: unknown memory directive field "Scope"`},
		{"duplicate nested key", `{"version": 1, "scope": "goal", "scope": "none", "sources": [], "write": "none"}`,
			`parse role handoff: duplicate JSON object key "scope"`},
		{"null", `null`, `parse role handoff: memory_directive: parse memory directive: memory directive must be a JSON object`},
		{"unknown scope", `{"version": 1, "scope": "world", "sources": [], "write": "none"}`,
			`parse role handoff: memory_directive: parse memory directive: scope must be "none", "goal", or "project", got "world"`},
		{"a write grant", `{"version": 1, "scope": "goal", "sources": [], "write": "all"}`,
			`parse role handoff: memory_directive: parse memory directive: write must be exactly "none", got "all"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, err := ParseRoleHandoff([]byte(v2HandoffJSON(tc.directive)))
			if err == nil {
				t.Fatalf("ParseRoleHandoff() = %+v, nil; want %q", h, tc.want)
			}
			if err.Error() != tc.want {
				t.Errorf("ParseRoleHandoff() = %q, want %q", err.Error(), tc.want)
			}
		})
	}
}

func TestRoleHandoffValidateHoldsTheDirectiveToItsVersion(t *testing.T) {
	directive := memoryscope.Directive{Version: 1, Scope: memoryscope.ScopeGoal, Sources: []memoryscope.Source{}, Write: "none"}
	for _, tc := range []struct {
		name   string
		mutate func(*RoleHandoff)
		want   string
	}{
		{"an unknown version", func(h *RoleHandoff) { h.Version = 3 }, "version must be 1 or 2, got 3"},
		{"a directive on a v1 record", func(h *RoleHandoff) { h.MemoryDirective = &directive }, "memory_directive requires version 2, got version 1"},
		{"an invalid directive on a v2 record", func(h *RoleHandoff) {
			bad := directive
			bad.Write = "all"
			h.Version, h.MemoryDirective = HandoffVersionV2, &bad
		}, `memory_directive: write must be exactly "none", got "all"`},
		{"the status before the directive", func(h *RoleHandoff) { h.Status, h.MemoryDirective = Status("paused"), &directive }, `status must be "completed" or "interrupted", got "paused"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := mustParse(t, validHandoffJSON())
			tc.mutate(&h)
			err := h.Validate()
			if err == nil || err.Error() != tc.want {
				t.Errorf("Validate() = %v, want %q", err, tc.want)
			}
		})
	}
	t.Run("a valid directive on a v2 record passes", func(t *testing.T) {
		h := mustParse(t, validHandoffJSON())
		h.Version, h.MemoryDirective = HandoffVersionV2, &directive
		if err := h.Validate(); err != nil {
			t.Errorf("Validate() = %v, want nil", err)
		}
	})
}

// v2RecordJSON is recordJSON as a version 2 record with the given raw directive, or none.
func v2RecordJSON(seq int, prevSHA, from, to, directive string) string {
	data := strings.Replace(recordJSON(seq, prevSHA, from, to, "completed", ""), `"version": 1`, `"version": 2`, 1)
	if directive != "" {
		data = strings.Replace(data, `"status": "completed"`, `"status": "completed", "memory_directive": `+directive, 1)
	}
	return data
}

func TestVerifyChainLetsAChainMoveFromV1ToV2(t *testing.T) {
	c1 := chainRecord(t, recordJSON(1, EmptyChainDigest, "shaper", "estimator", "completed", ""))
	c2 := chainRecord(t, v2RecordJSON(2, sha256hex.Sum(c1.Raw), "estimator", "builder", goalEngramDirective))
	c3 := chainRecord(t, v2RecordJSON(3, sha256hex.Sum(c2.Raw), "builder", "reviewer", ""))
	if err := VerifyChain([]ChainRecord{c1, c2, c3}); err != nil {
		t.Fatalf("VerifyChain() = %v, want nil: a chain may move from v1 to v2", err)
	}
	state, err := Resume([]ChainRecord{c1, c2, c3})
	if err != nil || state.Role != RoleReviewer || state.Interrupted || state.Terminal {
		t.Fatalf("Resume() = %+v, %v; want role=reviewer, not interrupted, not terminal", state, err)
	}
}

func TestVerifyChainNeverMovesBackFromV2ToV1(t *testing.T) {
	c1 := chainRecord(t, v2RecordJSON(1, EmptyChainDigest, "shaper", "estimator", goalEngramDirective))
	c2 := chainRecord(t, recordJSON(2, sha256hex.Sum(c1.Raw), "estimator", "builder", "completed", ""))
	err := VerifyChain([]ChainRecord{c1, c2})
	want := "role chain: seq 2 has version 1 after seq 1 version 2; a chain never moves back to an older version"
	if err == nil || err.Error() != want {
		t.Fatalf("VerifyChain() = %v, want %q", err, want)
	}
	if err := AdmitRecord([]ChainRecord{c1}, c2); err == nil || err.Error() != "role chain store: append: "+want {
		t.Fatalf("AdmitRecord() = %v, want %q", err, "role chain store: append: "+want)
	}
}

// TestVerifyChainBindsTheDirective proves the digest the next record chains to covers the
// directive: a record whose directive changed after the next one was written no longer verifies.
func TestVerifyChainBindsTheDirective(t *testing.T) {
	c1 := chainRecord(t, v2RecordJSON(1, EmptyChainDigest, "shaper", "estimator", goalEngramDirective))
	c2 := chainRecord(t, v2RecordJSON(2, sha256hex.Sum(c1.Raw), "estimator", "builder", ""))
	swapped := chainRecord(t, v2RecordJSON(1, EmptyChainDigest, "shaper", "estimator",
		`{"version": 1, "scope": "project", "sources": ["engram"], "write": "none"}`))
	if err := VerifyChain([]ChainRecord{c1, c2}); err != nil {
		t.Fatalf("VerifyChain(original) = %v, want nil", err)
	}
	err := VerifyChain([]ChainRecord{swapped, c2})
	if err == nil || !strings.Contains(err.Error(), "seq 2 prev_sha256") {
		t.Fatalf("VerifyChain(swapped directive) = %v, want a prev_sha256 mismatch at seq 2", err)
	}
}
