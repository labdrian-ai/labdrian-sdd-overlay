package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The manifest rows used across these tests mirror the real overlay.manifest
// shape: bare two-column skill rows, engine/* infra rows, a root-level row with
// no path separator, and three-column agent rows.
const routingManifest = `# comment line
engine/go.mod managed
engine/skills/sync.go managed
sdd-spec/SKILL.md managed
prespec-malandra/SKILL.md custom
prespec-malandra/references/coverage-taxonomy.md custom
_shared/minimalism-contract.md custom

skills.registry.yaml custom
GADU.md                  custom   agent
sdd-explore.md           managed  agent
opencode/agents/GADU.md custom   opencode-agent
`

func TestDeployableManifestPaths(t *testing.T) {
	got, err := DeployableManifestPaths(strings.NewReader(routingManifest))
	if err != nil {
		t.Fatalf("DeployableManifestPaths: unexpected error: %v", err)
	}

	// Only rows that route_resolve sends to skills/ belong here.
	want := []string{
		"sdd-spec/SKILL.md",
		"prespec-malandra/SKILL.md",
		"prespec-malandra/references/coverage-taxonomy.md",
		"_shared/minimalism-contract.md",
	}
	for _, w := range want {
		if _, ok := got[w]; !ok {
			t.Errorf("DeployableManifestPaths: missing deployable row %q", w)
		}
	}

	// engine/* is infra, a path with no separator is root-level, and the two
	// agent routes land outside skills/. None may be treated as deployable.
	excluded := []string{
		"engine/go.mod",
		"engine/skills/sync.go",
		"skills.registry.yaml",
		"GADU.md",
		"sdd-explore.md",
		"opencode/agents/GADU.md",
	}
	for _, e := range excluded {
		if _, ok := got[e]; ok {
			t.Errorf("DeployableManifestPaths: %q is not deployable to skills/ but was included", e)
		}
	}

	if len(got) != len(want) {
		t.Errorf("DeployableManifestPaths: got %d rows, want %d: %v", len(got), len(want), got)
	}
}

func TestScanSkillFiles(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"alpha/SKILL.md",
		"alpha/references/one.md",
		"_shared/contract.md",
		"beta/SKILL.md",
	}
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", f, err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}

	got, err := ScanSkillFiles(root)
	if err != nil {
		t.Fatalf("ScanSkillFiles: unexpected error: %v", err)
	}

	// Sorted, slash-separated, relative to root — directories are not entries.
	want := []string{
		"_shared/contract.md",
		"alpha/SKILL.md",
		"alpha/references/one.md",
		"beta/SKILL.md",
	}
	if len(got) != len(want) {
		t.Fatalf("ScanSkillFiles: got %d entries %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ScanSkillFiles[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestScanSkillFilesMissingDir(t *testing.T) {
	if _, err := ScanSkillFiles(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("ScanSkillFiles: expected an error for a missing directory, got nil")
	}
}

// TestDiffOnDiskCatchesUnregisteredSkill is the regression test for the class of
// bug that shipped anti-generic-design undeployed: a skill present on disk and in
// Git but absent from overlay.manifest. Registry-vs-manifest cross-checking
// cannot see it, because a skill missing from BOTH files reads as aligned.
func TestDiffOnDiskCatchesUnregisteredSkill(t *testing.T) {
	disk := []string{
		"registered/SKILL.md",
		"unregistered/SKILL.md",
		"unregistered/references/detail.md",
	}
	manifest := map[string]struct{}{
		"registered/SKILL.md": {},
	}

	divs := DiffOnDisk(disk, manifest)
	if len(divs) != 2 {
		t.Fatalf("DiffOnDisk: got %d divergences %v, want 2", len(divs), divs)
	}
	for _, d := range divs {
		if d.Class != DivUnregisteredOnDisk {
			t.Errorf("DiffOnDisk: class = %q, want %q", d.Class, DivUnregisteredOnDisk)
		}
		if !strings.HasPrefix(d.Path, "unregistered/") {
			t.Errorf("DiffOnDisk: unexpected path %q flagged", d.Path)
		}
	}
}

// TestDiffOnDiskCatchesManifestRowWithNoFile covers the symmetric failure: a
// manifest row whose source file does not exist makes the deploy step fail.
func TestDiffOnDiskCatchesManifestRowWithNoFile(t *testing.T) {
	disk := []string{"present/SKILL.md"}
	manifest := map[string]struct{}{
		"present/SKILL.md": {},
		"ghost/SKILL.md":   {},
	}

	divs := DiffOnDisk(disk, manifest)
	if len(divs) != 1 {
		t.Fatalf("DiffOnDisk: got %d divergences %v, want 1", len(divs), divs)
	}
	if divs[0].Class != DivMissingOnDisk {
		t.Errorf("DiffOnDisk: class = %q, want %q", divs[0].Class, DivMissingOnDisk)
	}
	if divs[0].Path != "ghost/SKILL.md" {
		t.Errorf("DiffOnDisk: path = %q, want %q", divs[0].Path, "ghost/SKILL.md")
	}
}

func TestDiffOnDiskClean(t *testing.T) {
	disk := []string{"a/SKILL.md", "a/references/r.md"}
	manifest := map[string]struct{}{
		"a/SKILL.md":        {},
		"a/references/r.md": {},
	}
	if divs := DiffOnDisk(disk, manifest); len(divs) != 0 {
		t.Fatalf("DiffOnDisk: got %d divergences %v, want none", len(divs), divs)
	}
}

// TestRepositorySkillsAreFullyRegistered is the live invariant over this
// repository: every file under skills/ must have a deploying overlay.manifest
// row, and every deploying row must have a file. This is the guard that would
// have failed while anti-generic-design sat on disk unregistered, at a time when
// apply, sync-check and skills validate all reported green.
func TestRepositorySkillsAreFullyRegistered(t *testing.T) {
	root := skillsRepoRoot(t)

	manifestFile, err := os.Open(filepath.Join(root, "overlay.manifest"))
	if err != nil {
		t.Fatalf("open overlay.manifest: %v", err)
	}
	defer manifestFile.Close()

	rows, err := DeployableManifestPaths(manifestFile)
	if err != nil {
		t.Fatalf("DeployableManifestPaths: %v", err)
	}

	disk, err := ScanSkillFiles(filepath.Join(root, "skills"))
	if err != nil {
		t.Fatalf("ScanSkillFiles: %v", err)
	}

	if divs := DiffOnDisk(disk, rows); len(divs) != 0 {
		var b strings.Builder
		for _, d := range divs {
			b.WriteString("\n  [" + string(d.Class) + "] " + d.Path + ": " + d.Detail)
		}
		t.Fatalf("skills/ and overlay.manifest disagree — %d divergence(s):%s", len(divs), b.String())
	}
}

// skillsRepoRoot resolves the repository root from this package's test working
// directory: engine/skills/ → engine/ → repo root.
func skillsRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Join(wd, "..", "..")
}
