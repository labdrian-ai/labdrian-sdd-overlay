package app_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// hookWorld is one bound repository: a binding of repoDir to wf-1 of proj-1 and the workflow it
// names, in the status the test gives it.
type hookWorld struct {
	locator   *fakeLocator
	bindings  *memoryBindings
	workflows *fakeWorkflows
	service   app.HookService
}

var editTools = []string{"Write", "Edit"}

func newHookWorld(status workflow.Status) *hookWorld {
	w := &hookWorld{locator: newLocator(), bindings: newBindings(), workflows: newWorkflows()}
	w.workflows.put("proj-1", "wf-1", owned(status))
	w.bindings.put(repoKey, projection.NewBinding(repoKey, "proj-1", "wf-1", firstInstant))
	w.service = app.HookService{Repositories: w.locator, Bindings: w.bindings, Workflows: w.workflows, EditTools: editTools}
	return w
}

func prompt(inputDir, processDir string) app.PromptRequest {
	return app.PromptRequest{InputDir: inputDir, ProcessDir: processDir}
}

func toolCall(name string) app.ToolCallRequest {
	return app.ToolCallRequest{InputDir: repoDir, Call: projection.ToolCall{Name: name}}
}

func TestOnPromptSaysNothingWhereThereIsNothingToSay(t *testing.T) {
	t.Run("no repository", func(t *testing.T) {
		w := newHookWorld(workflow.StatusRunning)
		if got := w.service.OnPrompt(prompt("/elsewhere", "/also-elsewhere")); got.Kind != app.PromptSilent {
			t.Fatalf("OnPrompt = %+v, want PromptSilent", got)
		}
		if w.bindings.loads != 0 {
			t.Errorf("the binding store was read %d times with no repository, want 0", w.bindings.loads)
		}
	})
	t.Run("a repository nothing is bound to", func(t *testing.T) {
		w := newHookWorld(workflow.StatusRunning)
		delete(w.bindings.files, repoKey)
		if got := w.service.OnPrompt(prompt(repoDir, "")); got.Kind != app.PromptProjection || got.Result.Context != "" || got.Result.Warning != "" || got.Result.Unbind {
			t.Fatalf("OnPrompt = %+v, want a projection of nothing", got)
		}
	})
}

func TestOnPromptProjectsTheWorkflowTheRepositoryFollows(t *testing.T) {
	w := newHookWorld(workflow.StatusRunning)

	got := w.service.OnPrompt(prompt(repoDir, ""))

	if got.Kind != app.PromptProjection || got.Result.Warning != "" {
		t.Fatalf("OnPrompt = %+v, want a projection with no warning", got)
	}
	for _, want := range []string{"workflow: wf-1 (project: proj-1)", "status: running"} {
		if !strings.Contains(got.Result.Context, want) {
			t.Errorf("the context does not say %q:\n%s", want, got.Result.Context)
		}
	}
	if len(w.workflows.asked) != 1 || w.workflows.asked[0] != "proj-1/wf-1" {
		t.Errorf("the workflow store was asked about %v, want proj-1/wf-1 once", w.workflows.asked)
	}
}

func TestOnPromptFindsTheRepositoryFromTheDirectoryOfTheInputAndElseOfTheProcess(t *testing.T) {
	w := newHookWorld(workflow.StatusRunning)
	w.locator.keys["/work/other"] = otherKey

	if got := w.service.OnPrompt(prompt(repoDir, "/work/other")); !strings.Contains(got.Result.Context, "wf-1") {
		t.Errorf("with a directory in the input the repository is %s: OnPrompt = %+v", repoDir, got)
	}
	if got := w.service.OnPrompt(prompt("", repoDir)); !strings.Contains(got.Result.Context, "wf-1") {
		t.Errorf("without one the directory of the process is used: OnPrompt = %+v", got)
	}
	if got := w.service.OnPrompt(prompt("", "/work/other")); got.Result.Context != "" {
		t.Errorf("another repository is not bound: OnPrompt = %+v", got)
	}
}

