package skills

// Checks of the files that make up the role agents slice of Phase 9 (per-role design v3, section 6,
// PR 1), read from the repository the way oo_quality_contract_artifact_test.go reads its contract:
// the role sources and the Claude agent files of the builder, shaper, polisher, prototyper, sweeper
// and delivery; their rows in overlay.manifest; the minimalism contract the gate scopes to the roles
// that write; and the judgment-day skill, which names the role agents for its judges and its fix
// actor. The reviewer and the verifier are the next slice, and the files of the agents they replace
// stay where they are until a later one deletes them.

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// roleCorePrefix is the six lines every role agent starts with (per-role design v2, section 2,
// lines 49-54, which v3 section 2 takes verbatim): the same text in every file, with the name of the
// role in the first line.
func roleCorePrefix(role string) []string {
	return []string{
		"You are the " + role + " sub-agent of a labdrian workflow. Do the work yourself; never launch sub-agents.",
		"Read every artifact the prompt locates (file path, or Engram topic via mem_get_observation) before acting. A missing required locator is a blocker: report it, never substitute another store.",
		"Write only the artifacts and paths the prompt names; for any other path, stop and report it.",
		"Report only commands you ran and their exact results; an unrun check is \"not run\", never a pass.",
		"End with the return envelope as text, never a tool call: status success|partial|blocked, summary, artifacts, next, risks, skill_resolution.",
		"Artifacts are in English.",
	}
}

// The tool lists v3 section 2 takes from the agents that exist today: the builder's is the one of
// sdd-apply, the shaper's the one of sdd-propose (no Bash), the polisher's the read-only one of
// jd-judge-a, the delivery's the one of sdd-archive; the prototyper's is the one of its design with
// no mem_save, and the sweeper's is the builder's without Write, mem_save and mem_update.
const (
	builderTools   = "Read, Edit, Write, Glob, Grep, Bash, mcp__engram__mem_search, mcp__plugin_engram_engram__mem_search, mcp__engram__mem_get_observation, mcp__plugin_engram_engram__mem_get_observation, mcp__engram__mem_save, mcp__plugin_engram_engram__mem_save, mcp__engram__mem_update, mcp__plugin_engram_engram__mem_update, mcp__codegraph__codegraph_explore"
	shaperTools    = "Read, Edit, Write, Grep, Glob, mcp__engram__mem_search, mcp__plugin_engram_engram__mem_search, mcp__engram__mem_get_observation, mcp__plugin_engram_engram__mem_get_observation, mcp__engram__mem_save, mcp__plugin_engram_engram__mem_save, mcp__codegraph__codegraph_explore"
	polisherTools  = "Read, Glob, Grep, mcp__plugin_engram_engram__mem_search, mcp__plugin_engram_engram__mem_get_observation, mcp__codegraph__codegraph_explore"
	prototypeTools = "Read, Glob, Grep, Bash, Write, Edit, mcp__plugin_engram_engram__mem_search, mcp__plugin_engram_engram__mem_get_observation"
	deliveryTools  = "Read, Edit, Write, Glob, Bash, mcp__engram__mem_search, mcp__plugin_engram_engram__mem_search, mcp__engram__mem_get_observation, mcp__plugin_engram_engram__mem_get_observation, mcp__engram__mem_save, mcp__plugin_engram_engram__mem_save, mcp__codegraph__codegraph_explore"
)

// sweeperTools is the tool list of the builder without the tools that create files and persist
// memory: the sweep only removes and corrects, and its outputs are its edits and its envelope.
func sweeperTools() string {
	drop := map[string]bool{
		"Write": true, "mcp__engram__mem_save": true, "mcp__plugin_engram_engram__mem_save": true,
		"mcp__engram__mem_update": true, "mcp__plugin_engram_engram__mem_update": true,
	}
	var kept []string
	for _, tool := range strings.Split(builderTools, ", ") {
		if !drop[tool] {
			kept = append(kept, tool)
		}
	}
	return strings.Join(kept, ", ")
}

