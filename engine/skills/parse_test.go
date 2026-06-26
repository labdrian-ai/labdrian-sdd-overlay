package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readTestFixture reads a YAML fixture from testdata/<name>.yaml.
func readTestFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name+".yaml"))
	if err != nil {
		t.Fatalf("fixture %q: %v", name, err)
	}
	return string(data)
}

// TestParseRegistry is the table-driven suite for ParseRegistry covering
// valid inputs (SC-01..SC-05) and all rejected-construct cases (ADR-1, R-001..R-012).
func TestParseRegistry(t *testing.T) {
	// --- valid cases ---

	t.Run("valid_core_and_custom", func(t *testing.T) {
		// SC-01: 2 entries, declaration order preserved, core first, custom second.
		reg, err := ParseRegistry(strings.NewReader(readTestFixture(t, "valid_core_and_custom")))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(reg.Skills) != 2 {
			t.Fatalf("len(Skills) = %d, want 2", len(reg.Skills))
		}
		// R-012: declaration order preserved.
		if reg.Skills[0].ID != "sdd-spec" {
			t.Errorf("Skills[0].ID = %q, want sdd-spec", reg.Skills[0].ID)
		}
		if reg.Skills[1].ID != "prespec-malandra" {
			t.Errorf("Skills[1].ID = %q, want prespec-malandra", reg.Skills[1].ID)
		}
		// SC-01: source types.
		if reg.Skills[0].Source.Type != "core" {
			t.Errorf("Skills[0].Source.Type = %q, want core", reg.Skills[0].Source.Type)
		}
		if reg.Skills[1].Source.Type != "custom" {
			t.Errorf("Skills[1].Source.Type = %q, want custom", reg.Skills[1].Source.Type)
		}
		// Upstream present for core.
		if reg.Skills[0].Source.Upstream == nil {
			t.Error("Skills[0].Source.Upstream is nil, want non-nil for core entry")
		} else if reg.Skills[0].Source.Upstream.Owner != "gentleman-programming" {
			t.Errorf("Skills[0].Source.Upstream.Owner = %q, want gentleman-programming",
				reg.Skills[0].Source.Upstream.Owner)
		}
		// Upstream absent for custom.
		if reg.Skills[1].Source.Upstream != nil {
			t.Error("Skills[1].Source.Upstream is non-nil, want nil for custom entry")
		}
		// Lifecycle strategies.
		if reg.Skills[0].Lifecycle.UpdateStrategy != "vendor-merge" {
			t.Errorf("Skills[0].Lifecycle.UpdateStrategy = %q, want vendor-merge",
				reg.Skills[0].Lifecycle.UpdateStrategy)
		}
		if reg.Skills[1].Lifecycle.UpdateStrategy != "overlay-only" {
			t.Errorf("Skills[1].Lifecycle.UpdateStrategy = %q, want overlay-only",
				reg.Skills[1].Lifecycle.UpdateStrategy)
		}
		// Install targets.
		if len(reg.Skills[0].Install.Targets) != 3 {
			t.Errorf("Skills[0].Install.Targets len = %d, want 3", len(reg.Skills[0].Install.Targets))
		}
		// Version.
		if reg.Version != "1" {
			t.Errorf("Version = %q, want 1", reg.Version)
		}
	})

	// --- error cases ---

	t.Run("missing_id", func(t *testing.T) {
		// SC-02: missing id → error, no partial data.
		reg, err := ParseRegistry(strings.NewReader(readTestFixture(t, "missing_id")))
		if err == nil {
			t.Fatal("expected non-nil error for missing id, got nil")
		}
		if len(reg.Skills) != 0 {
			t.Errorf("expected no entries on error, got %d", len(reg.Skills))
		}
	})

	t.Run("unknown_source_type", func(t *testing.T) {
		// SC-03: source.type: external → error (R-003).
		_, err := ParseRegistry(strings.NewReader(readTestFixture(t, "unknown_source_type")))
		if err == nil {
			t.Fatal("expected non-nil error for unknown source.type, got nil")
		}
	})

	t.Run("duplicate_id", func(t *testing.T) {
		// SC-04: error message must include the conflicting id (R-010).
		_, err := ParseRegistry(strings.NewReader(readTestFixture(t, "duplicate_id")))
		if err == nil {
			t.Fatal("expected non-nil error for duplicate id, got nil")
		}
		if !strings.Contains(err.Error(), "sdd-spec") {
			t.Errorf("error message %q does not reference the duplicate id %q", err.Error(), "sdd-spec")
		}
	})

	t.Run("upstream_on_custom_entry", func(t *testing.T) {
		// SC-05: custom entry with upstream block → error (R-004).
		_, err := ParseRegistry(strings.NewReader(readTestFixture(t, "upstream_on_custom_entry")))
		if err == nil {
			t.Fatal("expected non-nil error for upstream on custom entry, got nil")
		}
	})

	t.Run("missing_version", func(t *testing.T) {
		// R-001: absent version field → error.
		_, err := ParseRegistry(strings.NewReader(readTestFixture(t, "missing_version")))
		if err == nil {
			t.Fatal("expected non-nil error for missing version, got nil")
		}
	})

	t.Run("wrong_version", func(t *testing.T) {
		// R-001: version != "1" → error.
		_, err := ParseRegistry(strings.NewReader(readTestFixture(t, "wrong_version")))
		if err == nil {
			t.Fatal("expected non-nil error for wrong version, got nil")
		}
	})

	t.Run("empty_targets", func(t *testing.T) {
		// R-007: empty install.targets → error.
		_, err := ParseRegistry(strings.NewReader(readTestFixture(t, "empty_targets")))
		if err == nil {
			t.Fatal("expected non-nil error for empty targets, got nil")
		}
	})

	t.Run("invalid_target", func(t *testing.T) {
		// R-008: target not in {claude,opencode,codex} → error.
		_, err := ParseRegistry(strings.NewReader(readTestFixture(t, "invalid_target")))
		if err == nil {
			t.Fatal("expected non-nil error for invalid target value, got nil")
		}
	})

	t.Run("invalid_scope", func(t *testing.T) {
		// R-006: install.defaultScope != "global" → error.
		_, err := ParseRegistry(strings.NewReader(readTestFixture(t, "invalid_scope")))
		if err == nil {
			t.Fatal("expected non-nil error for invalid defaultScope, got nil")
		}
	})

	t.Run("invalid_lifecycle", func(t *testing.T) {
		// R-009: updateStrategy not in {vendor-merge,overlay-only} → error.
		_, err := ParseRegistry(strings.NewReader(readTestFixture(t, "invalid_lifecycle")))
		if err == nil {
			t.Fatal("expected non-nil error for invalid updateStrategy, got nil")
		}
	})

	t.Run("malformed_yaml_tab", func(t *testing.T) {
		// ADR-1: tab indentation → line-numbered error.
		_, err := ParseRegistry(strings.NewReader(readTestFixture(t, "malformed_yaml_tab")))
		if err == nil {
			t.Fatal("expected non-nil error for tab-indented YAML, got nil")
		}
	})

	t.Run("malformed_yaml_flow", func(t *testing.T) {
		// ADR-1: flow mapping {} → line-numbered error.
		_, err := ParseRegistry(strings.NewReader(readTestFixture(t, "malformed_yaml_flow")))
		if err == nil {
			t.Fatal("expected non-nil error for flow mapping YAML, got nil")
		}
	})

	t.Run("empty_file", func(t *testing.T) {
		// R-001: empty reader → missing version → error.
		_, err := ParseRegistry(strings.NewReader(""))
		if err == nil {
			t.Fatal("expected non-nil error for empty input, got nil")
		}
	})

	// --- FIX 1: inline trailing comments must be rejected (ADR-1) ---

	t.Run("inline_comment_unquoted_rejected", func(t *testing.T) {
		// Unquoted scalar with trailing " # ..." must error, not silently absorb the comment.
		yaml := "version: \"1\"\nskills:\n  - id: sdd-spec # my skill\n    path: sdd-spec\n    source:\n      type: custom\n    install:\n      defaultScope: global\n      targets:\n        - claude\n    lifecycle:\n      updateStrategy: overlay-only\n"
		_, err := ParseRegistry(strings.NewReader(yaml))
		if err == nil {
			t.Fatal("expected error for inline comment in unquoted value, got nil")
		}
		if !strings.Contains(err.Error(), "inline comment") {
			t.Errorf("error %q should mention inline comment", err.Error())
		}
	})

	t.Run("hash_inside_quoted_value_accepted", func(t *testing.T) {
		// A # inside a properly quoted value must NOT trigger the inline-comment error.
		yaml := "version: \"1\"\nskills:\n  - id: \"a # b\"\n    path: skill-path\n    source:\n      type: custom\n    install:\n      defaultScope: global\n      targets:\n        - claude\n    lifecycle:\n      updateStrategy: overlay-only\n"
		reg, err := ParseRegistry(strings.NewReader(yaml))
		if err != nil {
			t.Fatalf("unexpected error for # inside quoted value: %v", err)
		}
		if len(reg.Skills) != 1 || reg.Skills[0].ID != "a # b" {
			t.Errorf("expected ID %q, got Skills=%v", "a # b", reg.Skills)
		}
	})

	// --- FIX 2: unterminated quoted strings must error (ADR-1) ---

	t.Run("unterminated_double_quote", func(t *testing.T) {
		yaml := "version: \"1\"\nskills:\n  - id: \"unterminated\n    path: some-skill\n    source:\n      type: custom\n    install:\n      defaultScope: global\n      targets:\n        - claude\n    lifecycle:\n      updateStrategy: overlay-only\n"
		_, err := ParseRegistry(strings.NewReader(yaml))
		if err == nil {
			t.Fatal("expected error for unterminated double-quoted string, got nil")
		}
		if !strings.Contains(err.Error(), "unterminated") {
			t.Errorf("error %q should mention unterminated", err.Error())
		}
	})

	t.Run("unterminated_single_quote", func(t *testing.T) {
		yaml := "version: \"1\"\nskills:\n  - id: 'unterminated\n    path: some-skill\n    source:\n      type: custom\n    install:\n      defaultScope: global\n      targets:\n        - claude\n    lifecycle:\n      updateStrategy: overlay-only\n"
		_, err := ParseRegistry(strings.NewReader(yaml))
		if err == nil {
			t.Fatal("expected error for unterminated single-quoted string, got nil")
		}
		if !strings.Contains(err.Error(), "unterminated") {
			t.Errorf("error %q should mention unterminated", err.Error())
		}
	})

	// --- FIX 3: R-005 coverage — core entry, upstream present, owner empty ---

	t.Run("upstream_empty_owner_core_entry", func(t *testing.T) {
		// R-005: core entry with upstream block present but owner field empty → error.
		yaml := "version: \"1\"\nskills:\n  - id: some-core\n    path: some-core\n    source:\n      type: core\n      upstream:\n        owner:\n    install:\n      defaultScope: global\n      targets:\n        - claude\n    lifecycle:\n      updateStrategy: vendor-merge\n"
		_, err := ParseRegistry(strings.NewReader(yaml))
		if err == nil {
			t.Fatal("expected non-nil error for core entry with empty upstream.owner, got nil")
		}
	})

	// --- SUGGESTION-2: |-/|+ must emit "block scalars not supported", not a generic format error ---

	t.Run("block_scalar_chomping_clear_error", func(t *testing.T) {
		// ADR-1: |- and |+ are block scalar indicators; error must name them explicitly.
		yaml := "version: \"1\"\nskills:\n  - id: some-skill\n    path: some-skill\n    source:\n      type: custom\n    install:\n      defaultScope: global\n      targets:\n        - claude\n    lifecycle:\n      updateStrategy: |-\n        overlay-only\n"
		_, err := ParseRegistry(strings.NewReader(yaml))
		if err == nil {
			t.Fatal("expected error for block scalar indicator |-, got nil")
		}
		if !strings.Contains(err.Error(), "block scalar") {
			t.Errorf("error %q should mention block scalars, not generic format error", err.Error())
		}
	})

	// --- SC-10..SC-14: project scope + allowedProjects (T-02) ---

	t.Run("SC-10_project_scope_with_allowedProjects", func(t *testing.T) {
		// SC-10: defaultScope: project + allowedProjects → accepted; fields populated.
		yaml := "version: \"1\"\nskills:\n  - id: my-skill\n    path: my-skill\n    source:\n      type: custom\n    install:\n      defaultScope: project\n      targets:\n        - claude\n      allowedProjects:\n        - labdrian-sdd-overlay\n        - other-repo\n    lifecycle:\n      updateStrategy: overlay-only\n"
		reg, err := ParseRegistry(strings.NewReader(yaml))
		if err != nil {
			t.Fatalf("SC-10: unexpected error: %v", err)
		}
		if len(reg.Skills) != 1 {
			t.Fatalf("SC-10: len(Skills) = %d, want 1", len(reg.Skills))
		}
		e := reg.Skills[0]
		if e.Install.DefaultScope != "project" {
			t.Errorf("SC-10: DefaultScope = %q, want project", e.Install.DefaultScope)
		}
		want := []string{"labdrian-sdd-overlay", "other-repo"}
		if len(e.Install.AllowedProjects) != len(want) {
			t.Fatalf("SC-10: AllowedProjects = %v, want %v", e.Install.AllowedProjects, want)
		}
		for i, v := range want {
			if e.Install.AllowedProjects[i] != v {
				t.Errorf("SC-10: AllowedProjects[%d] = %q, want %q", i, e.Install.AllowedProjects[i], v)
			}
		}
	})

	t.Run("SC-11_unknown_scope_rejected", func(t *testing.T) {
		// SC-11: defaultScope: workspace → error with line number and invalid value.
		yaml := "version: \"1\"\nskills:\n  - id: bad-skill\n    path: bad-skill\n    source:\n      type: custom\n    install:\n      defaultScope: workspace\n      targets:\n        - claude\n    lifecycle:\n      updateStrategy: overlay-only\n"
		_, err := ParseRegistry(strings.NewReader(yaml))
		if err == nil {
			t.Fatal("SC-11: expected non-nil error for unknown defaultScope, got nil")
		}
		if !strings.Contains(err.Error(), "workspace") {
			t.Errorf("SC-11: error %q should contain the invalid value %q", err.Error(), "workspace")
		}
		// R-040 MUST: error must include the line number of the bad defaultScope line.
		// In the inline YAML above, "defaultScope: workspace" is on line 8.
		if !strings.Contains(err.Error(), "line 8") {
			t.Errorf("SC-11: error %q should contain the line number (line 8)", err.Error())
		}
	})

	t.Run("SC-12_allowedProjects_forbidden_on_global", func(t *testing.T) {
		// SC-12: defaultScope: global + allowedProjects present → error referencing entry id.
		yaml := "version: \"1\"\nskills:\n  - id: global-skill\n    path: global-skill\n    source:\n      type: custom\n    install:\n      defaultScope: global\n      targets:\n        - claude\n      allowedProjects:\n        - some-repo\n    lifecycle:\n      updateStrategy: overlay-only\n"
		_, err := ParseRegistry(strings.NewReader(yaml))
		if err == nil {
			t.Fatal("SC-12: expected non-nil error for global + allowedProjects, got nil")
		}
		if !strings.Contains(err.Error(), "global-skill") {
			t.Errorf("SC-12: error %q should reference entry id %q", err.Error(), "global-skill")
		}
		if !strings.Contains(err.Error(), "allowedProjects") {
			t.Errorf("SC-12: error %q should mention allowedProjects", err.Error())
		}
	})

	t.Run("SC-13_project_scope_no_allowedProjects", func(t *testing.T) {
		// SC-13: defaultScope: project with no allowedProjects key → valid, AllowedProjects == nil.
		yaml := "version: \"1\"\nskills:\n  - id: proj-skill\n    path: proj-skill\n    source:\n      type: custom\n    install:\n      defaultScope: project\n      targets:\n        - claude\n    lifecycle:\n      updateStrategy: overlay-only\n"
		reg, err := ParseRegistry(strings.NewReader(yaml))
		if err != nil {
			t.Fatalf("SC-13: unexpected error: %v", err)
		}
		if len(reg.Skills) != 1 {
			t.Fatalf("SC-13: len(Skills) = %d, want 1", len(reg.Skills))
		}
		if reg.Skills[0].Install.AllowedProjects != nil {
			t.Errorf("SC-13: AllowedProjects = %v, want nil", reg.Skills[0].Install.AllowedProjects)
		}
	})

	t.Run("T07_valid_project_scoped_fixture", func(t *testing.T) {
		// T-07: the testdata project-scoped fixture parses cleanly and produces
		// one entry with defaultScope=project and allowedProjects=[labdrian-sdd-overlay].
		reg, err := ParseRegistry(strings.NewReader(readTestFixture(t, "valid_project_scoped")))
		if err != nil {
			t.Fatalf("T-07 fixture parse error: %v", err)
		}
		if len(reg.Skills) != 1 {
			t.Fatalf("len(Skills) = %d, want 1", len(reg.Skills))
		}
		e := reg.Skills[0]
		if e.Install.DefaultScope != "project" {
			t.Errorf("DefaultScope = %q, want project", e.Install.DefaultScope)
		}
		if len(e.Install.AllowedProjects) != 1 || e.Install.AllowedProjects[0] != "labdrian-sdd-overlay" {
			t.Errorf("AllowedProjects = %v, want [labdrian-sdd-overlay]", e.Install.AllowedProjects)
		}
	})
}
