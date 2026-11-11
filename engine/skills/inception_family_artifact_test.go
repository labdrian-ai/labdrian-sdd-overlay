package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInceptionPipelineClosureReferenceActivation(t *testing.T) {
	root := actualsInstrumentationRepoRoot(t)
	entry := readRepoFile(t, root, inceptionPipelineSkillRelPath)
	section := markdownSectionBody(t, entry, inceptionClosureFeedbackHeading)
	if !strings.Contains(section, "Before measuring or writing closure, read `references/closure-feedback.md`") {
		t.Fatal("closure must require its reference before measurement or writes")
	}
	manifest := readRepoFile(t, root, "overlay.manifest")
	if strings.Count(manifest, "inception-pipeline/references/closure-feedback.md custom\n") != 1 {
		t.Fatal("the manifest must include the reference exactly once")
	}
	ref := readRepoFile(t, root, inceptionClosureReferenceRelPath)
	if strings.Contains(section, "### Validate") || strings.Contains(section, "### Execute") {
		t.Fatal("validation and execution belong to the linked reference")
	}
	if !strings.Contains(ref, "### Validate") || !strings.Contains(ref, "### Execute") {
		t.Fatal("the linked reference must own validation and execution")
	}
}

// readCompactSkill reads a rewritten skill entry and enforces the lint budget every C13 entry
// shares (description at most 250 runes, body at most 1000 estimated tokens, no hard lint
// finding, the five contract headings). It returns the description and the body.
func readCompactSkill(t *testing.T, root, rel string) (description, body string) {
	t.Helper()
	raw := readRepoFile(t, root, rel)
	front, body, err := SplitSkillFile([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	description = parseFrontmatterFields(front).Description
	if n := len([]rune(description)); n > 250 {
		t.Errorf("%s: description has %d runes, want at most 250", rel, n)
	}
	if n := EstimateBodyTokens(body); n > 1000 {
		t.Errorf("%s: body has %d estimated tokens, want at most 1000", rel, n)
	}
	if hard, _ := LintSkillFile([]byte(raw)); len(hard) != 0 {
		t.Errorf("%s: hard lint findings: %v", rel, hard)
	}
	for _, heading := range []string{"## Activation Contract", "## Hard Rules", "## Decision Gates", "## Execution Steps", "## Output Contract"} {
		if !strings.Contains(body, heading) {
			t.Errorf("%s: missing %s", rel, heading)
		}
	}
	return description, body
}

// requireInOrder fails unless every clause is present in doc and the clauses appear in the order given.
func requireInOrder(t *testing.T, label, doc string, clauses ...string) {
	t.Helper()
	last := -1
	for _, clause := range clauses {
		at := strings.Index(doc, clause)
		if at < 0 {
			t.Errorf("%s: missing %q", label, clause)
			continue
		}
		if at < last {
			t.Errorf("%s: %q must come after the previous clause", label, clause)
		}
		last = at
	}
}

// requireClauses fails for every clause missing from doc.
func requireClauses(t *testing.T, label, doc string, clauses ...string) {
	t.Helper()
	for _, clause := range clauses {
		if !strings.Contains(doc, clause) {
			t.Errorf("%s: missing %q", label, clause)
		}
	}
}

// rejectClauses fails for every clause still present in doc.
func rejectClauses(t *testing.T, label, doc string, clauses ...string) {
	t.Helper()
	for _, clause := range clauses {
		if strings.Contains(doc, clause) {
			t.Errorf("%s: must not contain %q", label, clause)
		}
	}
}

func TestInceptionPipelineCompactEntry(t *testing.T) {
	readCompactSkill(t, actualsInstrumentationRepoRoot(t), inceptionPipelineSkillRelPath)
}

// TestInceptionPipelineKeepsActivationAndClosureRules pins what the compaction must not lose:
// the description's activation triggers (a host picks a skill by its description alone), the
// closure wait for the landing, who records landing_commit, and the headroom below the hard
// token budget so a small edit cannot block approval.
func TestInceptionPipelineKeepsActivationAndClosureRules(t *testing.T) {
	root := actualsInstrumentationRepoRoot(t)
	description, body := readCompactSkill(t, root, inceptionPipelineSkillRelPath)
	if n := EstimateBodyTokens(body); n > 950 {
		t.Errorf("body has %d estimated tokens, want at most 950 so one edit cannot reach the hard budget", n)
	}
	requireClauses(t, "description", strings.ToLower(description),
		"first skill", "change request", "hotfix", "arrancá el proyecto", "nueva feature", "arreglo chico", "project-inception directly")

	closure := markdownSectionBody(t, body, inceptionClosureFeedbackHeading)
	requireClauses(t, "closure section", closure,
		"an error because the chain has no records, means it runs: wait, do not fire",
		"is not closed; wait for landing",
		"The parent names the landing commit (merge, squash or fast-forward); closure records it as `landing_commit`")
	rejectClauses(t, "closure section", closure, "merge commit", "default branch")

	steps := markdownSectionBody(t, body, "## Execution Steps")
	requireClauses(t, "execution steps", steps, "`requirements_brief_ref: {change}`")

	ref := readRepoFile(t, root, inceptionClosureReferenceRelPath)
	requireInOrder(t, "closure reference", ref, "#### Storage", "Use the parent's resolved storage", "#### t0", "typed `created_at`")
	requireClauses(t, "closure reference", ref,
		"the parent names the actual landing commit once the change has landed (a merge, squash or fast-forward commit, whatever its parent count)",
		"`landing_commit`, always: the explicitly named commit that landed the successful delivery (the last slice",
		"A delivered chain that has not landed is not closed",
		"closure's entry follows delivery's final Log entry")
	// The landing rule names no branch, neither the default one nor a selected one, and the
	// tree-ambiguity rule counts every commit carrying the tree, not those of one branch.
	requireClauses(t, "closure reference", ref, "more than one commit carries it, the anchor is ambiguous")
	rejectClauses(t, "closure reference", ref,
		"the actual merge commit", "onto the default branch", "integration branch", "on the default branch carries it")
}

func TestProjectInceptionCompactCoordinator(t *testing.T) {
	entry := readRepoFile(t, actualsInstrumentationRepoRoot(t), "skills/project-inception/SKILL.md")
	front, body, err := SplitSkillFile([]byte(entry))
	if err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(parseFrontmatterFields(front).Description)); n > 250 {
		t.Errorf("description has %d runes, want at most 250", n)
	}
	if n := EstimateBodyTokens(body); n > 1000 {
		t.Errorf("body has %d estimated tokens, want at most 1000", n)
	}
	if hard, _ := LintSkillFile([]byte(entry)); len(hard) != 0 {
		t.Errorf("hard lint findings: %v", hard)
	}
	for _, clause := range []string{
		"## Activation Contract", "## Hard Rules", "## Decision Gates", "## Execution Steps", "## Output Contract",
		"requirements_brief_ref", "project/{project}/requirements/{change}", "memorycard", "readback",
		"`full` is the default", "`reuse`", "both manifest and architecture", "report fallback to `full`",
		"../project-manifest/SKILL.md", "../project-architect/SKILL.md", "../roadmap-maker/SKILL.md",
		"`build`", "`incremental-insert`", "../inception-pipeline/SKILL.md", "../role-chain/SKILL.md",
		"completed", "blocked", "partial",
	} {
		if !strings.Contains(body, clause) {
			t.Errorf("missing coordinator obligation %q", clause)
		}
	}
}

func TestProjectManifestCompactContract(t *testing.T) {
	root := actualsInstrumentationRepoRoot(t)
	entry := readRepoFile(t, root, "skills/project-manifest/SKILL.md")
	front, body, err := SplitSkillFile([]byte(entry))
	if err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(parseFrontmatterFields(front).Description)); n > 250 {
		t.Errorf("description has %d runes", n)
	}
	if n := EstimateBodyTokens(body); n > 1000 {
		t.Errorf("body has %d estimated tokens", n)
	}
	if hard, _ := LintSkillFile([]byte(entry)); len(hard) != 0 {
		t.Errorf("hard lint findings: %v", hard)
	}
	for _, clause := range []string{"## Activation Contract", "## Hard Rules", "## Decision Gates", "## Execution Steps", "## Output Contract", "conversation_history", "phase_override", "discovery", "draft", "final", "memorycard", "readback", "project/{project}/manifest/{context,mission,rules}", "openspec/project/manifest/", "one blocking question", "references/universal-base-rules.md", "references/phase-templates.md", "<!-- BEGIN: project-manifest-rules (auto-generated) -->", "<!-- END: project-manifest-rules -->"} {
		if !strings.Contains(body, clause) {
			t.Errorf("missing manifest obligation %q", clause)
		}
	}
	for _, path := range []string{"skills/project-manifest/references/universal-base-rules.md", "skills/project-manifest/references/phase-templates.md"} {
		readRepoFile(t, root, path)
	}
	shared := readRepoFile(t, root, "skills/_shared/pre-sdd-contracts.md")
	for _, marker := range []string{"<!-- BEGIN: project-manifest-rules (auto-generated) -->", "<!-- END: project-manifest-rules -->"} {
		if strings.Count(body, marker) != 1 || strings.Count(shared, marker) != 1 {
			t.Errorf("manifest marker mismatch: %s", marker)
		}
	}
	for _, marker := range []string{"<!-- BEGIN: project-architect-constraints (auto-generated) -->", "<!-- END: project-architect-constraints -->"} {
		if strings.Count(shared, marker) != 1 || strings.Contains(body, marker) {
			t.Errorf("architect marker ownership mismatch: %s", marker)
		}
	}
	phase := readRepoFile(t, root, "skills/project-manifest/references/phase-templates.md")
	for _, clause := range []string{"at most one blocking question", "render new artifacts and envelopes in English", "Do not translate existing stored records"} {
		if !strings.Contains(phase, clause) {
			t.Errorf("missing phase seam %q", clause)
		}
	}

}

