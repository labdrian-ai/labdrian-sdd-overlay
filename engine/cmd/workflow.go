package main

// workflow subcommand: 'workflow create|start|pause|resume|stage|verify|
// close|status'. This is the Phase 6 standalone workflow lifecycle: it
// creates, starts, pauses, resumes, records stages on, structurally
// verifies, and closes one workflow instance, entirely as local bookkeeping
// under $XDG_STATE_HOME/labdrian/workflows/<project_id>/<workflow_id>.jsonl.
// No verb here executes a workflow step or check (that is Phase 7), and no
// verb requires Gentle AI, gentle-pi, a runtime, memory, or auth to be
// present: an unavailable declared dependency is recorded on the event as
// an observation, never approved or hidden (see engine/workflow's
// DependencyProber).
//
// Provenance (worktree root, git HEAD) is observed without running git or
// any other subprocess; see observeProvenance in workflow_provenance.go.
// --goal is read fresh from disk at both create and verify: the CLI never
// persists a Goal file path, so verify always re-reads the file the caller
// points it at (Decision 3: verify's Goal digest check is only meaningful
// against the Goal as it exists now).

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/goal"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// runWorkflow implements the 'workflow <verb>' subcommand.
func runWorkflow(args []string) {
	cwd, _ := os.Getwd() // best-effort; "" makes observeProvenance report empty provenance.
	runWorkflowCore(args, cwd, os.Stdout, os.Stderr, os.Exit)
}

// runWorkflowCore is the testable core of the workflow subcommand. Every
// exit(n) is followed by a return, because tests inject a non-terminating
// exit. cwd is injected so tests never depend on the process's real working
// directory.
func runWorkflowCore(args []string, cwd string, stdout, stderr io.Writer, exit func(int)) {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "error: workflow requires a verb: create, start, pause, resume, stage, verify, close, status")
		exit(1)
		return
	}
	verb, rest := args[0], args[1:]
	switch verb {
	case "create":
		runWorkflowCreate(rest, cwd, stdout, stderr, exit)
	case "start":
		runWorkflowTransition(rest, cwd, stdout, stderr, exit, "start", workflow.Lifecycle.Start)
	case "pause":
		runWorkflowTransition(rest, cwd, stdout, stderr, exit, "pause", workflow.Lifecycle.Pause)
	case "resume":
		runWorkflowTransition(rest, cwd, stdout, stderr, exit, "resume", workflow.Lifecycle.Resume)
	case "stage":
		runWorkflowStage(rest, cwd, stdout, stderr, exit)
	case "verify":
		runWorkflowVerify(rest, cwd, stdout, stderr, exit)
	case "close":
		runWorkflowClose(rest, cwd, stdout, stderr, exit)
	case "status":
		runWorkflowStatus(rest, cwd, stdout, stderr, exit)
	default:
		fmt.Fprintf(stderr, "error: workflow: unknown verb %q (expected create, start, pause, resume, stage, verify, close, or status)\n", verb)
		exit(1)
	}
}

// workflowOpts are the parsed workflow flags. Not every verb uses every
// field.
type workflowOpts struct {
	project, workflowID, goalFile, profile, roleChain, stage, outcome, reason string
}

// parseWorkflowArgs parses --project/--workflow/--goal/--profile/
// --role-chain/--stage/--outcome/--reason. It fails loud on an unknown
// flag, a flag without its value, a positional argument, or a missing
// required flag.
func parseWorkflowArgs(args []string, required []string) (workflowOpts, error) {
	var o workflowOpts
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--project", "--workflow", "--goal", "--profile", "--role-chain", "--stage", "--outcome", "--reason":
			if i+1 >= len(args) {
				return o, fmt.Errorf("%s requires a value", a)
			}
			i++
			switch a {
			case "--project":
				o.project = args[i]
			case "--workflow":
				o.workflowID = args[i]
			case "--goal":
				o.goalFile = args[i]
			case "--profile":
				o.profile = args[i]
			case "--role-chain":
				o.roleChain = args[i]
			case "--stage":
				o.stage = args[i]
			case "--outcome":
				o.outcome = args[i]
			case "--reason":
				o.reason = args[i]
			}
		default:
			if strings.HasPrefix(a, "-") {
				return o, fmt.Errorf("unknown flag %q", a)
			}
			return o, fmt.Errorf("unexpected argument %q", a)
		}
	}
	values := map[string]string{
		"--project": o.project, "--workflow": o.workflowID, "--goal": o.goalFile,
		"--profile": o.profile, "--stage": o.stage, "--outcome": o.outcome,
	}
	for _, name := range required {
		if values[name] == "" {
			return o, fmt.Errorf("%s is required", name)
		}
	}
	return o, nil
}

