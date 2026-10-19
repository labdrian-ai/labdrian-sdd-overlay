package app_test

// OnInstructions (C23 M6): the memory instructions of the workflow a repository follows, read from
// the binding and the workflow exactly as OnPrompt reads them, decided by the domain
// (projection.Instructions), and strictly read-only: a closed workflow's binding is left for the
// prompt hook to remove.

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

func engramName(identity string) string { return "engram:" + identity }

func TestOnInstructionsGivesTheDomainsBlockForTheWorkflowTheRepositoryFollows(t *testing.T) {
	w := newHookWorld(workflow.StatusRunning)
	w.service.EngramName = engramName

	got := w.service.OnInstructions(prompt(repoDir, ""))

	want := projection.Instructions(projection.ProjectionInput{
		Binding:  projection.Loaded{Classification: projection.ClassificationOwned, Binding: w.bindings.bound(repoKey)},
		Workflow: ptr(owned(workflow.StatusRunning)),
	}, engramName)
	if got.Kind != app.PromptProjection || got.Result != want || want.Block == "" {
		t.Fatalf("OnInstructions = %+v, want the domain's block %+v", got, want)
	}
	if got := w.service.OnInstructions(prompt("", repoDir)); got.Result != want {
		t.Errorf("without a directory in the input the process's is used: OnInstructions = %+v", got)
	}
}

func TestOnInstructionsSaysNothingWithoutABindingAndWarnsForAStoreItCannotRead(t *testing.T) {
	w := newHookWorld(workflow.StatusRunning)
	if got := w.service.OnInstructions(prompt("/elsewhere", "")); got.Kind != app.PromptSilent {
		t.Errorf("no repository: OnInstructions = %+v, want PromptSilent", got)
	}
	delete(w.bindings.files, repoKey)
	if got := w.service.OnInstructions(prompt(repoDir, "")); got.Kind != app.PromptProjection || got.Result != (projection.InstructionsResult{}) {
		t.Errorf("nothing bound: OnInstructions = %+v, want nothing", got)
	}
	if len(w.workflows.asked) != 0 {
		t.Errorf("the workflow store was asked about %v with nothing bound", w.workflows.asked)
	}
	w.bindings.loadErr = errStoreDown
	if got := w.service.OnInstructions(prompt(repoDir, "")); got.Kind != app.PromptWarning || got.Warning != projection.StoreWarning(errStoreDown) {
		t.Errorf("a store that cannot be read: OnInstructions = %+v, want its warning", got)
	}
}

func TestOnInstructionsNeverRemovesTheBindingOfAClosedWorkflow(t *testing.T) {
	w := newHookWorld(workflow.StatusClosed)
	w.workflows.put("proj-1", "wf-1", closed())

	got := w.service.OnInstructions(prompt(repoDir, ""))

	if got.Result.Block != "" || got.Result.Warning == "" {
		t.Errorf("a closed workflow: OnInstructions = %+v, want no block and its note", got)
	}
	if w.bindings.removals != 0 || w.bindings.bound(repoKey).WorkflowID != "wf-1" {
		t.Errorf("the binding of a closed workflow was touched (%d removals)", w.bindings.removals)
	}
}
