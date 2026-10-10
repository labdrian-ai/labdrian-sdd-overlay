package main

import "github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"

// The moments of a run that a test wants to change are reached through what the deps hand out,
// never through a variable of the program: the test builds the deps it needs, with the one field it
// wants other than the program's.

// withUnavailableProber is d with workflow.UnavailableProber, which confirms nothing, so that a test
// asserting "every dependency is unavailable" does not depend on the gentle-ai binary or the memory
// files of the machine it runs on.
func (d deps) withUnavailableProber() deps {
	d.workflowProber = func() workflow.DependencyProber { return workflow.UnavailableProber{} }
	return d
}
