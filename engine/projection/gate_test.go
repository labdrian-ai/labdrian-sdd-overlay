package projection_test

// Tests for the PreToolUse gate: the pure decision (Gate) and which tools it has an opinion
// about (GateRelevant). Everything here is a function of its arguments; nothing reads a file or
// starts a process, and nothing here knows a JSON document: what the gate is told of a call is
// its name and what a memory query names as its project (QueryArguments), as the hook adapter
// (engine/hookwire, with the tests in engine/cmd) reads it from the hook input.

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// --- fixtures ---------------------------------------------------------------

var (
	// editTools is the list a caller gives the gate: the gate has none of its own.
	editTools    = []string{"Write", "Edit", "MultiEdit", "NotebookEdit"}
	openStatuses = []workflow.Status{workflow.StatusCreated, workflow.StatusRunning, workflow.StatusPaused}
)

const (
	queryTool       = "mcp__longterm-mem__query"
	pluginQueryTool = "mcp__plugin_x_longterm-mem__query"
)

// named is what a memory query names as its project: the arguments are named, and project is
// the value of the "project" argument ("" when it names none: missing, null, not text, empty).
func named(project string) projection.QueryArguments {
	return projection.QueryArguments{Named: true, Project: project}
}

// unnamed is what a call whose arguments could not be read as named arguments carries.
var unnamed = projection.QueryArguments{}

// gate runs the gate for an owned binding to wf-1 of proj-1 and the workflow w, with the edit
// tools of editTools.
func gate(w *workflow.Loaded, tool string, query projection.QueryArguments) projection.GateResult {
	return projection.Gate(projection.GateInput{
		Binding:   ownedBinding(),
		Workflow:  w,
		EditTools: editTools,
		Call:      projection.ToolCall{Name: tool, Query: query},
	})
}

func allowed(t *testing.T, label string, got projection.GateResult) {
	t.Helper()
	if got.Deny || got.Reason != "" || got.Warning != "" {
		t.Errorf("%s: Gate() = %+v, want a plain allow (no denial, no reason, no warning)", label, got)
	}
}

func denied(t *testing.T, label string, got projection.GateResult, wantIn ...string) {
	t.Helper()
	if !got.Deny || got.Warning != "" {
		t.Fatalf("%s: Gate() = %+v, want a denial with no warning", label, got)
	}
	if strings.ContainsAny(got.Reason, "\n\r\x1b") || !utf8.ValidString(got.Reason) {
		t.Errorf("%s: reason %q is not one clean line", label, got.Reason)
	}
	for _, want := range wantIn {
		if !strings.Contains(got.Reason, want) {
			t.Errorf("%s: reason %q does not contain %q", label, got.Reason, want)
		}
	}
}

// --- which tools the gate has an opinion about --------------------------------

// TestGateRelevantSelectsOnlyTheToolsTheGateChecks: the gate needs the binding and the workflow
// only for a file-edit tool (of the list the caller gives) or a longterm-mem query; every other
// tool is decided from its name alone, so the hook never touches the stores for it.
func TestGateRelevantSelectsOnlyTheToolsTheGateChecks(t *testing.T) {
	for _, tool := range []string{"Write", "Edit", "MultiEdit", "NotebookEdit", "mcp__longterm-mem__query", "mcp__plugin_x_longterm-mem__query"} {
		if !projection.GateRelevant(editTools, tool) {
			t.Errorf("GateRelevant(%q) = false, want true", tool)
		}
	}
	for _, tool := range []string{
		"", "Bash", "Read", "Grep", "Glob", "Task", "WebFetch", "TodoWrite", "write", "Writer",
		"mcp__longterm-mem__get", "mcp__longterm-mem__promote", "mcp__longterm-memx__query", "mcp__engram__mem_save",
	} {
		if projection.GateRelevant(editTools, tool) {
			t.Errorf("GateRelevant(%q) = true, want false", tool)
		}
	}
}

