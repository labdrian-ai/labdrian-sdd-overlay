package skills

import (
	"strings"
	"testing"
)

// The C14 pins for requirements-from-transcripts (odd/tasks/phase9-c14-requirements-from-transcripts.md).
// They check where each obligation lives, not its wording, and read the consumer side of every contract
// other skills already depend on, so neither side can move alone.

const rftDir = "skills/requirements-from-transcripts/"

// rftRepoRoot names the repository root these pins read from; it reuses the shared root lookup the other
// skill-contract tests in this package use.
func rftRepoRoot(t *testing.T) string {
	t.Helper()
	return actualsInstrumentationRepoRoot(t)
}

// TestRequirementsFromTranscriptsEntryFitsTheLintBudget pins S6: the entry stays inside the lint limits,
// pinned below the hard budget so one edit cannot reach it, and keeps its activation triggers.
func TestRequirementsFromTranscriptsEntryFitsTheLintBudget(t *testing.T) {
	entry := readRepoFile(t, rftRepoRoot(t), rftDir+"SKILL.md")
	front, body, err := SplitSkillFile([]byte(entry))
	if err != nil {
		t.Fatal(err)
	}
	desc := parseFrontmatterFields(front).Description
	if n := len([]rune(desc)); n > 250 {
		t.Errorf("description has %d runes, want at most 250", n)
	}
	if n := EstimateBodyTokens(body); n > 950 {
		t.Errorf("body has %d estimated tokens, want at most 950", n)
	}
	if hard, _ := LintSkillFile([]byte(entry)); len(hard) != 0 {
		t.Errorf("hard lint findings: %v", hard)
	}
	requireClauses(t, "description", desc,
		"meeting transcripts", "customer stories", "requerimientos desde meets", "historias de cliente")
	requireClauses(t, "entry", body,
		"## Activation Contract", "## Hard Rules", "## Decision Gates", "## Execution Steps", "## Output Contract")
}

// TestRequirementsFromTranscriptsKeepsConsumedContracts pins S2: the contracts inception-pipeline,
// project-inception, closure-feedback, tier-selection and clarify-request already consume.
func TestRequirementsFromTranscriptsKeepsConsumedContracts(t *testing.T) {
	root := rftRepoRoot(t)
	_, body, err := SplitSkillFile([]byte(readRepoFile(t, root, rftDir+"SKILL.md")))
	if err != nil {
		t.Fatal(err)
	}
	// (a) change-name, derived once and inherited verbatim.
	requireClauses(t, "entry", body,
		"`change-name` exactly once as a kebab-case slug", "Project-Inception Handoff", "inherits it verbatim")
	requireClauses(t, "inception-pipeline", readRepoFile(t, root, "skills/inception-pipeline/SKILL.md"),
		"`change-name` comes once from requirements-from-transcripts")
	// (b) the topic key project-inception loads.
	requireClauses(t, "entry", body, "project/{project}/requirements/{change}")
	requireClauses(t, "project-inception", readRepoFile(t, root, "skills/project-inception/SKILL.md"),
		"project/{project}/requirements/{change}")
	// (c) EARS R-NNN items with their scope enum, which closure counts.
	requireClauses(t, "entry", body, "`R-001`", "new-capability | feature | fix", "exactly one SHALL",
		"never renumbered or reused", "carried verbatim downstream")
	// The persisted record holds what its consumers read, and the readback checks it.
	requireClauses(t, "entry", body,
		"every R-NNN item with its `scope`, and the Project-Inception Handoff table", "confirm it matches the brief as drafted")
	// The template the brief is filled from carries the slug in its header and in every handoff row.
	template := readRepoFile(t, root, rftDir+"references/output-template.md")
	requireClauses(t, "output template", template, "**change-name:**",
		"| Roadmap Order | Requirement ID | Manifest Input | Architecture Input | Change Name |")
	// One slug, one column: a second change-name column would let a brief carry two names.
	rejectClauses(t, "output template", template, "Suggested Change Name")
	requireClauses(t, "patterns catalog", readRepoFile(t, root, rftDir+"references/patterns-catalog.md"),
		"| Change name | The brief's `change-name`, the same in every row; the role chain inherits it verbatim. |")
	requireClauses(t, "entry", body, "3. Derive `change-name`, then fill the brief")
	// The shared contract and clarify-request own the other side of (a), (c) and (e).
	shared := readRepoFile(t, root, "skills/_shared/pre-sdd-contracts.md")
	requireClauses(t, "pre-sdd-contracts", shared,
		"`change-name` is derived **exactly once**, by the requirements-capture step (`requirements-from-transcripts`)",
		"EARS-formatted requirement items (R-NNN), each carrying a `scope` enum of `new-capability | feature | fix`")
	requireClauses(t, "clarify-request", readRepoFile(t, root, "skills/clarify-request/SKILL.md"),
		"hand off to `requirements-from-transcripts` with the brief as input: it owns the change-name and the EARS requirements")
	requireClauses(t, "closure-feedback", readRepoFile(t, root, "skills/inception-pipeline/references/closure-feedback.md"),
		"EARS R-NNN requirements")
	// (d) the Tier 3 lightweight object and its switch to the full pipeline.
	requireClauses(t, "entry", body,
		"R-NNN:", "scope: fix", "one_line_impact:", "acceptance_evidence:", "change_name:", "switch to the full pipeline")
	requireClauses(t, "tier-selection", readRepoFile(t, root, "skills/inception-pipeline/references/tier-selection.md"),
		"requirements-from-transcripts (lightweight)")
	// (e) the inputs clarify-request and the prespec transcript hand over.
	requireClauses(t, "entry", body, "clarify-request brief", "prespec transcript")
	// (f) English artifacts, verbatim quotes and the shared contract.
	requireClauses(t, "entry", body, "English", "verbatim", "../_shared/pre-sdd-contracts.md")
}

