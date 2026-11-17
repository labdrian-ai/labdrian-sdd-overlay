package runtime_test

import (
	"strings"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// The members the adapter inserts take the style of the file they go into: its line ending and the
// indentation of the members beside them. Removal leaves no blank line.

func TestOpenCodeKeepsACRLFFileCRLF(t *testing.T) {
	root := t.TempDir()
	original := "{\r\n  \"share\": \"manual\"\r\n}\r\n"
	writeOpenCodeJSON(t, root, original)
	adapter, _ := installIn(t, root, engineRuntime.OpenCodeOptions{})

	installed := readText(t, openCodeJSONPath(root))

	if lines, crlf := strings.Count(installed, "\n"), strings.Count(installed, "\r\n"); lines != crlf {
		t.Errorf("%d line ends, %d of them CRLF; the inserted lines must use the file's:\n%q", lines, crlf, installed)
	}
	adapter.Uninstall()
	if got := readText(t, openCodeJSONPath(root)); got != original {
		t.Errorf("uninstall = %q, want the original %q", got, original)
	}
}

func TestOpenCodeKeepsATabIndentedFileTabIndented(t *testing.T) {
	root := t.TempDir()
	original := "{\n\t\"share\": \"manual\",\n\t\"mcp\": {\n\t\t\"servers\": {\n\t\t\t\"mine\": {\"type\": \"remote\"}\n\t\t}\n\t}\n}\n"
	writeOpenCodeJSON(t, root, original)
	adapter, _ := installIn(t, root, engineRuntime.OpenCodeOptions{})

	installed := readText(t, openCodeJSONPath(root))

	for _, line := range strings.Split(installed, "\n") {
		if strings.HasPrefix(line, " ") {
			t.Errorf("a line is indented with spaces in a tab-indented file: %q\n%s", line, installed)
		}
	}
	if !strings.Contains(installed, "\n\t\t\t\"memorycard\": {\n\t\t\t\t\"codemode\"") || !strings.Contains(installed, ",\n\t\"default_agent\": \"foreman\"") {
		t.Errorf("the entries are not indented as their siblings are:\n%q", installed)
	}
	adapter.Uninstall()
	got := readText(t, openCodeJSONPath(root))
	if got != original || strings.Contains(got, "\n\n") {
		t.Errorf("uninstall = %q, want the original with no blank line", got)
	}
}

func TestOpenCodeRefusesWhatIsNotJSONWhitespaceAfterTheObject(t *testing.T) {
	for name, tail := range map[string]string{
		"a form feed":      "\f",
		"a vertical tab":   "\v",
		"a no-break space": "\xc2\xa0",
		"a next line":      "\xc2\x85",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			text := "{\"share\":\"auto\"}" + tail
			writeOpenCodeJSON(t, root, text)

			_, result := installIn(t, root, engineRuntime.OpenCodeOptions{})

			if result.Status != core.CapabilityPartial || !strings.Contains(result.Message, "opencode.json") {
				t.Errorf("Install() = %#v, want partial naming opencode.json", result)
			}
			if got := readText(t, openCodeJSONPath(root)); got != text {
				t.Errorf("the refused file changed to %q", got)
			}
		})
	}
	root := t.TempDir()
	writeOpenCodeJSON(t, root, "{}\r\n\t \n")
	if _, result := installIn(t, root, engineRuntime.OpenCodeOptions{}); result.Status != core.CapabilityRestartRequired {
		t.Errorf("JSON whitespace after the object is fine: Install() = %#v", result)
	}
}

func TestOpenCodeUpgradesOurBareEntryOnceTheProgramIsFound(t *testing.T) {
	root := t.TempDir()
	owner := "{\n  \"share\": \"manual\",\n  \"mcp\": {\n    \"servers\": {\n      \"mine\": {\"type\": \"remote\"}\n    }\n  }\n}\n"
	writeOpenCodeJSON(t, root, owner)
	installIn(t, root, engineRuntime.OpenCodeOptions{})
	found := fakeLookup(map[string]string{"memorycard": "/opt/bin/memorycard", "codegraph": "/opt/bin/codegraph"})
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{PathLookup: found})

	result := adapter.Install()

	servers := openCodeServers(t, readOpenCodeJSON(t, root))
	if got := jsonText(t, servers["memorycard"]); !strings.Contains(got, `"/opt/bin/memorycard","mcp"`) {
		t.Errorf("memorycard = %s, want the absolute path", got)
	}
	if got := jsonText(t, servers["codegraph"]); !strings.Contains(got, `"/opt/bin/codegraph","serve","--mcp"`) {
		t.Errorf("codegraph = %s, want the absolute path", got)
	}
	if !strings.Contains(result.Message, "memorycard upgraded to /opt/bin/memorycard") || strings.Contains(result.Message, "opencode.json already current") {
		t.Errorf("message = %q, want the upgrade said", result.Message)
	}
	if !strings.Contains(readText(t, openCodeJSONPath(root)), "\"mine\": {\"type\": \"remote\"}") {
		t.Errorf("a foreign server moved or changed")
	}
	if second := adapter.Install(); !strings.Contains(second.Message, "opencode.json already current") || strings.Contains(second.Message, "upgraded") {
		t.Errorf("a second install = %q, want already current", second.Message)
	}
	adapter.Uninstall()
	if got := readText(t, openCodeJSONPath(root)); got != owner {
		t.Errorf("uninstall = %q, want the owner's bytes %q", got, owner)
	}
}

func TestOpenCodeLeavesAnAbsoluteEntryOfOursAlone(t *testing.T) {
	root := t.TempDir()
	writeOpenCodeJSON(t, root, `{"mcp":{"servers":{"memorycard":{"type":"local","command":["/x/y/memorycard","mcp"],"codemode":true}}}}`)

	_, result := installIn(t, root, engineRuntime.OpenCodeOptions{PathLookup: fakeLookup(map[string]string{"memorycard": "/opt/bin/memorycard"})})

	if got := jsonText(t, openCodeServers(t, readOpenCodeJSON(t, root))["memorycard"]); !strings.Contains(got, `"/x/y/memorycard"`) || strings.Contains(result.Message, "memorycard upgraded") {
		t.Errorf("an absolute entry of ours was changed: %s, %q", got, result.Message)
	}
}
