package skills

// S16, S17 (labdrian-orchestrator-opencode T32): skills/branch-pr/SKILL.md is a labdrian skill for the
// labdrian flow. It keeps the branch-name rule and the Conventional Commit rule of the text it
// replaces, and adds the owner-owned push, pull request and merge (S16 B10), the `review check` that
// precedes a pull request and the `gh pr checks` that precedes a merge. It leaves out what only
// gentle's own repository needed (the approved-issue gate, the type labels, its PR Validation jobs and
// the shellcheck plan), carries no gentle author and no Engram tool, and meets the whole skills lint:
// it is not on lintBaselineSkills. Its ownership (a custom row with no upstream) is checked beside
// the other adopted skills in gentle_detach_test.go.

import (
	"strings"
	"testing"
)

func branchPRSkill(t *testing.T) (whole, frontmatter, body string) {
	t.Helper()
	whole = readRepoFile(t, skillsRepoRoot(t), "skills/branch-pr/SKILL.md")
	var err error
	if frontmatter, body, err = SplitSkillFile([]byte(whole)); err != nil {
		t.Fatalf("skills/branch-pr/SKILL.md: %v", err)
	}
	return whole, frontmatter, body
}

// The text follows the section shape of the other labdrian skills.
func TestBranchPRSkillHasTheFourSectionsInOrder(t *testing.T) {
	_, _, body := branchPRSkill(t)
	headings, _ := roleChainSections(body)
	want := []string{"Activation Contract", "Hard Rules", "Decision Gates", "Output Contract"}
	if !equalStrings(headings, want) {
		t.Errorf("the H2 headings = %q, want %q (further sections may follow Decision Gates)", headings, want)
	}
}

func equalStrings(got, want []string) bool {
	// Hard Rules, Decision Gates and Output Contract keep their order; a section may sit between
	// them (branch names, commits), so the check is that want is a subsequence of got.
	i := 0
	for _, h := range got {
		if i < len(want) && h == want[i] {
			i++
		}
	}
	return i == len(want)
}

func TestBranchPRSkillIsLabdriansAndLeavesNoGentleResidue(t *testing.T) {
	whole, frontmatter, _ := branchPRSkill(t)
	assertStringValue(t, parseYAMLScalar(t, frontmatter, "name"), "branch-pr")
	assertStringValue(t, parseYAMLScalar(t, frontmatter, "author"), "labdrian")
	description := parseYAMLScalar(t, frontmatter, "description")
	if strings.Contains(strings.ToLower(description), "gentle") {
		t.Errorf("the description names Gentle AI: %q", description)
	}
	lower := strings.ToLower(whole)
	for _, banned := range []string{
		"status:approved", "gentle", "type:bug", "type:feature", "shellcheck", "size:exception",
		"engram", "mem_save", "mem_search", "mem_get", "mem_context", "mem_update",
	} {
		if strings.Contains(lower, banned) {
			t.Errorf("skills/branch-pr/SKILL.md still names %q", banned)
		}
	}
}

// S16 B10: the owner owns push, pull request and merge; the agent acts only on an owner message that
// asks for that very step.
func TestBranchPRSkillLeavesPushPullRequestAndMergeToTheOwner(t *testing.T) {
	_, _, body := branchPRSkill(t)
	for _, command := range []string{"`git push`", "`gh pr create`", "`gh pr merge`"} {
		if !strings.Contains(body, command) {
			t.Errorf("the body does not name %s", command)
		}
	}
	if !strings.Contains(body, "only on an owner message that asks for") {
		t.Error("the body does not say that git push, gh pr create and gh pr merge run only on an owner message that asks for each")
	}
}

// delivery's closing entry is what a pull request waits for: its `review check` exited 0.
func TestBranchPRSkillWaitsForTheReviewCheckBeforeAPullRequest(t *testing.T) {
	_, _, body := branchPRSkill(t)
	for _, want := range []string{
		"`labdrian review check --feature <f> --onto <base>`",
		"exited 0",
		"closing entry",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the body does not contain %q", want)
		}
	}
	// The body of the pull request reports the feature document, its task ids, the review check and the checks run.
	for _, want := range []string{"feature document", "task ids", "`review check` result", "exact results"} {
		if !strings.Contains(body, want) {
			t.Errorf("the pull request body rule does not contain %q", want)
		}
	}
}

func TestBranchPRSkillMergesOnlyWhenNoCheckFails(t *testing.T) {
	_, _, body := branchPRSkill(t)
	if !strings.Contains(body, "`gh pr checks`") || !strings.Contains(body, "no failing line") {
		t.Error("the body does not tie a merge to `gh pr checks` showing no failing line")
	}
}

// Kept from the text it replaces: the branch-name rule and the Conventional Commit rule, and the
// labdrian additions to a commit: the Risk trailer and no AI attribution.
func TestBranchPRSkillKeepsTheBranchNameAndCommitRules(t *testing.T) {
	_, _, body := branchPRSkill(t)
	for _, want := range []string{
		`^(feat|fix|chore|docs|style|refactor|perf|test|build|ci|revert)\/[a-z0-9._-]+$`,
		`^(build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test)(\([a-z0-9\._-]+\))?!?: .+`,
		"`Risk:`",
		"Co-Authored-By",
		"`chained-pr`",
		"`work-unit-commits`",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the body does not contain %q", want)
		}
	}
}

// branch-pr met only the baseline lint before; it now meets the whole lint, advisories included.
func TestBranchPRSkillMeetsTheWholeLintAndIsOffTheBaseline(t *testing.T) {
	whole, _, _ := branchPRSkill(t)
	hard, warnings := LintSkillFile([]byte(whole))
	for _, err := range hard {
		t.Errorf("hard lint finding: %v", err)
	}
	for _, w := range warnings {
		t.Errorf("lint advisory: [%s] %s", w.Rule, w.Msg)
	}
	if inLintBaseline("branch-pr") {
		t.Error("branch-pr is still on lintBaselineSkills")
	}
}
