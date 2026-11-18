package agentgen_test

// The rules of the OpenCode files on the tools of the MCP servers (S7 of odd/tasks/labdrian-orchestrator-opencode.md,
// L102 (2), L103). Under Code Mode an agent that keeps execute reaches every tool of every server, so each
// agent gets, for each server, a deny on `<server>_*` and then an allow of each `<server>_<tool>` its Claude
// list names as `mcp__<server>__<tool>`. L102 (2) measured the form: the last matching rule wins.

import (
	"slices"
	"strings"
	"testing"
)

// openCodeServerNames are the servers of the labdrian flow, in the order the rules list them.
var openCodeServerNames = []string{"memorycard", "longterm-mem", "codegraph"}

// serverRuleLines are the rules of the file on a tool of a server, in the order of the file, each as
// "effect action" so a table row reads as the file does.
func (f openCodeFile) serverRuleLines() []string {
	var out []string
	for _, r := range f.rules {
		for _, server := range openCodeServerNames {
			if strings.HasPrefix(r.action, server+"_") {
				out = append(out, r.effect+" "+r.action)
			}
		}
	}
	return out
}

// wantServerRules are the rules of a file on the servers: the deny of each server and the allows of the
// tools of the Claude list, which the table names by their OpenCode name.
func wantServerRules(memory, longterm, codegraph []string) []string {
	var out []string
	for _, c := range []struct {
		server string
		allows []string
	}{{"memorycard", memory}, {"longterm-mem", longterm}, {"codegraph", codegraph}} {
		out = append(out, "deny "+c.server+"_*")
		for _, tool := range c.allows {
			out = append(out, "allow "+c.server+"_"+tool)
		}
	}
	return out
}

func TestTheOpenCodeFileDeniesEachServerAndAllowsOnlyTheToolsOfTheClaudeList(t *testing.T) {
	explore := []string{"codegraph_explore"}
	rows := []struct {
		agent string
		want  []string
	}{
		{"builder", wantServerRules([]string{"mem_search", "mem_get", "mem_save", "mem_update"}, nil, explore)},
		{"shaper", wantServerRules([]string{"mem_search", "mem_get", "mem_save"}, nil, explore)},
		// The polisher is read-only: it searches and reads memorycard and explores the code.
		{"polisher", wantServerRules([]string{"mem_search", "mem_get"}, nil, explore)},
		{"prototyper", wantServerRules([]string{"mem_search", "mem_get"}, nil, nil)},
		{"sweeper", wantServerRules([]string{"mem_search", "mem_get"}, nil, explore)},
		// The reviewer names no MCP tool: the three denies and no allow.
		{"reviewer", wantServerRules(nil, nil, nil)},
		// The verifier holds execute and names only mem_get: every other tool of the servers is denied.
		{"verifier", wantServerRules([]string{"mem_get"}, nil, nil)},
		{"delivery", wantServerRules([]string{"mem_search", "mem_get", "mem_save"}, nil, explore)},
		{"foreman", wantServerRules([]string{"mem_search", "mem_get", "mem_save", "mem_update"}, nil, explore)},
	}
	files := artifactContents(t, seededRoot(t))
	for _, row := range rows {
		t.Run(row.agent, func(t *testing.T) {
			f := parseOpenCode(t, files["opencode/agents/"+row.agent+".md"])
			if got := f.serverRuleLines(); !slices.Equal(got, row.want) {
				t.Errorf("the rules on the servers are\n%v\nwant\n%v", got, row.want)
			}
		})
	}
}

// The verifier and the reviewer in the words of S7: the verifier keeps execute and every other tool of
// memorycard is denied; the reviewer keeps execute denied and has no allow at all.
func TestTheVerifierKeepsExecuteAndTheReviewerDeniesItAndAllowsNothing(t *testing.T) {
	files := artifactContents(t, seededRoot(t))
	verifier := parseOpenCode(t, files["opencode/agents/verifier.md"])
	if slices.Contains(verifier.denied(t), "execute") {
		t.Errorf("the verifier denies execute, which its Claude list (Bash, mem_get) keeps")
	}
	reviewer := parseOpenCode(t, files["opencode/agents/reviewer.md"])
	if !slices.Contains(reviewer.denied(t), "execute") {
		t.Errorf("the reviewer does not deny execute: %v", reviewer.denied(t))
	}
	for _, r := range reviewer.rules {
		if r.effect == "allow" {
			t.Errorf("the reviewer allows %s, and its Claude list names no tool of a server", r.action)
		}
	}
}

// The last matching rule wins (L102 (2)), so an allow before the deny of its server is removed by it: in
// every file the deny of a server comes first, and every allow of a server follows its deny. Each allow and
// each deny covers every resource.
func TestNoAllowOfAServerPrecedesTheDenyOfThatServer(t *testing.T) {
	files := artifactContents(t, seededRoot(t))
	for _, a := range wantOpenCodeAgents() {
		t.Run(a.name, func(t *testing.T) {
			f := parseOpenCode(t, files["opencode/agents/"+a.name+".md"])
			denyAt := map[string]int{}
			for i, r := range f.rules {
				if r.resource != `"*"` {
					t.Errorf("the rule %+v does not cover every resource", r)
				}
				for _, server := range openCodeServerNames {
					switch {
					case r.effect == "deny" && r.action == server+"_*":
						denyAt[server] = i + 1
					case r.effect == "allow" && strings.HasPrefix(r.action, server+"_"):
						if denyAt[server] == 0 {
							t.Errorf("rule %d allows %s before the deny of %s_*", i+1, r.action, server)
						}
					}
				}
			}
			for _, server := range openCodeServerNames {
				if denyAt[server] == 0 {
					t.Errorf("no deny of %s_*", server)
				}
			}
		})
	}
}
