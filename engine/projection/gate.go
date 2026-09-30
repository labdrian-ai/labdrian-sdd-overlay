package projection

// The PreToolUse gate: the pure decision of whether a tool call may go ahead in a
// session whose repository is bound to a workflow. Like Project, it is a function
// of its arguments and nothing else: it reads no file, starts no process, keeps
// no state, and never writes, so the caller (the hook command in engine/cmd) does
// the reading and prints GateResult.PreToolUseOutput.
//
// The gate has two rules, both about a workflow that is open (created, running,
// or paused) and owned:
//
//   - The paused edit gate. While the workflow is paused, the file-edit tools
//     Write, Edit, MultiEdit, and NotebookEdit are denied. Bash is never gated
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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// HookEventPreToolUse is the Claude Code hook event that runs before a tool call
// and can deny it.
const HookEventPreToolUse = "PreToolUse"

// PreToolUseInput is what the gate keeps of the JSON a Claude Code PreToolUse
// hook receives on stdin: the event's name, the session's working directory, the
// tool's name, and the tool's input exactly as sent. Nothing else is read; in
// particular not the session or the tool call's id.
type PreToolUseInput struct {
	// HookEventName is hook_event_name, or empty when the input has none.
	HookEventName string
	// Cwd is the session's working directory as a cleaned absolute path, or empty
	// when the input has none or it is not an absolute path.
	Cwd string
	// ToolName is tool_name, or empty when the input has none.
	ToolName string
	// ToolInput is tool_input as raw JSON, or empty when the input has none. Its
	// shape depends on the tool, so it is decoded only by the rule that needs it.
	ToolInput json.RawMessage
}

// ParsePreToolUseInput reads the fields the gate needs from a PreToolUse hook's
// stdin. As for ParseHookInput, the input is Claude Code's, not ours, so it is
// decoded leniently: a JSON object no larger than MaxHookInputBytes, with fields
// it does not name ignored (including ones a later Claude Code adds) and a
// relative cwd read as missing. A field this function reads with a value of the
// wrong type (a tool_name that is not a string) is an error, and so is a
// document that is not exactly one object. tool_input may be any JSON value.
func ParsePreToolUseInput(data []byte) (PreToolUseInput, error) {
	if len(data) > MaxHookInputBytes {
		return PreToolUseInput{}, fmt.Errorf("parse hook input: %w: %d bytes exceeds the maximum of %d", ErrHookInputTooLarge, len(data), MaxHookInputBytes)
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return PreToolUseInput{}, errors.New("parse hook input: the input is not a JSON object")
	}
	var wire struct {
		HookEventName string          `json:"hook_event_name"`
		Cwd           string          `json:"cwd"`
		ToolName      string          `json:"tool_name"`
		ToolInput     json.RawMessage `json:"tool_input"`
	}
	if err := decodeHookObject(data, &wire); err != nil {
		return PreToolUseInput{}, err
	}
	return PreToolUseInput{HookEventName: wire.HookEventName, Cwd: cleanHookCwd(wire.Cwd), ToolName: wire.ToolName, ToolInput: wire.ToolInput}, nil
}

// GateInput is everything Gate decides from: the classification of the
// repository's binding as the binding store reported it, and, only when the
// binding is owned, the workflow it names as the workflow store reported it (nil
// when it could not be loaded), and the tool call.
type GateInput struct {
	Binding   Loaded
	Workflow  *workflow.Loaded
	ToolName  string
	ToolInput json.RawMessage
}

// GateResult is what Gate decides. Deny is true for a denial, and Reason then
// says why and how to proceed. Warning is one line for the user, set only when
// the gate allows a call it would have liked to check and could not.
type GateResult struct {
	Deny    bool
	Reason  string
	Warning string
}

// editToolList is the one list of tool names the paused edit gate denies,
// matched exactly. README, the help text, and the Claude Code cancellation
// declaration retype it in prose; a test checks each copy against EditTools.
var editToolList = []string{"Write", "Edit", "MultiEdit", "NotebookEdit"}

