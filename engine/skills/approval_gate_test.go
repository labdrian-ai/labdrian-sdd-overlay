package skills

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// setBaselineForTest replaces the grandfathered baseline for one test, so a
// fixture can grandfather bytes it controls. The real baseline pins the real
// repository's SKILL.md digests and cannot be reproduced by a fixture.
func setBaselineForTest(t *testing.T, entries []ApprovalBaselineEntry) {
	t.Helper()
	saved := approvalBaseline
	approvalBaseline = entries
	t.Cleanup(func() { approvalBaseline = saved })
}

// writeValidApproval writes a record for the SKILL.md currently on disk under
// root/id, exactly as `skills approve` would.
func writeValidApproval(t *testing.T, root, id string) {
	t.Helper()
	skill, err := os.ReadFile(filepath.Join(root, id, "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md for %q: %v", id, err)
	}
	data, err := SerializeApprovalRecord(ApprovalRecord{
		Skill:      id,
		SHA256:     SkillDigest(skill),
		ApprovedAt: "2026-09-30T12:00:00Z",
		Approver:   "fixture-reviewer",
	})
	if err != nil {
		t.Fatalf("serialize record for %q: %v", id, err)
	}
	writeTestFile(t, ApprovalRecordPath(root, id), string(data))
}

const approveCommandFor = "labdrian skills approve --id "

func TestEvaluateApproval_TheFourStatesAndTheBaseline(t *testing.T) {
	skill := []byte("the exact bytes")
	digest := SkillDigest(skill)
	recordPath := "skills/my-skill/" + ApprovalRecordName

	valid := ApprovalStatus{State: ApprovalValid}
	absent := ApprovalStatus{State: ApprovalAbsent}
	stale := ApprovalStatus{State: ApprovalStale, Detail: "SKILL.md changed after approval: record digest aaa, file digest bbb"}
	malformed := ApprovalStatus{State: ApprovalMalformed, Detail: "parse approval record: unexpected end of JSON input"}

	t.Run("valid record satisfies the requirement", func(t *testing.T) {
		v := EvaluateApproval("my-skill", recordPath, skill, valid)
		if !v.OK || v.Grandfathered {
			t.Fatalf("verdict = %+v, want OK and not grandfathered", v)
		}
	})

	for _, tc := range []struct {
		name   string
		status ApprovalStatus
		class  DivergenceClass
		state  string
	}{
		{"absent", absent, DivApprovalMissing, "no approval record"},
		{"stale", stale, DivApprovalStale, "stale"},
		{"malformed", malformed, DivApprovalMalformed, "malformed"},
	} {
		t.Run(tc.name+" record is refused with the skill, the state and the fixing command", func(t *testing.T) {
			setBaselineForTest(t, nil)
			v := EvaluateApproval("my-skill", recordPath, skill, tc.status)
			if v.OK {
				t.Fatalf("verdict = %+v, want a refusal", v)
			}
			if v.Class != tc.class {
				t.Errorf("class = %q, want %q", v.Class, tc.class)
			}
			for _, want := range []string{`"my-skill"`, tc.state, approveCommandFor + "my-skill --approver", recordPath} {
				if !strings.Contains(v.Detail, want) {
					t.Errorf("detail %q does not contain %q", v.Detail, want)
				}
			}
		})
	}

	t.Run("absent record is fine for the grandfathered bytes", func(t *testing.T) {
		setBaselineForTest(t, []ApprovalBaselineEntry{{ID: "my-skill", SHA256: digest}})
		v := EvaluateApproval("my-skill", recordPath, skill, absent)
		if !v.OK || !v.Grandfathered {
			t.Fatalf("verdict = %+v, want OK and grandfathered", v)
		}
	})

	t.Run("a baseline skill whose bytes changed needs a record and the refusal says why", func(t *testing.T) {
		setBaselineForTest(t, []ApprovalBaselineEntry{{ID: "my-skill", SHA256: SkillDigest([]byte("the original bytes"))}})
		v := EvaluateApproval("my-skill", recordPath, skill, absent)
		if v.OK || v.Class != DivApprovalMissing {
			t.Fatalf("verdict = %+v, want APPROVAL_MISSING", v)
		}
		if !strings.Contains(v.Detail, "baseline") {
			t.Errorf("detail %q must say the baseline skill was modified", v.Detail)
		}
		// With a valid record the modification is approved.
		if ok := EvaluateApproval("my-skill", recordPath, skill, valid); !ok.OK {
			t.Errorf("a valid record must satisfy a modified baseline skill: %+v", ok)
		}
	})

	t.Run("the baseline only excuses an absent record", func(t *testing.T) {
		// A record that is present must be valid, baseline or not: a stale or
		// malformed governance file is never silently ignored.
		setBaselineForTest(t, []ApprovalBaselineEntry{{ID: "my-skill", SHA256: digest}})
		if v := EvaluateApproval("my-skill", recordPath, skill, stale); v.OK || v.Class != DivApprovalStale {
			t.Errorf("stale record on unchanged baseline bytes: %+v, want APPROVAL_STALE", v)
		}
		if v := EvaluateApproval("my-skill", recordPath, skill, malformed); v.OK || v.Class != DivApprovalMalformed {
			t.Errorf("malformed record on unchanged baseline bytes: %+v, want APPROVAL_MALFORMED", v)
		}
	})

	t.Run("the baseline is per skill id", func(t *testing.T) {
		setBaselineForTest(t, []ApprovalBaselineEntry{{ID: "another-skill", SHA256: digest}})
		if v := EvaluateApproval("my-skill", recordPath, skill, absent); v.OK {
			t.Fatalf("another skill's baseline entry must not excuse this one: %+v", v)
		}
	})
}

