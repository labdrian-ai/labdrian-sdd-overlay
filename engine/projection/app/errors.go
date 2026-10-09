package app

import (
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// The errors below say what a use case refused. Each carries what a command needs to tell a person
// (the workflow, the binding, the status, the error underneath) and none of the words of a
// terminal, nor the commands a person should run next: the command that prints them chooses its
// own. Error() is a plain sentence for a log or a test failure.

// NoRepositoryError: there is no repository to key a binding on. The binding is keyed by the
// repository, so without one there is nothing to key on, and guessing from the working directory
// would bind the wrong thing.
type NoRepositoryError struct{}

func (*NoRepositoryError) Error() string {
	return "no repository was found at or above the working directory"
}

// InvalidIdentifierError: the project or workflow identifier is not valid. Err is the domain's
// error, which names the identifier and why.
type InvalidIdentifierError struct{ Err error }

func (e *InvalidIdentifierError) Error() string { return e.Err.Error() }
func (e *InvalidIdentifierError) Unwrap() error { return e.Err }

// WorkflowAbsentError: the workflow to bind does not exist.
type WorkflowAbsentError struct{ ProjectID, WorkflowID string }

func (e *WorkflowAbsentError) Error() string {
	return fmt.Sprintf("workflow %q of project %q does not exist", e.WorkflowID, e.ProjectID)
}

// WorkflowNotOwnedError: the workflow to bind exists but its log is not one this engine owns
// (drifted, malformed, foreign or unavailable). Detail says why.
type WorkflowNotOwnedError struct {
	ProjectID, WorkflowID string
	Classification        workflow.Classification
	Detail                string
}

func (e *WorkflowNotOwnedError) Error() string {
	return fmt.Sprintf("workflow %q of project %q is not owned (%s): %s", e.WorkflowID, e.ProjectID, e.Classification, e.Detail)
}

// WorkflowClosedError: the workflow to bind is closed, and a closed workflow cannot be followed.
type WorkflowClosedError struct {
	ProjectID, WorkflowID string
	Outcome               workflow.Outcome
}

func (e *WorkflowClosedError) Error() string {
	return fmt.Sprintf("workflow %q of project %q is closed (%s)", e.WorkflowID, e.ProjectID, e.Outcome)
}

// BoundToLiveWorkflowError: the repository is bound to another workflow that is still active
// (created, running or paused), which is never replaced silently.
type BoundToLiveWorkflowError struct {
	Bound  projection.Binding
	Status workflow.Status
}

func (e *BoundToLiveWorkflowError) Error() string {
	return fmt.Sprintf("the repository is bound to workflow %q of project %q (status: %s)", e.Bound.WorkflowID, e.Bound.ProjectID, e.Status)
}

// BoundToUnreadableWorkflowError: the repository is bound to another workflow whose log cannot be
// read right now, so it may still be active and is not replaced. Detail says why it cannot be read.
type BoundToUnreadableWorkflowError struct {
	Bound  projection.Binding
	Detail string
}

func (e *BoundToUnreadableWorkflowError) Error() string {
	return fmt.Sprintf("the repository is bound to workflow %q of project %q, whose log cannot be read (%s)", e.Bound.WorkflowID, e.Bound.ProjectID, e.Detail)
}

// BindingStoreError: the binding store refused or failed. Err is the store's error, which the
// domain's sentinels (projection.ErrBindingChanged, ErrAlreadyBound, ErrBindingBusy and the
// refusals of a file that is not ours) are found in with errors.Is.
type BindingStoreError struct{ Err error }

func (e *BindingStoreError) Error() string { return e.Err.Error() }
func (e *BindingStoreError) Unwrap() error { return e.Err }

// ReadBackError: the binding was written and then could not be read back as ours.
type ReadBackError struct {
	Classification projection.Classification
	Detail         string
}

func (e *ReadBackError) Error() string {
	return fmt.Sprintf("the binding could not be read back (%s): %s", e.Classification, e.Detail)
}

// ChangedConcurrentlyError: the binding was written and read back, and it names another workflow
// than the one asked for, so another process changed it meanwhile. Reporting it would name a
// workflow nobody asked for.
type ChangedConcurrentlyError struct {
	Found                 projection.Binding
	ProjectID, WorkflowID string
}

func (e *ChangedConcurrentlyError) Error() string {
	return fmt.Sprintf("the binding was changed concurrently: it now names workflow %q of project %q, not the requested workflow %q of project %q",
		e.Found.WorkflowID, e.Found.ProjectID, e.WorkflowID, e.ProjectID)
}
