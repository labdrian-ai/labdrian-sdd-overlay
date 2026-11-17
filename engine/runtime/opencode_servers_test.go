package runtime_test

import (
	"errors"
	"strings"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// The MCP servers are written with the absolute path of their program when it is found, and our entry
// is ours in either form.

func fakeLookup(found map[string]string) func(string) (string, error) {
	return func(name string) (string, error) {
		if path, ok := found[name]; ok {
			return path, nil
		}
		return "", errors.New("not found")
	}
}

func TestOpenCodeWritesTheAbsolutePathOfTheServersItFinds(t *testing.T) {
	root := t.TempDir()
	lookup := fakeLookup(map[string]string{"memorycard": "/opt/bin/memorycard", "codegraph": "/opt/bin/codegraph"})
	adapter, result := installIn(t, root, engineRuntime.OpenCodeOptions{PathLookup: lookup})

	servers := openCodeServers(t, readOpenCodeJSON(t, root))
	if got := jsonText(t, servers["memorycard"]); got != `{"codemode":true,"command":["/opt/bin/memorycard","mcp"],"type":"local"}` {
		t.Errorf("memorycard = %s", got)
	}
	if got := jsonText(t, servers["codegraph"]); got != `{"codemode":true,"command":["/opt/bin/codegraph","serve","--mcp"],"type":"local"}` {
		t.Errorf("codegraph = %s", got)
	}
	if strings.Contains(result.Message, "not found") {
		t.Errorf("message = %q: both were found", result.Message)
	}
	writeMatchingActiveMarker(t, root)
	if status := adapter.Status(); status.Status != core.CapabilitySupported {
		t.Errorf("Status() = %#v, want supported", status)
	}
}

func TestOpenCodeWritesTheBareNameAndSaysSoWhenTheServerIsNotOnThePath(t *testing.T) {
	root := t.TempDir()
	_, result := installIn(t, root, engineRuntime.OpenCodeOptions{PathLookup: fakeLookup(map[string]string{"codegraph": "/opt/bin/codegraph"})})

	servers := openCodeServers(t, readOpenCodeJSON(t, root))
	if got := jsonText(t, servers["memorycard"]); got != `{"codemode":true,"command":["memorycard","mcp"],"type":"local"}` {
		t.Errorf("memorycard = %s, want the bare name", got)
	}
	if !strings.Contains(result.Message, "memorycard not found on PATH") || strings.Contains(result.Message, "codegraph not found") {
		t.Errorf("message = %q, want it to say memorycard, and only memorycard, was not found", result.Message)
	}
}

func TestOpenCodeOwnsOurEntryInEitherFormAndNothingElse(t *testing.T) {
	root := t.TempDir()
	bare := `{"type":"local","command":["memorycard","mcp"],"codemode":true}`
	writeOpenCodeJSON(t, root, `{"default_agent":"foreman","mcp":{"servers":{"memorycard":`+bare+`,"codegraph":{"type":"local","command":["/x/y/codegraph","serve","--mcp"],"codemode":true}}}}`)
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{PathLookup: fakeLookup(map[string]string{"memorycard": "/opt/bin/memorycard", "codegraph": "/opt/bin/codegraph"})})

	adapter.Install()
	servers := openCodeServers(t, readOpenCodeJSON(t, root))
	if got := jsonText(t, servers["memorycard"]); !strings.Contains(got, `"memorycard","mcp"`) {
		t.Errorf("the bare entry of ours was replaced: %s", got)
	}
	writeMatchingActiveMarker(t, root)
	if status := adapter.Status(); status.Status != core.CapabilitySupported {
		t.Errorf("Status() = %#v, either form of our entry is current", status)
	}

	writeOpenCodeJSON(t, root, `{"default_agent":"foreman","mcp":{"servers":{"memorycard":{"type":"local","command":["/x/other-binary","mcp"],"codemode":true},"codegraph":{"type":"local","command":["codegraph","serve","--mcp"],"codemode":true}}}}`)
	if status := adapter.Status(); status.Status != core.CapabilityPartial || !strings.Contains(status.Message, "mcp.servers.memorycard") {
		t.Errorf("Status() = %#v, a command of another program is foreign", status)
	}
	adapter.Uninstall()
	servers = openCodeServers(t, readOpenCodeJSON(t, root))
	if servers["memorycard"] == nil || servers["codegraph"] != nil {
		t.Errorf("uninstall must remove only our entries: %#v", servers)
	}
}
