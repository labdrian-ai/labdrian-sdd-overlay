package skills_test

import (
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/skillsfs"
)

// The tests of package skills read the tree of an overlay through the file system adapter, as the
// program does. The adapter imports package skills, so a test file of that package cannot import
// it; this file, in the external test package, registers it (see export_test.go).
func init() {
	skills.UseOSTree(skillsfs.Tree{})
}
