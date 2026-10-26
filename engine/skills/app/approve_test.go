package app

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// baselineSkillID is a real member of the lint baseline: the list of the ids whose legacy lint
// findings are told as warnings and not refused is fixed data. It exempts nothing from approval.
const baselineSkillID = "sdd-apply"

// approveWorld is a skills tree that holds one skill that passes the hard lint and has no record.
type approveWorld struct{ root string }

func newApproveWorld(t *testing.T, id string) approveWorld {
	t.Helper()
	root := filepath.Join(t.TempDir(), "skills")
	put(t, filepath.Join(root, id, "SKILL.md"), skillFor(id))
	return approveWorld{root: root}
}

func (w approveWorld) ports(staged skills.StagedWrites, now func() string) ApprovePorts {
	return ApprovePorts{Approvals: skillsApprovals(), Staged: staged, Now: now}
}

func (w approveWorld) approve(staged skills.StagedWrites, id string) (ApproveResult, error) {
	return ApproveSkill(w.ports(staged, fixedClock(approvedAt)), ApproveInput{ID: id, Approver: "reviewer", ApproverGiven: true, SourceRoot: w.root})
}

func (w approveWorld) record(t *testing.T, id string) skills.ApprovalRecord {
	t.Helper()
	rec, err := skills.ParseApprovalRecord([]byte(read(t, skills.ApprovalRecordPath(w.root, id))))
	if err != nil {
		t.Fatalf("the record on disk does not parse: %v", err)
	}
	return rec
}

func TestApproveRecordsTheApprovalOfTheExactBytes(t *testing.T) {
	w := newApproveWorld(t, "my-skill")
	spy := newStagedSpy(nil)
	res, err := w.approve(spy, "my-skill")
	if err != nil {
		t.Fatal(err)
	}
	digest := digestOfSkillMDAlone(skillFor("my-skill"))
	want := ApproveResult{Verdict: "approved", ID: "my-skill", Digest: digest, RecordPath: skills.ApprovalRecordPath(w.root, "my-skill")}
	if res.Verdict != want.Verdict || res.ID != want.ID || res.Digest != want.Digest || res.RecordPath != want.RecordPath || len(res.Warnings) != 0 {
		t.Errorf("ApproveSkill = %+v, want %+v", res, want)
	}
	if rec := w.record(t, "my-skill"); rec != (skills.ApprovalRecord{Version: 2, Skill: "my-skill", SHA256: digest, ApprovedAt: approvedAt, Approver: "reviewer"}) {
		t.Errorf("record = %+v", rec)
	}
	// The record is committed with the skill, so it is staged world-readable, and the atomic write
	// leaves no temporary file behind.
	if len(spy.perms) != 1 || spy.perms[0] != 0o644 {
		t.Errorf("the record was staged at %v, want 0644", spy.perms)
	}
	entries, _ := os.ReadDir(filepath.Join(w.root, "my-skill"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("stray temp file %q left in the skill directory", e.Name())
		}
	}
}

func TestApproveTrimsTheApproverAndLeavesAValidRecordExactlyAsItIs(t *testing.T) {
	w := newApproveWorld(t, "my-skill")
	if _, err := ApproveSkill(w.ports(newStagedSpy(nil), fixedClock(approvedAt)), ApproveInput{ID: "my-skill", Approver: "  Jane Doe \t", ApproverGiven: true, SourceRoot: w.root}); err != nil {
		t.Fatal(err)
	}
	if got := w.record(t, "my-skill").Approver; got != "Jane Doe" {
		t.Errorf("approver = %q, want the label trimmed", got)
	}
	before := read(t, skills.ApprovalRecordPath(w.root, "my-skill"))
	spy := newStagedSpy(nil)
	res, err := ApproveSkill(w.ports(spy, fixedClock("2031-01-02T03:04:05Z")), ApproveInput{ID: "my-skill", Approver: "Someone Else", ApproverGiven: true, SourceRoot: w.root})
	if err != nil || res.Verdict != "unchanged" {
		t.Fatalf("ApproveSkill = %+v, %v, want the approval left unchanged", res, err)
	}
	if len(spy.ops) != 0 || read(t, skills.ApprovalRecordPath(w.root, "my-skill")) != before {
		t.Errorf("a valid record was written again (%q): the original approver and time must stay the record", spy.ops)
	}
}

