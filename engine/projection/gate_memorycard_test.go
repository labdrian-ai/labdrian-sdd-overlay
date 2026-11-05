package projection_test

// Tests for the memorycard reads under the memory gate. memorycard replaced Engram as the store of
// the episodic memory and holds its imported history, and it names a project as Engram does (the
// last segment of the origin remote). So its reads are of plan source engram: mem_search and
// mem_context are checked like the Engram reads that name a project, mem_get like a read by id, and
// its writes are never gated.

import (
	"fmt"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

var (
	// memorycardProjectReads name a project; memorycardIDReads look a memory up by its id.
	memorycardProjectReads = []string{"mcp__memorycard__mem_search", "mcp__memorycard__mem_context", "mcp__plugin_x_memorycard__mem_search"}
	memorycardIDReads      = []string{"mcp__memorycard__mem_get", "mcp__plugin_x_memorycard__mem_get"}
	memorycardWrites       = []string{"mcp__memorycard__mem_save", "mcp__memorycard__mem_update", "mcp__memorycard__mem_session"}
)

// TestGateRecognizesTheMemorycardReadsByTheirNamesAndNothingLooser: the three memorycard reads are
// recognized as strictly as the Engram ones (a plugin prefix is allowed, a lookalike server or tool
// name is not), its writes never are, and under scope none every recognized read is denied.
func TestGateRecognizesTheMemorycardReadsByTheirNamesAndNothingLooser(t *testing.T) {
	for _, tool := range []struct {
		name string
		read bool
	}{
		{"mcp__memorycard__mem_search", true},
		{"mcp__memorycard__mem_context", true},
		{"mcp__memorycard__mem_get", true},
		{"mcp__plugin_x_memorycard__mem_search", true},
		{"mcp__my-plugin_memorycard__mem_get", true},
		{"mcp__memorycard__mem_save", false},
		{"mcp__memorycard__mem_update", false},
		{"mcp__memorycard__mem_session", false},
		{"mcp__memorycard__mem_get_observation", false},
		{"mcp__memorycard__mem_timeline", false},
		{"mcp__memorycard__mem_getx", false},
		{"mcp__memorycardx__mem_search", false},
		{"mcp__xmemorycard__mem_search", false},
		{"mcp__a__memorycard__mem_search", false},
		{"memorycard__mem_search", false},
		{"mcp__memorycard__mem_search\n", false},
	} {
		t.Run(fmt.Sprintf("%q", tool.name), func(t *testing.T) {
			if got := projection.GateRelevant(editTools, tool.name); got != tool.read {
				t.Errorf("GateRelevant = %v, want %v", got, tool.read)
			}
			if got := projection.GateRelevantCall(editTools, projection.ToolCall{Name: tool.name}); got != tool.read {
				t.Errorf("GateRelevantCall = %v, want %v", got, tool.read)
			}
			got := gate(loadedWorkflow("standalone-minimal", workflow.StatusRunning), tool.name, named("proj-1"))
			if tool.read {
				denied(t, tool.name, got, "standalone-minimal", "permits no project", "every memorycard read is denied", "labdrian workflow unbind")
			} else {
				allowed(t, tool.name, got)
			}
		})
	}
}

// TestTheMemorycardReadsAreOfSourceEngram: a plan that holds engram lets a memorycard read of the
// right project through, and a plan narrowed to longterm-mem denies every memorycard read, by id
// too, naming source engram.
func TestTheMemorycardReadsAreOfSourceEngram(t *testing.T) {
	sdd := engramWorkflow("sdd")
	for _, tool := range memorycardProjectReads {
		allowed(t, "sdd/"+tool, gate(sdd, tool, engramRead(engramNameOfProject, false)))
	}
	for _, tool := range memorycardIDReads {
		allowed(t, "sdd/"+tool, gate(sdd, tool, unnamed))
	}

	onlyLongterm := narrowedWorkflow(narrowerDirective(memoryscope.ScopeProject, memoryscope.SourceLongtermMem), nil, nil)
	onlyLongterm.State.ProjectIdentity = engramIdentity
	for _, tool := range append(append([]string{}, memorycardProjectReads...), memorycardIDReads...) {
		denied(t, "narrowed/"+tool, gate(onlyLongterm, tool, engramRead(engramNameOfProject, false)),
			`does not include source "engram"`, `profile "odd"`, "memorycard read", "labdrian workflow unbind")
	}
}

// TestGateComparesTheProjectOfAMemorycardReadWithTheEngramNameOfTheIdentity: mem_search and
// mem_context must name the project the workflow's identity is known by; another one or none is
// denied, and a reason names the read as a memorycard read.
func TestGateComparesTheProjectOfAMemorycardReadWithTheEngramNameOfTheIdentity(t *testing.T) {
	w := engramWorkflow("odd")
	for _, tool := range memorycardProjectReads {
		t.Run(tool, func(t *testing.T) {
			allowed(t, "the project name", gate(w, tool, engramRead(engramNameOfProject, false)))
			denied(t, "the identity itself", gate(w, tool, engramRead(engramIdentity, false)),
				"this memorycard read asks for project", `"github.com/acme/proj-1"`, `permits only project "proj-1"`, `Name project "proj-1"`, "labdrian workflow unbind")
			denied(t, "another project", gate(w, tool, engramRead("other", false)), `"other"`, `"proj-1"`, "memorycard read")
			warned(t, "arguments that are not named", gate(w, tool, unnamed), "memorycard read", "not a JSON object", "not checked")
		})
	}
	for _, tool := range engramProjectReads {
		denied(t, "Engram wording kept/"+tool, gate(w, tool, engramRead("other", false)), "this Engram read asks for project")
	}
}

// TestGateLetsAMemorycardReadWithNoProjectResolveItsOwn: memorycard resolves an omitted project
// from its working directory, the session's own project, so a mem_search or mem_context that names
// no project and no directory passes; one that names no project but a directory (cwd or directory,
// which mem_context reads only when project is empty) is denied, since that directory picks the
// project. An Engram read that names no project is still denied, and every project still is.
func TestGateLetsAMemorycardReadWithNoProjectResolveItsOwn(t *testing.T) {
	for _, profile := range []string{"odd", "sdd"} {
		w := engramWorkflow(profile)
		for _, tool := range memorycardProjectReads {
			allowed(t, profile+"/no project, no directory/"+tool, gate(w, tool, engramRead("", false)))
			denied(t, profile+"/no project, a directory/"+tool,
				gate(w, tool, projection.QueryArguments{Named: true, WorkDir: "/elsewhere"}),
				"this memorycard read names no project", "a directory", `"/elsewhere"`, `"proj-1"`, "labdrian workflow unbind")
			denied(t, profile+"/every project/"+tool, gate(w, tool, engramRead("", true)), "all_projects is true")
		}
		for _, tool := range engramProjectReads {
			denied(t, profile+"/Engram no project/"+tool, gate(w, tool, engramRead("", false)),
				"this Engram read names no project", "missing, not a string, or empty")
		}
	}
}

// TestGateLetsAMemorycardGetThroughWithoutReadingIt: mem_get names no project; it passes whatever
// it carries once the plan includes engram, and its arguments are never read.
func TestGateLetsAMemorycardGetThroughWithoutReadingIt(t *testing.T) {
	w := engramWorkflow("odd")
	for _, tool := range memorycardIDReads {
		reads := 0
		got := projection.Gate(projection.GateInput{
			Binding: ownedBinding(), Workflow: w, EditTools: editTools, EngramName: lastSegment,
			Call: projection.ToolCall{Name: tool, ReadQuery: func() projection.QueryArguments { reads++; return engramRead("other", true) }},
		})
		allowed(t, tool, got)
		if reads != 0 {
			t.Errorf("%s: the arguments were read %d times, want 0", tool, reads)
		}
	}
}

// TestGateNeverTouchesAMemorycardWrite: the plan grants no write and forbids none.
func TestGateNeverTouchesAMemorycardWrite(t *testing.T) {
	for _, profile := range []string{"standalone-minimal", "sdd", "odd"} {
		w := loadedWorkflow(profile, workflow.StatusRunning)
		for _, tool := range memorycardWrites {
			if projection.GateRelevant(editTools, tool) {
				t.Errorf("GateRelevant(%q) = true, want false: a write is never gated", tool)
			}
			allowed(t, profile+"/"+tool, gate(w, tool, engramRead("other", true)))
		}
	}
}