// pathGoalReader is the workflow.GoalReader the CLI uses: it re-reads and
// re-parses one Goal v2 file from disk on every LoadGoal call (there is no
// caching and no persisted path), and requires the parsed Goal's
// project_id/goal_id to match what the caller asked for. path is empty for
// verbs that never call LoadGoal (everything except verify).
type pathGoalReader struct{ path string }

func (r pathGoalReader) LoadGoal(projectID, goalID string) (goal.Goal, error) {
	if r.path == "" {
		return goal.Goal{}, fmt.Errorf("no --goal file was supplied")
	}
	data, err := os.ReadFile(r.path)
	if err != nil {
		return goal.Goal{}, fmt.Errorf("read --goal: %w", err)
	}
	g, err := goal.Parse(data)
	if err != nil {
		return goal.Goal{}, fmt.Errorf("parse --goal: %w", err)
	}
	if g.ProjectID != projectID || g.GoalID != goalID {
		return goal.Goal{}, fmt.Errorf("--goal file declares project_id=%q goal_id=%q, want %q/%q", g.ProjectID, g.GoalID, projectID, goalID)
	}
	return g, nil
}

// newWorkflowLifecycle builds a Lifecycle over the real XDG-resolved Store
// and the real role chain store, with provenance observed from cwd (no
// subprocess) and the default UnavailableProber (no capability is ever
// approved on its own authority from the CLI). goalFile is used only by
// verbs that call LoadGoal (verify); it may be empty otherwise.
func newWorkflowLifecycle(cwd, goalFile string) (workflow.Lifecycle, error) {
	store, err := workflow.NewStore()
	if err != nil {
		return workflow.Lifecycle{}, err
	}
	chains, err := roles.NewChainStore()
	if err != nil {
		return workflow.Lifecycle{}, err
	}
	return workflow.NewLifecycle(store, time.Now, observeProvenance(cwd), pathGoalReader{path: goalFile}, chains, nil)
}

// runWorkflowCreate implements 'workflow create --project --workflow --goal
// --profile [--role-chain]'.
func runWorkflowCreate(args []string, cwd string, stdout, stderr io.Writer, exit func(int)) {
	o, err := parseWorkflowArgs(args, []string{"--project", "--workflow", "--goal", "--profile"})
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow create: %v\n", err)
		exit(1)
		return
	}
	data, err := os.ReadFile(o.goalFile)
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow create: read --goal: %v\n", err)
		exit(2)
		return
	}
	g, err := goal.Parse(data)
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow create: parse --goal: %v\n", err)
		exit(2)
		return
	}
	lc, err := newWorkflowLifecycle(cwd, o.goalFile)
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow create: %v\n", err)
		exit(2)
		return
	}
	state, err := lc.Create(o.project, o.workflowID, g, o.profile, o.roleChain)
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow create: %v\n", err)
		exit(2)
		return
	}
	writeWorkflowState(stdout, stderr, workflow.ClassificationOwned, state, exit)
}

// runWorkflowTransition implements the three verbs that need only
// --project/--workflow: start, pause, resume.
func runWorkflowTransition(args []string, cwd string, stdout, stderr io.Writer, exit func(int), label string, op func(workflow.Lifecycle, string, string) (workflow.State, error)) {
	o, err := parseWorkflowArgs(args, []string{"--project", "--workflow"})
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow %s: %v\n", label, err)
		exit(1)
		return
	}
	lc, err := newWorkflowLifecycle(cwd, "")
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow %s: %v\n", label, err)
		exit(2)
		return
	}
	state, err := op(lc, o.project, o.workflowID)
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow %s: %v\n", label, err)
		exit(2)
		return
	}
	writeWorkflowState(stdout, stderr, workflow.ClassificationOwned, state, exit)
}

// runWorkflowStage implements 'workflow stage --project --workflow --stage'.
func runWorkflowStage(args []string, cwd string, stdout, stderr io.Writer, exit func(int)) {
	o, err := parseWorkflowArgs(args, []string{"--project", "--workflow", "--stage"})
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow stage: %v\n", err)
		exit(1)
		return
	}
	lc, err := newWorkflowLifecycle(cwd, "")
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow stage: %v\n", err)
		exit(2)
		return
	}
	state, err := lc.RecordStage(o.project, o.workflowID, o.stage)
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow stage: %v\n", err)
		exit(2)
		return
	}
	writeWorkflowState(stdout, stderr, workflow.ClassificationOwned, state, exit)
}

// runWorkflowVerify implements 'workflow verify --project --workflow
// --goal'. --goal is required: verify always re-reads the Goal from the
// path given here, never from a path recorded at create.
func runWorkflowVerify(args []string, cwd string, stdout, stderr io.Writer, exit func(int)) {
	o, err := parseWorkflowArgs(args, []string{"--project", "--workflow", "--goal"})
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow verify: %v\n", err)
		exit(1)
		return
	}
	lc, err := newWorkflowLifecycle(cwd, o.goalFile)
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow verify: %v\n", err)
		exit(2)
		return
	}
	state, err := lc.Verify(o.project, o.workflowID)
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow verify: %v\n", err)
		exit(2)
		return
	}
	writeWorkflowState(stdout, stderr, workflow.ClassificationOwned, state, exit)
}

