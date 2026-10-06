package skills

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
)

// SkillsCoreAt is the testable CLI core for the verbs of `engine skills` that have not yet moved
// behind a use case (Phase 9 unit H20): install and adopt, and the four verbs of a project
// (project-register, project-revise, project-status, project-retire).
// list, status, lint, validate, add, remove, sync-manifest and approve are not dispatched here:
// they are use cases in engine/skills/app, run by the CLI adapter in engine/cmd, which reads their
// arguments with its one strict parser and takes the overlay lock they need.
// Unknown or empty verbs fail loud (exit 1), mirroring the prespec pattern (ADR-2).
// No global state; all I/O is injected through deps. deps.Now returns the current time as an
// RFC 3339 UTC timestamp for the verbs that record one; the production caller passes
// the wall clock, and nil is legal for every verb it dispatches. Every verb it
// dispatches takes a lock, and refuses to run unserialized when deps.Locker is nil.
//
// deps.Registries is how every verb that works on the registry reads it: the composition root
// builds one, the verbs know no file format.
func SkillsCoreAt(verb string, args []string, deps Deps, stdout, stderr io.Writer, exit func(int)) {
	// install writes into the working directory. It is resolved here, once, before
	// any lock is asked for, and the lock and the verb are both given this answer:
	// a second call could return another directory, and a directory that cannot be
	// named cannot be locked. A verb that cannot name where it writes does not run.
	installRoot := ""
	if verb == "install" || verb == "adopt" {
		cwd, err := workingDirectory(deps)
		if err != nil || !filepath.IsAbs(cwd) {
			reason := fmt.Sprintf("%q is not an absolute path", cwd)
			if err != nil {
				reason = err.Error()
			}
			did := map[string]string{"install": "installed", "adopt": "adopted"}[verb]
			fmt.Fprintf(stderr, "error: skills %s: cannot resolve the project directory it works in (%s); nothing was locked and nothing was %s\n", verb, reason, did)
			exit(1)
			return
		}
		installRoot = cwd
	}
	runLocked(verb, args, installRoot, deps, stdout, stderr, exit)
}

// workingDirectory is the directory the process works in, asked of the port the composition root
// gave. A root that gave none has not wired it, which is an answer a verb can refuse with.
func workingDirectory(deps Deps) (string, error) {
	if deps.Cwd == nil {
		return "", errors.New("no working directory is wired")
	}
	return deps.Cwd()
}

// runLocked runs a verb under the locks it needs. A verb that cannot get them does not run: the
// refusal is told and the verb exits with the code that answers it (see AcquireLocks).
func runLocked(verb string, args []string, installRoot string, deps Deps, stdout, stderr io.Writer, exit func(int)) {
	release, ok := acquireLocks(verb, args, installRoot, deps.Locker, stderr, exit)
	if !ok {
		return
	}
	defer release()
	dispatchVerb(verb, args, installRoot, deps, stdout, stderr, exit)
}

// needsProject lists the verbs that read or write the files of a project through the Deps'
// Project: the verbs that install into a project or keep its lock.
var needsProject = map[string]bool{
	"install": true, "adopt": true,
	"project-register": true, "project-revise": true, "project-status": true, "project-retire": true,
}

// dispatchVerb runs the verb. The locks, if it needs any, are already held, and
// installRoot is the directory install was resolved to and locked.
func dispatchVerb(verb string, args []string, installRoot string, deps Deps, stdout, stderr io.Writer, exit func(int)) {
	readFile, registries := deps.ReadFile, deps.Registries
	// A composition root that forgot a port: a refusal, not a crash.
	if deps.Tree == nil && (verb == "install" || verb == "adopt") {
		fmt.Fprintf(stderr, "error: skills %s: no skill tree is wired, so it cannot read the skills of the overlay\n", verb)
		exit(1)
		return
	}
	if deps.Project == nil && needsProject[verb] {
		fmt.Fprintf(stderr, "error: skills %s: no project file system is wired, so it cannot read or write files\n", verb)
		exit(1)
		return
	}
	switch verb {
	case "install":
		env := installEnvOf(deps, func() (string, error) { return installRoot, nil })
		env.readProject = readFile
		renderInstall(env, args, stdout, stderr, exit)
	case "adopt":
		env := installEnvOf(deps, func() (string, error) { return installRoot, nil })
		env.readProject = readFile
		renderAdopt(env, args, stdout, stderr, exit)
	case "project-register":
		RenderProjectRegisterCore(stripVerb(args, "project-register"), readFile, registries, deps.Project.Stat, deps.Project.ResolvePath, deps.Project, stdout, stderr, exit)
	case "project-revise":
		RenderProjectReviseCore(stripVerb(args, "project-revise"), readFile, deps.Project.ReadDir, deps.Project.Stat, deps.Project.ResolvePath, deps.Project, stdout, stderr, exit)
	case "project-status":
		RenderProjectStatusCore(stripVerb(args, "project-status"), readFile, registries, deps.Project.ReadDir, deps.Project.ResolvePath, stdout, stderr, exit)
	case "project-retire":
		RenderProjectRetireCore(stripVerb(args, "project-retire"), readFile, registries, deps.Project.ReadDir, deps.Project.Stat, deps.Project.ResolvePath, deps.Project, stdout, stderr, exit)
	case "":
		fmt.Fprintln(stderr, "error: skills requires a verb: list, status, validate, install, adopt, add, remove, sync-manifest, lint, approve, project-register, project-revise, project-status, project-retire")
		exit(1)
	default:
		fmt.Fprintf(stderr, "error: unknown skills verb %q (supported: list, status, validate, install, adopt, add, remove, sync-manifest, lint, approve, project-register, project-revise, project-status, project-retire)\n", verb)
		exit(1)
	}
}

// stripVerb removes the first occurrence of verb from args (used so that
// SkillsCore can dispatch to AddCore / RemoveCore without the verb token
// appearing as a spurious positional argument in the downstream flag parser).
func stripVerb(args []string, verb string) []string {
	for i, a := range args {
		if a == verb {
			out := make([]string, 0, len(args)-1)
			out = append(out, args[:i]...)
			out = append(out, args[i+1:]...)
			return out
		}
	}
	return args
}
