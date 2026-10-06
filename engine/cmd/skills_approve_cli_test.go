package main

// Tests of what `skills approve` tells and refuses, through its adapter: the words of every
// refusal of a command line or of an approval, the output of an approval, and the warnings of a
// skill of the approval baseline. What the use case decides (the record, the digest, the order of
// its checks) is tested in engine/skills/app.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// baselineSkillID is a real member of the approval baseline: which ids are exempt from the lint
// refusal is the point of the exemption, and the list is fixed data.
const baselineSkillID = "sdd-apply"

// overBudgetSkillMD is lint-clean front matter and sections followed by enough body to fail the
// hard token budget, which is the finding the baseline skills have.
func overBudgetSkillMD(id string) string {
	return overlaySkillMD(id) + strings.Repeat("Another line of the procedure, long enough to count.\n", 100)
}

// approveEnv is a skills tree that holds one skill and no record of its approval.
type approveEnv struct {
	t    *testing.T
	root string
	id   string
	deps skills.Deps
	now  func() string
}

// anyRegistryLocker takes no lock and says every registry is there: approve reads no registry, so
// these tests give it none, and the lock it would take on the default one is not their subject.
type anyRegistryLocker struct{ noopOverlayLocker }

func (anyRegistryLocker) Exists(string) error { return nil }

func newApproveEnvWith(t *testing.T, id, content string) approveEnv {
	t.Helper()
	dir := t.TempDir()
	w := overlayWorld{t: t, dir: dir, reg: filepath.Join(dir, "registry.yaml"), root: filepath.Join(dir, "skills")}
	writeTestFile(t, filepath.Join(w.root, id, "SKILL.md"), content)
	return approveEnv{t: t, root: w.root, id: id, deps: w.deps(anyRegistryLocker{}, os.ReadFile), now: overlayClock}
}

func newApproveEnv(t *testing.T, id string) approveEnv {
	t.Helper()
	return newApproveEnvWith(t, id, overlaySkillMD(id))
}

func (e approveEnv) recordPath() string { return skills.ApprovalRecordPath(e.root, e.id) }

func (e approveEnv) args(extra ...string) []string {
	return append([]string{"approve", "--id", e.id, "--approver", "reviewer", "--source-root", e.root}, extra...)
}

func (e approveEnv) run(args []string) verbRun {
	e.t.Helper()
	deps := e.deps
	deps.Now = e.now
	return runSkillsVerb(skillsApprove, deps, args...)
}

func (e approveEnv) assertNoRecord() {
	e.t.Helper()
	if _, err := os.Stat(e.recordPath()); err == nil {
		e.t.Errorf("no record may be written, but %s exists", e.recordPath())
	}
}

func (e approveEnv) withNow(now func() string) approveEnv { e.now = now; return e }

func TestApproveTellsTheApprovalOfTheExactBytes(t *testing.T) {
	e := newApproveEnv(t, "my-skill")
	r := e.run(e.args())
	digest := skills.SkillDigest([]byte(overlaySkillMD("my-skill")))
	want := "approved: my-skill\nsha256: " + digest + "\nrecord: " + filepath.ToSlash(e.recordPath()) + "\n"
	if r.code() != 0 || r.stdout != want || r.stderr != "" {
		t.Errorf("approve = exit %d, %q, %q, want %q", r.code(), r.stdout, r.stderr, want)
	}
	// The labdrian wrapper appends --registry and --manifest after every verb's own arguments;
	// approve needs neither and does not choke on them. The registry only names the lock.
	reg := filepath.Join(filepath.Dir(e.root), "skills.registry.yaml")
	again := e.run(e.args("--registry", reg, "--manifest", "/unused/overlay.manifest"))
	if again.code() != 0 || !strings.HasPrefix(again.stdout, "unchanged: my-skill\n") {
		t.Errorf("approve with the flags of the wrapper = exit %d, %q, %q", again.code(), again.stdout, again.stderr)
	}
}

