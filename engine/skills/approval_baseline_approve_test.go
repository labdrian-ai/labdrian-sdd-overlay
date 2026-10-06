package skills

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The baseline skills predate the lint budget, and most of them fail the hard lint
// (body past 1,000 estimated tokens, a multi-line description). A byte change to one
// of them, an upstream merge included, makes `skills validate` ask for an approval,
// so `skills approve` must be able to give it: for a skill in the approval baseline
// it reports every hard finding as a warning and records the approval of those exact
// bytes. A skill outside the baseline still needs a clean lint.

// baselineWarningSuffix is the tail of every warning line. The whole wording is
// pinned by TestApprove_TheBaselineWarningWording.
const baselineWarningSuffix = " (baseline skill: approved with lint findings)"

// baselineSkillID is a real member of the approval baseline. The tests that use it
// keep the real list in place: which ids are exempt from the lint refusal is the
// point of the change, and the list is fixed data.
const baselineSkillID = "sdd-apply"

// overBudgetSkillMD is lint-clean front matter and sections followed by enough body
// to pass the hard token budget.
func overBudgetSkillMD(id string) string {
	return lintCleanSkillMD(id) + strings.Repeat("Another line of the procedure, long enough to count.\n", 100)
}

// multiLineDescriptionSkillMD has a block-scalar description, a hard finding of its own.
func multiLineDescriptionSkillMD(id string) string {
	return "---\n" +
		"name: " + id + "\n" +
		"description: >\n" +
		"  A description that runs\n" +
		"  over two physical lines.\n" +
		"license: MIT\n" +
		"metadata:\n" +
		"  author: tester\n" +
		"  version: \"1.0\"\n" +
		"---\n" +
		"## Activation Contract\n" +
		"Load this skill for its documented procedure.\n"
}

// hardLintShapes are the ways a baseline skill fails the hard lint today (the
// legacy findings: body over the budget, description too long or on several
// lines). A baseline skill is approved with them as warnings.
var hardLintShapes = []struct {
	name string
	md   func(id string) string
}{
	{"body over the hard budget", overBudgetSkillMD},
	{"multi-line description", multiLineDescriptionSkillMD},
	{"multi-line description and a body over the budget", func(id string) string {
		return multiLineDescriptionSkillMD(id) + strings.Repeat("Another line of the procedure, long enough to count.\n", 100)
	}},
}

// structuralLintShapes are hard findings that mean the file is not a usable skill
// (for example a merge that truncated it). They refuse a baseline skill too:
// the exemption covers only the legacy findings.
var structuralLintShapes = []struct {
	name string
	md   func(id string) string
}{
	{"no front matter", func(string) string { return "no frontmatter here\n" }},
	{"no front matter beside a body over the budget", func(string) string {
		return strings.Repeat("Another line of the procedure, long enough to count.\n", 100)
	}},
}

func TestApprove_ABaselineSkillWithAStructuralLintFindingIsRefused(t *testing.T) {
	for _, shape := range structuralLintShapes {
		t.Run(shape.name, func(t *testing.T) {
			md := shape.md(baselineSkillID)
			hardFindingLines(t, md)
			e := newApproveEnvWith(t, baselineSkillID, md)

			_, stderr, code := runApprove(e.args(), fixedClock(approveFixedNow))

			if code != 1 {
				t.Fatalf("exit = %d, want 1 (refused); stderr=%q", code, stderr)
			}
			if strings.Contains(stderr, baselineWarningSuffix) {
				t.Errorf("stderr = %q: a structural finding must refuse, not warn", stderr)
			}
			if _, err := os.Stat(e.recordPath()); !os.IsNotExist(err) {
				t.Errorf("an approval record was written (stat err = %v); a refusal writes nothing", err)
			}
		})
	}
}

// hardFindingLines are the lines of the hard lint findings of md, in lint order.
func hardFindingLines(t *testing.T, md string) []string {
	t.Helper()
	hard, _ := LintSkillFile([]byte(md))
	if len(hard) == 0 {
		t.Fatalf("the fixture must fail the hard lint:\n%s", md)
	}
	lines := make([]string, 0, len(hard))
	for _, finding := range hard {
		lines = append(lines, finding.Error())
	}
	return lines
}

// newApproveEnvWith is an approve fixture holding one skill with the given bytes.
func newApproveEnvWith(t *testing.T, id, md string) approveEnv {
	t.Helper()
	root := filepath.Join(t.TempDir(), "skills")
	writeTestFile(t, filepath.Join(root, id, "SKILL.md"), md)
	return approveEnv{root: root, id: id}
}

