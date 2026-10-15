package installer_test

// The routing pins of manifests that are not ordinary: one written the way a hand edit leaves it,
// one that holds a longterm-mem row the script refuses, and one with the mcp rows the longterm-mem
// install is dispatched from. What the script reads from them, and what it does when it cannot
// read a row, are as much what a pin holds as the ordinary manifest (routing_pins_test.go has the
// world).

import (
	"os"
	"path/filepath"
	"testing"
)

// pinQuirkyManifest is a manifest written the way a hand edit leaves one: tabs between the
// columns, blanks before and after a row, a comment that starts after a tab, lines that end in
// a carriage return, a trailing word that is not a column, and a last line with no line end.
const pinQuirkyManifest = "\t# a comment after a tab\n" +
	"tabbed/SKILL.md\tmanaged\n" +
	"padded/SKILL.md managed   \n" +
	"   indented/SKILL.md\tmanaged\tagent\n" +
	"crlf/SKILL.md managed\r\n" +
	"crlf-agent.md custom agent\r\n" +
	"\r\n" +
	"   \t \n" +
	"noted/SKILL.md managed # not a column\n" +
	"last/SKILL.md managed"

var pinQuirkyFiles = map[string]string{
	"skills/tabbed/SKILL.md":   "# tabbed\n",
	"skills/padded/SKILL.md":   "# padded\n",
	"agents/indented/SKILL.md": "# indented agent\n",
	"skills/crlf/SKILL.md":     "# crlf\n",
	"agents/crlf-agent.md":     "# crlf agent\n",
	"skills/noted/SKILL.md":    "# noted\n",
	"skills/last/SKILL.md":     "# last\n",
}

func TestRoutingPins_ManifestReadingKeepsItsQuirks(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	w := newPinWorld(t, pinQuirkyManifest, pinQuirkyFiles)
	pin(t, "quirks/status", pinRunText(w.run("status", "--target", "claude")))
	pin(t, "quirks/apply", pinRunText(w.run("apply", "--target", "claude")))
	pin(t, "quirks/files", w.tree(w.home))
	w.write(w.home, ".claude/skills/tabbed/SKILL.md", "# tabbed live\n")
	w.write(w.home, ".claude/skills/crlf/SKILL.md", "# crlf live\n")
	pin(t, "quirks/capture", pinRunText(w.run("capture", "--target", "claude")))
	pin(t, "quirks/capture-upstream", w.branchFiles("upstream"))
}

// An unrouted longterm-mem row is refused wherever a command reaches it. The managed one is
// reached by every command; the custom one only by the commands that read every row.
const pinUnroutedCustomManifest = `skill-a/SKILL.md   managed
GADU.md   managed   agent
longterm-mem/internal/foo.go   custom
skill-b/SKILL.md   custom
`

const pinUnroutedManagedManifest = `skill-a/SKILL.md   managed
longterm-mem/internal/foo.go   managed   bogus
skill-b/SKILL.md   custom
`

var pinUnroutedFiles = map[string]string{
	"skills/skill-a/SKILL.md": "# skill-a\n",
	"skills/skill-b/SKILL.md": "# skill-b\n",
	"agents/GADU.md":          "# GADU claude\n",
}

func TestRoutingPins_AnUnroutedLongtermMemRowIsRefusedWhereItIsReached(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	for name, manifest := range map[string]string{"custom": pinUnroutedCustomManifest, "managed": pinUnroutedManagedManifest} {
		w := newPinWorld(t, manifest, pinUnroutedFiles)
		pin(t, "refused-"+name+"/status", pinRunText(w.run("status", "--target", "claude")))
		if err := os.MkdirAll(filepath.Join(w.home, ".claude", "skills"), 0o755); err != nil {
			t.Fatal(err)
		}
		pin(t, "refused-"+name+"/sync-check", pinRunText(w.run("sync-check", "--target", "claude")))
		pin(t, "refused-"+name+"/apply", pinRunText(w.run("apply", "--target", "claude")))
		pin(t, "refused-"+name+"/files", w.tree(w.home))
		w.write(w.home, ".claude/skills/skill-a/SKILL.md", "# skill-a live\n")
		pin(t, "refused-"+name+"/capture", pinRunText(w.run("capture", "--target", "claude")))
		pin(t, "refused-"+name+"/capture-upstream", w.branchFiles("upstream"))
	}
}

func TestRoutingPins_RestoreStopsAtAnUnroutedLongtermMemRow(t *testing.T) {
	w := deployed(t)
	w.commitOnMain("main: change a source", map[string]string{"skills/skill-a/SKILL.md": "# skill-a v2\n"})
	if run := w.run("apply", "--target", "claude"); run.Exit != 0 {
		t.Fatalf("apply: %s", pinRunText(run))
	}
	w.write(w.overlay, "overlay.manifest", pinManifest+"longterm-mem/internal/foo.go   managed\n")
	w.write(w.home, ".claude/skills/skill-a/SKILL.md", "# skill-a edited\n")
	pin(t, "refused-restore/run", pinRunText(w.run("restore", "--target", "claude")))
	pin(t, "refused-restore/files", w.tree(filepath.Join(w.home, ".claude")))
}

// pinMcpManifest has the sentinel rows of the longterm-mem install: routed mcp, copied nowhere.
const pinMcpManifest = `skill-a/SKILL.md   managed
longterm-mem/go.mod   managed   mcp
longterm-mem/internal/x.go   custom   mcp
`

func TestRoutingPins_AnMcpRowIsReadByEveryCommandAndCopiedNowhere(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	w := newPinWorld(t, pinMcpManifest, map[string]string{
		"skills/skill-a/SKILL.md": "# skill-a\n",
		"longterm-mem/go.mod":     "module longterm-mem\n",
	})
	pin(t, "mcp/status", pinRunText(w.run("status", "--target", "claude")))
	if err := os.MkdirAll(filepath.Join(w.home, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	pin(t, "mcp/sync-check", pinRunText(w.run("sync-check", "--target", "claude")))
	w.write(w.home, ".claude/skills/skill-a/SKILL.md", "# skill-a live\n")
	pin(t, "mcp/capture", pinRunText(w.run("capture", "--target", "claude")))
	pin(t, "mcp/capture-upstream", w.branchFiles("upstream"))
}
