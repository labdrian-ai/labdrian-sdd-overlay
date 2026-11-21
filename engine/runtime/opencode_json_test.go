package runtime_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// The tests below pin what the OpenCode adapter keeps in <config>/opencode.json (S12): the default
// agent and the memorycard and codegraph MCP servers in the v2 shape measured in L10 T4. Every
// other key of the file is the owner's.

func openCodeJSONPath(root string) string { return filepath.Join(root, "opencode.json") }

func readOpenCodeJSON(t *testing.T, root string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(openCodeJSONPath(root))
	if err != nil {
		t.Fatalf("read opencode.json: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("decode opencode.json: %v\n%s", err, data)
	}
	return doc
}

func writeOpenCodeJSON(t *testing.T, root, text string) {
	t.Helper()
	if err := os.WriteFile(openCodeJSONPath(root), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func openCodeServers(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	mcp, _ := doc["mcp"].(map[string]any)
	servers, _ := mcp["servers"].(map[string]any)
	if servers == nil {
		t.Fatalf("opencode.json has no mcp.servers: %#v", doc)
	}
	return servers
}

func jsonText(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestOpenCodeInstallWritesTheV2ConfigWithForemanAndTheMCPServers(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{})

	result := adapter.Install()

	if result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	doc := readOpenCodeJSON(t, root)
	if doc["default_agent"] != "foreman" {
		t.Errorf("default_agent = %#v, want foreman", doc["default_agent"])
	}
	servers := openCodeServers(t, doc)
	want := map[string]string{
		"memorycard": `{"codemode":true,"command":["memorycard","mcp"],"type":"local"}`,
		"codegraph":  `{"codemode":true,"command":["codegraph","serve","--mcp"],"type":"local"}`,
	}
	for name, entry := range want {
		if got := jsonText(t, servers[name]); got != entry {
			t.Errorf("mcp.servers.%s = %s, want %s", name, got, entry)
		}
	}
	if len(servers) != len(want) {
		t.Errorf("mcp.servers = %#v, want only memorycard and codegraph", servers)
	}
	for _, key := range []string{"plugin", "plugins", "providers", "provider", "model"} {
		if _, present := doc[key]; present {
			t.Errorf("opencode.json holds %q, which the install does not own (the plugin directory is loaded without it)", key)
		}
	}
	if !strings.Contains(result.Message, "opencode.json") {
		t.Errorf("message = %q, want it to say what happened to opencode.json", result.Message)
	}
}

func TestOpenCodeInstallKeepsEveryForeignKeyAndWritesABackupOnce(t *testing.T) {
	root := t.TempDir()
	original := `{"$schema":"https://opencode.ai/config.json","permission":{"edit":"ask"},"mcp":{"servers":{"longterm-mem":{"type":"local","command":["/x/longterm-mem","mcp"],"codemode":true}}},"share":"manual"}`
	writeOpenCodeJSON(t, root, original)
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{})

	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}

	doc := readOpenCodeJSON(t, root)
	if doc["share"] != "manual" || doc["$schema"] != "https://opencode.ai/config.json" || jsonText(t, doc["permission"]) != `{"edit":"ask"}` {
		t.Errorf("foreign keys changed: %#v", doc)
	}
	servers := openCodeServers(t, doc)
	if jsonText(t, servers["longterm-mem"]) != `{"codemode":true,"command":["/x/longterm-mem","mcp"],"type":"local"}` {
		t.Errorf("the longterm-mem server changed: %#v", servers["longterm-mem"])
	}
	backup, err := os.ReadFile(openCodeJSONPath(root) + ".labdrian.bak")
	if err != nil || string(backup) != original {
		t.Fatalf("backup = %q, %v; want the file as the install found it", backup, err)
	}
	// A later change must not replace the backup of the file as the owner had it.
	doc2 := readOpenCodeJSON(t, root)
	delete(openCodeServers(t, doc2), "codegraph")
	data, _ := json.Marshal(doc2)
	writeOpenCodeJSON(t, root, string(data))
	if result := adapter.Update(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Update() = %#v", result)
	}
	if backup, _ := os.ReadFile(openCodeJSONPath(root) + ".labdrian.bak"); string(backup) != original {
		t.Errorf("backup was replaced: %q", backup)
	}
}

func TestOpenCodeInstallTwiceWritesNothingTheSecondTime(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{})
	adapter.Install()
	first, err := os.ReadFile(openCodeJSONPath(root))
	if err != nil {
		t.Fatal(err)
	}
	info1, _ := os.Stat(openCodeJSONPath(root))

	result := adapter.Update()

	second, _ := os.ReadFile(openCodeJSONPath(root))
	info2, _ := os.Stat(openCodeJSONPath(root))
	if string(first) != string(second) || !info1.ModTime().Equal(info2.ModTime()) {
		t.Errorf("the second install rewrote opencode.json:\n%s\n---\n%s", first, second)
	}
	if !strings.Contains(result.Message, "already current") {
		t.Errorf("message = %q, want it to say opencode.json is already current", result.Message)
	}
	if _, err := os.Stat(openCodeJSONPath(root) + ".labdrian.bak"); !os.IsNotExist(err) {
		t.Errorf("a file the install created has no backup, stat err: %v", err)
	}
}