func TestOnPromptWarnsOnceWhenTheBindingStoreCannotBeRead(t *testing.T) {
	w := newHookWorld(workflow.StatusRunning)
	w.bindings.loadErr = errStoreDown

	got := w.service.OnPrompt(prompt(repoDir, ""))

	if got.Kind != app.PromptWarning || got.Warning != projection.StoreWarning(errStoreDown) {
		t.Fatalf("OnPrompt = %+v, want the warning for a store that cannot be read", got)
	}
	if len(w.workflows.asked) != 0 {
		t.Errorf("the workflow store was asked about %v after the binding store failed, want nothing", w.workflows.asked)
	}
}

func TestOnPromptAsksForTheWorkflowOnlyOfAnOwnedBinding(t *testing.T) {
	w := newHookWorld(workflow.StatusRunning)
	w.bindings.files[repoKey] = []byte(`{"version": 2}`)

	got := w.service.OnPrompt(prompt(repoDir, ""))

	if got.Kind != app.PromptProjection || got.Result.Warning == "" {
		t.Fatalf("OnPrompt = %+v, want a warning about a binding that cannot be followed", got)
	}
	if len(w.workflows.asked) != 0 {
		t.Errorf("the workflow store was asked about %v for a binding that is not ours, want nothing", w.workflows.asked)
	}
}

func TestOnPromptRemovesTheBindingOfAClosedWorkflowAndSaysSo(t *testing.T) {
	w := newHookWorld(workflow.StatusClosed)
	w.workflows.put("proj-1", "wf-1", closed())

	got := w.service.OnPrompt(prompt(repoDir, ""))

	if !got.Result.Unbind || !strings.Contains(got.Result.Context, "binding to it was removed") {
		t.Fatalf("OnPrompt = %+v, want a note that the binding was removed", got)
	}
	if loaded := w.bindings.read(repoKey); loaded.Classification != projection.ClassificationAbsent {
		t.Errorf("the store holds %v, want the binding removed", loaded.Classification)
	}
}

func TestOnPromptLeavesABindingThatChangedAfterItWasRead(t *testing.T) {
	w := newHookWorld(workflow.StatusClosed)
	w.workflows.put("proj-1", "wf-1", closed())
	fresh := projection.NewBinding(repoKey, "proj-1", "wf-2", firstInstant.Add(time.Minute))
	w.bindings.beforeRemoval = func() { w.bindings.put(repoKey, fresh) }

	got := w.service.OnPrompt(prompt(repoDir, ""))

	if !strings.Contains(got.Result.Context, "had already changed, so it was left alone") {
		t.Fatalf("OnPrompt = %+v, want a note that the binding was left alone", got)
	}
	if stored := w.bindings.bound(repoKey); stored != fresh {
		t.Errorf("the store holds %+v, want the fresh binding %+v kept", stored, fresh)
	}
}

func TestOnPromptSaysWhenTheBindingCouldNotBeRemoved(t *testing.T) {
	w := newHookWorld(workflow.StatusClosed)
	w.workflows.put("proj-1", "wf-1", closed())
	w.bindings.unbindErr = errors.New("disk full")

	got := w.service.OnPrompt(prompt(repoDir, ""))

	if !strings.Contains(got.Result.Context, "failed") || !strings.Contains(got.Result.Context, "disk full") || !strings.Contains(got.Result.Context, "labdrian workflow unbind") {
		t.Fatalf("OnPrompt = %+v, want a note that the removal failed, why, and how to finish it", got)
	}
	if loaded := w.bindings.read(repoKey); loaded.Classification != projection.ClassificationOwned {
		t.Errorf("the store holds %v, want the binding kept", loaded.Classification)
	}
}

func TestOnPromptAsksThePolicyItWasGiven(t *testing.T) {
	w := newHookWorld(workflow.StatusRunning)
	var seen projection.ProjectionInput
	w.service.Project = func(in projection.ProjectionInput) projection.ProjectionResult {
		seen = in
		return projection.ProjectionResult{Context: "from the policy it was given"}
	}

	got := w.service.OnPrompt(prompt(repoDir, ""))

	if got.Result.Context != "from the policy it was given" {
		t.Fatalf("OnPrompt = %+v, want the answer of the injected policy", got)
	}
	if seen.Binding.Binding.WorkflowID != "wf-1" || seen.Workflow == nil || seen.Workflow.State.Status != workflow.StatusRunning {
		t.Errorf("the policy was given %+v, want the binding and the running workflow", seen)
	}
}

