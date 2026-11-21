package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The skill-registry skill is labdrian's own (T30, S17): it tells an agent to generate the registry
// with the engine's refresh, not with the gentle-ai binary, and it keeps no copy of the registry in a
// memory store. These tests read the repository files the way the README test does.

func readRepoFile(t *testing.T, rel ...string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(append([]string{"..", ".."}, rel...)...))
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(rel...), err)
	}
	return string(data)
}

func TestTheSkillRegistrySkillGeneratesTheRegistryWithLabdrian(t *testing.T) {
	text := readRepoFile(t, "skills", "skill-registry", "SKILL.md")
	if !strings.Contains(text, "labdrian skill-registry refresh --cwd <project>") {
		t.Errorf("the skill does not say how to generate the registry: labdrian skill-registry refresh --cwd <project>")
	}
	for _, banned := range []string{"gentle-ai", "Gentle", "Engram", "engram", "gentleman-programming"} {
		if strings.Contains(text, banned) {
			t.Errorf("the skill still names %q", banned)
		}
	}
	if !regexp.MustCompile(`(?m)^  author: labdrian$`).MatchString(text) {
		t.Errorf("the skill is not authored by labdrian")
	}
	if !strings.HasPrefix(text, "---\nname: skill-registry\n") {
		t.Errorf("the skill lost its name in the frontmatter")
	}
	for _, kept := range []string{"## Activation Contract", "## Hard Rules", "## Decision Gates", "## Execution Steps", "## Output Contract", "`_shared`", "`sdd-*`", ".atl/skill-registry.md"} {
		if !strings.Contains(text, kept) {
			t.Errorf("the skill lost %q", kept)
		}
	}
}

func TestTheSkillRegistrySkillIsCustomInTheManifestAndTheRegistry(t *testing.T) {
	manifest := readRepoFile(t, "overlay.manifest")
	if !regexp.MustCompile(`(?m)^skill-registry/SKILL\.md custom$`).MatchString(manifest) {
		t.Errorf("overlay.manifest does not route skill-registry/SKILL.md as custom")
	}
	registry := readRepoFile(t, "skills.registry.yaml")
	_, after, ok := strings.Cut(registry, "  - id: skill-registry\n")
	if !ok {
		t.Fatalf("skills.registry.yaml has no skill-registry entry")
	}
	entry, _, _ := strings.Cut(after, "\n  - id: ")
	for _, want := range []string{"type: custom", "updateStrategy: overlay-only"} {
		if !strings.Contains(entry, want) {
			t.Errorf("the skill-registry entry lacks %q:\n%s", want, entry)
		}
	}
	for _, banned := range []string{"upstream", "gentleman-programming", "type: core"} {
		if strings.Contains(entry, banned) {
			t.Errorf("the skill-registry entry still says %q:\n%s", banned, entry)
		}
	}
}
