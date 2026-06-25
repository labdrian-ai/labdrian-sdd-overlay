package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifestView(t *testing.T) {
	t.Run("skill_dirs_extracted", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "overlay.manifest")
		content := "sdd-spec/SKILL.md managed\ngadu-orchestrate/SKILL.md custom\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		mv, err := LoadManifestView(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(mv) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(mv))
		}
		if _, ok := mv["sdd-spec"]; !ok {
			t.Error("expected entry for sdd-spec")
		}
		if _, ok := mv["gadu-orchestrate"]; !ok {
			t.Error("expected entry for gadu-orchestrate")
		}
	})

	t.Run("infra_prefixes_excluded", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "overlay.manifest")
		content := "engine/cmd/main.go managed\n_shared/minimalism-contract.md custom\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		mv, err := LoadManifestView(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(mv) != 0 {
			t.Fatalf("expected 0 entries, got %d: %v", len(mv), mv)
		}
	})

	t.Run("non_skill_md_rows_ignored", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "overlay.manifest")
		// SKILL.md row creates the entry; references row does not create a separate entry
		content := "genesis-design-system/SKILL.md custom\ngenesis-design-system/references/forms.md custom\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		mv, err := LoadManifestView(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(mv) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(mv))
		}
		if _, ok := mv["genesis-design-system"]; !ok {
			t.Error("expected entry for genesis-design-system")
		}
	})

	t.Run("managed_maps_to_core", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "overlay.manifest")
		content := "sdd-spec/SKILL.md managed\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		mv, err := LoadManifestView(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		e, ok := mv["sdd-spec"]
		if !ok {
			t.Fatal("expected entry for sdd-spec")
		}
		if e.Tag != "managed" {
			t.Errorf("expected tag managed, got %q", e.Tag)
		}
	})

	t.Run("custom_maps_to_custom", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "overlay.manifest")
		content := "gadu-orchestrate/SKILL.md custom\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		mv, err := LoadManifestView(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		e, ok := mv["gadu-orchestrate"]
		if !ok {
			t.Fatal("expected entry for gadu-orchestrate")
		}
		if e.Tag != "custom" {
			t.Errorf("expected tag custom, got %q", e.Tag)
		}
	})

	t.Run("missing_file", func(t *testing.T) {
		_, err := LoadManifestView("/nonexistent/path/overlay.manifest")
		if err == nil {
			t.Error("expected error for missing file, got nil")
		}
	})

	t.Run("blank_and_comment_lines_skipped", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "overlay.manifest")
		content := "# this is a comment\n\nsdd-spec/SKILL.md managed\n\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		mv, err := LoadManifestView(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(mv) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(mv))
		}
	})

	t.Run("mixed_tags_flagged", func(t *testing.T) {
		// Two SKILL.md rows for the same dir with different tags must set HasConflict = true.
		dir := t.TempDir()
		path := filepath.Join(dir, "overlay.manifest")
		content := "foo/SKILL.md managed\nfoo/SKILL.md custom\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		mv, err := LoadManifestView(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		e, ok := mv["foo"]
		if !ok {
			t.Fatal("expected entry for foo")
		}
		if !e.HasConflict {
			t.Errorf("expected HasConflict = true when rows carry mixed tags, got false")
		}
	})

	t.Run("same_tag_not_flagged", func(t *testing.T) {
		// Two SKILL.md rows for the same dir with the same tag must NOT set HasConflict.
		dir := t.TempDir()
		path := filepath.Join(dir, "overlay.manifest")
		content := "bar/SKILL.md managed\nbar/SKILL.md managed\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		mv, err := LoadManifestView(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		e, ok := mv["bar"]
		if !ok {
			t.Fatal("expected entry for bar")
		}
		if e.HasConflict {
			t.Errorf("expected HasConflict = false when rows carry the same tag, got true")
		}
	})
}
