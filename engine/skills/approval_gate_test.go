package skills

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

// writeValidApproval writes a record for the files currently on disk under root/id, exactly as
// `skills approve` would: the digest of every file of the skill (SkillDigest), not of SKILL.md
// alone.
func writeValidApproval(t *testing.T, root, id string) {
	t.Helper()
	entries, err := ReadSkillTreeOfTheDouble(root, id)
	if err != nil {
		t.Fatalf("read the files of %q: %v", id, err)
	}
	digest, err := SkillDigest(entries)
	if err != nil {
		t.Fatalf("the files of %q have no digest: %v", id, err)
	}
	data, err := SerializeApprovalRecord(ApprovalRecord{
		Skill:      id,
		SHA256:     digest,
		ApprovedAt: "2026-09-30T12:00:00Z",
		Approver:   "fixture-reviewer",
	})
	if err != nil {
		t.Fatalf("serialize record for %q: %v", id, err)
	}
	writeTestFile(t, ApprovalRecordPath(root, id), string(data))
}

// symlinkOrSkip makes a symbolic link at link to target, or skips the test on a file system that
// has none.
func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot make a symbolic link here: %v", err)
	}
}

const approveCommandFor = "labdrian skills approve --id "

func TestEvaluateApproval_OnlyAValidRecordSatisfiesTheRequirement(t *testing.T) {
	recordPath := "skills/my-skill/" + ApprovalRecordName

	t.Run("a valid record satisfies the requirement", func(t *testing.T) {
		if v := EvaluateApproval("my-skill", recordPath, ApprovalStatus{State: ApprovalValid}); !v.OK {
			t.Fatalf("verdict = %+v, want OK", v)
		}
	})

	for _, tc := range []struct {
		name   string
		status ApprovalStatus
		class  DivergenceClass
		want   []string // what the refusal must say
	}{
		{"absent", ApprovalStatus{State: ApprovalAbsent}, DivApprovalMissing,
			[]string{`"my-skill"`, "no approval record", recordPath, "every file of the skill", approveCommandFor + "my-skill --approver"}},
		{"stale", ApprovalStatus{State: ApprovalStale, Detail: "the skill changed after approval: record digest aaa, current digest bbb"}, DivApprovalStale,
			[]string{`"my-skill"`, "stale", "record digest aaa, current digest bbb", recordPath, "every file of the skill", approveCommandFor + "my-skill --approver"}},
		{"malformed", ApprovalStatus{State: ApprovalMalformed, Detail: "parse approval record: unexpected end of JSON input"}, DivApprovalMalformed,
			[]string{`"my-skill"`, "malformed", "unexpected end of JSON input", recordPath, approveCommandFor + "my-skill --approver"}},
		{"unverifiable", ApprovalStatus{State: ApprovalUnverifiable, Detail: "references/link.md is a symbolic link"}, DivApprovalUnverifiable,
			[]string{`"my-skill"`, "cannot be verified", "references/link.md is a symbolic link", "regular files"}},
	} {
		t.Run(tc.name+" is refused with the skill, the reason and what to do", func(t *testing.T) {
			v := EvaluateApproval("my-skill", recordPath, tc.status)
			if v.OK {
				t.Fatalf("verdict = %+v, want a refusal", v)
			}
			if v.Class != tc.class {
				t.Errorf("class = %q, want %q", v.Class, tc.class)
			}
			for _, want := range tc.want {
				if !strings.Contains(v.Detail, want) {
					t.Errorf("detail %q does not contain %q", v.Detail, want)
				}
			}
		})
	}

	t.Run("a state it does not know is never an approval", func(t *testing.T) {
		for _, state := range []ApprovalState{"", "granted"} {
			if v := EvaluateApproval("my-skill", recordPath, ApprovalStatus{State: state}); v.OK {
				t.Errorf("state %q: verdict = %+v, want a refusal", state, v)
			}
		}
	})
}

