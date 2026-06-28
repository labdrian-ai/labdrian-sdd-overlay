package gadu_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gadu"
)

// repoRoot returns the overlay repo root, two levels above engine/gadu/.
func repoRoot(t *testing.T) string {
	t.Helper()
	// engine/gadu/gadu_test.go → engine/ → repo root
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Join(wd, "..", "..")
}

// TestGenerate_BothFilesEmitted verifies that Generate writes both delivery
// artifacts into the repo-root-shaped tempdir.
func TestGenerate_BothFilesEmitted(t *testing.T) {
	dir := t.TempDir()
	if err := gadu.Generate(dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	agentPath := filepath.Join(dir, "agents", "GADU.md")
	skillPath := filepath.Join(dir, "skills", "gadu-operator", "SKILL.md")

	for _, p := range []string{agentPath, skillPath} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected file to exist: %s — %v", p, err)
		}
	}
}

// TestGenerate_BodyIsIdentical verifies that both emitted files contain the
// canonical persona body bytes byte-for-byte (D7: same body bytes in both outputs).
func TestGenerate_BodyIsIdentical(t *testing.T) {
	dir := t.TempDir()
	if err := gadu.Generate(dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	canonicalBody := gadu.PersonaBody()
	if canonicalBody == "" {
		t.Fatal("PersonaBody() returned empty — embedded body.md may be missing")
	}

	agentBytes, err := os.ReadFile(filepath.Join(dir, "agents", "GADU.md"))
	if err != nil {
		t.Fatalf("read agent file: %v", err)
	}
	skillBytes, err := os.ReadFile(filepath.Join(dir, "skills", "gadu-operator", "SKILL.md"))
	if err != nil {
		t.Fatalf("read skill file: %v", err)
	}

	// Both files must contain the canonical body verbatim (D7).
	if !strings.Contains(string(agentBytes), canonicalBody) {
		t.Error("agents/GADU.md does not contain the canonical persona body verbatim")
	}
	if !strings.Contains(string(skillBytes), canonicalBody) {
		t.Error("skills/gadu-operator/SKILL.md does not contain the canonical persona body verbatim")
	}
}

// TestGenerate_AgentFrontmatter verifies the agent file frontmatter has the
// required fields: name, description, model: opus, tools. (R-004)
func TestGenerate_AgentFrontmatter(t *testing.T) {
	dir := t.TempDir()
	if err := gadu.Generate(dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "agents", "GADU.md"))
	if err != nil {
		t.Fatalf("read agent: %v", err)
	}
	s := string(content)

	fm := extractFrontmatter(s)

	cases := []struct {
		field    string
		wantKey  string
		wantVal  string
		exact    bool
	}{
		{"name", "name", "GADU", true},
		{"model", "model", "opus", true},
		{"tools", "tools", "'*'", true},
		{"description (non-empty)", "description", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			val, ok := fm[tc.wantKey]
			if !ok {
				t.Errorf("frontmatter missing key %q; frontmatter: %v", tc.wantKey, fm)
				return
			}
			if tc.exact && val != tc.wantVal {
				t.Errorf("frontmatter %q = %q, want %q", tc.wantKey, val, tc.wantVal)
			}
			if !tc.exact && val == "" {
				t.Errorf("frontmatter %q must be non-empty", tc.wantKey)
			}
		})
	}
}

// extractFrontmatter parses a simple YAML-like frontmatter block (key: value
// lines between the opening and closing "---" markers) into a map.
// This minimal parser handles the flat frontmatter fields used by agents and skills.
func extractFrontmatter(content string) map[string]string {
	result := map[string]string{}
	if !strings.HasPrefix(strings.TrimSpace(content), "---") {
		return result
	}
	idx := strings.Index(content, "---")
	rest := content[idx+3:]
	end := strings.Index(rest, "---")
	if end < 0 {
		return result
	}
	block := rest[:end]
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return result
}

