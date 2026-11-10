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
