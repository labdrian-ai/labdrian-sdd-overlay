package agentgen_test

// The denies of the OpenCode files on the v2 tools other than edit and shell (S7 of
// odd/tasks/labdrian-orchestrator-opencode.md, L16, L19, L21). A deny on a tool the Claude list of an agent has no
// tool for keeps the OpenCode agent to what its Claude file allows: no role launches a sub-agent, asks a
// question, loads a skill, reaches the web or generates an image, and a role with no MCP tool has the Code Mode
// wrapper execute denied. The foreman has its own set, derived the same way from its own Claude list.

import (
	"slices"
	"strings"
	"testing"
)

// The full list of denies of every agent is the first group, then the others, in the order the file lists
// them, each a deny on every resource and nothing else.
func TestTheOpenCodeFileDeniesTheOtherV2ToolsTheClaudeListHasNoToolFor(t *testing.T) {
	files := artifactContents(t, seededRoot(t))
	for _, a := range wantOpenCodeAgents() {
		t.Run(a.name, func(t *testing.T) {
			f := parseOpenCode(t, files["opencode/agents/"+a.name+".md"])
			want := append(slices.Clone(a.editShell), a.others...)
			if got := f.denied(t); !slices.Equal(got, want) {
				t.Errorf("the denies are %v, want %v", got, want)
			}
			// Every file has a deny, so permissions is its fourth key.
			if !slices.Equal(f.keys, []string{"description", "mode", "model", "permissions"}) {
				t.Errorf("the frontmatter keys are %v, want description, mode, model, permissions", f.keys)
			}
		})
	}
}

// None of the eight roles holds the subagent tool: each body says "never launch sub-agents", so subagent is
// denied in every one of their files.
func TestNoRoleOfTheEightHoldsTheSubagentTool(t *testing.T) {
	files := artifactContents(t, seededRoot(t))
	for _, a := range wantOpenCodeAgents() {
		if a.name == "foreman" {
			continue
		}
		if got := parseOpenCode(t, files["opencode/agents/"+a.name+".md"]).denied(t); !slices.Contains(got, "subagent") {
			t.Errorf("%s does not deny subagent: %v", a.name, got)
		}
	}
}

// Only the reviewer names no MCP tool in its Claude list, so only its file denies execute.
func TestOnlyTheRoleWithNoMcpToolHasExecuteDenied(t *testing.T) {
	files := artifactContents(t, seededRoot(t))
	var denying []string
	for _, a := range wantOpenCodeAgents() {
		if slices.Contains(parseOpenCode(t, files["opencode/agents/"+a.name+".md"]).denied(t), "execute") {
			denying = append(denying, a.name)
		}
	}
	if !slices.Equal(denying, []string{"reviewer"}) {
		t.Errorf("the agents that deny execute are %v, want only the reviewer", denying)
	}
}

// The foreman row (S7, acceptance of L21): mode primary, the frontier model and variant, no deny on edit,
// shell, subagent, execute or skill, and a deny on exactly question, webfetch, websearch and
// experimental_image_generate.
func TestTheForemanFileKeepsItsToolsAndDeniesExactlyTheFourOthers(t *testing.T) {
	files := artifactContents(t, seededRoot(t))
	f := parseOpenCode(t, files["opencode/agents/foreman.md"])
	if f.values["mode"] != "primary" {
		t.Errorf("mode = %q, want primary", f.values["mode"])
	}
	if f.values["model"] != ocFrontier {
		t.Errorf("model = %q, want %q", f.values["model"], ocFrontier)
	}
	denied := f.denied(t)
	for _, kept := range []string{"edit", "shell", "subagent", "execute", "skill"} {
		if slices.Contains(denied, kept) {
			t.Errorf("the foreman file denies %s, which the foreman keeps", kept)
		}
	}
	want := []string{"question", "webfetch", "websearch", "experimental_image_generate"}
	slices.Sort(want)
	got := slices.Clone(denied)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("the foreman denies %v, want exactly %v", denied, want)
	}
}

// A deny on a tool the eight roles share is written as the schema has it: an action, a resource that
// matches every use (quoted, since a bare * starts a YAML alias) and the effect deny.
func TestADenyIsAnActionOnEveryResourceWithTheEffectDeny(t *testing.T) {
	files := artifactContents(t, seededRoot(t))
	text := files["opencode/agents/reviewer.md"]
	const rule = "  - action: question\n    resource: \"*\"\n    effect: deny\n"
	if strings.Count(text, rule) != 1 {
		t.Errorf("opencode/agents/reviewer.md does not hold the rule\n%s exactly once:\n%s", rule, text)
	}
}
