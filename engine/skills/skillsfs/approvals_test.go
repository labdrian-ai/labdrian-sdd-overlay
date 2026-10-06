package skillsfs_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/skillsfs"
)

func TestApprovalsReadTheFilesTheDomainNames(t *testing.T) {
	root := t.TempDir()
	put(t, skills.SkillMDPath(root, "alpha"), "skill bytes")
	put(t, skills.ApprovalRecordPath(root, "alpha"), `{"record": true}`)

	skill, err := skillsfs.Approvals{}.ReadSkill(root, "alpha")
	if err != nil || string(skill) != "skill bytes" {
		t.Errorf("ReadSkill = %q, %v, want the bytes of alpha/SKILL.md", skill, err)
	}
	record, err := skillsfs.Approvals{}.ReadRecord(root, "alpha")
	if err != nil || string(record) != `{"record": true}` {
		t.Errorf("ReadRecord = %q, %v, want the bytes of alpha/.approval.json", record, err)
	}
}

func TestApprovalsSayAMissingFileWithTheErrorThatMeansAbsent(t *testing.T) {
	root := t.TempDir()
	if _, err := (skillsfs.Approvals{}).ReadSkill(root, "none"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadSkill of a skill that is not there: %v, want fs.ErrNotExist", err)
	}
	if _, err := (skillsfs.Approvals{}).ReadRecord(root, "none"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadRecord of a skill with no record: %v, want fs.ErrNotExist", err)
	}
}

// A record that is a directory is not "no record": the failure is the system's, and is not
// fs.ErrNotExist, so the domain refuses instead of taking it for an absent approval.
func TestApprovalsDoNotTakeAnUnreadableRecordForAnAbsentOne(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "alpha", skills.ApprovalRecordName), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := (skillsfs.Approvals{}).ReadRecord(root, "alpha"); err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadRecord of a directory: %v, want a failure that is not fs.ErrNotExist", err)
	}
}
