package app_test

// Tests for HookService.OnToolResult (C23 M7): the evidence of a memory read that ran is appended
// to its workflow's log, and a write that fails is one warning line.

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

type fakeEvidence struct {
	records []projection.ReadRecord
	err     error
}

func (f *fakeEvidence) Append(r projection.ReadRecord) error {
	if f.err != nil {
		return f.err
	}
	f.records = append(f.records, r)
	return nil
}

type fixedClock struct{ at time.Time }

func (c fixedClock) Now() time.Time { return c.at }

func evidenceWorld(status workflow.Status) (*hookWorld, *fakeEvidence) {
	w := newHookWorld(status)
	log := &fakeEvidence{}
	w.service.Evidence = log
	w.service.Clock = fixedClock{at: firstInstant}
	return w, log
}

func resultRequest(tool string) app.ToolResultRequest {
	return app.ToolResultRequest{
		InputDir: repoDir, CallID: "toolu_1", SessionID: "s-1", ResponseSHA256: "abc", ResponseBytes: 3,
		Call: projection.ToolCall{Name: tool, ReadQuery: func() projection.QueryArguments {
			return projection.QueryArguments{Named: true, Project: "p"}
		}},
	}
}

func TestOnToolResultAppendsTheEvidenceOfAMemoryRead(t *testing.T) {
	w, log := evidenceWorld(workflow.StatusRunning)
	if got := w.service.OnToolResult(resultRequest("mcp__engram__mem_search")); got.Warning != "" {
		t.Fatalf("OnToolResult warned %q", got.Warning)
	}
	if len(log.records) != 1 {
		t.Fatalf("%d records, want 1", len(log.records))
	}
	r := log.records[0]
	want := projection.ReadRecord{
		Evidence: projection.ReadEvidence{ProjectID: "proj-1", WorkflowID: "wf-1", Tool: "mcp__engram__mem_search", Source: memoryscope.SourceEngram, Project: "p"},
		At:       firstInstant, CallID: "toolu_1", SessionID: "s-1", ResponseSHA256: "abc", ResponseBytes: 3,
	}
	if r.Evidence.ProjectID != want.Evidence.ProjectID || r.Evidence.WorkflowID != want.Evidence.WorkflowID || r.Evidence.Tool != want.Evidence.Tool ||
		r.Evidence.Source != want.Evidence.Source || r.Evidence.Project != want.Evidence.Project || !r.At.Equal(want.At) ||
		r.CallID != want.CallID || r.SessionID != want.SessionID || r.ResponseSHA256 != want.ResponseSHA256 || r.ResponseBytes != want.ResponseBytes {
		t.Errorf("record = %+v, want %+v", r, want)
	}
}

func TestOnToolResultKeepsTheCallAndSessionIDsOnOneBoundedLine(t *testing.T) {
	w, log := evidenceWorld(workflow.StatusRunning)
	req := resultRequest("mcp__engram__mem_search")
	req.SessionID, req.CallID = "sess\nX"+strings.Repeat("s", 1000), "tu\x1b[31m"+strings.Repeat("c", 1000)
	w.service.OnToolResult(req)
	if len(log.records) != 1 {
		t.Fatalf("%d records, want 1", len(log.records))
	}
	r := log.records[0]
	if !strings.HasPrefix(r.SessionID, "sess X") || strings.ContainsAny(r.SessionID+r.CallID, "\n\x1b") ||
		!strings.HasSuffix(r.SessionID, "...") || !strings.HasSuffix(r.CallID, "...") || len(r.SessionID) > 210 || len(r.CallID) > 210 {
		t.Errorf("session %q (%d bytes), call %q (%d bytes)", r.SessionID[:10], len(r.SessionID), r.CallID[:10], len(r.CallID))
	}
}

func TestOnToolResultRecordsNothingForAnotherToolWithoutTouchingAnyStore(t *testing.T) {
	w, log := evidenceWorld(workflow.StatusRunning)
	for _, tool := range []string{"Edit", "Bash", "mcp__engram__mem_save", "mcp__longterm-mem__promote"} {
		if got := w.service.OnToolResult(resultRequest(tool)); got.Warning != "" {
			t.Errorf("%s: warned %q", tool, got.Warning)
		}
	}
	if len(log.records) != 0 || w.bindings.loads != 0 {
		t.Errorf("%d records, %d binding reads; want none", len(log.records), w.bindings.loads)
	}
}

func TestOnToolResultIsSilentWithoutAnOpenWorkflowOfOurs(t *testing.T) {
	t.Run("no repository", func(t *testing.T) {
		w, log := evidenceWorld(workflow.StatusRunning)
		req := resultRequest("mcp__engram__mem_search")
		req.InputDir = "/elsewhere"
		if got := w.service.OnToolResult(req); got.Warning != "" || len(log.records) != 0 {
			t.Errorf("OnToolResult = %+v, %d records", got, len(log.records))
		}
	})
	t.Run("a binding store that cannot be read", func(t *testing.T) {
		w, log := evidenceWorld(workflow.StatusRunning)
		w.bindings.loadErr = errStoreDown
		if got := w.service.OnToolResult(resultRequest("mcp__engram__mem_search")); got.Warning != "" || len(log.records) != 0 {
			t.Errorf("OnToolResult = %+v, %d records", got, len(log.records))
		}
	})
	t.Run("a closed workflow", func(t *testing.T) {
		w, log := evidenceWorld(workflow.StatusClosed)
		if got := w.service.OnToolResult(resultRequest("mcp__engram__mem_search")); got.Warning != "" || len(log.records) != 0 {
			t.Errorf("OnToolResult = %+v, %d records", got, len(log.records))
		}
	})
}

func TestOnToolResultWarnsOnceWhenTheEvidenceCannotBeWritten(t *testing.T) {
	w, log := evidenceWorld(workflow.StatusRunning)
	log.err = errors.New("lock busy")
	got := w.service.OnToolResult(resultRequest("mcp__engram__mem_search"))
	if !strings.Contains(got.Warning, "was not recorded") || !strings.Contains(got.Warning, "(lock busy)") {
		t.Errorf("warning = %q", got.Warning)
	}
	w.service.Evidence = nil
	if got := w.service.OnToolResult(resultRequest("mcp__engram__mem_search")); !strings.Contains(got.Warning, "was not recorded") {
		t.Errorf("with no log: warning = %q", got.Warning)
	}
	w.service.Evidence, w.service.Clock = &fakeEvidence{}, nil
	if got := w.service.OnToolResult(resultRequest("mcp__engram__mem_search")); !strings.Contains(got.Warning, "was not recorded") {
		t.Errorf("with no clock: warning = %q", got.Warning)
	}
}

func TestOnToolResultRecordsAProceduralSkillThroughTheLock(t *testing.T) {
	w, lock := sddWorld()
	log := &fakeEvidence{}
	w.service.Evidence, w.service.Clock = log, fixedClock{at: firstInstant}
	req := app.ToolResultRequest{InputDir: repoDir, Call: projection.ToolCall{Name: "Skill", ReadSkill: func() string { return "deploy-checks" }}}
	if got := w.service.OnToolResult(req); got.Warning != "" {
		t.Fatalf("warned %q", got.Warning)
	}
	if len(log.records) != 1 || log.records[0].Evidence.Skill != "deploy-checks" || len(lock.asked) != 1 {
		t.Errorf("records %+v, lock asked %v", log.records, lock.asked)
	}
}