func TestApproveReplacesAStaleOrMalformedRecord(t *testing.T) {
	for name, content := range map[string]string{
		"a record of other bytes": "",
		"a malformed record":      "not json",
		// The approval of version 1 covered SKILL.md alone, and is replaced by one of every file even
		// though SKILL.md has not changed since.
		"a record of version 1": legacyRecord("my-skill", skillFor("my-skill")),
	} {
		t.Run(name, func(t *testing.T) {
			w := newApproveWorld(t, "my-skill")
			if content == "" {
				if _, err := w.approve(newStagedSpy(nil), "my-skill"); err != nil {
					t.Fatal(err)
				}
				put(t, filepath.Join(w.root, "my-skill", "SKILL.md"), skillFor("my-skill")+"\nA changed line.\n")
			} else {
				put(t, skills.ApprovalRecordPath(w.root, "my-skill"), content)
			}
			res, err := w.approve(newStagedSpy(nil), "my-skill")
			if err != nil || res.Verdict != "approved" {
				t.Fatalf("ApproveSkill = %+v, %v, want a new record written", res, err)
			}
			if got := w.record(t, "my-skill").SHA256; got != res.Digest {
				t.Errorf("the record holds %s, want the digest of the bytes now on disk, %s", got, res.Digest)
			}
			if got := w.record(t, "my-skill").Version; got != 2 {
				t.Errorf("the record is of version %d, want the version that covers every file, 2", got)
			}
		})
	}
}

func TestApproveWritesNothingWhenItRefuses(t *testing.T) {
	good := ApproveInput{ID: "my-skill", Approver: "reviewer", ApproverGiven: true}
	for name, tc := range map[string]struct {
		change  func(in *ApproveInput, now *func() string)
		wantErr error
		message string
	}{
		"no id":                  {func(in *ApproveInput, _ *func() string) { in.ID = "" }, ErrApproveIDRequired, ""},
		"an id that is no slug":  {func(in *ApproveInput, _ *func() string) { in.ID = "My_Skill" }, nil, `skills approve: id "My_Skill": invalid slug (must match ^[a-z0-9][a-z0-9-]*$)`},
		"no approver":            {func(in *ApproveInput, _ *func() string) { in.ApproverGiven = false }, ErrApproveApproverRequired, ""},
		"a blank approver":       {func(in *ApproveInput, _ *func() string) { in.Approver = "   " }, nil, "skills approve: approver must not be blank"},
		"an approver of 2 lines": {func(in *ApproveInput, _ *func() string) { in.Approver = "one\ntwo" }, nil, ""},
		"no source root":         {func(in *ApproveInput, _ *func() string) { in.SourceRoot = "" }, ErrApproveSourceRequired, ""},
		"no clock":               {func(_ *ApproveInput, now *func() string) { *now = nil }, ErrApproveNoClock, ""},
	} {
		t.Run(name, func(t *testing.T) {
			w := newApproveWorld(t, "my-skill")
			in, now := good, fixedClock(approvedAt)
			in.SourceRoot = w.root
			tc.change(&in, &now)
			spy := newStagedSpy(nil)
			_, err := ApproveSkill(w.ports(spy, now), in)
			var refusal *ApproveRefusal
			if !errors.As(err, &refusal) {
				t.Fatalf("err = %v, want an approval refusal", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Errorf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.message != "" && err.Error() != tc.message {
				t.Errorf("err = %q, want %q", err, tc.message)
			}
			if len(spy.ops) != 0 {
				t.Errorf("the writes were called (%q) before the refusal", spy.ops)
			}
		})
	}
}

func TestApproveRefusesASkillThatIsNotThereOrFailsTheHardLintAndWritesNothing(t *testing.T) {
	w := newApproveWorld(t, "my-skill")
	spy := newStagedSpy(nil)
	_, err := w.approve(spy, "absent")
	if err == nil || !strings.Contains(err.Error(), `skill "absent": SKILL.md not found or unreadable at`) {
		t.Errorf("err = %v, want the SKILL.md not found", err)
	}
	put(t, filepath.Join(w.root, "my-skill", "SKILL.md"), strings.Replace(skillFor("my-skill"), "  version: \"1.0\"\n", "", 1))
	var lint *LintRefusal
	if _, err := w.approve(spy, "my-skill"); !errors.As(err, &lint) || len(lint.Findings) == 0 {
		t.Errorf("err = %v, want the hard findings of the lint", err)
	}
	if len(spy.ops) != 0 {
		t.Errorf("the writes were called (%q) for a skill that was refused", spy.ops)
	}
}

// unreadableTree is an approval store whose files of a skill cannot be listed for the reason it is
// given.
type unreadableTree struct {
	skills.ApprovalRecordStore
	cause error
}

func (u unreadableTree) ReadSkillTree(string, string) ([]skills.SkillEntry, error) {
	return nil, u.cause
}

// A SKILL.md that is not there and one that cannot be read are one refusal in the words of the
// verb, and the refusal carries the cause, so that a person is told which of the two it is and a
// caller can tell them apart with errors.Is.
func TestApproveSaysWhetherTheSkillIsMissingOrCannotBeRead(t *testing.T) {
	w := newApproveWorld(t, "my-skill")
	for _, tc := range []struct {
		name  string
		cause error
		is    error
		words string
	}{
		{"missing", &fs.PathError{Op: "open", Path: "x", Err: fs.ErrNotExist}, fs.ErrNotExist, "file does not exist"},
		{"unreadable", &fs.PathError{Op: "open", Path: "x", Err: fs.ErrPermission}, fs.ErrPermission, "permission denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ports := w.ports(newStagedSpy(nil), fixedClock(approvedAt))
			ports.Approvals = unreadableTree{skillsApprovals(), tc.cause}
			_, err := ApproveSkill(ports, ApproveInput{ID: "my-skill", Approver: "reviewer", ApproverGiven: true, SourceRoot: w.root})
			var refusal *ApproveRefusal
			if !errors.As(err, &refusal) || !errors.Is(err, tc.is) {
				t.Fatalf("err = %v, want an *ApproveRefusal whose cause is %v", err, tc.is)
			}
			for _, want := range []string{`skill "my-skill": SKILL.md not found or unreadable at`, tc.words} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("err = %q, want it to say %q", err, want)
				}
			}
		})
	}
}

