package main

import (
	"errors"
	"fmt"
	"os"

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
func resolveProjectFlag(cmd, given string) (string, int) {
	if given == "" {
		id, err := resolveFromWorkingDirectory()
		if err != nil {
			fmt.Fprintf(os.Stderr, "longterm-mem: %s: --project is required: it could not be resolved from the working directory: %v\n", cmd, err)
			return "", exitUsage
		}
		fmt.Fprintf(os.Stderr, "longterm-mem: %s: --project not given, resolved %q from the working directory (via the %s rule)\n", cmd, id.Project, id.Rule)
		return id.Project, exitOK
	}

	wd, err := os.Getwd()
	if err != nil {
		// The correspondence check is a diagnostic, never a gate: an
		// unreadable working directory must not turn an explicit
		// --project into a failure.
		return given, exitOK
	}
	c, err := projectid.CheckCorrespondence(wd, given)
	if err != nil {
		if !errors.Is(err, projectid.ErrNotARepository) {
			fmt.Fprintf(os.Stderr, "WARN %s: could not check --project against the working directory: %v\n", cmd, err)
		}
		return given, exitOK
	}
	if w := c.Warning(); w != "" {
		fmt.Fprintf(os.Stderr, "WARN %s: %s\n", cmd, w)
	}
	return given, exitOK
}

// resolveFromWorkingDirectory answers the CLI's own working directory,
// which is the directory the operator is standing in.
//
// This seam exists for the CLI and only for the CLI. Do NOT reach for it
// from internal/mcpserver to validate or default an MCP tool's project
// field: the MCP server is launched by a runtime (an editor, an agent
// host) whose working directory has no relationship to the project a given
// call is asking about, so checking a tool call against the server's cwd
// would reject correct calls and, worse, "correct" them into the wrong
// project. The MCP tools keep their explicit project field for exactly
// that reason.
func resolveFromWorkingDirectory() (projectid.Identity, error) {
	wd, err := os.Getwd()
	if err != nil {
		return projectid.Identity{}, fmt.Errorf("reading the working directory: %w", err)
	}
	return projectid.Resolve(wd)
}
