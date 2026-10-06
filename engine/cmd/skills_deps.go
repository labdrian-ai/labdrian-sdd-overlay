package main

// The one value the skills verbs reach the outside world through. engine/skills owns the ports
// and knows no os call (Phase 9 unit H17); the composition root is the one place that chooses
// what stands behind each: the file system for the files of a tree and of a project, the YAML file for the
// registry, filelock for the locks, the wall clock for the approval time. A test of this
// package that wants another world builds its own Deps.

import (
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/projectidentity"
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
		Identity:   newProjectIdentity(),
		Locker:     newSkillsLocker(),
		Now:        wallClockUTC,
	}
}

// newProjectIdentity is the chain of sources that name the project a directory is, in the owner's
// order (Phase 9, decision Q8, enabled 2026-10-06): what the person said with --project-id, then
// the origin remote of the repository (projectidentity.GitOrigin, read from its .git/config
// without running git), then the name of the directory. A checkout whose origin is
// github.com/acme/demo is that project whatever its directory is called, so a registry that admits
// a skill to a project names it by its origin.
func newProjectIdentity() skills.ProjectIdentity {
	return projectidentity.Chain(projectidentity.Explicit{}, projectidentity.GitOrigin{}, projectidentity.DirectoryName{})
}
