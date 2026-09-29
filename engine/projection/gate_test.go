package projection_test

// Tests for the PreToolUse gate: the pure decision (Gate), the lenient parsing of
// the hook input (ParsePreToolUseInput), and the output Claude Code reads
// (PreToolUseOutput). Everything here is a function of its arguments; nothing
// reads a file or starts a process.

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// --- fixtures ---------------------------------------------------------------

var (
	editTools    = []string{"Write", "Edit", "MultiEdit", "NotebookEdit"}
	openStatuses = []workflow.Status{workflow.StatusCreated, workflow.StatusRunning, workflow.StatusPaused}
)

const (
	queryTool       = "mcp__longterm-mem__query"
	pluginQueryTool = "mcp__plugin_x_longterm-mem__query"
)

// gate runs the gate for an owned binding to wf-1 of proj-1 and the workflow w.
func gate(w *workflow.Loaded, tool string, input string) projection.GateResult {
	return projection.Gate(projection.GateInput{
		Binding:   ownedBinding(),
		Workflow:  w,
		ToolName:  tool,
		ToolInput: json.RawMessage(input),
	})
}

// queryInput is the tool_input of a longterm-mem query for project (a JSON
// value, or "" to leave the key out).
func queryInput(project string) string {
	if project == "" {
		return `{"query":"anything","limit":5}`
	}
	return `{"query":"anything","project":` + project + `}`
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
					ToolName:  tool,
					ToolInput: json.RawMessage(queryInput(`"other"`)),
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
				allowed(t, name, gate(w, tool, queryInput(`"other"`)))
			})
		}
	}
}

func TestGateAllowsEverythingForAClosedWorkflow(t *testing.T) {
	// The prompt hook unbinds a closed workflow; the gate is strictly read-only
	// and never does, it just stops gating.
	w := loadedWorkflow("odd", workflow.StatusClosed)
	for _, tool := range append(editTools, queryTool, pluginQueryTool, "Bash") {
		allowed(t, "closed/"+tool, gate(w, tool, queryInput(`"other"`)))
	}
}

func TestGateAllowsAnUnrecognizedStatus(t *testing.T) {
	w := loadedWorkflow("odd", workflow.Status("hibernating"))
	for _, tool := range append(editTools, queryTool) {
		allowed(t, "status hibernating/"+tool, gate(w, tool, queryInput(`"other"`)))
	}
}

// --- rule 4: the paused edit gate -------------------------------------------

