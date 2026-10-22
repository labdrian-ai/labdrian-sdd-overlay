package skills

// Checks of the role-chain skill and of the review results convention of Phase 9 (per-role design v3,
// section 6, PR 2), read from the repository the way role_agents_artifact_test.go reads the role
// agents: the skill that carries the duties of the parent of the role chain (section 3 of the
// design), the manifest row that deploys it, and odd/review-results/README.md, which says where the
// parent stores what the reviewer returns and what binds it to a record of the chain. The registry
// entry of the skill, and its order of the chain against the roles package, are checked in
// role_chain_registry_test.go.

import (
	"reflect"
	"strings"
	"testing"
)

// roleChainSkill reads skills/role-chain/SKILL.md: the whole file, its frontmatter and its body.
func roleChainSkill(t *testing.T) (whole, frontmatter, body string) {
	t.Helper()
	whole = readRepoFile(t, skillsRepoRoot(t), "skills/role-chain/SKILL.md")
	var err error
	if frontmatter, body, err = SplitSkillFile([]byte(whole)); err != nil {
		t.Fatalf("skills/role-chain/SKILL.md: %v", err)
	}
	return whole, frontmatter, body
}

// roleChainSections splits the body at its H2 headings: the headings in order, and the text under
// each. What comes before the first heading is under the empty name.
func roleChainSections(body string) (headings []string, under map[string]string) {
	under = map[string]string{}
	current := ""
	for _, line := range strings.Split(body, "\n") {
		if heading, ok := strings.CutPrefix(line, "## "); ok {
			current = strings.TrimSpace(heading)
			headings = append(headings, current)
			continue
		}
		under[current] += line + "\n"
	}
	return headings, under
}

// The skill is named for its directory (the Pi package build refuses a name that differs) and the
// model cannot invoke it by itself: the orchestrator runs it (v3 section 3).
func TestRoleChainSkillIsNamedForItsDirectoryAndNotInvokableByTheModel(t *testing.T) {
	_, frontmatter, _ := roleChainSkill(t)
	assertStringValue(t, parseYAMLScalar(t, frontmatter, "name"), "role-chain")
	assertStringValue(t, parseYAMLScalar(t, frontmatter, "disable-model-invocation"), "true")
	for _, key := range []string{"description", "license", "author", "version"} {
		if parseYAMLScalar(t, frontmatter, key) == "" {
			t.Errorf("the frontmatter key %q is empty", key)
		}
	}
}

// The skill is within the lint budget (v3 section 3, lint.go: the hard and the recommended bounds of
// the body): no hard error and no warning. If the lint ever refuses the size, the design splits the
// sweeper trigger into skills/sweeper/SKILL.md and nothing else.
func TestRoleChainSkillIsWithinTheLintBudget(t *testing.T) {
	whole, _, body := roleChainSkill(t)
	hard, warnings := LintSkillFile([]byte(whole))
	for _, err := range hard {
		t.Errorf("hard lint error: %v", err)
	}
	for _, w := range warnings {
		t.Errorf("lint warning [%s]: %s", w.Rule, w.Msg)
	}
	if tokens := EstimateBodyTokens(body); tokens > BodyRecommendedTokens {
		t.Errorf("the body is an estimated %d tokens, want at most %d", tokens, BodyRecommendedTokens)
	}
}

// The skill opens with the order of the chain, as v3 section 3 gives it: the prototyper, the sweeper
// and the polisher are optional.
func TestRoleChainSkillStatesTheOrderOfTheChain(t *testing.T) {
	_, _, body := roleChainSkill(t)
	const order = "Chain: `[prototyper] → shaper → estimator → builder → [sweeper] → [polisher] → reviewer → delivery`."
	if _, under := roleChainSections(body); !strings.Contains(under[""], order) {
		t.Errorf("the text before the first heading must state the order of the chain:\n%s\ngot:\n%s", order, under[""])
	}
}