// gateFixture is a registry with global and project-scope entries plus a
// matching skills tree, for CheckApprovals.
type gateFixture struct {
	root string
	reg  Registry
}

func newGateFixture(t *testing.T, globals []string, projects []string) gateFixture {
	t.Helper()
	root := filepath.Join(t.TempDir(), "skills")
	var reg Registry
	reg.Version = "1"
	for _, id := range globals {
		writeTestFile(t, filepath.Join(root, id, "SKILL.md"), lintCleanSkillMD(id))
		reg.Skills = append(reg.Skills, Entry{ID: id, Path: id, Install: Install{DefaultScope: "global"}})
	}
	for _, id := range projects {
		writeTestFile(t, filepath.Join(root, id, "SKILL.md"), lintCleanSkillMD(id))
		reg.Skills = append(reg.Skills, Entry{ID: id, Path: id, Install: Install{DefaultScope: "project", AllowedProjects: []string{"p"}}})
	}
	return gateFixture{root: root, reg: reg}
}

func classesByPath(divs []Divergence) map[string]DivergenceClass {
	out := map[string]DivergenceClass{}
	for _, d := range divs {
		out[d.Path] = d.Class
	}
	return out
}

func TestCheckApprovals_GlobalSkillsNeedAValidRecordAndProjectSkillsDoNot(t *testing.T) {
	setBaselineForTest(t, nil)
	f := newGateFixture(t, []string{"approved", "unapproved", "stale-one", "broken-one"}, []string{"project-skill"})
	writeValidApproval(t, f.root, "approved")
	writeValidApproval(t, f.root, "stale-one")
	writeTestFile(t, filepath.Join(f.root, "stale-one", "SKILL.md"), lintCleanSkillMD("stale-one")+"\nchanged after approval\n")
	writeTestFile(t, ApprovalRecordPath(f.root, "broken-one"), "{ nope")

	divs, sum := CheckApprovals(f.reg, f.root, fileApprovals(os.ReadFile))

	got := classesByPath(divs)
	want := map[string]DivergenceClass{
		"unapproved": DivApprovalMissing,
		"stale-one":  DivApprovalStale,
		"broken-one": DivApprovalMalformed,
	}
	if len(got) != len(want) {
		t.Fatalf("divergences = %v, want exactly %v", got, want)
	}
	for path, class := range want {
		if got[path] != class {
			t.Errorf("%s: class = %q, want %q", path, got[path], class)
		}
	}
	if _, flagged := got["project-skill"]; flagged {
		t.Error("project-tier skills stay autonomous: no approval is required")
	}
	if sum.Global != 4 || sum.Approved != 1 || sum.Grandfathered != 0 {
		t.Errorf("summary = %+v, want 4 global, 1 approved, 0 grandfathered", sum)
	}
}

