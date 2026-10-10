package main

import (
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// The moments of a run that a test wants to put code in front of are reached by decorating what the
// deps hand out, never by a variable of the program: a decorator is built by the test that needs
// it, in front of the real store or the real policy, and lives as long as the deps it was put in.

// bindingSeams are moments of the binding store at which a test runs code, each just before the
// operation it is named for.
type bindingSeams struct {
	// beforeLoad runs before the binding of a repository is read.
	beforeLoad func()
	// beforeStaleReplace runs before bind replaces the binding it judged stale: the window in which
	// another process can bind a live workflow, which the store's BindIfUnchanged must then refuse
	// to overwrite.
	beforeStaleReplace func()
	// beforeHookUnbind runs before the hook removes the binding of a closed workflow: the window in
	// which another process can bind the next workflow, which the store's UnbindIfUnchanged must
	// then refuse to remove.
	beforeHookUnbind func()
}

// withBindingSeams is d with its binding store decorated by the seams.
func (d deps) withBindingSeams(seams bindingSeams) deps {
	open := d.openBindings
	d.openBindings = func() (projection.BindingStore, error) {
		store, err := open()
		if err != nil {
			return nil, err
		}
		return seamedBindingStore{BindingStore: store, seams: seams}, nil
	}
	return d
}

// seamedBindingStore is a binding store with a test's seams in front of three of its operations.
type seamedBindingStore struct {
	projection.BindingStore
	seams bindingSeams
}

func (s seamedBindingStore) Load(repoKey string) (projection.Loaded, error) {
	if s.seams.beforeLoad != nil {
		s.seams.beforeLoad()
	}
	return s.BindingStore.Load(repoKey)
}

func (s seamedBindingStore) BindIfUnchanged(repoKey, projectID, workflowID string, now time.Time, expected projection.Binding) error {
	if s.seams.beforeStaleReplace != nil {
		s.seams.beforeStaleReplace()
	}
	return s.BindingStore.BindIfUnchanged(repoKey, projectID, workflowID, now, expected)
}

func (s seamedBindingStore) UnbindIfUnchanged(repoKey string, expected projection.Binding) (bool, error) {
	if s.seams.beforeHookUnbind != nil {
		s.seams.beforeHookUnbind()
	}
	return s.BindingStore.UnbindIfUnchanged(repoKey, expected)
}

// withGateDecision is d with a gate that runs before runs, then decides as the program's does: a
// test makes the gate panic where a bug in it would by passing a function that panics.
func (d deps) withGateDecision(before func()) deps {
	decide := d.gate
	d.gate = func(in projection.GateInput) projection.GateResult {
		before()
		return decide(in)
	}
	return d
}

// withUnavailableProber is d with workflow.UnavailableProber, which confirms nothing, so that a test
// asserting "every dependency is unavailable" does not depend on the gentle-ai binary or the memory
// files of the machine it runs on.
func (d deps) withUnavailableProber() deps {
	d.workflowProber = func() workflow.DependencyProber { return workflow.UnavailableProber{} }
	return d
}
