package main

import (
	"fmt"
	"io"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings/settingsfile"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/status"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/status/fsfiles"
)

// runStatus is the entry point of the 'status' subcommand: it builds the use case over the
// machine's files, asks where the installation is (the HOME of the process and the directory it
// runs in) and exits with the code of the outcome:
//
//	0 — every check passed (healthy).
//	1 — at least one hard check FAILED.
//	2 — no hard failure, but at least one check is DEGRADED (e.g. the registry
//	    exists but its scoped block is missing). Distinct from 1 so callers can
//	    tell "broken" from "present-but-needs-attention".
//
// Two options narrow it, for the callers that judge one thing (the doctor of the overlay script asks
// for the registry of a project): --cwd names the directory whose registry is looked for instead of
// the working directory, and --only registry reports that registry alone, with the exit code of
// that one check. Anything else on the line is ignored, as it always was.
func runStatus(p process, d deps, args []string) {
	opts, err := parseStatusArgs(args)
	if err != nil {
		fmt.Fprintf(p.stderr, "error: %v\n", err)
		p.exit(1)
		return
	}
	cwd := opts.cwd
	if cwd == "" {
		// A directory that cannot be determined is none: the registry is then not looked for.
		cwd, _ = d.workingDir()
	}
	if opts.only == onlyRegistry {
		if cwd == "" {
			fmt.Fprintln(p.stderr, "error: status --only registry: the working directory cannot be determined; give --cwd")
			p.exit(1)
			return
		}
		report := status.Report{Checks: []status.Check{newStatusService().Registry(cwd)}}
		renderReport(p.stdout, report)
		if code := statusExitCode(report.Outcome()); code != 0 {
			p.exit(code)
		}
		return
	}
	outcome := statusCore(p.stdout, newStatusService(), status.Request{Home: d.env("HOME"), Cwd: cwd})
	if code := statusExitCode(outcome); code != 0 {
		p.exit(code)
	}
}

// onlyRegistry is the one value of --only: the registry of the project.
const onlyRegistry = "registry"

// statusArgs are the two options of 'status', read.
type statusArgs struct {
	only string
	cwd  string
}

// parseStatusArgs reads --only and --cwd. A flag of them that has no value, and a value of --only
// that names no check, are refused, saying which; every other argument is ignored.
func parseStatusArgs(args []string) (statusArgs, error) {
	var out statusArgs
	for i := 0; i < len(args); i++ {
		if args[i] != "--only" && args[i] != "--cwd" {
			continue
		}
		if i+1 >= len(args) {
			return statusArgs{}, fmt.Errorf("status: flag %q requires a value", args[i])
		}
		i++
		if args[i-1] == "--cwd" {
			out.cwd = args[i]
			continue
		}
		if args[i] != onlyRegistry {
			return statusArgs{}, fmt.Errorf("status: --only %q names no check; expected %s", args[i], onlyRegistry)
		}
		out.only = args[i]
	}
	return out, nil
}

// newStatusService is the status use case over the files of the machine: the one place the
// adapters of its two ports are named.
func newStatusService() status.Service {
	return status.Service{Files: fsfiles.Files{}, Settings: settingsfile.Reader{}}
}

// statusCore runs the checks of an installation and writes the report to stdout, one line per
// check. It is runStatus without the machine: the service and the request are handed in.
func statusCore(stdout io.Writer, service status.Service, req status.Request) status.Outcome {
	report := service.Check(req)
	renderReport(stdout, report)
	return report.Outcome()
}

// renderReport writes a line for each check: its level, its label and, when it has one, its note.
func renderReport(w io.Writer, report status.Report) {
	for _, c := range report.Checks {
		tag := "OK  "
		switch c.Level {
		case status.Warn:
			tag = "WARN"
		case status.Fail:
			tag = "FAIL"
		}
		if c.Note != "" {
			fmt.Fprintf(w, "[%s] %s — %s\n", tag, c.Label, c.Note)
		} else {
			fmt.Fprintf(w, "[%s] %s\n", tag, c.Label)
		}
	}
}

// statusExitCode is the exit code of an outcome of 'status'.
func statusExitCode(outcome status.Outcome) int {
	switch outcome {
	case status.Failed:
		return 1
	case status.Degraded:
		return 2
	}
	return 0
}