// roleAgentSpec is what v3 section 2 says of one role: the model and the tools of its agent file,
// the number of lines of its delta (the lines after the core prefix) and fragments that its delta
// must carry.
type roleAgentSpec struct {
	role       string
	model      string
	tools      string
	deltaLines int
	delta      []string
}

func roleAgentSpecs() []roleAgentSpec {
	return []roleAgentSpec{
		{"builder", "sonnet", builderTools, 4, []string{
			"Implement only the tasks the prompt names", "[x]", "apply-progress", "strict-tdd.md", "confirmed ledger ids", "one focused test per fix",
		}},
		{"shaper", "opus", shaperTools, 4, []string{
			"proposal, spec, design or tasks", "Never write code or tests", "risks", "exactly one question",
			"strongest counter-argument", "recommend one option", "tradeoff", "source",
		}},
		{"polisher", "sonnet", polisherTools, 3, []string{
			"Modify nothing", "immutable target", `{"findings":[],"evidence":`,
		}},
		{"prototyper", "sonnet", prototypeTools, 4, []string{
			"spike worktree", "mktemp -d", "positive control", "open_questions", "kill | narrow | continue", "lookup/spike box",
		}},
		{"sweeper", "sonnet", sweeperTools(), 4, []string{
			"Sweep only the candidates the prompt lists", "Never delete an exported symbol", "deadcode -test", "chore(sweep)", "nothing to sweep",
		}},
		{"delivery", "haiku", deliveryTools, 3, []string{
			"cp -R", "git mv", "diff -r", "Never rewrite checkboxes", "unfinished",
		}},
	}
}

// agentFile reads agents/<role>.md and splits it at the frontmatter: the text between the two ---
// lines, and what follows the second one.
func agentFile(t *testing.T, root, role string) (whole, frontmatter, rest string) {
	t.Helper()
	whole = readRepoFile(t, root, "agents/"+role+".md")
	parts := strings.SplitN(whole, "---", 3)
	if len(parts) < 3 || parts[0] != "" {
		t.Fatalf("agents/%s.md must start with a YAML frontmatter between two --- lines", role)
	}
	return whole, parts[1], parts[2]
}

func frontmatterKeys(frontmatter string) []string {
	var keys []string
	for _, line := range strings.Split(strings.TrimSpace(frontmatter), "\n") {
		key, _, _ := strings.Cut(line, ":")
		keys = append(keys, key)
	}
	return keys
}

// Each role has an agent file whose frontmatter is the v3 row of the role: its name, a one-line
// description, the model and the tools, and nothing else.
func TestRoleAgentFilesCarryTheFrontmatterOfTheirRole(t *testing.T) {
	root := skillsRepoRoot(t)
	for _, spec := range roleAgentSpecs() {
		t.Run(spec.role, func(t *testing.T) {
			_, frontmatter, _ := agentFile(t, root, spec.role)
			if got, want := frontmatterKeys(frontmatter), []string{"name", "description", "model", "tools"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("frontmatter keys = %v, want %v", got, want)
			}
			assertStringValue(t, parseYAMLScalar(t, frontmatter, "name"), spec.role)
			if description := parseYAMLScalar(t, frontmatter, "description"); len(description) < 20 {
				t.Errorf("description = %q, want a sentence that says what the role does", description)
			}
			assertStringValue(t, parseYAMLScalar(t, frontmatter, "model"), spec.model)
			assertStringValue(t, parseYAMLScalar(t, frontmatter, "tools"), spec.tools)
		})
	}
}

