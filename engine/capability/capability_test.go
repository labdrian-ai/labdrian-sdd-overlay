package capability_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
)

func TestCapabilitiesAreTheClosedSetInFixedOrder(t *testing.T) {
	want := []string{
		"installation", "projection", "dispatch", "cancellation",
		"persistence", "restart", "authentication", "memory-enforcement",
		"skills",
	}
	var got []string
	for _, c := range capability.Capabilities() {
		got = append(got, string(c))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Capabilities() = %v, want %v", got, want)
	}
}

func TestCapabilitiesReturnsACopy(t *testing.T) {
	first := capability.Capabilities()
	first[0] = "tampered"
	if got := capability.Capabilities()[0]; got != capability.Installation {
		t.Fatalf("mutating a returned slice changed package state: Capabilities()[0] = %q", got)
	}
}

func TestTargetsAreTheFourRuntimesInFixedOrder(t *testing.T) {
	want := []string{"claude", "codex", "pi", "opencode"}
	if got := capability.Targets(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Targets() = %v, want %v", got, want)
	}
}

func TestTargetsReturnsACopy(t *testing.T) {
	first := capability.Targets()
	first[0] = "tampered"
	if got := capability.Targets()[0]; got != capability.TargetClaude {
		t.Fatalf("mutating a returned slice changed package state: Targets()[0] = %q", got)
	}
}

func TestIsTargetAcceptsExactlyTheDeclaredRuntimes(t *testing.T) {
	for _, target := range capability.Targets() {
		if !capability.IsTarget(target) {
			t.Errorf("IsTarget(%q) = false, want true: it is in Targets()", target)
		}
	}
	for _, other := range []string{"", "all", "Claude", " claude", "claude ", "cursor", "longterm-mem"} {
		if capability.IsTarget(other) {
			t.Errorf("IsTarget(%q) = true, want false: it is not a declared runtime", other)
		}
	}
}

func TestStatusVocabularyWireValues(t *testing.T) {
	for _, tt := range []struct {
		status capability.Status
		want   string
	}{
		{capability.Supported, "supported"},
		{capability.Partial, "partial"},
		{capability.Unsupported, "unsupported"},
	} {
		if string(tt.status) != tt.want {
			t.Errorf("status constant = %q, want %q", tt.status, tt.want)
		}
	}
}

func TestClaimJSONShape(t *testing.T) {
	tests := []struct {
		name  string
		claim capability.Claim
		want  string
	}{
		{
			name:  "an unsupported claim with nil tests serializes an empty array, never null",
			claim: capability.Claim{Capability: capability.Projection, Status: capability.Unsupported, Detail: "no projection code exists"},
			want:  `{"capability":"projection","status":"unsupported","tests":[],"detail":"no projection code exists"}`,
		},
		{
			name:  "a supported claim without a detail omits the key",
			claim: capability.Claim{Capability: capability.Installation, Status: capability.Supported, Tests: []string{"runtime:TestExample"}},
			want:  `{"capability":"installation","status":"supported","tests":["runtime:TestExample"]}`,
		},
		{
			name:  "a partial claim keeps its tests and its limit",
			claim: capability.Claim{Capability: capability.Restart, Status: capability.Partial, Tests: []string{"a:TestOne", "b:TestTwo"}, Detail: "soft restart only"},
			want:  `{"capability":"restart","status":"partial","tests":["a:TestOne","b:TestTwo"],"detail":"soft restart only"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.claim)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("json = %s\nwant   %s", got, tt.want)
			}
		})
	}
}

func TestDeclarationJSONShape(t *testing.T) {
	tests := []struct {
		name string
		decl capability.Declaration
		want string
	}{
		{
			name: "untested is omitted when the runtime can be exercised",
			decl: capability.Declaration{Target: "claude", Claims: []capability.Claim{}},
			want: `{"target":"claude","claims":[]}`,
		},
		{
			name: "untested carries its reason between target and claims",
			decl: capability.Declaration{Target: "opencode", Untested: "cannot be exercised here", Claims: []capability.Claim{}},
			want: `{"target":"opencode","untested":"cannot be exercised here","claims":[]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.decl)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("json = %s\nwant   %s", got, tt.want)
			}
		})
	}
}