func TestProjectArchitectCompactContract(t *testing.T) {
	root := actualsInstrumentationRepoRoot(t)
	entry := readRepoFile(t, root, "skills/project-architect/SKILL.md")
	front, body, err := SplitSkillFile([]byte(entry))
	if err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(parseFrontmatterFields(front).Description)); n > 250 {
		t.Errorf("description has %d runes", n)
	}
	if n := EstimateBodyTokens(body); n > 1000 {
		t.Errorf("body has %d estimated tokens", n)
	}
	if hard, _ := LintSkillFile([]byte(entry)); len(hard) != 0 {
		t.Errorf("hard lint findings: %v", hard)
	}
	for _, clause := range []string{"## Activation Contract", "## Hard Rules", "## Decision Gates", "## Execution Steps", "## Output Contract", "references/blocked-response.md", "references/architecture-template.md", "explicitly selects", "Section 15", "memorycard", "readback", "project/{project}/architect/final", "openspec/project/architect/final.md", "one blocking question", "<!-- BEGIN: project-architect-constraints (auto-generated) -->", "<!-- END: project-architect-constraints -->"} {
		if !strings.Contains(body, clause) {
			t.Errorf("missing architect obligation %q", clause)
		}
	}
	shared := readRepoFile(t, root, "skills/_shared/pre-sdd-contracts.md")
	for _, marker := range []string{"<!-- BEGIN: project-architect-constraints (auto-generated) -->", "<!-- END: project-architect-constraints -->"} {
		if strings.Count(body, marker) != 1 || strings.Count(shared, marker) != 1 {
			t.Errorf("architect marker mismatch: %s", marker)
		}
	}
	for _, marker := range []string{"<!-- BEGIN: project-manifest-rules (auto-generated) -->", "<!-- END: project-manifest-rules -->"} {
		if strings.Count(shared, marker) != 1 || strings.Contains(body, marker) {
			t.Errorf("manifest marker ownership mismatch: %s", marker)
		}
	}
	blocked := readRepoFile(t, root, "skills/project-architect/references/blocked-response.md")
	for _, clause := range []string{"blocked-no-manifest", "blocked-incomplete-manifest", "1", "2", "3", "inline"} {
		if !strings.Contains(blocked, clause) {
			t.Errorf("missing blocked response %q", clause)
		}
	}
	template := readRepoFile(t, root, "skills/project-architect/references/architecture-template.md")
	for _, clause := range []string{"## 15. Manifest Rules Compliance", "explicitly selected inline context", "do not claim compliance with an absent persisted manifest", "real trade-offs"} {
		if !strings.Contains(template, clause) {
			t.Errorf("missing template obligation %q", clause)
		}
	}
	if strings.Contains(template, "Minimum 3 trade-offs") || strings.Contains(template, "/sdd-new") {
		t.Error("template retains quota or retired route")
	}
}

