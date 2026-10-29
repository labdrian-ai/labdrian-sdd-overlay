package skills

// Checks of the role-chain skill and of the review results convention of Phase 9 (per-role design v3,
// section 6, PR 2), read from the repository the way role_agents_artifact_test.go reads the role
// agents: the skill that carries the duties of the parent of the role chain (section 3 of the
// design), the manifest row that deploys it, and odd/review-results/README.md, which says where the
// parent stores what the reviewer returns and what binds it to a record of the chain. The registry
// entry of the skill, and its order of the chain against the roles package, are checked in
// role_chain_registry_test.go.

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewverdict"
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

// The sweeper trigger of the role-chain skill (own-rdd T6b7a, S6 g) names the three commands the
// parent runs around the sweeper: the sweep that finds the candidates, and the two records it stores.
func TestRoleChainSweeperTriggerNamesTheSweepAndTheRecords(t *testing.T) {
	_, _, body := roleChainSkill(t)
	const want = "- Sweeper, on the builder's commit (`base` = parent): `labdrian review sweep --feature F --base B --head H` runs T1-T3 (`labdrian --help`); if it fires, launch the sweeper on its candidates, store its report with `labdrian review record --role sweeper`; if the `Risk:` trailer names an item, store each polisher judge's findings with `labdrian review record --role polisher`."
	if _, under := roleChainSections(body); !strings.Contains(under["Triggers"], want+"\n") {
		t.Errorf("the Triggers section must carry the sweeper line, exactly:\n%s", want)
	}
	for _, fragment := range []string{
		"labdrian review sweep --feature F --base B --head H", "labdrian review record --role sweeper", "labdrian review record --role polisher",
	} {
		if !strings.Contains(body, fragment) {
			t.Errorf("the role-chain skill must name %q", fragment)
		}
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
			"`exploration`, `feature-doc`, `progress`, `review-report`, `verify-report`, `close-report`",
			"`shaper → estimator`", "`estimator → builder`", "same payload digest",
			"`partial` or `blocked`", "`interrupted`", "`resume_reason`",
		}},
		{"Triggers", []string{
			"does installed tool X behave as premise P says", "what is number N", "can X be done within the appetite",
			"back to the asker", "lookup/spike box",
			"runs T1-T3 (`labdrian --help`)",
			"readability row", "docs, public API, messages or help", "second blind launch", "another `model`", "Risk line",
		}},
		{"Worktrees", []string{
			"`git worktree add --detach ~/labdrian-sdd-overlay-worktrees/spike-<goal>`", "reviews: `review bind` makes it",
			"Never under the repository or the scratchpad", "remove after the verdict",
			"the delivery writes only the paths it names", "Engram `project`",
		}},
		{"Binding", []string{
			"`labdrian review bind --base B --head H`", "`base`", "`head` (full SHAs)", "`diff_sha256`", "`worktree`",
			"150 KB", "one launch per commit", "never truncated", "S# criteria",
			"copied verbatim from `roles/reviewer/rows.json`", "`default` always",
			"`labdrian review record --feature F --base B --head H`", "Exit 2",
			"`candidate`", "`inspection.status`", "`completed`", "`paths`", "any other rule of the reviewer's contract",
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
		"`candidate`", "`inspection`", "`findings`", "`evidence`", "`agents/reviewer.md`", "`<head12>-<n>.verifier.json`",
		"`review-report`", "`payload_sha256`", "`review-diff`", "`review-result`", "`verifier-report`", "`<base>..<head>`",
		"`labdrian review bind`", "`labdrian review record`", "`labdrian review verify`", "the sha256 of the canonical diff", "trusts the record only on equality",
		"`reviewverdict.Parse`", "`reviewverdict.Decide`", "the correction round of the candidate was spent",
		"`binding`", "`reply`", "`binding.head`", "git 2.40 or later", "`diff-tree`", "`--attr-source`",
		"Exit 0 stored, 2 reply refused",
		"`labdrian roles append` holds a completed `review-report` record to the files", "the message names the evidence kind and its ref",
		"`labdrian review check`", "at least one result binds it and every one is clear", "A check that did not run never passes",
		"check evidence", "never review authority", "`odd/review-receipts/`", "`interrupted`",
	} {
		if !strings.Contains(readme, fragment) {
			t.Errorf("odd/review-results/README.md must carry %q", fragment)
		}
	}
}

// envelopeOverride starts the line of a delta that replaces the return envelope of the core prefix.
const envelopeOverride = "This replaces the return envelope of the core prefix: "

// verdictObject is the object the line of the delta of roles/verifier/body.md that replaces the return envelope
// spells out, decoded, and the one verdict it shows. The line is found by how it starts, not by its place.
func verdictObject(t *testing.T) (line string, spelled, verdict map[string]any) {
	t.Helper()
	delta := roleDelta(t, skillsRepoRoot(t), "verifier")
	at := slices.IndexFunc(delta, func(l string) bool { return strings.HasPrefix(l, envelopeOverride) })
	if at < 0 {
		t.Fatalf("no line of the verifier delta starts %q; got:\n%s", envelopeOverride, strings.Join(delta, "\n"))
	}
	line = delta[at]
	start := strings.Index(line, `{"verdicts"`)
	if start < 0 {
		t.Fatalf("the third line of the verifier delta spells out no JSON object that starts with \"verdicts\"; got:\n%s", line)
	}
	if err := json.NewDecoder(strings.NewReader(line[start:])).Decode(&spelled); err != nil {
		t.Fatalf("the object the verifier delta spells out is not JSON: %v", err)
	}
	if verdicts, _ := spelled["verdicts"].([]any); len(verdicts) == 1 {
		verdict, _ = verdicts[0].(map[string]any)
	}
	if verdict == nil {
		t.Fatalf("the verifier delta must spell out one verdict, as an object; got %v", spelled["verdicts"])
	}
	return line, spelled, verdict
}

