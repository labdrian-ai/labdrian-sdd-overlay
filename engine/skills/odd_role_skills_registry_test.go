package skills_test

// The registry entries of the two overlay skills that the shaper, the builder and delivery follow
// (C15, S5): odd-feature-doc and test-first. They need the YAML registry the program reads, which package
// skills cannot import; the text of the skills and their manifest rows are checked in
// odd_role_skills_artifact_test.go.

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// Each skill is registered as a global skill of the overlay's own (custom, updated by the overlay
// alone), installed for the three runtimes the role agents run on: Claude Code, Codex and Pi, as
// role-chain is. OpenCode is not among them: it is not a runtime in use. The approval record of each is
// the owner's (`skills approve`), so no test here asks for one.
func TestOddRoleSkillsAreRegisteredAsGlobalOverlaySkillsForClaudeCodexAndPi(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "skills.registry.yaml"))
	if err != nil {
		t.Fatalf("reading the registry of the repository: %v", err)
	}
	reg, err := decode(data)
	if err != nil {
		t.Fatalf("parsing the registry of the repository: %v", err)
	}
	for _, id := range []string{"odd-feature-doc", "test-first"} {
		t.Run(id, func(t *testing.T) {
			var found []skills.Entry
			for _, entry := range reg.Skills {
				if entry.ID == id {
					found = append(found, entry)
				}
			}
			want := []skills.Entry{{
				ID:        id,
				Path:      id,
				Source:    skills.Source{Type: "custom"},
				Install:   skills.Install{DefaultScope: "global", Targets: []string{"claude", "codex", "pi"}},
				Lifecycle: skills.Lifecycle{UpdateStrategy: "overlay-only"},
			}}
			if !reflect.DeepEqual(found, want) {
				t.Errorf("registry entries named %s = %+v, want exactly %+v", id, found, want)
			}
		})
	}
}