// TestTheEditToolsAreTheCallers: the gate has no list of file-edit tools of its own. Given
// another list it denies those tools, and the ones it was not given are left alone, so a list
// that is empty (a caller that forgot it) gates no edit.
func TestTheEditToolsAreTheCallers(t *testing.T) {
	paused := loadedWorkflow("odd", workflow.StatusPaused)
	in := func(tools []string, tool string) projection.GateResult {
		return projection.Gate(projection.GateInput{Binding: ownedBinding(), Workflow: paused, EditTools: tools, Call: projection.ToolCall{Name: tool}})
	}
	denied(t, "a tool of the list", in([]string{"Patch"}, "Patch"), "Patch", "paused")
	allowed(t, "a tool the list does not name", in([]string{"Patch"}, "Edit"))
	allowed(t, "no list", in(nil, "Edit"))
	if projection.GateRelevant(nil, "Edit") || !projection.GateRelevant([]string{"Patch"}, "Patch") || projection.GateRelevant([]string{"Patch"}, "Edit") {
		t.Error("GateRelevant does not follow the list it is given")
	}
	// A tool is matched whole and as written.
	for _, tool := range []string{"patch", "Patch ", " Patch", "Patchwork", ""} {
		allowed(t, fmt.Sprintf("%q", tool), in([]string{"Patch"}, tool))
	}
	// The memory gate does not depend on the list.
	denied(t, "a memory query with no list", projection.Gate(projection.GateInput{
		Binding: ownedBinding(), Workflow: loadedWorkflow("odd", workflow.StatusRunning), Call: projection.ToolCall{Name: queryTool, Query: named("other")},
	}), `"other"`)
}

// --- rule 1: unbound repositories and bindings that are not ours ------------

func TestGateAllowsWhenTheRepositoryIsNotBoundToAnOwnedBinding(t *testing.T) {
	// Even a paused workflow and a query to the wrong project change nothing
	// when there is no owned binding: the gate is not this repository's business,
	// and for a binding it cannot use the prompt hook already warns, so the gate
	// stays silent instead of warning on every tool call.
	w := loadedWorkflow("odd", workflow.StatusPaused)
	for _, class := range []projection.Classification{
		projection.ClassificationAbsent,
		projection.ClassificationForeign,
		projection.ClassificationMalformed,
		projection.ClassificationUnavailable,
		projection.Classification("something else"),
		projection.Classification(""),
	} {
		for _, tool := range append(editTools, queryTool) {
			t.Run(string(class)+"/"+tool, func(t *testing.T) {
				got := projection.Gate(projection.GateInput{
					Binding:   projection.Loaded{Classification: class, Detail: "x"},
					Workflow:  w,
					EditTools: editTools,
					Call:      projection.ToolCall{Name: tool, Query: named("other")},
				})
				allowed(t, "binding "+string(class), got)
			})
		}
	}
}

// --- rules 2 and 3: a workflow that is not owned, or closed ------------------

func TestGateNeverDeniesOnAWorkflowItCannotFollow(t *testing.T) {
	paused := loadedWorkflow("odd", workflow.StatusPaused)
	for name, w := range map[string]*workflow.Loaded{
		"not loaded": nil,
		"absent":     {Classification: workflow.ClassificationAbsent, State: paused.State},
		"foreign":    {Classification: workflow.ClassificationForeign, State: paused.State, Detail: "d"},
		"malformed":  {Classification: workflow.ClassificationMalformed, State: paused.State},
		"drifted":    {Classification: workflow.ClassificationDrifted, State: paused.State},
		"unavailable": {
			Classification: workflow.ClassificationUnavailable, State: paused.State},
		"unrecognized": {Classification: workflow.Classification("odd one"), State: paused.State},
	} {
		for _, tool := range append(editTools, queryTool, pluginQueryTool) {
			t.Run(name+"/"+tool, func(t *testing.T) {
				allowed(t, name, gate(w, tool, named("other")))
			})
		}
	}
}

