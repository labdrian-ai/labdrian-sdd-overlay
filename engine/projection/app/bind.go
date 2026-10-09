package app

import (
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// BindWorkflow is the use case of the binding verbs: it records which workflow the repository that
// holds a directory follows, removes that record, and says what it holds. A binding is a pointer:
// these operations never append to a workflow's log, and Describe never writes anything.
type BindWorkflow struct {
	// Repositories finds the repository a directory belongs to.
	Repositories projection.RepoLocator
	// Bindings keeps the binding of each repository.
	Bindings projection.BindingStore
	// Workflows reads the workflows a binding names.
	Workflows WorkflowReader
	// Clock tells the time a binding is made at.
	Clock Clock
}

// BindRequest is what Bind is asked to do: bind the repository that holds Dir to the workflow
// WorkflowID of project ProjectID.
type BindRequest struct {
	Dir, ProjectID, WorkflowID string
}

// Bind binds the repository that holds req.Dir to the workflow, and returns the binding that is
// stored: for a repository already bound to that workflow, the original binding with its original
// bound_at.
//
// The workflow must exist, be owned, and not be closed. If the repository is bound to a different
// workflow, that binding is replaced only when it is stale: the bound workflow is closed, gone, or
// a log that can never be followed again because it is corrupt or not ours (drifted, malformed,
// foreign). A binding to a workflow that is still active (created, running or paused), or whose
// log cannot be read right now (unavailable, so it may be active), is never replaced silently.
// A stale binding is replaced only if it is still the exact binding that was judged stale
// (BindingStore.BindIfUnchanged): if another process changed it in between, nothing is replaced
// and the error holds projection.ErrBindingChanged.
//
// A refusal is one of the typed errors of this package, or a *BindingStoreError.
func (s BindWorkflow) Bind(req BindRequest) (projection.Binding, error) {
	repoKey, ok := s.Repositories.RepoKey(req.Dir)
	if !ok {
		return projection.Binding{}, &NoRepositoryError{}
	}
	if err := workflow.ValidateIdentifier("project_id", req.ProjectID); err != nil {
		return projection.Binding{}, &InvalidIdentifierError{Err: err}
	}
	if err := workflow.ValidateIdentifier("workflow_id", req.WorkflowID); err != nil {
		return projection.Binding{}, &InvalidIdentifierError{Err: err}
	}
	if err := s.judgeTarget(req); err != nil {
		return projection.Binding{}, err
	}

	// stale is the binding judged stale below, or nil when there is nothing to replace. The
	// judgment and the replacement are two steps, and the window between them is closed by
	// replacing only that exact binding.
	current, err := s.Bindings.Load(repoKey)
	if err != nil {
		return projection.Binding{}, &BindingStoreError{Err: err}
	}
	stale, err := s.judgeCurrent(current, req)
	if err != nil {
		return projection.Binding{}, err
	}

	now := s.Clock.Now()
	if stale != nil {
		err = s.Bindings.BindIfUnchanged(repoKey, req.ProjectID, req.WorkflowID, now, *stale)
	} else {
		err = s.Bindings.Bind(repoKey, req.ProjectID, req.WorkflowID, now, false)
	}
	if err != nil {
		return projection.Binding{}, &BindingStoreError{Err: err}
	}

	// What is stored is what is reported, which for an idempotent bind is the original binding with
	// its original bound_at. Another process may have changed it meanwhile.
	stored, err := s.Bindings.Load(repoKey)
	if err != nil {
		return projection.Binding{}, &BindingStoreError{Err: err}
	}
	if stored.Classification != projection.ClassificationOwned {
		return projection.Binding{}, &ReadBackError{Classification: stored.Classification, Detail: stored.Detail}
	}
	if b := stored.Binding; b.ProjectID != req.ProjectID || b.WorkflowID != req.WorkflowID {
		return projection.Binding{}, &ChangedConcurrentlyError{Found: b, ProjectID: req.ProjectID, WorkflowID: req.WorkflowID}
	}
	return stored.Binding, nil
}

// judgeTarget refuses a workflow that cannot be bound: one that does not exist, is not owned or
// is closed.
func (s BindWorkflow) judgeTarget(req BindRequest) error {
	target := s.Workflows.Load(req.ProjectID, req.WorkflowID)
	switch {
	case target.Classification == workflow.ClassificationAbsent:
		return &WorkflowAbsentError{ProjectID: req.ProjectID, WorkflowID: req.WorkflowID}
	case target.Classification != workflow.ClassificationOwned:
		return &WorkflowNotOwnedError{ProjectID: req.ProjectID, WorkflowID: req.WorkflowID, Classification: target.Classification, Detail: target.Detail}
	case target.State.Status == workflow.StatusClosed:
		return &WorkflowClosedError{ProjectID: req.ProjectID, WorkflowID: req.WorkflowID, Outcome: target.State.CloseOutcome}
	}
	return nil
}

// judgeCurrent judges the binding the repository already has. It returns the binding to replace
// when the repository is bound to a different workflow whose binding is stale, nil when there is
// nothing to replace, and a refusal when the binding is to a workflow that may be active.
func (s BindWorkflow) judgeCurrent(current projection.Loaded, req BindRequest) (*projection.Binding, error) {
	if current.Classification != projection.ClassificationOwned {
		return nil, nil
	}
	bound := current.Binding
	if bound.ProjectID == req.ProjectID && bound.WorkflowID == req.WorkflowID {
		return nil, nil
	}
	previous := s.Workflows.Load(bound.ProjectID, bound.WorkflowID)
	switch {
	case previous.Classification == workflow.ClassificationOwned && previous.State.Status != workflow.StatusClosed:
		return nil, &BoundToLiveWorkflowError{Bound: bound, Status: previous.State.Status}
	case previous.Classification == workflow.ClassificationUnavailable:
		return nil, &BoundToUnreadableWorkflowError{Bound: bound, Detail: previous.Detail}
	}
	return &bound, nil
}

// Unbind removes the binding of the repository that holds dir, and reports whether it removed
// one. It is idempotent: unbinding a repository that is not bound removes nothing and succeeds. A
// binding that is foreign or malformed is refused and left untouched.
func (s BindWorkflow) Unbind(dir string) (removed bool, err error) {
	repoKey, ok := s.Repositories.RepoKey(dir)
	if !ok {
		return false, &NoRepositoryError{}
	}
	removed, err = s.Bindings.Unbind(repoKey)
	if err != nil {
		return false, &BindingStoreError{Err: err}
	}
	return removed, nil
}

// BindingView is what Describe says about the binding of a repository: what the binding store
// holds and, only when the binding is owned, the workflow it names as the workflow store reports it.
type BindingView struct {
	Binding  projection.Loaded
	Workflow *workflow.Loaded
}

// Describe says what the repository that holds dir is bound to. It is strictly read-only. A state
// that is not ours (foreign, malformed, unavailable) is described by its classification and
// detail, not refused, and so is a bound workflow the log of which cannot be read.
func (s BindWorkflow) Describe(dir string) (BindingView, error) {
	repoKey, ok := s.Repositories.RepoKey(dir)
	if !ok {
		return BindingView{}, &NoRepositoryError{}
	}
	loaded, err := s.Bindings.Load(repoKey)
	if err != nil {
		return BindingView{}, &BindingStoreError{Err: err}
	}
	view := BindingView{Binding: loaded}
	if loaded.Classification == projection.ClassificationOwned {
		w := s.Workflows.Load(loaded.Binding.ProjectID, loaded.Binding.WorkflowID)
		view.Workflow = &w
	}
	return view, nil
}
