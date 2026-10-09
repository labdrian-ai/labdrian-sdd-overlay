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

// bindWorld is one repository, one store and the workflows a test gives it.
type bindWorld struct {
	locator   *fakeLocator
	bindings  *memoryBindings
	workflows *fakeWorkflows
	clock     *fakeClock
	service   app.BindWorkflow
}

var firstInstant = time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)

func newBindWorld() *bindWorld {
	w := &bindWorld{locator: newLocator(), bindings: newBindings(), workflows: newWorkflows(), clock: &fakeClock{now: firstInstant}}
	w.service = app.BindWorkflow{Repositories: w.locator, Bindings: w.bindings, Workflows: w.workflows, Clock: w.clock}
	return w
}

func (w *bindWorld) bind(project, id string) (projection.Binding, error) {
	return w.service.Bind(app.BindRequest{Dir: repoDir, ProjectID: project, WorkflowID: id})
}

func (w *bindWorld) mustBind(t *testing.T, project, id string) projection.Binding {
	t.Helper()
	b, err := w.bind(project, id)
	if err != nil {
		t.Fatalf("Bind(%s/%s) = %v, want nil", project, id, err)
	}
	return b
}

func TestBindRecordsTheWorkflowAtTheTimeOfTheClock(t *testing.T) {
	w := newBindWorld()
	w.workflows.put("proj-1", "wf-1", owned(workflow.StatusRunning))

	got := w.mustBind(t, "proj-1", "wf-1")

	want := projection.NewBinding(repoKey, "proj-1", "wf-1", firstInstant)
	if got != want {
		t.Fatalf("Bind = %+v, want %+v", got, want)
	}
	if stored := w.bindings.bound(repoKey); stored != want {
		t.Fatalf("the store holds %+v, want %+v", stored, want)
	}
	if w.clock.asked != 1 {
		t.Errorf("the clock was asked %d times, want 1", w.clock.asked)
	}
}

func TestBindingTheSameWorkflowAgainKeepsTheOriginalBinding(t *testing.T) {
	w := newBindWorld()
	w.workflows.put("proj-1", "wf-1", owned(workflow.StatusRunning))
	first := w.mustBind(t, "proj-1", "wf-1")

	w.clock.now = firstInstant.Add(time.Hour)
	second := w.mustBind(t, "proj-1", "wf-1")

	if second != first {
		t.Fatalf("the second Bind = %+v, want the first, %+v, with its original BoundAt", second, first)
	}
}

func TestBindOpensAnyOpenWorkflow(t *testing.T) {
	for _, status := range []workflow.Status{workflow.StatusCreated, workflow.StatusRunning, workflow.StatusPaused} {
		w := newBindWorld()
		w.workflows.put("proj-1", "wf-1", owned(status))
		if _, err := w.bind("proj-1", "wf-1"); err != nil {
			t.Errorf("a %s workflow: Bind = %v, want nil", status, err)
		}
	}
}

func TestBindRefusesWhatItCannotBindWithATypedError(t *testing.T) {
	drifted := workflow.Loaded{Classification: workflow.ClassificationDrifted, Detail: "event 1 does not follow event 0"}

	t.Run("no repository", func(t *testing.T) {
		w := newBindWorld()
		_, err := w.service.Bind(app.BindRequest{Dir: "/elsewhere", ProjectID: "proj-1", WorkflowID: "wf-1"})
		var refusal *app.NoRepositoryError
		if !errors.As(err, &refusal) {
			t.Fatalf("Bind = %v, want a *NoRepositoryError", err)
		}
	})
	t.Run("a project id that is not valid", func(t *testing.T) {
		_, err := newBindWorld().bind("no spaces", "wf-1")
		var refusal *app.InvalidIdentifierError
		if !errors.As(err, &refusal) || !strings.Contains(refusal.Error(), "project_id") {
			t.Fatalf("Bind = %v, want an *InvalidIdentifierError naming project_id", err)
		}
	})
	t.Run("a workflow id that is not valid", func(t *testing.T) {
		_, err := newBindWorld().bind("proj-1", "../wf")
		var refusal *app.InvalidIdentifierError
		if !errors.As(err, &refusal) || !strings.Contains(refusal.Error(), "workflow_id") {
			t.Fatalf("Bind = %v, want an *InvalidIdentifierError naming workflow_id", err)
		}
	})
	t.Run("a workflow that does not exist", func(t *testing.T) {
		_, err := newBindWorld().bind("proj-1", "wf-none")
		var refusal *app.WorkflowAbsentError
		if !errors.As(err, &refusal) || refusal.ProjectID != "proj-1" || refusal.WorkflowID != "wf-none" {
			t.Fatalf("Bind = %v, want a *WorkflowAbsentError naming proj-1 and wf-none", err)
		}
	})
	t.Run("a workflow that is not owned", func(t *testing.T) {
		w := newBindWorld()
		w.workflows.put("proj-1", "wf-1", drifted)
		_, err := w.bind("proj-1", "wf-1")
		var refusal *app.WorkflowNotOwnedError
		if !errors.As(err, &refusal) || refusal.Classification != workflow.ClassificationDrifted || refusal.Detail != drifted.Detail {
			t.Fatalf("Bind = %v, want a *WorkflowNotOwnedError with the classification and the detail", err)
		}
	})
	t.Run("a closed workflow", func(t *testing.T) {
		w := newBindWorld()
		w.workflows.put("proj-1", "wf-1", closed())
		_, err := w.bind("proj-1", "wf-1")
		var refusal *app.WorkflowClosedError
		if !errors.As(err, &refusal) || refusal.Outcome != workflow.OutcomeAbandoned {
			t.Fatalf("Bind = %v, want a *WorkflowClosedError with the outcome", err)
		}
	})
}