func TestOpenCodeInstallLeavesAForeignDefaultAgentAndSaysSo(t *testing.T) {
	root := t.TempDir()
	writeOpenCodeJSON(t, root, `{"default_agent":"build"}`)
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{})

	install := adapter.Install()

	if got := readOpenCodeJSON(t, root)["default_agent"]; got != "build" {
		t.Errorf("default_agent = %#v, a foreign value must stay", got)
	}
	if !strings.Contains(install.Message, `"build"`) || !strings.Contains(install.Message, "foreman") {
		t.Errorf("message = %q, want it to name the foreign value and the foreman it did not enable", install.Message)
	}
	writeMatchingActiveMarker(t, root)
	status := adapter.Status()
	if status.Status != core.CapabilityPartial || !strings.Contains(status.Message, "default_agent") {
		t.Errorf("Status() = %#v, want partial naming default_agent", status)
	}
}

func TestOpenCodeInstallLeavesAForeignMCPEntryAndSaysSo(t *testing.T) {
	root := t.TempDir()
	foreign := `{"mcp":{"servers":{"memorycard":{"type":"local","command":["/opt/mc","serve"],"codemode":false}}}}`
	writeOpenCodeJSON(t, root, foreign)
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{})

	install := adapter.Install()

	servers := openCodeServers(t, readOpenCodeJSON(t, root))
	if got := jsonText(t, servers["memorycard"]); got != `{"codemode":false,"command":["/opt/mc","serve"],"type":"local"}` {
		t.Errorf("the foreign memorycard server changed: %s", got)
	}
	if servers["codegraph"] == nil {
		t.Errorf("the codegraph server is ours and absent, it should have been added: %#v", servers)
	}
	if !strings.Contains(install.Message, "mcp.servers.memorycard") {
		t.Errorf("message = %q, want it to name mcp.servers.memorycard", install.Message)
	}
	writeMatchingActiveMarker(t, root)
	if status := adapter.Status(); status.Status != core.CapabilityPartial || !strings.Contains(status.Message, "mcp.servers.memorycard") {
		t.Errorf("Status() = %#v, want partial naming mcp.servers.memorycard", status)
	}
}