// An existing record that cannot be read is refused, not overwritten blind.
func TestApproveRefusesARecordThatCannotBeReadInsteadOfOverwritingIt(t *testing.T) {
	w := newApproveWorld(t, "my-skill")
	if err := os.MkdirAll(skills.ApprovalRecordPath(w.root, "my-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	spy := newStagedSpy(nil)
	_, err := w.approve(spy, "my-skill")
	if err == nil || !strings.Contains(err.Error(), `skill "my-skill": reading approval record`) || len(spy.ops) != 0 {
		t.Errorf("err = %v, writes %q, want the unreadable record refused", err, spy.ops)
	}
}

// A skill of the approval baseline predates the lint budget: its legacy findings are told as
// warnings, once the approval has happened, and do not stop it; any other hard finding does, and
// a skill outside the baseline is refused for the same findings.
func TestApproveTellsTheLegacyFindingsOfABaselineSkillAsWarningsAndRefusesTheOthers(t *testing.T) {
	overBudget := skillFor("x") + strings.Repeat("Another line of the procedure, long enough to count.\n", 100)
	for name, tc := range map[string]struct {
		id, md       string
		wantWarnings bool
		wantRefused  bool
	}{
		"a baseline skill over the body budget":        {baselineSkillID, overBudget, true, false},
		"a baseline skill with no front matter":        {baselineSkillID, "no frontmatter here\n", false, true},
		"a skill outside the baseline over the budget": {"my-skill", overBudget, false, true},
		"a clean baseline skill":                       {baselineSkillID, skillFor(baselineSkillID), false, false},
	} {
		t.Run(name, func(t *testing.T) {
			w := newApproveWorld(t, tc.id)
			put(t, filepath.Join(w.root, tc.id, "SKILL.md"), tc.md)
			spy := newStagedSpy(nil)
			res, err := w.approve(spy, tc.id)
			var lint *LintRefusal
			switch {
			case tc.wantRefused && (!errors.As(err, &lint) || len(spy.ops) != 0 || len(res.Warnings) != 0):
				t.Fatalf("ApproveSkill = %+v, %v (writes %q), want the lint refusal and nothing written", res, err, spy.ops)
			case !tc.wantRefused && err != nil:
				t.Fatalf("ApproveSkill = %v, want it approved", err)
			case tc.wantWarnings && (len(res.Warnings) == 0 || !strings.HasSuffix(res.Warnings[0], " (baseline skill: approved with lint findings)") || !strings.HasPrefix(res.Warnings[0], "warning: [lint:")):
				t.Errorf("warnings = %q, want the legacy findings as warnings", res.Warnings)
			case !tc.wantWarnings && !tc.wantRefused && len(res.Warnings) != 0:
				t.Errorf("warnings = %q, want none for a skill with no finding", res.Warnings)
			}
		})
	}
}

func TestApprovePutsBackWhatItStagedWhenItCannotWriteTheRecord(t *testing.T) {
	for name, tc := range map[string]struct {
		spy  func() *stagedSpy
		want string
	}{
		"staging": {
			func() *stagedSpy { return failing("writetemp", "", 1, errors.New("create temp: no room")) },
			`skills approve: skill "foo": writing approval record: writeFileAtomic: create temp: no room`,
		},
		"committing": {
			func() *stagedSpy { return failing("rename", "", 1, errInjected) },
			`skills approve: skill "foo": writing approval record: finalizing "%s": injected failure`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			w := newApproveWorld(t, "foo")
			_, err := w.approve(tc.spy(), "foo")
			want := tc.want
			want = strings.Replace(want, "%s", skills.ApprovalRecordPath(w.root, "foo"), 1)
			if err == nil || err.Error() != want {
				t.Errorf("err = %v, want %q", err, want)
			}
			if _, statErr := os.Stat(skills.ApprovalRecordPath(w.root, "foo")); !errors.Is(statErr, os.ErrNotExist) {
				t.Errorf("a record is there after a failed write: %v", statErr)
			}
			entries, _ := os.ReadDir(filepath.Join(w.root, "foo"))
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), ".tmp-skills-") {
					t.Errorf("the temporary file %s was left behind", e.Name())
				}
			}
		})
	}
}

