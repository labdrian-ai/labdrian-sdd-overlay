package app

import (
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// treeOf is a SkillTree that lists the files it is given, or fails.
type treeOf struct {
	files []string
	err   error
}

func (t treeOf) ScanSkillFiles(string) ([]string, error) { return t.files, t.err }
func (treeOf) ReadSkillSource(string) ([]skills.SourceFile, error) {
	return nil, errors.New("not used")
}

// approvalStore holds SKILL.md files and records by skill path and id.
type approvalStore struct {
	skillMD map[string][]byte
	records map[string][]byte
}

func (s approvalStore) ReadSkill(_, path string) ([]byte, error) {
	if data, ok := s.skillMD[path]; ok {
		return data, nil
	}
	return nil, fs.ErrNotExist
}

func (s approvalStore) ReadRecord(_, id string) ([]byte, error) {
	if data, ok := s.records[id]; ok {
		return data, nil
	}
	return nil, fs.ErrNotExist
}

func record(t *testing.T, id string, skillMD []byte) []byte {
	t.Helper()
	data, err := skills.SerializeApprovalRecord(skills.ApprovalRecord{Skill: id, SHA256: skills.SkillDigest(skillMD), ApprovedAt: "2026-09-30T12:00:00Z", Approver: "reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// world is an overlay whose registry holds alpha and beta, both global, both approved, with the
// manifest and the tree that agree.
type world struct {
	registry skills.Registry
	manifest map[string]string
	tree     treeOf
	approved approvalStore
}

func newWorld(t *testing.T) *world {
	t.Helper()
	alpha, beta := []byte("alpha skill"), []byte("beta skill")
	return &world{
		registry: registryOf(entry("alpha", "custom", "claude"), entry("beta", "custom", "claude")),
		manifest: map[string]string{"overlay.manifest": "alpha/SKILL.md custom\nbeta/SKILL.md custom\n"},
		tree:     treeOf{files: []string{"alpha/SKILL.md", "beta/SKILL.md"}},
		approved: approvalStore{
			skillMD: map[string][]byte{"alpha": alpha, "beta": beta},
			records: map[string][]byte{"alpha": record(t, "alpha", alpha), "beta": record(t, "beta", beta)},
		},
	}
}

func (w *world) ports() ValidatePorts {
	return ValidatePorts{
		Registries: registries{"r.yaml": w.registry},
		Manifest: func(name string) ([]byte, error) {
			if data, ok := w.manifest[name]; ok {
				return []byte(data), nil
			}
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		},
		Tree:      w.tree,
		Approvals: w.approved,
	}
}

func input() ValidateInput {
	return ValidateInput{RegistryPath: "r.yaml", ManifestPath: "overlay.manifest", SourceRoot: "skills"}
}

func TestValidateOverlayPassesAnOverlayWhoseChecksAreAllClean(t *testing.T) {
	w := newWorld(t)
	res, err := ValidateOverlay(w.ports(), input())
	if err != nil || !res.Passed() {
		t.Fatalf("ValidateOverlay = %+v, %v, want a pass", res, err)
	}
	if res.SkillCount != 2 || res.DiskFiles != 2 || res.Approvals.Global != 2 || res.Approvals.Approved != 2 || res.Approvals.Grandfathered != 0 {
		t.Errorf("the tallies are %+v, want 2 skills, 2 files, 2 global, 2 approved", res)
	}
}

func TestValidateOverlayNeedsTheSourceRootAndReadsNothingWithout(t *testing.T) {
	w := newWorld(t)
	ports := w.ports()
	ports.Registries = nil // would be refused if it were asked
	in := input()
	in.SourceRoot = ""
	res, err := ValidateOverlay(ports, in)
	if !errors.Is(err, ErrSourceRootRequired) || res.SkillCount != 0 {
		t.Errorf("ValidateOverlay = %+v, %v, want ErrSourceRootRequired and nothing read", res, err)
	}
	if err.Error() != "skills validate requires --source-root <skills-dir>" {
		t.Errorf("message = %q", err)
	}
}

func TestValidateOverlayTellsARegistryItCannotUse(t *testing.T) {
	w := newWorld(t)
	in := input()
	in.RegistryPath = "absent.yaml"
	_, err := ValidateOverlay(w.ports(), in)
	var refusal *RegistryError
	if !errors.As(err, &refusal) || !refusal.Unreadable() {
		t.Errorf("err = %v, want an unreadable RegistryError", err)
	}
}

func TestValidateOverlayReportsEveryDivergenceOfEveryCheckInOneRun(t *testing.T) {
	w := newWorld(t)
	w.manifest["overlay.manifest"] = "alpha/SKILL.md custom\nghost/SKILL.md custom\n"          // beta has no row; ghost has no entry
	w.tree.files = []string{"alpha/SKILL.md", "beta/SKILL.md", "stray.txt"}                    // stray is on disk and not in the manifest
	w.approved.records = map[string][]byte{"alpha": record(t, "alpha", []byte("alpha skill"))} // beta is not approved
	res, err := ValidateOverlay(w.ports(), input())
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed() || !res.RegistryDiverges || len(res.RegistryDivergences) == 0 || len(res.OnDisk) == 0 || len(res.Unapproved) != 1 {
		t.Errorf("result = %+v, want the registry/manifest, the on-disk and the approval checks all to report", res)
	}
	if res.Unapproved[0].Path != "beta" {
		t.Errorf("unapproved = %+v, want beta", res.Unapproved)
	}
}

func TestValidateOverlayKeepsTheRegistryDivergencesWhenALaterStageStops(t *testing.T) {
	w := newWorld(t)
	w.manifest["overlay.manifest"] = "alpha/SKILL.md custom\n" // beta has no row
	w.tree.err = errors.New("the tree is gone")
	res, err := ValidateOverlay(w.ports(), input())
	var scan *ScanError
	if !errors.As(err, &scan) || scan.Root != "skills" {
		t.Fatalf("err = %v, want a ScanError naming the source root", err)
	}
	if !res.RegistryDiverges || len(res.RegistryDivergences) == 0 {
		t.Errorf("result = %+v, want the divergence found before the scan failed", res)
	}
	if want := `scanning skills directory "skills": the tree is gone`; err.Error() != want {
		t.Errorf("message = %q, want %q", err, want)
	}
}

func TestValidateOverlaySaysAManifestItCannotRead(t *testing.T) {
	w := newWorld(t)
	delete(w.manifest, "overlay.manifest")
	res, err := ValidateOverlay(w.ports(), input())
	var read *ManifestReadError
	if !errors.As(err, &read) || read.Path != "overlay.manifest" || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err = %v, want a ManifestReadError for overlay.manifest", err)
	}
	if res.RegistryDiverges {
		t.Error("with no manifest read there is no registry divergence to tell")
	}
	if !strings.HasPrefix(err.Error(), `reading manifest "overlay.manifest": `) {
		t.Errorf("message = %q", err)
	}
}

func TestValidateOverlayNotesTheIdsThatShareAPathAndPasses(t *testing.T) {
	w := newWorld(t)
	twin := entry("alpha-copy", "custom", "claude")
	twin.Path = "alpha"
	w.registry = registryOf(entry("alpha", "custom", "claude"), twin)
	w.manifest["overlay.manifest"] = "alpha/SKILL.md custom\n"
	w.tree.files = []string{"alpha/SKILL.md"}
	res, err := ValidateOverlay(w.ports(), input())
	if err != nil || !res.Passed() {
		t.Fatalf("ValidateOverlay = %+v, %v, want a pass: a shared path is no failure", res, err)
	}
	if len(res.SharedPathNotes) != 1 || !strings.Contains(res.SharedPathNotes[0], `"alpha" and "alpha-copy" share the path "alpha"`) {
		t.Errorf("notes = %q, want one note naming both ids and the path", res.SharedPathNotes)
	}
}

func TestValidateOverlayDoesNotPassARegistryTheReaderLeftFieldsOutOf(t *testing.T) {
	w := newWorld(t)
	w.registry.Unread = []string{"line 3: unknown key \"color\" in skill entry"}
	res, err := ValidateOverlay(w.ports(), input())
	if err != nil || res.Passed() || res.NotVerifiable == nil {
		t.Fatalf("ValidateOverlay = %+v, %v, want no pass: validate cannot vouch for a registry it read in part", res, err)
	}
	if res.UnreadWarning == "" {
		t.Error("the warning of what was left out is not carried")
	}
	if len(res.RegistryDivergences)+len(res.OnDisk)+len(res.Unapproved) != 0 {
		t.Errorf("the checks that were clean reported divergences: %+v", res)
	}
}

func TestValidateOverlayTreatsSkillsOfProjectScopeAsAutonomous(t *testing.T) {
	w := newWorld(t)
	scoped := entry("tidy", "custom", "claude")
	scoped.Install = skills.Install{DefaultScope: "project", Targets: []string{"claude"}, AllowedProjects: []string{"demo"}}
	w.registry = registryOf(entry("alpha", "custom", "claude"), scoped)
	w.manifest["overlay.manifest"] = "alpha/SKILL.md custom\ntidy/SKILL.md custom\n"
	w.tree.files = []string{"alpha/SKILL.md", "tidy/SKILL.md"}
	res, err := ValidateOverlay(w.ports(), input())
	if err != nil || !res.Passed() || res.Approvals.Global != 1 {
		t.Errorf("ValidateOverlay = %+v, %v, want a pass that counted one global skill", res, err)
	}
}
