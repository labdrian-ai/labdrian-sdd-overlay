package app_test

import (
	"errors"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/oddtask"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/oddtask/app"
)

// fakeDocs answers Read with file or err and remembers what it was asked.
type fakeDocs struct {
	file      oddtask.File
	err       error
	reads     int
	path, cwd string
}

func (f *fakeDocs) Read(path, cwd string) (oddtask.File, error) {
	f.reads++
	f.path, f.cwd = path, cwd
	return f.file, f.err
}

const shapedDoc = "## Tasks\n\n- [ ] T2 — S1 — delegated — x — shaped: L3 — commit: -\n- [ ] T4 — S1 — delegated — y — commit: -\n\n" +
	"## Log\n\n- **L3** (2026-10-28, shaper): design\n"

func TestCheckAllowsACitedTaskTheShaperMarked(t *testing.T) {
	docs := &fakeDocs{file: oddtask.File{Path: "/repo/odd/tasks/f.md", Text: shapedDoc}}
	got := app.New(docs).Check("Spec: odd/tasks/f.md, T2, S1", "/repo")
	if got != "" {
		t.Fatalf("Check = %q, want allowed", got)
	}
	if docs.path != "odd/tasks/f.md" || docs.cwd != "/repo" {
		t.Fatalf("Read(%q, %q), want the cited path and the session cwd", docs.path, docs.cwd)
	}
}

func TestCheckDeniesAnUnmarkedTaskNamingTheResolvedPath(t *testing.T) {
	docs := &fakeDocs{file: oddtask.File{Path: "/repo/odd/tasks/f.md", Text: shapedDoc}}
	want := oddtask.Reason(`task T4 of /repo/odd/tasks/f.md is not marked as designed by the shaper (its line has no "shaped: L<n>")`)
	if got := app.New(docs).Check("Spec: odd/tasks/f.md, T2, T4", "/repo"); got != want {
		t.Fatalf("Check = %q, want %q", got, want)
	}
}

func TestCheckReadsNothingWhenThePromptCitesNoTask(t *testing.T) {
	cases := map[string]oddtask.Reason{
		"build it": oddtask.ReasonNoCitation,
		"Spec: odd/tasks/f.md, T1\nSpec: odd/tasks/f.md, T2": oddtask.ReasonManyCitations,
		"Spec: odd/tasks/f.md":                               oddtask.ReasonBadCitation,
	}
	for prompt, want := range cases {
		docs := &fakeDocs{}
		if got := app.New(docs).Check(prompt, "/repo"); got != want {
			t.Errorf("Check(%q) = %q, want %q", prompt, got, want)
		}
		if docs.reads != 0 {
			t.Errorf("Check(%q) read %d documents, want none", prompt, docs.reads)
		}
	}
}

func TestCheckDeniesAnUnreadableDocumentWithItsCause(t *testing.T) {
	docs := &fakeDocs{err: &oddtask.ReadError{Path: "/repo/odd/tasks/f.md", Cause: oddtask.CauseNotFound}}
	want := oddtask.Reason("the feature document /repo/odd/tasks/f.md cannot be read (not found)")
	if got := app.New(docs).Check("Spec: odd/tasks/f.md, T2", "/repo"); got != want {
		t.Fatalf("Check = %q, want %q", got, want)
	}
}

// fakeFlows answers Of with view or err and counts the questions.
type fakeFlows struct {
	view  oddtask.WorkflowView
	err   error
	asked int
	cwd   string
}

func (f *fakeFlows) Of(cwd string) (oddtask.WorkflowView, error) {
	f.asked++
	f.cwd = cwd
	return f.view, f.err
}

var runningView = oddtask.WorkflowView{
	BindingClass: "owned", Workflow: "f-t2", WorkflowClass: "owned", Status: "running", GoalID: "f", ChainID: "f-t2", Records: 2, Role: "builder",
}