// The tool lists carry the limits the harness enforces in place of prose: no role launches
// sub-agents or reaches the web, the polisher cannot change anything, the shaper has no shell, the
// sweeper creates no file and saves no memory, and the prototyper saves no memory and has no
// CodeGraph.
func TestRoleAgentToolsCarryTheLimitsOfTheirRole(t *testing.T) {
	root := skillsRepoRoot(t)
	forbidden := map[string][]string{
		"builder":    {},
		"shaper":     {"Bash"},
		"polisher":   {"Write", "Edit", "Bash"},
		"prototyper": {"mem_save", "mem_update", "codegraph"},
		"sweeper":    {"Write", "mem_save", "mem_update"},
		"delivery":   {},
	}
	for _, spec := range roleAgentSpecs() {
		t.Run(spec.role, func(t *testing.T) {
			_, frontmatter, _ := agentFile(t, root, spec.role)
			tools := strings.Split(parseYAMLScalar(t, frontmatter, "tools"), ", ")
			for _, tool := range tools {
				for _, banned := range append([]string{"Agent", "Task", "WebFetch", "WebSearch", "NotebookEdit"}, forbidden[spec.role]...) {
					if tool == banned || strings.Contains(tool, banned) {
						t.Errorf("%s must not have the tool %q (it has %q)", spec.role, banned, tool)
					}
				}
			}
		})
	}
}

// Each agent file is its frontmatter and the role source roles/<role>/body.md, nothing between or
// after, so that the source is the one place the text of a role is edited. The body is the core
// prefix, a blank line and the delta of the role, in the number of lines v3 section 2 gives it.
func TestRoleAgentFilesAreTheirFrontmatterAndTheRoleSource(t *testing.T) {
	root := skillsRepoRoot(t)
	for _, spec := range roleAgentSpecs() {
		t.Run(spec.role, func(t *testing.T) {
			_, _, rest := agentFile(t, root, spec.role)
			body := readRepoFile(t, root, "roles/"+spec.role+"/body.md")
			if rest != "\n\n"+body {
				t.Fatalf("agents/%[1]s.md must be its frontmatter, a blank line and roles/%[1]s/body.md; edit the source and copy it", spec.role)
			}
			prefix := strings.Join(roleCorePrefix(spec.role), "\n")
			if !strings.HasPrefix(body, prefix+"\n\n") {
				t.Fatalf("roles/%s/body.md must start with the core prefix, verbatim:\n%s", spec.role, prefix)
			}
			delta := strings.Split(strings.TrimSuffix(strings.TrimPrefix(body, prefix+"\n\n"), "\n"), "\n")
			if len(delta) != spec.deltaLines {
				t.Fatalf("the delta of %s has %d lines, want %d:\n%s", spec.role, len(delta), spec.deltaLines, strings.Join(delta, "\n"))
			}
			for i, line := range delta {
				if strings.TrimSpace(line) == "" {
					t.Errorf("delta line %d of %s is blank", i+1, spec.role)
				}
			}
			for _, fragment := range spec.delta {
				if !strings.Contains(strings.Join(delta, "\n"), fragment) {
					t.Errorf("the delta of %s must carry %q", spec.role, fragment)
				}
			}
		})
	}
}

// The agent files are short, plain, English and free of what v3 leaves out of them: the markers
// gentle-ai stamps and syncs, a persona, and any text that is not ASCII (a unicode escape that a
// writer expanded into a character, or a symbol a model has to guess at).
func TestRoleAgentFilesAreShortPlainAndFreeOfStampsAndPersona(t *testing.T) {
	root := skillsRepoRoot(t)
	stamp := regexp.MustCompile(`<!--|gentle-ai:`)
	for _, spec := range roleAgentSpecs() {
		t.Run(spec.role, func(t *testing.T) {
			whole, _, _ := agentFile(t, root, spec.role)
			if lines := len(strings.Split(strings.TrimSuffix(whole, "\n"), "\n")); lines < 15 || lines > 30 {
				t.Errorf("agents/%s.md has %d lines, want 15 to 30 (v3 section 2: about 20 to 30)", spec.role, lines)
			}
			if stamp.MatchString(whole) {
				t.Errorf("agents/%s.md carries a gentle-ai stamped block or an HTML comment", spec.role)
			}
			if lower := strings.ToLower(whole); strings.Contains(lower, "gadu") || strings.Contains(lower, "persona") {
				t.Errorf("agents/%s.md names a persona; the roles have none", spec.role)
			}
			for i := 0; i < len(whole); i++ {
				if whole[i] > 0x7e || (whole[i] < 0x20 && whole[i] != '\n') {
					t.Fatalf("agents/%s.md has a byte that is not plain ASCII text at offset %d", spec.role, i)
				}
			}
			if strings.Contains(whole, `\u`) {
				t.Errorf("agents/%s.md has a \\u escape", spec.role)
			}
		})
	}
}

