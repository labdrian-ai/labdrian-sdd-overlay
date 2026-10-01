package workflow

import "github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"

// EventLog is the persistence port of the workflow lifecycle: the append-only
// event log of each workflow, which Lifecycle reads to learn a workflow's state
// and appends to when it changes it. Lifecycle depends on this port and on nothing
// that reaches a file system; the concrete log (Store today, a file adapter once
// it moves out of this package) is chosen where the program is wired.
//
// An implementation owns the safety of the log. Append must refuse an event that
// does not extend the log it finds (a stale or skipped seq, a broken chain, an
// illegal transition) and must serialize concurrent appenders; Load must classify
// what it finds (see Classification) rather than fail on a log that is not ours.
// Store is the reference implementation and its tests define these rules.
type EventLog interface {
	// Load classifies the log of one workflow and, when it is ours
	// (ClassificationOwned), returns its events and the State they replay to. It
	// never writes.
	Load(projectID, workflowID string) (Loaded, error)
	// Append adds next to the log if it is legal, and appends nothing otherwise.
	Append(projectID, workflowID string, next WorkflowEvent) error
}

// ProfileCatalog is the port through which Lifecycle learns what a Workflow
// Profile declares: its stages, its review policy, its memory defaults. Every
// lifecycle operation resolves a profile here and nowhere else, so the catalog in
// use is one decision made where the program is wired.
type ProfileCatalog interface {
	// Resolve returns the profile named name, or an error when the catalog has
	// no such profile.
	Resolve(name string) (workflowprofile.WorkflowProfile, error)
}

// ProfileCatalogFunc makes a function a ProfileCatalog. The built-in catalog is
// ProfileCatalogFunc(workflowprofile.Resolve).
type ProfileCatalogFunc func(name string) (workflowprofile.WorkflowProfile, error)

// Resolve calls f.
func (f ProfileCatalogFunc) Resolve(name string) (workflowprofile.WorkflowProfile, error) {
	return f(name)
}
