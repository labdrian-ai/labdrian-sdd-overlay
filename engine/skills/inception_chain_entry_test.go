package skills

import (
	"regexp"
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

func inceptionClosureContent(t *testing.T, root, skill string) string {
	t.Helper()
	return markdownSectionBody(t, skill, inceptionClosureFeedbackHeading) + "\n" + readRepoFile(t, root, inceptionClosureReferenceRelPath)
}

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
		section := inceptionClosureContent(t, repoRoot, skill)
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
		section := inceptionClosureContent(t, repoRoot, skill)
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
			if strings.Contains(skill+inceptionClosureContent(t, repoRoot, skill), forbidden) {
				t.Errorf("%s must not contain %q: closure-feedback no longer reads the gentle-ai receipt or the retired entry", inceptionPipelineSkillRelPath, forbidden)
			}
		}
	})

	t.Run("S5_planned_slice_count_comes_from_the_chain_not_the_retired_entry", func(t *testing.T) {
		section := inceptionClosureContent(t, repoRoot, skill)
		if want := "planned slice count not recorded"; !strings.Contains(section, want) {
			t.Errorf("the closure-feedback section must say %q when the chain's shaper/estimator record states no planned count", want)
		}
		if forbidden := "review_slices"; strings.Contains(section, forbidden) {
			t.Errorf("the closure-feedback section must not name %q: that field belonged to the retired entry contract", forbidden)
		}
	})

	t.Run("S5_archive_anchor_gate_is_history_only", func(t *testing.T) {
		mentions := 0
		for _, line := range strings.Split(inceptionClosureContent(t, repoRoot, skill), "\n") {
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

// TestInceptionPipelineClosureInputsAreLocatedByTheChain pins the current
// feature-document target, its pre-write guard, and the bound evidence locations.
func TestInceptionPipelineClosureInputsAreLocatedByTheChain(t *testing.T) {
	repoRoot := actualsInstrumentationRepoRoot(t)
	skill := readRepoFile(t, repoRoot, inceptionPipelineSkillRelPath)

	t.Run("C13_chain_entry_names_the_feature_document", func(t *testing.T) {
		section := markdownSectionBody(t, skill, "## Chain entry")
		for _, want := range []string{
			"The parent names `odd/tasks/<feature>.md`",
			"delivery adds its final Log entry in place",
		} {
			if !strings.Contains(section, want) {
				t.Errorf("the Chain entry section must say %q: the parent must bind the original document and delivery must close it in place", want)
			}
		}
	})

	t.Run("B1_closure_names_only_the_topics_inception_itself_writes", func(t *testing.T) {
		section := inceptionClosureContent(t, repoRoot, skill)
		// inception-pipeline writes sdd/{change}/pipeline-state (Tier Selection) and, as the
		// single writer, sdd/{change}/actuals (Execute item 1). No chain contract places any
		// other sdd/{change}/<name> topic, so naming one sends closure-feedback to a target
		// nothing created.
		writes := map[string]bool{"pipeline-state": true, "actuals": true}
		seen := map[string]bool{}
		for _, m := range regexp.MustCompile(`sdd/\{change\}/([a-z-]+)`).FindAllStringSubmatch(section, -1) {
			if writes[m[1]] || seen[m[1]] {
				continue
			}
			seen[m[1]] = true
			t.Errorf("the closure-feedback section names the topic %q, which inception-pipeline does not write and no chain contract places", m[0])
		}
	})

	t.Run("B1_does_not_say_the_chain_or_its_delivery_hop_places_an_output_it_never_places", func(t *testing.T) {
		for _, forbidden := range []string{
			"the chain's own outputs",
			"the delivery hop writes `archive-report.md`",
			"the delivery hop's `archive-report.md`",
		} {
			if strings.Contains(skill+inceptionClosureContent(t, repoRoot, skill), forbidden) {
				t.Errorf("%s must not contain %q: agents/delivery.md writes \"the archive report\" at the path its prompt names, and no chain contract names that file or those topics", inceptionPipelineSkillRelPath, forbidden)
			}
		}
	})

	t.Run("C13_document_is_confirmed_before_the_first_write", func(t *testing.T) {
		section := inceptionClosureContent(t, repoRoot, skill)
		for _, want := range []string{
			"inside the project repository BEFORE the first write",
			"missing or unreadable",
			"write nothing",
		} {
			if !strings.Contains(section, want) {
				t.Errorf("the closure-feedback section must say %q: Execute item 3 cannot be skipped, so a missing or unreadable document must stop all closure writes", want)
			}
		}
	})

	t.Run("C13_current_closure_inputs_and_outcome", func(t *testing.T) {
		section := inceptionClosureContent(t, repoRoot, skill)
		for _, want := range []string{
			"feature document's Specs and builder/delivery Log",
			"delivery outcome and an explicitly recorded `landing_commit`",
			"upstream requirements brief", "EARS R-NNN", "otherwise omit `requirement_count`",
			"NEW closure-feedback Log entry", "fenced Markdown block", "2-row table",
			"memorycard", "read back", "informational", "not proof of memorycard sync",
		} {
			if !strings.Contains(section, want) {
				t.Errorf("closure-feedback must contain %q", want)
			}
		}
		for _, forbidden := range []string{"engram export", "Builder's apply-progress", "shaper's spec"} {
			if strings.Contains(section, forbidden) {
				t.Errorf("current closure must not require %q", forbidden)
			}
		}
	})

	t.Run("B1_verifier_reports_are_read_where_the_chain_stores_them", func(t *testing.T) {
		const stored = "`<head12>-<n>.verifier.json`"
		section := inceptionClosureContent(t, repoRoot, skill)
		if !strings.Contains(section, stored) {
			t.Errorf("the closure-feedback section must read the verifier's report where the chain stores it, %s beside the review result", stored)
		}
		readme := readRepoFile(t, repoRoot, "odd/review-results/README.md")
		if !strings.Contains(readme, stored) {
			t.Errorf("odd/review-results/README.md must still say that the verifier's report is stored as %s: closure-feedback reads it there", stored)
		}
	})

	t.Run("B1_shared_contracts_say_the_role_chain_does_not_write_the_engine_keys", func(t *testing.T) {
		contracts := readRepoFile(t, repoRoot, "skills/_shared/pre-sdd-contracts.md")
		var note string
		for _, line := range strings.Split(contracts, "\n") {
			if strings.HasPrefix(line, "> **Note:** Engine keys") {
				note = line
				break
			}
		}
		if note == "" {
			t.Fatal("skills/_shared/pre-sdd-contracts.md must still carry the note that lists the SDD engine's keys")
		}
		for _, want := range []string{
			"role chain",
			"Chain entry",
		} {
			if !strings.Contains(note, want) {
				t.Errorf("the engine-keys note must say %q: it assigns the archive-report, verify-report, apply-progress and spec keys to the SDD engine and forbids reuse, and without that line a reader cannot tell where the role chain's agents write", want)
			}
		}
	})
}
