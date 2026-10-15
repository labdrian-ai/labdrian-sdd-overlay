package installer_test

// The routing pins of the commands that read files back into the overlay: capture, from the live
// files of a target and from a snapshot, and bootstrap, which extracts a snapshot and layers the
// live files of Claude over it. What each puts on 'upstream' and on 'main' says where the script
// takes each row's file from, and where it puts it (routing_pins_test.go has the world).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/enginetest"
)

// branchFiles lists the files of a branch with their content, as the world's tree does, but for
// the manifest, which no command here changes.
func (w *pinWorld) branchFiles(branch string) string {
	w.t.Helper()
	var lines []string
	for _, name := range strings.Fields(w.git("ls-tree", "-r", "--name-only", branch)) {
		if name == "overlay.manifest" {
			continue
		}
		lines = append(lines, name+": "+quoteBody(w.git("show", branch+":"+name)))
	}
	return pinText(strings.Join(lines, "\n") + "\n")
}

func TestRoutingPins_CaptureCopiesTheLiveFilesOfManagedRows(t *testing.T) {
	w := deployed(t)
	w.write(w.home, ".claude/skills/skill-a/SKILL.md", "# skill-a live\n")
	w.write(w.home, ".claude/skills/shared/helper.md", "# helper live\n")
	w.write(w.home, ".claude/agents/GADU.md", "# GADU claude live\n")
	w.write(w.home, ".claude/skills/weird/SKILL.md", "# weird live\n")
	pin(t, "capture/claude", pinRunText(w.run("capture", "--target", "claude")))
	pin(t, "capture/claude-upstream", w.branchFiles("upstream"))
	w.write(w.home, ".config/opencode/agents/GADU.md", "# GADU opencode live\n")
	pin(t, "capture/opencode", pinRunText(w.run("capture", "--target", "opencode")))
	pin(t, "capture/opencode-upstream", w.branchFiles("upstream"))
}

// writeSnapshot writes, at rel under the home of the world, a snapshot with a file for some of the
// managed rows, each where its route puts it in a backup, and returns its path. It is in the home
// so that the path the script reports is the same on every machine.
func writeSnapshot(t *testing.T, w *pinWorld, rel string) string {
	t.Helper()
	snapshot, err := os.ReadFile(createTarball(t, map[string]string{
		"files/home/labdrian/.claude/skills/skill-a/SKILL.md":      "# skill-a backup\n",
		"files/home/labdrian/.claude/skills/skill-a/refs/notes.md": "# notes backup\n",
		"files/home/labdrian/.claude/skills/shared/helper.md":      "# helper backup\n",
		"files/home/labdrian/.claude/agents/GADU.md":               "# GADU claude backup\n",
		"files/home/labdrian/.config/opencode/agents/GADU.md":      "# GADU opencode backup\n",
		"files/home/labdrian/.claude/weird/SKILL.md":               "# weird backup\n",
		"files/home/labdrian/.claude/skills/dup/SKILL.md":          "# dup backup\n",
	}))
	if err != nil {
		t.Fatal(err)
	}
	w.write(w.home, rel, string(snapshot))
	return filepath.Join(w.home, rel)
}

func TestRoutingPins_CaptureFromABackupExtractsWhereEachRouteKeepsItsFile(t *testing.T) {
	w := fullPinWorld(t)
	pin(t, "capture-from-backup/run", pinRunText(w.run("capture", "--from-backup", writeSnapshot(t, w, "snapshot.tar.gz"))))
	pin(t, "capture-from-backup/upstream", w.branchFiles("upstream"))
}

// newBootstrapWorld is what bootstrap runs over: an overlay directory that is no repository yet,
// with the manifest, the snapshot it extracts and some live files of Claude to layer over it, and
// a home that has the engine.
func newBootstrapWorld(t *testing.T) *pinWorld {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	w := &pinWorld{t: t, script: overlayScript(t), home: t.TempDir(), overlay: t.TempDir()}
	// bootstrap makes the repository itself, with the first branch the configuration names.
	w.env = append(pinEnv(w.home, w.overlay),
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=init.defaultBranch", "GIT_CONFIG_VALUE_0=master")
	enginetest.Provision(t, w.home)
	w.write(w.overlay, "overlay.manifest", pinManifest)
	w.write(w.overlay, "bin/overlay", "#!/bin/sh\n")
	writeSnapshot(t, w, ".gentle-ai/backups/upgrade-20260615T175529Z/snapshot.tar.gz")
	w.write(w.home, ".claude/skills/skill-a/SKILL.md", "# skill-a live\n")
	w.write(w.home, ".claude/skills/bare/SKILL.md", "# bare live\n")
	w.write(w.home, ".claude/agents/GADU.md", "# GADU claude live\n")
	return w
}

func TestRoutingPins_BootstrapExtractsAndLayersTheManagedAndTheTrackedRows(t *testing.T) {
	w := newBootstrapWorld(t)
	pin(t, "bootstrap/run", pinRunText(w.run("bootstrap")))
	pin(t, "bootstrap/upstream", w.branchFiles("upstream"))
	pin(t, "bootstrap/main", w.branchFiles("main"))
}