func TestGateDeniesTheEditToolsOfAPausedWorkflow(t *testing.T) {
	w := loadedWorkflow("odd", workflow.StatusPaused)
	for _, tool := range editTools {
		t.Run(tool, func(t *testing.T) {
			got := gate(w, tool, `{"file_path":"/repo/x.go"}`)
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
		allowed(t, fmt.Sprintf("paused/%q", tool), gate(w, tool, `{"file_path":"/repo/x.go"}`))
	}
}

func TestGateOnlyDeniesEditsWhilePaused(t *testing.T) {
	for _, status := range []workflow.Status{workflow.StatusCreated, workflow.StatusRunning} {
		for _, tool := range editTools {
			allowed(t, string(status)+"/"+tool, gate(loadedWorkflow("odd", status), tool, `{}`))
		}
	}
	for _, tool := range editTools {
		if got := gate(loadedWorkflow("odd", workflow.StatusPaused), tool, `{}`); !got.Deny {
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
		ToolName: "Edit", ToolInput: json.RawMessage(`{}`),
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
				got := gate(loadedWorkflow("odd", status), tool.name, queryInput(`"other"`))
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
	// odd is scope project: the plan's project is the binding's, proj-1.
	w := loadedWorkflow("odd", workflow.StatusRunning)
	for _, tt := range []struct {
		name    string
		input   string
		deny    bool
		wantIn  []string
		warning bool
	}{
		{name: "equal", input: queryInput(`"proj-1"`)},
		{name: "different", input: queryInput(`"proj-2"`), deny: true,
			wantIn: []string{`"proj-2"`, `"proj-1"`, "Query project", "labdrian workflow unbind"}},
		{name: "different only in case", input: queryInput(`"PROJ-1"`), deny: true, wantIn: []string{`"PROJ-1"`, `"proj-1"`}},
		{name: "different by whitespace", input: queryInput(`"proj-1 "`), deny: true, wantIn: []string{`"proj-1 "`, `"proj-1"`}},
		{name: "a prefix of the plan project", input: queryInput(`"proj"`), deny: true, wantIn: []string{`"proj"`}},
		{name: "missing", input: queryInput(""), deny: true, wantIn: []string{"no project", `"proj-1"`, "labdrian workflow unbind"}},
		{name: "empty", input: queryInput(`""`), deny: true, wantIn: []string{"no project", `"proj-1"`}},
		{name: "null", input: queryInput(`null`), deny: true, wantIn: []string{"no project", `"proj-1"`}},
		{name: "a number", input: queryInput(`1`), deny: true, wantIn: []string{"no project", `"proj-1"`}},
		{name: "a boolean", input: queryInput(`true`), deny: true, wantIn: []string{"no project", `"proj-1"`}},
		{name: "an array holding the right project", input: queryInput(`["proj-1"]`), deny: true, wantIn: []string{"no project"}},
		{name: "an object", input: queryInput(`{"id":"proj-1"}`), deny: true, wantIn: []string{"no project"}},
		{name: "an empty object", input: `{}`, deny: true, wantIn: []string{"no project"}},
		{name: "the key in another case", input: `{"Project":"proj-1"}`, deny: true, wantIn: []string{"no project"}},
		{name: "the last of a repeated key wins, as in the server", input: `{"project":"proj-2","project":"proj-1"}`},
		{name: "input that is not an object", input: `"proj-1"`, warning: true},
		{name: "an array as input", input: `["proj-1"]`, warning: true},
		{name: "null input", input: `null`, warning: true},
		{name: "no input at all", input: ``, warning: true},
		{name: "input that is not JSON", input: `{"project":`, warning: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := gate(w, queryTool, tt.input)
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
	for _, input := range []string{queryInput(`"proj-1"`), queryInput(`"other"`), queryInput(""), queryInput(`""`), `{}`, `null`, ``} {
		for _, tool := range []string{queryTool, pluginQueryTool} {
			got := gate(w, tool, input)
			denied(t, fmt.Sprintf("%s %q", tool, input), got,
				"standalone-minimal", "permits no project", "labdrian workflow unbind")
		}
	}
	// Tools other than the query stay open, so is every edit tool while running.
	for _, tool := range []string{"mcp__longterm-mem__get", "mcp__longterm-mem__promote", "mcp__engram__mem_save", "Bash", "Edit", "Write"} {
		allowed(t, "scope none/"+tool, gate(w, tool, `{}`))
	}
}

func TestGateAppliesTheMemoryGateInEveryOpenStatusAndOnlyThen(t *testing.T) {
	for _, status := range openStatuses {
		denied(t, string(status), gate(loadedWorkflow("odd", status), queryTool, queryInput(`"other"`)), `"other"`)
		allowed(t, string(status)+" right project", gate(loadedWorkflow("odd", status), queryTool, queryInput(`"proj-1"`)))
	}
	allowed(t, "closed", gate(loadedWorkflow("odd", workflow.StatusClosed), queryTool, queryInput(`"other"`)))
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

			right := gate(w, queryTool, queryInput(`"proj-1"`))
			wrong := gate(w, queryTool, queryInput(`"proj-2"`))
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
		got := gate(w, tool, queryInput(`"other"`))
		if got.Deny || got.Reason != "" || strings.Contains(got.Warning, "\n") ||
			!strings.Contains(got.Warning, "no-such-profile") || !strings.Contains(got.Warning, "not checked") || !strings.HasPrefix(got.Warning, "labdrian:") {
			t.Errorf("%s: Gate() = %+v, want an allow with a one-line warning that names the profile and says the query was not checked", tool, got)
		}
	}
	// Nothing else is affected, and nothing else warns.
	for _, tool := range []string{"Bash", "Edit", "mcp__longterm-mem__get", "mcp__longterm-mem__promote"} {
		allowed(t, "unresolvable profile/"+tool, gate(w, tool, `{}`))
	}
	// A paused workflow with such a profile still gates its edit tools: the paused
	// gate does not depend on the plan.
	paused := loadedWorkflow("no-such-profile", workflow.StatusPaused)
	denied(t, "paused, unresolvable profile", gate(paused, "Edit", `{}`), "paused")

	// A goal-scoped profile whose goal id was lost cannot be resolved either.
	noGoal := loadedWorkflow("incident-recovery", workflow.StatusRunning)
	noGoal.State.GoalID = ""
	if got := gate(noGoal, queryTool, queryInput(`"other"`)); got.Deny || got.Warning == "" {
		t.Errorf("Gate() = %+v, want an allow and a warning: the plan needs a goal id the log does not have", got)
	}
}

func TestGateSanitizesAndBoundsWhatItQuotesFromTheToolInput(t *testing.T) {
	hostile := "evil\\nsecond line \\u001b[31mred\\u001b[0m \\u202ebidi " + strings.Repeat("é", 5000)
	got := gate(loadedWorkflow("odd", workflow.StatusRunning), queryTool, queryInput(`"`+hostile+`"`))
	denied(t, "hostile project", got, `evil\nsecond line`, `\x1b[31mred`, `\u202ebidi`, `"proj-1"`, "...")
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
		ToolName: "Write", ToolInput: json.RawMessage(`{}`),
	})
	denied(t, "hostile identifiers", got, "paused")
}

func TestGateIsTotal(t *testing.T) {
	// Whatever the tool input holds, the gate returns a decision and never
	// panics; anything it cannot read is an allow.
	w := loadedWorkflow("odd", workflow.StatusRunning)
	for _, input := range []string{
		"", "null", "0", "[]", `""`, "{", "}", `{"project"`, `{"project":}`, "\x00\xff\xfe",
		strings.Repeat("[", 100000), `{"project":"` + strings.Repeat("a", 1<<20) + `"}`,
	} {
		for _, tool := range append(editTools, queryTool, "", "Bash") {
			got := gate(w, tool, input)
			if got.Deny && !strings.Contains(got.Reason, "project") {
				t.Errorf("Gate(%s, %.20q) = %+v, denied for a reason other than the project", tool, input, got)
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
	before, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(queryInput(`"other"`))
	rawBefore := string(raw)
	in := projection.GateInput{Binding: ownedBinding(), Workflow: w, ToolName: queryTool, ToolInput: raw}
	first := projection.Gate(in)
	second := projection.Gate(in)
	if first != second {
		t.Errorf("Gate() is not deterministic: %+v then %+v", first, second)
	}
	after, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || string(raw) != rawBefore {
		t.Error("Gate() changed its input")
	}
}

// --- ParsePreToolUseInput ----------------------------------------------------

func TestParsePreToolUseInputKeepsOnlyWhatTheGateNeeds(t *testing.T) {
	data := `{
		"session_id": "abc123",
		"transcript_path": "/home/u/.claude/projects/x/abc123.jsonl",
		"cwd": "/home/u/../v/repo/",
		"permission_mode": "default",
		"hook_event_name": "PreToolUse",
		"tool_name": "mcp__longterm-mem__query",
		"tool_input": {"query":"x","project":"proj-1","nested":[1,{"a":null}]},
		"tool_use_id": "toolu_1",
		"a_future_field": {"x": [1, 2]}
	}`
	got, err := projection.ParsePreToolUseInput([]byte(data))
	if err != nil {
		t.Fatalf("ParsePreToolUseInput() = %v, want nil", err)
	}
	if got.HookEventName != "PreToolUse" || got.Cwd != "/home/v/repo" || got.ToolName != "mcp__longterm-mem__query" {
		t.Fatalf("ParsePreToolUseInput() = %+v, want the event, the cleaned cwd, and the tool name", got)
	}
	var input map[string]any
	if err := json.Unmarshal(got.ToolInput, &input); err != nil || input["project"] != "proj-1" {
		t.Errorf("tool_input = %s (%v), want the raw input kept", got.ToolInput, err)
	}
}

func TestParsePreToolUseInputAcceptsAnyToolInputAndMissingFields(t *testing.T) {
	for _, tt := range []struct {
		name, data, wantInput string
	}{
		{"an object", `{"tool_name":"Edit","tool_input":{"a":1}}`, `{"a":1}`},
		{"a string", `{"tool_name":"Edit","tool_input":"x"}`, `"x"`},
		{"an array", `{"tool_name":"Edit","tool_input":[1]}`, `[1]`},
		{"null", `{"tool_name":"Edit","tool_input":null}`, `null`},
		{"absent", `{"tool_name":"Edit"}`, ``},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := projection.ParsePreToolUseInput([]byte(tt.data))
			if err != nil || got.ToolName != "Edit" || string(got.ToolInput) != tt.wantInput {
				t.Fatalf("ParsePreToolUseInput() = %+v, %v, want tool Edit and tool_input %q", got, err, tt.wantInput)
			}
		})
	}
	got, err := projection.ParsePreToolUseInput([]byte(" \n{}\n "))
	if err != nil || got.HookEventName != "" || got.Cwd != "" || got.ToolName != "" || len(got.ToolInput) != 0 {
		t.Errorf("ParsePreToolUseInput({}) = %+v, %v, want the zero value", got, err)
	}
}

func TestParsePreToolUseInputAcceptsOnlyAnAbsoluteWorkingDirectory(t *testing.T) {
	for _, tt := range []struct{ name, cwd, want string }{
		{"absolute", `"/home/u/repo"`, "/home/u/repo"},
		{"absolute with dot segments is cleaned", `"/home/u/../v/repo"`, "/home/v/repo"},
		{"relative", `"repo/sub"`, ""},
		{"empty", `""`, ""},
		{"null", `null`, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := projection.ParsePreToolUseInput([]byte(`{"cwd":` + tt.cwd + `}`))
			if err != nil || got.Cwd != tt.want {
				t.Fatalf("ParsePreToolUseInput() = %+v, %v, want cwd %q", got, err, tt.want)
			}
		})
	}
}

func TestParsePreToolUseInputRefusesWhatIsNotAUsableObject(t *testing.T) {
	for name, data := range map[string]string{
		"empty":                          "",
		"whitespace":                     "  \n",
		"a string":                       `"PreToolUse"`,
		"an array":                       `[]`,
		"a number":                       `1`,
		"null":                           `null`,
		"not JSON":                       `{"tool_name":`,
		"trailing data":                  `{"tool_name":"Edit"} {"x":1}`,
		"a tool name that is a number":   `{"tool_name":7}`,
		"a tool name that is an object":  `{"tool_name":{"x":1}}`,
		"an event name that is a number": `{"hook_event_name":1}`,
		"a cwd that is a number":         `{"cwd":1}`,
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := projection.ParsePreToolUseInput([]byte(data)); err == nil {
				t.Errorf("ParsePreToolUseInput(%q) = %+v, want an error", data, got)
			}
		})
	}
}

func TestParsePreToolUseInputSizeCap(t *testing.T) {
	pad := func(n int) []byte {
		prefix, suffix := `{"tool_name":"Edit","tool_input":"`, `"}`
		return []byte(prefix + strings.Repeat("a", n-len(prefix)-len(suffix)) + suffix)
	}
	if _, err := projection.ParsePreToolUseInput(pad(projection.MaxHookInputBytes)); err != nil {
		t.Errorf("an input of exactly MaxHookInputBytes = %v, want it accepted", err)
	}
	if _, err := projection.ParsePreToolUseInput(pad(projection.MaxHookInputBytes + 1)); err == nil {
		t.Error("an input one byte over MaxHookInputBytes was accepted")
	}
}

// --- PreToolUseOutput --------------------------------------------------------

func TestPreToolUseOutputOfADenialIsTheDocumentedShape(t *testing.T) {
	reason := "workflow wf-1 <of> project proj-1 & more is paused"
	out, err := projection.GateResult{Deny: true, Reason: reason}.PreToolUseOutput()
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	if !strings.HasPrefix(text, "{") || !strings.HasSuffix(text, "}\n") || strings.Count(text, "\n") != 1 {
		t.Fatalf("output %q is not exactly one JSON object on one line", text)
	}
	// The escapes are spelled in two pieces so no tooling decodes them in this source.
	if strings.Contains(text, "\\"+"u003c") || strings.Contains(text, "\\"+"u003e") || strings.Contains(text, "\\"+"u0026") {
		t.Errorf("output %q escapes <, >, or &; the transcript should stay readable", text)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(out, &top); err != nil {
		t.Fatal(err)
	}
	if len(top) != 1 || top["hookSpecificOutput"] == nil {
		t.Fatalf("output %s, want exactly hookSpecificOutput (the JSON decides; no top-level decision)", out)
	}
	var specific map[string]string
	if err := json.Unmarshal(top["hookSpecificOutput"], &specific); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"hookEventName":            "PreToolUse",
		"permissionDecision":       "deny",
		"permissionDecisionReason": reason,
	}
	if len(specific) != len(want) {
		t.Fatalf("hookSpecificOutput = %v, want exactly %v", specific, want)
	}
	for k, v := range want {
		if specific[k] != v {
			t.Errorf("hookSpecificOutput[%q] = %q, want %q", k, specific[k], v)
		}
	}
}

func TestPreToolUseOutputOfAnAllowIsNothingOrOnlyAWarning(t *testing.T) {
	out, err := projection.GateResult{}.PreToolUseOutput()
	if err != nil || len(out) != 0 {
		t.Errorf("a plain allow printed %q, %v, want nothing (never permissionDecision allow: that would bypass the normal permission flow)", out, err)
	}
	out, err = projection.GateResult{Warning: "labdrian: careful"}.PreToolUseOutput()
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(out, &top); err != nil {
		t.Fatalf("output %q: %v", out, err)
	}
	if len(top) != 1 || string(top["systemMessage"]) != `"labdrian: careful"` {
		t.Errorf("output %s, want only the systemMessage", out)
	}
	if strings.Contains(string(out), "permissionDecision") {
		t.Errorf("output %s carries a permission decision for an allow", out)
	}
}

func TestPreToolUseOutputOfADenialWithAWarningCarriesBoth(t *testing.T) {
	out, err := projection.GateResult{Deny: true, Reason: "no", Warning: "w"}.PreToolUseOutput()
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(out, &top); err != nil || len(top) != 2 || top["hookSpecificOutput"] == nil || string(top["systemMessage"]) != `"w"` {
		t.Errorf("output %s (%v), want hookSpecificOutput and systemMessage", out, err)
	}
}

func TestPreToolUseOutputOfADenialAlwaysCarriesAReason(t *testing.T) {
	// A denial without a reason would leave the model and the user guessing.
	out, err := projection.GateResult{Deny: true}.PreToolUseOutput()
	if err != nil {
		t.Fatal(err)
	}
	var top struct {
		Specific struct {
			Reason string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(out, &top); err != nil || top.Specific.Reason == "" {
		t.Errorf("output %s (%v), want a non-empty permissionDecisionReason", out, err)
	}
}
