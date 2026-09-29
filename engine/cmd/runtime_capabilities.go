package main

// runtime capabilities: 'runtime capabilities [--target
// claude|codex|pi|opencode|all]'. It prints, as JSON, what each runtime
// adapter declares it supports (engine/capability): one claim per capability,
// with the tests that prove it or the limit that bounds it.
//
// The action is strictly read-only and declarative. It never constructs an
// adapter, resolves a configuration root, reads HOME or any other
// environment variable, or opens a file: it prints values compiled into the
// binary. That contract is enforced statically, by the import allowlist in
// runtime_capabilities_test.go (no os, no engine/runtime, no settings), and
// engine/runtime's Phase 7 import policy keeps os/exec and the network out of
// this file too.

import (
	"fmt"
	"io"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
)

// capabilitiesTargetAll is the --target value that selects every declaration.
const capabilitiesTargetAll = "all"

// runRuntimeCapabilities implements 'runtime capabilities'. Exit 0 prints the
// report; exit 2 refuses an unknown --target value; exit 1 is a usage error
// (an unknown flag, a --target without its value, or a positional argument,
// as for the other runtime actions) or a failed write of the report. Every
// exit(n) is followed by a return, because tests inject a non-terminating
// exit.
func runRuntimeCapabilities(args []string, stdout, stderr io.Writer, exit func(int)) {
	target, code, err := parseCapabilitiesArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "error: runtime capabilities: %v\n", err)
		exit(code)
		return
	}

	declarations, err := selectDeclarations(target)
	if err != nil {
		fmt.Fprintf(stderr, "error: runtime capabilities: %v\n", err)
		exit(2)
		return
	}

	data, err := capability.Report{Version: capability.ReportVersion, Declarations: declarations}.Marshal()
	if err != nil {
		fmt.Fprintf(stderr, "error: runtime capabilities: %v\n", err)
		exit(1)
		return
	}
	if _, err := stdout.Write(data); err != nil {
		fmt.Fprintf(stderr, "error: runtime capabilities: writing report: %v\n", err)
		exit(1)
		return
	}
	exit(0)
}

// parseCapabilitiesArgs parses --target, whose default is "all", and the last
// occurrence wins as it does for the other runtime actions. On failure it
// returns the exit code to use, always 1: a bad command line is a usage error,
// as it is for the other runtime actions; an unknown --target value is refused
// later, with exit 2. Only --target exists here; the flags of the lifecycle actions
// (--config-root, --component, --state-dir) are unknown flags, because this
// action reads no configuration.
func parseCapabilitiesArgs(args []string) (target string, code int, err error) {
	target = capabilitiesTargetAll
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--target":
			if i+1 >= len(args) {
				return "", 1, fmt.Errorf("--target requires a value")
			}
			i++
			// Trimmed like runtime.ParseTarget trims the same flag on the
			// lifecycle actions.
			target = strings.TrimSpace(args[i])
		case strings.HasPrefix(a, "-"):
			return "", 1, fmt.Errorf("unknown flag %q", a)
		default:
			return "", 1, fmt.Errorf("unexpected argument %q", a)
		}
	}
	return target, 0, nil
}

// selectDeclarations returns every declaration for "all", or the single
// declaration of one target.
func selectDeclarations(target string) ([]capability.Declaration, error) {
	if target == capabilitiesTargetAll {
		return capability.All(), nil
	}
	d, err := capability.Declare(target)
	if err != nil {
		return nil, fmt.Errorf("unknown target %q (expected %s, or %s)", target, strings.Join(capability.Targets(), ", "), capabilitiesTargetAll)
	}
	return []capability.Declaration{d}, nil
}
