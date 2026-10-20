package app

import (
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
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

// ProceduralSkillsReader is how the hook reads the procedural skills lock of the repository a
// directory belongs to (C23 M4 decision 5). The use case owns the port; an adapter at the edge reads
// the lock file and the composition root wires the two.
//
// Load never fails: no lock is the zero value (no procedural skill), and a lock that cannot be read
// or is not valid is reported in ProceduralSkills.Unreadable, which the gate turns into one warning.
type ProceduralSkillsReader interface {
	Load(dir string) projection.ProceduralSkills
}

// SourceAvailabilityProber is how the SessionStart hook finds out, when a session begins, which of
// the memory sources of its plan can be read (C23 M8, docs/architecture/adr-c23-m8-session-availability.md).
// The use case owns the port; an adapter at the edge asks each source, with a deadline, and the
// composition root wires the two. dir is the directory of the session, which decides the project a
// source belongs to.
//
// Probe never fails and never waits without a bound: a source that does not answer in time, or at
// all, is reported unavailable with the reason, and the sources it was not asked about are not
// reported.
type SourceAvailabilityProber interface {
	Probe(dir string, sources []memoryscope.Source) []projection.SourceAvailability
}

// Clock tells the time a binding is made at. The use case owns the port so that none of the inner
// ring takes the time from the machine.
type Clock interface {
	Now() time.Time
}
