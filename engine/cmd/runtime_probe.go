package main

// runtime probe: 'runtime probe [--target claude|codex|pi|all]'. It prints, as
// JSON, what the presence prober (engine/capability) finds: for the selected
// runtimes a credentials signal, and for every run the memory and Gentle AI
// review signals a workflow records. The prober is the one the workflow verbs
// use, pointed at the real home directory and PATH.
//
// The action is read-only and stat-only. It never opens, reads, hashes, or
// prints the contents of a credentials, database, or registration file, it
// never runs a program, and it never names a path in its output: an available
// signal only says a file or binary is present, and its detail says what that
// does not prove (a credentials file that exists is not an authenticated
// session). The static test in runtime_probe_test.go pins this source to
// os.UserHomeDir and os.Getenv, with no file access of its own, and the prober's
// own static test pins the rest; engine/runtime's Phase 7 import policy keeps
// os/exec and the network out of both.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// probeReportVersion is the wire version of the probe report.
const probeReportVersion = 1

// probeTimeout bounds one probe run. The checks are stats of a handful of
// paths, so this is generous; it exists so a hung filesystem cannot hang the
// command.
const probeTimeout = 5 * time.Second

// probeTargetAll is the --target value that selects every runtime's credentials
// signal.
const probeTargetAll = "all"

// probeReport is what 'runtime probe' prints.
type probeReport struct {
	Version      int                    `json:"version"`
	Observations []workflow.Observation `json:"observations"`
}

// runtimeProbeEnv returns the home directory and PATH the process runs with.
// The home is empty when it cannot be determined, which the prober reads as
// "unknown", never as a location to guess.
func runtimeProbeEnv() (home, path string) {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return home, os.Getenv("PATH")
}

// runRuntimeProbe implements 'runtime probe'. Exit 0 prints the report; exit 2
// refuses an unknown --target value; exit 1 is a usage error (an unknown flag, a
// --target without its value, or a positional argument) or a failed write of the
// report. Every exit(n) is followed by a return, because tests inject a
// non-terminating exit.
func runRuntimeProbe(args []string, home, path string, stdout, stderr io.Writer, exit func(int)) {
	target, err := parseProbeArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "error: runtime probe: %v\n", err)
		exit(1)
		return
	}
	names, err := probeCapabilities(target)
	if err != nil {
		fmt.Fprintf(stderr, "error: runtime probe: %v\n", err)
		exit(2)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	observations, err := capability.PresenceProber{Home: home, Path: path}.Probe(ctx, names)
	if err != nil {
		fmt.Fprintf(stderr, "error: runtime probe: %v\n", err)
		exit(1)
		return
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(probeReport{Version: probeReportVersion, Observations: observations}); err != nil {
		fmt.Fprintf(stderr, "error: runtime probe: %v\n", err)
		exit(1)
		return
	}
	if _, err := stdout.Write(buf.Bytes()); err != nil {
		fmt.Fprintf(stderr, "error: runtime probe: writing report: %v\n", err)
		exit(1)
		return
	}
	exit(0)
}

// parseProbeArgs parses --target, whose default is "all"; the last occurrence
// wins, and the value is trimmed, as for 'runtime capabilities'. Only --target
// exists: the flags of the lifecycle actions are unknown flags here.
func parseProbeArgs(args []string) (string, error) {
	target := probeTargetAll
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--target":
			if i+1 >= len(args) {
				return "", fmt.Errorf("--target requires a value")
			}
			i++
			target = strings.TrimSpace(args[i])
		case strings.HasPrefix(a, "-"):
			return "", fmt.Errorf("unknown flag %q", a)
		default:
			return "", fmt.Errorf("unexpected argument %q", a)
		}
	}
	return target, nil
}

// probeCapabilities returns the capability names to probe, in report order: the
// credentials signal of each selected runtime, then the memory and review
// signals a workflow records.
func probeCapabilities(target string) ([]string, error) {
	targets := []string{target}
	if target == probeTargetAll {
		targets = []string{capability.TargetClaude, capability.TargetCodex, capability.TargetPi}
	}
	var names []string
	for _, t := range targets {
		name, ok := capability.CredentialsCapability(t)
		if !ok {
			return nil, fmt.Errorf("unknown target %q (expected %s, %s, %s, or %s)", target, capability.TargetClaude, capability.TargetCodex, capability.TargetPi, probeTargetAll)
		}
		names = append(names, name)
	}
	return append(names,
		capability.CapabilityMemoryEngram,
		capability.CapabilityMemoryLongtermMem,
		capability.CapabilityMemoryProcedural,
		capability.CapabilityGentleAIReview,
	), nil
}