// The approval is of every file of the skill, found at any depth, and not of SKILL.md alone: the
// digest the record holds is the one sha256sum gives for the list of them (skills.SkillDigest),
// and a change to any file but the record itself is a change the next approval records.
func TestApproveRecordsTheApprovalOfEveryFileOfTheSkill(t *testing.T) {
	w := newApproveWorld(t, "my-skill")
	put(t, filepath.Join(w.root, "my-skill", "scripts", "run.sh"), "echo hi\n")
	put(t, filepath.Join(w.root, "my-skill", "references", "deep", "notes.md"), "notes\n")

	first, err := w.approve(newStagedSpy(nil), "my-skill")
	if err != nil || first.Verdict != "approved" {
		t.Fatalf("ApproveSkill = %+v, %v, want the skill approved", first, err)
	}
	alone := digestOfSkillMDAlone(skillFor("my-skill"))
	if first.Digest == alone {
		t.Errorf("the digest %s is the one of SKILL.md alone: the script and the reference were left out", first.Digest)
	}
	// By hand, as the doc of skills.SkillDigest says: the list sorted by the bytes of the path.
	list := sha256hex.Sum([]byte(skillFor("my-skill"))) + "  SKILL.md\n" +
		sha256hex.Sum([]byte("notes\n")) + "  references/deep/notes.md\n" +
		sha256hex.Sum([]byte("echo hi\n")) + "  scripts/run.sh\n"
	if want := sha256hex.Sum([]byte(list)); first.Digest != want {
		t.Errorf("digest = %s, want %s (the digest of the list of the three files)", first.Digest, want)
	}

	// Approving the same files again changes nothing, with the record sitting among them.
	spy := newStagedSpy(nil)
	if again, err := w.approve(spy, "my-skill"); err != nil || again.Verdict != "unchanged" || len(spy.ops) != 0 {
		t.Fatalf("ApproveSkill = %+v, %v (writes %q), want the approval left unchanged", again, err, spy.ops)
	}

	// A file beside SKILL.md changing is a skill that is approved again, with another digest.
	put(t, filepath.Join(w.root, "my-skill", "scripts", "run.sh"), "echo changed\n")
	second, err := w.approve(newStagedSpy(nil), "my-skill")
	if err != nil || second.Verdict != "approved" || second.Digest == first.Digest {
		t.Fatalf("ApproveSkill = %+v, %v, want a new approval with another digest than %s", second, err, first.Digest)
	}
	if got := w.record(t, "my-skill").SHA256; got != second.Digest {
		t.Errorf("the record holds %s, want %s", got, second.Digest)
	}
}

