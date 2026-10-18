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
	"github.com/labdrian-ai/labdrian-sdd-overlay/identity/gitfs"
	"github.com/labdrian-ai/labdrian-sdd-overlay/identity/resolve"
)

// newSkillsDeps returns the Deps of the production entry point, over the working directory and
// the lock wait of d.
func newSkillsDeps(d deps) skills.Deps {
	return skills.Deps{
		ReadFile:     os.ReadFile,
		Approvals:    skillsfs.Approvals{},
		Registries:   newRegistryRepository(),
		Tree:         skillsfs.Tree{},
		Project:      skillsfs.Project{},
		ProjectLocks: skillsfs.ProjectLocks{},
		Cwd:          d.workingDir,
		Identity:     newProjectIdentity(),
		Locker:       newSkillsLocker(d.skillsLockWait),
		Now:          wallClockUTC,
	}
}

// newProjectIdentity names the project a directory is with the shared resolver (identity/resolve,
// C23 M2), the one the workflow and longterm-mem name it with, in the owner's order of 2026-10-18:
// what the person said with --project-id, then the file .longterm-mem-project at the root of the
// main checkout, then the origin remote of the repository, then the name of the main checkout's
// root, and outside a repository the name of the directory. The repository is read by
// identity/gitfs, from its files and without running git. A checkout whose origin is
// github.com/acme/demo is that project whatever its directory is called, so a registry that admits
// a skill to a project names it by its origin.
func newProjectIdentity() skills.ProjectIdentity {
	return projectidentity.Shared{Rules: sharedProjectRules("--project-id")}
}

// sharedProjectRules is the shared resolver reading this machine's disk. explicitFlag is the flag
// of the verb through which a person names the project without the repository being looked at; a
// refusal to read a repository says to give it, and says nothing of a flag when it is "".
func sharedProjectRules(explicitFlag string) resolve.Resolver {
	return resolve.New(gitfs.Repositories{ExplicitFlag: explicitFlag})
}