// The skill has the five groups of parent duties of v3 section 3, in the order it gives them, and no
// other section.
func TestRoleChainSkillHasTheFiveParentDutiesInOrder(t *testing.T) {
	_, _, body := roleChainSkill(t)
	headings, _ := roleChainSections(body)
	if want := []string{"Appends", "Triggers", "Worktrees", "Binding", "Persistence"}; !reflect.DeepEqual(headings, want) {
		t.Errorf("the sections of the skill = %v, want %v", headings, want)
	}
}

// roleChainDuty is what v3 section 3 and the designs it cites (the prototyper P section 3, the sweeper
// S section 3, the estimation skill E section 5, the reviewer R sections 3 to 6) give one group of
// duties: the commands, the names and the rules that the group must carry, as the text of the skill
// has them.
type roleChainDuty struct {
	heading   string
	fragments []string
}

func roleChainDuties() []roleChainDuty {
	return []roleChainDuty{
		{"Appends", []string{
			"`labdrian roles append --project P --goal G --chain C --stdin`", "`payload_kind`",
			"`exploration`", "`proposal`…`apply-progress`", "`review-report`", "`verify-report`", "`archive-report`",
			"`shaper → estimator`", "`estimator → builder`", "same payload digest",
			"`partial` or `blocked`", "`interrupted`", "`resume_reason`",
		}},
		{"Triggers", []string{
			"does installed tool X behave as premise P says", "what is number N", "can X be done within the appetite",
			"back to the asker", "lookup/spike box",
			"`git diff --diff-filter=DR --name-only base..HEAD`", "`rg -w`", "`deadcode -test`", "`staticcheck`", "`shellcheck`",
			"`mktemp -d`", "`git status --porcelain --untracked-files=all`",
			"readability row", "docs, public API, messages or help", "second blind launch", "another `model`", "Risk line",
		}},
		{"Worktrees", []string{
			"`git worktree add --detach ~/labdrian-sdd-overlay-worktrees/spike-<goal>`", "`…/review-<head12> <head>`",
			"Never under the repository or the scratchpad", "remove after the verdict",
			"the delivery writes only the paths it names", "Engram `project`",
		}},
		{"Binding", []string{
			"`base`", "`head` (full SHAs)", "`diff_sha256` = sha256 of `git diff base head`",
			"150 KB", "one launch per commit", "never truncated", "S# criteria", "reliability and resilience",
			"a risk line (secrets, permissions, path escapes, exec/net imports)",
			"`candidate`", "`inspection.status`", "`completed`", "`paths`", "`git diff --name-only base head`",
			"`resume_reason: review not run: <reason>`", "retry once with another `model`",
			"`odd/review-results/<feature>/<head12>-<n>.json`", "`review-diff`", "`review-result`", "`verifier-report`",
			"recomputes `diff_sha256`", "only on equality",
			"introduced, activated or worsened", "reproduced", "(`unknown`: on `base` first)", "`verifier`", "within 3 calls",
			"`reviewer → builder` (the only backward edge)", "WARNING: fix before delivery", "SUGGESTION: carry", "`reviewer → delivery`",
		}},
		{"Persistence", []string{"prototyper's and sweeper's artifacts", "keep saving"}},
	}
}

// Each group of duties carries what the design gives it, in its own section: a rule that moved to
// another section would be read by a parent looking in the wrong place.
func TestRoleChainDutiesCarryWhatTheDesignGivesEachOne(t *testing.T) {
	_, _, body := roleChainSkill(t)
	_, under := roleChainSections(body)
	for _, duty := range roleChainDuties() {
		t.Run(duty.heading, func(t *testing.T) {
			for _, fragment := range duty.fragments {
				if !strings.Contains(under[duty.heading], fragment) {
					t.Errorf("the section %s must carry %q", duty.heading, fragment)
				}
			}
		})
	}
}

