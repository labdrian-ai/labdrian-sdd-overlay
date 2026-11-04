package app_test

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// TestOnToolCallChecksAMemorycardReadAgainstTheMemoryPlan: a memorycard mem_search in a repository
// bound to a running workflow reaches the gate (it is not answered as a tool the gate never
// checks), which lets the workflow's project through and denies another one.
func TestOnToolCallChecksAMemorycardReadAgainstTheMemoryPlan(t *testing.T) {
	w := newHookWorld(workflow.StatusRunning)
	running := owned(workflow.StatusRunning)
	running.State.Profile, running.State.ProjectIdentity = "odd", "github.com/acme/proj-1"
	w.workflows.put("proj-1", "wf-1", running)
	w.service.EngramName = func(identity string) string { return identity[strings.LastIndex(identity, "/")+1:] }
	search := func(project string) app.ToolCallRequest {
		return app.ToolCallRequest{InputDir: repoDir, Call: projection.ToolCall{Name: "mcp__memorycard__mem_search",
			ReadQuery: func() projection.QueryArguments { return projection.QueryArguments{Named: true, Project: project} }}}
	}

	if got := w.service.OnToolCall(search("proj-1")); !got.Decided || got.Result.Deny || got.Result.Warning != "" {
		t.Errorf("a read of the workflow's project: OnToolCall = %+v, want it decided and allowed", got)
	}
	got := w.service.OnToolCall(search("other"))
	if !got.Decided || !got.Result.Deny || !strings.Contains(got.Result.Reason, "this memorycard read asks for project \"other\"") ||
		!strings.Contains(got.Result.Reason, `permits only project "proj-1"`) {
		t.Errorf("a read of another project: OnToolCall = %+v, want a denial naming both projects", got)
	}
	if w.bindings.loads != 2 {
		t.Errorf("the binding store was read %d times for two memorycard reads, want 2", w.bindings.loads)
	}
}

// TestOnToolResultAppendsTheEvidenceOfAMemorycardRead: a memorycard read that ran is recorded like
// an Engram one, as a read of source engram with the project it named.
func TestOnToolResultAppendsTheEvidenceOfAMemorycardRead(t *testing.T) {
	w, log := evidenceWorld(workflow.StatusRunning)
	if got := w.service.OnToolResult(resultRequest("mcp__memorycard__mem_search")); got.Warning != "" {
		t.Fatalf("OnToolResult warned %q", got.Warning)
	}
	if len(log.records) != 1 {
		t.Fatalf("%d records, want 1", len(log.records))
	}
	ev := log.records[0].Evidence
	if ev.Tool != "mcp__memorycard__mem_search" || ev.Source != memoryscope.SourceEngram || ev.Project != "p" ||
		ev.ProjectID != "proj-1" || ev.WorkflowID != "wf-1" {
		t.Errorf("evidence = %+v, want the memorycard read of source engram and project p", ev)
	}
	if got := w.service.OnToolResult(resultRequest("mcp__memorycard__mem_save")); got.Warning != "" || len(log.records) != 1 {
		t.Errorf("a memorycard write: warning %q, %d records, want none added", got.Warning, len(log.records))
	}
}