// A record of version 1 is not an error to fix by hand: it is an approval of less than there is to
// approve, and the refusal says what it covered and which command approves the skill again.
func TestEvaluateApproval_ARecordOfVersionOneIsToldToBeApprovedAgain(t *testing.T) {
	recordPath := "skills/my-skill/" + ApprovalRecordName
	status := ClassifyApproval("my-skill", abcSkill(), []byte(legacyRecordJSON("my-skill", abcDigest)), true)

	v := EvaluateApproval("my-skill", recordPath, status)
	if v.OK || v.Class != DivApprovalStale {
		t.Fatalf("verdict = %+v, want APPROVAL_STALE", v)
	}
	for _, want := range []string{"version 1", "covers only SKILL.md", "every file of the skill", approveCommandFor + "my-skill --approver <name>"} {
		if !strings.Contains(v.Detail, want) {
			t.Errorf("detail %q does not say %q", v.Detail, want)
		}
	}
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
	if sum.Global != 4 || sum.Approved != 1 {
		t.Errorf("summary = %+v, want 4 global and 1 approved", sum)
	}
}

// The lint baseline names the global skills that were once approved without a record. None is now:
// each needs a valid record for the files it has, like any skill, and a skill of the baseline is
// no different in this from a skill that was never on it.
func TestCheckApprovals_NoSkillIsExcusedFromApprovalByItsId(t *testing.T) {
	f := newGateFixture(t, lintBaselineSkills, nil)

	divs, sum := CheckApprovals(f.reg, f.root, fileApprovals(os.ReadFile))

	got := classesByPath(divs)
	for _, id := range lintBaselineSkills {
		if got[id] != DivApprovalMissing {
			t.Errorf("%s: class = %q, want %q: no global skill is exempt from approval", id, got[id], DivApprovalMissing)
		}
	}
	if sum.Global != len(lintBaselineSkills) || sum.Approved != 0 {
		t.Errorf("summary = %+v, want %d global and none approved", sum, len(lintBaselineSkills))
	}
}

func TestCheckApprovals_EveryFileOfTheSkillIsPartOfWhatIsApproved(t *testing.T) {
	ids := []string{"companion-changed", "companion-added", "companion-removed", "nested-changed", "untouched"}
	f := newGateFixture(t, ids, nil)
	// Every skill has a script beside SKILL.md and a reference two directories down, all approved as
	// they are. The record each skill then has sits in its directory while the digest is checked, and
	// the digest does not cover it: a record is not a file of what it approves.
	for _, id := range ids {
		writeTestFile(t, filepath.Join(f.root, id, "scripts", "run.sh"), "echo "+id+"\n")
		writeTestFile(t, filepath.Join(f.root, id, "references", "deep", "notes.md"), "notes of "+id+"\n")
		writeValidApproval(t, f.root, id)
	}
	if divs, sum := CheckApprovals(f.reg, f.root, fileApprovals(os.ReadFile)); len(divs) != 0 || sum.Approved != len(ids) {
		t.Fatalf("before any change: divergences = %v, summary = %+v, want every skill approved", divs, sum)
	}

	writeTestFile(t, filepath.Join(f.root, "companion-changed", "scripts", "run.sh"), "echo changed\n")
	writeTestFile(t, filepath.Join(f.root, "companion-added", "scripts", "extra.sh"), "echo added\n")
	if err := os.Remove(filepath.Join(f.root, "companion-removed", "scripts", "run.sh")); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(f.root, "nested-changed", "references", "deep", "notes.md"), "notes changed\n")

	divs, sum := CheckApprovals(f.reg, f.root, fileApprovals(os.ReadFile))

	got := classesByPath(divs)
	for _, id := range []string{"companion-changed", "companion-added", "companion-removed", "nested-changed"} {
		if got[id] != DivApprovalStale {
			t.Errorf("%s: class = %q, want %q: a change to a file beside SKILL.md voids the approval", id, got[id], DivApprovalStale)
		}
	}
	if _, flagged := got["untouched"]; flagged || len(divs) != 4 {
		t.Errorf("divergences = %v, want only the four skills that changed", got)
	}
	if sum.Approved != 1 {
		t.Errorf("summary = %+v, want only the untouched skill approved", sum)
	}
}

