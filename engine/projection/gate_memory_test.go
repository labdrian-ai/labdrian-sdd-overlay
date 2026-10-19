package projection_test

// Tests for the memory gate of C23 M4 (docs/architecture/adr-c23-m4-memory-gate.md): the plan's
// sources are enforced for every gated read (decision 1), writes are never gated (decision 2), an
// Engram read that names a project must name the workflow's (decision 3), and an Engram read by id
// passes when the plan includes engram (decision 4).

import (
	"fmt"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

const (
	getTool             = "mcp__longterm-mem__get"
	pluginGetTool       = "mcp__plugin_x_longterm-mem__get"
	searchTool          = "mcp__plugin_engram_engram__mem_search"
	bareSearchTool      = "mcp__engram__mem_search"
	contextTool         = "mcp__plugin_engram_engram__mem_context"
	observationTool     = "mcp__plugin_engram_engram__mem_get_observation"
	timelineTool        = "mcp__plugin_engram_engram__mem_timeline"
	engramIdentity      = "github.com/acme/proj-1"
	engramNameOfProject = "proj-1"
)

var (
	// engramProjectReads name a project; engramIDReads look an observation up by its id.
	engramProjectReads = []string{searchTool, bareSearchTool, contextTool, "mcp__engram__mem_context"}
	engramIDReads      = []string{observationTool, timelineTool, "mcp__engram__mem_get_observation", "mcp__engram__mem_timeline"}
	longtermReads      = []string{queryTool, pluginQueryTool, getTool, pluginGetTool}
	// memoryWrites are never gated, whatever the plan says.
	memoryWrites = []string{
		"mcp__plugin_engram_engram__mem_save", "mcp__plugin_engram_engram__mem_update", "mcp__plugin_engram_engram__mem_session_summary",
		"mcp__plugin_engram_engram__mem_save_prompt", "mcp__plugin_engram_engram__mem_judge", "mcp__engram__mem_save",
		"mcp__longterm-mem__promote", "mcp__plugin_x_longterm-mem__promote",
	}
)

// engramWorkflow is a running workflow of profile whose recorded identity is engramIdentity, so
// Engram names its project engramNameOfProject.
func engramWorkflow(profile string) *workflow.Loaded {
	w := loadedWorkflow(profile, workflow.StatusRunning)
	w.State.ProjectIdentity = engramIdentity
	return w
}

// engramRead is a read that names project, and asks for every project when all is true.
func engramRead(project string, all bool) projection.QueryArguments {
	return projection.QueryArguments{Named: true, Project: project, AllProjects: all}
}

func warned(t *testing.T, label string, got projection.GateResult, wantIn ...string) {
	t.Helper()
	if got.Deny || got.Reason != "" || got.Warning == "" || strings.ContainsAny(got.Warning, "\n\r") || !strings.HasPrefix(got.Warning, "labdrian:") {
		t.Fatalf("%s: Gate() = %+v, want an allow with a one-line warning", label, got)
	}
	for _, want := range wantIn {
		if !strings.Contains(got.Warning, want) {
			t.Errorf("%s: warning %q does not contain %q", label, got.Warning, want)
		}
	}
}

// --- which tools are memory reads ----------------------------------------------

// TestGateRecognizesTheMemoryReadsByTheirNamesAndNothingLooser: the Engram reads and longterm-mem
// get are recognized as strictly as the query (a plugin prefix is allowed, a lookalike server name
// is not), and a write never is. Under scope none every recognized read is denied.
func TestGateRecognizesTheMemoryReadsByTheirNamesAndNothingLooser(t *testing.T) {
	for _, tool := range []struct {
		name string
		read bool
	}{
		{"mcp__engram__mem_search", true},
		{"mcp__engram__mem_context", true},
		{"mcp__engram__mem_get_observation", true},
		{"mcp__engram__mem_timeline", true},
		{"mcp__plugin_engram_engram__mem_search", true},
		{"mcp__plugin_engram_engram__mem_context", true},
		{"mcp__plugin_engram_engram__mem_get_observation", true},
		{"mcp__plugin_engram_engram__mem_timeline", true},
		{"mcp__my-plugin_engram__mem_search", true},
		{"mcp__longterm-mem__get", true},
		{"mcp__plugin_x_longterm-mem__get", true},
		{"mcp__a-b_c_longterm-mem__get", true},
		{"mcp__engramx__mem_search", false},
		{"mcp__xengram__mem_search", false},
		{"mcp__plugin_xengram__mem_search", false},
		{"mcp__plugin_engram_engramx__mem_search", false},
		{"mcp__a__engram__mem_search", false},
		{"mcp__engram__mem_searchx", false},
		{"mcp__engram__mem_search_all", false},
		{"mcp__engram__mem_save", false},
		{"mcp__engram__mem_update", false},
		{"mcp__engram__mem_session_summary", false},
		{"mcp__engram__mem_current_project", false},
		{"mcp__engram__Mem_search", false},
		{"engram__mem_search", false},
		{"mcp_engram__mem_search", false},
		{"mcp__engram__mem_search ", false},
		{"mcp__engram__mem_search\n", false},
		{"mcp__longterm-memx__get", false},
		{"mcp__xlongterm-mem__get", false},
		{"mcp__longterm-mem__getx", false},
		{"mcp__longterm-mem__Get", false},
		{"mcp__a__longterm-mem__get", false},
		{"mcp__longterm-mem__promote", false},
		{"mcp__plugin_x_longterm-mem__promote", false},
	} {
		t.Run(fmt.Sprintf("%q", tool.name), func(t *testing.T) {
			if got := projection.GateRelevant(editTools, tool.name); got != tool.read {
				t.Errorf("GateRelevant = %v, want %v", got, tool.read)
			}
			got := gate(loadedWorkflow("standalone-minimal", workflow.StatusRunning), tool.name, named("proj-1"))
			if tool.read {
				denied(t, tool.name, got, "standalone-minimal", "permits no project", "labdrian workflow unbind")
			} else {
				allowed(t, tool.name, got)
			}
		})
	}
}

// --- decision 1: the sources are enforced -------------------------------------

// TestGateDeniesAReadOfASourceThePlanExcludes: sdd's plan holds engram alone, so every longterm-mem
// read is denied, naming the source and the profile and advising unbind, while the Engram reads of
// the right project pass. A plan narrowed to longterm-mem denies every Engram read, by id too.
func TestGateDeniesAReadOfASourceThePlanExcludes(t *testing.T) {
	sdd := engramWorkflow("sdd")
	for _, tool := range longtermReads {
		denied(t, "sdd/"+tool, gate(sdd, tool, named(engramIdentity)),
			`does not include source "longterm-mem"`, `profile "sdd"`, "workflow wf-1 of project proj-1", "labdrian workflow unbind")
	}
	for _, tool := range engramProjectReads {
		allowed(t, "sdd/"+tool, gate(sdd, tool, engramRead(engramNameOfProject, false)))
	}
	for _, tool := range engramIDReads {
		allowed(t, "sdd/"+tool, gate(sdd, tool, unnamed))
	}

	// incident-recovery is scope goal with engram and procedural-skills: no longterm-mem either.
	incident := engramWorkflow("incident-recovery")
	denied(t, "incident-recovery/query", gate(incident, queryTool, named(engramIdentity)), `"longterm-mem"`, `profile "incident-recovery"`)
	denied(t, "incident-recovery/get", gate(incident, getTool, unnamed), `"longterm-mem"`)

	onlyLongterm := narrowedWorkflow(narrowerDirective(memoryscope.ScopeProject, memoryscope.SourceLongtermMem), nil, nil)
	onlyLongterm.State.ProjectIdentity = engramIdentity
	for _, tool := range append(append([]string{}, engramProjectReads...), engramIDReads...) {
		denied(t, "narrowed/"+tool, gate(onlyLongterm, tool, engramRead(engramNameOfProject, false)),
			`does not include source "engram"`, `profile "odd"`, "Engram read", "labdrian workflow unbind")
	}
	allowed(t, "narrowed/query", gate(onlyLongterm, queryTool, named(engramIdentity)))
	allowed(t, "narrowed/get", gate(onlyLongterm, getTool, unnamed))
}

// TestGateDeniesEveryMemoryReadWhenThePlanHasNoProject: scope none has no sources, so each gated
// read is denied with the scope none wording the query always had.
func TestGateDeniesEveryMemoryReadWhenThePlanHasNoProject(t *testing.T) {
	w := loadedWorkflow("standalone-minimal", workflow.StatusRunning)
	for _, tool := range append(append(append([]string{}, longtermReads...), engramProjectReads...), engramIDReads...) {
		denied(t, tool, gate(w, tool, engramRead("proj-1", false)), "standalone-minimal", "permits no project", "(scope none)", "labdrian workflow unbind")
	}
	denied(t, "Engram wording", gate(w, searchTool, unnamed), "every Engram read is denied")
	denied(t, "get wording", gate(w, getTool, unnamed), "every longterm-mem get is denied")
	denied(t, "query wording", gate(w, queryTool, unnamed), "every longterm-mem query is denied", "to query.")
}

// TestGateLetsAReadByIDThroughWhenThePlanIncludesItsSource: longterm-mem get, mem_get_observation
// and mem_timeline name no project; they pass whatever they carry and their arguments are never
// read (decision 4).
func TestGateLetsAReadByIDThroughWhenThePlanIncludesItsSource(t *testing.T) {
	w := engramWorkflow("odd")
	for _, tool := range append([]string{getTool, pluginGetTool}, engramIDReads...) {
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

// --- decision 2: writes are never gated -----------------------------------------

func TestGateNeverTouchesAMemoryWrite(t *testing.T) {
	for _, profile := range []string{"standalone-minimal", "sdd", "odd", "incident-recovery"} {
		for _, status := range openStatuses {
			w := loadedWorkflow(profile, status)
			for _, tool := range memoryWrites {
				if projection.GateRelevant(editTools, tool) {
					t.Errorf("GateRelevant(%q) = true, want false: a write is never gated", tool)
				}
				allowed(t, profile+"/"+string(status)+"/"+tool, gate(w, tool, engramRead("other", true)))
			}
		}
	}
}

// --- decision 3: the Engram project check ---------------------------------------

// TestGateComparesTheProjectOfAnEngramReadWithTheEngramNameOfTheIdentity: mem_search and
// mem_context must name the project Engram knows the workflow's identity by; another one, none, or
// every project is denied.
func TestGateComparesTheProjectOfAnEngramReadWithTheEngramNameOfTheIdentity(t *testing.T) {
	w := engramWorkflow("odd")
	for _, tool := range engramProjectReads {
		t.Run(tool, func(t *testing.T) {
			allowed(t, "the Engram name", gate(w, tool, engramRead(engramNameOfProject, false)))
			denied(t, "the identity itself", gate(w, tool, engramRead(engramIdentity, false)),
				`asks for project "github.com/acme/proj-1"`, `permits only project "proj-1"`, `Name project "proj-1"`, "labdrian workflow unbind")
			denied(t, "another project", gate(w, tool, engramRead("other", false)), `"other"`, `"proj-1"`, "Engram read")
			denied(t, "a case difference", gate(w, tool, engramRead("PROJ-1", false)), `"PROJ-1"`)
			denied(t, "no project", gate(w, tool, engramRead("", false)),
				"names no project", "missing, not a string, or empty", `"proj-1"`, "labdrian workflow unbind")
			denied(t, "every project", gate(w, tool, engramRead(engramNameOfProject, true)),
				"every project", "all_projects", `"proj-1"`, "labdrian workflow unbind")
			denied(t, "every project and none named", gate(w, tool, engramRead("", true)), "all_projects")
			warned(t, "arguments that are not named", gate(w, tool, unnamed), "Engram read", "not a JSON object", "not checked")
		})
	}
}

// TestTheEngramProjectCheckUsesTheCallersNamingRule: the gate has no rule of its own for how
// Engram names a project; it asks the one the caller gives, and with none it checks nothing and
// says so.
func TestTheEngramProjectCheckUsesTheCallersNamingRule(t *testing.T) {
	w := engramWorkflow("odd")
	with := func(name func(string) string, args projection.QueryArguments) projection.GateResult {
		return projection.Gate(projection.GateInput{Binding: ownedBinding(), Workflow: w, EditTools: editTools, EngramName: name,
			Call: projection.ToolCall{Name: searchTool, ReadQuery: reading(args)}})
	}
	marked := func(identity string) string { return "engram:" + identity }
	allowed(t, "the name the rule gives", with(marked, engramRead("engram:"+engramIdentity, false)))
	denied(t, "another name", with(marked, engramRead(engramNameOfProject, false)), `"engram:github.com/acme/proj-1"`)
	warned(t, "no rule", with(nil, engramRead("other", false)), "Engram read", "not checked")
	// The longterm-mem query does not depend on it.
	denied(t, "a query with no rule", projection.Gate(projection.GateInput{Binding: ownedBinding(), Workflow: w, EditTools: editTools,
		Call: projection.ToolCall{Name: queryTool, ReadQuery: reading(named("other"))}}), `"other"`)
}

// TestGateAllowsTheEngramReadOfAWorkflowThatRecordedNoIdentity: as for longterm-mem, a workflow
// created before M2 does not say which project Engram knows it by.
func TestGateAllowsTheEngramReadOfAWorkflowThatRecordedNoIdentity(t *testing.T) {
	w := loadedWorkflow("odd", workflow.StatusRunning)
	w.State.ProjectIdentity = ""
	for _, tool := range engramProjectReads {
		for _, args := range []projection.QueryArguments{engramRead("other", false), engramRead("", true), unnamed} {
			warned(t, tool, gate(w, tool, args), "records no project identity", "Engram read", "not checked")
		}
	}
	for _, tool := range engramIDReads {
		allowed(t, tool, gate(w, tool, unnamed))
	}
}

// TestGateWarnsOnceForAnEngramReadWhenThePlanCannotBeComputed: an unknown is an allow.
func TestGateWarnsOnceForAnEngramReadWhenThePlanCannotBeComputed(t *testing.T) {
	w := loadedWorkflow("no-such-profile", workflow.StatusRunning)
	for _, tool := range append(append([]string{getTool}, engramProjectReads...), engramIDReads...) {
		warned(t, tool, gate(w, tool, engramRead("other", true)), "no-such-profile", "not checked")
	}
}

// TestTheMemoryReadsAreGatedInEveryOpenStatusAndOnlyThen: like the query, while paused too.
func TestTheMemoryReadsAreGatedInEveryOpenStatusAndOnlyThen(t *testing.T) {
	for _, status := range openStatuses {
		w := loadedWorkflow("sdd", status)
		denied(t, string(status)+"/get", gate(w, getTool, unnamed), `"longterm-mem"`)
		denied(t, string(status)+"/search", gate(w, searchTool, engramRead("other", false)), `"other"`)
	}
	closed := loadedWorkflow("sdd", workflow.StatusClosed)
	allowed(t, "closed/get", gate(closed, getTool, unnamed))
	allowed(t, "closed/search", gate(closed, searchTool, engramRead("other", false)))
}
