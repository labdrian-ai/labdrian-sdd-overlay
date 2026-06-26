package skills

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// mustParseBytes is a test helper that parses YAML bytes or fatals.
func mustParseBytes(t *testing.T, data []byte) Registry {
	t.Helper()
	reg, err := ParseRegistry(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ParseRegistry: %v", err)
	}
	return reg
}

// mustSerialize is a test helper that calls Serialize or fatals.
func mustSerialize(t *testing.T, reg Registry) []byte {
	t.Helper()
	out, err := Serialize(reg)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	return out
}

// TestSerializeRoundTripAllowedProjects verifies SC-21: a project-scoped entry with
// allowedProjects round-trips through serialize → parse with DeepEqual.
func TestSerializeRoundTripAllowedProjects(t *testing.T) {
	reg := Registry{
		Version: "1",
		Skills: []Entry{
			{
				ID:   "prespec-malandra",
				Path: "prespec-malandra",
				Source: Source{
					Type: "custom",
				},
				Install: Install{
					DefaultScope:    "project",
					Targets:         []string{"claude"},
					AllowedProjects: []string{"labdrian-sdd-overlay"},
				},
				Lifecycle: Lifecycle{
					UpdateStrategy: "overlay-only",
				},
			},
		},
	}

	out := mustSerialize(t, reg)
	got := mustParseBytes(t, out)

	if !reflect.DeepEqual(reg, got) {
		t.Errorf("round-trip mismatch\noriginal: %+v\ngot:      %+v", reg, got)
	}
}

// TestSerializeDeterministic verifies SC-22: two calls on the same Registry produce
// identical byte slices.
func TestSerializeDeterministic(t *testing.T) {
	reg := Registry{
		Version: "1",
		Skills: []Entry{
			{
				ID:   "sdd-spec",
				Path: "sdd-spec",
				Source: Source{
					Type:     "core",
					Upstream: &Upstream{Owner: "gentleman-programming"},
				},
				Install: Install{
					DefaultScope: "global",
					Targets:      []string{"claude", "opencode", "codex"},
				},
				Lifecycle: Lifecycle{UpdateStrategy: "vendor-merge"},
			},
			{
				ID:   "my-custom",
				Path: "my-custom",
				Source: Source{
					Type: "custom",
				},
				Install: Install{
					DefaultScope: "global",
					Targets:      []string{"claude", "opencode", "codex"},
				},
				Lifecycle: Lifecycle{UpdateStrategy: "overlay-only"},
			},
		},
	}

	a := mustSerialize(t, reg)
	b := mustSerialize(t, reg)

	if !bytes.Equal(a, b) {
		t.Errorf("Serialize is not deterministic:\nfirst call:\n%s\nsecond call:\n%s", a, b)
	}
}

// TestSerializeNoUpstreamForCustom verifies SC-23: output for a custom entry must not
// contain the string "upstream".
func TestSerializeNoUpstreamForCustom(t *testing.T) {
	reg := Registry{
		Version: "1",
		Skills: []Entry{
			{
				ID:   "my-custom",
				Path: "my-custom",
				Source: Source{
					Type:     "custom",
					Upstream: nil,
				},
				Install: Install{
					DefaultScope: "global",
					Targets:      []string{"claude", "opencode", "codex"},
				},
				Lifecycle: Lifecycle{UpdateStrategy: "overlay-only"},
			},
		},
	}

	out := mustSerialize(t, reg)
	if strings.Contains(string(out), "upstream") {
		t.Errorf("output for custom entry must not contain 'upstream', got:\n%s", out)
	}
}

// TestSerializeUpstreamForCore verifies SC-24: a serialized core entry re-parsed has a
// non-nil Source.Upstream with a non-empty Owner.
func TestSerializeUpstreamForCore(t *testing.T) {
	reg := Registry{
		Version: "1",
		Skills: []Entry{
			{
				ID:   "sdd-spec",
				Path: "sdd-spec",
				Source: Source{
					Type:     "core",
					Upstream: &Upstream{Owner: "gentleman-programming"},
				},
				Install: Install{
					DefaultScope: "global",
					Targets:      []string{"claude", "opencode", "codex"},
				},
				Lifecycle: Lifecycle{UpdateStrategy: "vendor-merge"},
			},
		},
	}

	out := mustSerialize(t, reg)
	got := mustParseBytes(t, out)

	if got.Skills[0].Source.Upstream == nil {
		t.Fatal("re-parsed core entry: Source.Upstream is nil, want non-nil")
	}
	if got.Skills[0].Source.Upstream.Owner != "gentleman-programming" {
		t.Errorf("re-parsed upstream.owner = %q, want %q", got.Skills[0].Source.Upstream.Owner, "gentleman-programming")
	}
}