// Spikes and reviews run in worktrees that git detaches under the owner's worktrees directory, never
// under the repository or the scratchpad (a worktree there made the stop hook fail on the untracked
// files of the scratchpad), and no instruction of the skill makes the platform create one for it: the
// worktree tool and the isolation option put theirs under .claude/worktrees, inside the repository.
func TestRoleChainSkillConfinesSpikesAndReviewsOutsideTheRepository(t *testing.T) {
	whole, _, _ := roleChainSkill(t)
	for _, line := range strings.Split(whole, "\n") {
		if !strings.Contains(line, "git worktree add") {
			continue
		}
		for _, need := range []string{"--detach", "~/labdrian-sdd-overlay-worktrees/"} {
			if !strings.Contains(line, need) {
				t.Errorf("a worktree is added without %q:\n%s", need, line)
			}
		}
	}
	for _, banned := range []string{"/tmp/", ".claude/worktrees", "EnterWorktree", "isolation: worktree"} {
		if strings.Contains(whole, banned) {
			t.Errorf("the skill names %q; its worktrees are git's, outside the repository and the scratchpad", banned)
		}
	}
}

// The skill is deployed by a manifest row of the overlay (custom, not from upstream) that takes the
// skill route, to the skills directory of every runtime that has the skill (v3 section 3: "deployed to
// all three runtimes by the existing skill routes").
func TestRoleChainSkillIsDeployedByTheSkillRoutesToClaudeCodexAndPi(t *testing.T) {
	root := skillsRepoRoot(t)
	rows := ReadManifestRows([]byte(readRepoFile(t, root, "overlay.manifest")))
	var found []ManifestRow
	for _, row := range rows {
		if row.Path == "role-chain/SKILL.md" {
			found = append(found, row)
		}
	}
	if want := []ManifestRow{{Path: "role-chain/SKILL.md", Tag: "custom"}}; !reflect.DeepEqual(found, want) {
		t.Fatalf("rows named role-chain/SKILL.md = %+v, want exactly %+v", found, want)
	}
	for _, target := range []string{"claude", "codex", "pi"} {
		t.Run(target, func(t *testing.T) {
			routes, err := ResolveRoutes(rows, false, "/o", RouteDestinations{Target: target, SkillsRoot: "/h/skills", AgentsRoot: "/h/agents"})
			if err != nil {
				t.Fatalf("ResolveRoutes over the manifest of the repository: %v", err)
			}
			want := ResolvedRoute{Path: "role-chain/SKILL.md", Route: RouteSkill, RepoRel: "skills/role-chain/SKILL.md", RepoSource: "/o/skills/role-chain/SKILL.md", Dest: "/h/skills/role-chain/SKILL.md"}
			for _, r := range routes {
				if r.Path == want.Path {
					if r != want {
						t.Errorf("role-chain/SKILL.md resolves to %+v, want %+v", r, want)
					}
					return
				}
			}
			t.Errorf("no route for role-chain/SKILL.md")
		})
	}
}

// The convention of the review results (v3 decision D4; reviewer design R sections 3, 4 and 6) says
// where the parent stores what the reviewer returns, what the stored file is, and what binds it to a
// record of the chain; it says too what a result is not, so that a labdrian review is never taken for
// the review authority of gentle-ai, whose receipts are in a directory of their own.
func TestReviewResultsConventionSaysWhereAResultIsStoredAndWhatBindsItToARecord(t *testing.T) {
	readme := readRepoFile(t, skillsRepoRoot(t), "odd/review-results/README.md")
	for _, fragment := range []string{
		"`odd/review-results/<feature>/<head12>-<n>.json`", "`odd/tasks/<feature>.md`", "first 12 hex characters",
		"`candidate`", "`inspection`", "`findings`", "`evidence`", "`agents/reviewer.md`", "`<head12>-<n>.verifier.md`",
		"`review-report`", "`payload_sha256`", "`review-diff`", "`review-result`", "`verifier-report`", "`<base>..<head>`",
		"sha256 of `git diff base head`", "trusts the record only on equality",
		"check evidence", "never review authority", "`odd/review-receipts/`", "`interrupted`",
	} {
		if !strings.Contains(readme, fragment) {
			t.Errorf("odd/review-results/README.md must carry %q", fragment)
		}
	}
}
