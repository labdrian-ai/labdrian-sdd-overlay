package installer_test

// The routing pins of manifests that are not ordinary: one written the way a hand edit leaves it,
// one that holds a longterm-mem row the script refuses, and one with the mcp rows the longterm-mem
// install is dispatched from. What the script reads from them, and what it does when it cannot
// read a row, are as much what a pin holds as the ordinary manifest (routing_pins_test.go has the
// world).

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
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

// The quirks the pins keep, and why they are kept and not fixed:
//
//   - A line that ends in a carriage return (a manifest with Windows line ends) keeps it on its
//     last column, so "crlf/SKILL.md managed\r" is a row tagged "managed\r", which is not
//     "managed". Apply, status and sync-check still handle the row (they read every row and need
//     only its path), but capture, restore, the backup and the digest read the managed rows, and
//     the row is not one of them: capture SKIPS it SILENTLY, with no line for it and no warning.
//     That is what the script did (awk's $2 == "managed" is false for "managed\r"), the engine
//     keeps it on purpose (skills.ReadManifestRows), and fixing it changes what a command prints
//     and copies for such a manifest, which is a decision of the owner, not of this move. The
//     live file the test writes for the row is there to show that capture leaves it alone.
//   - A comment that starts after a column is a column ("noted/SKILL.md managed # not a column"
//     is a row with the route "#", which no one knows, so the skill route), and a row with no
//     tag, or a last line with no line end, is read as any other.
func TestRoutingPins_ManifestReadingKeepsItsQuirks(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	w := newPinWorld(t, pinQuirkyManifest, pinQuirkyFiles)
	pin(t, "quirks/status", pinRunText(w.run("status", "--target", "claude")))
	pin(t, "quirks/apply", pinRunText(w.run("apply", "--target", "claude")))
	pin(t, "quirks/files", w.tree(w.home))
	overlaytest.WriteFile(w.t, w.home, ".claude/skills/tabbed/SKILL.md", "# tabbed live\n")
	// The row of this file is tagged "managed\r": capture does not copy it (see above).
	overlaytest.WriteFile(w.t, w.home, ".claude/skills/crlf/SKILL.md", "# crlf live\n")
	pin(t, "quirks/capture", pinRunText(w.run("capture", "--target", "claude")))
	pin(t, "quirks/capture-upstream", w.branchFiles("upstream"))
}

// Two manifests with a longterm-mem row the script cannot place (no route, an unknown one). The
// managed row is asked for by every command; the custom row only by the commands that read every
// row (status, sync-check, apply), because capture, restore, the backup and the digest read the
// managed rows alone and the engine places, and refuses, only the rows it is asked for.
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

// The record of the commands that stop at the row (refused-*) is the one part of the pins that
// changed when the routing moved into the engine. The script used to learn the route of a row as
// it reached it, so a command had acted on every row before the refused one when it stopped:
// apply had copied them, status and sync-check had said them, restore had put them back. The
// engine answers for the whole manifest or not at all, so now the command stops before it acts on
// any row (the owner's decision of 2026-10-15). Everything else in the record is as it was.
//
// Capture of the custom manifest still succeeds, as it did, and that is no asymmetry of this
// change: capture reads the managed rows alone, and the row that cannot be placed is a custom
// one, so capture never asks for it (before and after, exit 0 and the files of the managed rows
// copied). What differs from the old record in its output is a consequence of the apply before
// it: apply no longer deploys GADU.md before it refuses, so the live GADU.md that capture looks
// for is not there and capture says so. The managed manifest shows capture refusing, as status
// and apply do. Whether the commands that read the managed rows should also refuse a manifest
// whose custom row cannot be placed is a rule the script never had, and is listed in the ledger
// as a decision for the owner.
func TestRoutingPins_AnUnroutedLongtermMemRowIsRefusedWhereItIsReached(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	for _, tc := range []struct{ name, manifest string }{
		{"custom", pinUnroutedCustomManifest},
		{"managed", pinUnroutedManagedManifest},
	} {
		name := tc.name
		w := newPinWorld(t, tc.manifest, pinUnroutedFiles)
		pin(t, "refused-"+name+"/status", pinRunText(w.run("status", "--target", "claude")))
		if err := os.MkdirAll(filepath.Join(w.home, ".claude", "skills"), 0o755); err != nil {
			t.Fatal(err)
		}
		pin(t, "refused-"+name+"/sync-check", pinRunText(w.run("sync-check", "--target", "claude")))
		pin(t, "refused-"+name+"/apply", pinRunText(w.run("apply", "--target", "claude")))
		pin(t, "refused-"+name+"/files", w.tree(w.home))
		overlaytest.WriteFile(w.t, w.home, ".claude/skills/skill-a/SKILL.md", "# skill-a live\n")
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
	overlaytest.WriteFile(w.t, w.overlay, "overlay.manifest", pinManifest+"longterm-mem/internal/foo.go   managed\n")
	overlaytest.WriteFile(w.t, w.home, ".claude/skills/skill-a/SKILL.md", "# skill-a edited\n")
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
	overlaytest.WriteFile(w.t, w.home, ".claude/skills/skill-a/SKILL.md", "# skill-a live\n")
	pin(t, "mcp/capture", pinRunText(w.run("capture", "--target", "claude")))
	pin(t, "mcp/capture-upstream", w.branchFiles("upstream"))
}

// pinStdoutText is a run as a pin records it when its words on stderr are the words of whatever
// reads the manifest: how it ended and what it said on stdout.
func pinStdoutText(r pinRun) string {
	return fmt.Sprintf("exit %d\n--- stdout\n%s", r.Exit, pinText(r.Stdout))
}

// A manifest that is not there is read as one with no row: the commands act on nothing, end as
// they would for an empty manifest, and say on stderr that the manifest is missing. The pins hold
// what they print on stdout and how they end; the words on stderr were grep's, and are now the
// engine's, so they are only held to name the manifest.
func TestRoutingPins_AManifestThatIsNotThereHasNoRow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	w := newPinWorld(t, "skill-a/SKILL.md   managed\n", map[string]string{"skills/skill-a/SKILL.md": "# skill-a\n"})
	// Capture reads the manifest of 'upstream', which it checks out, so the manifest goes from both.
	w.git("checkout", "-q", "upstream")
	w.git("rm", "-q", "overlay.manifest")
	w.git("commit", "-q", "-m", "upstream: no manifest")
	w.git("checkout", "-q", "main")
	w.git("merge", "-q", "--ff-only", "upstream")
	if err := os.MkdirAll(filepath.Join(w.home, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"status", "--target", "claude"},
		{"sync-check", "--target", "claude"},
		{"apply", "--target", "claude"},
		{"capture", "--target", "claude"},
	} {
		run := w.run(args...)
		pin(t, "no-manifest/"+args[0], pinStdoutText(run))
		if !strings.Contains(run.Stderr, "overlay.manifest") {
			t.Errorf("%s said nothing of the missing manifest on stderr: %q", args[0], run.Stderr)
		}
	}
	pin(t, "no-manifest/files", w.tree(w.home))
}
