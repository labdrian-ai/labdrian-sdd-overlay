package skills

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const approveFixedNow = "2026-09-30T12:00:00Z"

func fixedClock(ts string) func() string { return func() string { return ts } }

// approveEnv is one temp skills tree holding a single lint-clean skill.
type approveEnv struct {
	root string // the --source-root
	id   string
}

func newApproveEnv(t *testing.T, id string) approveEnv {
	t.Helper()
	root := filepath.Join(t.TempDir(), "skills")
	writeTestFile(t, filepath.Join(root, id, "SKILL.md"), string(validDraft(id)))
	return approveEnv{root: root, id: id}
}

func (e approveEnv) skillPath() string  { return filepath.Join(e.root, e.id, "SKILL.md") }
func (e approveEnv) recordPath() string { return ApprovalRecordPath(e.root, e.id) }

func (e approveEnv) args(extra ...string) []string {
	return append([]string{"--id", e.id, "--approver", "reviewer", "--source-root", e.root}, extra...)
}

// runApprove drives RenderApproveCore and returns stdout, stderr and the exit
// code (-1 when exit was never called).
func runApprove(args []string, now func() string) (stdout, stderr string, code int) {
	var out, errBuf bytes.Buffer
	code = -1
	RenderApproveCore(args, os.ReadFile, now, &out, &errBuf, func(c int) { code = c })
	return out.String(), errBuf.String(), code
}

func readRecord(t *testing.T, path string) ApprovalRecord {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read record %q: %v", path, err)
	}
	rec, err := ParseApprovalRecord(data)
	if err != nil {
		t.Fatalf("record on disk does not parse: %v\n%s", err, data)
	}
	return rec
}

func assertNoRecord(t *testing.T, e approveEnv) {
	t.Helper()
	if _, err := os.Stat(e.recordPath()); err == nil {
		t.Errorf("no record may be written, but %s exists", e.recordPath())
	}
}

func TestApprove_WritesARecordBoundToTheExactBytes(t *testing.T) {
	e := newApproveEnv(t, "my-skill")

	stdout, stderr, code := runApprove(e.args(), fixedClock(approveFixedNow))
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", code, stderr)
	}

	skill, _ := os.ReadFile(e.skillPath())
	rec := readRecord(t, e.recordPath())
	want := ApprovalRecord{
		Version:    1,
		Skill:      "my-skill",
		SHA256:     SkillDigest(skill),
		ApprovedAt: approveFixedNow,
		Approver:   "reviewer",
	}
	if rec != want {
		t.Fatalf("record = %+v, want %+v", rec, want)
	}

	wantOut := "approved: my-skill\n" +
		"sha256: " + SkillDigest(skill) + "\n" +
		"record: " + filepath.ToSlash(e.recordPath()) + "\n"
	if stdout != wantOut {
		t.Errorf("stdout = %q, want %q", stdout, wantOut)
	}

	// The record is committed with the skill, so it must be a normal readable
	// file, and the atomic write must leave no temp file behind.
	info, err := os.Stat(e.recordPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("record mode = %v, want 0644", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(filepath.Join(e.root, e.id))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".tmp-") {
			t.Errorf("stray temp file %q left in the skill directory", entry.Name())
		}
	}
}

func TestApprove_TrimsTheApproverLabel(t *testing.T) {
	e := newApproveEnv(t, "my-skill")
	args := []string{"--id", e.id, "--approver", "  Ada Lovelace  ", "--source-root", e.root}
	if _, stderr, code := runApprove(args, fixedClock(approveFixedNow)); code != 0 {
		t.Fatalf("exit = %d; stderr=%q", code, stderr)
	}
	if got := readRecord(t, e.recordPath()).Approver; got != "Ada Lovelace" {
		t.Errorf("approver = %q, want the trimmed label", got)
	}
}