func TestGateAllowsEverythingForAClosedWorkflow(t *testing.T) {
	// The prompt hook unbinds a closed workflow; the gate is strictly read-only
	// and never does, it just stops gating.
	w := loadedWorkflow("odd", workflow.StatusClosed)
	for _, tool := range append(editTools, queryTool, pluginQueryTool, "Bash") {
		allowed(t, "closed/"+tool, gate(w, tool, named("other")))
	}
}

func TestGateAllowsAnUnrecognizedStatus(t *testing.T) {
	w := loadedWorkflow("odd", workflow.Status("hibernating"))
	for _, tool := range append(editTools, queryTool) {
		allowed(t, "status hibernating/"+tool, gate(w, tool, named("other")))
	}
}

// --- rule 4: the paused edit gate -------------------------------------------

func TestGateDeniesTheEditToolsOfAPausedWorkflow(t *testing.T) {
	w := loadedWorkflow("odd", workflow.StatusPaused)
	for _, tool := range editTools {
		t.Run(tool, func(t *testing.T) {
			got := gate(w, tool, unnamed)
			denied(t, tool, got,
				"wf-1", "proj-1", "paused", tool,
				"labdrian workflow resume --project proj-1 --workflow wf-1",
				"labdrian workflow unbind")
		})
	}
}

func TestGateNeverDeniesAnyOtherToolOfAPausedWorkflow(t *testing.T) {
	w := loadedWorkflow("odd", workflow.StatusPaused)
	// Near-misses of the four names, and the tools that must never be gated: Bash
	// cannot be classified reliably, and reads and memory writes are not edits.
	for _, tool := range []string{
		"write", "edit", "multiedit", "notebookedit", "WRITE",
		"Writer", "Editor", "MultiEdits", "NotebookEdit2", "NotebookRead",
		"Write ", " Edit", "Write\n", "Write\x00", "",
		"Bash", "Read", "Grep", "Glob", "Task", "Agent", "WebFetch", "TodoWrite",
		"mcp__engram__mem_save", "mcp__longterm-mem__promote", "mcp__longterm-mem__get",
		"mcp__fs__Write", "mcp__fs__Edit", "Edit__Write",
	} {
		allowed(t, fmt.Sprintf("paused/%q", tool), gate(w, tool, unnamed))
	}
}

func TestGateOnlyDeniesEditsWhilePaused(t *testing.T) {
	for _, status := range []workflow.Status{workflow.StatusCreated, workflow.StatusRunning} {
		for _, tool := range editTools {
			allowed(t, string(status)+"/"+tool, gate(loadedWorkflow("odd", status), tool, unnamed))
		}
	}
	for _, tool := range editTools {
		if got := gate(loadedWorkflow("odd", workflow.StatusPaused), tool, unnamed); !got.Deny {
			t.Errorf("paused/%s was not denied", tool)
		}
	}
}

func TestGateNamesTheWorkflowOfThePausedDenial(t *testing.T) {
	// The reason names the workflow the binding names, not anything else.
	b := ownedBinding()
	b.Binding.ProjectID, b.Binding.WorkflowID = "pay-core", "release-9"
	got := projection.Gate(projection.GateInput{
		Binding: b, Workflow: loadedWorkflow("odd", workflow.StatusPaused),
		EditTools: editTools, Call: projection.ToolCall{Name: "Edit"},
	})
	denied(t, "named workflow", got,
		"workflow release-9 of project pay-core",
		"labdrian workflow resume --project pay-core --workflow release-9")
}

// --- rule 5: the memory gate -------------------------------------------------

