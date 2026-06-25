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
}