func TestOpenCodeInstallRefusesAFileItCannotParseAndChangesNothing(t *testing.T) {
	for name, text := range map[string]string{
		"not json":              `{"default_agent": foreman`,
		"a list":                `[]`,
		"null":                  `null`,
		"mcp is not an object":  `{"mcp":[]}`,
		"servers is not":        `{"mcp":{"servers":"x"}}`,
		"servers is a 1.x item": `{"mcp":{"servers":{"type":"local","command":["a"]}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeOpenCodeJSON(t, root, text)
			adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{})

			result := adapter.Install()

			if result.Status != core.CapabilityPartial || !strings.Contains(result.Message, "opencode.json") {
				t.Fatalf("Install() = %#v, want partial naming opencode.json", result)
			}
			if got, _ := os.ReadFile(openCodeJSONPath(root)); string(got) != text {
				t.Errorf("the file changed to %q", got)
			}
			if _, err := os.Stat(openCodeJSONPath(root) + ".labdrian.bak"); !os.IsNotExist(err) {
				t.Errorf("a backup was written for a file that was refused, stat err: %v", err)
			}
		})
	}
}

func TestOpenCodeStatusIsPartialWhileTheOwnedEntriesAreMissing(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{})
	adapter.Install()
	writeMatchingActiveMarker(t, root)
	if status := adapter.Status(); status.Status != core.CapabilitySupported {
		t.Fatalf("Status() after install = %#v, want supported", status)
	}

	if err := os.Remove(openCodeJSONPath(root)); err != nil {
		t.Fatal(err)
	}

	status := adapter.Status()
	if status.Status != core.CapabilityPartial || !strings.Contains(status.Message, "engine runtime install --target opencode") {
		t.Errorf("Status() = %#v, want partial naming the verb that fixes it", status)
	}
}

func TestOpenCodeUninstallRemovesOnlyTheOwnedEntries(t *testing.T) {
	root := t.TempDir()
	writeOpenCodeJSON(t, root, `{"default_agent":"foreman","mcp":{"servers":{"memorycard":{"type":"local","command":["memorycard","mcp"],"codemode":true},"codegraph":{"type":"local","command":["other"],"codemode":true},"mine":{"type":"remote","url":"https://x"}}},"share":"auto"}`)
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{})

	result := adapter.Uninstall()

	if result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Uninstall() = %#v", result)
	}
	doc := readOpenCodeJSON(t, root)
	if _, present := doc["default_agent"]; present {
		t.Errorf("the owned default_agent stayed: %#v", doc)
	}
	servers := openCodeServers(t, doc)
	if servers["memorycard"] != nil {
		t.Errorf("the owned memorycard server stayed: %#v", servers)
	}
	if servers["codegraph"] == nil || servers["mine"] == nil || doc["share"] != "auto" {
		t.Errorf("a foreign entry was removed: %#v", doc)
	}
}

func TestOpenCodeUninstallGivesBackAnAbsentFileAbsentAndKeepsAForeignAgent(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{})
	adapter.Install()
	adapter.Uninstall()
	if _, err := os.Stat(openCodeJSONPath(root)); !os.IsNotExist(err) {
		t.Errorf("opencode.json holds nothing after the uninstall and should be gone, stat err: %v", err)
	}

	writeOpenCodeJSON(t, root, `{"default_agent":"build"}`)
	adapter.Install()
	adapter.Uninstall()
	if got := readOpenCodeJSON(t, root)["default_agent"]; got != "build" {
		t.Errorf("default_agent = %#v, a foreign value must survive the uninstall", got)
	}
}

func TestOpenCodeUninstallOfAFileItCannotParseSaysSoAndKeepsIt(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{})
	adapter.Install()
	writeOpenCodeJSON(t, root, `{broken`)

	result := adapter.Uninstall()

	if result.Status != core.CapabilityPartial || !strings.Contains(result.Message, "opencode.json") {
		t.Errorf("Uninstall() = %#v, want partial naming opencode.json", result)
	}
	if got, _ := os.ReadFile(openCodeJSONPath(root)); string(got) != `{broken` {
		t.Errorf("the file changed to %q", got)
	}
	if _, err := os.Stat(openCodePluginPath(root)); !os.IsNotExist(err) {
		t.Errorf("the plugin is removed whatever happened to opencode.json, stat err: %v", err)
	}
}