// Each role agent is deployed to Claude Code by a row of overlay.manifest that the overlay owns
// (custom, not from upstream) and that takes the agent route: its file is agents/<role>.md and it is
// copied to the agents directory of Claude Code and to no other runtime. The role sources are not
// deployed, so no row names them.
func TestRoleAgentsAreDeployedToClaudeByCustomAgentRows(t *testing.T) {
	root := skillsRepoRoot(t)
	rows := ReadManifestRows([]byte(readRepoFile(t, root, "overlay.manifest")))
	dest := RouteDestinations{Target: "claude", SkillsRoot: "/h/.claude/skills", AgentsRoot: "/h/.claude/agents"}
	routes, err := ResolveRoutes(rows, false, "/o", dest)
	if err != nil {
		t.Fatalf("ResolveRoutes over the manifest of the repository: %v", err)
	}
	for _, spec := range roleAgentSpecs() {
		path := spec.role + ".md"
		var found []ManifestRow
		for _, row := range rows {
			if row.Path == path {
				found = append(found, row)
			}
		}
		if want := []ManifestRow{{Path: path, Tag: "custom", Route: RouteAgent}}; !reflect.DeepEqual(found, want) {
			t.Errorf("rows named %s = %+v, want exactly %+v", path, found, want)
			continue
		}
		var route ResolvedRoute
		for _, r := range routes {
			if r.Path == path {
				route = r
			}
		}
		want := ResolvedRoute{Path: path, Route: RouteAgent, RepoRel: "agents/" + path, RepoSource: "/o/agents/" + path, Dest: "/h/.claude/agents/" + path}
		if route != want {
			t.Errorf("%s resolves to %+v, want %+v", path, route, want)
		}
		if _, err := os.Stat(filepath.Join(root, "agents", path)); err != nil {
			t.Errorf("the row %s names a file that is not in the repository: %v", path, err)
		}
	}
	for _, row := range rows {
		if strings.HasPrefix(row.Path, "roles/") {
			t.Errorf("row %q: the role sources are not deployed, so no row names them", row.Path)
		}
	}
}

// The overlay's own agents reach no runtime but Claude Code, and none is a row from upstream: a sync
// or a restore from upstream never takes the role agents for its own.
func TestRoleAgentsAreNeitherCopiedToOtherRuntimesNorManaged(t *testing.T) {
	root := skillsRepoRoot(t)
	rows := ReadManifestRows([]byte(readRepoFile(t, root, "overlay.manifest")))
	for _, target := range []string{"codex", "pi", "opencode"} {
		dest := RouteDestinations{Target: target, SkillsRoot: "/h/skills", AgentsRoot: "/h/agents"}
		routes, err := ResolveRoutes(rows, false, "/o", dest)
		if err != nil {
			t.Fatalf("ResolveRoutes for %s: %v", target, err)
		}
		for _, r := range routes {
			for _, spec := range roleAgentSpecs() {
				if r.Path == spec.role+".md" && r.Dest != "" {
					t.Errorf("%s: %s is copied to %s, want it copied nowhere", target, r.Path, r.Dest)
				}
			}
		}
	}
	managed, err := ResolveRoutes(rows, true, "/o", RouteDestinations{Target: "claude", SkillsRoot: "/h/skills", AgentsRoot: "/h/agents"})
	if err != nil {
		t.Fatalf("ResolveRoutes of the managed rows: %v", err)
	}
	for _, r := range managed {
		for _, spec := range roleAgentSpecs() {
			if r.Path == spec.role+".md" {
				t.Errorf("%s is a managed row; the role agents belong to the overlay", r.Path)
			}
		}
	}
}

