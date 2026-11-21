package runtime_test

import (
	"path/filepath"
	"strings"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// The style goes into AGENTS.md without its frontmatter, whatever shape the frontmatter has.
func TestOpenCodeDropsTheStyleFrontmatterInEveryShape(t *testing.T) {
	for name, style := range map[string]string{
		"empty frontmatter":            "---\n---\n# Style body\n- Lead with the answer.\n",
		"closing line at end of file":  "---\nname: labdrian\n---",
		"empty frontmatter at the end": "---\n---",
		"no frontmatter":               "# Style body\n- Lead with the answer.\n",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{OverlayDir: overlayWith(t, fakeRules, style)})

			adapter.Install()

			text := readText(t, agentsPath(root))
			if strings.Contains(text, "name: labdrian") {
				t.Errorf("the frontmatter is in AGENTS.md:\n%s", text)
			}
			if strings.Contains(style, "# Style body") && !strings.Contains(text, "# Style body\n- Lead with the answer.") {
				t.Errorf("the body of the style is missing:\n%s", text)
			}
			if lines := strings.Split(text, "\n"); containsLine(lines, "---") {
				t.Errorf("a frontmatter delimiter line is left in AGENTS.md:\n%s", text)
			}
		})
	}
}

func containsLine(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}

// The note about the labdrian skills found where OpenCode reads skills first is on every status
// result, whatever the status says, because a shadowing copy is a fact about the machine.
func TestOpenCodeStatusReportsShadowingSkillsWhateverItsOutcome(t *testing.T) {
	root, agentsSkills := t.TempDir(), t.TempDir()
	overlay := overlayWith(t, fakeRules, fakeStyle, "role-chain")
	writeText(t, filepath.Join(agentsSkills, "role-chain", "SKILL.md"), "copy")
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{OverlayDir: overlay, AgentsSkillsDir: agentsSkills})

	notInstalled := adapter.Status()
	if notInstalled.Status != core.CapabilityUnsupported || !strings.Contains(notInstalled.Message, agentsSkills+": role-chain") {
		t.Errorf("not installed: Status() = %#v, want the shadowing note", notInstalled)
	}

	adapter.Install()
	restart := adapter.Status()
	if restart.Status != core.CapabilityRestartRequired || !strings.Contains(restart.Message, agentsSkills+": role-chain") {
		t.Errorf("not yet active: Status() = %#v, want the shadowing note", restart)
	}

	writeOpenCodeJSON(t, root, `{broken`)
	partial := adapter.Status()
	if partial.Status != core.CapabilityPartial || !strings.Contains(partial.Message, agentsSkills+": role-chain") {
		t.Errorf("config refused: Status() = %#v, want the shadowing note", partial)
	}
	if sync := adapter.SyncCheck(); !strings.Contains(sync.Message, agentsSkills+": role-chain") {
		t.Errorf("SyncCheck() = %#v, want the shadowing note", sync)
	}
}
