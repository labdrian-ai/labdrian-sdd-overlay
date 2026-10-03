package registryyaml_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/registryyaml"
)

// update rewrites the golden file: go test ./skills/registryyaml -run TestEncodeGoldenTwoEntry -update
var update = flag.Bool("update", false, "update golden files")

// mustParseBytes is a test helper that parses YAML bytes or fatals.
func mustParseBytes(t *testing.T, data []byte) skills.Registry {
	t.Helper()
	reg, err := readRegistryBytes(data)
	if err != nil {
		t.Fatalf("reading the registry: %v", err)
	}
	return reg
}

// mustEncode is a test helper that encodes a registry or fatals.
func mustEncode(t *testing.T, reg skills.Registry) []byte {
	t.Helper()
	out, err := registryyaml.Encode(reg)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	return out
}

// TestEncodeRoundTripAllowedProjects verifies SC-21: a project-scoped entry with
// allowedProjects round-trips through serialize → parse with DeepEqual.
func TestEncodeRoundTripAllowedProjects(t *testing.T) {
	reg := skills.Registry{
		Version: "1",
		Skills: []skills.Entry{
			{
				ID:   "prespec-malandra",
				Path: "prespec-malandra",
				Source: skills.Source{
					Type: "custom",
				},
				Install: skills.Install{
					DefaultScope:    "project",
					Targets:         []string{"claude"},
					AllowedProjects: []string{"labdrian-sdd-overlay"},
				},
				Lifecycle: skills.Lifecycle{
					UpdateStrategy: "overlay-only",
				},
			},
		},
	}

	out := mustEncode(t, reg)
	got := mustParseBytes(t, out)

	if !reflect.DeepEqual(reg, got) {
		t.Errorf("round-trip mismatch\noriginal: %+v\ngot:      %+v", reg, got)
	}
}

// TestEncodeDeterministic verifies SC-22: two calls on the same Registry produce
// identical byte slices.
func TestEncodeDeterministic(t *testing.T) {
	reg := skills.Registry{
		Version: "1",
		Skills: []skills.Entry{
			{
				ID:   "sdd-spec",
				Path: "sdd-spec",
				Source: skills.Source{
					Type:     "core",
					Upstream: &skills.Upstream{Owner: "gentleman-programming"},
				},
				Install: skills.Install{
					DefaultScope: "global",
					Targets:      []string{"claude", "opencode", "codex"},
				},
				Lifecycle: skills.Lifecycle{UpdateStrategy: "vendor-merge"},
			},
			{
				ID:   "my-custom",
				Path: "my-custom",
				Source: skills.Source{
					Type: "custom",
				},
				Install: skills.Install{
					DefaultScope: "global",
					Targets:      []string{"claude", "opencode", "codex"},
				},
				Lifecycle: skills.Lifecycle{UpdateStrategy: "overlay-only"},
			},
		},
	}

	a := mustEncode(t, reg)
	b := mustEncode(t, reg)

	if !bytes.Equal(a, b) {
		t.Errorf("Serialize is not deterministic:\nfirst call:\n%s\nsecond call:\n%s", a, b)
	}
}

// TestEncodeNoUpstreamForCustom verifies SC-23: output for a custom entry must not
// contain the string "upstream".
func TestEncodeNoUpstreamForCustom(t *testing.T) {
	reg := skills.Registry{
		Version: "1",
		Skills: []skills.Entry{
			{
				ID:   "my-custom",
				Path: "my-custom",
				Source: skills.Source{
					Type:     "custom",
					Upstream: nil,
				},
				Install: skills.Install{
					DefaultScope: "global",
					Targets:      []string{"claude", "opencode", "codex"},
				},
				Lifecycle: skills.Lifecycle{UpdateStrategy: "overlay-only"},
			},
		},
	}

	out := mustEncode(t, reg)
	if strings.Contains(string(out), "upstream") {
		t.Errorf("output for custom entry must not contain 'upstream', got:\n%s", out)
	}
}

// TestEncodeUpstreamForCore verifies SC-24: a serialized core entry re-parsed has a
// non-nil Source.Upstream with a non-empty Owner.
func TestEncodeUpstreamForCore(t *testing.T) {
	reg := skills.Registry{
		Version: "1",
		Skills: []skills.Entry{
			{
				ID:   "sdd-spec",
				Path: "sdd-spec",
				Source: skills.Source{
					Type:     "core",
					Upstream: &skills.Upstream{Owner: "gentleman-programming"},
				},
				Install: skills.Install{
					DefaultScope: "global",
					Targets:      []string{"claude", "opencode", "codex"},
				},
				Lifecycle: skills.Lifecycle{UpdateStrategy: "vendor-merge"},
			},
		},
	}

	out := mustEncode(t, reg)
	got := mustParseBytes(t, out)

	if got.Skills[0].Source.Upstream == nil {
		t.Fatal("re-parsed core entry: Source.Upstream is nil, want non-nil")
	}
	if got.Skills[0].Source.Upstream.Owner != "gentleman-programming" {
		t.Errorf("re-parsed upstream.owner = %q, want %q", got.Skills[0].Source.Upstream.Owner, "gentleman-programming")
	}
}

