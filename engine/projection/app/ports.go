package app

import (
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// WorkflowReader is how the use cases read a workflow, the one a binding names or the one a person
// asks to bind. The use cases own the port; an adapter at the edge answers it over the workflow
// log, and the composition root wires the two. A test of a use case hands it a fake.
//
// Load never fails: a workflow store that cannot be opened or read is reported as a workflow of
// classification unavailable, with the reason as its Detail. A binding must be able to describe a
// workflow it cannot read, and bind must be able to say why it cannot bind one, so the reason is
// data and not an error. The identifiers must already be valid.
type WorkflowReader interface {
	Load(projectID, workflowID string) workflow.Loaded
}

// Clock tells the time a binding is made at. The use case owns the port so that none of the inner
// ring takes the time from the machine.
type Clock interface {
	Now() time.Time
}
