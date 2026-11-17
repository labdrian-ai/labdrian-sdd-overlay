package runtime_test

import (
	"bytes"
	"context"
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

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "--standalone", "--print-logs", "--log-level", "debug", "debug", "agents")
	command.Dir = sandbox
	command.Env = append(os.Environ(),
		"HOME="+sandbox,
		"XDG_CONFIG_HOME="+configHome,
		"XDG_DATA_HOME="+filepath.Join(sandbox, "data"),
		"XDG_STATE_HOME="+filepath.Join(sandbox, "state"),
		"XDG_CACHE_HOME="+filepath.Join(sandbox, "cache"),
	)
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Run(); err != nil {
		t.Fatalf("opencode debug agents: %v\n%s", err, output.String())
	}

	for _, line := range strings.Split(output.String(), "\n") {
		if strings.Contains(line, "failed to load plugin") && strings.Contains(line, "labdrian-runtime-parity") {
			t.Errorf("OpenCode could not load the installed plugin: %s", line)
		}
	}
	marker := filepath.Join(root, "labdrian-runtime-parity.active.json")
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("the plugin did not write its active marker %s: %v", marker, err)
	}
}