// The verifier's whole reply is one JSON object, and its agent file is the only place it can read the shape of
// that object: the prompt of the parent lists the finding ids and the checks, not the format. The third line
// of its delta says it replaces the return envelope of the core prefix (the envelope asks for a status and a
// summary, which the parent would fail to read as a verdict), and spells the object out with the members
// engine/reviewverdict requires, the values it lists and the rules it refuses a reply for.
func TestVerifierDeltaSpellsOutTheVerdictObjectItReturns(t *testing.T) {
	line, spelled, verdict := verdictObject(t)
	if !strings.HasPrefix(line, "This replaces the return envelope of the core prefix: ") {
		t.Errorf("the third line of the verifier delta must start with the replacement of the return envelope; got:\n%s", line)
	}
	if len(spelled) != 1 {
		t.Errorf("the object has %d members, want only verdicts: a verdict is evidence, never a schema, a receipt or an authority", len(spelled))
	}
	var members []string
	for member := range verdict {
		members = append(members, member)
	}
	sort.Strings(members)
	if want := []string{"base_result", "command", "id", "output", "result"}; !reflect.DeepEqual(members, want) {
		t.Errorf("a verdict has the members %v, want %v", members, want)
	}
	for _, member := range []string{"result", "base_result"} {
		if verdict[member] != "reproduced|not-reproduced|not-run" {
			t.Errorf("%s = %v, want the three values of the contract", member, verdict[member])
		}
	}
	// The rules the engine refuses a reply for (engine/reviewverdict), told to the agent that must obey them: a
	// refused reply is a verification that did not run, and the parent pays for the launch again.
	for _, rule := range []string{
		"one verdict for each finding id the prompt lists, none twice and none the prompt does not list",
		"result is the check on head, base_result the same check on base",
		"output is never empty and command is never empty when a check ran",
		"no member is omitted or null",
		"not-run is never a pass",
	} {
		if !strings.Contains(line, rule) {
			t.Errorf("the third line of the verifier delta must say %q", rule)
		}
	}
}

// The object agents/verifier.md spells out is the object the engine parses, and a drift between the two is a
// verification that never runs: the agent would reply with a member the parser refuses as unknown, or without
// one it requires. The test fills the object the delta spells out with a legal value for each member and
// requires reviewverdict.Parse to accept it for every pair of the values the delta lists for result and
// base_result. A member the delta gains that this test has no legal value for fails it, and so does one the
// parser does not know.
func TestVerifierDeltaObjectIsTheOneTheEngineParses(t *testing.T) {
	_, _, verdict := verdictObject(t)
	answered := reviewresult.Result{Findings: []reviewresult.Finding{{ID: "F1"}}}
	legal := map[string]any{"id": "F1", "command": "go test ./engine/", "output": "--- FAIL: TestGuard"}
	alternativesOf := func(member string) []string {
		list, ok := verdict[member].(string)
		if !ok {
			t.Fatalf("the verifier delta must list the alternatives of %q as one string; got %v", member, verdict[member])
		}
		return strings.Split(list, "|")
	}
	want := []string{string(reviewverdict.Reproduced), string(reviewverdict.NotReproduced), string(reviewverdict.NotRun)}
	for _, member := range []string{"result", "base_result"} {
		if got := alternativesOf(member); !reflect.DeepEqual(got, want) {
			t.Fatalf("the verifier delta lists %v for %s, want the values of the contract %v", got, member, want)
		}
	}
	instance := func(result, base string) []byte {
		filled := map[string]any{}
		for member := range verdict {
			value, ok := legal[member]
			switch member {
			case "result":
				value, ok = result, true
			case "base_result":
				value, ok = base, true
			}
			if !ok {
				t.Fatalf("the verifier delta names the member %q, and this test has no legal value for it", member)
			}
			filled[member] = value
		}
		text, err := json.Marshal(map[string]any{"verdicts": []any{filled}})
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		return text
	}
	for _, result := range want {
		for _, base := range want {
			if _, err := reviewverdict.Parse(instance(result, base), answered); err != nil {
				t.Errorf("reviewverdict.Parse refuses result %q with base_result %q, which the verifier delta lists: %v\n%s", result, base, err, instance(result, base))
			}
		}
	}
	// What the delta forbids, the parser refuses: an empty output, and no command although a check ran.
	for _, rule := range []struct{ says, member string }{
		{"output is never empty", "output"},
		{"command is never empty when a check ran", "command"},
	} {
		saved := legal[rule.member]
		legal[rule.member] = ""
		if _, err := reviewverdict.Parse(instance(string(reviewverdict.Reproduced), string(reviewverdict.NotRun)), answered); !errors.Is(err, reviewverdict.ErrMissingEvidence) {
			t.Errorf("the verifier delta says %q, and reviewverdict.Parse of a verdict with an empty %s returns %v, want %v", rule.says, rule.member, err, reviewverdict.ErrMissingEvidence)
		}
		legal[rule.member] = saved
	}
}
