package main

// workflow subcommand: 'workflow create|start|pause|resume|stage|verify|
// close|status|bind|unbind|binding'. This is the Phase 6 standalone workflow
// lifecycle: it creates, starts, pauses, resumes, records stages on,
// structurally verifies, and closes one workflow instance, entirely as local
// bookkeeping under
// $XDG_STATE_HOME/labdrian/workflows/<project_id>/<workflow_id>.jsonl. No verb
// here executes a workflow step or check (that is Phase 7), and no verb
// requires Gentle AI, gentle-pi, a runtime, memory, or auth to be present: an
// unavailable declared dependency is recorded on the event as an observation,
// never approved or hidden (see engine/workflow's DependencyProber).
//
// bind, unbind, and binding (workflow_bind.go) are the Phase 7 addition: they
// record which workflow a git repository follows, in a separate store, without
// touching any workflow's log.
//
// Provenance (worktree root, git HEAD) is observed without running git or
// any other subprocess; see engine/gitfs.
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
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"
)

// runWorkflow implements the 'workflow <verb>' subcommand.
func runWorkflow(p process, d deps, args []string) {
	cwd, _ := d.workingDir() // best-effort; "" makes the locator report empty provenance.
	runWorkflowCore(d, args, cwd, p.stdout, p.stderr, p.exit)
}

// runWorkflowCore is the testable core of the workflow subcommand. Every
// exit(n) is followed by a return, because tests inject a non-terminating
// exit. cwd is injected so tests never depend on the process's real working
// directory.
func runWorkflowCore(d deps, args []string, cwd string, stdout, stderr io.Writer, exit func(int)) {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "error: workflow requires a verb: create, start, pause, resume, stage, verify, close, status, bind, unbind, binding")
		exit(1)
		return
	}
	verb, rest := args[0], args[1:]
	switch verb {
	case "create":
		runWorkflowCreate(d, rest, cwd, stdout, stderr, exit)
	case "start":
		runWorkflowTransition(d, rest, cwd, stdout, stderr, exit, "start", workflow.Lifecycle.Start)
	case "pause":
		runWorkflowTransition(d, rest, cwd, stdout, stderr, exit, "pause", workflow.Lifecycle.Pause)
	case "resume":
		runWorkflowTransition(d, rest, cwd, stdout, stderr, exit, "resume", workflow.Lifecycle.Resume)
	case "stage":
		runWorkflowStage(d, rest, cwd, stdout, stderr, exit)
	case "verify":
		runWorkflowVerify(d, rest, cwd, stdout, stderr, exit)
	case "close":
		runWorkflowClose(d, rest, cwd, stdout, stderr, exit)
	case "status":
		runWorkflowStatus(d, rest, cwd, stdout, stderr, exit)
	case "bind":
		runWorkflowBind(d, rest, cwd, stdout, stderr, exit)
	case "unbind":
		runWorkflowUnbind(d, rest, cwd, stdout, stderr, exit)
	case "binding":
		runWorkflowBinding(d, rest, cwd, stdout, stderr, exit)
	default:
		fmt.Fprintf(stderr, "error: workflow: unknown verb %q (expected create, start, pause, resume, stage, verify, close, status, bind, unbind, or binding)\n", verb)
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
// (its EventLog), the built-in profile catalog, and the real role chain store,
// with provenance observed from cwd (no
// subprocess) and the prober of the deps (deps.workflowProber; a dependency is
// recorded available only when its presence is seen by stat, never on the
// CLI's own authority). goalFile is used only by
// verbs that call LoadGoal (verify); it must be empty for every other verb,
// since Create reads and validates its own Goal argument directly (see
// runWorkflowCreate) rather than through the injected GoalReader, and no
// other verb needs a Goal at all. When a dependency probe degrades (an
// error, a timeout, or an unexpected observation count -- see
// workflow.DegradedHook), one line is printed to stderr so the degradation
// is visible to whoever ran the command, even though the operation still
// succeeds.
func newWorkflowLifecycle(d deps, cwd, goalFile string, stderr io.Writer) (workflow.Lifecycle, error) {
	store, err := newWorkflowStore()
	if err != nil {
		return workflow.Lifecycle{}, err
	}
	chains, err := newRoleChainStore()
	if err != nil {
		return workflow.Lifecycle{}, err
	}
	profiles := builtInProfileCatalog()
	lc, err := workflow.NewLifecycle(store, profiles, time.Now, newRepoLocator().Provenance(cwd), pathGoalReader{path: goalFile}, chains, d.dependencyProber())
	if err != nil {
		return workflow.Lifecycle{}, err
	}
	return lc.WithDegradedHook(func(detail string) {
		fmt.Fprintf(stderr, "warning: workflow: dependency probing degraded: %s\n", detail)
	}), nil
}

// runWorkflowCreate implements 'workflow create --project --workflow --goal
// --profile [--role-chain]'.
func runWorkflowCreate(d deps, args []string, cwd string, stdout, stderr io.Writer, exit func(int)) {
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
	// "" here, not o.goalFile: Create takes the already-read-and-parsed g
	// directly and never calls the injected GoalReader (only Verify does),
	// so wiring o.goalFile in would be dead wiring reading nothing.
	lc, err := newWorkflowLifecycle(d, cwd, "", stderr)
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
func runWorkflowTransition(d deps, args []string, cwd string, stdout, stderr io.Writer, exit func(int), label string, op func(workflow.Lifecycle, string, string) (workflow.State, error)) {
	o, err := parseWorkflowArgs(args, []string{"--project", "--workflow"})
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow %s: %v\n", label, err)
		exit(1)
		return
	}
	lc, err := newWorkflowLifecycle(d, cwd, "", stderr)
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
func runWorkflowStage(d deps, args []string, cwd string, stdout, stderr io.Writer, exit func(int)) {
	o, err := parseWorkflowArgs(args, []string{"--project", "--workflow", "--stage"})
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow stage: %v\n", err)
		exit(1)
		return
	}
	lc, err := newWorkflowLifecycle(d, cwd, "", stderr)
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
func runWorkflowVerify(d deps, args []string, cwd string, stdout, stderr io.Writer, exit func(int)) {
	o, err := parseWorkflowArgs(args, []string{"--project", "--workflow", "--goal"})
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow verify: %v\n", err)
		exit(1)
		return
	}
	lc, err := newWorkflowLifecycle(d, cwd, o.goalFile, stderr)
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
	// A verification that succeeded says, once, when the profile the workflow recorded is no longer
	// the built-in one of its name. It is a warning and nothing more: the workflow was verified
	// against its recorded profile, and the exit code is as it was.
	if drift, err := workflow.DetectProfileDrift(state, builtInProfileCatalog()); err != nil {
		fmt.Fprintf(stderr, "warning: workflow verify: could not tell whether the profile drifted: %v\n", err)
	} else if drift != nil {
		fmt.Fprint(stderr, profileDriftWarning(*drift))
	}
	writeWorkflowState(stdout, stderr, workflow.ClassificationOwned, state, exit)
}

// runWorkflowClose implements 'workflow close --project --workflow
// --outcome completed|abandoned [--reason]'.
func runWorkflowClose(d deps, args []string, cwd string, stdout, stderr io.Writer, exit func(int)) {
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
	lc, err := newWorkflowLifecycle(d, cwd, "", stderr)
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
func runWorkflowStatus(d deps, args []string, cwd string, stdout, stderr io.Writer, exit func(int)) {
	o, err := parseWorkflowArgs(args, []string{"--project", "--workflow"})
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow status: %v\n", err)
		exit(1)
		return
	}
	lc, err := newWorkflowLifecycle(d, cwd, "", stderr)
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
	drift, err := workflow.DetectProfileDrift(state, builtInProfileCatalog())
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow status: %v\n", err)
		exit(2)
		return
	}
	writeWorkflowStateWithDrift(stdout, stderr, classification, state, drift, exit)
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
	// ProfileDrift is present only in the answer of 'workflow status', and only when the profile a
	// workflow recorded is no longer the built-in profile of its name; without drift the field is
	// absent, so every status that did not report drift keeps its bytes.
	ProfileDrift *profileDriftJSON `json:"profile_drift,omitempty"`
}