func closureActualsReplacementPresent(doc string, clauses []string) bool {
	for _, clause := range clauses {
		i := strings.Index(doc, clause)
		if i < 0 {
			return false
		}
		doc = doc[i+len(clause):]
	}
	return true
}

func TestClosureActualsValidatesExactReplacement(t *testing.T) {
	ref := readRepoFile(t, actualsInstrumentationRepoRoot(t), inceptionClosureReferenceRelPath)
	clauses := []string{
		"Read and reconcile the existing actuals as evidence",
		"preserving its topic identity and metadata",
		"construct ONE FINAL closed-schema object",
		"remove or omit now-unsourced optional values",
		"old elapsed time when a landing or approved anchor is rejected",
		"persist and read back that exact object as a full replacement",
		"if unavailable, report it and STOP before any write",
		"Validate it with the **deterministic validator**",
		"persist the exact validated object as a full replacement",
		"Never merge, patch, or modify actuals after validation",
		"A returned payload differing from the validated candidate is a partial-write failure",
	}
	requireInOrder(t, "actuals replacement", ref, clauses...)
	for _, clause := range clauses {
		mutant := strings.Replace(ref, clause, "", 1)
		if closureActualsReplacementPresent(mutant, clauses) {
			t.Errorf("reverted control accepted missing clause %q", clause)
		}
	}
	if strings.Contains(ref, "Merge existing records and read back each write") {
		t.Error("blanket post-validation merge remains")
	}
}