func TestCheckApprovals_ASkillApprovedWithARecordOfVersionOneIsStaleUntilItIsApprovedAgain(t *testing.T) {
	f := newGateFixture(t, []string{"legacy"}, nil)
	// What `skills approve` wrote before the digest covered every file: the digest of SKILL.md.
	writeTestFile(t, ApprovalRecordPath(f.root, "legacy"), legacyRecordJSON("legacy", sha256hex.Sum([]byte(lintCleanSkillMD("legacy")))))

	divs, sum := CheckApprovals(f.reg, f.root, fileApprovals(os.ReadFile))

	if len(divs) != 1 || divs[0].Class != DivApprovalStale || divs[0].Path != "legacy" {
		t.Fatalf("divergences = %+v, want one APPROVAL_STALE for legacy", divs)
	}
	for _, want := range []string{"covers only SKILL.md", approveCommandFor + "legacy --approver"} {
		if !strings.Contains(divs[0].Detail, want) {
			t.Errorf("detail %q does not say %q", divs[0].Detail, want)
		}
	}
	if sum.Approved != 0 {
		t.Errorf("summary = %+v, want nothing approved by a record of version 1", sum)
	}

	writeValidApproval(t, f.root, "legacy")
	divs, sum = CheckApprovals(f.reg, f.root, fileApprovals(os.ReadFile))
	if len(divs) != 0 || sum.Approved != 1 {
		t.Fatalf("after the approval of version 2: divergences = %v, summary = %+v, want it approved", divs, sum)
	}
}

func TestCheckApprovals_ASkillWithALinkInItIsUnverifiableWhateverItsRecordSays(t *testing.T) {
	f := newGateFixture(t, []string{"linked", "dangling", "linked-skill-md"}, nil)
	for _, id := range []string{"linked", "dangling", "linked-skill-md"} {
		writeValidApproval(t, f.root, id) // approved while it was only regular files
	}
	symlinkOrSkip(t, "SKILL.md", filepath.Join(f.root, "linked", "alias.md"))
	symlinkOrSkip(t, "nothing-is-here", filepath.Join(f.root, "dangling", "broken.md"))
	skillMD := filepath.Join(f.root, "linked-skill-md", "SKILL.md")
	if err := os.Rename(skillMD, filepath.Join(f.root, "linked-skill-md", "real-skill.md")); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, "real-skill.md", skillMD)

	divs, sum := CheckApprovals(f.reg, f.root, fileApprovals(os.ReadFile))

	got := classesByPath(divs)
	for id, entry := range map[string]string{"linked": "alias.md", "dangling": "broken.md", "linked-skill-md": "SKILL.md"} {
		if got[id] != DivApprovalUnverifiable {
			t.Errorf("%s: class = %q, want %q: an approval cannot cover a link", id, got[id], DivApprovalUnverifiable)
		}
		for _, d := range divs {
			if d.Path == id && !strings.Contains(d.Detail, entry+" is a symbolic link") {
				t.Errorf("%s: detail %q does not name %s as a symbolic link", id, d.Detail, entry)
			}
		}
	}
	if sum.Approved != 0 || sum.SkillFileMissing != 0 || len(divs) != 3 {
		t.Errorf("summary = %+v with %d divergences, want none approved and none set aside", sum, len(divs))
	}
}

