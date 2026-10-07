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
	dispatchVerb(verb, deps, stdout, stderr, exit)
}

// dispatchVerb is what is left of the dispatcher: a verb of `engine skills` that is none of those the
// adapter runs is refused, and so is a missing one.
func dispatchVerb(verb string, deps Deps, stdout, stderr io.Writer, exit func(int)) {
	switch verb {
	case "":
		fmt.Fprintln(stderr, "error: skills requires a verb: list, status, validate, install, adopt, add, remove, sync-manifest, lint, approve, project-register, project-revise, project-status, project-retire")
		exit(1)
	default:
		fmt.Fprintf(stderr, "error: unknown skills verb %q (supported: list, status, validate, install, adopt, add, remove, sync-manifest, lint, approve, project-register, project-revise, project-status, project-retire)\n", verb)
		exit(1)
	}
}
