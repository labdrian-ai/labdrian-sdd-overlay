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

func TestEvidenceStore_ManifestHoldsFourCustomRows(t *testing.T) {
	manifest := readRepoFile(t, skillsRepoRoot(t), "overlay.manifest")
	for _, name := range []string{"evidence.sh", "evidence-rules.md", "LICENSE", "PROVENANCE.md"} {
		row := "_shared/evidence/" + name + " custom\n"
		if !strings.Contains(manifest, row) {
			t.Errorf("overlay.manifest has no row %q", strings.TrimSuffix(row, "\n"))
		}
	}
}

func TestEvidenceStore_DeploysBesideEachOtherForClaude(t *testing.T) {
	manifest := readRepoFile(t, skillsRepoRoot(t), "overlay.manifest")
	rows := ReadManifestRows([]byte(manifest))
	routes, err := ResolveRoutes(rows, false, testOverlay, RouteDestinations{Target: "claude", SkillsRoot: "/s", AgentsRoot: "/a"})
	if err != nil {
		t.Fatalf("ResolveRoutes: %v", err)
	}
	dest := map[string]string{}
	for _, r := range routes {
		dest[r.Path] = r.Dest
	}
	for _, name := range []string{"evidence.sh", "evidence-rules.md"} {
		path := "_shared/evidence/" + name
		if want := "/s/" + path; dest[path] != want {
			t.Errorf("%s deploys to %q, want %q", path, dest[path], want)
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
