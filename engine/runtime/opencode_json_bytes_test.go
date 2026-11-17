package runtime_test

import (
	"os"
	"strings"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// These tests read the bytes of opencode.json, not the values: the owner's file keeps its key order,
// its indentation and every byte the adapter does not own (the writer of longterm-mem splices the
// same way), and an uninstall after an install gives the owner's bytes back.

func installIn(t *testing.T, root string, options engineRuntime.OpenCodeOptions) (engineRuntime.OpenCodeAdapter, core.LifecycleResult) {
	t.Helper()
	adapter := engineRuntime.NewOpenCodeAdapter(root, options)
	return adapter, adapter.Install()
}

func TestOpenCodeInstallSplicesOnlyTheOwnedEntriesIntoTheOwnersBytes(t *testing.T) {
	root := t.TempDir()
	original := "{\n    \"share\": \"manual\",\n    \"permission\": {\"edit\": \"ask\"},\n    \"mcp\": {\n        \"servers\": {\n            \"mine\": {\"type\": \"remote\", \"url\": \"https://x\"}\n        }\n    },\n    \"$schema\": \"https://opencode.ai/config.json\"\n}\n"
	writeOpenCodeJSON(t, root, original)
	adapter, result := installIn(t, root, engineRuntime.OpenCodeOptions{})
	if result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}

	installed := readText(t, openCodeJSONPath(root))
	for _, kept := range []string{
		"{\n    \"share\": \"manual\",\n    \"permission\": {\"edit\": \"ask\"},\n    \"mcp\": {\n        \"servers\": {\n            \"mine\": {\"type\": \"remote\", \"url\": \"https://x\"}",
		"\n    \"$schema\": \"https://opencode.ai/config.json\",\n",
	} {
		if !strings.Contains(installed, kept) {
			t.Errorf("the owner's bytes changed; %q is gone from:\n%s", kept, installed)
		}
	}
	if !strings.Contains(installed, "\n            \"memorycard\": {") || !strings.Contains(installed, "\n    \"default_agent\": \"foreman\"") {
		t.Errorf("the entries were not written in the file's own indentation:\n%s", installed)
	}

	adapter.Uninstall()
	if got := readText(t, openCodeJSONPath(root)); got != original {
		t.Errorf("uninstall did not give the owner's bytes back:\n%q\nwant\n%q", got, original)
	}
}

func TestOpenCodeInstallKeepsTheBytesAroundAContainerItHasToCreate(t *testing.T) {
	root := t.TempDir()
	original := "{\"zeta\": 1, \"alpha\": [3,2,1]}"
	writeOpenCodeJSON(t, root, original)
	adapter, _ := installIn(t, root, engineRuntime.OpenCodeOptions{})

	installed := readText(t, openCodeJSONPath(root))
	if !strings.HasPrefix(installed, "{\"zeta\": 1, \"alpha\": [3,2,1]") {
		t.Errorf("the owner's bytes moved:\n%s", installed)
	}
	adapter.Uninstall()
	got := readText(t, openCodeJSONPath(root))
	if !strings.HasPrefix(got, "{\"zeta\": 1, \"alpha\": [3,2,1]") || strings.Contains(got, "foreman") || strings.Contains(got, "memorycard") {
		t.Errorf("uninstall left our entries or moved the owner's bytes:\n%s", got)
	}
}

func TestOpenCodeInstallRefusesDuplicateKeysAndTrailingContent(t *testing.T) {
	for name, text := range map[string]string{
		"duplicate root key":    `{"default_agent":"build","default_agent":"foreman"}`,
		"duplicate mcp key":     `{"mcp":{"servers":{}},"mcp":{}}`,
		"duplicate server key":  `{"mcp":{"servers":{"memorycard":{},"memorycard":{}}}}`,
		"text after the object": `{"share":"auto"} garbage`,
		"a stray brace":         `{} }`,
		"a second object":       `{} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeOpenCodeJSON(t, root, text)
			_, result := installIn(t, root, engineRuntime.OpenCodeOptions{})

			if result.Status != core.CapabilityPartial || !strings.Contains(result.Message, "opencode.json") {
				t.Fatalf("Install() = %#v, want partial naming opencode.json", result)
			}
			if got := readText(t, openCodeJSONPath(root)); got != text {
				t.Errorf("the refused file changed to %q", got)
			}
			if _, err := os.Stat(openCodeJSONPath(root) + ".labdrian.bak"); !os.IsNotExist(err) {
				t.Errorf("a backup was written for a refused file, stat err: %v", err)
			}
		})
	}
}
