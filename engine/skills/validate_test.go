package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiff(t *testing.T) {
	// makeReg builds a Registry with the given entries (path = id).
	makeReg := func(entries []Entry) Registry {
		return Registry{Version: "1", Skills: entries}
	}
	// coreEntry returns a core Entry with the given path/id.
	coreEntry := func(id string) Entry {
		return Entry{
			ID:   id,
			Path: id,
			Source: Source{
				Type:     "core",
				Upstream: &Upstream{Owner: "gentleman-programming"},
			},
			Install:   Install{DefaultScope: "global", Targets: []string{"claude", "opencode", "codex"}},
			Lifecycle: Lifecycle{UpdateStrategy: "vendor-merge"},
		}
	}
	// customEntry returns a custom Entry with the given path/id.
	customEntry := func(id string) Entry {
		return Entry{
			ID:        id,
			Path:      id,
			Source:    Source{Type: "custom"},
			Install:   Install{DefaultScope: "global", Targets: []string{"claude", "opencode", "codex"}},
			Lifecycle: Lifecycle{UpdateStrategy: "overlay-only"},
		}
	}

	t.Run("aligned_registry_and_manifest", func(t *testing.T) {
		reg := makeReg([]Entry{coreEntry("sdd-spec"), customEntry("gadu-orchestrate")})
		mv := ManifestView{
			"sdd-spec":        {Dir: "sdd-spec", Tag: "managed"},
			"gadu-orchestrate": {Dir: "gadu-orchestrate", Tag: "custom"},
		}
		divs := Diff(reg, mv)
		if len(divs) != 0 {
			t.Errorf("expected no divergences, got %d: %v", len(divs), divs)
		}
	})

	t.Run("registry_entry_not_in_manifest", func(t *testing.T) {
		reg := makeReg([]Entry{customEntry("new-skill")})
		mv := ManifestView{} // manifest has nothing
		divs := Diff(reg, mv)
		if len(divs) != 1 {
			t.Fatalf("expected 1 divergence, got %d: %v", len(divs), divs)
		}
		if divs[0].Class != DivMissingInManifest {
			t.Errorf("expected MISSING_IN_MANIFEST, got %s", divs[0].Class)
		}
		if !strings.Contains(divs[0].Path, "new-skill") {
			t.Errorf("expected path to contain new-skill, got %q", divs[0].Path)
		}
	})

	t.Run("manifest_skill_not_in_registry", func(t *testing.T) {
		reg := makeReg(nil) // empty registry
		mv := ManifestView{
			"orphan-skill": {Dir: "orphan-skill", Tag: "custom"},
		}
		divs := Diff(reg, mv)
		if len(divs) != 1 {
			t.Fatalf("expected 1 divergence, got %d: %v", len(divs), divs)
		}
		if divs[0].Class != DivMissingInRegistry {
			t.Errorf("expected MISSING_IN_REGISTRY, got %s", divs[0].Class)
		}
		if divs[0].Path != "orphan-skill" {
			t.Errorf("expected path orphan-skill, got %q", divs[0].Path)
		}
	})

	t.Run("non_skill_rows_ignored", func(t *testing.T) {
		// Registry and manifest are aligned on the one skill; infra rows do not appear
		// in ManifestView (LoadManifestView already filters them), so Diff sees only skills.
		reg := makeReg([]Entry{coreEntry("sdd-spec")})
		mv := ManifestView{
			"sdd-spec": {Dir: "sdd-spec", Tag: "managed"},
			// infra rows would not be here after LoadManifestView — verified end-to-end
		}
		divs := Diff(reg, mv)
		if len(divs) != 0 {
			t.Errorf("expected 0 divergences, got %d: %v", len(divs), divs)
		}
	})

	t.Run("tag_mismatch", func(t *testing.T) {
		// registry says core, manifest says custom
		reg := makeReg([]Entry{coreEntry("sdd-spec")})
		mv := ManifestView{
			"sdd-spec": {Dir: "sdd-spec", Tag: "custom"}, // mismatch
		}
		divs := Diff(reg, mv)
		if len(divs) != 1 {
			t.Fatalf("expected 1 divergence, got %d: %v", len(divs), divs)
		}
		if divs[0].Class != DivTagMismatch {
			t.Errorf("expected TAG_MISMATCH, got %s", divs[0].Class)
		}
		if !strings.Contains(divs[0].Path, "sdd-spec") {
			t.Errorf("expected path to contain sdd-spec, got %q", divs[0].Path)
		}
	})

	t.Run("all_divergences_collected_not_first_error", func(t *testing.T) {
		// Two missing-in-manifest entries; both must be reported (full scan).
		reg := makeReg([]Entry{customEntry("skill-a"), customEntry("skill-b")})
		mv := ManifestView{} // neither present
		divs := Diff(reg, mv)
		if len(divs) != 2 {
			t.Errorf("expected 2 divergences (full scan), got %d: %v", len(divs), divs)
		}
	})

	t.Run("mixed_tag_emitted", func(t *testing.T) {
		// A manifest dir with HasConflict = true must produce DivMixedTag.
		// Tag still matches the registry (managed/core), so no TAG_MISMATCH, only MIXED_TAG.
		reg := makeReg([]Entry{coreEntry("foo")})
		mv := ManifestView{
			"foo": {Dir: "foo", Tag: "managed", HasConflict: true},
		}
		divs := Diff(reg, mv)
		var hasMixed bool
		for _, d := range divs {
			if d.Class == DivMixedTag && d.Path == "foo" {
				hasMixed = true
			}
		}
		if !hasMixed {
			t.Errorf("expected DivMixedTag for foo, got: %v", divs)
		}
	})

	t.Run("mixed_tag_detail_names_dir", func(t *testing.T) {
		// The DivMixedTag divergence detail must name the conflicting dir.
		reg := makeReg([]Entry{customEntry("conflict-dir")})
		mv := ManifestView{
			"conflict-dir": {Dir: "conflict-dir", Tag: "custom", HasConflict: true},
		}
		divs := Diff(reg, mv)
		for _, d := range divs {
			if d.Class == DivMixedTag {
				if !strings.Contains(d.Detail, "conflict-dir") {
					t.Errorf("DivMixedTag detail must name the dir; got %q", d.Detail)
				}
				return
			}
		}
		t.Errorf("expected DivMixedTag divergence, got: %v", divs)
	})

	t.Run("no_conflict_no_mixed_tag", func(t *testing.T) {
		// A manifest dir with HasConflict = false must NOT produce DivMixedTag.
		reg := makeReg([]Entry{coreEntry("clean-dir")})
		mv := ManifestView{
			"clean-dir": {Dir: "clean-dir", Tag: "managed", HasConflict: false},
		}
		divs := Diff(reg, mv)
		for _, d := range divs {
			if d.Class == DivMixedTag {
				t.Errorf("expected no DivMixedTag for clean dir, got one: %v", d)
			}
		}
	})

	t.Run("mixed_tag_uncovered_by_registry", func(t *testing.T) {
		// A manifest dir with HasConflict = true but no registry entry emits
		// both DivMissingInRegistry and DivMixedTag.
		reg := makeReg(nil)
		mv := ManifestView{
			"orphan-conflict": {Dir: "orphan-conflict", Tag: "managed", HasConflict: true},
		}
		divs := Diff(reg, mv)
		hasMissing := false
		hasMixed := false
		for _, d := range divs {
			if d.Class == DivMissingInRegistry && d.Path == "orphan-conflict" {
				hasMissing = true
			}
			if d.Class == DivMixedTag && d.Path == "orphan-conflict" {
				hasMixed = true
			}
		}
		if !hasMissing {
			t.Errorf("expected DivMissingInRegistry for orphan-conflict, got: %v", divs)
		}
		if !hasMixed {
			t.Errorf("expected DivMixedTag for orphan-conflict, got: %v", divs)
		}
	})
}