func TestBindJudgesTheWorkflowBeforeItTouchesTheStore(t *testing.T) {
	w := newBindWorld()
	w.bindings.loadErr = errStoreDown

	_, err := w.bind("proj-1", "wf-none")

	var refusal *app.WorkflowAbsentError
	if !errors.As(err, &refusal) {
		t.Fatalf("Bind = %v, want the workflow to be refused first", err)
	}
	if w.bindings.loads != 0 {
		t.Errorf("the store was read %d times before the workflow was refused, want 0", w.bindings.loads)
	}
}

func TestBindNeverReplacesABindingThatMayStillBeActive(t *testing.T) {
	for _, status := range []workflow.Status{workflow.StatusCreated, workflow.StatusRunning, workflow.StatusPaused} {
		t.Run(string(status), func(t *testing.T) {
			w := newBindWorld()
			w.workflows.put("proj-1", "wf-bound", owned(status))
			w.workflows.put("proj-1", "wf-next", owned(workflow.StatusRunning))
			bound := w.mustBind(t, "proj-1", "wf-bound")

			_, err := w.bind("proj-1", "wf-next")

			var refusal *app.BoundToLiveWorkflowError
			if !errors.As(err, &refusal) || refusal.Bound != bound || refusal.Status != status {
				t.Fatalf("Bind = %v, want a *BoundToLiveWorkflowError naming the bound workflow and its status %s", err, status)
			}
			if stored := w.bindings.bound(repoKey); stored != bound {
				t.Errorf("the store holds %+v after the refusal, want the original %+v", stored, bound)
			}
		})
	}
}

func TestBindNeverReplacesABindingWhoseWorkflowCannotBeRead(t *testing.T) {
	w := newBindWorld()
	w.workflows.put("proj-1", "wf-bound", owned(workflow.StatusRunning))
	w.workflows.put("proj-1", "wf-next", owned(workflow.StatusRunning))
	bound := w.mustBind(t, "proj-1", "wf-bound")
	w.workflows.put("proj-1", "wf-bound", workflow.Loaded{Classification: workflow.ClassificationUnavailable, Detail: "permission denied"})

	_, err := w.bind("proj-1", "wf-next")

	var refusal *app.BoundToUnreadableWorkflowError
	if !errors.As(err, &refusal) || refusal.Bound != bound || refusal.Detail != "permission denied" {
		t.Fatalf("Bind = %v, want a *BoundToUnreadableWorkflowError with the reason", err)
	}
	if stored := w.bindings.bound(repoKey); stored != bound {
		t.Errorf("the store holds %+v after the refusal, want the original %+v", stored, bound)
	}
}

func TestBindReplacesABindingThatCanNeverBeFollowedAgain(t *testing.T) {
	for _, tc := range []struct {
		name  string
		after workflow.Loaded
	}{
		{"a closed workflow", closed()},
		{"a workflow whose log is gone", workflow.Loaded{Classification: workflow.ClassificationAbsent}},
		{"a workflow whose log drifted", workflow.Loaded{Classification: workflow.ClassificationDrifted}},
		{"a workflow whose log is malformed", workflow.Loaded{Classification: workflow.ClassificationMalformed}},
		{"a workflow whose log is not ours", workflow.Loaded{Classification: workflow.ClassificationForeign}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newBindWorld()
			w.workflows.put("proj-1", "wf-bound", owned(workflow.StatusRunning))
			w.workflows.put("proj-1", "wf-next", owned(workflow.StatusRunning))
			w.mustBind(t, "proj-1", "wf-bound")
			w.workflows.put("proj-1", "wf-bound", tc.after)
			w.clock.now = firstInstant.Add(time.Minute)

			got := w.mustBind(t, "proj-1", "wf-next")

			want := projection.NewBinding(repoKey, "proj-1", "wf-next", w.clock.now)
			if got != want {
				t.Fatalf("Bind = %+v, want %+v", got, want)
			}
		})
	}
}

