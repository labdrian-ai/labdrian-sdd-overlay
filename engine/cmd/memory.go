package main

// memory subcommand: 'memory plan --profile <name> [--goal <path>]
// [--goal-directive <path>] [--handoff-directive <path>]', or 'memory plan
// --project <id> --workflow <id>'. It is entirely read-only. The first form
// resolves a workflow profile's default memory directive, narrows it with an
// optional Goal-supplied directive and an optional handoff-supplied directive
// (in that order), and prints the resulting query plan as JSON: it tries
// directives before a workflow records them. The second prints the plan the
// memory gate applies to a workflow, computed from its log alone
// (workflow.PlanOf, C23 M3). No verb here executes a memory query or grants a
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
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
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
	// project and workflowID name a workflow whose log the plan is read from
	// (C23 M3, decision 7); they do not combine with the flags above.
	project, workflowID string
}

// fromWorkflow reports whether the plan is read from a workflow's log.
func (o memoryPlanOpts) fromWorkflow() bool { return o.project != "" || o.workflowID != "" }

// parseMemoryPlanArgs parses --profile/--goal/--goal-directive/
// --handoff-directive, or --project/--workflow. It fails loud on an unknown
// flag, a flag without its value, a positional argument, a missing --profile
// (in the flag form), --project without --workflow or the other way round, and
// --project/--workflow given with any flag of the other form.
func parseMemoryPlanArgs(args []string) (memoryPlanOpts, error) {
	var o memoryPlanOpts
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--profile", "--goal", "--goal-directive", "--handoff-directive", "--project", "--workflow":
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
			case "--project":
				o.project = args[i]
			case "--workflow":
				o.workflowID = args[i]
			}
		default:
			if strings.HasPrefix(a, "-") {
				return o, fmt.Errorf("unknown flag %q", a)
			}
			return o, fmt.Errorf("unexpected argument %q", a)
		}
	}
	if o.fromWorkflow() {
		if o.profile != "" || o.goalFile != "" || o.goalDirectiveFile != "" || o.handoffDirectiveFile != "" {
			return o, fmt.Errorf("--project and --workflow read the plan from a workflow's log and do not combine with --profile, --goal, --goal-directive or --handoff-directive")
		}
		if o.project == "" || o.workflowID == "" {
			return o, fmt.Errorf("--project and --workflow go together")
		}
		return o, nil
	}
	if o.profile == "" {
		return o, fmt.Errorf("--profile is required")
	}
	return o, nil
}

// workflowMemoryPlan is the plan the memory gate applies to the workflow
// project/workflowID: workflow.PlanOf over its log, with the profile it goes on
// from (its snapshot, or for a version 1 log the built-in profile of its name),
// exactly as the projection computes it.
func workflowMemoryPlan(project, workflowID string) (memoryscope.Plan, error) {
	store, err := newWorkflowStore()
	if err != nil {
		return memoryscope.Plan{}, err
	}
	loaded, err := store.Load(project, workflowID)
	if err != nil {
		return memoryscope.Plan{}, err
	}
	if loaded.Classification != workflow.ClassificationOwned {
		detail := ""
		if loaded.Detail != "" {
			detail = " (" + loaded.Detail + ")"
		}
		return memoryscope.Plan{}, fmt.Errorf("workflow %s/%s is not owned: its log is %s%s", project, workflowID, loaded.Classification, detail)
	}
	profile, err := workflow.ProfileOf(loaded.State, workflow.ProfileCatalogFunc(workflowprofile.Resolve))
	if err != nil {
		return memoryscope.Plan{}, err
	}
	return workflow.PlanOf(loaded.State, project, profile)
}

// writeMemoryPlan prints plan as indented JSON and exits 0, or exits 2 on a
// marshal failure and 1 on a failed write.
func writeMemoryPlan(stdout, stderr io.Writer, plan memoryscope.Plan, exit func(int)) {
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
	if o.fromWorkflow() {
		plan, err := workflowMemoryPlan(o.project, o.workflowID)
		if err != nil {
			fmt.Fprintf(stderr, "error: memory plan: %v\n", err)
			exit(2)
			return
		}
		writeMemoryPlan(stdout, stderr, plan, exit)
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
	var narrowers []memoryscope.Directive
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
		// A Goal v3's own directive narrows first, as in a workflow created from it (C23 M3).
		if g.MemoryDirective != nil {
			narrowers = append(narrowers, *g.MemoryDirective)
		}
	}

	fileNarrowers, err := loadMemoryDirectiveNarrowers(o)
	narrowers = append(narrowers, fileNarrowers...)
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
	writeMemoryPlan(stdout, stderr, plan, exit)
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