func TestGateRecognizesOnlyTheLongtermMemQueryToolName(t *testing.T) {
	// A wrong project, so a recognized query is denied and anything else is not.
	// Every open status applies.
	for _, tool := range []struct {
		name  string
		query bool
	}{
		{"mcp__longterm-mem__query", true},
		{"mcp__plugin_x_longterm-mem__query", true},
		{"mcp__plugin_labdrian_longterm-mem__query", true},
		{"mcp__a_b_c_longterm-mem__query", true},
		{"mcp__my-plugin_longterm-mem__query", true},
		{"mcp__longterm-mem__get", false},
		{"mcp__longterm-mem__promote", false},
		{"mcp__longterm-mem__queries", false},
		{"mcp__longterm-mem__query_all", false},
		{"mcp__longterm-mem__Query", false},
		{"mcp__longterm-memx__query", false},
		{"mcp__xlongterm-mem__query", false},
		{"mcp__plugin_xlongterm-mem__query", false},
		{"mcp__plugin_x_longterm-mem_y__query", false},
		{"mcp__engram__mem_search", false},
		{"mcp__engram__query", false},
		{"mcp__other__query", false},
		{"longterm-mem__query", false},
		{"mcp_longterm-mem__query", false},
		{"mcp__longterm-mem_query", false},
		{"mcp__longterm-mem__query__extra", false},
		{"mcp____longterm-mem__query", false},
		{"mcp__a____longterm-mem__query", false},
		{"mcp__a__longterm-mem__query", false},
		{"mcp___longterm-mem__query", false},
		{"mcp__longterm-mem__", false},
		{"mcp__query", false},
		{"query", false},
		{"", false},
		{"MCP__longterm-mem__query", false},
		{" mcp__longterm-mem__query", false},
		{"mcp__longterm-mem__query ", false},
		{"mcp__longterm-mem__query\n", false},
	} {
		for _, status := range openStatuses {
			t.Run(fmt.Sprintf("%q/%s", tool.name, status), func(t *testing.T) {
				got := gate(loadedWorkflow("odd", status), tool.name, named("other"))
				if tool.query {
					denied(t, tool.name, got, `"other"`, `"proj-1"`)
				} else {
					allowed(t, tool.name, got)
				}
			})
		}
	}
}

func TestGateComparesTheQueryProjectWithThePlanProject(t *testing.T) {
	// odd is scope project: the plan's project is the binding's, proj-1. A query that names no
	// project (missing, null, empty, not text: the adapter reads them all as "") is denied; one
	// whose arguments could not be read as named arguments is allowed with a warning.
	w := loadedWorkflow("odd", workflow.StatusRunning)
	for _, tt := range []struct {
		name    string
		query   projection.QueryArguments
		deny    bool
		wantIn  []string
		warning bool
	}{
		{name: "equal", query: named("proj-1")},
		{name: "different", query: named("proj-2"), deny: true,
			wantIn: []string{`"proj-2"`, `"proj-1"`, "Query project", "labdrian workflow unbind"}},
		{name: "different only in case", query: named("PROJ-1"), deny: true, wantIn: []string{`"PROJ-1"`, `"proj-1"`}},
		{name: "different by whitespace", query: named("proj-1 "), deny: true, wantIn: []string{`"proj-1 "`, `"proj-1"`}},
		{name: "a prefix of the plan project", query: named("proj"), deny: true, wantIn: []string{`"proj"`}},
		{name: "no project", query: named(""), deny: true, wantIn: []string{"no project", `"proj-1"`, "labdrian workflow unbind"}},
		{name: "arguments that could not be read", query: unnamed, warning: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := gate(w, queryTool, tt.query)
			switch {
			case tt.deny:
				denied(t, tt.name, got, tt.wantIn...)
			case tt.warning:
				// Unexpected input is allowed, and said out loud once.
				if got.Deny || got.Reason != "" || !strings.Contains(got.Warning, "not checked") || strings.Contains(got.Warning, "\n") {
					t.Errorf("Gate() = %+v, want an allow with a one-line warning that the query was not checked", got)
				}
			default:
				allowed(t, tt.name, got)
			}
		})
	}
}

func TestGateDeniesEveryQueryWhenThePlanHasNoProject(t *testing.T) {
	// standalone-minimal is scope none: no project is permitted.
	w := loadedWorkflow("standalone-minimal", workflow.StatusRunning)
	for _, query := range []projection.QueryArguments{named("proj-1"), named("other"), named(""), unnamed} {
		for _, tool := range []string{queryTool, pluginQueryTool} {
			got := gate(w, tool, query)
			denied(t, fmt.Sprintf("%s %+v", tool, query), got,
				"standalone-minimal", "permits no project", "labdrian workflow unbind")
		}
	}
	// Tools other than the query stay open, so is every edit tool while running.
	for _, tool := range []string{"mcp__longterm-mem__get", "mcp__longterm-mem__promote", "mcp__engram__mem_save", "Bash", "Edit", "Write"} {
		allowed(t, "scope none/"+tool, gate(w, tool, unnamed))
	}
}