// profileDriftJSON is the CLI's view of workflow.ProfileDrift: kind is "changed" (the built-in
// profile of that name differs from the recorded one in fields) or "retired" (there is none any
// more, and the digest of the built-in one and the fields are absent).
type profileDriftJSON struct {
	Kind           string   `json:"kind"`
	Profile        string   `json:"profile"`
	RecordedDigest string   `json:"recorded_digest"`
	CurrentDigest  string   `json:"current_digest,omitempty"`
	Fields         []string `json:"fields,omitempty"`
}

func newProfileDriftJSON(d *workflow.ProfileDrift) *profileDriftJSON {
	if d == nil {
		return nil
	}
	return &profileDriftJSON{Kind: string(d.Kind), Profile: d.Profile, RecordedDigest: d.RecordedDigest, CurrentDigest: d.CurrentDigest, Fields: d.Fields}
}

// builtInProfileCatalog is the catalog the program is wired with: the built-in profiles.
func builtInProfileCatalog() workflow.ProfileCatalog {
	return workflow.ProfileCatalogFunc(workflowprofile.Resolve)
}

// profileDriftWarning is the one line 'workflow verify' prints on stderr when the profile the
// workflow recorded is no longer the built-in one of its name.
func profileDriftWarning(d workflow.ProfileDrift) string {
	if d.Kind == workflow.ProfileRetired {
		return fmt.Sprintf("warning: workflow verify: profile drift: the profile %q this workflow recorded is no longer a built-in profile (retired); the workflow goes on from the recorded profile\n", d.Profile)
	}
	return fmt.Sprintf("warning: workflow verify: profile drift: the built-in profile %q differs from the one this workflow recorded (changed: %s); the workflow goes on from the recorded profile\n", d.Profile, strings.Join(d.Fields, ", "))
}

func newWorkflowStateJSON(classification workflow.Classification, s workflow.State, drift *workflow.ProfileDrift) workflowStateJSON {
	stages := s.Stages
	if stages == nil {
		stages = []string{}
	}
	return workflowStateJSON{
		ProfileDrift:    newProfileDriftJSON(drift),
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
	writeWorkflowStateWithDrift(stdout, stderr, classification, state, nil, exit)
}

// writeWorkflowStateWithDrift is writeWorkflowState with the drift finding of the workflow, which
// only 'workflow status' passes.
func writeWorkflowStateWithDrift(stdout, stderr io.Writer, classification workflow.Classification, state workflow.State, drift *workflow.ProfileDrift, exit func(int)) {
	data, err := json.MarshalIndent(newWorkflowStateJSON(classification, state, drift), "", "  ")
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