// TestEncodeGoldenTwoEntry verifies SC-25: the serialized form of a two-entry registry
// matches a golden file. Run with -update to write the golden file.
func TestEncodeGoldenTwoEntry(t *testing.T) {
	reg := skills.Registry{
		Version: "1",
		Skills: []skills.Entry{
			{
				ID:   "sdd-spec",
				Path: "sdd-spec",
				Source: skills.Source{
					Type:     "core",
					Upstream: &skills.Upstream{Owner: "gentleman-programming"},
				},
				Install: skills.Install{
					DefaultScope: "global",
					Targets:      []string{"claude", "opencode", "codex"},
				},
				Lifecycle: skills.Lifecycle{UpdateStrategy: "vendor-merge"},
			},
			{
				ID:   "my-custom",
				Path: "my-custom",
				Source: skills.Source{
					Type: "custom",
				},
				Install: skills.Install{
					DefaultScope: "global",
					Targets:      []string{"claude", "opencode", "codex"},
				},
				Lifecycle: skills.Lifecycle{UpdateStrategy: "overlay-only"},
			},
		},
	}

	out := mustEncode(t, reg)

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

// TestEncodeRejectsUnrepresentable verifies ADR-7: values containing forbidden
// characters must cause Serialize to return a non-nil error with no bytes.
func TestEncodeRejectsUnrepresentable(t *testing.T) {
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
			reg := skills.Registry{
				Version: "1",
				Skills: []skills.Entry{
					{
						ID:   "test-entry",
						Path: "test-entry",
						Source: skills.Source{
							Type:     "core",
							Upstream: &skills.Upstream{Owner: tc.value},
						},
						Install: skills.Install{
							DefaultScope: "global",
							Targets:      []string{"claude"},
						},
						Lifecycle: skills.Lifecycle{UpdateStrategy: "vendor-merge"},
					},
				},
			}
			out, err := registryyaml.Encode(reg)
			if err == nil {
				t.Errorf("expected non-nil error for forbidden value %q, got nil; output:\n%s", tc.value, out)
			}
			if len(out) != 0 {
				t.Errorf("expected nil bytes on error, got %d bytes", len(out))
			}
		})
	}
}

// TestEncodeRoundTripRealRegistry verifies SC-20: parse the real skills.registry.yaml,
// serialize, re-parse, and confirm DeepEqual for all entries.
func TestEncodeRoundTripRealRegistry(t *testing.T) {
	// Locate the real registry relative to the module root.
	// Running from engine/skills/, the registry is two levels up.
	registryPath := filepath.Join("..", "..", "..", "skills.registry.yaml")

	data, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatalf("read real registry %s: %v", registryPath, err)
	}

	original, err := readRegistryBytes(data)
	if err != nil {
		t.Fatalf("parse real registry: %v", err)
	}

	serialized, err := registryyaml.Encode(original)
	if err != nil {
		t.Fatalf("Serialize real registry: %v", err)
	}

	reparsed, err := readRegistryBytes(serialized)
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

// externalEntry returns a minimal valid external Registry entry for use in tests.
func externalEntry(id, repo, ref string) skills.Entry {
	return skills.Entry{
		ID:   id,
		Path: id,
		Source: skills.Source{
			Type: "external",
			Repo: repo,
			Ref:  ref,
		},
		Install: skills.Install{
			DefaultScope: "global",
			Targets:      []string{"claude"},
		},
		Lifecycle: skills.Lifecycle{UpdateStrategy: "overlay-only"},
	}
}

// TestEncodeRoundTripExternal verifies SC-60: parse(serialize(r)) == r for a Registry
// containing an external entry with both Repo and Ref set.
func TestEncodeRoundTripExternal(t *testing.T) {
	reg := skills.Registry{
		Version: "1",
		Skills:  []skills.Entry{externalEntry("my-ext-skill", "https://github.com/example/skills", "a1b2c3d")},
	}

	out := mustEncode(t, reg)

	// Serialized bytes must contain "repo:" and "ref:" under the source block.
	if !strings.Contains(string(out), "repo:") {
		t.Errorf("SC-60: serialized output does not contain 'repo:'\n%s", out)
	}
	if !strings.Contains(string(out), "ref:") {
		t.Errorf("SC-60: serialized output does not contain 'ref:'\n%s", out)
	}

	parsed := mustParseBytes(t, out)

	if !reflect.DeepEqual(reg, parsed) {
		t.Errorf("SC-60: round-trip mismatch\noriginal: %+v\ngot:      %+v", reg, parsed)
	}
}

// TestEncodeExternalRejectsForbiddenRepo verifies SC-61: a repo value containing a
// forbidden character causes Serialize to return a non-nil error naming the entry id
// and "source.repo".
func TestEncodeExternalRejectsForbiddenRepo(t *testing.T) {
	reg := skills.Registry{
		Version: "1",
		Skills:  []skills.Entry{externalEntry("bad-entry", "https://example.com/{repo}", "")},
	}

	out, err := registryyaml.Encode(reg)
	if err == nil {
		t.Fatalf("SC-61: expected non-nil error for repo with forbidden char, got nil; output:\n%s", out)
	}
	if len(out) != 0 {
		t.Errorf("SC-61: expected nil bytes on error, got %d bytes", len(out))
	}
	if !strings.Contains(err.Error(), "bad-entry") {
		t.Errorf("SC-61: error %q should name the entry id", err.Error())
	}
	if !strings.Contains(err.Error(), "source.repo") {
		t.Errorf("SC-61: error %q should mention source.repo", err.Error())
	}
}
