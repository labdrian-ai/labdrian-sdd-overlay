package skillsfs

import (
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// Approvals is the skills.ApprovalRecordStore of the real file system: the SKILL.md of a skill
// and the approval record beside it are read from the paths the domain names (SkillMDPath and
// ApprovalRecordPath). It holds nothing: the zero value is ready, and it returns the failure of
// the system as it is, so a missing file is an error that is fs.ErrNotExist and the words of
// every other refusal are the system's.
type Approvals struct{}

var _ skills.ApprovalRecordStore = Approvals{}

// ReadSkill reads the SKILL.md of the skill whose directory is path under sourceRoot.
func (Approvals) ReadSkill(sourceRoot, path string) ([]byte, error) {
	return os.ReadFile(skills.SkillMDPath(sourceRoot, path))
}

// ReadRecord reads the approval record of skill id under sourceRoot.
func (Approvals) ReadRecord(sourceRoot, id string) ([]byte, error) {
	return os.ReadFile(skills.ApprovalRecordPath(sourceRoot, id))
}