// TestSerializeGoldenTwoEntry verifies SC-25: the serialized form of a two-entry registry
// matches a golden file. Run with -update to write the golden file.
func TestSerializeGoldenTwoEntry(t *testing.T) {
	reg := Registry{
		Version: "1",
		Skills: []Entry{
			{
				ID:   "sdd-spec",
				Path: "sdd-spec",
				Source: Source{
					Type:     "core",
					Upstream: &Upstream{Owner: "gentleman-programming"},
				},
				Install: Install{
					DefaultScope: "global",
					Targets:      []string{"claude", "opencode", "codex"},
				},
				Lifecycle: Lifecycle{UpdateStrategy: "vendor-merge"},
			},
			{
				ID:   "my-custom",
				Path: "my-custom",
				Source: Source{
					Type: "custom",
				},
				Install: Install{
					DefaultScope: "global",
					Targets:      []string{"claude", "opencode", "codex"},
				},
				Lifecycle: Lifecycle{UpdateStrategy: "overlay-only"},
			},
		},
	}

	out := mustSerialize(t, reg)

	goldenPath := filepath.Join("testdata", "golden", "two_entry.yaml")

	if *update {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, out, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("golden file updated: %s", goldenPath)
		return
	}

	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update to create)", goldenPath, err)
	}
	if !bytes.Equal(golden, out) {
		t.Errorf("output does not match golden file %s\nwant:\n%s\ngot:\n%s", goldenPath, golden, out)
	}

	// The golden itself must re-parse clean (invariant check).
	reparsed := mustParseBytes(t, golden)
	if !reflect.DeepEqual(reg, reparsed) {
		t.Errorf("golden file does not round-trip: re-parsed result differs from original registry")
	}
}

// TestSerializeRejectsUnrepresentable verifies ADR-7: values containing forbidden
// characters must cause Serialize to return a non-nil error with no bytes.
func TestSerializeRejectsUnrepresentable(t *testing.T) {
	forbidden := []struct {
		name  string
		value string
	}{
		{"curly-open", "{open"},
		{"curly-close", "clos}e"},
		{"bracket-open", "[open"},
		{"bracket-close", "clos]e"},
		{"ampersand", "foo&bar"},
		{"asterisk", "foo*bar"},
		{"exclamation", "foo!bar"},
		{"tab", "foo\tbar"},
	}

	for _, tc := range forbidden {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Inject the forbidden value as the owner field (a representable scalar slot)
			// by constructing a core entry with a forbidden owner value.
			// We test via ID since slug guard in AddEntry would block this in normal flow;
			// direct struct construction bypasses that guard.
			reg := Registry{
				Version: "1",
				Skills: []Entry{
					{
						ID:   "test-entry",
						Path: "test-entry",
						Source: Source{
							Type:     "core",
							Upstream: &Upstream{Owner: tc.value},
						},
						Install: Install{
							DefaultScope: "global",
							Targets:      []string{"claude"},
						},
						Lifecycle: Lifecycle{UpdateStrategy: "vendor-merge"},
					},
				},
			}
			out, err := Serialize(reg)
			if err == nil {
				t.Errorf("expected non-nil error for forbidden value %q, got nil; output:\n%s", tc.value, out)
			}
			if len(out) != 0 {
				t.Errorf("expected nil bytes on error, got %d bytes", len(out))
			}
		})
	}
}

// TestSerializeRoundTripRealRegistry verifies SC-20: parse the real skills.registry.yaml,
// serialize, re-parse, and confirm DeepEqual for all entries.
func TestSerializeRoundTripRealRegistry(t *testing.T) {
	// Locate the real registry relative to the module root.
	// Running from engine/skills/, the registry is two levels up.
	registryPath := filepath.Join("..", "..", "skills.registry.yaml")

	data, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatalf("read real registry %s: %v", registryPath, err)
	}

	original, err := ParseRegistry(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("parse real registry: %v", err)
	}

	serialized, err := Serialize(original)
	if err != nil {
		t.Fatalf("Serialize real registry: %v", err)
	}

	reparsed, err := ParseRegistry(bytes.NewReader(serialized))
	if err != nil {
		t.Fatalf("re-parse serialized registry: %v\noutput:\n%s", err, serialized)
	}

	if !reflect.DeepEqual(original, reparsed) {
		t.Errorf("real registry did not round-trip\noriginal entries: %d\nreparsed entries: %d",
			len(original.Skills), len(reparsed.Skills))
		for i, e := range original.Skills {
			if i >= len(reparsed.Skills) {
				t.Errorf("  missing reparsed entry[%d]: %q", i, e.ID)
				continue
			}
			if !reflect.DeepEqual(e, reparsed.Skills[i]) {
				t.Errorf("  entry[%d] %q differs:\n  original: %+v\n  reparsed: %+v",
					i, e.ID, e, reparsed.Skills[i])
			}
		}
	}

	t.Logf("real registry round-tripped: %d entries", len(original.Skills))
}
