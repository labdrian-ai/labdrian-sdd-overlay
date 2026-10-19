package projection_test

// Tests for what is recorded of a memory read that ran (C23 M7): which calls are memory reads,
// exactly the ones the gate checks, and what is kept of each.

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// evidenceCall is a call naming query as its arguments, skill, file and id as the hook adapter
// reads them.
func evidenceCall(tool string, query projection.QueryArguments, skill, file, id string) projection.ToolCall {
	return projection.ToolCall{
		Name:         tool,
		ReadQuery:    reading(query),
		ReadSkill:    func() string { return skill },
		ReadFilePath: func() string { return file },
		ReadRecordID: func() string { return id },
	}
}

func evidenceOf(w *workflow.Loaded, call projection.ToolCall, lock projection.ProceduralSkills) (projection.ReadEvidence, bool) {
	return projection.Evidence(projection.GateInput{Binding: ownedBinding(), Workflow: w, EditTools: editTools, Call: call, Procedural: lock})
}

func TestEvidenceRecordsEveryMemoryReadTheGateChecks(t *testing.T) {
	q := projection.QueryArguments{Named: true, Project: "p", Goal: "g"}
	lock := projection.ProceduralSkills{Skills: []projection.ProceduralSkill{{ID: "deploy", SkillFiles: []string{"/repo/.claude/skills/deploy/SKILL.md"}}}}
	cases := []struct {
		call projection.ToolCall
		want projection.ReadEvidence
	}{
		{evidenceCall("mcp__plugin_engram_engram__mem_search", q, "", "", "9"),
			projection.ReadEvidence{Tool: "mcp__plugin_engram_engram__mem_search", Source: memoryscope.SourceEngram, Project: "p", Goal: "g"}},
		{evidenceCall("mcp__engram__mem_context", q, "", "", ""),
			projection.ReadEvidence{Tool: "mcp__engram__mem_context", Source: memoryscope.SourceEngram, Project: "p", Goal: "g"}},
		{evidenceCall("mcp__engram__mem_get_observation", q, "", "", "4216"),
			projection.ReadEvidence{Tool: "mcp__engram__mem_get_observation", Source: memoryscope.SourceEngram, IDs: []string{"4216"}}},
		{evidenceCall("mcp__engram__mem_timeline", q, "", "", "17"),
			projection.ReadEvidence{Tool: "mcp__engram__mem_timeline", Source: memoryscope.SourceEngram, IDs: []string{"17"}}},
		{evidenceCall(queryTool, q, "", "", ""),
			projection.ReadEvidence{Tool: queryTool, Source: memoryscope.SourceLongtermMem, Project: "p", Goal: "g"}},
		{evidenceCall("mcp__longterm-mem__get", q, "", "", "42"),
			projection.ReadEvidence{Tool: "mcp__longterm-mem__get", Source: memoryscope.SourceLongtermMem, IDs: []string{"42"}}},
		{evidenceCall("Skill", q, "deploy", "", ""),
			projection.ReadEvidence{Tool: "Skill", Source: memoryscope.SourceProceduralSkills, Skill: "deploy"}},
		{evidenceCall("Read", q, "", "/repo/.claude/skills/deploy/SKILL.md", ""),
			projection.ReadEvidence{Tool: "Read", Source: memoryscope.SourceProceduralSkills, Skill: "deploy"}},
	}
	for _, status := range openStatuses {
		for _, c := range cases {
			got, ok := evidenceOf(loadedWorkflow("sdd", status), c.call, lock)
			c.want.ProjectID, c.want.WorkflowID = "proj-1", "wf-1"
			if !ok || got.Tool != c.want.Tool || got.Source != c.want.Source || got.Project != c.want.Project ||
				got.Goal != c.want.Goal || got.Skill != c.want.Skill || !slices.Equal(got.IDs, c.want.IDs) ||
				got.ProjectID != c.want.ProjectID || got.WorkflowID != c.want.WorkflowID {
				t.Errorf("%s, %s: Evidence = %+v, %v; want %+v", status, c.call.Name, got, ok, c.want)
			}
		}
	}
}

