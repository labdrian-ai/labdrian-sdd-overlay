package runtime_test

import (
	"path/filepath"
	"strings"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// A result of the OpenCode install or uninstall says what each of its parts did: one part refused
// does not hide the others.

func TestOpenCodeRefusedConfigDoesNotStopAGENTSNorTheSkillsReport(t *testing.T) {
	root, agentsSkills := t.TempDir(), t.TempDir()
	overlay := overlayWith(t, fakeRules, fakeStyle, "role-chain")
	writeText(t, filepath.Join(agentsSkills, "role-chain", "SKILL.md"), "copy")
	writeOpenCodeJSON(t, root, `{"default_agent": foreman`)

	_, result := installIn(t, root, engineRuntime.OpenCodeOptions{OverlayDir: overlay, AgentsSkillsDir: agentsSkills})

	if result.Status != core.CapabilityPartial || !strings.Contains(result.Message, "opencode.json") {
		t.Fatalf("Install() = %#v, want partial naming the opencode.json reason", result)
	}
	if !strings.Contains(readText(t, agentsPath(root)), "labdrian:opencode-instructions") {
		t.Errorf("AGENTS.md was not written because opencode.json was refused")
	}
	if !strings.Contains(result.Message, "role-chain") {
		t.Errorf("message = %q, want the shadowing skills report too", result.Message)
	}
}

func TestOpenCodeRefusedAGENTSDoesNotStopTheConfigAndNamesItsReason(t *testing.T) {
	root := t.TempDir()
	writeText(t, agentsPath(root), "x\n<!-- labdrian:opencode-instructions -->\nno close\n")

	_, result := installIn(t, root, engineRuntime.OpenCodeOptions{OverlayDir: overlayWith(t, fakeRules, fakeStyle)})

	if result.Status != core.CapabilityPartial || !strings.Contains(result.Message, "AGENTS.md") {
		t.Fatalf("Install() = %#v, want partial naming AGENTS.md", result)
	}
	if readOpenCodeJSON(t, root)["default_agent"] != "foreman" {
		t.Errorf("opencode.json was not written because AGENTS.md was refused")
	}
}

func TestOpenCodeUninstallNamesBothCleanupsThatFailed(t *testing.T) {
	root := t.TempDir()
	adapter, _ := installIn(t, root, engineRuntime.OpenCodeOptions{OverlayDir: overlayWith(t, fakeRules, fakeStyle)})
	writeOpenCodeJSON(t, root, `{broken`)
	writeText(t, agentsPath(root), "<!-- labdrian:opencode-instructions -->\nno close\n")

	result := adapter.Uninstall()

	if result.Status != core.CapabilityPartial || !strings.Contains(result.Message, "opencode.json") || !strings.Contains(result.Message, "AGENTS.md") {
		t.Errorf("Uninstall() = %#v, want one partial message naming both opencode.json and AGENTS.md", result)
	}
}
