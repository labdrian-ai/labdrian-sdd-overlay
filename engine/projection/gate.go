package projection

// The PreToolUse gate: the pure decision of whether a tool call may go ahead in a
// session whose repository is bound to a workflow. Like Project, it is a function
// of its arguments and nothing else: it reads no file, starts no process, keeps
// no state, and never writes, so the caller (the hook command in engine/cmd) does
// the reading and says the result to the runtime (engine/hookwire). Nothing here
// knows the format that is spoken in: the gate is told the name of the tool and,
// for a memory query, what it names as its project.
//
// The gate has two rules, both about a workflow that is open (created, running,
// or paused) and owned:
//
//   - The paused edit gate. While the workflow is paused, the file-edit tools
//     are denied: the ones the caller names (GateInput.EditTools; the engine
//     names Write, Edit, MultiEdit, and NotebookEdit). Bash is never gated
//     (a shell command cannot be classified reliably), and neither is anything
//     that only reads.
//   - The memory gate. A call to the longterm-mem query tool is checked against
//     the workflow's memory plan: its project argument must be the plan's project,
//     and a plan with no project (scope none) permits no query at all. The tools
//     that carry no project (get), promote, every Engram tool, and every write
//     are never touched: the plan grants no write, and it does not forbid one.
//
// The gate denies only when it has a valid workflow state and a clear violation.
// Every unknown (no binding, a binding or workflow it cannot follow, a closed
// workflow, an unrecognized status, a plan it cannot compute, input it cannot
// read) is an allow, with a one-line warning only where the gate had something
// to check and could not. It never unbinds: removing the binding of a closed
// workflow is the prompt hook's job.

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// GateInput is everything Gate decides from: the classification of the
// repository's binding as the binding store reported it, and, only when the
// binding is owned, the workflow it names as the workflow store reported it (nil
// when it could not be loaded), the tools the paused edit gate denies, and the
// tool call.
type GateInput struct {
	Binding  Loaded
	Workflow *workflow.Loaded
	// EditTools are the names of the tools that edit a file, which the paused edit
	// gate denies, matched exactly. The caller says which: the gate has no list of
	// its own, and with none it gates no edit.
	EditTools []string
	Call      ToolCall
}

// ToolCall is the tool call the gate is asked about, as the gate reads it.
type ToolCall struct {
	// Name is the name of the tool, or empty when the call has none.
	Name string
	// Query is what the call names as the project of a memory query. It is read only
	// for a longterm-mem query, and is the zero value for any other call.
	Query QueryArguments
}

// QueryArguments is the arguments of a call read as those of a memory query.
type QueryArguments struct {
	// Named is true when the arguments could be read as named arguments, among which is the
	// project. It is false when they could not (they are not a set of named arguments).
	Named bool
	// Project is the project the query names: empty when it names none, which is the case
	// when the argument is missing, null, not text, or empty.
	Project string
}

// GateResult is what Gate decides. Deny is true for a denial, and Reason then
// says why and how to proceed. Warning is one line for the user, set only when
// the gate allows a call it would have liked to check and could not.
type GateResult struct {
	Deny    bool
	Reason  string
	Warning string
}

// denyFallbackReason is the reason of a denial that somehow has none, so a denial
// never reaches the model and the user without a word.
const denyFallbackReason = "labdrian: this tool call was denied by the workflow this repository is bound to."

// Explanation is what a denial says: its reason, or, for a denial that somehow has
// none, a fixed sentence. Gate always gives a reason; this is for the caller that
// would otherwise print a denial with nothing to read.
func (r GateResult) Explanation() string {
	if r.Reason == "" {
		return denyFallbackReason
	}
	return r.Reason
}

// GateRelevant reports whether Gate can ever have an opinion about a tool: a
// file-edit tool of editTools (paused edit gate) or the longterm-mem query tool
// (memory gate). It is decided from the name alone, so a caller can answer for every
// other tool without reading the binding or the workflow. Gate itself denies or
// warns only for tools this reports true for.
func GateRelevant(editTools []string, toolName string) bool {
	return slices.Contains(editTools, toolName) || longtermQueryTool.MatchString(toolName)
}

// longtermQueryTool matches the MCP name of the longterm-mem query tool and
// nothing looser: "mcp__", an optional plugin prefix made of one or more
// [A-Za-z0-9-] segments each followed by a single underscore, the server name
// "longterm-mem", and "__query". So "mcp__longterm-mem__query" and
// "mcp__plugin_x_longterm-mem__query" match, and "mcp__longterm-memx__query",
// "mcp__longterm-mem__get", "mcp__a__longterm-mem__query", and
// "longterm-mem__query" do not.
var longtermQueryTool = regexp.MustCompile(`^mcp__([A-Za-z0-9-]+_)*longterm-mem__query$`)

