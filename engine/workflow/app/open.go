// Package app is the application layer of the workflow: the use cases that compose the stores and the
// lifecycle of the workflow domain without writing their rules again.
//
// Open (own-rdd S19 b) opens the workflow of one task of a feature: it appends the two role handoff
// records that hand the shaper's design to the builder, creates the workflow of the task over that chain,
// starts it, records the five stages that come before the work, and binds the repository to it. Each step
// is the operation its own store or lifecycle already has (roles.ChainLog.Append, workflow.Lifecycle's
// Create, Start and RecordStage, projection/app's Bind); the stores have no transaction across them, so a
// step that fails after an earlier one wrote leaves what was written and says so.
package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/goal"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/oddtask"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	projapp "github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// Profile is the workflow profile every task's workflow follows.
const Profile = "odd"

// PreWorkStages are the stages recorded, in order, before the work of a task starts: the profile's
// stages that the shaping already did.
var PreWorkStages = []string{"authorize", "explore", "resolve-uncertainty", "classify", "track-if-substantial"}

// The steps of Open, as an error names them.
const (
	StepShaperRecord    = "append the shaper record"
	StepEstimatorRecord = "append the estimator record"
	StepCreate          = "create"
	StepStart           = "start"
	StepStages          = "record the five stages"
	StepBind            = "bind"
)

// Sources reads the two files Open starts from, in the repository of the session.
type Sources interface {
	// Goal returns the goal file at override, or the feature's default one when override is empty.
	Goal(feature, override string) ([]byte, error)
	// Document returns the bytes of the feature document odd/tasks/<feature>.md as they are on disk.
	Document(feature string) ([]byte, error)
}

// Workflows is the part of workflow.Lifecycle that Open uses.
type Workflows interface {
	Create(projectID, workflowID, projectIdentity string, g goal.Goal, profileName, roleChainID string) (workflow.State, error)
	Start(projectID, workflowID string) (workflow.State, error)
	RecordStage(projectID, workflowID, stage string) (workflow.State, error)
	Status(projectID, workflowID string) (workflow.Classification, workflow.State, error)
}

// Binder is the part of projection/app's BindWorkflow that Open uses.
type Binder interface {
	Bind(req projapp.BindRequest) (projection.Binding, error)
}

// Open is the use case 'workflow open'.
type Open struct {
	Sources   Sources
	Chains    roles.ChainLog
	Workflows Workflows
	Binder    Binder
	// Identity is the shared identity of the project that Create records for the workflow.
	Identity func(project string) string
}

// OpenRequest asks Open to open the workflow of Task of Feature, binding the repository that holds Dir.
type OpenRequest struct {
	Feature, Task, GoalPath, Dir string
}

// Opened says what Open opened.
type Opened struct {
	Workflow string `json:"workflow"`
	Chain    string `json:"chain"`
	Goal     string `json:"goal"`
	Project  string `json:"project"`
	Status   string `json:"status"`
	Bound    bool   `json:"bound"`
}

// RefusedError is a refusal made before anything was written.
type RefusedError struct{ Reason string }

func (e *RefusedError) Error() string { return e.Reason }

// StepError is the failure of one step after the steps in Done wrote.
type StepError struct {
	Done []string
	Step string
	Err  error
}

func (e *StepError) Error() string {
	done := "none"
	if len(e.Done) > 0 {
		done = strings.Join(e.Done, ", ")
	}
	return fmt.Sprintf("%s failed (done: %s): %v", e.Step, done, e.Err)
}

func (e *StepError) Unwrap() error { return e.Err }

func refuse(format string, args ...any) error {
	return &RefusedError{Reason: fmt.Sprintf(format, args...)}
}