// editTools is editToolList as a set.
var editTools = func() map[string]bool {
	m := make(map[string]bool, len(editToolList))
	for _, t := range editToolList {
		m[t] = true
	}
	return m
}()

// EditTools returns the tool names the paused edit gate denies, in a stable
// order. The result is a copy: editing it never changes the gate.
func EditTools() []string { return append([]string(nil), editToolList...) }

// GateRelevant reports whether Gate can ever have an opinion about a tool: a
// file-edit tool (paused edit gate) or the longterm-mem query tool (memory
// gate). It is decided from the name alone, so a caller can answer for every
// other tool without reading the binding or the workflow. Gate itself denies or
// warns only for tools this reports true for.
func GateRelevant(toolName string) bool {
	return editTools[toolName] || longtermQueryTool.MatchString(toolName)
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
	if !GateRelevant(in.ToolName) {
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
		if editTools[in.ToolName] {
			return GateResult{Deny: true, Reason: pausedReason(in.Binding.Binding, in.ToolName)}
		}
	case workflow.StatusCreated, workflow.StatusRunning:
	default:
		// Closed, or a status this package does not know: not a state the gate
		// enforces anything in.
		return GateResult{}
	}
	if longtermQueryTool.MatchString(in.ToolName) {
		return memoryGate(in.Binding.Binding, w.State, in.ToolInput)
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
func memoryGate(b Binding, s workflow.State, toolInput json.RawMessage) GateResult {
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

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(toolInput, &fields); err != nil || fields == nil {
		return GateResult{Warning: "labdrian: the input of this longterm-mem query is not a JSON object, so its project was not checked against the memory plan of " +
			workflowRef(b) + "."}
	}
	var given string
	if err := json.Unmarshal(fields["project"], &given); err != nil || given == "" {
		// Absent (a nil raw message does not unmarshal), null, not a string, or
		// empty: the query names no project.
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

// --- the hook output --------------------------------------------------------

// preToolUseOutput is the JSON object a PreToolUse hook prints to deny a call:
// the decision and its reason sit inside hookSpecificOutput, next to the event
// name, and the hook exits 0 (the JSON decides). Nothing here ever says "allow":
// that would bypass Claude Code's normal permission flow, so an allowed call
// prints nothing, or only a systemMessage.
type preToolUseOutput struct {
	HookSpecificOutput *preToolUseSpecific `json:"hookSpecificOutput,omitempty"`
	SystemMessage      string              `json:"systemMessage,omitempty"`
}

type preToolUseSpecific struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

// denyFallbackReason is the reason of a denial that somehow has none, so a denial
// never reaches the model and the user without a word.
const denyFallbackReason = "labdrian: this tool call was denied by the workflow this repository is bound to."

// PreToolUseOutput renders r as the one JSON object a PreToolUse hook writes to
// stdout, followed by a newline. A denial is
// {"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny",
// "permissionDecisionReason":"..."}}; a warning is a systemMessage. An allow with
// no warning yields no output at all, which Claude Code reads as "nothing to
// say". The JSON keeps <, >, and & as they are, so the transcript stays readable.
func (r GateResult) PreToolUseOutput() ([]byte, error) {
	if !r.Deny && r.Warning == "" {
		return nil, nil
	}
	out := preToolUseOutput{SystemMessage: r.Warning}
	if r.Deny {
		reason := r.Reason
		if reason == "" {
			reason = denyFallbackReason
		}
		out.HookSpecificOutput = &preToolUseSpecific{
			HookEventName:            HookEventPreToolUse,
			PermissionDecision:       "deny",
			PermissionDecisionReason: reason,
		}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		return nil, fmt.Errorf("projection: encode hook output: %w", err)
	}
	return buf.Bytes(), nil
}