func TestApprove_ABaselineSkillWithHardLintFindingsIsApprovedWithWarnings(t *testing.T) {
	for _, shape := range hardLintShapes {
		t.Run(shape.name, func(t *testing.T) {
			md := shape.md(baselineSkillID)
			findings := hardFindingLines(t, md)
			e := newApproveEnvWith(t, baselineSkillID, md)

			stdout, stderr, code := runApprove(e.args(), fixedClock(approveFixedNow))

			if code != 0 {
				t.Fatalf("exit = %d, want 0; stderr=%q", code, stderr)
			}
			// One warning line per hard finding, in lint order, and nothing else on stderr.
			want := ""
			for _, finding := range findings {
				want += "warning: " + finding + baselineWarningSuffix + "\n"
			}
			if stderr != want {
				t.Errorf("stderr = %q, want %q", stderr, want)
			}
			if !strings.HasPrefix(stdout, "approved: "+baselineSkillID+"\n") {
				t.Errorf("stdout = %q, want it to start with %q", stdout, "approved: "+baselineSkillID+"\n")
			}
			// The approval is of these exact bytes, in the unchanged record format.
			skill, _ := os.ReadFile(e.skillPath())
			rec := readRecord(t, e.recordPath())
			wantRec := ApprovalRecord{Version: 1, Skill: baselineSkillID, SHA256: SkillDigest(skill), ApprovedAt: approveFixedNow, Approver: "reviewer"}
			if rec != wantRec {
				t.Errorf("record = %+v, want %+v", rec, wantRec)
			}
		})
	}
}

func TestApprove_TheBaselineWarningWording(t *testing.T) {
	e := newApproveEnvWith(t, baselineSkillID, overBudgetSkillMD(baselineSkillID))
	_, stderr, code := runApprove(e.args(), fixedClock(approveFixedNow))
	if code != 0 {
		t.Fatalf("exit = %d; stderr=%q", code, stderr)
	}
	lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("stderr = %q, want exactly one warning line for one finding", stderr)
	}
	const prefix = "warning: [lint:body-hard-budget] body is an estimated "
	const suffix = " tokens, exceeds the hard budget of 1000 (baseline skill: approved with lint findings)"
	if !strings.HasPrefix(lines[0], prefix) || !strings.HasSuffix(lines[0], suffix) {
		t.Errorf("warning line = %q, want it to start with %q and end with %q", lines[0], prefix, suffix)
	}
}

func TestApprove_ANonBaselineSkillWithTheSameFindingsIsStillRefused(t *testing.T) {
	for _, shape := range hardLintShapes {
		t.Run(shape.name, func(t *testing.T) {
			// The baseline is per skill id: the same bytes under another id are refused,
			// and so is an id that only looks like a baseline one.
			for _, id := range []string{"my-skill", baselineSkillID + "-copy"} {
				md := shape.md(id)
				findings := hardFindingLines(t, md)
				e := newApproveEnvWith(t, id, md)

				stdout, stderr, code := runApprove(e.args(), fixedClock(approveFixedNow))

				if code != 1 {
					t.Fatalf("%s: exit = %d, want 1; stderr=%q", id, code, stderr)
				}
				for _, finding := range findings {
					if !strings.Contains(stderr, finding) {
						t.Errorf("%s: stderr %q does not carry the finding %q", id, stderr, finding)
					}
				}
				if strings.Contains(stderr, "warning:") || strings.Contains(stderr, "baseline") {
					t.Errorf("%s: stderr %q treats a non-baseline skill as a baseline one", id, stderr)
				}
				if stdout != "" {
					t.Errorf("%s: a refusal prints nothing on stdout, got %q", id, stdout)
				}
				assertNoRecord(t, e)
			}
		})
	}
}

func TestApprove_AnUnchangedBaselineSkillIsApprovedToo(t *testing.T) {
	// "Unchanged" means the bytes are the grandfathered ones, so validate needs no
	// record. Approving them is still allowed, for the human who wants one, and it
	// reports the findings the same way.
	md := overBudgetSkillMD("legacy")
	e := newApproveEnvWith(t, "legacy", md)
	setBaselineForTest(t, []ApprovalBaselineEntry{{ID: "legacy", SHA256: SkillDigest([]byte(md))}})
	reg := Registry{Version: "1", Skills: []Entry{{ID: "legacy", Path: "legacy", Install: Install{DefaultScope: "global"}}}}
	if divs, sum := CheckApprovals(reg, e.root, os.ReadFile); len(divs) != 0 || sum.Grandfathered != 1 {
		t.Fatalf("before approve: divs=%v summary=%+v, want the skill grandfathered", divs, sum)
	}

	_, stderr, code := runApprove(e.args(), fixedClock(approveFixedNow))

	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stderr, "warning: [lint:body-hard-budget]") {
		t.Errorf("stderr %q does not carry the finding as a warning", stderr)
	}
	if divs, sum := CheckApprovals(reg, e.root, os.ReadFile); len(divs) != 0 || sum.Approved != 1 || sum.Grandfathered != 0 {
		t.Errorf("after approve: divs=%v summary=%+v, want the skill approved by its record", divs, sum)
	}
}

