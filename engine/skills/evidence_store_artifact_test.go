package skills

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// The evidence store (S4, T2) is absorbed from the playbook snapshot into skills/_shared/evidence/
// as overlay-owned code. These pins hold its files, its manifest rows, its deployed path and the
// provenance that travels with it.

const evidenceDirRel = "skills/_shared/evidence"

var evidenceStoreNames = []string{"evidence.sh", "evidence-rules.md", "LICENSE", "PROVENANCE.md"}

func TestEvidenceStore_ManifestHoldsFourClaudeSkillRows(t *testing.T) {
	rows := ReadManifestRows([]byte(readRepoFile(t, skillsRepoRoot(t), "overlay.manifest")))
	for _, name := range evidenceStoreNames {
		path := "_shared/evidence/" + name
		var found []ManifestRow
		for _, row := range rows {
			if row.Path == path {
				found = append(found, row)
			}
		}
		if len(found) != 1 || found[0].Tag != "custom" || found[0].Route != "claude-skill" {
			t.Errorf("overlay.manifest rows for %s = %+v, want exactly one, tagged custom and routed claude-skill", path, found)
		}
	}
}

func TestEvidenceStore_DeploysToClaudeAndToNoOtherTarget(t *testing.T) {
	rows := ReadManifestRows([]byte(readRepoFile(t, skillsRepoRoot(t), "overlay.manifest")))
	for _, tc := range []struct {
		target string
		dest   RouteDestinations
		root   string
	}{
		{"claude", RouteDestinations{Target: "claude", SkillsRoot: "/s", AgentsRoot: "/a"}, "/s"},
		{"codex", RouteDestinations{Target: "codex", SkillsRoot: "/c", AgentsRoot: "/ca"}, ""},
		{"opencode", RouteDestinations{Target: "opencode", SkillsRoot: "/o2", AgentsRoot: "/oa"}, ""},
	} {
		routes, err := ResolveRoutes(rows, false, testOverlay, tc.dest)
		if err != nil {
			t.Fatalf("ResolveRoutes for %s: %v", tc.target, err)
		}
		dest := map[string]string{}
		for _, r := range routes {
			dest[r.Path] = r.Dest
		}
		for _, name := range evidenceStoreNames {
			path := "_shared/evidence/" + name
			want := ""
			if tc.root != "" {
				want = tc.root + "/" + path
			}
			if got, ok := dest[path]; !ok || got != want {
				t.Errorf("%s for %s deploys to %q (row present %v), want %q", path, tc.target, got, ok, want)
			}
		}
	}
}

func TestEvidenceStore_LicenseAndProvenanceTravelWithTheCode(t *testing.T) {
	root := skillsRepoRoot(t)
	license := readRepoFile(t, root, evidenceDirRel+"/LICENSE")
	for _, want := range []string{"MIT License", "Copyright (c) 2026 Andres Frei"} {
		if !strings.Contains(license, want) {
			t.Errorf("LICENSE lacks %q", want)
		}
	}
	provenance := readRepoFile(t, root, evidenceDirRel+"/PROVENANCE.md")
	for _, want := range []string{
		"6de8566bf7f1150f49a75c1162c0f4cf87bf57c5",
		"v3.6.0",
		"andresfrei/playbook",
		"plugins/playbook-agents/evidence.sh",
		"plugins/playbook-agents/evidence-rules.md",
		"MIT",
	} {
		if !strings.Contains(provenance, want) {
			t.Errorf("PROVENANCE.md lacks %q", want)
		}
	}
}

func TestEvidenceStore_ScriptParsesAsPosixShell(t *testing.T) {
	overlaytest.RequireShAndAwk(t)
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Fatalf("sh: %v", err)
	}
	script := filepath.Join(skillsRepoRoot(t), evidenceDirRel, "evidence.sh")
	if out, err := exec.Command(sh, "-n", script).CombinedOutput(); err != nil {
		t.Fatalf("sh -n %s: %v\n%s", script, err, out)
	}
}

// The git blob ids of the three files copied from the snapshot 6de8566 (v3.6.0), from
// `git -C ~/.local/share/playbook rev-parse 6de8566:<path>`. The snapshot never changes, so neither do these.
var evidenceSnapshotBlobs = map[string]string{
	"evidence.sh":       "09b44a09878454b0f96f81ca00431236b142fd59",
	"evidence-rules.md": "48c4bf91cda2322dc71f4fb080c9add7f6233031",
	"LICENSE":           "b6f9c476b8c3982bfd1b4368d61c83ea53e22dad",
}

// gitBlobID is the id git gives a file's bytes: SHA-1 of "blob <size>\0<bytes>". skills/** is -text, so the
// bytes on disk are the bytes of the blob.
func gitBlobID(content string) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("blob %d\x00%s", len(content), content)))
	return hex.EncodeToString(sum[:])
}

// changedSinceTheSnapshot is the text under "## Changed since the snapshot" of PROVENANCE.md, up to the next
// heading or the end of the file.
func changedSinceTheSnapshot(t *testing.T, provenance string) string {
	t.Helper()
	_, after, ok := strings.Cut(provenance, "## Changed since the snapshot\n")
	if !ok {
		t.Fatal(`PROVENANCE.md has no "## Changed since the snapshot" section`)
	}
	section, _, _ := strings.Cut(after, "\n## ")
	return strings.TrimSpace(section)
}

func TestEvidenceStore_CopiesMatchTheSnapshotOrAreListedAsChanged(t *testing.T) {
	root := skillsRepoRoot(t)
	section := changedSinceTheSnapshot(t, readRepoFile(t, root, evidenceDirRel+"/PROVENANCE.md"))
	changed := 0
	for name, blob := range evidenceSnapshotBlobs {
		equal := gitBlobID(readRepoFile(t, root, evidenceDirRel+"/"+name)) == blob
		listed := strings.Contains(section, "`"+name+"`")
		switch {
		case equal && listed:
			t.Errorf("%s equals the snapshot but the Changed section names it", name)
		case !equal && !listed:
			t.Errorf("%s differs from the snapshot and the Changed section does not name it", name)
		}
		if !equal {
			changed++
		}
	}
	if changed == 0 && section != "- Nothing." {
		t.Errorf("all three copies equal the snapshot, so the Changed section is exactly %q, got %q", "- Nothing.", section)
	}
}