// TestTagMatchesSourceType verifies SC-62: tagMatchesSourceType must treat
// "external" as matching the "custom" manifest tag (ADR-13, R-122).
func TestTagMatchesSourceType(t *testing.T) {
	tests := []struct {
		tag        string
		sourceType string
		want       bool
	}{
		// SC-62: external matches custom tag
		{tag: "custom", sourceType: "external", want: true},
		{tag: "managed", sourceType: "external", want: false},
		// Regression: existing mappings unchanged
		{tag: "managed", sourceType: "core", want: true},
		{tag: "custom", sourceType: "custom", want: true},
		{tag: "managed", sourceType: "custom", want: false},
		{tag: "custom", sourceType: "core", want: false},
		{tag: "unknown", sourceType: "core", want: false},
	}
	for _, tt := range tests {
		got := tagMatchesSourceType(tt.tag, tt.sourceType)
		if got != tt.want {
			t.Errorf("tagMatchesSourceType(%q, %q) = %v, want %v", tt.tag, tt.sourceType, got, tt.want)
		}
	}
}

// TestDiffExternalEntry verifies SC-63: a registry with an external entry
// aligned against a manifest tag of "custom" must produce zero divergences.
func TestDiffExternalEntry(t *testing.T) {
	extEntry := Entry{
		ID:        "my-ext-skill",
		Path:      "my-ext-skill",
		Source:    Source{Type: "external", Repo: "https://github.com/example/skills", Ref: "a1b2c3d"},
		Install:   Install{DefaultScope: "global", Targets: []string{"claude"}},
		Lifecycle: Lifecycle{UpdateStrategy: "overlay-only"},
	}

	t.Run("SC-63_external_aligned_custom_tag", func(t *testing.T) {
		reg := Registry{Version: "1", Skills: []Entry{extEntry}}
		mv := ManifestView{
			"my-ext-skill": {Dir: "my-ext-skill", Tag: "custom"},
		}
		divs := Diff(reg, mv)
		if len(divs) != 0 {
			t.Errorf("expected zero divergences for external entry with custom tag, got %d: %v", len(divs), divs)
		}
	})

	t.Run("SC-63_external_managed_tag_produces_mismatch", func(t *testing.T) {
		reg := Registry{Version: "1", Skills: []Entry{extEntry}}
		mv := ManifestView{
			"my-ext-skill": {Dir: "my-ext-skill", Tag: "managed"},
		}
		divs := Diff(reg, mv)
		if len(divs) != 1 || divs[0].Class != DivTagMismatch {
			t.Errorf("expected TAG_MISMATCH for external+managed, got %v", divs)
		}
	})
}

