package main

import (
	"fmt"
	"io"
	"os"

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
func runStatus(_ []string) {
	// A directory that cannot be determined is none: the registry is then not looked for.
	cwd, _ := os.Getwd()
	outcome := statusCore(os.Stdout, newStatusService(), status.Request{Home: os.Getenv("HOME"), Cwd: cwd})
	if code := statusExitCode(outcome); code != 0 {
		os.Exit(code)
	}
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