func TestCheckApprovals_LeavesAMissingSkillFileToTheRightCheck(t *testing.T) {
	f := newGateFixture(t, []string{"missing-file", "directory-for-file", "missing-directory"}, nil)
	if err := os.Remove(filepath.Join(f.root, "missing-file", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	// A directory where SKILL.md should be is no SKILL.md, and a directory that is not there holds none.
	skillPath := filepath.Join(f.root, "directory-for-file", "SKILL.md")
	if err := os.Remove(skillPath); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(skillPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(f.root, "missing-directory")); err != nil {
		t.Fatal(err)
	}

	divs, sum := CheckApprovals(f.reg, f.root, fileApprovals(os.ReadFile))

	if len(divs) != 0 {
		t.Errorf("divergences = %v, want none: a missing SKILL.md is already reported by the manifest and on-disk cross-checks, and is not reported twice", divs)
	}
	if sum.Global != 3 || sum.SkillFileMissing != 3 || sum.Approved != 0 {
		t.Errorf("summary = %+v, want 3 global, all 3 set aside for the lack of a SKILL.md", sum)
	}
}

// The summary must account for every global entry exactly once: it is either
// approved, reported as an approval divergence, or set aside because its
// SKILL.md does not exist (which the manifest and on-disk cross-checks report).
// An entry that fell into none of them would make the counts read as if it had
// been verified.
func TestCheckApprovals_TheSummaryAccountsForEveryGlobalEntryExactlyOnce(t *testing.T) {
	f := newGateFixture(t, []string{"approved", "unapproved", "no-file", "also-no-file"}, []string{"project-skill"})
	writeValidApproval(t, f.root, "approved")
	for _, id := range []string{"no-file", "also-no-file"} {
		if err := os.Remove(filepath.Join(f.root, id, "SKILL.md")); err != nil {
			t.Fatal(err)
		}
	}

	divs, sum := CheckApprovals(f.reg, f.root, fileApprovals(os.ReadFile))

	if sum.Global != 4 {
		t.Errorf("Global = %d, want 4: project-tier entries are not counted", sum.Global)
	}
	if sum.Approved != 1 || len(divs) != 1 || sum.SkillFileMissing != 2 {
		t.Errorf("summary = %+v with %d divergences, want 1 approved, 1 divergence, 2 without a SKILL.md", sum, len(divs))
	}
	if accounted := sum.Approved + sum.SkillFileMissing + len(divs); accounted != sum.Global {
		t.Errorf("the summary accounts for %d of %d global entries: %+v with %d divergences", accounted, sum.Global, sum, len(divs))
	}
	if got := classesByPath(divs); got["unapproved"] != DivApprovalMissing || len(got) != 1 {
		t.Errorf("divergences = %v, want only unapproved as APPROVAL_MISSING: a missing SKILL.md is not an approval finding", got)
	}
}

func TestCheckApprovals_AnUnreadableRecordIsUnverifiableNotAbsent(t *testing.T) {
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

// ---- the lint baseline ---------------------------------------------------------

func TestLintBaseline_IsWellFormedAndFixed(t *testing.T) {
	// The count is pinned: adding to or removing from the list is a deliberate, reviewed change that
	// must update this number with it.
	const lintBaselineSize = 34
	if len(lintBaselineSkills) != lintBaselineSize {
		t.Fatalf("the lint baseline has %d ids, want the fixed %d", len(lintBaselineSkills), lintBaselineSize)
	}
	if !sort.StringsAreSorted(lintBaselineSkills) {
		t.Error("the lint baseline ids must be sorted")
	}
	seen := map[string]bool{}
	for _, id := range lintBaselineSkills {
		if seen[id] {
			t.Errorf("duplicate lint baseline id %q", id)
		}
		seen[id] = true
		if !slugRe.MatchString(id) {
			t.Errorf("lint baseline id %q is not a valid skill slug", id)
		}
	}
}

func TestBaselineLintDecision(t *testing.T) {
	const baselineID, otherID = "sdd-apply", "my-new-skill"
	budget := &LintError{Rule: "body-hard-budget", Msg: "body is 1200 estimated tokens"}
	description := &LintError{Rule: "description-max", Msg: "description is 300 characters"}
	structural := &LintError{Rule: "frontmatter-fence", Msg: "no front matter"}

	for _, tc := range []struct {
		name         string
		id           string
		hard         []error
		wantRefused  bool
		wantWarnings []string
	}{
		{"no findings", baselineID, nil, false, nil},
		{"no findings on a skill outside the baseline", otherID, nil, false, nil},
		{"a legacy finding of a baseline skill is a warning", baselineID, []error{budget}, false,
			[]string{"warning: [lint:body-hard-budget] body is 1200 estimated tokens (baseline skill: approved with lint findings)"}},
		{"each legacy finding is a warning of its own", baselineID, []error{budget, description}, false,
			[]string{
				"warning: [lint:body-hard-budget] body is 1200 estimated tokens (baseline skill: approved with lint findings)",
				"warning: [lint:description-max] description is 300 characters (baseline skill: approved with lint findings)",
			}},
		{"a legacy finding refuses a skill outside the baseline", otherID, []error{budget}, true, nil},
		{"a structural finding refuses a baseline skill", baselineID, []error{structural}, true, nil},
		{"one structural finding among legacy ones refuses a baseline skill", baselineID, []error{budget, structural}, true, nil},
		{"an error that is no lint finding is structural", baselineID, []error{errors.New("boom")}, true, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			warnings, refused := BaselineLintDecision(tc.id, tc.hard)
			if refused != tc.wantRefused {
				t.Errorf("refused = %v, want %v", refused, tc.wantRefused)
			}
			if strings.Join(warnings, "\n") != strings.Join(tc.wantWarnings, "\n") {
				t.Errorf("warnings = %q, want %q", warnings, tc.wantWarnings)
			}
		})
	}
}