// TestRequirementsFromTranscriptsKeepsItsGatingRules pins S3, S4 and S5 on the entry: the rules that decide
// whether a brief may exist, memorycard storage with readback, and the role-chain route.
func TestRequirementsFromTranscriptsKeepsItsGatingRules(t *testing.T) {
	_, body, err := SplitSkillFile([]byte(readRepoFile(t, rftRepoRoot(t), rftDir+"SKILL.md")))
	if err != nil {
		t.Fatal(err)
	}
	requireClauses(t, "entry", body,
		"Do not invent requirements", "mark it as an assumption",
		"permissions, data, workflow, money, compliance or user-visible behavior",
		"one issue at a time", "stop and wait", "named assumptions",
		"banned soft verbs", "One fix or feature per requirement", "acceptance evidence", "keywords",
		"memorycard", "read back", "../inception-pipeline/SKILL.md", "role chain", "catalog read in step 1")
	rejectClauses(t, "entry", body,
		"capture_prompt", "Engram-first", "SDD verify agents", "sdd/<change-name>/{proposal", "before PASS/archive")
}

// TestRequirementsFromTranscriptsReferencesHoldTheDraftingDetail pins S3 and S5 on the two references: the
// drafting detail the entry delegates lives there, and neither routes into retired SDD or Engram artifacts.
func TestRequirementsFromTranscriptsReferencesHoldTheDraftingDetail(t *testing.T) {
	root := rftRepoRoot(t)
	catalog := readRepoFile(t, root, rftDir+"references/patterns-catalog.md")
	template := readRepoFile(t, root, rftDir+"references/output-template.md")
	requireClauses(t, "patterns catalog", catalog,
		"## EARS Sentence Patterns", "Ubiquitous", "Banned soft verbs", "## Contradiction and Ambiguity Classification")
	requireClauses(t, "output template", template,
		"## Contradiction/Ambiguity Decision Prompt", "## Full Requirements Brief Template",
		"## Manifest Inputs", "## Architecture Inputs", "## Roadmap Inputs", "## Project-Inception Handoff",
		"## Traceability Matrix", "## Quality Bar", "PASS is forbidden when any critical requirement has no evidence")
	_, body, err := SplitSkillFile([]byte(readRepoFile(t, root, rftDir+"SKILL.md")))
	if err != nil {
		t.Fatal(err)
	}
	// The owner retired SDD (L3): no SDD wording, heading or route survives anywhere in the skill.
	for label, doc := range map[string]string{"patterns catalog": catalog, "output template": template, "entry": body} {
		rejectClauses(t, label, doc,
			"capture_prompt", "Engram", "SDD", "sdd/", "before PASS/archive", "mem_save", "openspec/")
	}
	if !strings.Contains(body, "references/patterns-catalog.md") || !strings.Contains(body, "references/output-template.md") {
		t.Error("entry must tell the reader to load both references at the step that needs them")
	}
}