func TestBindReplacesOnlyTheBindingItJudgedStale(t *testing.T) {
	w := newBindWorld()
	w.workflows.put("proj-1", "wf-bound", owned(workflow.StatusRunning))
	w.workflows.put("proj-1", "wf-next", owned(workflow.StatusRunning))
	w.workflows.put("proj-1", "wf-other", owned(workflow.StatusRunning))
	w.mustBind(t, "proj-1", "wf-bound")
	w.workflows.put("proj-1", "wf-bound", closed())

	// In the window between the judgment and the replacement another process binds a live workflow.
	rival := projection.NewBinding(repoKey, "proj-1", "wf-other", firstInstant.Add(time.Second))
	w.bindings.beforeReplace = func() { w.bindings.put(repoKey, rival) }

	_, err := w.bind("proj-1", "wf-next")

	var stored *app.BindingStoreError
	if !errors.As(err, &stored) || !errors.Is(err, projection.ErrBindingChanged) {
		t.Fatalf("Bind = %v, want a *BindingStoreError holding projection.ErrBindingChanged", err)
	}
	if now := w.bindings.bound(repoKey); now != rival {
		t.Errorf("the store holds %+v, want the rival's binding %+v left alone", now, rival)
	}
}

func TestBindSaysWhenTheBindingChangedBetweenTheCheckAndTheWrite(t *testing.T) {
	w := newBindWorld()
	w.workflows.put("proj-1", "wf-1", owned(workflow.StatusRunning))
	w.bindings.afterBind = func() {
		w.bindings.put(repoKey, projection.NewBinding(repoKey, "proj-1", "wf-2", firstInstant.Add(time.Second)))
	}

	_, err := w.bind("proj-1", "wf-1")

	var refusal *app.ChangedConcurrentlyError
	if !errors.As(err, &refusal) {
		t.Fatalf("Bind = %v, want a *ChangedConcurrentlyError", err)
	}
	if refusal.Found.WorkflowID != "wf-2" || refusal.WorkflowID != "wf-1" || refusal.ProjectID != "proj-1" {
		t.Errorf("the error holds %+v, want the binding found (wf-2) and the workflow asked for (wf-1)", refusal)
	}
}

func TestBindSaysWhenTheBindingCannotBeReadBack(t *testing.T) {
	w := newBindWorld()
	w.workflows.put("proj-1", "wf-1", owned(workflow.StatusRunning))
	w.bindings.afterBind = func() { w.bindings.files[repoKey] = []byte("not a binding") }

	_, err := w.bind("proj-1", "wf-1")

	var refusal *app.ReadBackError
	if !errors.As(err, &refusal) || refusal.Classification != projection.ClassificationMalformed || refusal.Detail == "" {
		t.Fatalf("Bind = %v, want a *ReadBackError with the classification and the detail", err)
	}
}

func TestBindPassesTheRefusalsOfTheStoreThrough(t *testing.T) {
	t.Run("a binding file that is not ours", func(t *testing.T) {
		w := newBindWorld()
		w.workflows.put("proj-1", "wf-1", owned(workflow.StatusRunning))
		w.bindings.files[repoKey] = []byte(`{"version": 2}`)

		_, err := w.bind("proj-1", "wf-1")

		var stored *app.BindingStoreError
		if !errors.As(err, &stored) || !errors.Is(err, projection.ErrRefuseForeignBinding) {
			t.Fatalf("Bind = %v, want a *BindingStoreError holding projection.ErrRefuseForeignBinding", err)
		}
		if string(w.bindings.files[repoKey]) != `{"version": 2}` {
			t.Errorf("the file was changed to %q", w.bindings.files[repoKey])
		}
	})
	t.Run("a store that cannot be read", func(t *testing.T) {
		w := newBindWorld()
		w.workflows.put("proj-1", "wf-1", owned(workflow.StatusRunning))
		w.bindings.loadErr = errStoreDown

		_, err := w.bind("proj-1", "wf-1")

		var stored *app.BindingStoreError
		if !errors.As(err, &stored) || !errors.Is(err, errStoreDown) || err.Error() != errStoreDown.Error() {
			t.Fatalf("Bind = %v, want a *BindingStoreError that says what the store said", err)
		}
	})
}