func TestApproveRefusesWithoutWritingAnything(t *testing.T) {
	tests := []struct {
		name string
		args func(e approveEnv) []string
		now  func() string
		want string // substring of stderr
	}{
		{"missing --id", func(e approveEnv) []string { return []string{"approve", "--approver", "r", "--source-root", e.root} }, overlayClock, "--id"},
		{"missing --approver", func(e approveEnv) []string { return []string{"approve", "--id", e.id, "--source-root", e.root} }, overlayClock, "--approver"},
		{"missing --source-root", func(e approveEnv) []string { return []string{"approve", "--id", e.id, "--approver", "r"} }, overlayClock, "--source-root"},
		{"blank approver", func(e approveEnv) []string { return e.args("--approver", "   ") }, overlayClock, "approver"},
		{"empty approver", func(e approveEnv) []string {
			return []string{"approve", "--id", e.id, "--approver", "", "--source-root", e.root}
		}, overlayClock, "approver"},
		{"approver with a control character", func(e approveEnv) []string { return e.args("--approver", "a\tb") }, overlayClock, "approver"},
		{"invalid id slug", func(e approveEnv) []string {
			return []string{"approve", "--id", "../escape", "--approver", "r", "--source-root", e.root}
		}, overlayClock, "invalid"},
		{"no clock configured", func(e approveEnv) []string { return e.args() }, nil, "clock"},
		{"clock yields a malformed timestamp", func(e approveEnv) []string { return e.args() }, func() string { return "yesterday" }, "approved_at"},
		{"unknown flag", func(e approveEnv) []string { return e.args("--force") }, overlayClock, "--force"},
		{"flag without a value", func(e approveEnv) []string { return []string{"approve", "--id"} }, overlayClock, "--id"},
		{"flag value that is a flag", func(e approveEnv) []string { return []string{"approve", "--id", "--approver", "r"} }, overlayClock, "--id"},
		{"stray positional", func(e approveEnv) []string { return e.args("extra") }, overlayClock, "extra"},
		// approve takes no positional argument, so an end-of-options marker would have nothing to
		// protect: it is an unknown flag like any other.
		{"end-of-options marker", func(e approveEnv) []string { return e.args("--") }, overlayClock, `unknown flag "--"`},
		{"end-of-options marker before the flags", func(e approveEnv) []string { return append([]string{"approve", "--"}, e.args()[1:]...) }, overlayClock, `unknown flag "--"`},
		// The refusal of a value that begins with "-" is deliberate, and for the approver it says
		// why in its own words.
		{"approver that begins with a dash", func(e approveEnv) []string { return e.args("--approver", "-jo") }, overlayClock, `--approver`},
		{"approver that is a lone dash", func(e approveEnv) []string { return e.args("--approver", "-") }, overlayClock, `starts with "-"`},
		{"approver of one rune over the bound", func(e approveEnv) []string {
			return e.args("--approver", strings.Repeat("é", skills.ApprovalApproverMaxRunes+1))
		}, overlayClock, "exceeds the bound"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newApproveEnv(t, "my-skill").withNow(tc.now)
			r := e.run(tc.args(e))
			if r.code() != 1 || r.stdout != "" || !strings.Contains(r.stderr, tc.want) {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, nothing on stdout, and %q on stderr", r.code(), r.stdout, r.stderr, tc.want)
			}
			e.assertNoRecord()
		})
	}
}

