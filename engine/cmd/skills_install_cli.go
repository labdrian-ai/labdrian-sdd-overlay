package main

// The command-line adapter of the verbs that install into a project (Phase 9 unit H20): `install`
// and `adopt`. Each reads its arguments with the one parser (skills_flags.go), names the directory
// it works in, takes the locks it needs (the overlay's, shared, then the project's, exclusive),
// asks its use case in engine/skills/app, and tells a person what it answered. The use cases parse
// nothing and print nothing.

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/app"
)

// installUseCase is what install and adopt ask: the same input, the same ports, the same answer.
type installUseCase func(app.InstallPorts, app.InstallInput) (app.InstallResult, error)

// skillsInstall is `skills install`: it copies the skills the registry admits to the project in the
// working directory into it, records them in the project lock, and says what it did to each.
func skillsInstall(deps skills.Deps, args []string, stdout, stderr io.Writer, exit func(int)) {
	skillsInstallRun("install", "installed", app.InstallProject, deps, args, stdout, stderr, exit)
}

// skillsAdopt is `skills adopt`: the same arguments as install, and the same ownership rules, used
// to record what is already in the project instead of writing it.
func skillsAdopt(deps skills.Deps, args []string, stdout, stderr io.Writer, exit func(int)) {
	skillsInstallRun("adopt", "adopted", app.AdoptProject, deps, args, stdout, stderr, exit)
}

// skillsInstallRun is a verb that works in the directory the process is in: the command line is
// read first, then the directory is named once, and the locks are taken on that answer, so that
// what is locked is what is written. A verb that cannot name where it works does not run, and a
// command line that is refused is refused before any lock is asked for.
func skillsInstallRun(verb, did string, use installUseCase, deps skills.Deps, args []string, stdout, stderr io.Writer, exit func(int)) {
	parsed, err := skillsInstallSpec(verb).parse(args)
	if err != nil {
		refuseSkillsUsage(err, stderr, exit)
		return
	}
	root, ok := workingProject(verb, did, deps, stderr, exit)
	if !ok {
		return
	}
	in := app.InstallInput{
		RegistryPath: parsed.value(flagRegistry, defaultSkillsRegistry),
		SourceRoot:   parsed.value(flagSourceRoot, ""),
		ProjectID:    skills.ProjectID(parsed.value(flagProjectID, "")),
		ProjectRoot:  root,
	}
	held, err := skills.AcquireLocks(verb, deps.Locker, append(skills.OverlayLocks(verb, in.RegistryPath), skills.ProjectLocks(verb, root)...))
	if err != nil {
		refuseLocks(err, stderr, exit)
		return
	}
	defer held.Release()
	if !installWired(verb, deps, stderr, exit) {
		return
	}
	result, err := use(app.InstallPorts{
		Registries: deps.Registries, Identity: deps.Identity, Tree: deps.Tree,
		Locks: deps.ProjectLocks, Files: deps.ReadFile, Project: deps.Project,
	}, in)
	tellUnread(result.UnreadWarning, stderr)
	if err != nil {
		refuseInstall(verb, did, err, stderr, exit)
		return
	}
	if result.NoneAdmitted {
		fmt.Fprintf(stdout, "no project-scoped skills admitted for project %q\n", result.ProjectID)
		exit(0)
		return
	}
	for _, o := range result.Skills {
		fmt.Fprintf(stdout, "%s: %s\n", o.Status, o.ID)
	}
	for _, n := range result.Notes {
		fmt.Fprintf(stdout, "note: %s\n", n)
	}
	exit(0)
}

// workingProject names the directory a verb installs into, which must be an absolute path. It
// reports false, having told why and exited, when it cannot: nothing was locked and nothing done.
func workingProject(verb, did string, deps skills.Deps, stderr io.Writer, exit func(int)) (string, bool) {
	var cwd string
	err := errors.New("no working directory is wired")
	if deps.Cwd != nil {
		cwd, err = deps.Cwd()
	}
	if err == nil && filepath.IsAbs(cwd) {
		return cwd, true
	}
	reason := fmt.Sprintf("%q is not an absolute path", cwd)
	if err != nil {
		reason = err.Error()
	}
	fmt.Fprintf(stderr, "error: skills %s: cannot resolve the project directory it works in (%s); nothing was locked and nothing was %s\n", verb, reason, did)
	exit(1)
	return "", false
}

// installWired reports whether the ports install and adopt read and write through are wired: a
// composition root that forgot one gets a refusal, not a crash.
func installWired(verb string, deps skills.Deps, stderr io.Writer, exit func(int)) bool {
	switch {
	case deps.Tree == nil:
		fmt.Fprintf(stderr, "error: skills %s: no skill tree is wired, so it cannot read the skills of the overlay\n", verb)
	case deps.Project == nil:
		fmt.Fprintf(stderr, "error: skills %s: no project file system is wired, so it cannot read or write files\n", verb)
	case deps.ProjectLocks == nil:
		fmt.Fprintf(stderr, "error: skills %s: no project lock store is wired, so it cannot read the lock of the project\n", verb)
	default:
		return true
	}
	exit(1)
	return false
}

// refuseInstall says why install or adopt stopped, and exits 1. A registry that cannot be used, the
// sources that are missing, the reasons of the ownership rules and what the executor reported are
// told as the lines they are; every other refusal is one line.
func refuseInstall(verb, did string, err error, stderr io.Writer, exit func(int)) {
	var (
		registry *app.RegistryError
		missing  *app.SourcesMissingError
		refusal  *app.PlanRefusal
		failed   *app.ExecutionError
	)
	switch {
	case errors.As(err, &registry):
		refuseRegistry(err, false, stderr, exit)
		return
	case errors.As(err, &missing):
		for _, op := range missing.Missing {
			fmt.Fprintf(stderr, "error: skill %s: source dir not found: %s\n", op.SkillID, op.Src)
		}
		fmt.Fprintf(stderr, "error: %v\n", err)
	case errors.As(err, &refusal):
		for _, reason := range refusal.Reasons {
			fmt.Fprintf(stderr, "error: %s\n", reason)
		}
		fmt.Fprintf(stderr, "error: skills %s: refused, so nothing was %s\n", verb, did)
	case errors.As(err, &failed):
		fmt.Fprint(stderr, failed.Report)
		fmt.Fprintf(stderr, "error: %v\n", err)
	default:
		fmt.Fprintf(stderr, "error: %v\n", err)
	}
	exit(1)
}