func TestEvidenceRecordsNoOtherCall(t *testing.T) {
	q := named("p")
	lock := projection.ProceduralSkills{Skills: []projection.ProceduralSkill{{ID: "deploy"}}}
	for _, call := range []projection.ToolCall{
		evidenceCall("mcp__engram__mem_save", q, "", "", ""),
		evidenceCall("mcp__longterm-mem__promote", q, "", "", ""),
		evidenceCall("mcp__engramx__mem_search", q, "", "", ""),
		evidenceCall("Write", q, "", "", ""),
		evidenceCall("Bash", q, "", "", ""),
		evidenceCall("Skill", q, "not-procedural", "", ""),
		evidenceCall("Read", q, "", "/repo/README.md", ""),
		evidenceCall("Read", q, "", "/elsewhere/SKILL.md", ""),
	} {
		if got, ok := evidenceOf(loadedWorkflow("sdd", workflow.StatusRunning), call, lock); ok {
			t.Errorf("%s: recorded %+v", call.Name, got)
		}
	}
}

func TestEvidenceNeedsAnOpenWorkflowOfOurs(t *testing.T) {
	call := evidenceCall("mcp__engram__mem_search", named("p"), "", "", "")
	if _, ok := evidenceOf(loadedWorkflow("sdd", workflow.StatusClosed), call, projection.ProceduralSkills{}); ok {
		t.Error("a closed workflow recorded a read")
	}
	if _, ok := evidenceOf(nil, call, projection.ProceduralSkills{}); ok {
		t.Error("no workflow recorded a read")
	}
	foreign := projection.GateInput{Binding: projection.Loaded{Classification: projection.ClassificationForeign}, Workflow: loadedWorkflow("sdd", workflow.StatusRunning), Call: call}
	if _, ok := projection.Evidence(foreign); ok {
		t.Error("a foreign binding recorded a read")
	}
	unreadable := projection.ProceduralSkills{Unreadable: "bad lock"}
	if _, ok := evidenceOf(loadedWorkflow("sdd", workflow.StatusRunning), evidenceCall("Skill", named(""), "deploy", "", ""), unreadable); ok {
		t.Error("a skill use under an unreadable lock was recorded as procedural")
	}
}

func TestEvidenceKeepsEachValueOnOneBoundedLine(t *testing.T) {
	long := strings.Repeat("x", 1000)
	q := projection.QueryArguments{Named: true, Project: "a\nb", Goal: long}
	got, ok := evidenceOf(loadedWorkflow("sdd", workflow.StatusRunning), evidenceCall("mcp__engram__mem_search", q, "", "", ""), projection.ProceduralSkills{})
	if !ok {
		t.Fatal("not recorded")
	}
	// 200 runes, and the "..." clip ends a cut value with.
	if got.Project != "a b" || got.Goal != strings.Repeat("x", 200)+"..." {
		t.Errorf("project %q, goal of %d runes", got.Project, utf8.RuneCountInString(got.Goal))
	}
}

func TestEvidenceWarningSaysTheReadWasNotRecordedAndWhy(t *testing.T) {
	ev := projection.ReadEvidence{ProjectID: "proj-1", WorkflowID: "wf-1", Tool: "mcp__engram__mem_search", Source: memoryscope.SourceEngram}
	got := projection.EvidenceWarning(ev, errors.New("lock busy\nnow"))
	for _, want := range []string{"labdrian:", "mcp__engram__mem_search", "was not recorded", "workflow wf-1 of project proj-1", "(lock busy now)"} {
		if !strings.Contains(got, want) {
			t.Errorf("warning %q lacks %q", got, want)
		}
	}
	if strings.Contains(got, "\n") {
		t.Errorf("warning %q is more than one line", got)
	}
}