func TestCheckApprovals_GrandfatheredBaselineNeedsNoRecordUntilItsBytesChange(t *testing.T) {
	f := newGateFixture(t, []string{"legacy"}, nil)
	original, _ := os.ReadFile(filepath.Join(f.root, "legacy", "SKILL.md"))
	setBaselineForTest(t, []ApprovalBaselineEntry{{ID: "legacy", SHA256: SkillDigest(original)}})

	divs, sum := CheckApprovals(f.reg, f.root, fileApprovals(os.ReadFile))
	if len(divs) != 0 || sum.Grandfathered != 1 {
		t.Fatalf("unchanged baseline skill: divs=%v summary=%+v, want none and 1 grandfathered", divs, sum)
	}

	// Modify the baseline skill: now it needs a record.
	writeTestFile(t, filepath.Join(f.root, "legacy", "SKILL.md"), string(original)+"\nedited\n")
	divs, _ = CheckApprovals(f.reg, f.root, fileApprovals(os.ReadFile))
	if got := classesByPath(divs); got["legacy"] != DivApprovalMissing {
		t.Fatalf("modified baseline skill: divergences = %v, want APPROVAL_MISSING", got)
	}

	// Approve the modification: clean again, and it is counted as approved.
	writeValidApproval(t, f.root, "legacy")
	divs, sum = CheckApprovals(f.reg, f.root, fileApprovals(os.ReadFile))
	if len(divs) != 0 || sum.Approved != 1 || sum.Grandfathered != 0 {
		t.Fatalf("approved modification: divs=%v summary=%+v", divs, sum)
	}
}

func TestCheckApprovals_LeavesMissingAndUnreadableSkillFilesToTheRightCheck(t *testing.T) {
	setBaselineForTest(t, nil)
	f := newGateFixture(t, []string{"missing-file", "unreadable"}, nil)
	if err := os.Remove(filepath.Join(f.root, "missing-file", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	// A directory where SKILL.md should be makes the read fail with something
	// other than "does not exist": the approval cannot be verified.
	skillPath := filepath.Join(f.root, "unreadable", "SKILL.md")
	if err := os.Remove(skillPath); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(skillPath, 0o755); err != nil {
		t.Fatal(err)
	}

	divs, _ := CheckApprovals(f.reg, f.root, fileApprovals(os.ReadFile))
	got := classesByPath(divs)
	if _, flagged := got["missing-file"]; flagged {
		t.Error("a missing SKILL.md is already reported by the manifest and on-disk cross-checks; do not report it twice")
	}
	if got["unreadable"] != DivApprovalUnverifiable {
		t.Errorf("unreadable SKILL.md: class = %q, want %q", got["unreadable"], DivApprovalUnverifiable)
	}
}

// The summary must account for every global entry exactly once: it is either
// approved, grandfathered, reported as an approval divergence, or set aside
// because its SKILL.md does not exist (which the manifest and on-disk
// cross-checks report). An entry that fell into none of them would make the
// counts read as if it had been verified.
func TestCheckApprovals_TheSummaryAccountsForEveryGlobalEntryExactlyOnce(t *testing.T) {
	f := newGateFixture(t, []string{"approved", "legacy", "unapproved", "no-file", "also-no-file"}, []string{"project-skill"})
	legacy, _ := os.ReadFile(filepath.Join(f.root, "legacy", "SKILL.md"))
	setBaselineForTest(t, []ApprovalBaselineEntry{{ID: "legacy", SHA256: SkillDigest(legacy)}})
	writeValidApproval(t, f.root, "approved")
	for _, id := range []string{"no-file", "also-no-file"} {
		if err := os.Remove(filepath.Join(f.root, id, "SKILL.md")); err != nil {
			t.Fatal(err)
		}
	}

	divs, sum := CheckApprovals(f.reg, f.root, fileApprovals(os.ReadFile))

	if sum.Global != 5 {
		t.Errorf("Global = %d, want 5: project-tier entries are not counted", sum.Global)
	}
	if sum.Approved != 1 || sum.Grandfathered != 1 || len(divs) != 1 || sum.SkillFileMissing != 2 {
		t.Errorf("summary = %+v with %d divergences, want 1 approved, 1 grandfathered, 1 divergence, 2 without a SKILL.md", sum, len(divs))
	}
	if accounted := sum.Approved + sum.Grandfathered + sum.SkillFileMissing + len(divs); accounted != sum.Global {
		t.Errorf("the summary accounts for %d of %d global entries: %+v with %d divergences", accounted, sum.Global, sum, len(divs))
	}
	if got := classesByPath(divs); got["unapproved"] != DivApprovalMissing || len(got) != 1 {
		t.Errorf("divergences = %v, want only unapproved as APPROVAL_MISSING: a missing SKILL.md is not an approval finding", got)
	}
}

func TestCheckApprovals_AnUnreadableRecordIsUnverifiableNotAbsent(t *testing.T) {
	setBaselineForTest(t, nil)
	f := newGateFixture(t, []string{"odd"}, nil)
	if err := os.MkdirAll(ApprovalRecordPath(f.root, "odd"), 0o755); err != nil {
		t.Fatal(err)
	}
	divs, _ := CheckApprovals(f.reg, f.root, fileApprovals(os.ReadFile))
	if got := classesByPath(divs); got["odd"] != DivApprovalUnverifiable {
		t.Fatalf("divergences = %v, want APPROVAL_UNVERIFIABLE", got)
	}
}

// ---- skills add ---------------------------------------------------------------

func setupFixtureWithoutApprovals(t *testing.T, dir, regContent, mfContent string, ids []string) (regPath, mfPath, root string) {
	t.Helper()
	regPath, mfPath, root = setupFixture(t, dir, regContent, mfContent, ids)
	for _, id := range ids {
		if err := os.Remove(ApprovalRecordPath(root, id)); err != nil {
			t.Fatalf("remove fixture record for %q: %v", id, err)
		}
	}
	return regPath, mfPath, root
}

func snapshotFiles(t *testing.T, paths ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		out[p] = string(b)
	}
	return out
}

// ---- the baseline --------------------------------------------------------------

func TestApprovalBaseline_IsWellFormedAndFixed(t *testing.T) {
	entries := ApprovalBaseline()
	// The count is pinned: adding to or removing from the grandfathered list is
	// a deliberate, reviewed change that must update this number with it.
	const baselineSize = 37
	if len(entries) != baselineSize {
		t.Fatalf("baseline has %d entries, want the fixed %d", len(entries), baselineSize)
	}
	if !sort.SliceIsSorted(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID }) {
		t.Error("baseline entries must be sorted by id")
	}
	digestRe := regexp.MustCompile(`^[0-9a-f]{64}$`)
	seen := map[string]bool{}
	for _, e := range entries {
		if seen[e.ID] {
			t.Errorf("duplicate baseline id %q", e.ID)
		}
		seen[e.ID] = true
		if !slugRe.MatchString(e.ID) {
			t.Errorf("baseline id %q is not a valid skill slug", e.ID)
		}
		if !digestRe.MatchString(e.SHA256) {
			t.Errorf("baseline digest for %q is not a lowercase 64-character hex: %q", e.ID, e.SHA256)
		}
	}
}

