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

// approvalStore holds the files of skills and their records by skill path and id.
type approvalStore struct {
	trees   map[string][]skills.SkillEntry
	records map[string][]byte
}

func (s approvalStore) ReadSkillTree(_, path string) ([]skills.SkillEntry, error) {
	if entries, ok := s.trees[path]; ok {
		return entries, nil
	}
	return nil, fs.ErrNotExist
}

func (s approvalStore) ReadRecord(_, id string) ([]byte, error) {
	if data, ok := s.records[id]; ok {
		return data, nil
	}
	return nil, fs.ErrNotExist
}

// skillOf is the files of a skill whose directory holds just this SKILL.md.
func skillOf(skillMD string) []skills.SkillEntry {
	return []skills.SkillEntry{{Rel: "SKILL.md", Data: []byte(skillMD)}}
}

// record is the record of version 2 that approves exactly these files of skill id.
func record(t *testing.T, id string, entries []skills.SkillEntry) []byte {
	t.Helper()
	digest, err := skills.SkillDigest(entries)
	if err != nil {
		t.Fatal(err)
	}
	data, err := skills.SerializeApprovalRecord(skills.ApprovalRecord{Skill: id, SHA256: digest, ApprovedAt: "2026-09-30T12:00:00Z", Approver: "reviewer"})
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
	alpha, beta := skillOf("alpha skill"), skillOf("beta skill")
	return &world{
		registry: registryOf(entry("alpha", "custom", "claude"), entry("beta", "custom", "claude")),
		manifest: map[string]string{"overlay.manifest": "alpha/SKILL.md custom\nbeta/SKILL.md custom\n"},
		tree:     treeOf{files: []string{"alpha/SKILL.md", "beta/SKILL.md"}},
		approved: approvalStore{
			trees:   map[string][]skills.SkillEntry{"alpha": alpha, "beta": beta},
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
	// SkillFileMissing counts the global skills set aside because they have no SKILL.md; both have one.
	if res.SkillCount != 2 || res.DiskFiles != 2 || res.Approvals.Global != 2 || res.Approvals.Approved != 2 || res.Approvals.SkillFileMissing != 0 {
		t.Errorf("the tallies are %+v, want 2 skills, 2 files, 2 global, 2 approved, none set aside", res)
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
	w.manifest["overlay.manifest"] = "alpha/SKILL.md custom\nghost/SKILL.md custom\n"           // beta has no row; ghost has no entry
	w.tree.files = []string{"alpha/SKILL.md", "beta/SKILL.md", "stray.txt"}                     // stray is on disk and not in the manifest
	w.approved.records = map[string][]byte{"alpha": record(t, "alpha", skillOf("alpha skill"))} // beta is not approved
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

func classesOf(divs []skills.Divergence) string {
	var classes []string
	for _, d := range divs {
		classes = append(classes, string(d.Class))
	}
	return strings.Join(classes, ",")
}

func TestValidateOverlayReportsAFileOnDiskThatNoRowDeploysThenPassesOnceItHasOne(t *testing.T) {
	w := newWorld(t)
	w.tree.files = []string{"alpha/SKILL.md", "beta/SKILL.md", "orphan/notes.md"}
	res, err := ValidateOverlay(w.ports(), input())
	if err != nil || res.Passed() || classesOf(res.OnDisk) != "UNREGISTERED_ON_DISK" {
		t.Fatalf("ValidateOverlay = %+v, %v, want one UNREGISTERED_ON_DISK divergence", res, err)
	}
	w.manifest["overlay.manifest"] += "orphan/notes.md custom\n"
	res, err = ValidateOverlay(w.ports(), input())
	if err != nil || !res.Passed() {
		t.Errorf("ValidateOverlay = %+v, %v, want a pass once the row is added", res, err)
	}
}

func TestValidateOverlayReportsARowWithNoFileOnDisk(t *testing.T) {
	w := newWorld(t)
	w.manifest["overlay.manifest"] += "ghost/SKILL.md custom\n"
	res, err := ValidateOverlay(w.ports(), input())
	if err != nil || res.Passed() || !strings.Contains(classesOf(res.OnDisk), "MISSING_ON_DISK") {
		t.Errorf("ValidateOverlay = %+v, %v, want a MISSING_ON_DISK divergence", res, err)
	}
}

func TestValidateOverlayReportsTheDivergencesOfEveryClassInOneRun(t *testing.T) {
	w := newWorld(t)
	// beta has no manifest row; ghost is an orphan row; unregistered/SKILL.md has no row either.
	w.manifest["overlay.manifest"] = "alpha/SKILL.md custom\nghost/SKILL.md custom\n"
	w.tree.files = []string{"alpha/SKILL.md", "unregistered/SKILL.md"}
	res, err := ValidateOverlay(w.ports(), input())
	if err != nil {
		t.Fatal(err)
	}
	all := classesOf(res.RegistryDivergences) + "," + classesOf(res.OnDisk)
	for _, class := range []string{"MISSING_IN_MANIFEST", "MISSING_ON_DISK", "UNREGISTERED_ON_DISK"} {
		if !strings.Contains(all, class) {
			t.Errorf("divergences %q do not include %s", all, class)
		}
	}
}

// recordingTree remembers the directories it was asked to list.
type recordingTree struct {
	treeOf
	asked *[]string
}

func (r recordingTree) ScanSkillFiles(dir string) ([]string, error) {
	*r.asked = append(*r.asked, dir)
	return r.treeOf.ScanSkillFiles(dir)
}

// The tree that is listed is the one --source-root names, as it was written: absolute or
// relative to the process (CI runs it as ../skills), never the working directory.
func TestValidateOverlayListsTheSourceRootItIsGivenAsItWasGiven(t *testing.T) {
	for _, root := range []string{"/does-not-matter", "../skills", "skills"} {
		w := newWorld(t)
		var asked []string
		ports := w.ports()
		ports.Tree = recordingTree{treeOf: w.tree, asked: &asked}
		in := input()
		in.SourceRoot = root
		if _, err := ValidateOverlay(ports, in); err != nil {
			t.Fatal(err)
		}
		if len(asked) != 1 || asked[0] != root {
			t.Errorf("the tree was asked for %q, want one listing of %q", asked, root)
		}
	}
}

// No global skill passes without a valid record of its files: one with no record is unapproved,
// whatever its id or its history, and the tally has no other way to count it.
func TestValidateOverlayRefusesAGlobalSkillWithNoRecord(t *testing.T) {
	w := newWorld(t)
	delete(w.approved.records, "beta")

	res, err := ValidateOverlay(w.ports(), input())

	if err != nil || res.Passed() || len(res.Unapproved) != 1 || res.Unapproved[0].Path != "beta" || res.Unapproved[0].Class != skills.DivApprovalMissing {
		t.Fatalf("ValidateOverlay = %+v, %v, want beta unapproved as APPROVAL_MISSING", res, err)
	}
	if res.Approvals.Global != 2 || res.Approvals.Approved != 1 {
		t.Errorf("the tally is %+v, want 2 global and 1 approved", res.Approvals)
	}
}

// The approval is of every file of the skill: a file added beside SKILL.md after the record was
// made, or one removed, or one changed, is a skill whose approval is stale.
func TestValidateOverlayRefusesASkillWhoseFilesChangedAfterTheApproval(t *testing.T) {
	for name, change := range map[string]func(files []skills.SkillEntry) []skills.SkillEntry{
		"a file added": func(files []skills.SkillEntry) []skills.SkillEntry {
			return append(files, skills.SkillEntry{Rel: "scripts/run.sh", Data: []byte("echo hi\n")})
		},
		"a file removed": func(files []skills.SkillEntry) []skills.SkillEntry { return files[:1] },
		"a file changed": func(files []skills.SkillEntry) []skills.SkillEntry {
			files[1].Data = []byte("echo changed\n")
			return files
		},
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			approvedFiles := []skills.SkillEntry{{Rel: "SKILL.md", Data: []byte("beta skill")}, {Rel: "scripts/run.sh", Data: []byte("echo hi\n")}}
			w.approved.trees["beta"] = approvedFiles
			w.approved.records["beta"] = record(t, "beta", approvedFiles)
			if res, err := ValidateOverlay(w.ports(), input()); err != nil || !res.Passed() {
				t.Fatalf("before the change: %+v, %v, want a pass", res, err)
			}

			w.approved.trees["beta"] = change([]skills.SkillEntry{{Rel: "SKILL.md", Data: []byte("beta skill")}, {Rel: "scripts/run.sh", Data: []byte("echo hi\n")}})
			res, err := ValidateOverlay(w.ports(), input())

			if err != nil || res.Passed() || len(res.Unapproved) != 1 || res.Unapproved[0].Class != skills.DivApprovalStale {
				t.Errorf("ValidateOverlay = %+v, %v, want beta unapproved as APPROVAL_STALE", res, err)
			}
		})
	}
}

// A record of version 1 approved SKILL.md alone: it is stale even while SKILL.md is what it was,
// and the finding says what it covered and which command approves the skill again.
func TestValidateOverlayRefusesARecordOfVersionOne(t *testing.T) {
	w := newWorld(t)
	w.approved.records["beta"] = []byte(legacyRecord("beta", "beta skill"))

	res, err := ValidateOverlay(w.ports(), input())

	if err != nil || res.Passed() || len(res.Unapproved) != 1 || res.Unapproved[0].Class != skills.DivApprovalStale {
		t.Fatalf("ValidateOverlay = %+v, %v, want beta unapproved as APPROVAL_STALE", res, err)
	}
	for _, want := range []string{"covers only SKILL.md", "labdrian skills approve --id beta"} {
		if !strings.Contains(res.Unapproved[0].Detail, want) {
			t.Errorf("detail %q does not say %q", res.Unapproved[0].Detail, want)
		}
	}
}

// A record of any other version cannot be read: it is malformed, not stale.
func TestValidateOverlayTakesARecordOfAnotherVersionForMalformed(t *testing.T) {
	w := newWorld(t)
	w.approved.records["beta"] = []byte(strings.Replace(string(record(t, "beta", skillOf("beta skill"))), `"version": 2`, `"version": 3`, 1))

	res, err := ValidateOverlay(w.ports(), input())

	if err != nil || len(res.Unapproved) != 1 || res.Unapproved[0].Class != skills.DivApprovalMalformed {
		t.Errorf("ValidateOverlay = %+v, %v, want beta unapproved as APPROVAL_MALFORMED", res, err)
	}
}

// What an approval cannot cover makes the skill unverifiable, whatever its record says.
func TestValidateOverlayCannotVerifyASkillWithALinkInIt(t *testing.T) {
	w := newWorld(t)
	w.approved.trees["beta"] = append(skillOf("beta skill"), skills.SkillEntry{Rel: "alias.md", Type: fs.ModeSymlink})

	res, err := ValidateOverlay(w.ports(), input())

	if err != nil || res.Passed() || len(res.Unapproved) != 1 || res.Unapproved[0].Class != skills.DivApprovalUnverifiable {
		t.Fatalf("ValidateOverlay = %+v, %v, want beta unapproved as APPROVAL_UNVERIFIABLE", res, err)
	}
	if !strings.Contains(res.Unapproved[0].Detail, "alias.md is a symbolic link") {
		t.Errorf("detail %q does not name the link", res.Unapproved[0].Detail)
	}
}