// The agents that exist before the role agents keep their rows, their tags and their files: the
// slice replaces none of them (they are deleted by a later one).
func TestTheAgentsBeforeTheRoleAgentsKeepTheirRows(t *testing.T) {
	root := skillsRepoRoot(t)
	rows := ReadManifestRows([]byte(readRepoFile(t, root, "overlay.manifest")))
	want := []ManifestRow{
		{"GADU.md", "custom", RouteAgent}, {"opencode/agents/GADU.md", "custom", RouteOpencodeAgent},
		{"sdd-apply.md", "managed", RouteAgent}, {"sdd-archive.md", "managed", RouteAgent}, {"sdd-design.md", "managed", RouteAgent},
		{"sdd-init.md", "managed", RouteAgent}, {"sdd-onboard.md", "managed", RouteAgent}, {"sdd-propose.md", "managed", RouteAgent},
		{"sdd-spec.md", "managed", RouteAgent}, {"sdd-tasks.md", "managed", RouteAgent}, {"sdd-verify.md", "managed", RouteAgent},
		{"jd-fix-agent.md", "managed", RouteAgent}, {"jd-judge-a.md", "managed", RouteAgent}, {"jd-judge-b.md", "managed", RouteAgent},
		{"review-readability.md", "managed", RouteAgent}, {"review-refuter.md", "managed", RouteAgent},
		{"review-reliability.md", "managed", RouteAgent}, {"review-resilience.md", "managed", RouteAgent}, {"review-risk.md", "managed", RouteAgent},
	}
	for _, w := range want {
		var got []ManifestRow
		for _, row := range rows {
			if row.Path == w.Path {
				got = append(got, row)
			}
		}
		if !reflect.DeepEqual(got, []ManifestRow{w}) {
			t.Errorf("rows named %s = %+v, want exactly %+v", w.Path, got, w)
		}
		file := filepath.Join(root, "agents", w.Path)
		if w.Route == RouteOpencodeAgent {
			file = filepath.Join(root, w.Path)
		}
		if _, err := os.Stat(file); err != nil {
			t.Errorf("the file of the row %s is gone: %v", w.Path, err)
		}
	}
}

// The minimalism contract is scoped to the roles that write, kept out of the roles that judge and
// verify and of the phases it always excluded, and still injects under the heading it always did.
// sdd-tasks and sdd-apply keep it until their agents are removed (owner decision, 2026-10-22).
func TestMinimalismContractIsScopedToTheRolesThatWrite(t *testing.T) {
	root := skillsRepoRoot(t)
	frontmatter := ooQualityFrontmatter(t, readRepoFile(t, root, "skills/_shared/minimalism-contract.md"))
	if got, want := parseInlineYAMLList(t, frontmatter, "applies_to_phases"), []string{"sdd-tasks", "sdd-apply", "shaper", "builder"}; !reflect.DeepEqual(got, want) {
		t.Errorf("applies_to_phases = %v, want %v", got, want)
	}
	wantExcluded := []string{"sdd-propose", "sdd-spec", "sdd-design", "sdd-verify", "sdd-archive", "reviewer", "verifier"}
	if got := parseInlineYAMLList(t, frontmatter, "excluded_phases"); !reflect.DeepEqual(got, wantExcluded) {
		t.Errorf("excluded_phases = %v, want %v", got, wantExcluded)
	}
	assertStringValue(t, parseYAMLScalar(t, frontmatter, "injection_point"), "## Skills to load before work")
}

// The note that says how the scope works names what really reads the frontmatter: the gate that
// injects or strips the contract at every sub-agent launch and the propagation of the registry row.
// The earlier note said that no resolver parses it, which the gate has made untrue.
func TestMinimalismContractNoteNamesWhatParsesItsFrontmatter(t *testing.T) {
	contract := readRepoFile(t, skillsRepoRoot(t), "skills/_shared/minimalism-contract.md")
	if strings.Contains(contract, "resolver does not parse it") || strings.Contains(contract, "documentation only") {
		t.Error("the note still says the frontmatter is documentation that no resolver parses")
	}
	for _, reader := range []string{"engine gate-task", "engine propagate", "`shaper` and `builder`"} {
		if !strings.Contains(contract, reader) {
			t.Errorf("the note must mention %s", reader)
		}
	}
}

