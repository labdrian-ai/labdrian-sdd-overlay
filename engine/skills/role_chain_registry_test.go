package skills_test

// The tests of the role-chain skill (Phase 9, per-role design v3, section 6, PR 2) that need an
// adapter or a package that package skills does not import: the YAML registry the program reads, and
// the roles package whose vocabulary the skill orders. The text of the skill, its lint, its manifest
// row and the review results convention are checked in role_chain_artifact_test.go.

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// The skill is registered as a global skill of the overlay's own (custom, updated by the overlay
// alone), installed for the three runtimes the overlay serves: Claude Code, Codex and Pi (v3 section
// 3: "deployed to all three runtimes by the existing skill routes"). OpenCode is not among them: it is
// not a runtime in use.
func TestRoleChainIsRegisteredAsAGlobalOverlaySkillForClaudeCodexAndPi(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "skills.registry.yaml"))
	if err != nil {
		t.Fatalf("reading the registry of the repository: %v", err)
	}
	reg, err := decode(data)
	if err != nil {
		t.Fatalf("parsing the registry of the repository: %v", err)
	}
	var found []skills.Entry
	for _, entry := range reg.Skills {
		if entry.ID == "role-chain" {
			found = append(found, entry)
		}
	}
	want := []skills.Entry{{
		ID:        "role-chain",
		Path:      "role-chain",
		Source:    skills.Source{Type: "custom"},
		Install:   skills.Install{DefaultScope: "global", Targets: []string{"claude", "codex", "pi"}},
		Lifecycle: skills.Lifecycle{UpdateStrategy: "overlay-only"},
	}}
	if !reflect.DeepEqual(found, want) {
		t.Errorf("registry entries named role-chain = %+v, want exactly %+v", found, want)
	}
}

// The order the skill gives is the vocabulary of the roles package, in its order: a role renamed,
// added or moved there fails here, instead of leaving the parent a chain the engine refuses. The
// roles in brackets are the ones a chain may skip: the optional ones of the catalog, and the
// prototyper, which a chain may start without (VerifyChain accepts a first record from the shaper).
func TestRoleChainOrderIsTheVocabularyOfTheRolesPackage(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "skills", "role-chain", "SKILL.md"))
	if err != nil {
		t.Fatalf("reading the skill: %v", err)
	}
	text := string(data)
	start := strings.Index(text, "`[prototyper]")
	if start < 0 {
		t.Fatal("the skill states no order of the chain that starts with `[prototyper]")
	}
	order, _, found := strings.Cut(text[start+1:], "`")
	if !found {
		t.Fatal("the order of the chain in the skill is not closed by a backtick")
	}

	var wantNames, wantSkippable, gotNames, gotSkippable []string
	for _, info := range roles.Catalog {
		wantNames = append(wantNames, string(info.Role))
		if info.Optional || info.Role == roles.RolePrototyper {
			wantSkippable = append(wantSkippable, string(info.Role))
		}
	}
	for _, step := range strings.Split(order, " → ") {
		name, bracketed := strings.CutSuffix(strings.TrimPrefix(step, "["), "]")
		gotNames = append(gotNames, name)
		if bracketed {
			gotSkippable = append(gotSkippable, name)
		}
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Errorf("the order of the skill names the roles %v, the roles package has %v", gotNames, wantNames)
	}
	if !reflect.DeepEqual(gotSkippable, wantSkippable) {
		t.Errorf("the skill puts %v in brackets, the roles a chain may skip are %v", gotSkippable, wantSkippable)
	}
}