func TestCheckLaunchAllowsADesignedTaskInsideItsRunningWorkflow(t *testing.T) {
	docs := &fakeDocs{file: oddtask.File{Path: "/repo/odd/tasks/f.md", Text: shapedDoc}}
	flows := &fakeFlows{view: runningView}
	if got := app.New(docs).WithWorkflows(flows).CheckLaunch("Spec: odd/tasks/f.md, T2, S1", "/repo"); got != "" {
		t.Fatalf("CheckLaunch = %q, want allowed", got)
	}
	if flows.asked != 1 || flows.cwd != "/repo" {
		t.Fatalf("workflow asked %d times at %q, want once at the session cwd", flows.asked, flows.cwd)
	}
}

func TestCheckLaunchDeniesWithTheWorkflowMessageNamingTheFeatureAndTheFirstTask(t *testing.T) {
	docs := &fakeDocs{file: oddtask.File{Path: "/repo/odd/tasks/f.md", Text: shapedDoc}}
	view := runningView
	view.Status = "paused"
	got := app.New(docs).WithWorkflows(&fakeFlows{view: view}).CheckLaunch("Spec: /repo/odd/tasks/f.md, S1, T2", "/repo")
	want := oddtask.WorkflowMessage("the workflow f-t2 bound to the repository is paused, not running", "f", "T2")
	if got != want {
		t.Fatalf("CheckLaunch = %q, want %q", got, want)
	}
}

func TestCheckLaunchReadsNoWorkflowWhenTheDesignChecksFail(t *testing.T) {
	docs := &fakeDocs{file: oddtask.File{Path: "/repo/odd/tasks/f.md", Text: shapedDoc}}
	flows := &fakeFlows{view: runningView}
	want := oddtask.Message(oddtask.Reason(`task T4 of /repo/odd/tasks/f.md is not marked as designed by the shaper (its line has no "shaped: L<n>")`))
	if got := app.New(docs).WithWorkflows(flows).CheckLaunch("Spec: odd/tasks/f.md, T4", "/repo"); got != want {
		t.Fatalf("CheckLaunch = %q, want %q", got, want)
	}
	if got := app.New(docs).WithWorkflows(flows).CheckLaunch("build it", "/repo"); got != oddtask.Message(oddtask.ReasonNoCitation) {
		t.Fatalf("CheckLaunch(no citation) = %q", got)
	}
	if flows.asked != 0 {
		t.Fatalf("the workflow was read %d times, want none", flows.asked)
	}
}

func TestCheckLaunchDeniesWhenTheWorkflowCannotBeReadOrHasNoReader(t *testing.T) {
	docs := &fakeDocs{file: oddtask.File{Path: "/repo/odd/tasks/f.md", Text: shapedDoc}}
	got := app.New(docs).WithWorkflows(&fakeFlows{err: errors.New("disk")}).CheckLaunch("Spec: odd/tasks/f.md, T2", "/repo")
	if want := oddtask.WorkflowMessage(oddtask.WorkflowUnreadableReason("disk"), "f", "T2"); got != want {
		t.Fatalf("CheckLaunch = %q, want %q", got, want)
	}
	got = app.New(docs).CheckLaunch("Spec: odd/tasks/f.md, T2", "/repo")
	if want := oddtask.WorkflowMessage(oddtask.WorkflowUnreadableReason("the guard has no reader of the workflow"), "f", "T2"); got != want {
		t.Fatalf("CheckLaunch with no port = %q, want %q", got, want)
	}
}

func TestCheckDeniesAnErrorThatIsNotAReadError(t *testing.T) {
	docs := &fakeDocs{err: errors.New("boom")}
	want := oddtask.Reason("the feature document odd/tasks/f.md cannot be read (boom)")
	if got := app.New(docs).Check("Spec: odd/tasks/f.md, T2", "/repo"); got != want {
		t.Fatalf("Check = %q, want %q", got, want)
	}
}
