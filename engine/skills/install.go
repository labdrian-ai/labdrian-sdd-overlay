package skills

import (
	"path/filepath"
)

// CopyOp is a single file-tree copy directive: copy the Src/ tree to Dst/.
type CopyOp struct {
	SkillID string // entry id, used for output messages
	Src     string // <sourceRoot>/<entry.Path>
	Dst     string // <targetRoot>/.claude/skills/<entry.ID>
}

// PlanInstall filters reg for project-scoped skills allowed for projectID,
// building one CopyOp per admitted entry. Pure: no filesystem access.
// Declaration order from reg.Skills is preserved in the returned slice.
func PlanInstall(reg Registry, projectID, sourceRoot, targetRoot string) ([]CopyOp, error) {
	var ops []CopyOp
	for _, e := range reg.Skills {
		if e.Install.DefaultScope != "project" {
			continue
		}
		if !containsString(e.Install.AllowedProjects, projectID) {
			continue
		}
		ops = append(ops, CopyOp{
			SkillID: e.ID,
			Src:     filepath.Join(sourceRoot, e.Path),
			Dst:     filepath.Join(targetRoot, ".claude", "skills", e.ID),
		})
	}
	return ops, nil
}

// containsString reports whether slice contains s (case-sensitive).
func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
