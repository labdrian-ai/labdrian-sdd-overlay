package skills

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// generatedBlockBeginMarker and generatedBlockEndMarker delimit the
// machine-generated lint rule table inside skills/skill-creator/references/
// skill-style-guide.md (design.md's "LintSkill rule table is the single
// source" decision). There is no file-writing generator: a human runs
// `labdrian skills lint --rules` and pastes the output between these exact
// markers.
const (
	generatedBlockBeginMarker = "<!-- BEGIN GENERATED: skill-lint-rules (source: engine/skills/lint.go; print with `labdrian skills lint --rules`) -->"
	generatedBlockEndMarker   = "<!-- END GENERATED: skill-lint-rules -->"
)

const skillCreatorStyleGuidePath = "skills/skill-creator/references/skill-style-guide.md"
const skillImproverStyleGuidePath = "skills/skill-improver/references/skill-style-guide.md"

// extractGeneratedBlock returns the text strictly between the BEGIN/END
// generated-block markers in content, trimmed of the single leading and
// trailing newline the markers themselves introduce. It fails the test if
// either marker is absent or out of order.
func extractGeneratedBlock(t *testing.T, content string) string {
	t.Helper()
	begin := strings.Index(content, generatedBlockBeginMarker)
	if begin < 0 {
		t.Fatalf("missing generated-block BEGIN marker %q", generatedBlockBeginMarker)
	}
	afterBegin := begin + len(generatedBlockBeginMarker)
	end := strings.Index(content[afterBegin:], generatedBlockEndMarker)
	if end < 0 {
		t.Fatalf("missing generated-block END marker %q", generatedBlockEndMarker)
	}
	block := content[afterBegin : afterBegin+end]
	return strings.Trim(block, "\n")
}

// TestSkillStyleGuideLintRulesInSync proves the generated block in
// skill-creator's style guide is byte-identical to RenderLintRules(). On
// failure it prints the expected block so a human can paste it in directly
// (design.md: "On failure it prints the expected block").
func TestSkillStyleGuideLintRulesInSync(t *testing.T) {
	repoRoot := ooQualityContractRepoRoot(t)
	content := readRepoFile(t, repoRoot, skillCreatorStyleGuidePath)

	got := extractGeneratedBlock(t, content)
	want := strings.TrimRight(RenderLintRules(), "\n")

	if got != want {
		t.Fatalf(
			"generated skill-lint-rules block in %s is out of sync with RenderLintRules().\nExpected block (paste this between the markers):\n%s",
			skillCreatorStyleGuidePath, want,
		)
	}
}

// TestSkillStyleGuideCopiesAreByteIdentical proves skill-improver's style
// guide is a byte-identical copy of skill-creator's, per design.md's
// "single-source guards" (deleting the improver copy would churn a managed
// manifest row for no gain over a byte-identity test).
func TestSkillStyleGuideCopiesAreByteIdentical(t *testing.T) {
	repoRoot := ooQualityContractRepoRoot(t)
	creator := readRepoFile(t, repoRoot, skillCreatorStyleGuidePath)
	improver := readRepoFile(t, repoRoot, skillImproverStyleGuidePath)

	if creator != improver {
		t.Fatalf("%s is not byte-identical to %s", skillImproverStyleGuidePath, skillCreatorStyleGuidePath)
	}
}

// TestSkillFilesDoNotReferenceRemovedStyleGuideDoc proves none of the three
// skills that used to reference a repo-root docs/skill-style-guide.md still
// mention it (design.md: "A test asserts that none of skills/skill-creator/
// SKILL.md, skills/skill-improver/SKILL.md or skills/skill-registry/SKILL.md
// mentions docs/skill-style-guide.md").
func TestSkillFilesDoNotReferenceRemovedStyleGuideDoc(t *testing.T) {
	repoRoot := ooQualityContractRepoRoot(t)

	for _, path := range []string{
		"skills/skill-creator/SKILL.md",
		"skills/skill-improver/SKILL.md",
		"skills/skill-registry/SKILL.md",
	} {
		content := readRepoFile(t, repoRoot, path)
		if strings.Contains(content, "docs/skill-style-guide.md") {
			t.Errorf("%s must not reference docs/skill-style-guide.md", path)
		}
	}
}

// styleGuideRestatedNumberRe matches any of the four numeric lint bounds
// that must appear nowhere in skill-creator's style guide prose outside the
// generated block (design.md: "The prose sections 'Frontmatter Rules' and
// 'Body Budget' ... lose their numeric restatements (250, 160, 700, 1000).
// The generated block is the only place those numbers appear.").
var styleGuideRestatedNumberRe = regexp.MustCompile(`\b(250|160|700|1000)\b`)

// TestSkillStyleGuideProseHasNoSecondNumericSource proves the "Frontmatter
// Rules" and "Body Budget" prose sections contain no restatement of the
// generated block's numeric bounds. The 180-450 human-judgement target
// token range is deliberately excluded: it is not one of the four
// machine-checked bounds and stays as prose per design.md.
func TestSkillStyleGuideProseHasNoSecondNumericSource(t *testing.T) {
	repoRoot := ooQualityContractRepoRoot(t)
	content := readRepoFile(t, repoRoot, skillCreatorStyleGuidePath)

	// Strip the generated block itself before scanning prose: the rule
	// table is the one legitimate place these numbers appear.
	beginIdx := strings.Index(content, generatedBlockBeginMarker)
	endIdx := strings.Index(content, generatedBlockEndMarker)
	if beginIdx < 0 || endIdx < 0 || endIdx < beginIdx {
		t.Fatalf("missing or misordered generated-block markers in %s", skillCreatorStyleGuidePath)
	}
	withoutGenerated := content[:beginIdx] + content[endIdx+len(generatedBlockEndMarker):]

	for _, section := range []string{"Frontmatter Rules", "Body Budget"} {
		body := extractMarkdownSection(t, withoutGenerated, section)
		if m := styleGuideRestatedNumberRe.FindString(body); m != "" {
			t.Errorf("section %q restates numeric lint bound %q outside the generated block: %q", section, m, body)
		}
	}
}

// extractMarkdownSection returns the text of the H2 section named heading,
// up to (but excluding) the next H2 heading or end of file.
func extractMarkdownSection(t *testing.T, content, heading string) string {
	t.Helper()
	marker := fmt.Sprintf("## %s", heading)
	start := strings.Index(content, marker)
	if start < 0 {
		t.Fatalf("missing section %q", heading)
	}
	rest := content[start+len(marker):]
	if next := strings.Index(rest, "\n## "); next >= 0 {
		rest = rest[:next]
	}
	return rest
}
