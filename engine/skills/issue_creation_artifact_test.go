package skills

// S17 (labdrian-orchestrator-opencode T33): skills/issue-creation/SKILL.md is a labdrian skill for the
// labdrian flow. It keeps the exact target, the YAML Issue Form as the only body format, one
// open-and-closed duplicate search before a write, body files in a private temporary directory, the
// privacy scan, one attempt with read-back and exactly one of confirmed | no_write | unknown, and a
// write only on the owner's current direct instruction. It drops the protected-label gate and the
// post-publication workflow actions, adds the foreman and inception-pipeline hop for an issue that
// turns into work, carries no gentle author and no Engram tool, and meets the whole skills lint: it
// is not on lintBaselineSkills. Its ownership (a custom row with no upstream) is checked beside the
// other adopted skills in gentle_detach_test.go.

import (
	"strings"
	"testing"
)

func issueCreationSkill(t *testing.T) (whole, frontmatter, body string) {
	t.Helper()
	whole = readRepoFile(t, skillsRepoRoot(t), "skills/issue-creation/SKILL.md")
	var err error
	if frontmatter, body, err = SplitSkillFile([]byte(whole)); err != nil {
		t.Fatalf("skills/issue-creation/SKILL.md: %v", err)
	}
	return whole, frontmatter, body
}

func TestIssueCreationSkillHasTheFourSectionsInOrder(t *testing.T) {
	_, _, body := issueCreationSkill(t)
	headings, _ := roleChainSections(body)
	want := []string{"Activation Contract", "Hard Rules", "Decision Gates", "Output Contract"}
	if !equalStrings(headings, want) {
		t.Errorf("the H2 headings = %q, want %q", headings, want)
	}
}

func TestIssueCreationSkillIsLabdriansAndLeavesNoGentleResidue(t *testing.T) {
	whole, frontmatter, _ := issueCreationSkill(t)
	assertStringValue(t, parseYAMLScalar(t, frontmatter, "name"), "issue-creation")
	assertStringValue(t, parseYAMLScalar(t, frontmatter, "author"), "labdrian")
	lower := strings.ToLower(whole)
	for _, banned := range []string{
		"status:approved", "viewerpermission", "gentle", "size:exception",
		"delegated-workflow-actions",
		"engram", "mem_save", "mem_search", "mem_get", "mem_context", "mem_update",
	} {
		if strings.Contains(lower, banned) {
			t.Errorf("skills/issue-creation/SKILL.md still names %q", banned)
		}
	}
}

func TestIssueCreationSkillKeepsTargetFormAndDuplicateSearch(t *testing.T) {
	_, _, body := issueCreationSkill(t)
	for _, want := range []string{
		"`[HOST/]OWNER/REPO`",
		"YAML Issue Form",
		"One duplicate search over open and closed issues runs before a write.",
		"`gh issue list --repo \"$TARGET\" --state all",
		"`gh issue create --repo \"$TARGET\"",
		"private temporary directory",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the body does not contain %q", want)
		}
	}
}

func TestIssueCreationSkillRunsThePrivacyScanBeforeTheWrite(t *testing.T) {
	_, _, body := issueCreationSkill(t)
	for _, want := range []string{"privacy scan", "immediately before the write"} {
		if !strings.Contains(body, want) {
			t.Errorf("the body does not contain %q", want)
		}
	}
}

func TestIssueCreationSkillReportsExactlyOneOfThreeOutcomes(t *testing.T) {
	_, _, body := issueCreationSkill(t)
	for _, want := range []string{
		"exactly one of `confirmed | no_write | unknown`",
		"one attempt",
		"read it back",
		"`unknown` stops every later write and retry",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the body does not contain %q", want)
		}
	}
}

func TestIssueCreationSkillDefinesTheThreeOutcomes(t *testing.T) {
	_, _, body := issueCreationSkill(t)
	for _, want := range []string{
		"- `confirmed`: the write returned a stable identity and the read-back matches.\n",
		"- `no_write`: an authoritative rejection, or a refusal before the write.\n",
		"- `unknown`: a timeout, a lost response, a missing identity, a read-back that is unavailable, or a mismatch.\n",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the body does not contain the outcome line %q", want)
		}
	}
}

func TestIssueCreationSkillReadsACommentBackByItsIdentity(t *testing.T) {
	_, _, body := issueCreationSkill(t)
	for _, want := range []string{
		"`gh api --hostname \"$HOST\" \"repos/$REPO/issues/comments/$COMMENT_ID\"`",
		"compare its body",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the comment read-back rule does not contain %q", want)
		}
	}
}

func TestIssueCreationSkillReadsAndComparesEachDuplicateCandidate(t *testing.T) {
	_, _, body := issueCreationSkill(t)
	for _, want := range []string{
		"Read each candidate from the target host with `gh issue view",
		"compare it with the selected form",
		"A candidate that cannot be read is `unknown`: stop.",
		"| A duplicate does not conform to the form | Report it and ask the owner; never edit it unasked. |",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the duplicate procedure does not contain %q", want)
		}
	}
}

func TestIssueCreationSkillTakesTheFormFromTheTargetRepository(t *testing.T) {
	_, _, body := issueCreationSkill(t)
	for _, want := range []string{
		"the target repository's own `.github/ISSUE_TEMPLATE/*.yml` that fits the request",
		"If none fits, stop and ask.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the form rule does not contain %q", want)
		}
	}
	if strings.Contains(body, "this repository:") || strings.Contains(body, "feature.yml)") {
		t.Error("the body hardcodes this repository's form")
	}
}

func TestIssueCreationSkillWritesOnlyOnTheOwnersCurrentInstruction(t *testing.T) {
	_, _, body := issueCreationSkill(t)
	line := "\n- A write runs only on the owner's current direct instruction that names the target and the action.\n"
	if !strings.Contains(body, line) {
		t.Error("no Hard Rule ties a write to the owner's current direct instruction")
	}
}

func TestIssueCreationSkillSendsWorkThroughTheForemanAndInceptionPipeline(t *testing.T) {
	_, _, body := issueCreationSkill(t)
	for _, want := range []string{"foreman", "`inception-pipeline`", "feature document", "never straight to a builder"} {
		if !strings.Contains(body, want) {
			t.Errorf("the body does not contain %q", want)
		}
	}
}

func TestIssueCreationSkillMeetsTheWholeLintAndIsOffTheBaseline(t *testing.T) {
	whole, _, _ := issueCreationSkill(t)
	hard, warnings := LintSkillFile([]byte(whole))
	for _, err := range hard {
		t.Errorf("hard lint finding: %v", err)
	}
	for _, w := range warnings {
		t.Errorf("lint advisory: [%s] %s", w.Rule, w.Msg)
	}
	if inLintBaseline("issue-creation") {
		t.Error("issue-creation is still on lintBaselineSkills")
	}
}