func TestGateAppliesTheMemoryGateInEveryOpenStatusAndOnlyThen(t *testing.T) {
	for _, status := range openStatuses {
		denied(t, string(status), gate(loadedWorkflow("odd", status), queryTool, named("other")), `"other"`)
		allowed(t, string(status)+" right project", gate(loadedWorkflow("odd", status), queryTool, named("proj-1")))
	}
	allowed(t, "closed", gate(loadedWorkflow("odd", workflow.StatusClosed), queryTool, named("other")))
}

func TestGateUsesTheProfilesPlanForEveryProfile(t *testing.T) {
	// The gate and the context must agree on the plan, so the same inputs are
	// used: the context's memory line says which project (or none) is permitted,
	// and the gate must permit exactly that.
	for _, profile := range []string{"odd", "sdd", "standalone-minimal", "maintenance", "incident-recovery"} {
		t.Run(profile, func(t *testing.T) {
			w := loadedWorkflow(profile, workflow.StatusRunning)
			ctx := project(ownedBinding(), w).Context
			line := lineWithPrefix(t, ctx, "memory plan")
			permitted := !strings.Contains(line, "project_id=none")
			if permitted && !strings.Contains(line, "project_id=proj-1") {
				t.Fatalf("test bug: line %q names neither a project nor none", line)
			}

			right := gate(w, queryTool, named("proj-1"))
			wrong := gate(w, queryTool, named("proj-2"))
			if !wrong.Deny {
				t.Errorf("a query to the wrong project was not denied: %+v", wrong)
			}
			if permitted && right.Deny {
				t.Errorf("a query to the plan's project was denied: %+v", right)
			}
			if !permitted && !right.Deny {
				t.Errorf("the plan permits no project, yet a query was allowed: %+v", right)
			}
		})
	}
}

func TestGateAllowsAndWarnsWhenThePlanCannotBeComputed(t *testing.T) {
	// A profile that does not resolve: the plan cannot be computed, and the gate
	// never denies on an error. It says so, once, for the query only.
	w := loadedWorkflow("no-such-profile", workflow.StatusRunning)
	for _, tool := range []string{queryTool, pluginQueryTool} {
		got := gate(w, tool, named("other"))
		if got.Deny || got.Reason != "" || strings.Contains(got.Warning, "\n") ||
			!strings.Contains(got.Warning, "no-such-profile") || !strings.Contains(got.Warning, "not checked") || !strings.HasPrefix(got.Warning, "labdrian:") {
			t.Errorf("%s: Gate() = %+v, want an allow with a one-line warning that names the profile and says the query was not checked", tool, got)
		}
	}
	// Nothing else is affected, and nothing else warns.
	for _, tool := range []string{"Bash", "Edit", "mcp__longterm-mem__get", "mcp__longterm-mem__promote"} {
		allowed(t, "unresolvable profile/"+tool, gate(w, tool, unnamed))
	}
	// A paused workflow with such a profile still gates its edit tools: the paused
	// gate does not depend on the plan.
	paused := loadedWorkflow("no-such-profile", workflow.StatusPaused)
	denied(t, "paused, unresolvable profile", gate(paused, "Edit", unnamed), "paused")

	// A goal-scoped profile whose goal id was lost cannot be resolved either.
	noGoal := loadedWorkflow("incident-recovery", workflow.StatusRunning)
	noGoal.State.GoalID = ""
	if got := gate(noGoal, queryTool, named("other")); got.Deny || got.Warning == "" {
		t.Errorf("Gate() = %+v, want an allow and a warning: the plan needs a goal id the log does not have", got)
	}
}