// Open opens the workflow of the task. It refuses, before writing anything, a goal that is missing,
// invalid or of another goal id, a task that is not marked as the shaper's design, a chain that already
// has records and a workflow that already exists.
func (o Open) Open(req OpenRequest) (Opened, error) {
	if !oddtask.ValidFeature(req.Feature) {
		return Opened{}, refuse("feature %q is not a feature name", req.Feature)
	}
	if !oddtask.ValidTask(req.Task) {
		return Opened{}, refuse("task %q is not a task id", req.Task)
	}
	g, err := o.readGoal(req)
	if err != nil {
		return Opened{}, err
	}
	docBytes, err := o.Sources.Document(req.Feature)
	if err != nil {
		return Opened{}, refuse("read feature document: %v", err)
	}
	docPath := "odd/tasks/" + req.Feature + ".md"
	shaped, reason := oddtask.ShapedBy(docPath, oddtask.ParseDocument(string(docBytes)), req.Task)
	if reason != "" {
		return Opened{}, refuse("%s", reason)
	}

	name := req.Feature + "-" + strings.ToLower(req.Task)
	project := g.ProjectID
	if err := o.checkFree(project, req.Feature, name); err != nil {
		return Opened{}, err
	}
	first, second, err := handoffs(project, req.Feature, name, req.Task, docPath, shaped, sha256hex.Sum(docBytes))
	if err != nil {
		return Opened{}, err
	}

	var done []string
	step := func(name string, err error) error {
		if err != nil {
			return &StepError{Done: done, Step: name, Err: err}
		}
		done = append(done, name)
		return nil
	}
	if err := step(StepShaperRecord, appendRecord(o.Chains, first)); err != nil {
		return Opened{}, err
	}
	if err := step(StepEstimatorRecord, appendRecord(o.Chains, second)); err != nil {
		return Opened{}, err
	}
	state, err := o.Workflows.Create(project, name, o.Identity(project), g, Profile, name)
	if err := step(StepCreate, err); err != nil {
		return Opened{}, err
	}
	state, err = o.Workflows.Start(project, name)
	if err := step(StepStart, err); err != nil {
		return Opened{}, err
	}
	for _, stage := range PreWorkStages {
		if state, err = o.Workflows.RecordStage(project, name, stage); err != nil {
			err = fmt.Errorf("stage %s: %w", stage, err)
			break
		}
	}
	if err := step(StepStages, err); err != nil {
		return Opened{}, err
	}
	_, err = o.Binder.Bind(projapp.BindRequest{Dir: req.Dir, ProjectID: project, WorkflowID: name})
	if err := step(StepBind, err); err != nil {
		return Opened{}, err
	}
	return Opened{Workflow: name, Chain: name, Goal: req.Feature, Project: project, Status: string(state.Status), Bound: true}, nil
}

// readGoal reads the goal file and holds it to the feature: its goal id is the feature's name.
func (o Open) readGoal(req OpenRequest) (goal.Goal, error) {
	data, err := o.Sources.Goal(req.Feature, req.GoalPath)
	if err != nil {
		return goal.Goal{}, refuse("read goal: %v", err)
	}
	g, err := goal.Parse(data)
	if err != nil {
		return goal.Goal{}, refuse("parse goal: %v", err)
	}
	if g.GoalID != req.Feature {
		return goal.Goal{}, refuse("the goal file declares goal_id %q, want %q (the feature)", g.GoalID, req.Feature)
	}
	return g, nil
}

// checkFree refuses a chain that already has records and a workflow that already exists.
func (o Open) checkFree(project, feature, name string) error {
	records, err := o.Chains.LoadChain(project, feature, name)
	if err != nil {
		return refuse("read role chain %s: %v", name, err)
	}
	if len(records) > 0 {
		return refuse("role chain %s already has %d records", name, len(records))
	}
	classification, _, err := o.Workflows.Status(project, name)
	if err != nil {
		return refuse("read workflow %s: %v", name, err)
	}
	if classification != workflow.ClassificationAbsent {
		return refuse("workflow %s of project %s already exists (%s)", name, project, classification)
	}
	return nil
}

// handoffs builds the two records that hand the shaper's design to the builder, each parsed back by
// roles.ParseRoleHandoff, so the chain store appends bytes it has already accepted once.
func handoffs(project, feature, chain, task, docPath string, shaped int, payload string) (first, second []byte, err error) {
	build := func(seq int, from, to roles.Role, prev, summary string) ([]byte, error) {
		raw, err := json.Marshal(roles.RoleHandoff{
			Version: roles.HandoffVersion, ProjectID: project, GoalID: feature, ChainID: chain, Seq: seq,
			FromRole: from, ToRole: to, PrevSHA256: prev, PayloadKind: "feature-doc", PayloadSHA256: payload,
			Evidence: []roles.Evidence{},
			Context:  roles.Context{Summary: summary, Decisions: []string{}, OpenQuestions: []string{}},
			Status:   roles.StatusCompleted,
		})
		if err != nil {
			return nil, refuse("build the handoff record %d: %v", seq, err)
		}
		if _, err := roles.ParseRoleHandoff(raw); err != nil {
			return nil, refuse("the handoff record %d is invalid: %v", seq, err)
		}
		return raw, nil
	}
	first, err = build(1, roles.RoleShaper, roles.RoleEstimator, roles.EmptyChainDigest,
		fmt.Sprintf("The shaper designed %s of %s (shaped: L%d) and hands the design to the estimator.", task, docPath, shaped))
	if err != nil {
		return nil, nil, err
	}
	second, err = build(2, roles.RoleEstimator, roles.RoleBuilder, sha256hex.Sum(first),
		fmt.Sprintf("The estimator hands %s of %s (shaped: L%d) to the builder.", task, docPath, shaped))
	return first, second, err
}

func appendRecord(chains roles.ChainLog, raw []byte) error {
	_, err := chains.Append(raw)
	return err
}

// IsRefusal reports whether err is a refusal made before anything was written.
func IsRefusal(err error) bool {
	var refused *RefusedError
	return errors.As(err, &refused)
}