func TestApprove_ACleanBaselineSkillHasNothingToWarnAbout(t *testing.T) {
	e := newApproveEnvWith(t, baselineSkillID, lintCleanSkillMD(baselineSkillID))
	stdout, stderr, code := runApprove(e.args(), fixedClock(approveFixedNow))
	if code != 0 || stderr != "" {
		t.Fatalf("exit = %d, stderr = %q, want 0 and nothing on stderr; stdout=%q", code, stderr, stdout)
	}
}

func TestApprove_OnlyHardFindingsAreWarnedAbout(t *testing.T) {
	// A long description is an advisory finding: it is not a lint refusal anywhere, so
	// it is not repeated as a baseline warning either.
	md := overBudgetSkillMD(baselineSkillID)
	md = strings.Replace(md, "description: A concise procedural skill for "+baselineSkillID+".",
		"description: "+strings.Repeat("A long description. ", 12), 1)
	hard, warns := LintSkillFile([]byte(md))
	if len(hard) == 0 || len(warns) == 0 {
		t.Fatalf("fixture must have hard findings and advisory warnings: hard=%v warns=%v", hard, warns)
	}
	e := newApproveEnvWith(t, baselineSkillID, md)

	_, stderr, code := runApprove(e.args(), fixedClock(approveFixedNow))

	if code != 0 {
		t.Fatalf("exit = %d; stderr=%q", code, stderr)
	}
	if got := strings.Count(stderr, "warning:"); got != len(hard) {
		t.Errorf("%d warning lines, want %d (one per hard finding); stderr=%q", got, len(hard), stderr)
	}
	for _, w := range warns {
		if strings.Contains(stderr, w.Msg) {
			t.Errorf("stderr %q repeats the advisory warning %q", stderr, w.Msg)
		}
	}
}

func TestApprove_ReapprovingABaselineSkillIsIdempotentAndStillShowsItsFindings(t *testing.T) {
	e := newApproveEnvWith(t, baselineSkillID, overBudgetSkillMD(baselineSkillID))
	if _, stderr, code := runApprove(e.args(), fixedClock(approveFixedNow)); code != 0 {
		t.Fatalf("first approve: exit %d; %q", code, stderr)
	}
	first, _ := os.ReadFile(e.recordPath())

	args := []string{"--id", e.id, "--approver", "someone-else", "--source-root", e.root}
	stdout, stderr, code := runApprove(args, fixedClock("2027-01-01T00:00:00Z"))

	if code != 0 {
		t.Fatalf("second approve: exit %d; %q", code, stderr)
	}
	if second, _ := os.ReadFile(e.recordPath()); !bytes.Equal(first, second) {
		t.Errorf("re-approving identical bytes rewrote the record:\n%s\n---\n%s", first, second)
	}
	if !strings.HasPrefix(stdout, "unchanged: "+baselineSkillID+"\n") {
		t.Errorf("stdout = %q, want it to start with %q", stdout, "unchanged: "+baselineSkillID+"\n")
	}
	if !strings.Contains(stderr, "warning: [lint:body-hard-budget]") {
		t.Errorf("stderr %q: the findings are a fact about the bytes and are reported on every run", stderr)
	}
}

func TestApprove_NoWarningIsPrintedForAnApprovalThatDoesNotHappen(t *testing.T) {
	// A warning says the bytes are approved. When the approval fails, nothing may say so.
	t.Run("the record cannot be written", func(t *testing.T) {
		e := newApproveEnvWith(t, baselineSkillID, overBudgetSkillMD(baselineSkillID))
		denyWrites(t, filepath.Join(e.root, e.id))
		stdout, stderr, code := runApprove(e.args(), fixedClock(approveFixedNow))
		if code != 1 || stdout != "" {
			t.Fatalf("exit = %d, stdout = %q, want a refusal with nothing on stdout; stderr=%q", code, stdout, stderr)
		}
		if strings.Contains(stderr, "warning:") {
			t.Errorf("stderr %q carries a warning for an approval that did not happen", stderr)
		}
		assertNoRecord(t, e)
	})
	t.Run("the existing record cannot be read", func(t *testing.T) {
		e := newApproveEnvWith(t, baselineSkillID, overBudgetSkillMD(baselineSkillID))
		if err := os.MkdirAll(e.recordPath(), 0o755); err != nil {
			t.Fatal(err)
		}
		stdout, stderr, code := runApprove(e.args(), fixedClock(approveFixedNow))
		if code != 1 || stdout != "" {
			t.Fatalf("exit = %d, stdout = %q, want a refusal with nothing on stdout; stderr=%q", code, stdout, stderr)
		}
		if strings.Contains(stderr, "warning:") {
			t.Errorf("stderr %q carries a warning for an approval that did not happen", stderr)
		}
	})
	t.Run("the approver label is refused", func(t *testing.T) {
		e := newApproveEnvWith(t, baselineSkillID, overBudgetSkillMD(baselineSkillID))
		_, stderr, code := runApprove(e.args("--approver", "   "), fixedClock(approveFixedNow))
		if code != 1 || strings.Contains(stderr, "warning:") {
			t.Fatalf("exit = %d, stderr = %q, want a refusal without a warning", code, stderr)
		}
		assertNoRecord(t, e)
	})
}

