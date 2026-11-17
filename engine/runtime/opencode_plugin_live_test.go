package runtime_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// The plugin is written against modules the tests stub (stubPluginPackage), so a stub that does not
// match what OpenCode ships hides a plugin that never loads: this is how the 2.2.7 import
// `{ Tool } from "@opencode/plugin/promise/tool"` (the module exports only CallID and Error) went
// unseen. This test loads the installed plugin into the real OpenCode, when there is one.
func TestOpenCodeLoadsTheInstalledPluginAndItWritesItsActiveMarker(t *testing.T) {
	binary, err := exec.LookPath("opencode")
	if err != nil {
		t.Skipf("no opencode binary on PATH, so the installed plugin cannot be loaded by a real OpenCode: %v", err)
	}

	sandbox := t.TempDir()
	configHome := filepath.Join(sandbox, "config")
	root := filepath.Join(configHome, "opencode")
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{OverlayDir: shippedOverlay(t)})
	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}

	agents := runOpenCodeIn(t, binary, sandbox, configHome, "--print-logs", "--log-level", "debug", "debug", "agents")
	output := agents.stdout + agents.stderr

	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "failed to load plugin") && strings.Contains(line, "labdrian-runtime-parity") {
			t.Errorf("OpenCode could not load the installed plugin: %s", line)
		}
	}
	marker := filepath.Join(root, "labdrian-runtime-parity.active.json")
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("the plugin did not write its active marker %s: %v", marker, err)
	}

	config := runOpenCodeIn(t, binary, sandbox, configHome, "debug", "config")
	assertOpenCodeParsesTheInstalledConfig(t, config.stdout, config.stderr, filepath.Join(root, "opencode.json"))
}

// openCodeRun is what a run of the real opencode wrote on each stream.
type openCodeRun struct{ stdout, stderr string }

// runOpenCodeIn runs the real opencode in the sandbox (its HOME and every XDG directory inside it) with
// --standalone and the arguments, and returns what it wrote on both streams.
func runOpenCodeIn(t *testing.T, binary, sandbox, configHome string, args ...string) openCodeRun {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, binary, append([]string{"--standalone"}, args...)...)
	command.Dir = sandbox
	command.Env = append(os.Environ(),
		"HOME="+sandbox,
		"XDG_CONFIG_HOME="+configHome,
		"XDG_DATA_HOME="+filepath.Join(sandbox, "data"),
		"XDG_STATE_HOME="+filepath.Join(sandbox, "state"),
		"XDG_CACHE_HOME="+filepath.Join(sandbox, "cache"),
	)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("opencode %s: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
	return openCodeRun{stdout: stdout.String(), stderr: stderr.String()}
}

// assertOpenCodeParsesTheInstalledConfig reads `opencode debug config`, the sources OpenCode parsed with
// what it kept of each, and requires that it kept our keys of the installed opencode.json: the default
// agent, and both MCP servers as local servers with Code Mode on. OpenCode drops what it does not
// understand without saying so, so the file on disk proves nothing about what it reads.
func assertOpenCodeParsesTheInstalledConfig(t *testing.T, output, stderr, configPath string) {
	t.Helper()
	var sources []struct {
		Type string `json:"type"`
		Path string `json:"path"`
		Info struct {
			DefaultAgent string `json:"default_agent"`
			MCP          struct {
				Servers map[string]struct {
					Type     string   `json:"type"`
					Command  []string `json:"command"`
					Codemode bool     `json:"codemode"`
				} `json:"servers"`
			} `json:"mcp"`
		} `json:"info"`
	}
	if err := json.Unmarshal([]byte(output), &sources); err != nil {
		t.Fatalf("opencode debug config is not a JSON list of sources on stdout: %v\nstdout:\n%s\nstderr:\n%s", err, output, stderr)
	}
	for _, source := range sources {
		if source.Type != "document" || !samePath(source.Path, configPath) {
			continue
		}
		if source.Info.DefaultAgent != "foreman" {
			t.Errorf("OpenCode kept default_agent %q, want foreman", source.Info.DefaultAgent)
		}
		for name, program := range map[string]string{"memorycard": "memorycard", "codegraph": "codegraph"} {
			server, ok := source.Info.MCP.Servers[name]
			if !ok || server.Type != "local" || !server.Codemode || len(server.Command) == 0 || filepath.Base(server.Command[0]) != program {
				t.Errorf("OpenCode kept mcp.servers.%s as %#v, want a local server of %s with codemode true", name, server, program)
			}
		}
		return
	}
	t.Errorf("opencode debug config does not list %s among its sources:\nstdout:\n%s\nstderr:\n%s", configPath, output, stderr)
}

// samePath says whether two paths name the same file once symlinks are resolved: OpenCode reports the
// real path of a sandbox that sits under a symlinked temporary directory.
func samePath(a, b string) bool {
	resolve := func(path string) string {
		if real, err := filepath.EvalSymlinks(path); err == nil {
			return real
		}
		return path
	}
	return resolve(a) == resolve(b)
}