// What an approval cannot cover is refused before anything is written, and the refusal says what it
// is and what to do, in the words `skills validate` uses for the same skill.
func TestApproveRefusesASkillWhoseDirectoryHoldsWhatNoDigestCovers(t *testing.T) {
	for name, tc := range map[string]struct {
		prepare func(t *testing.T, dir string)
		entry   string
	}{
		"a link beside SKILL.md": {func(t *testing.T, dir string) {
			if err := os.Symlink("SKILL.md", filepath.Join(dir, "alias.md")); err != nil {
				t.Skipf("no symbolic link here: %v", err)
			}
		}, "alias.md is a symbolic link"},
		"a link to nothing in a subdirectory": {func(t *testing.T, dir string) {
			if err := os.MkdirAll(filepath.Join(dir, "references"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("nothing-is-here", filepath.Join(dir, "references", "broken.md")); err != nil {
				t.Skipf("no symbolic link here: %v", err)
			}
		}, "references/broken.md is a symbolic link"},
		"a name with a line break": {func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, "two\nlines"), []byte("x"), 0o644); err != nil {
				t.Skipf("no file name with a line break here: %v", err)
			}
		}, `"two\nlines" has a line break in its name`},
	} {
		t.Run(name, func(t *testing.T) {
			w := newApproveWorld(t, "my-skill")
			tc.prepare(t, filepath.Join(w.root, "my-skill"))
			spy := newStagedSpy(nil)

			_, err := w.approve(spy, "my-skill")

			var refusal *ApproveRefusal
			if !errors.As(err, &refusal) {
				t.Fatalf("err = %v, want an approval refusal", err)
			}
			for _, want := range []string{`skill "my-skill"`, "cannot be verified", tc.entry, "regular files only"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("err = %q, want it to say %q", err, want)
				}
			}
			if len(spy.ops) != 0 {
				t.Errorf("the writes were called (%q) for a skill nothing can approve", spy.ops)
			}
			if _, statErr := os.Stat(skills.ApprovalRecordPath(w.root, "my-skill")); !errors.Is(statErr, os.ErrNotExist) {
				t.Errorf("a record is there for a skill nothing can approve: %v", statErr)
			}
		})
	}
}

// A SKILL.md that is a link has no bytes to lint and nothing to approve: the refusal is the one of
// an approval that cannot cover it, not a lint finding of an empty file.
func TestApproveRefusesASKILLmdThatIsALink(t *testing.T) {
	w := newApproveWorld(t, "my-skill")
	dir := filepath.Join(w.root, "my-skill")
	if err := os.Rename(filepath.Join(dir, "SKILL.md"), filepath.Join(dir, "real-skill.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real-skill.md", filepath.Join(dir, "SKILL.md")); err != nil {
		t.Skipf("no symbolic link here: %v", err)
	}
	spy := newStagedSpy(nil)

	_, err := w.approve(spy, "my-skill")

	var lint *LintRefusal
	if err == nil || errors.As(err, &lint) || !strings.Contains(err.Error(), "SKILL.md is a symbolic link") || len(spy.ops) != 0 {
		t.Errorf("err = %v (writes %q), want the refusal of an approval that cannot cover a link", err, spy.ops)
	}
}

// A directory that holds files but no SKILL.md is not a skill, and is told so in the words of a
// SKILL.md that is not there.
func TestApproveRefusesADirectoryWithNoSKILLmd(t *testing.T) {
	w := newApproveWorld(t, "my-skill")
	put(t, filepath.Join(w.root, "empty-handed", "README.md"), "not a skill")
	spy := newStagedSpy(nil)

	_, err := w.approve(spy, "empty-handed")

	if err == nil || !strings.Contains(err.Error(), `skill "empty-handed": SKILL.md not found or unreadable at`) || !strings.Contains(err.Error(), "holds no file named SKILL.md") || len(spy.ops) != 0 {
		t.Errorf("err = %v (writes %q), want the SKILL.md not found", err, spy.ops)
	}
}
