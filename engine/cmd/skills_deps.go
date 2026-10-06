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

// newProjectIdentity is the chain of sources that name the project a directory is, in the order the
// program asks them: what the person said with --project-id, and then the name of the directory.
//
// The owner's order (Phase 9, decision Q8) has one more link between the two: projectidentity.
// GitOrigin, the origin remote of the repository read from its .git/config without running git.
// It is built and tested and it is NOT in this chain yet, because putting it there changes what
// install and adopt do without --project-id in a repository that has an origin: the project would
// be github.com/acme/demo where it is demo today, so a registry that admits a skill to demo admits
// it to nobody in a checkout of that repository, and the goldens under testdata/
// project-identity-golden say which cases change. That is a decision of the owner, not of the
// refactor that put the id behind a port (H18); the day it is taken it is one line here.
func newProjectIdentity() skills.ProjectIdentity {
	return projectidentity.Chain(projectidentity.Explicit{}, projectidentity.DirectoryName{})
}