// What the contract tells a role to do is not touched by its scope: the ladder, the tiebreaker and
// the comment convention are all still there.
func TestMinimalismContractKeepsItsRules(t *testing.T) {
	contract := readRepoFile(t, skillsRepoRoot(t), "skills/_shared/minimalism-contract.md")
	for _, rule := range []string{
		"# Minimalism Contract", "## Preference ladder", "1. **YAGNI**", "2. **stdlib / language built-ins**",
		"3. **Native platform feature**", "4. **Existing dependency**", "5. **One-liner / minimal local code**",
		"6. **Custom code / new abstraction**", "## Architectural tiebreaker (mandatory)", "## Comment convention",
		"`// minimal: <reason>`", "`// minimal: forced — <design/constraint ref>`",
	} {
		if !strings.Contains(contract, rule) {
			t.Errorf("the minimalism contract lost %q", rule)
		}
	}
}

// The judgment-day skill launches the polisher for its two judges, with a different per-launch model
// for the second, and the builder for its fix actor; the names of the agents it replaces are gone from
// it, and the protocol they ran is the one it had.
func TestJudgmentDayLaunchesTheRoleAgentsForItsActors(t *testing.T) {
	skill := readRepoFile(t, skillsRepoRoot(t), "skills/judgment-day/SKILL.md")
	for _, legacy := range []string{"jd-judge-a", "jd-judge-b", "jd-fix-agent"} {
		if strings.Contains(skill, legacy) {
			t.Errorf("the skill still names %s", legacy)
		}
	}
	for _, named := range []string{"`polisher`", "`builder`", "per-launch `model`"} {
		if !strings.Contains(skill, named) {
			t.Errorf("the skill must name %s", named)
		}
	}
	for _, model := range []string{"`sonnet`", "`opus`"} {
		if !strings.Contains(skill, model) {
			t.Errorf("the skill must say which model each judge runs: %s is missing", model)
		}
	}
}

// What the judgment-day protocol says is the same after the renaming: two blind judges and no
// refuter, at most two fix rounds and two scoped re-judgments, the two terminal verdicts, and no
// receipt or delivery authority. The skill's references still point at files that exist.
func TestJudgmentDayProtocolIsUnchanged(t *testing.T) {
	root := skillsRepoRoot(t)
	skill := readRepoFile(t, root, "skills/judgment-day/SKILL.md")
	for _, rule := range []string{
		"launch two blind read-only judges in parallel with identical scope and criteria",
		"Each judge returns one neutral findings result and terminates. Wait for both; never accept a partial judgment.",
		"Never launch `review-refuter`; two-judge agreement is the corroboration mechanism.",
		"Fix only severe findings confirmed by both judges. WARNING/SUGGESTION rows remain `info`.",
		"Permit at most two fix rounds and two scoped re-judgments.",
		"The only terminal verdicts are `APPROVED | ESCALATED`",
		"A judgment issues no receipt and carries no delivery authority",
		"| Both judges confirm severe finding | Ask before round-one correction; then use the bounded fix actor. |",
		"`JUDGMENT: APPROVED ✅` or `JUDGMENT: ESCALATED ⚠️`",
	} {
		if !strings.Contains(skill, rule) {
			t.Errorf("the protocol of the skill lost %q", rule)
		}
	}
	for _, link := range regexp.MustCompile(`\]\(([^)#]+)\)`).FindAllStringSubmatch(skill, -1) {
		if _, err := os.Stat(filepath.Join(root, "skills", "judgment-day", link[1])); err != nil {
			t.Errorf("the skill refers to %s, which is not there: %v", link[1], err)
		}
	}
}