func TestApprovalBaseline_ReturnsACopy(t *testing.T) {
	first := ApprovalBaseline()
	first[0].ID = "tampered"
	if ApprovalBaseline()[0].ID == "tampered" {
		t.Fatal("ApprovalBaseline must not expose the backing slice")
	}
}

// TestApprovalBaseline_PinnedToTheRepositoryRegistry pins the baseline against
// this repository's real registry and real SKILL.md files, read-only.
//
// At the Phase 8 base every registered skill was global and unapproved, and the
// baseline grandfathers exactly those 37. So on the repository as it is:
//   - every baseline id is a registered global skill;
//   - every registered global skill is either grandfathered at its pinned
//     digest or carries a valid record (the latter is how a later, approved
//     modification of a baseline skill or a newly added skill stays green);
//   - CheckApprovals, the check behind `skills validate`, reports nothing.
func TestApprovalBaseline_PinnedToTheRepositoryRegistry(t *testing.T) {
	root := skillsRepoRoot(t)
	regData, err := os.ReadFile(filepath.Join(root, "skills.registry.yaml"))
	if err != nil {
		t.Fatalf("read real registry: %v", err)
	}
	// parseRegistry (export_test.go) takes the bytes of the YAML file, which is what regData
	// holds: the adapter decodes them and the domain judges the registry, as the program does.
	reg, err := parseRegistry(regData)
	if err != nil {
		t.Fatalf("parse real registry: %v", err)
	}
	global := map[string]bool{}
	for _, e := range reg.Skills {
		if e.Install.DefaultScope == "global" {
			global[e.Path] = true
		}
	}
	for _, b := range ApprovalBaseline() {
		if !global[b.ID] {
			t.Errorf("baseline id %q is not a registered global skill in skills.registry.yaml; a retired skill must be removed from the baseline deliberately", b.ID)
		}
	}

	skillsRoot := filepath.Join(root, "skills")
	divs, sum := CheckApprovals(reg, skillsRoot, fileApprovals(os.ReadFile))
	for _, d := range divs {
		t.Errorf("[%s] %s: %s", d.Class, d.Path, d.Detail)
	}
	if sum.Global != len(global) {
		t.Errorf("CheckApprovals examined %d global skills, the registry has %d", sum.Global, len(global))
	}
	if sum.Approved+sum.Grandfathered != sum.Global {
		t.Errorf("summary = %+v: every global skill must be approved or grandfathered", sum)
	}
}