// This is the scenario an upstream merge creates: a baseline skill that fails the hard
// lint and whose bytes changed. validate asks for an approval; approve gives it; validate
// then passes.
func TestBaselineSkillThatFailsTheHardLint_ValidateAsksForApprovalAndApproveResolvesIt(t *testing.T) {
	dir := t.TempDir()
	regPath, mfPath, root := setupFixtureWithoutApprovals(t, dir, minimalRegistry(baselineSkillID), minimalManifest(baselineSkillID), []string{baselineSkillID})
	writeTestFile(t, filepath.Join(root, baselineSkillID, "SKILL.md"), overBudgetSkillMD(baselineSkillID))
	validate := func() (string, string, int) {
		var out, errBuf bytes.Buffer
		code := 0 // validate calls exit only to fail
		RenderValidateCore([]string{"--registry", regPath, "--manifest", mfPath, "--source-root", root}, os.ReadFile, testRegistries(os.ReadFile), scanSkillFiles, &out, &errBuf, func(c int) { code = c })
		return out.String(), errBuf.String(), code
	}

	_, stderr, code := validate()
	if code != 1 || !strings.Contains(stderr, "[APPROVAL_MISSING] "+baselineSkillID) || !strings.Contains(stderr, "differs from the grandfathered baseline") {
		t.Fatalf("validate before approve: exit %d, stderr %q, want APPROVAL_MISSING naming the changed baseline skill", code, stderr)
	}

	approveArgs := []string{"--id", baselineSkillID, "--approver", "reviewer", "--source-root", root}
	if _, stderr, code := runApprove(approveArgs, fixedClock(approveFixedNow)); code != 0 || !strings.Contains(stderr, "warning: [lint:body-hard-budget]") {
		t.Fatalf("approve: exit %d, stderr %q, want success with the finding as a warning", code, stderr)
	}

	stdout, stderr, code := validate()
	if code != 0 {
		t.Fatalf("validate after approve: exit %d, stderr %q, want 0", code, stderr)
	}
	if !strings.Contains(stdout, "global skill approvals verified (1 skills: 1 approved, 0 grandfathered)") {
		t.Errorf("stdout %q must say the skill is approved by its record", stdout)
	}
}

// skills add is deliberately unchanged: it still refuses a hard lint finding, so a
// baseline skill that fails the lint cannot be registered again by add, approved or
// not. Baseline skills are already registered, so the only way to meet this is to
// remove one and add it back; the rewrite of these skills (which brings each one
// within the budget) is the way out. This test states the limit.
func TestAddCore_ABaselineSkillThatFailsTheHardLintStillCannotBeAddedEvenWhenApproved(t *testing.T) {
	dir := t.TempDir()
	regPath, mfPath, root := setupFixtureWithoutApprovals(t, dir, minimalRegistry("existing"), minimalManifest("existing"), []string{"existing", baselineSkillID})
	writeTestFile(t, filepath.Join(root, baselineSkillID, "SKILL.md"), overBudgetSkillMD(baselineSkillID))
	if _, stderr, code := runApprove([]string{"--id", baselineSkillID, "--approver", "reviewer", "--source-root", root}, fixedClock(approveFixedNow)); code != 0 {
		t.Fatalf("approve: exit %d; %q", code, stderr)
	}
	before := snapshotFiles(t, regPath, mfPath)

	r := runAdd(t, regPath, mfPath, root, baselineSkillID)

	if r.code != 1 || !strings.Contains(r.stderr, "[lint:body-hard-budget]") {
		t.Fatalf("add: exit %d, stderr %q, want a refusal with the lint finding", r.code, r.stderr)
	}
	for p, b := range snapshotFiles(t, regPath, mfPath) {
		if before[p] != b {
			t.Errorf("%s changed on a refused add", p)
		}
	}
}
