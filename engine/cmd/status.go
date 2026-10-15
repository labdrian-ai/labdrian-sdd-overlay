package main

import (
	"fmt"
	"io"
	"strings"

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
// that one check. Anything else on the line is a usage error (exit 1; 2 is the code of a degraded
// report), and so is a flag without a value (parseStatusArgs).
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

// The flags of 'status'.
const (
	flagOnly = "--only"
	flagCwd  = "--cwd"
)

// parseStatusArgs reads the arguments strictly, left to right, as the other commands of the engine
// do: every argument is --only or --cwd and the value that follows it, in the next argument or after
// an equals sign in its own (--cwd=<dir>, --only=<check>), and what is not is refused. A flag whose
// value is missing, empty or another flag ("--cwd --only registry" is a --cwd without a value, not a
// directory called --only), a value of --only that names no check, and an argument nobody named are
// each refused, saying which. The equals form is how a value that starts with two dashes is given:
// --cwd=--odd is the directory --odd. A flag given twice takes its last value.
func parseStatusArgs(args []string) (statusArgs, error) {
	var out statusArgs
	for i := 0; i < len(args); i++ {
		flag, value, hasValue := strings.Cut(args[i], "=")
		if flag != flagOnly && flag != flagCwd {
			return statusArgs{}, fmt.Errorf("status: unknown argument %q", args[i])
		}
		if !hasValue {
			// The value is the next argument, which is never a flag: "--cwd --only registry" is a --cwd
			// with no value. A value that starts with two dashes is given as --cwd=<value>.
			if i+1 >= len(args) || isStatusFlag(args[i+1]) {
				return statusArgs{}, fmt.Errorf("status: flag %q requires a value", flag)
			}
			i++
			value = args[i]
		}
		if value == "" {
			return statusArgs{}, fmt.Errorf("status: flag %q requires a value", flag)
		}
		if flag == flagCwd {
			out.cwd = value
			continue
		}
		if value != onlyRegistry {
			return statusArgs{}, fmt.Errorf("status: --only %q names no check; expected %s", value, onlyRegistry)
		}
		out.only = value
	}
	return out, nil
}

// isStatusFlag says whether arg is a flag of status, with its value after an equals sign or not.
func isStatusFlag(arg string) bool {
	name, _, _ := strings.Cut(arg, "=")
	return name == flagOnly || name == flagCwd
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