func TestApprove_AnApproverOfExactlyTheBoundIsAccepted(t *testing.T) {
	// The bound is in runes, not bytes: 100 two-byte runes are 200 bytes and
	// must pass, and the label must survive the write and the strict re-read
	// unchanged.
	for name, label := range map[string]string{
		"ascii":     strings.Repeat("a", ApprovalApproverMaxRunes),
		"multibyte": strings.Repeat("é", ApprovalApproverMaxRunes),
	} {
		t.Run(name, func(t *testing.T) {
			e := newApproveEnv(t, "my-skill")
			args := []string{"--id", e.id, "--approver", label, "--source-root", e.root}
			if _, stderr, code := runApprove(args, fixedClock(approveFixedNow)); code != 0 {
				t.Fatalf("exit = %d, want 0 for a label of exactly %d runes; stderr=%q", code, ApprovalApproverMaxRunes, stderr)
			}
			if got := readRecord(t, e.recordPath()).Approver; got != label {
				t.Errorf("approver = %q, want the %d-rune label unchanged", got, ApprovalApproverMaxRunes)
			}
		})
	}
}

func TestApprove_ReapprovingIdenticalBytesIsIdempotent(t *testing.T) {
	e := newApproveEnv(t, "my-skill")
	if _, stderr, code := runApprove(e.args(), fixedClock(approveFixedNow)); code != 0 {
		t.Fatalf("first approve: exit %d; %q", code, stderr)
	}
	first, _ := os.ReadFile(e.recordPath())

	// A second run with a later clock and a different label must change nothing:
	// the original approval stays the record of who approved these bytes, and when.
	args := []string{"--id", e.id, "--approver", "someone-else", "--source-root", e.root}
	stdout, stderr, code := runApprove(args, fixedClock("2027-01-01T00:00:00Z"))
	if code != 0 {
		t.Fatalf("second approve: exit %d; %q", code, stderr)
	}
	second, _ := os.ReadFile(e.recordPath())
	if !bytes.Equal(first, second) {
		t.Errorf("re-approving identical bytes rewrote the record:\n%s\n---\n%s", first, second)
	}
	if !strings.HasPrefix(stdout, "unchanged: my-skill\n") {
		t.Errorf("stdout = %q, want it to start with %q", stdout, "unchanged: my-skill\n")
	}
}

func TestApprove_ChangedBytesReplaceTheRecord(t *testing.T) {
	e := newApproveEnv(t, "my-skill")
	if _, stderr, code := runApprove(e.args(), fixedClock(approveFixedNow)); code != 0 {
		t.Fatalf("first approve: exit %d; %q", code, stderr)
	}
	oldDigest := readRecord(t, e.recordPath()).SHA256

	changed := string(validDraft("my-skill")) + "\nOne more line.\n"
	writeTestFile(t, e.skillPath(), changed)

	args := []string{"--id", e.id, "--approver", "second-reviewer", "--source-root", e.root}
	if _, stderr, code := runApprove(args, fixedClock("2026-10-01T08:30:00Z")); code != 0 {
		t.Fatalf("re-approve: exit %d; %q", code, stderr)
	}
	rec := readRecord(t, e.recordPath())
	if rec.SHA256 == oldDigest || rec.SHA256 != SkillDigest([]byte(changed)) {
		t.Errorf("record digest = %s, want the digest of the changed bytes (old was %s)", rec.SHA256, oldDigest)
	}
	if rec.Approver != "second-reviewer" || rec.ApprovedAt != "2026-10-01T08:30:00Z" {
		t.Errorf("record = %+v, want the new approver and time", rec)
	}
}

func TestApprove_ReplacesAMalformedOrForeignRecord(t *testing.T) {
	for name, existing := range map[string]string{
		"malformed": "{ this is not json",
		"foreign":   goodRecordJSON("another-skill", abcDigest),
		"empty":     "",
	} {
		t.Run(name, func(t *testing.T) {
			e := newApproveEnv(t, "my-skill")
			writeTestFile(t, e.recordPath(), existing)
			if _, stderr, code := runApprove(e.args(), fixedClock(approveFixedNow)); code != 0 {
				t.Fatalf("exit %d; %q", code, stderr)
			}
			skill, _ := os.ReadFile(e.skillPath())
			st, err := ReadApprovalStatus(e.root, e.id, skill, os.ReadFile)
			if err != nil || st.State != ApprovalValid {
				t.Fatalf("after approve: state=%q err=%v, want valid", st.State, err)
			}
		})
	}
}

