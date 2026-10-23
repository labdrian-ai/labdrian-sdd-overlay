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

// inceptionClosureFeedbackHeading is the heading of the closure-feedback section of
// skills/inception-pipeline/SKILL.md.
const inceptionClosureFeedbackHeading = "## Closure-Feedback (plan → validate → execute; single writer)"

// TestInceptionPipelineClosureFeedbackReadsTheChain pins the closure-feedback inputs of
// skills/inception-pipeline/SKILL.md after ADR C13-S0 (Decision 3, owner decision 4a):
// closure-feedback is re-anchored on the labdrian role chain, not deleted. The chain is
// finished when `labdrian roles resume` reports it terminal, the review inputs are the
// results stored under odd/review-results/, `approved_tree` is the tree of the reviewed head,
// and neither the gentle-ai receipt inputs nor the retired `sdd/{change}/entry` are named.
// Spec S5 of odd/tasks/inception-role-chain.md. The phrases the actuals instrumentation gates
// pin (actuals_instrumentation_*_test.go) stay in the skill wherever their rule still holds.
func TestInceptionPipelineClosureFeedbackReadsTheChain(t *testing.T) {
	repoRoot := actualsInstrumentationRepoRoot(t)
	skill := readRepoFile(t, repoRoot, inceptionPipelineSkillRelPath)

	t.Run("S5_fires_when_labdrian_roles_resume_reports_the_chain_terminal", func(t *testing.T) {
		section := markdownSectionBody(t, skill, inceptionClosureFeedbackHeading)
		for _, want := range []string{
			"labdrian roles resume",
			"\"terminal\": true",
		} {
			if !strings.Contains(section, want) {
				t.Errorf("the closure-feedback section must name %q: the chain is finished when `labdrian roles resume` reports it terminal", want)
			}
		}
	})

	t.Run("S5_review_inputs_are_the_stored_results_and_the_tree_of_the_reviewed_head", func(t *testing.T) {
		section := markdownSectionBody(t, skill, inceptionClosureFeedbackHeading)
		for _, want := range []string{
			"odd/review-results/",
			"`candidate.head`",
			"`git show -s --format=%T <reviewed head>`",
		} {
			if !strings.Contains(section, want) {
				t.Errorf("the closure-feedback section must name %q: approved_tree is the tree of the head a stored review result names", want)
			}
		}
	})

	t.Run("S5_names_nothing_of_the_gentle_ai_receipt_or_the_retired_entry", func(t *testing.T) {
		for _, forbidden := range []string{
			"gentle-ai.review-receipt",
			"review-transactions/v2",
			"SDD engine archives",
			"sdd/{change}/entry",
		} {
			if strings.Contains(skill, forbidden) {
				t.Errorf("%s must not contain %q: closure-feedback no longer reads the gentle-ai receipt or the retired entry", inceptionPipelineSkillRelPath, forbidden)
			}
		}
	})

	t.Run("S5_planned_slice_count_comes_from_the_chain_not_the_retired_entry", func(t *testing.T) {
		section := markdownSectionBody(t, skill, inceptionClosureFeedbackHeading)
		if want := "planned slice count not recorded"; !strings.Contains(section, want) {
			t.Errorf("the closure-feedback section must say %q when the chain's shaper/estimator record states no planned count", want)
		}
		if forbidden := "review_slices"; strings.Contains(section, forbidden) {
			t.Errorf("the closure-feedback section must not name %q: that field belonged to the retired entry contract", forbidden)
		}
	})

	t.Run("S5_archive_anchor_gate_is_history_only", func(t *testing.T) {
		mentions := 0
		for _, line := range strings.Split(skill, "\n") {
			if !strings.Contains(line, "archive-anchor-gate") {
				continue
			}
			mentions++
			if !strings.Contains(line, "history-only") {
				t.Errorf("a line that names archive-anchor-gate must say it is history-only (owner decision 5a), got:\n%s", line)
			}
		}
		if mentions == 0 {
			t.Errorf("%s must still say, in one line, that archive-anchor-gate is history-only", inceptionPipelineSkillRelPath)
		}
	})
}