// Gate decides whether the tool call in in may go ahead. See the file comment
// for the rules. It is total: it never fails and never panics, and anything it
// does not understand is an allow.
func Gate(in GateInput) GateResult {
	if !GateRelevant(in.EditTools, in.Call.Name) {
		return GateResult{}
	}
	if in.Binding.Classification != ClassificationOwned {
		return GateResult{}
	}
	w := in.Workflow
	if w == nil || w.Classification != workflow.ClassificationOwned {
		return GateResult{}
	}
	switch w.State.Status {
	case workflow.StatusPaused:
		if slices.Contains(in.EditTools, in.Call.Name) {
			return GateResult{Deny: true, Reason: pausedReason(in.Binding.Binding, in.Call.Name)}
		}
	case workflow.StatusCreated, workflow.StatusRunning:
	default:
		// Closed, or a status this package does not know: not a state the gate
		// enforces anything in.
		return GateResult{}
	}
	if longtermQueryTool.MatchString(in.Call.Name) {
		return memoryGate(in.Binding.Binding, w.State, in.Call.Query)
	}
	return GateResult{}
}

func pausedReason(b Binding, tool string) string {
	flags := "--project " + sanitizeLine(b.ProjectID) + " --workflow " + sanitizeLine(b.WorkflowID)
	return "labdrian: " + workflowRef(b) + " is paused, so " + tool + " is denied: a paused workflow is not to be advanced. " +
		"Resume it with 'labdrian workflow resume " + flags + "', or stop following it with 'labdrian workflow unbind'."
}

// memoryGate checks one longterm-mem query against the memory plan of the
// workflow, computed exactly as the projected context computes it.
func memoryGate(b Binding, s workflow.State, query QueryArguments) GateResult {
	plan, err := resolvePlan(s.Profile, b.ProjectID, s.GoalID)
	if err != nil {
		return GateResult{Warning: "labdrian: the memory plan of " + workflowRef(b) + " could not be computed" + detailIn(err.Error()) +
			", so this longterm-mem query was not checked against it."}
	}
	if plan.Scope == memoryscope.ScopeNone {
		return GateResult{Deny: true, Reason: "labdrian: " + workflowRef(b) + " (profile " + strconv.Quote(sanitizeLine(s.Profile)) +
			") permits no project in its memory plan (scope none), so every longterm-mem query is denied. " +
			"Stop following this workflow with 'labdrian workflow unbind' to query."}
	}
	planProject := plan.Filters.ProjectID
	if planProject == "" {
		// A plan with a scope that names a project always has one; this cannot be
		// reached through Resolve, and the gate does not deny on what it cannot
		// explain.
		return GateResult{Warning: "labdrian: the memory plan of " + workflowRef(b) + " names no project although its scope is " +
			strconv.Quote(sanitizeLine(string(plan.Scope))) + ", so this longterm-mem query was not checked against it."}
	}

	if !query.Named {
		return GateResult{Warning: "labdrian: the input of this longterm-mem query is not a JSON object, so its project was not checked against the memory plan of " +
			workflowRef(b) + "."}
	}
	given := query.Project
	if given == "" {
		// Absent, null, not text, or empty: the query names no project.
		return GateResult{Deny: true, Reason: "labdrian: this longterm-mem query names no project (the project argument is missing, not a string, or empty), " +
			"but the memory plan of " + workflowRef(b) + " permits only project " + strconv.Quote(sanitizeLine(planProject)) + ". " + queryAdvice(planProject)}
	}
	if given != planProject {
		return GateResult{Deny: true, Reason: "labdrian: this longterm-mem query asks for project " + quoteValue(given) +
			", but the memory plan of " + workflowRef(b) + " permits only project " + strconv.Quote(sanitizeLine(planProject)) + ". " + queryAdvice(planProject)}
	}
	return GateResult{}
}

// quoteValue renders a value taken from the tool input as a quoted Go string cut
// to maxDetailRunes characters. Quoting escapes every control, format, and
// invalid character (a line break, an escape sequence, a bidirectional override)
// as visible text, so the value is safe on one line, and unlike sanitizing it
// keeps a difference that is only white space visible: "proj-1 " is not shown
// as "proj-1".
func quoteValue(s string) string {
	return strconv.Quote(clip(strings.ToValidUTF8(s, ""), maxDetailRunes))
}

// queryAdvice tells the model and the user how to go on after a denied query.
func queryAdvice(planProject string) string {
	return "Query project " + strconv.Quote(sanitizeLine(planProject)) + ", or stop following this workflow with 'labdrian workflow unbind'."
}
