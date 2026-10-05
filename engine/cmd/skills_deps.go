package main

// The one value the skills verbs reach the outside world through. engine/skills owns the ports
// and knows no os call (Phase 9 unit H17); the composition root is the one place that chooses
// what stands behind each: the file system for the files of a tree and of a project, the YAML file for the
// registry, filelock for the locks, the wall clock for the approval time. A test of this
// package that wants another world builds its own Deps.

import (
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/skillsfs"
)

// newSkillsDeps returns the Deps of the production entry point.
func newSkillsDeps() skills.Deps {
	return skills.Deps{
		ReadFile:   os.ReadFile,
		Registries: newRegistryRepository(),
		Tree:       skillsfs.Tree{},
		Project:    skillsfs.Project{},
		Cwd:        os.Getwd,
		Locker:     newSkillsLocker(),
		Now:        wallClockUTC,
	}
}