func TestOnToolCallAnswersForTheToolsItNeverChecksWithoutTouchingAnyStore(t *testing.T) {
	w := newHookWorld(workflow.StatusPaused)

	for _, name := range []string{"Read", "Bash", "Grep", "", "edit"} {
		if got := w.service.OnToolCall(toolCall(name)); got.Decided {
			t.Errorf("OnToolCall(%q) = %+v, want it undecided", name, got)
		}
	}
	if w.locator.asked != 0 || w.bindings.loads != 0 || len(w.workflows.asked) != 0 {
		t.Errorf("the stores were asked (%d repositories, %d bindings, %v workflows) for tools the gate never checks, want none", w.locator.asked, w.bindings.loads, w.workflows.asked)
	}
}

func TestOnToolCallDeniesTheEditToolsOfAPausedWorkflowAndNothingElse(t *testing.T) {
	w := newHookWorld(workflow.StatusPaused)

	for _, name := range editTools {
		got := w.service.OnToolCall(toolCall(name))
		if !got.Decided || !got.Result.Deny || !strings.Contains(got.Result.Explanation(), "wf-1") {
			t.Errorf("OnToolCall(%q) = %+v, want a denial that names the workflow", name, got)
		}
	}
	running := newHookWorld(workflow.StatusRunning)
	if got := running.service.OnToolCall(toolCall("Edit")); !got.Decided || got.Result.Deny {
		t.Errorf("an edit of a running workflow: OnToolCall = %+v, want it decided and allowed", got)
	}
}

func TestOnToolCallIsSilentWhereItCannotFollow(t *testing.T) {
	t.Run("no repository", func(t *testing.T) {
		w := newHookWorld(workflow.StatusPaused)
		req := toolCall("Edit")
		req.InputDir = "/elsewhere"
		if got := w.service.OnToolCall(req); got.Decided {
			t.Fatalf("OnToolCall = %+v, want it undecided", got)
		}
		if w.bindings.loads != 0 {
			t.Errorf("the binding store was read %d times with no repository, want 0", w.bindings.loads)
		}
	})
	t.Run("a binding store that cannot be read", func(t *testing.T) {
		w := newHookWorld(workflow.StatusPaused)
		w.bindings.loadErr = errStoreDown
		if got := w.service.OnToolCall(toolCall("Edit")); got.Decided {
			t.Fatalf("OnToolCall = %+v, want it undecided, with no warning: the prompt hook warns", got)
		}
	})
}

func TestOnToolCallFindsTheRepositoryFromTheInputAndElseTheProcess(t *testing.T) {
	w := newHookWorld(workflow.StatusPaused)
	req := app.ToolCallRequest{ProcessDir: repoDir, Call: projection.ToolCall{Name: "Edit"}}
	if got := w.service.OnToolCall(req); !got.Decided || !got.Result.Deny {
		t.Fatalf("OnToolCall = %+v, want the process directory used when the input names none", got)
	}
}

func TestOnToolCallAsksThePolicyItWasGiven(t *testing.T) {
	w := newHookWorld(workflow.StatusPaused)
	var seen projection.GateInput
	w.service.Gate = func(in projection.GateInput) projection.GateResult {
		seen = in
		return projection.GateResult{Warning: "from the policy it was given"}
	}

	got := w.service.OnToolCall(toolCall("Edit"))

	if !got.Decided || got.Result.Warning != "from the policy it was given" {
		t.Fatalf("OnToolCall = %+v, want the answer of the injected policy", got)
	}
	if seen.Call.Name != "Edit" || strings.Join(seen.EditTools, ",") != "Write,Edit" || seen.Workflow == nil {
		t.Errorf("the policy was given %+v, want the call, the edit tools and the workflow", seen)
	}
}

func TestAPolicyThatPanicsIsNotCaughtHere(t *testing.T) {
	// The command that serves the hook recovers a panic and turns it into one warning; the service
	// must let it through, not swallow it into silence.
	w := newHookWorld(workflow.StatusPaused)
	w.service.Gate = func(projection.GateInput) projection.GateResult { panic("boom") }

	defer func() {
		if recover() == nil {
			t.Error("the panic of the policy did not reach the caller")
		}
	}()
	w.service.OnToolCall(toolCall("Edit"))
}
