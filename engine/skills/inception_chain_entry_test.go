package skills

import (
	"strings"
	"testing"
)

// TestInceptionPipelineEntersTheRoleChain pins the handoff of
// skills/inception-pipeline/SKILL.md after ADR C13-S0
// (docs/architecture/adr-c13-inception-role-chain.md, Decision 1): the skill enters the
// labdrian role chain through `labdrian roles append`, and it no longer gates on the gentle-ai
// SDD engine, whose `sdd-status` verb gentle-ai 4.2.0 does not have, so every tier failed at
// its handoff. Specs S1-S3 of odd/tasks/inception-role-chain.md.
func TestInceptionPipelineEntersTheRoleChain(t *testing.T) {
	repoRoot := actualsInstrumentationRepoRoot(t)
	skill := readRepoFile(t, repoRoot, inceptionPipelineSkillRelPath)
	frontmatter, _, err := SplitSkillFile([]byte(skill))
	if err != nil {
		t.Fatalf("splitting %s: %v", inceptionPipelineSkillRelPath, err)
	}
	fm := parseFrontmatterFields(frontmatter)

	t.Run("S1_names_nothing_of_the_gentle_ai_sdd_handoff", func(t *testing.T) {
		for _, forbidden := range []string{
			"gentle-ai sdd-status",
			"nextRecommended",
			"sdd-apply",
			"sdd-verify",
			"team-lite",
		} {
			if strings.Contains(skill, forbidden) {
				t.Errorf("%s must not contain %q: the gentle-ai SDD handoff it belongs to is gone", inceptionPipelineSkillRelPath, forbidden)
			}
		}
	})

	t.Run("S1_chain_entry_section_names_the_adapter_and_the_chain_skill", func(t *testing.T) {
		section := markdownSectionBody(t, skill, "## Chain entry")
		for _, want := range []string{
			"labdrian roles append",
			"skills/role-chain/SKILL.md",
		} {
			if !strings.Contains(section, want) {
				t.Errorf("the Chain entry section must name %q, got:\n%s", want, section)
			}
		}
	})

	t.Run("S1_description_names_the_role_chain", func(t *testing.T) {
		if !strings.Contains(fm.Description, "role chain") {
			t.Errorf("the frontmatter description must name the role chain, got %q", fm.Description)
		}
	})

	t.Run("S1_version_is_3_0_0", func(t *testing.T) {
		if fm.MetaVersion != "3.0.0" {
			t.Errorf("metadata.version = %q, want %q", fm.MetaVersion, "3.0.0")
		}
	})

	t.Run("S2_writes_role_chain_records_only", func(t *testing.T) {
		section := markdownSectionBody(t, skill, "## Chain entry")
		if !strings.Contains(section, "role-chain records only") {
			t.Errorf("the Chain entry section must say that inception writes role-chain records only, got:\n%s", section)
		}
		if strings.Contains(skill, "workflow create") {
			t.Errorf("%s must not name `workflow create`: opening a labdrian workflow is a separate goal", inceptionPipelineSkillRelPath)
		}
	})

	t.Run("S3_gives_no_instruction_to_write_the_entry_contract", func(t *testing.T) {
		for _, forbidden := range []string{
			"validate-entry-contract",
			"--exists-root",
			"entry-contract.schema.json",
		} {
			if strings.Contains(skill, forbidden) {
				t.Errorf("%s must not contain %q: sdd/{change}/entry is no longer written", inceptionPipelineSkillRelPath, forbidden)
			}
		}
	})
}
