package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/adoption"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/projectid"
)

// projectFlagUsage is the shared --project flag description. The flag is
// optional now: omitted, the project is resolved from the working
// directory (internal/projectid).
const projectFlagUsage = "project name (default: resolved from the working directory)"

// resolveProjectFlag turns a possibly-empty --project into the project a
// command should act on, and returns exitOK or the exit code the caller
// must return.
//
// Two behaviours meet here.
//
// When --project is omitted, the project is resolved from the working
// directory through projectid's chain (declared file, normalized origin
// remote, realpath of the git common dir). When that resolution fails, the
// command refuses exactly as it did before -- an unresolved project must
// never become an empty one, because Engram is queried
// `WHERE project = ?` with whatever it is handed -- but the refusal now
// says why resolution failed instead of only restating the flag's name.
//
// When --project IS given and the working directory is inside a git
// repository whose canonical identity does not correspond to it, the
// mismatch is reported and the command proceeds. That is the moment a
// fragmenting call is detectable, and the argument for warning rather than
// refusing is written out in projectid.Correspondence.Warning: the failure
// being prevented is a silent one, and an operator naming another project
// on purpose is legitimate and must stay possible.
func (c commands) resolveProjectFlag(cmd, given string) (string, int) {
	return c.resolveProjectFlagWith(cmd, given, adoption.RecordDerived)
}

// resolveProjectFlagReadOnly is resolveProjectFlag for a command that
// promises not to change the repository it is inspecting.
//
// doctor's own doc comment says it runs read-only diagnostic checks, and it
// did not: resolving its project recorded this repository's derived names
// in the identity ledger, so the first run of a diagnostic on a suspect
// repository changed the state the operator was about to inspect. A
// diagnostic you must mutate the subject to run is a diagnostic you cannot
// trust the first result of.
//
// Only the WRITE is skipped. The ledger is still READ, because reading is
// what makes an already-established name win over a freshly derived one --
// suppressing that would make doctor report a different project than every
// other command resolves, which is a worse lie than the one being fixed.
func (c commands) resolveProjectFlagReadOnly(cmd, given string) (string, int) {
	return c.resolveProjectFlagWith(cmd, given, adoption.DoNotRecordDerived)
}

func (c commands) resolveProjectFlagWith(cmd, given string, policy adoption.RecordPolicy) (string, int) {
	if given == "" {
		return c.adoptProject(cmd, policy)
	}
	return checkGivenProject(cmd, given)
}

// adoptProject is the project of a command that was given none: resolved from the working directory, with what
// is already stored deciding which of the names the repository derives is used.
//
// Adoption runs only on this path, where longterm-mem is choosing the name itself. An explicit --project is the
// operator choosing it, and there the correspondence warning (checkGivenProject) already says when that choice
// disagrees with the directory.
//
// This seam exists for the CLI and only for the CLI. Do NOT reach for it from internal/mcpserver to default or
// validate an MCP tool's project field: the MCP server is launched by a runtime (an editor, an agent host) whose
// working directory has no relationship to the project a given call is asking about, so adopting from its cwd
// would bind observations to wherever the host happened to start -- the exact misattribution this whole
// mechanism exists to prevent.
func (c commands) adoptProject(cmd string, policy adoption.RecordPolicy) (string, int) {
	wd, err := os.Getwd()
	if err != nil {
		return refuseUnresolved(cmd, fmt.Errorf("reading the working directory: %w", err))
	}
	adopted, notes, err := c.adoption().Adopt(wd, policy)
	if err != nil {
		return refuseUnresolved(cmd, err)
	}
	reportAdoption(cmd, adopted, notes)
	return adopted.Identity.Project, exitOK
}

// checkGivenProject keeps the project the operator named and warns when the working directory's repository
// resolves to another. The check is a diagnostic, never a gate: an unreadable working directory must not turn an
// explicit --project into a failure.
func checkGivenProject(cmd, given string) (string, int) {
	wd, err := os.Getwd()
	if err != nil {
		return given, exitOK
	}
	check, err := identityRules().CheckCorrespondence(wd, given)
	if err != nil {
		if !errors.Is(err, projectid.ErrNotARepository) {
			fmt.Fprintf(os.Stderr, "WARN %s: could not check --project against the working directory: %v\n", cmd, err)
		}
		return given, exitOK
	}
	if w := check.Warning(); w != "" {
		fmt.Fprintf(os.Stderr, "WARN %s: %s\n", cmd, w)
	}
	return given, exitOK
}

// refuseUnresolved is the refusal of a command that was given no --project and could not resolve one.
func refuseUnresolved(cmd string, err error) (string, int) {
	fmt.Fprintf(os.Stderr, "longterm-mem: %s: --project is required: it could not be resolved from the working directory: %v\n", cmd, err)
	return "", exitUsage
}

// reportAdoption tells the operator which project the command is acting on
// and, when it applies, what integrating that identity still owes.
func reportAdoption(cmd string, a projectid.Adoption, notes []string) {
	if a.Adopted {
		fmt.Fprintf(os.Stderr, "longterm-mem: %s: --project not given, adopted %q from the working directory (the memory already lives under this name; derived via the %s rule)\n", cmd, a.Identity.Project, a.Identity.Rule)
	} else {
		fmt.Fprintf(os.Stderr, "longterm-mem: %s: --project not given, resolved %q from the working directory (via the %s rule)\n", cmd, a.Identity.Project, a.Identity.Rule)
	}

	if len(a.PendingIntegration) > 0 {
		// Provable, not guessed: every name here came out of THIS
		// repository's own metadata in a single read, so they are the same
		// repository beyond argument. What is not ours is the merging --
		// R-002 keeps this module's Engram connection read-only -- so the
		// operator gets the finding and the remedy rather than a silent
		// half-measure or a write we are not entitled to make.
		fmt.Fprintf(os.Stderr, "WARN %s: %s also hold memory and are the same repository as %q; they are owed an integration into it. longterm-mem reads Engram read-only and must not merge it: run `engram projects consolidate` to fold them in.\n",
			cmd, quoteAll(a.PendingIntegration), a.Identity.Project)
	}

	for _, note := range notes {
		// An unconsulted store is not an empty one. Saying so is the whole
		// point: silence here would be indistinguishable from "this really
		// is a fresh repository", which is the answer that mints the second
		// pile.
		fmt.Fprintf(os.Stderr, "WARN %s: %s, so an identity the memory already uses may have been missed\n", cmd, note)
	}
}

func quoteAll(names []string) string {
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, fmt.Sprintf("%q", n))
	}
	return strings.Join(quoted, ", ")
}
