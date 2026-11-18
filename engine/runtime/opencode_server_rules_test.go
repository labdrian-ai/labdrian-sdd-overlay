package runtime_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/agentgen"
	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
)

// S7 of odd/tasks/labdrian-orchestrator-opencode.md (T39): every OpenCode agent file denies `<server>_*` for each
// MCP server the adapter writes into opencode.json, because under Code Mode an agent that keeps execute reaches
// every tool of every server. The list of servers lives in engine/agentgen, the file in this package, so the test
// sits here, where both are reachable (engine/architecture_test.go lets runtime import agentgen, as pi_agents.go
// does): a server added to opencode.json and not to the list leaves every agent with that server's tools.
func TestEveryServerOfOpencodeJSONHasADenyInEveryOpenCodeAgentFile(t *testing.T) {
	root := t.TempDir()
	installIn(t, root, engineRuntime.OpenCodeOptions{PathLookup: fakeLookup(nil)})
	servers := openCodeServers(t, readOpenCodeJSON(t, root))
	if len(servers) == 0 {
		t.Fatal("opencode.json names no MCP server")
	}

	artifacts, err := agentgen.Artifacts(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Artifacts: %v", err)
	}
	var agents int
	for _, a := range artifacts {
		if !strings.HasPrefix(a.Path, "opencode/agents/") {
			continue
		}
		agents++
		for server := range servers {
			if !strings.Contains(a.Content, "  - action: "+server+"_*\n    resource: \"*\"\n    effect: deny\n") {
				t.Errorf("%s has no deny of %s_*, though opencode.json names the server %s", a.Path, server, server)
			}
		}
	}
	if agents != 9 {
		t.Errorf("%d OpenCode agent files, want nine", agents)
	}
}
