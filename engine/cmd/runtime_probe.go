package main

// runtime probe: 'runtime probe [--target claude|codex|pi|all]'. It prints, as
// JSON, what the presence prober (engine/capability/presence) finds: for the selected
// runtimes a credentials signal, and for every run the memory and Gentle AI
// review signals a workflow records. The prober is the one the workflow verbs
// use, pointed at the real home directory and PATH.
//
// The action is read-only and stat-only. It never opens, reads, hashes, or
// prints the contents of a credentials, database, or registration file, it
// never runs a program, and it never names a path in its output: an available
// signal only says a file or binary is present, and its detail says what that
// does not prove (a credentials file that exists is not an authenticated
// session). The static test in runtime_probe_test.go pins this source to no use of
// package os at all (the home and PATH are the deps', which main builds from the
// process), with no file access of its own, and the prober's own static test pins the
// rest; engine/runtime's Phase 7 import policy keeps os/exec and the network out of both.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability/presence"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// probeReportVersion is the wire version of the probe report.
const probeReportVersion = 1

// defaultProbeTimeout bounds one probe run (deps.probeTimeout). The checks are stats of a
// handful of paths, so this is generous; it exists so a hung filesystem cannot hang the
// command. A blocked stat cannot be interrupted, so the probe runs in its own goroutine and the
// command stops waiting at the deadline (see probeWithin).
const defaultProbeTimeout = 5 * time.Second

// probeTargetAll is the --target value that selects every runtime's credentials
// signal.
const probeTargetAll = "all"

// probeReport is what 'runtime probe' prints.
type probeReport struct {
	Version      int                    `json:"version"`
	Observations []workflow.Observation `json:"observations"`
}

// runtimeProbeEnv returns the home directory and PATH the process runs with, as the deps give
// them. The home is empty when it cannot be determined, which the prober reads as "unknown",
// never as a location to guess.
func runtimeProbeEnv(d deps) (home, path string) {
	home, err := d.userHomeDir()
	if err != nil {
		home = ""
	}
	return home, d.getenv("PATH")
}

// runRuntimeProbe implements 'runtime probe'. Exit 0 prints the report; exit 2
// refuses an unknown --target value; exit 1 is a usage error (an unknown flag, a
// --target without its value, or a positional argument) or a failed write of the
// report. Every exit(n) is followed by a return, because tests inject a
// non-terminating exit.
func runRuntimeProbe(d deps, args []string, stdout, stderr io.Writer, exit func(int)) {
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

	home, path := runtimeProbeEnv(d)
	observations, err := probeWithin(presence.Prober{Home: home, Path: path, ProbeFS: d.probeFS}, names, d.probeTimeout)
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
		name, ok := presence.CredentialsCapability(t)
		if !ok {
			return nil, fmt.Errorf("unknown target %q (expected %s, %s, %s, or %s)", target, capability.TargetClaude, capability.TargetCodex, capability.TargetPi, probeTargetAll)
		}
		names = append(names, name)
	}
	return append(names,
		presence.CapabilityMemoryEngram,
		presence.CapabilityMemoryLongtermMem,
		presence.CapabilityMemoryProcedural,
		presence.CapabilityGentleAIReview,
	), nil
}

// probeWithin runs prober in its own goroutine and waits at most timeout. A
// probe that does not finish in time, for example because a stat is blocked on
// an unresponsive filesystem, is reported as every capability unavailable with
// the reason; the blocked goroutine is abandoned, which a short-lived command
// can afford.
func probeWithin(prober workflow.DependencyProber, names []string, timeout time.Duration) ([]workflow.Observation, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	type result struct {
		observations []workflow.Observation
		err          error
	}
	done := make(chan result, 1)
	go func() {
		observations, err := prober.Probe(ctx, names)
		done <- result{observations, err}
	}()
	select {
	case r := <-done:
		if r.err == nil || !errors.Is(r.err, context.DeadlineExceeded) {
			return r.observations, r.err
		}
	case <-ctx.Done():
	}
	detail := fmt.Sprintf("the probe did not finish within %s (a filesystem may be unresponsive)", timeout)
	observations := make([]workflow.Observation, len(names))
	for i, name := range names {
		observations[i] = workflow.Observation{Capability: name, Status: workflow.ObservationUnavailable, Detail: detail}
	}
	return observations, nil
}
