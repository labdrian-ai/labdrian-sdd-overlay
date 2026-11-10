package skills

import (
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

func TestInceptionPipelineCompactEntry(t *testing.T) {
	entry := readRepoFile(t, actualsInstrumentationRepoRoot(t), inceptionPipelineSkillRelPath)
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
	for _, heading := range []string{"## Activation Contract", "## Hard Rules", "## Decision Gates", "## Execution Steps", "## Output Contract"} {
		if !strings.Contains(body, heading) {
			t.Errorf("missing %s", heading)
		}
	}
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
	for _, clause := range []string{"## 15. Cumplimiento de Reglas del Manifest", "explicitly selected inline context", "do not claim compliance with an absent persisted manifest", "real trade-offs"} {
		if !strings.Contains(template, clause) {
			t.Errorf("missing template obligation %q", clause)
		}
	}
	if strings.Contains(template, "Minimum 3 trade-offs") || strings.Contains(template, "/sdd-new") {
		t.Error("template retains quota or retired route")
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