func TestValidate(t *testing.T) {
	coreEntry := func(id string) Entry {
		return Entry{
			ID:   id,
			Path: id,
			Source: Source{
				Type:     "core",
				Upstream: &Upstream{Owner: "gentleman-programming"},
			},
			Install:   Install{DefaultScope: "global", Targets: []string{"claude"}},
			Lifecycle: Lifecycle{UpdateStrategy: "vendor-merge"},
		}
	}

	t.Run("aligned_returns_nil_error", func(t *testing.T) {
		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "overlay.manifest")
		content := "sdd-spec/SKILL.md managed\n"
		if err := os.WriteFile(manifestPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		reg := Registry{Version: "1", Skills: []Entry{coreEntry("sdd-spec")}}
		divs, err := Validate(reg, manifestPath)
		if err != nil {
			t.Errorf("expected nil error for aligned input, got: %v", err)
		}
		if len(divs) != 0 {
			t.Errorf("expected no divergences, got %d", len(divs))
		}
	})

	t.Run("diverged_returns_error_with_list", func(t *testing.T) {
		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "overlay.manifest")
		// manifest is empty; registry has an entry → MISSING_IN_MANIFEST
		if err := os.WriteFile(manifestPath, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
		reg := Registry{Version: "1", Skills: []Entry{coreEntry("sdd-spec")}}
		divs, err := Validate(reg, manifestPath)
		if err == nil {
			t.Error("expected non-nil error for diverged input")
		}
		if len(divs) == 0 {
			t.Error("expected at least one divergence")
		}
		if !strings.Contains(err.Error(), "sdd-spec") {
			t.Errorf("expected error message to contain sdd-spec, got: %v", err)
		}
	})

	t.Run("missing_manifest_file_returns_error", func(t *testing.T) {
		reg := Registry{}
		_, err := Validate(reg, "/nonexistent/overlay.manifest")
		if err == nil {
			t.Error("expected error for missing manifest file")
		}
	})

	t.Run("mixed_tag_manifest_reports_mixed_tag_divergence", func(t *testing.T) {
		// End-to-end: a manifest file with conflicting SKILL.md rows for the same dir
		// must cause Validate to return a DivMixedTag divergence and a non-nil error.
		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "overlay.manifest")
		// "foo" has both managed and custom SKILL.md rows.
		content := "foo/SKILL.md managed\nfoo/SKILL.md custom\n"
		if err := os.WriteFile(manifestPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		reg := Registry{Version: "1", Skills: []Entry{coreEntry("foo")}}
		divs, err := Validate(reg, manifestPath)
		if err == nil {
			t.Error("expected non-nil error for mixed-tag manifest")
		}
		var hasMixed bool
		for _, d := range divs {
			if d.Class == DivMixedTag {
				hasMixed = true
				if !strings.Contains(d.Detail, "foo") {
					t.Errorf("DivMixedTag detail must name the dir; got %q", d.Detail)
				}
			}
		}
		if !hasMixed {
			t.Errorf("expected DivMixedTag in divergences, got: %v", divs)
		}
	})
}
