package installer_test

// The routing pins: what bin/labdrian-overlay does with the rows of overlay.manifest, command by
// command, recorded from the script as it was before its routing moved into the engine
// (testdata/routing_pins.golden; see routing_pins_golden_test.go). This file pins the commands that
// deploy and report: status and sync-check (what they say of each row), apply (what it copies,
// backs up and records) and restore. routing_pins_capture_test.go pins the commands that read the
// deployed files back, and routing_pins_manifest_test.go the manifests that are not ordinary.
//
// The manifest below has a row of every kind the script tells apart: a bare two-column row, an
// explicit skill, an agent, an opencode-agent, the rows that are never deployed (an engine file, a
// root file, an opencode-agent row outside opencode/agents/), a route the script does not know, a
// row with a path and no tag, a path named twice, and a row whose source is not in the overlay.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// pinManifest is the manifest of the full world.
const pinManifest = `# overlay manifest used to pin the routing
   # an indented comment

skill-a/SKILL.md     managed
skill-a/refs/notes.md   managed
skill-b/SKILL.md   custom
shared/helper.md   managed   skill
GADU.md   managed   agent
claude-only.md   custom   agent
opencode/agents/GADU.md   managed   opencode-agent
other/stray.md   custom   opencode-agent
engine/go.mod   managed
skills.registry.yaml   custom
weird/SKILL.md   managed   unknown-route
bare/SKILL.md
dup/SKILL.md   managed
dup/SKILL.md   custom   agent
absent/SKILL.md   managed
late/SKILL.md   custom
`

// pinFiles are the sources of the full world. All of them are on 'upstream' and on 'main'; the
// customization that adds late/SKILL.md is on 'main' alone (fullPinWorld).
var pinFiles = map[string]string{
	"skills/skill-a/SKILL.md":      "# skill-a\n",
	"skills/skill-a/refs/notes.md": "# skill-a notes\n",
	"skills/skill-b/SKILL.md":      "# skill-b\n",
	"skills/shared/helper.md":      "# helper\n",
	"skills/weird/SKILL.md":        "# weird\n",
	"skills/bare/SKILL.md":         "# bare\n",
	"skills/dup/SKILL.md":          "# dup\n",
	"agents/GADU.md":               "# GADU claude\n",
	"agents/claude-only.md":        "# claude only\n",
	"opencode/agents/GADU.md":      "# GADU opencode\n",
	"engine/go.mod":                "module engine\n",
	"skills.registry.yaml":         "skills: []\n",
	"other/stray.md":               "# stray\n",
	"weird/SKILL.md":               "# weird at its repository path\n",
}

// pinCopyTargets are the targets that copy files, in the order `--target all` expands to.
var pinCopyTargets = []string{"claude", "opencode", "codex"}

// fullPinWorld is the world with the full manifest, a customization only 'main' has, and nothing
// deployed.
func fullPinWorld(t *testing.T) *pinWorld {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	w := newPinWorld(t, pinManifest, pinFiles)
	w.commitOnMain("main: a customization upstream does not have", map[string]string{"skills/late/SKILL.md": "# late\n"})
	return w
}

// deployed is the full world after apply has run for every copy target.
func deployed(t *testing.T) *pinWorld {
	t.Helper()
	w := fullPinWorld(t)
	for _, target := range pinCopyTargets {
		if run := w.run("apply", "--target", target); run.Exit != 0 {
			t.Fatalf("apply --target %s: %s", target, pinRunText(run))
		}
	}
	return w
}

func TestRoutingPins_StatusBeforeAnythingIsDeployed(t *testing.T) {
	w := fullPinWorld(t)
	for _, target := range pinCopyTargets {
		pin(t, "status-before/"+target, pinRunText(w.run("status", "--target", target)))
	}
}

func TestRoutingPins_ApplyCopiesEachRowByItsRoute(t *testing.T) {
	w := fullPinWorld(t)
	for _, target := range pinCopyTargets {
		pin(t, "apply/"+target, pinRunText(w.run("apply", "--target", target)))
	}
	pin(t, "apply/files", w.tree(w.home))
}

func TestRoutingPins_StatusAndSyncCheckOfADeployedOverlay(t *testing.T) {
	w := deployed(t)
	for _, target := range pinCopyTargets {
		pin(t, "status-synced/"+target, pinRunText(w.run("status", "--target", target)))
		pin(t, "sync-check-synced/"+target, pinRunText(w.run("sync-check", "--target", target)))
	}
}

func TestRoutingPins_StatusAndSyncCheckTellEachKindOfDrift(t *testing.T) {
	w := deployed(t)
	// A customization on main that the live copy does not have yet: upstream and live agree.
	w.commitOnMain("main: customize skill-a", map[string]string{"skills/skill-a/SKILL.md": "# skill-a customized\n"})
	// A live copy that is neither main's nor upstream's.
	overlaytest.WriteFile(w.t, w.home, ".claude/skills/shared/helper.md", "# helper edited in place\n")
	// A live copy that is gone.
	if err := os.Remove(filepath.Join(w.home, ".claude", "agents", "claude-only.md")); err != nil {
		t.Fatal(err)
	}
	// A file only main has, edited in place.
	overlaytest.WriteFile(w.t, w.home, ".claude/skills/late/SKILL.md", "# late edited in place\n")
	overlaytest.WriteFile(w.t, w.home, ".config/opencode/agents/GADU.md", "# GADU opencode edited in place\n")
	for _, target := range pinCopyTargets {
		pin(t, "status-drift/"+target, pinRunText(w.run("status", "--target", target)))
		pin(t, "sync-check-drift/"+target, pinRunText(w.run("sync-check", "--target", target)))
	}
}

func TestRoutingPins_ApplyBacksUpOnlyWhatItOverwritesAndRestoreBringsItBack(t *testing.T) {
	w := deployed(t)
	w.commitOnMain("main: change three sources", map[string]string{
		"skills/skill-a/SKILL.md": "# skill-a v2\n",
		"agents/GADU.md":          "# GADU claude v2\n",
		"skills/weird/SKILL.md":   "# weird v2\n",
	})
	pin(t, "apply-again/claude", pinRunText(w.run("apply", "--target", "claude")))
	pin(t, "apply-again/state", w.tree(filepath.Join(w.home, ".labdrian-overlay")))
	pin(t, "restore/list", pinRunText(w.run("restore", "--target", "claude", "--list")))
	pin(t, "restore/run", pinRunText(w.run("restore", "--target", "claude")))
	pin(t, "restore/files", w.tree(filepath.Join(w.home, ".claude")))
	pin(t, "restore/state", w.tree(filepath.Join(w.home, ".labdrian-overlay")))
}