func TestApproveTellsWhyASkillIsRefused(t *testing.T) {
	e := newApproveEnv(t, "my-skill")
	if err := os.Remove(filepath.Join(e.root, "my-skill", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if r := e.run(e.args()); r.code() != 1 || !strings.Contains(r.stderr, "my-skill") || !strings.Contains(r.stderr, "SKILL.md") {
		t.Errorf("a skill that is not there: exit %d, stderr %q, want it to name the skill and SKILL.md", r.code(), r.stderr)
	}
	writeTestFile(t, filepath.Join(e.root, "my-skill", "SKILL.md"), "no frontmatter here\n")
	if r := e.run(e.args()); r.code() != 1 || !strings.Contains(r.stderr, "[lint:") || strings.Contains(r.stderr, "error:") {
		t.Errorf("a skill that fails the hard lint: exit %d, stderr %q, want the findings as the lint prints them", r.code(), r.stderr)
	}
	e.assertNoRecord()
	// A directory where the record is makes the read fail with something other than "does not
	// exist": it is refused, not overwritten.
	writeTestFile(t, filepath.Join(e.root, "my-skill", "SKILL.md"), overlaySkillMD("my-skill"))
	if err := os.MkdirAll(e.recordPath(), 0o755); err != nil {
		t.Fatal(err)
	}
	if r := e.run(e.args()); r.code() != 1 || !strings.Contains(r.stderr, "approval record") {
		t.Errorf("a record that cannot be read: exit %d, stderr %q, want it to name the approval record", r.code(), r.stderr)
	}
	if info, err := os.Stat(e.recordPath()); err != nil || !info.IsDir() {
		t.Error("the unreadable record must be left untouched")
	}
}

func TestApproveTellsAWriteThatFailedAndLeavesNoRecord(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("a directory without write permission does not stop root")
	}
	e := newApproveEnv(t, "my-skill")
	dir := filepath.Join(e.root, e.id)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	r := e.run(e.args())
	if r.code() != 1 || !strings.Contains(r.stderr, "writing approval record") {
		t.Errorf("exit %d, stderr %q, want the failed write named", r.code(), r.stderr)
	}
	e.assertNoRecord()
}

// The baseline skills predate the lint budget, and most of them fail the hard lint. A byte change
// to one of them, an upstream merge included, makes `skills validate` ask for an approval, so
// `skills approve` must be able to give it: it tells every hard finding as a warning, after the
// approval, and records the approval of those exact bytes.
func TestApproveTellsTheFindingsOfABaselineSkillAsWarningsAfterTheApproval(t *testing.T) {
	e := newApproveEnvWith(t, baselineSkillID, overBudgetSkillMD(baselineSkillID))
	r := e.run(e.args())
	if r.code() != 0 || !strings.HasPrefix(r.stdout, "approved: "+baselineSkillID+"\n") {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.code(), r.stdout, r.stderr)
	}
	lines := strings.Split(strings.TrimSuffix(r.stderr, "\n"), "\n")
	const prefix = "warning: [lint:body-hard-budget] body is an estimated "
	const suffix = " tokens, exceeds the hard budget of 1000 (baseline skill: approved with lint findings)"
	if len(lines) != 1 || !strings.HasPrefix(lines[0], prefix) || !strings.HasSuffix(lines[0], suffix) {
		t.Errorf("stderr = %q, want exactly one warning that starts with %q and ends with %q", r.stderr, prefix, suffix)
	}
	// The findings are a fact about the bytes, and are told on every run, even when the record is
	// left as it is.
	again := e.run(e.args("--approver", "someone-else"))
	if !strings.HasPrefix(again.stdout, "unchanged: "+baselineSkillID+"\n") || !strings.Contains(again.stderr, "warning: [lint:body-hard-budget]") {
		t.Errorf("re-approving = %q, %q, want it unchanged and the findings told again", again.stdout, again.stderr)
	}
}

func TestApproveRefusesTheSameFindingsOfASkillOutsideTheBaselineWithoutWarnings(t *testing.T) {
	for _, id := range []string{"my-skill", baselineSkillID + "-copy"} {
		t.Run(id, func(t *testing.T) {
			e := newApproveEnvWith(t, id, overBudgetSkillMD(id))
			r := e.run(e.args())
			if r.code() != 1 || r.stdout != "" || !strings.Contains(r.stderr, "[lint:body-hard-budget]") {
				t.Errorf("exit %d, stdout %q, stderr %q, want the finding refused", r.code(), r.stdout, r.stderr)
			}
			if strings.Contains(r.stderr, "warning:") || strings.Contains(r.stderr, "baseline") {
				t.Errorf("stderr %q treats a skill outside the baseline as a baseline one", r.stderr)
			}
			e.assertNoRecord()
		})
	}
}

// A warning says the bytes are approved. When the approval does not happen, nothing says so.
func TestApproveTellsNoWarningForAnApprovalThatDoesNotHappen(t *testing.T) {
	t.Run("the existing record cannot be read", func(t *testing.T) {
		e := newApproveEnvWith(t, baselineSkillID, overBudgetSkillMD(baselineSkillID))
		if err := os.MkdirAll(e.recordPath(), 0o755); err != nil {
			t.Fatal(err)
		}
		if r := e.run(e.args()); r.code() != 1 || r.stdout != "" || strings.Contains(r.stderr, "warning:") {
			t.Errorf("exit %d, stdout %q, stderr %q, want a refusal with no warning", r.code(), r.stdout, r.stderr)
		}
	})
	t.Run("the approver label is refused", func(t *testing.T) {
		e := newApproveEnvWith(t, baselineSkillID, overBudgetSkillMD(baselineSkillID))
		if r := e.run(e.args("--approver", "   ")); r.code() != 1 || strings.Contains(r.stderr, "warning:") {
			t.Errorf("exit %d, stderr %q, want a refusal with no warning", r.code(), r.stderr)
		}
		e.assertNoRecord()
	})
}
