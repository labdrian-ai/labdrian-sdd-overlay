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

// generatedBlockMarkerLocation returns the byte offsets of the BEGIN and END
// generated-block markers in content: beginIdx is the start of the BEGIN
// marker text, and endIdx is the start of the END marker text, found after
// the BEGIN marker so the two markers cannot be matched out of order. It
// fails the test if either marker is absent or END precedes/overlaps BEGIN.
// This is the single marker-location definition shared by every contract
// test that needs to find or exclude the generated block, so they cannot
// disagree with each other about how the block is found. Two callers share
// it, and they use it for OPPOSITE purposes on the same offsets:
// extractGeneratedBlock keeps the span BETWEEN the markers (the generated
// rule table, compared byte-for-byte against RenderLintRules()), while
// TestSkillStyleGuideProseHasNoSecondNumericSource discards that same span
// and scans only the prose AROUND it, because the rule table is the one
// legitimate place those numeric bounds may appear. A divergent second definition would let
// the drift scan exclude a different span than the sync check compares, so
// a number restated just outside the block would be invisible to both
// (review-b75e4a27b9494ff8 R2-extract-block-comment-mismatch).
func generatedBlockMarkerLocation(t *testing.T, content string) (beginIdx, endIdx int) {
	t.Helper()
	beginIdx = strings.Index(content, generatedBlockBeginMarker)
	if beginIdx < 0 {
		t.Fatalf("missing generated-block BEGIN marker %q", generatedBlockBeginMarker)
	}
	afterBegin := beginIdx + len(generatedBlockBeginMarker)
	relEnd := strings.Index(content[afterBegin:], generatedBlockEndMarker)
	if relEnd < 0 {
		t.Fatalf("missing generated-block END marker %q after BEGIN marker", generatedBlockEndMarker)
	}
	endIdx = afterBegin + relEnd
	return beginIdx, endIdx
}

// extractGeneratedBlock returns the text strictly between the BEGIN/END
// generated-block markers in content, trimmed of exactly one leading and one
// trailing newline — the newlines the markers themselves introduce when each
// sits on its own line. Extra blank lines pasted inside the markers are
// preserved rather than silently trimmed away, so they surface as a
// byte-identity mismatch instead of passing the sync check.
func extractGeneratedBlock(t *testing.T, content string) string {
	t.Helper()
	beginIdx, endIdx := generatedBlockMarkerLocation(t, content)
	block := content[beginIdx+len(generatedBlockBeginMarker) : endIdx]
	block = strings.TrimPrefix(block, "\n")
	block = strings.TrimSuffix(block, "\n")
	return block
}

// TestExtractGeneratedBlock_ExtraInternalBlankLinesAreNotSilentlyTrimmed
// proves extractGeneratedBlock trims exactly one leading and one trailing
// newline, not every leading/trailing newline. An extra blank line pasted
// just inside either marker must therefore survive into the extracted block
// and make a byte-identity sync check fail instead of silently passing
// (review-b75e4a27b9494ff8 R3-extract-block-trims-all-newlines).
func TestExtractGeneratedBlock_ExtraInternalBlankLinesAreNotSilentlyTrimmed(t *testing.T) {
	clean := generatedBlockBeginMarker + "\nrule table content\n" + generatedBlockEndMarker
	withExtraBlankLines := generatedBlockBeginMarker + "\n\nrule table content\n\n" + generatedBlockEndMarker

	got := extractGeneratedBlock(t, withExtraBlankLines)
	want := extractGeneratedBlock(t, clean)

	if got == want {
		t.Fatalf("expected extra internal blank lines to change the extracted block, both were %q", got)
	}
	if got != "\nrule table content\n" {
		t.Errorf("expected exactly the outer newlines trimmed, got %q", got)
	}
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
// The generated block is the only place those numbers appear."). Built from
// the same lint.go constants RenderLintRules() renders, instead of a second
// literal source: if a bound changes, this guard changes with it rather than
// silently passing a stale restatement of the new value
// (review-b75e4a27b9494ff8 R2-duplicated-numeric-bounds).
var styleGuideRestatedNumberRe = regexp.MustCompile(
	fmt.Sprintf(`\b(%d|%d|%d|%d)\b`, DescriptionMaxRunes, DescriptionShouldRunes, BodyRecommendedTokens, BodyHardTokens),
)

// TestSkillStyleGuideProseHasNoSecondNumericSource proves the "Frontmatter
// Rules" and "Body Budget" prose sections contain no restatement of the
// generated block's numeric bounds. The 180-450 human-judgement target
// token range is deliberately excluded: it is not one of the four
// machine-checked bounds and stays as prose per design.md.
func TestSkillStyleGuideProseHasNoSecondNumericSource(t *testing.T) {
	repoRoot := ooQualityContractRepoRoot(t)
	content := readRepoFile(t, repoRoot, skillCreatorStyleGuidePath)

	// Strip the generated block itself before scanning prose: the rule
	// table is the one legitimate place these numbers appear. Reuses the
	// same marker-location helper as extractGeneratedBlock so the two
	// contract checks cannot disagree about how the block is found
	// (review-b75e4a27b9494ff8 R2-extract-block-comment-mismatch).
	beginIdx, endIdx := generatedBlockMarkerLocation(t, content)
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
