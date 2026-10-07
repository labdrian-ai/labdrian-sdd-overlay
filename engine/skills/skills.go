package skills

import (
	"fmt"
	"io"
)

// SkillsCoreAt is the testable CLI core for the verbs of `engine skills` that have not yet moved
// behind a use case (Phase 9 unit H20): the four verbs of a project (project-register,
// project-revise, project-status, project-retire).
// list, status, lint, validate, add, remove, sync-manifest, approve, install and adopt are not
// dispatched here:
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
	runLocked(verb, args, deps, stdout, stderr, exit)
}

// runLocked runs a verb under the locks it needs. A verb that cannot get them does not run: the
// refusal is told and the verb exits with the code that answers it (see AcquireLocks).
func runLocked(verb string, args []string, deps Deps, stdout, stderr io.Writer, exit func(int)) {
	release, ok := acquireLocks(verb, args, deps.Locker, stderr, exit)
	if !ok {
		return
	}
	defer release()
	dispatchVerb(verb, args, deps, stdout, stderr, exit)
}

// needsProject lists the verbs that read or write the files of a project through the Deps'
// Project: the verbs that keep a project's lock.
var needsProject = map[string]bool{
	"project-register": true, "project-revise": true, "project-status": true, "project-retire": true,
}

// dispatchVerb runs the verb. The locks, if it needs any, are already held.
func dispatchVerb(verb string, args []string, deps Deps, stdout, stderr io.Writer, exit func(int)) {
	readFile, registries := deps.ReadFile, deps.Registries
	// A composition root that forgot a port: a refusal, not a crash.
	if deps.Project == nil && needsProject[verb] {
		fmt.Fprintf(stderr, "error: skills %s: no project file system is wired, so it cannot read or write files\n", verb)
		exit(1)
		return
	}
	switch verb {
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