// TestGenerate_SkillFrontmatter verifies the skill file has a valid overlay
// skill frontmatter with name: gadu-operator and a Trigger: description. (R-005)
func TestGenerate_SkillFrontmatter(t *testing.T) {
	dir := t.TempDir()
	if err := gadu.Generate(dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "skills", "gadu-operator", "SKILL.md"))
	if err != nil {
		t.Fatalf("read skill: %v", err)
	}
	s := string(content)

	fm := extractFrontmatter(s)

	t.Run("name", func(t *testing.T) {
		if fm["name"] != "gadu-operator" {
			t.Errorf("skill frontmatter name = %q, want %q", fm["name"], "gadu-operator")
		}
	})
	t.Run("description contains Trigger:", func(t *testing.T) {
		desc := fm["description"]
		if !strings.Contains(desc, "Trigger:") {
			t.Errorf("skill frontmatter description must contain 'Trigger:'; got: %q", desc)
		}
	})
}

// TestGenerate_DoNotEditHeader verifies both emitted files contain the
// do-not-edit comment string. (R-012)
func TestGenerate_DoNotEditHeader(t *testing.T) {
	dir := t.TempDir()
	if err := gadu.Generate(dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	files := map[string]string{
		"agent": filepath.Join(dir, "agents", "GADU.md"),
		"skill": filepath.Join(dir, "skills", "gadu-operator", "SKILL.md"),
	}
	for label, path := range files {
		t.Run(label, func(t *testing.T) {
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", label, err)
			}
			if !strings.Contains(string(content), "GENERATED — DO NOT EDIT") {
				t.Errorf("%s must contain 'GENERATED — DO NOT EDIT'; got:\n%s", label, content)
			}
		})
	}
}

// TestCheck_FailsWhenMissing verifies Check returns an error when the skill
// file is missing from the repo root. (R-003)
func TestCheck_FailsWhenMissing(t *testing.T) {
	dir := t.TempDir()
	// Generate only the agent file so the skill is missing.
	if err := os.MkdirAll(filepath.Join(dir, "agents"), 0755); err != nil {
		t.Fatal(err)
	}
	// Don't call Generate — no artifacts exist.
	err := gadu.Check(dir)
	if err == nil {
		t.Error("Check must return non-nil error when artifacts are missing")
	}
}

// TestCheck_FailsWhenStale verifies Check returns an error when an artifact
// has been modified after generation. (R-003)
func TestCheck_FailsWhenStale(t *testing.T) {
	dir := t.TempDir()
	if err := gadu.Generate(dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Corrupt the agent file.
	agentPath := filepath.Join(dir, "agents", "GADU.md")
	content, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatalf("read agent: %v", err)
	}
	modified := string(content) + "\n<!-- hand-edited line -->\n"
	if err := os.WriteFile(agentPath, []byte(modified), 0644); err != nil {
		t.Fatalf("write agent: %v", err)
	}

	if err := gadu.Check(dir); err == nil {
		t.Error("Check must return non-nil error when artifact is stale")
	}
}

// TestCheck_PassesWhenInSync verifies Check returns nil against the real
// committed generated files. Skipped until A5 (generated files must exist on disk).
func TestCheck_PassesWhenInSync(t *testing.T) {
	root := repoRoot(t)

	// Skip if the generated files are not yet on disk (pre-A5).
	agentPath := filepath.Join(root, "agents", "GADU.md")
	skillPath := filepath.Join(root, "skills", "gadu-operator", "SKILL.md")
	if _, err := os.Stat(agentPath); os.IsNotExist(err) {
		t.Skip("agents/GADU.md not yet generated (pre-A5) — skipping staleness check")
	}
	if _, err := os.Stat(skillPath); os.IsNotExist(err) {
		t.Skip("skills/gadu-operator/SKILL.md not yet generated (pre-A5) — skipping staleness check")
	}

	if err := gadu.Check(root); err != nil {
		t.Errorf("Check must return nil when committed artifacts match generator output; got: %v", err)
	}
}
