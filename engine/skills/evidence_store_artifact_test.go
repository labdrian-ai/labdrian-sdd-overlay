package skills

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is not on PATH")
	}
	script := filepath.Join(skillsRepoRoot(t), evidenceDirRel, "evidence.sh")
	if out, err := exec.Command(sh, "-n", script).CombinedOutput(); err != nil {
		t.Fatalf("sh -n %s: %v\n%s", script, err, out)
	}
}