func TestApprove_RefusesWithoutWritingAnything(t *testing.T) {
	tests := []struct {
		name string
		args func(e approveEnv) []string
		now  func() string
		want string // substring of stderr
	}{
		{"missing --id", func(e approveEnv) []string { return []string{"--approver", "r", "--source-root", e.root} }, fixedClock(approveFixedNow), "--id"},
		{"missing --approver", func(e approveEnv) []string { return []string{"--id", e.id, "--source-root", e.root} }, fixedClock(approveFixedNow), "--approver"},
		{"missing --source-root", func(e approveEnv) []string { return []string{"--id", e.id, "--approver", "r"} }, fixedClock(approveFixedNow), "--source-root"},
		{"blank approver", func(e approveEnv) []string { return e.args("--approver", "   ") }, fixedClock(approveFixedNow), "approver"},
		{"empty approver", func(e approveEnv) []string { return []string{"--id", e.id, "--approver", "", "--source-root", e.root} }, fixedClock(approveFixedNow), "approver"},
		{"approver with a control character", func(e approveEnv) []string { return e.args("--approver", "a\tb") }, fixedClock(approveFixedNow), "approver"},
		{"invalid id slug", func(e approveEnv) []string {
			return []string{"--id", "../escape", "--approver", "r", "--source-root", e.root}
		}, fixedClock(approveFixedNow), "invalid"},
		{"no clock configured", func(e approveEnv) []string { return e.args() }, nil, "clock"},
		{"clock yields a malformed timestamp", func(e approveEnv) []string { return e.args() }, fixedClock("yesterday"), "approved_at"},
		{"unknown flag", func(e approveEnv) []string { return e.args("--force") }, fixedClock(approveFixedNow), "--force"},
		{"flag without a value", func(e approveEnv) []string { return []string{"--id"} }, fixedClock(approveFixedNow), "--id"},
		{"flag value that is a flag", func(e approveEnv) []string { return []string{"--id", "--approver", "r"} }, fixedClock(approveFixedNow), "--id"},
		{"stray positional", func(e approveEnv) []string { return e.args("extra") }, fixedClock(approveFixedNow), "extra"},
		// approve takes no positional argument, so an end-of-options marker
		// would have nothing to protect: it is an unknown flag like any other,
		// and it must not switch the parser into a mode that changes anything.
		{"end-of-options marker", func(e approveEnv) []string { return e.args("--") }, fixedClock(approveFixedNow), `unknown flag "--"`},
		{"end-of-options marker before the flags", func(e approveEnv) []string {
			return append([]string{"--"}, e.args()...)
		}, fixedClock(approveFixedNow), `unknown flag "--"`},
		// The refusal of a value that begins with "-" is deliberate (a flag
		// where a value belongs is a common mistake, and no sibling verb has a
		// --flag=value form), and for the approver it says why in its own words.
		{"approver that begins with a dash", func(e approveEnv) []string { return e.args("--approver", "-jo") }, fixedClock(approveFixedNow), `--approver`},
		{"approver that is a lone dash", func(e approveEnv) []string { return e.args("--approver", "-") }, fixedClock(approveFixedNow), `starts with "-"`},
		{"approver of one rune over the bound", func(e approveEnv) []string {
			return e.args("--approver", strings.Repeat("é", ApprovalApproverMaxRunes+1))
		}, fixedClock(approveFixedNow), "exceeds the bound"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newApproveEnv(t, "my-skill")
			stdout, stderr, code := runApprove(tc.args(e), tc.now)
			if code != 1 {
				t.Fatalf("exit = %d, want 1; stdout=%q stderr=%q", code, stdout, stderr)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Errorf("stderr %q does not contain %q", stderr, tc.want)
			}
			if stdout != "" {
				t.Errorf("a refusal must print nothing on stdout, got %q", stdout)
			}
			assertNoRecord(t, e)
		})
	}
}

func TestApprove_RefusesASkillThatIsNotThere(t *testing.T) {
	e := newApproveEnv(t, "my-skill")
	if err := os.Remove(e.skillPath()); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runApprove(e.args(), fixedClock(approveFixedNow))
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "my-skill") || !strings.Contains(stderr, "SKILL.md") {
		t.Errorf("stderr %q must name the skill and SKILL.md", stderr)
	}
	assertNoRecord(t, e)
}

func TestApprove_RefusesASkillThatFailsTheHardLint(t *testing.T) {
	e := newApproveEnv(t, "my-skill")
	writeTestFile(t, e.skillPath(), "no frontmatter here\n")
	_, stderr, code := runApprove(e.args(), fixedClock(approveFixedNow))
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "[lint:") {
		t.Errorf("stderr %q must carry the lint finding", stderr)
	}
	assertNoRecord(t, e)
}

