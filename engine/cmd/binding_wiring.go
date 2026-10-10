package main

import (
	"sync"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// This file is where the use cases of the session binding and of the projection hook
// (engine/projection/app) are wired to their adapters: the finder of the repository (engine/gitfs),
// the binding store the deps open (engine/projection/fsstore in the program), the workflow log and
// the clock of the machine. The commands that use them (workflow_bind.go, projection_hook.go)
// parse the command line or decode the hook input, call a use case and say the answer in their own
// words.

// bindWorkflow builds the use case of 'workflow bind', 'unbind' and 'binding'.
func (d deps) bindWorkflow() app.BindWorkflow {
	return app.BindWorkflow{
		Repositories: newRepoLocator(),
		Bindings:     d.runBindings(),
		Workflows:    workflowReader{},
		Clock:        wallClock{},
	}
}

// promptHook builds the use case of the projection hook for a prompt.
func (d deps) promptHook() app.HookService {
	return app.HookService{
		Repositories: newRepoLocator(),
		Bindings:     d.runBindings(),
		Workflows:    workflowReader{},
		EditTools:    gatedEditTools(),
	}
}

// gateHook builds the use case of the projection hook for a tool call: the prompt's, deciding with
// the policy the deps give (projection.Gate in the program).
func (d deps) gateHook() app.HookService {
	hook := d.promptHook()
	hook.Gate = d.gate
	return hook
}

// runBindings is the binding store of one run of a command: built on first use from the store the
// deps open. It is the one place the stack is put together, so the three use cases cannot differ in
// it.
func (d deps) runBindings() projection.BindingStore {
	return &lazyBindings{open: d.openBindings}
}

// wallClock is the clock of the machine.
type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now() }

// workflowReader reads a workflow through a store built from the environment. A store that cannot
// be built or read is reported as an unavailable workflow, with the reason as its detail, rather
// than as an error: binding must be able to describe a workflow it cannot read, and bind must be
// able to say why it cannot bind one. It builds the store on every call, as the commands always
// did, so a state home that changes between two calls is read as it is then.
type workflowReader struct{}

func (workflowReader) Load(projectID, workflowID string) workflow.Loaded {
	unavailable := func(err error) workflow.Loaded {
		return workflow.Loaded{Classification: workflow.ClassificationUnavailable, Detail: err.Error()}
	}
	store, err := newWorkflowStore()
	if err != nil {
		return unavailable(err)
	}
	loaded, err := store.Load(projectID, workflowID)
	if err != nil {
		return unavailable(err)
	}
	return loaded
}

// lazyBindings is the binding store of the commands: it is built from the environment on first
// use, and a store that cannot be built answers every operation with the reason. A command that
// never goes to the store (a workflow that is refused first, a tool the gate does not check) so
// never builds it, and one that does is told the same words as before, whichever operation was
// first. It is made for one run of a command, which is why it keeps what it built; the building
// happens once even when several goroutines ask at the same time.
type lazyBindings struct {
	open  func() (projection.BindingStore, error)
	once  sync.Once
	store projection.BindingStore
	err   error
}

func (l *lazyBindings) get() (projection.BindingStore, error) {
	l.once.Do(func() { l.store, l.err = l.open() })
	return l.store, l.err
}

func (l *lazyBindings) Load(repoKey string) (projection.Loaded, error) {
	store, err := l.get()
	if err != nil {
		return projection.Loaded{}, err
	}
	return store.Load(repoKey)
}

func (l *lazyBindings) Bind(repoKey, projectID, workflowID string, now time.Time, replace bool) error {
	store, err := l.get()
	if err != nil {
		return err
	}
	return store.Bind(repoKey, projectID, workflowID, now, replace)
}

func (l *lazyBindings) BindIfUnchanged(repoKey, projectID, workflowID string, now time.Time, expected projection.Binding) error {
	store, err := l.get()
	if err != nil {
		return err
	}
	return store.BindIfUnchanged(repoKey, projectID, workflowID, now, expected)
}

func (l *lazyBindings) Unbind(repoKey string) (bool, error) {
	store, err := l.get()
	if err != nil {
		return false, err
	}
	return store.Unbind(repoKey)
}

func (l *lazyBindings) UnbindIfUnchanged(repoKey string, expected projection.Binding) (bool, error) {
	store, err := l.get()
	if err != nil {
		return false, err
	}
	return store.UnbindIfUnchanged(repoKey, expected)
}
