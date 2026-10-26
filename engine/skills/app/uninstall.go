package app

import (
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// UninstallInput is what `skills uninstall` is asked: which installed skill to remove from which
// project, and whether only to say what it would remove.
type UninstallInput struct {
	// ProjectRoot is the directory the skill was installed into: absolute, and the one the caller
	// locked.
	ProjectRoot string
	// ID is the installed skill.
	ID string
	// DryRun plans and writes nothing.
	DryRun bool
}

// UninstallResult is what uninstall did, or would do.
type UninstallResult struct {
	// Planned is every file a dry run would remove, repo-relative, as git pathspecs.
	Planned []string
	// PlannedLock is the project lock a dry run would rewrite without the install record: it is
	// updated, never removed.
	PlannedLock string
	// Removed is every file that was removed.
	Removed []string
	// Kept is every skill directory left because it still holds files the install record did not
	// list.
	Kept []string
}

// UninstallProject removes an installed skill from the project: every file its install record
// lists that is still as install wrote it, in every runtime directory, then the record, all or
// nothing (skills.PlanUninstall decides, skills.ExecuteInstallPlan carries out), and then the
// skill directories that are left empty. It needs no registry and no source: the install record is
// the whole of what it reads. An error is a *ProjectLockReadError (no lock, or one that cannot be
// read), a *PlanRefusal or an *ExecutionError; where it is not an *ExecutionError nothing was
// removed.
func UninstallProject(p InstallPorts, in UninstallInput) (UninstallResult, error) {
	var res UninstallResult
	root := filepath.Clean(in.ProjectRoot)
	lockData, lockExists, err := readOptionalProjectLock(p.Locks, root)
	if err != nil {
		return res, err
	}
	planned, refusals := skills.PlanUninstall(skills.InstallInput{
		ProjectRoot: root,
		LockData:    lockData,
		LockExists:  lockExists,
		ReadFile:    p.Files,
		Stat:        p.Project.Stat,
		ResolvePath: p.Project.ResolvePath,
		ReadDir:     p.Project.ReadDir,
	}, in.ID)
	if len(refusals) > 0 {
		return res, &PlanRefusal{Verb: "uninstall", Reasons: refusals}
	}
	if in.DryRun {
		res.Planned = relPaths(planned.Deletes)
		res.PlannedLock = planned.Lock.Rel
		return res, nil
	}
	if err := skills.ExecuteInstallPlan(planned, root, p.Project); err != nil {
		return res, executionFailure(err)
	}
	res.Removed = relPaths(planned.Deletes)
	res.Kept = skills.RemoveEmptySkillDirs(p.Project, root, planned)
	return res, nil
}