// runWorkflowClose implements 'workflow close --project --workflow
// --outcome completed|abandoned [--reason]'.
func runWorkflowClose(args []string, cwd string, stdout, stderr io.Writer, exit func(int)) {
	o, err := parseWorkflowArgs(args, []string{"--project", "--workflow", "--outcome"})
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow close: %v\n", err)
		exit(1)
		return
	}
	var outcome workflow.Outcome
	switch o.outcome {
	case string(workflow.OutcomeCompleted):
		outcome = workflow.OutcomeCompleted
	case string(workflow.OutcomeAbandoned):
		outcome = workflow.OutcomeAbandoned
	default:
		fmt.Fprintf(stderr, "error: workflow close: --outcome must be %q or %q, got %q\n", workflow.OutcomeCompleted, workflow.OutcomeAbandoned, o.outcome)
		exit(1)
		return
	}
	lc, err := newWorkflowLifecycle(cwd, "")
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow close: %v\n", err)
		exit(2)
		return
	}
	state, err := lc.Close(o.project, o.workflowID, outcome, o.reason)
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow close: %v\n", err)
		exit(2)
		return
	}
	writeWorkflowState(stdout, stderr, workflow.ClassificationOwned, state, exit)
}

// runWorkflowStatus implements 'workflow status --project --workflow'. It
// is entirely read-only: it never appends anything, even for a workflow
// whose on-disk state is foreign, malformed, drifted, or unavailable (those
// classifications are reported, not refused, since status never writes).
func runWorkflowStatus(args []string, cwd string, stdout, stderr io.Writer, exit func(int)) {
	o, err := parseWorkflowArgs(args, []string{"--project", "--workflow"})
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow status: %v\n", err)
		exit(1)
		return
	}
	lc, err := newWorkflowLifecycle(cwd, "")
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow status: %v\n", err)
		exit(2)
		return
	}
	classification, state, err := lc.Status(o.project, o.workflowID)
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow status: %v\n", err)
		exit(2)
		return
	}
	writeWorkflowState(stdout, stderr, classification, state, exit)
}

// workflowStateJSON is the CLI's stable JSON view of a workflow's
// classification and replayed State (see engine/workflow.State). It exists
// so the CLI's wire shape does not depend on engine/workflow.State's
// internal field names or its unexported bookkeeping fields.
type workflowStateJSON struct {
	Classification  string   `json:"classification"`
	Status          string   `json:"status"`
	Profile         string   `json:"profile,omitempty"`
	GoalID          string   `json:"goal_id,omitempty"`
	GoalDigest      string   `json:"goal_digest,omitempty"`
	RoleChainID     string   `json:"role_chain_id,omitempty"`
	RoleChainHead   string   `json:"role_chain_head,omitempty"`
	Stages          []string `json:"stages"`
	LastVerifiedSeq int      `json:"last_verified_seq"`
	CloseOutcome    string   `json:"close_outcome,omitempty"`
	CloseReason     string   `json:"close_reason,omitempty"`
}

func newWorkflowStateJSON(classification workflow.Classification, s workflow.State) workflowStateJSON {
	stages := s.Stages
	if stages == nil {
		stages = []string{}
	}
	return workflowStateJSON{
		Classification:  string(classification),
		Status:          string(s.Status),
		Profile:         s.Profile,
		GoalID:          s.GoalID,
		GoalDigest:      s.GoalDigest,
		RoleChainID:     s.RoleChainID,
		RoleChainHead:   s.RoleChainHead,
		Stages:          stages,
		LastVerifiedSeq: s.LastVerifiedSeq,
		CloseOutcome:    string(s.CloseOutcome),
		CloseReason:     s.CloseReason,
	}
}

// writeWorkflowState prints classification and state as indented JSON on
// stdout and exits 0, or reports a write/marshal failure on stderr and
// exits 1 (these never happen for a valid State, but are handled the same
// way every other command in this package handles an output failure).
func writeWorkflowState(stdout, stderr io.Writer, classification workflow.Classification, state workflow.State, exit func(int)) {
	data, err := json.MarshalIndent(newWorkflowStateJSON(classification, state), "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow: %v\n", err)
		exit(1)
		return
	}
	if _, err := stdout.Write(append(data, '\n')); err != nil {
		fmt.Fprintf(stderr, "error: workflow: writing state: %v\n", err)
		exit(1)
		return
	}
	exit(0)
}