func TestUnbindRemovesTheBindingAndIsIdempotent(t *testing.T) {
	w := newBindWorld()
	w.workflows.put("proj-1", "wf-1", owned(workflow.StatusRunning))

	if removed, err := w.service.Unbind(repoDir); removed || err != nil {
		t.Fatalf("Unbind with nothing bound = %v, %v, want false, nil", removed, err)
	}
	w.mustBind(t, "proj-1", "wf-1")
	if removed, err := w.service.Unbind(repoDir); !removed || err != nil {
		t.Fatalf("Unbind = %v, %v, want true, nil", removed, err)
	}
	if removed, err := w.service.Unbind(repoDir); removed || err != nil {
		t.Fatalf("the second Unbind = %v, %v, want false, nil", removed, err)
	}
	if loaded := w.bindings.read(repoKey); loaded.Classification != projection.ClassificationAbsent {
		t.Errorf("the store holds %v after unbinding, want nothing", loaded.Classification)
	}
}

func TestUnbindRefusesWhatItCannotRemove(t *testing.T) {
	t.Run("no repository", func(t *testing.T) {
		_, err := newBindWorld().service.Unbind("/elsewhere")
		var refusal *app.NoRepositoryError
		if !errors.As(err, &refusal) {
			t.Fatalf("Unbind = %v, want a *NoRepositoryError", err)
		}
	})
	t.Run("a binding file that is not ours", func(t *testing.T) {
		w := newBindWorld()
		w.bindings.files[repoKey] = []byte("this is not a binding")

		removed, err := w.service.Unbind(repoDir)

		var stored *app.BindingStoreError
		if removed || !errors.As(err, &stored) || !errors.Is(err, projection.ErrRefuseMalformedBinding) {
			t.Fatalf("Unbind = %v, %v, want false and a *BindingStoreError holding projection.ErrRefuseMalformedBinding", removed, err)
		}
		if string(w.bindings.files[repoKey]) != "this is not a binding" {
			t.Errorf("the file was changed to %q", w.bindings.files[repoKey])
		}
	})
}

func TestDescribeSaysWhatTheRepositoryIsBoundTo(t *testing.T) {
	w := newBindWorld()
	w.workflows.put("proj-1", "wf-1", owned(workflow.StatusPaused))

	view, err := w.service.Describe(repoDir)
	if err != nil || view.Binding.Classification != projection.ClassificationAbsent || view.Workflow != nil {
		t.Fatalf("Describe with nothing bound = %+v, %v, want an absent binding and no workflow", view, err)
	}
	if len(w.workflows.asked) != 0 {
		t.Errorf("the workflow store was asked about %v with nothing bound, want nothing", w.workflows.asked)
	}

	bound := w.mustBind(t, "proj-1", "wf-1")
	view, err = w.service.Describe(repoDir)
	if err != nil {
		t.Fatalf("Describe = %v", err)
	}
	if view.Binding.Classification != projection.ClassificationOwned || view.Binding.Binding != bound {
		t.Errorf("the binding is %+v, want the owned binding %+v", view.Binding, bound)
	}
	if view.Workflow == nil || view.Workflow.State.Status != workflow.StatusPaused {
		t.Errorf("the workflow is %+v, want the paused one", view.Workflow)
	}
}

func TestDescribeReportsAWorkflowItCannotReadAsDataAndABindingThatIsNotOursWithoutAWorkflow(t *testing.T) {
	w := newBindWorld()
	w.workflows.put("proj-1", "wf-1", owned(workflow.StatusRunning))
	w.mustBind(t, "proj-1", "wf-1")
	w.workflows.put("proj-1", "wf-1", workflow.Loaded{Classification: workflow.ClassificationUnavailable, Detail: "permission denied"})

	view, err := w.service.Describe(repoDir)
	if err != nil || view.Workflow == nil || view.Workflow.Classification != workflow.ClassificationUnavailable || view.Workflow.Detail != "permission denied" {
		t.Fatalf("Describe = %+v, %v, want the unavailable workflow as data", view, err)
	}

	w.bindings.files[repoKey] = []byte(`{"version": 2}`)
	view, err = w.service.Describe(repoDir)
	if err != nil || view.Binding.Classification != projection.ClassificationForeign || view.Binding.Detail == "" || view.Workflow != nil {
		t.Fatalf("Describe = %+v, %v, want a foreign binding with its detail and no workflow", view, err)
	}
}

func TestDescribeRefusesWhatItCannotDescribe(t *testing.T) {
	_, err := newBindWorld().service.Describe("/elsewhere")
	var refusal *app.NoRepositoryError
	if !errors.As(err, &refusal) {
		t.Fatalf("Describe with no repository = %v, want a *NoRepositoryError", err)
	}

	w := newBindWorld()
	w.bindings.loadErr = errStoreDown
	_, err = w.service.Describe(repoDir)
	var stored *app.BindingStoreError
	if !errors.As(err, &stored) || !errors.Is(err, errStoreDown) {
		t.Fatalf("Describe with a store that cannot be read = %v, want a *BindingStoreError", err)
	}
}
