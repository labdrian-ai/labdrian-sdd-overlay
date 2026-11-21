package runtime_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// The registered OpenCode adapter finds the MCP servers on the $PATH of its Config, the first
// directory that holds an executable file of the name, and writes the absolute path it found.
func TestRegisteredOpenCodeWritesTheAbsolutePathItFindsOnConfigPath(t *testing.T) {
	root, bin := t.TempDir(), t.TempDir()
	program := filepath.Join(bin, "memorycard")
	if err := os.WriteFile(program, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "codegraph"), []byte("not executable"), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter, err := shippedRegistry(t).New(core.TargetOpenCode, core.Config{ConfigRoot: root, Path: "relative" + string(os.PathListSeparator) + bin})
	if err != nil {
		t.Fatal(err)
	}

	result := adapter.Install()

	servers := openCodeServers(t, readOpenCodeJSON(t, root))
	if got := jsonText(t, servers["memorycard"]); !strings.Contains(got, `"`+program+`","mcp"`) {
		t.Errorf("memorycard = %s, want the path %s", got, program)
	}
	if got := jsonText(t, servers["codegraph"]); !strings.Contains(got, `"codegraph","serve"`) || !strings.Contains(result.Message, "codegraph not found on PATH") {
		t.Errorf("codegraph = %s, message %q: a file that is not executable is not found", got, result.Message)
	}
}

func TestOpenCodeWithoutAPathWritesBareNamesSilently(t *testing.T) {
	root := t.TempDir()
	adapter, err := shippedRegistry(t).New(core.TargetOpenCode, core.Config{ConfigRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if result := adapter.Install(); strings.Contains(result.Message, "not found on PATH") {
		t.Errorf("message = %q, no PATH means no lookup", result.Message)
	}
}
