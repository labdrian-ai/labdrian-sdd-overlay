package skills

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The approval record sits inside the skill directory it approves, so every
// whole-tree copier must leave it behind: it is repository governance state,
// not skill content, and a runtime that loaded it would treat it as part of
// the skill. These tests pin the copiers that live in this package; the Pi
// package builder pins its own in engine/pipkg.

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

func TestExecuteInstall_DoesNotProjectTheApprovalRecord(t *testing.T) {
	src := filepath.Join(t.TempDir(), "my-skill")
	dst := filepath.Join(t.TempDir(), ".claude", "skills", "my-skill")
	writeTestFile(t, filepath.Join(src, "SKILL.md"), "body\n")
	writeTestFile(t, filepath.Join(src, "references", "notes.md"), "notes\n")
	writeTestFile(t, filepath.Join(src, ApprovalRecordName), goodRecordJSON("my-skill", abcDigest))

	var out, errBuf bytes.Buffer
	plan := []CopyOp{{SkillID: "my-skill", Src: src, Dst: dst}}
	if err := ExecuteInstall(plan, &out, &errBuf); err != nil {
		t.Fatalf("ExecuteInstall: %v (stderr %q)", err, errBuf.String())
	}

	for _, want := range []string{"SKILL.md", filepath.Join("references", "notes.md")} {
		if _, err := os.Stat(filepath.Join(dst, want)); err != nil {
			t.Errorf("skill content %q must still be projected: %v", want, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, ApprovalRecordName)); err == nil {
		t.Errorf("the approval record must not be projected into %s", dst)
	}
}

func TestExecuteInstall_OnlyTheRootLevelRecordIsSkipped(t *testing.T) {
	// A file that merely shares the record's name deeper in the tree belongs to
	// the skill's own content and is copied as usual.
	src := filepath.Join(t.TempDir(), "my-skill")
	dst := filepath.Join(t.TempDir(), "out")
	writeTestFile(t, filepath.Join(src, "SKILL.md"), "body\n")
	writeTestFile(t, filepath.Join(src, "references", ApprovalRecordName), "skill-owned\n")

	var out, errBuf bytes.Buffer
	if err := ExecuteInstall([]CopyOp{{SkillID: "my-skill", Src: src, Dst: dst}}, &out, &errBuf); err != nil {
		t.Fatalf("ExecuteInstall: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "references", ApprovalRecordName)); err != nil {
		t.Errorf("a nested file sharing the record name is skill content and must be copied: %v", err)
	}
}

func TestExecuteInstall_ADirectoryInTheRecordsPlaceIsNotProjectedEither(t *testing.T) {
	src := filepath.Join(t.TempDir(), "my-skill")
	dst := filepath.Join(t.TempDir(), "out")
	writeTestFile(t, filepath.Join(src, "SKILL.md"), "body\n")
	writeTestFile(t, filepath.Join(src, ApprovalRecordName, "inner.txt"), "not skill content\n")

	var out, errBuf bytes.Buffer
	if err := ExecuteInstall([]CopyOp{{SkillID: "my-skill", Src: src, Dst: dst}}, &out, &errBuf); err != nil {
		t.Fatalf("ExecuteInstall: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, ApprovalRecordName)); err == nil {
		t.Errorf("nothing at the record's path may be projected, directory included")
	}
}
