// Package app is the use case of the builder guard (own-rdd S14): given the prompt of a builder launch and
// the working directory of the session, it joins the citation of the prompt, the feature document it names
// and the decision of engine/oddtask.
package app

import (
	"errors"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/oddtask"
)

// Documents is the port to the feature documents of a repository.
type Documents interface {
	// Read returns the feature document at path, resolved against cwd when it is relative. It fails with an
	// *oddtask.ReadError when the document cannot be read.
	Read(path, cwd string) (oddtask.File, error)
}

// Workflows is the port to the workflow of a repository (own-rdd S19 a): what the repository that holds cwd is
// bound to, the state of that workflow and the role chain it follows, as the stores of the engine report them.
type Workflows interface {
	// Of fails when the binding, the workflow or the chain could not be read at all; a binding, a workflow or a
	// chain that was read but is not usable is described in the view instead.
	Of(cwd string) (oddtask.WorkflowView, error)
}

// Guard is the builder guard over a Documents port, and over a Workflows port when it has been given one.
type Guard struct {
	docs  Documents
	flows Workflows
}

// New is the Guard that reads feature documents from docs.
func New(docs Documents) Guard { return Guard{docs: docs} }

// WithWorkflows is the Guard that also holds a launch to the workflow of its feature, read from flows.
func (g Guard) WithWorkflows(flows Workflows) Guard {
	g.flows = flows
	return g
}

// CheckLaunch is the whole decision about a builder launch with prompt in a session at cwd: the checks of
// Check, then the workflow check of S19. It returns the message of the denial, or "" when the launch may go on.
// The workflow is read only after the prompt cites a task the shaper designed. A Guard with no Workflows port
// cannot check the workflow, which is a failure to read it.
func (g Guard) CheckLaunch(prompt, cwd string) string {
	if reason := g.Check(prompt, cwd); reason != "" {
		return oddtask.Message(reason)
	}
	citation, reason := oddtask.ParseCitation(prompt)
	if reason != "" {
		return oddtask.Message(reason)
	}
	feature, task := citation.Feature(), citation.Tasks[0]
	deny := func(r oddtask.Reason) string { return oddtask.WorkflowMessage(r, feature, task) }
	if g.flows == nil {
		return deny(oddtask.WorkflowUnreadableReason("the guard has no reader of the workflow"))
	}
	view, err := g.flows.Of(cwd)
	if err != nil {
		return deny(oddtask.WorkflowUnreadableReason(err.Error()))
	}
	if reason := oddtask.DecideWorkflow(cwd, feature, view); reason != "" {
		if oddtask.WaitsForHop(feature, view) {
			return oddtask.HopMessage(reason, view.Workflow, view.ChainID)
		}
		return deny(reason)
	}
	return ""
}

// Check says whether a builder may be launched with prompt in a session at cwd. The Reason is empty when it
// may; otherwise it is why not. A prompt that cites no task is refused before any document is read.
func (g Guard) Check(prompt, cwd string) oddtask.Reason {
	citation, reason := oddtask.ParseCitation(prompt)
	if reason != "" {
		return reason
	}
	file, err := g.docs.Read(citation.Path, cwd)
	if err != nil {
		var unreadable *oddtask.ReadError
		if errors.As(err, &unreadable) {
			return oddtask.UnreadableReason(unreadable.Path, unreadable.Cause)
		}
		return oddtask.UnreadableReason(citation.Path, fmt.Sprint(err))
	}
	return oddtask.Decide(file.Path, oddtask.ParseDocument(file.Text), citation.Tasks)
}
