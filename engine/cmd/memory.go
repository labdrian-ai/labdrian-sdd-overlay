package main

// memory subcommand: 'memory plan --profile <name> [--goal <path>]
// [--goal-directive <path>] [--handoff-directive <path>]'. It is entirely
// read-only: it resolves a workflow profile's default memory directive,
// narrows it with an optional Goal-supplied directive and an optional
// handoff-supplied directive (in that order), and prints the resulting
// query plan as JSON. No verb here executes a memory query or grants a
// memory write; that belongs to a runtime adapter outside this package
// (Phase 7).

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/goal"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"
)

// runMemory implements the 'memory <verb>' subcommand.
func runMemory(p process, args []string) {
	runMemoryCore(args, p.stdout, p.stderr, p.exit)
}

// runMemoryCore is the testable core of the memory subcommand. Every exit(n)
// is followed by a return, because tests inject a non-terminating exit.
func runMemoryCore(args []string, stdout, stderr io.Writer, exit func(int)) {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "error: memory requires a verb: plan")
		exit(1)
		return
	}
	switch args[0] {
	case "plan":
		runMemoryPlan(args[1:], stdout, stderr, exit)
	default:
		fmt.Fprintf(stderr, "error: memory: unknown verb %q (expected plan)\n", args[0])
		exit(1)
	}
}

// memoryPlanOpts are the parsed 'memory plan' flags.
type memoryPlanOpts struct {
	profile, goalFile, goalDirectiveFile, handoffDirectiveFile string
}

// parseMemoryPlanArgs parses --profile/--goal/--goal-directive/
// --handoff-directive. It fails loud on an unknown flag, a flag without its
// value, a positional argument, or a missing --profile.
func parseMemoryPlanArgs(args []string) (memoryPlanOpts, error) {
	var o memoryPlanOpts
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--profile", "--goal", "--goal-directive", "--handoff-directive":
			if i+1 >= len(args) {
				return o, fmt.Errorf("%s requires a value", a)
			}
			i++
			switch a {
			case "--profile":
				o.profile = args[i]
			case "--goal":
				o.goalFile = args[i]
			case "--goal-directive":
				o.goalDirectiveFile = args[i]
			case "--handoff-directive":
				o.handoffDirectiveFile = args[i]
			}
		default:
			if strings.HasPrefix(a, "-") {
				return o, fmt.Errorf("unknown flag %q", a)
			}
			return o, fmt.Errorf("unexpected argument %q", a)
		}
	}
	if o.profile == "" {
		return o, fmt.Errorf("--profile is required")
	}
	return o, nil
}

// runMemoryPlan implements 'memory plan'. Exit 0 on a resolved plan, exit 2
// on a refused or invalid input (unknown profile, unparseable Goal or
// directive file, or a narrowing refusal), exit 1 on a usage error or a failed
// write of the plan.
func runMemoryPlan(args []string, stdout, stderr io.Writer, exit func(int)) {
	o, err := parseMemoryPlanArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "error: memory plan: %v\n", err)
		exit(1)
		return
	}

	var base memoryscope.Directive
	profile, err := workflowprofile.Resolve(o.profile)
	if err == nil {
		base, err = memoryscope.DefaultFor(profile)
	}
	if err != nil {
		fmt.Fprintf(stderr, "error: memory plan: %v\n", err)
		exit(2)
		return
	}

	var projectID, goalID string
	if o.goalFile != "" {
		data, err := os.ReadFile(o.goalFile)
		if err != nil {
			fmt.Fprintf(stderr, "error: memory plan: read --goal: %v\n", err)
			exit(2)
			return
		}
		g, err := goal.Parse(data)
		if err != nil {
			fmt.Fprintf(stderr, "error: memory plan: parse --goal: %v\n", err)
			exit(2)
			return
		}
		projectID = g.ProjectID
		goalID = g.GoalID
	}

	narrowers, err := loadMemoryDirectiveNarrowers(o)
	if err != nil {
		fmt.Fprintf(stderr, "error: memory plan: %v\n", err)
		exit(2)
		return
	}

	plan, err := memoryscope.Resolve(base, projectID, goalID, narrowers...)
	if err != nil {
		fmt.Fprintf(stderr, "error: memory plan: %v\n", err)
		exit(2)
		return
	}

	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "error: memory plan: %v\n", err)
		exit(2)
		return
	}
	if _, err := stdout.Write(append(data, '\n')); err != nil {
		fmt.Fprintf(stderr, "error: memory plan: writing plan: %v\n", err)
		exit(1)
		return
	}
	exit(0)
}

// loadMemoryDirectiveNarrowers reads and strictly parses the optional
// --goal-directive and --handoff-directive files, in that order, so the
// caller narrows a profile default first with the Goal-supplied directive
// and then with the handoff-supplied one.
func loadMemoryDirectiveNarrowers(o memoryPlanOpts) ([]memoryscope.Directive, error) {
	var narrowers []memoryscope.Directive
	for _, f := range []struct{ label, path string }{
		{"goal-directive", o.goalDirectiveFile},
		{"handoff-directive", o.handoffDirectiveFile},
	} {
		if f.path == "" {
			continue
		}
		data, err := os.ReadFile(f.path)
		if err != nil {
			return nil, fmt.Errorf("read --%s: %w", f.label, err)
		}
		d, err := memoryscope.ParseDirective(data)
		if err != nil {
			return nil, fmt.Errorf("parse --%s: %w", f.label, err)
		}
		narrowers = append(narrowers, d)
	}
	return narrowers, nil
}