func TestGateSanitizesAndBoundsWhatItQuotesFromTheToolInput(t *testing.T) {
	hostile := "evil\nsecond line \x1b[31mred\x1b[0m " + string(rune(0x202e)) + "bidi " + strings.Repeat("é", 5000)
	got := gate(loadedWorkflow("odd", workflow.StatusRunning), queryTool, named(hostile))
	denied(t, "hostile project", got, `evil\nsecond line`, `\x1b[31mred`, `\`+"u202ebidi", `"proj-1"`, "...")
	if len(got.Reason) > 1500 {
		t.Errorf("reason is %d bytes, want a short one", len(got.Reason))
	}
	if strings.ContainsRune(got.Reason, 0x202e) {
		t.Errorf("reason %q keeps a bidirectional control character", got.Reason)
	}
}

func TestGateSanitizesTheIdentifiersOfTheBinding(t *testing.T) {
	b := ownedBinding()
	b.Binding.ProjectID = "proj\n-1\x1b[0m"
	b.Binding.WorkflowID = "wf\r-1"
	got := projection.Gate(projection.GateInput{
		Binding: b, Workflow: loadedWorkflow("odd", workflow.StatusPaused),
		EditTools: editTools, Call: projection.ToolCall{Name: "Write"},
	})
	denied(t, "hostile identifiers", got, "paused")
}

func TestGateIsTotal(t *testing.T) {
	// Whatever the call carries, the gate returns a decision and never panics;
	// anything it cannot read is an allow.
	w := loadedWorkflow("odd", workflow.StatusRunning)
	for _, query := range []projection.QueryArguments{
		unnamed, named(""), named("\x00\xff\xfe"), named(strings.Repeat("a", 1<<20)), named(strings.Repeat("[", 100000)),
	} {
		for _, tool := range append(editTools, queryTool, "", "Bash") {
			got := gate(w, tool, query)
			if got.Deny && !strings.Contains(got.Reason, "project") {
				t.Errorf("Gate(%s, %.20q) = %+v, denied for a reason other than the project", tool, query.Project, got)
			}
		}
	}
	// The zero value of every part is an allow.
	if got := projection.Gate(projection.GateInput{}); got.Deny || got.Reason != "" || got.Warning != "" {
		t.Errorf("Gate(zero) = %+v, want a plain allow", got)
	}
}

func TestGateDoesNotModifyItsInput(t *testing.T) {
	w := loadedWorkflow("odd", workflow.StatusPaused)
	before := fmt.Sprintf("%+v", *w)
	tools := append([]string(nil), editTools...)
	in := projection.GateInput{Binding: ownedBinding(), Workflow: w, EditTools: tools, Call: projection.ToolCall{Name: queryTool, Query: named("other")}}
	first := projection.Gate(in)
	second := projection.Gate(in)
	if first != second {
		t.Errorf("Gate() is not deterministic: %+v then %+v", first, second)
	}
	if after := fmt.Sprintf("%+v", *w); before != after || strings.Join(tools, ",") != strings.Join(editTools, ",") {
		t.Error("Gate() changed its input")
	}
}

// --- the reason of a denial ----------------------------------------------------

// A denial without a reason would leave the model and the user guessing, so the gate says what
// it is. The sentence is the gate's own, and the same wherever it is printed.
func TestADenialAlwaysHasAnExplanation(t *testing.T) {
	const fallback = "labdrian: this tool call was denied by the workflow this repository is bound to."
	if got := (projection.GateResult{Deny: true}).Explanation(); got != fallback {
		t.Errorf("Explanation() of a denial with no reason = %q, want %q", got, fallback)
	}
	if got := (projection.GateResult{Deny: true, Reason: "no"}).Explanation(); got != "no" {
		t.Errorf("Explanation() of a denial with a reason = %q, want the reason", got)
	}
	// What Gate itself decides always has a reason of its own.
	for _, tool := range editTools {
		got := gate(loadedWorkflow("odd", workflow.StatusPaused), tool, unnamed)
		if got.Reason == "" || got.Explanation() != got.Reason {
			t.Errorf("the denial of %s = %+v, want its own reason", tool, got)
		}
	}
}
