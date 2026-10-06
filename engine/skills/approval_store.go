package skills

import "path/filepath"

// ApprovalRecordStore is where the evidence of an approval is read from: the SKILL.md of a
// global skill, and the record beside it that a human approved exactly those bytes. The domain
// decides where each lives (SkillMDPath, ApprovalRecordPath) and what a missing or unreadable
// one means; the store only reads. The verbs that judge approval (validate, add, approve) read
// through it, so a test or another medium can stand where the files of an overlay stand.
type ApprovalRecordStore interface {
	// ReadSkill reads the SKILL.md of the skill whose directory is path under sourceRoot.
	ReadSkill(sourceRoot, path string) ([]byte, error)
	// ReadRecord reads the approval record of skill id under sourceRoot. An error that is
	// fs.ErrNotExist means the skill has no record, which is an answer; any other means the record
	// could not be read, and the caller refuses rather than take it for no record.
	ReadRecord(sourceRoot, id string) ([]byte, error)
}

// SkillMDPath returns where the SKILL.md of the skill whose directory is path lives under
// sourceRoot.
func SkillMDPath(sourceRoot, path string) string {
	return filepath.Join(sourceRoot, path, "SKILL.md")
}