func TestArchitectEmittedEntryPathsHaveRepositoryRoot(t *testing.T) {
	root := actualsInstrumentationRepoRoot(t)
	template := readRepoFile(t, root, "skills/project-architect/references/architecture-template.md")
	const handoff = "Hand off to the parent; see repository-relative skill identities `skills/project-inception/SKILL.md` and `skills/role-chain/SKILL.md`"
	if !strings.Contains(template, handoff) {
		t.Error("rendered handoff must name repository-relative skill identities")
	}
	for _, path := range []string{"skills/project-inception/SKILL.md", "skills/role-chain/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Errorf("repository identity %s: %v", path, err)
		}
	}
	reverted := strings.Replace(template, handoff, "Hand off to the parent; see `../project-inception/SKILL.md` and `../role-chain/SKILL.md`", 1)
	if strings.Contains(reverted, handoff) {
		t.Error("reverted handoff must fail the identity check")
	}
}

// Match consecutive complete instructions, not words embedded in a negation.
func roadmapActualsWriterRulePresent(hardRules string) bool {
	const writer = "Read `sdd/{change}/actuals`; only inception closure writes actuals"
	const prohibition = "Never write or infer them from plans or estimates"
	for _, line := range strings.Split(hardRules, "\n") {
		instruction := strings.TrimPrefix(strings.TrimSpace(line), "- ")
		sentences := strings.Split(strings.TrimSuffix(instruction, "."), ". ")
		for i := 0; i+1 < len(sentences); i++ {
			if sentences[i] == writer && sentences[i+1] == prohibition {
				return true
			}
		}
	}
	return false
}

func TestRoadmapActualsWriterRuleRejectsNegation(t *testing.T) {
	entry := readRepoFile(t, actualsInstrumentationRepoRoot(t), roadmapMakerSkillRelPath)
	rules := markdownSectionBody(t, entry, "## Hard Rules")
	const rule = "only inception closure writes actuals"
	if !roadmapActualsWriterRulePresent(rules) {
		t.Fatal("current roadmap writer rule must be accepted")
	}
	mutant := strings.Replace(rules, rule, "it is false that "+rule, 1)
	if mutant == rules || !strings.Contains(mutant, rule) {
		t.Fatal("mutation must retain the old substring")
	}
	if roadmapActualsWriterRulePresent(mutant) {
		t.Error("negated roadmap writer rule must be rejected")
	}
	if roadmapActualsWriterRulePresent(strings.Replace(rules, rule, "", 1)) {
		t.Error("deleted roadmap writer rule must be rejected")
	}
}