func TestApprove_AdvisoryWarningsDoNotBlockApproval(t *testing.T) {
	// The same rule as `skills add`: only hard findings refuse.
	e := newApproveEnv(t, "my-skill")
	body := string(validDraft("my-skill")) + "\nRun `cat notes.md` on 2026-01-02.\n"
	writeTestFile(t, e.skillPath(), body)
	if hard, warns := LintSkillFile([]byte(body)); len(hard) != 0 || len(warns) == 0 {
		t.Fatalf("fixture must have warnings only: hard=%v warns=%v", hard, warns)
	}
	if _, stderr, code := runApprove(e.args(), fixedClock(approveFixedNow)); code != 0 {
		t.Fatalf("exit = %d; stderr=%q", code, stderr)
	}
}

func TestApprove_AnUnreadableExistingRecordRefusesInsteadOfOverwriting(t *testing.T) {
	e := newApproveEnv(t, "my-skill")
	// A directory in the record's place makes the read fail with something
	// other than "does not exist".
	if err := os.MkdirAll(e.recordPath(), 0o755); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runApprove(e.args(), fixedClock(approveFixedNow))
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stderr=%q", code, stderr)
	}
	if !strings.Contains(stderr, "approval record") {
		t.Errorf("stderr %q must name the approval record", stderr)
	}
	if info, err := os.Stat(e.recordPath()); err != nil || !info.IsDir() {
		t.Errorf("the unreadable record must be left untouched")
	}
}

func TestApprove_AWriteFailureIsReportedAndLeavesNoRecord(t *testing.T) {
	e := newApproveEnv(t, "my-skill")
	denyWrites(t, filepath.Join(e.root, e.id))

	_, stderr, code := runApprove(e.args(), fixedClock(approveFixedNow))
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stderr=%q", code, stderr)
	}
	if !strings.Contains(stderr, "writing approval record") {
		t.Errorf("stderr %q must name the failed write", stderr)
	}
	assertNoRecord(t, e)
}

func TestApprove_ConsumesTheWrapperInjectedFlags(t *testing.T) {
	// The labdrian wrapper appends --registry and --manifest after every
	// verb's own arguments; approve needs neither and must not choke on them.
	e := newApproveEnv(t, "my-skill")
	args := e.args("--registry", "/unused/skills.registry.yaml", "--manifest", "/unused/overlay.manifest")
	if _, stderr, code := runApprove(args, fixedClock(approveFixedNow)); code != 0 {
		t.Fatalf("exit = %d; stderr=%q", code, stderr)
	}
}

func TestSkillsCoreAt_DispatchesApprove(t *testing.T) {
	e := newApproveEnv(t, "my-skill")
	var out, errBuf bytes.Buffer
	code := -1
	// The labdrian wrapper always names the registry; approve does not read it, but
	// the overlay lock is keyed by it.
	registry := filepath.Join(filepath.Dir(e.root), "skills.registry.yaml")
	args := append([]string{"approve"}, e.args("--registry", registry)...)
	SkillsCoreAt("approve", args, os.ReadFile, fixedClock(approveFixedNow), noopLocker{}, &out, &errBuf, func(c int) { code = c })
	if code != 0 {
		t.Fatalf("exit = %d; stderr=%q", code, errBuf.String())
	}
	if !strings.HasPrefix(out.String(), "approved: my-skill\n") {
		t.Errorf("stdout = %q", out.String())
	}
}

func TestSkillsCore_WithoutAClockCannotApprove(t *testing.T) {
	// SkillsCore is the clock-less entry point kept for existing callers;
	// approve through it fails closed rather than inventing a timestamp.
	e := newApproveEnv(t, "my-skill")
	var out, errBuf bytes.Buffer
	code := -1
	SkillsCore("approve", append([]string{"approve"}, e.args()...), os.ReadFile, &out, &errBuf, func(c int) { code = c })
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	assertNoRecord(t, e)
}

func TestSkillsCore_UnknownVerbListsApprove(t *testing.T) {
	var out, errBuf bytes.Buffer
	SkillsCore("nuke", nil, os.ReadFile, &out, &errBuf, func(int) {})
	if !strings.Contains(errBuf.String(), "approve") {
		t.Errorf("the supported-verb list %q must include approve", errBuf.String())
	}
}
