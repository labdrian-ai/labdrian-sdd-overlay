package skills

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoad verifies the Load file adapter (R-022, R-023).
func TestLoad(t *testing.T) {
	t.Run("valid_file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "registry.yaml")
		content := `version: "1"
skills:
  - id: test-skill
    path: test-skill
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		reg, err := Load(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(reg.Skills) != 1 {
			t.Errorf("len(Skills) = %d, want 1", len(reg.Skills))
		}
		if reg.Skills[0].ID != "test-skill" {
			t.Errorf("Skills[0].ID = %q, want test-skill", reg.Skills[0].ID)
		}
	})

	t.Run("missing_file", func(t *testing.T) {
		// R-022: unreadable registry → non-nil error.
		dir := t.TempDir()
		path := filepath.Join(dir, "nonexistent.yaml")
		_, err := Load(path)
		if err == nil {
			t.Fatal("expected non-nil error for missing file, got nil")
		}
	})

	t.Run("invalid_yaml", func(t *testing.T) {
		// Parse error propagates through Load.
		dir := t.TempDir()
		path := filepath.Join(dir, "registry.yaml")
		// Tab indentation is rejected by the YAML subset parser.
		content := "version: \"1\"\nskills:\n\t- id: bad\n"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		_, err := Load(path)
		if err == nil {
			t.Fatal("expected non-nil error for invalid YAML, got nil")
		}
	})
}
