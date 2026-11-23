package skills

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
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

// The two texts the License line of PROVENANCE.md may have, one for each state of LICENSE.
const (
	licenseLineIdentical = "- License: MIT, in `LICENSE` beside this file (the snapshot `LICENSE`, byte-identical)."
	licenseLineChanged   = "- License: MIT, in `LICENSE` beside this file (changed since the snapshot `LICENSE`; see \"Changed since the snapshot\")."
)

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
	provenance := readRepoFile(t, root, evidenceDirRel+"/PROVENANCE.md")
	section := changedSinceTheSnapshot(t, provenance)
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
	if changed > 0 {
		for _, line := range strings.Split(section, "\n") {
			if strings.TrimSpace(line) == "- Nothing." {
				t.Errorf("%d copies differ from the snapshot, so the Changed section must not say %q", changed, "- Nothing.")
			}
		}
	}

	// The License line is tied to the blob of LICENSE in both states: while it equals the snapshot's the line
	// is exactly licenseLineIdentical, and while it differs the line is exactly licenseLineChanged. No word
	// of the line is a key, so a reworded claim fails either way.
	licenseLine, found := "", false
	for _, line := range strings.Split(provenance, "\n") {
		if strings.HasPrefix(line, "- License:") {
			licenseLine, found = line, true
			break
		}
	}
	if !found {
		t.Errorf("PROVENANCE.md has no %q line", "- License:")
		return
	}
	got, want := gitBlobID(readRepoFile(t, root, evidenceDirRel+"/LICENSE")), evidenceSnapshotBlobs["LICENSE"]
	switch {
	case got == want && licenseLine != licenseLineIdentical:
		t.Errorf("LICENSE equals the snapshot, so the License line is exactly %q, got %q", licenseLineIdentical, licenseLine)
	case got != want && licenseLine != licenseLineChanged:
		t.Errorf("LICENSE differs from the snapshot (blob %s, not %s), so the License line is exactly %q, got %q", got, want, licenseLineChanged, licenseLine)
	}
}

// The files of the evidence store are deployed beside the skills and read in another repository, which has no task
// document: none of them names a section, task or log entry of one (S, T or L and a number, or OD- and a number;
// review 00564ea07ad9-5 F4, T4g, L62 choice 4).
func TestEvidenceStore_NamesNoSectionTaskOrLogEntryOfATaskDocument(t *testing.T) {
	root := skillsRepoRoot(t)
	pattern := regexp.MustCompile(`\b(S|T|L)[0-9]+\b|\bOD-[0-9]+\b`)
	for _, name := range evidenceStoreNames {
		if match := pattern.FindString(readRepoFile(t, root, evidenceDirRel+"/"+name)); match != "" {
			t.Errorf("%s names %q, a reference into a task document that the deployed file cannot carry", name, match)
		}
	}
}