func TestRoadmapMakerCompactContract(t *testing.T) {
	root := actualsInstrumentationRepoRoot(t)
	entry := readRepoFile(t, root, roadmapMakerSkillRelPath)
	front, body, err := SplitSkillFile([]byte(entry))
	if err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(parseFrontmatterFields(front).Description)); n > 250 {
		t.Errorf("description has %d runes", n)
	}
	if n := EstimateBodyTokens(body); n > 1000 {
		t.Errorf("body has %d estimated tokens", n)
	}
	if hard, _ := LintSkillFile([]byte(entry)); len(hard) != 0 {
		t.Errorf("hard lint findings: %v", hard)
	}
	for _, clause := range []string{"## Activation Contract", "## Hard Rules", "## Decision Gates", "## Execution Steps", "## Output Contract", "assets/roadmap-template.md", "incremental-insert", "unrelated rows", "actuals and history", "../inception-pipeline/SKILL.md", "../role-chain/SKILL.md", "sdd/{change}/actuals", "memorycard", "readback"} {
		if !strings.Contains(body, clause) {
			t.Errorf("missing roadmap obligation %q", clause)
		}
	}
	if !roadmapActualsWriterRulePresent(markdownSectionBody(t, entry, "## Hard Rules")) {
		t.Error("roadmap Hard Rules must reserve actuals writes to inception closure")
	}
	template := readRepoFile(t, root, "skills/roadmap-maker/assets/roadmap-template.md")
	for _, clause := range []string{"odd/review-results/<feature>/", "shaper", "estimator", "historical fallback", "[PENDING]", "[NOT MEASURED — reason]"} {
		if !strings.Contains(template, clause) {
			t.Errorf("missing template obligation %q", clause)
		}
	}
	if strings.Contains(template, "/sdd-new") || strings.Contains(template, "verify-report.md") {
		t.Error("template retains retired current source")
	}
}

func TestC13ReviewCorrectionContracts(t *testing.T) {
	root := actualsInstrumentationRepoRoot(t)
	cases := []struct {
		path    string
		clauses []string
	}{
		{"skills/project-inception/SKILL.md", []string{"bootstrap completo", "manifest architect roadmap", "manifest is final enough before architecture", "architecture is final enough before roadmap", "never rewrite either foundation", "both foundation results and exact locators"}},
		{"skills/project-manifest/SKILL.md", []string{"manifiesto", "PRD inicial", "all eight", "merge continuation updates", "architect block in `.atl/skill-registry.md`"}},
		{"skills/project-manifest/references/phase-templates.md", []string{"Confirmed domain rules", "Source: project-manifest skill. Do NOT edit manually", "Triggers: when working on ANY change in this project", "Full rules"}},
		{"skills/project-architect/SKILL.md", []string{"diseñar arquitectura", "qué stack uso", "create `.atl/skill-registry.md`", "read manifest", "write architecture"}},
		{"skills/project-architect/references/architecture-template.md", []string{"## 15. Manifest Rules Compliance", "**Backup and recovery**", "None — blocked by missing inputs.", "owner must choose Option 1, 2, or 3"}},
		{"skills/project-architect/references/blocked-response.md", []string{"Option 2 — Provide context inline", "answers `1`, `2`, or `3`"}},
		{"skills/roadmap-maker/SKILL.md", []string{"ordenar SDDs", "planificá los sprints", "no cycles", "before each change", "after closure", "when reality diverges", "Do not write spec, design or apply detail", "only inception closure writes actuals", "Never write or infer them from plans or estimates", "measured versus reconstructed", "legacy sentinels", "[NOT MEASURED — reason]", "[PENDING DECISION]", "replace the entire roadmap"}},
		{"skills/roadmap-maker/assets/roadmap-template.md", []string{"repository-relative skill identities", "`skills/inception-pipeline/SKILL.md`", "before inception-pipeline 3.0.0"}},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) { requireClauses(t, c.path, readRepoFile(t, root, c.path), c.clauses...) })
	}
}
