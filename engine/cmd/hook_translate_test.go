package main

// Tests for the translation between the hook protocol and the policies (hook_translate.go): the
// mapping of each decoded value to the value a policy takes, and of each decision to the reply
// that says it. The decoders and encoders have their tests in engine/hookwire, the policies in
// engine/projection and engine/gate, and the bytes of every hook end to end are the golden
// files; what is tested here is only the seam between them, the part that is neither.

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gate"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
)

// decodePreToolUse is the decoded form of a PreToolUse input, which a test cannot build as a
// struct: its tool_input is private to hookwire.
func decodePreToolUse(t *testing.T, json string) hookwire.PreToolUse {
	t.Helper()
	in, err := hookwire.DecodePreToolUse([]byte(json))
	if err != nil {
		t.Fatalf("DecodePreToolUse(%q) = %v", json, err)
	}
	return in
}

// --- the tools the engine gates --------------------------------------------------------------

// The list is handed to every GateInput and read by the tests of the settings matcher. A caller
// that changed what it was given would change the list for every later call of the process, so
// each call gets a list of its own.
func TestGatedEditToolsAreAListOfTheCallersOwn(t *testing.T) {
	want := []string{"Write", "Edit", "MultiEdit", "NotebookEdit"}
	first, second := gatedEditTools(), gatedEditTools()
	first[0] = "Tampered"
	if !slices.Equal(second, want) {
		t.Errorf("a list changed by a caller changed another caller's: %v, want %v", second, want)
	}
	if got := gatedEditTools(); !slices.Equal(got, want) {
		t.Errorf("gatedEditTools() = %v after a caller changed an earlier result, want the four tools unchanged", got)
	}
}

// --- which occasion a warning is for ---------------------------------------------------------

func TestOccasionOfNamesWhatTheHookWasDoing(t *testing.T) {
	for event, want := range map[string]projection.Occasion{
		hookwire.EventUserPromptSubmit: projection.OccasionPrompt,
		hookwire.EventPreToolUse:       projection.OccasionToolCall,
		"":                             projection.OccasionUnknown,
		"PostToolUse":                  projection.OccasionUnknown,
		"userpromptsubmit":             projection.OccasionUnknown,
		"Stop":                         projection.OccasionUnknown,
	} {
		if got := occasionOf(event); got != want {
			t.Errorf("occasionOf(%q) = %v, want %v", event, got, want)
		}
	}
}

// --- the call the gate is asked about --------------------------------------------------------

func TestGateCallCarriesTheNameAndReadsTheArgumentsOfAMemoryQueryWhenAsked(t *testing.T) {
	const query = `{"hook_event_name":"PreToolUse","tool_name":"mcp__longterm-mem__query","tool_input":`
	for name, tc := range map[string]struct {
		input string
		want  projection.QueryArguments
	}{
		"a project":                    {query + `{"project":"proj-1"}}`, projection.QueryArguments{Named: true, Project: "proj-1"}},
		"no project":                   {query + `{"query":"q"}}`, projection.QueryArguments{Named: true}},
		"a project of the wrong type":  {query + `{"project":5}}`, projection.QueryArguments{Named: true}},
		"arguments that are not named": {query + `[1]}`, projection.QueryArguments{}},
		"arguments that are a string":  {query + `"proj-1"}`, projection.QueryArguments{}},
		"no arguments":                 {`{"tool_name":"mcp__longterm-mem__query"}`, projection.QueryArguments{}},
	} {
		t.Run(name, func(t *testing.T) {
			call := gateCall(decodePreToolUse(t, tc.input))
			if call.Name != "mcp__longterm-mem__query" {
				t.Errorf("Name = %q, want the tool's name", call.Name)
			}
			if call.ReadQuery == nil {
				t.Fatal("ReadQuery is nil, want a reader of the arguments")
			}
			if got := call.ReadQuery(); got != tc.want {
				t.Errorf("ReadQuery() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestGateCallOfAnotherToolKeepsOnlyItsName(t *testing.T) {
	call := gateCall(decodePreToolUse(t, `{"tool_name":"Write","tool_input":{"file_path":"/f","content":"x"}}`))
	if call.Name != "Write" {
		t.Errorf("Name = %q, want Write", call.Name)
	}
	if got := call.ReadQuery(); got != (projection.QueryArguments{Named: true}) {
		t.Errorf("ReadQuery() = %+v, want the arguments of an object with no project, read on request", got)
	}
	if got := gateCall(decodePreToolUse(t, `{}`)); got.Name != "" {
		t.Errorf("a call with no tool name = %+v, want the empty name", got)
	}
}

// --- what a decision of the gate says --------------------------------------------------------

func TestGateReplySaysWhatTheGateDecided(t *testing.T) {
	for name, tc := range map[string]struct {
		result projection.GateResult
		want   hookwire.PreToolUseReply
	}{
		"a denial with its reason":    {projection.GateResult{Deny: true, Reason: "paused"}, hookwire.PreToolUseReply{Deny: true, Reason: "paused"}},
		"a denial with no reason":     {projection.GateResult{Deny: true}, hookwire.PreToolUseReply{Deny: true, Reason: projection.GateResult{Deny: true}.Explanation()}},
		"an allow":                    {projection.GateResult{}, hookwire.PreToolUseReply{}},
		"an allow with a warning":     {projection.GateResult{Warning: "could not check"}, hookwire.PreToolUseReply{Warning: "could not check"}},
		"a denial and a warning":      {projection.GateResult{Deny: true, Reason: "r", Warning: "w"}, hookwire.PreToolUseReply{Deny: true, Reason: "r", Warning: "w"}},
		"a reason on an allow is not": {projection.GateResult{Reason: "left over"}, hookwire.PreToolUseReply{}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := gateReply(tc.result); got != tc.want {
				t.Errorf("gateReply(%+v) = %+v, want %+v", tc.result, got, tc.want)
			}
		})
	}
	// The sentence of a denial that has no reason is the policy's, not a copy here.
	if got := gateReply(projection.GateResult{Deny: true}).Reason; got == "" || !strings.Contains(got, "denied") {
		t.Errorf("the reason of a denial with none = %q, want the policy's sentence", got)
	}
}

func TestPromptReplySaysWhatTheProjectionProduced(t *testing.T) {
	for name, tc := range map[string]struct {
		result projection.ProjectionResult
		want   hookwire.PromptReply
	}{
		"nothing":                     {projection.ProjectionResult{}, hookwire.PromptReply{}},
		"a context":                   {projection.ProjectionResult{Context: "c"}, hookwire.PromptReply{Context: "c"}},
		"a warning":                   {projection.ProjectionResult{Warning: "w"}, hookwire.PromptReply{Warning: "w"}},
		"both":                        {projection.ProjectionResult{Context: "c", Warning: "w"}, hookwire.PromptReply{Context: "c", Warning: "w"}},
		"the unbind is not an answer": {projection.ProjectionResult{Context: "c", Unbind: true}, hookwire.PromptReply{Context: "c"}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := promptReply(tc.result); got != tc.want {
				t.Errorf("promptReply(%+v) = %+v, want %+v", tc.result, got, tc.want)
			}
		})
	}
}

// The projection bounds the context it produces (projection.MaxContextBytes) before it is
// encoded, and encoding can only make it longer: every line break and quote of a context is two
// bytes of JSON. What reaches Claude Code is still one valid JSON object, the context nested in
// hookSpecificOutput (at the top level Claude Code ignores it without a word), and the context
// comes back from it whole, whatever characters it holds. The states a real workflow projects
// are pinned end to end by the golden files (prompt-projects-an-open-workflow and the odd
// profile); this is the worst case of the same path, a context of exactly the bound made of the
// characters the encoder escapes.
func TestAContextAtTheProjectionsBoundSurvivesTheEncoder(t *testing.T) {
	unit := "a\n\"b\\<c>&d\té" + string(rune(0x2028)) + "\x01"
	// Whole units, then plain letters up to the bound: a cut inside a multi-byte character would
	// have to be repaired, and the repair could leave the context shorter than the bound.
	context := strings.Repeat(unit, projection.MaxContextBytes/len(unit))
	context += strings.Repeat("x", projection.MaxContextBytes-len(context))
	if len(context) != projection.MaxContextBytes || !utf8.ValidString(context) {
		t.Fatalf("the context is %d bytes (valid UTF-8: %v), want exactly %d", len(context), utf8.ValidString(context), projection.MaxContextBytes)
	}
	out, err := promptReply(projection.ProjectionResult{Context: context, Warning: "a warning"}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(out) || !strings.HasSuffix(string(out), "}\n") || strings.Count(string(out), "\n") != 1 {
		t.Fatalf("the answer is not one JSON object on one line (%d bytes)", len(out))
	}
	var decoded struct {
		HookSpecific struct {
			EventName         string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
		SystemMessage string `json:"systemMessage"`
		AtTopLevel    string `json:"additionalContext"`
	}
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.HookSpecific.AdditionalContext != context || decoded.HookSpecific.EventName != hookwire.EventUserPromptSubmit {
		t.Errorf("the context did not come back whole from hookSpecificOutput (got %d bytes, event %q)", len(decoded.HookSpecific.AdditionalContext), decoded.HookSpecific.EventName)
	}
	if decoded.AtTopLevel != "" || decoded.SystemMessage != "a warning" {
		t.Errorf("additionalContext at the top level = %q, systemMessage = %q, want none and the warning", decoded.AtTopLevel, decoded.SystemMessage)
	}
	if len(out) <= len(context) {
		t.Errorf("the answer is %d bytes for a context of %d: encoding cannot make it shorter", len(out), len(context))
	}
	// The escapes are spelled in pieces so no tool that handles this file decodes them.
	esc := func(code string) string { return `\u` + code }
	if strings.Contains(string(out), esc("003c")) || strings.Contains(string(out), esc("0026")) {
		t.Error("the encoder escaped < or &: a prompt reply keeps them as they are")
	}
}

// --- what 'gate-task' prints -----------------------------------------------------------------

// sdApplyGateConfig is a contract that injects its path into the prompt of an sdd-apply sub-agent.
var sdApplyGateConfig = gate.Config{Contracts: []gate.ContractConfig{{Path: absoluteContractPath, Content: contractContentForE2E}}}

const sdApplyCallInput = `{"tool_name":"Agent","tool_input":{"description":"d","prompt":"do it","subagent_type":"sdd-apply"}}`

func TestAgentGateAnswerRewritesWhatTheGateChanges(t *testing.T) {
	out := agentGateAnswer([]byte(sdApplyCallInput), sdApplyGateConfig)
	if !strings.Contains(string(out), `"updatedInput"`) || !strings.Contains(string(out), absoluteContractPath) || !strings.HasSuffix(string(out), "}\n") {
		t.Errorf("agentGateAnswer() = %q, want the call with the contract injected, on one line", out)
	}
}

// Everything the gate leaves alone, and everything it cannot read, is the one pass-through, byte
// for byte: the fail-safe of the hook never denies and never says anything but {}.
func TestAgentGateAnswerPassesThroughWhatItLeavesAloneOrCannotRead(t *testing.T) {
	for name, raw := range map[string]string{
		"a sub-agent the contract does not apply to":  `{"tool_name":"Agent","tool_input":{"description":"d","prompt":"p","subagent_type":"sdd-propose"}}`,
		"a call with no prompt":                       `{"tool_name":"Agent","tool_input":{"description":"d","subagent_type":"sdd-tasks"}}`,
		"an empty prompt":                             `{"tool_name":"Agent","tool_input":{"description":"d","prompt":"","subagent_type":"sdd-tasks"}}`,
		"a tool input of null":                        `{"tool_name":"Agent","tool_input":null}`,
		"input that is not JSON":                      `nope`,
		"input that is empty":                         ``,
		"input with no tool_input":                    `{"tool_name":"Agent"}`,
		"a field of the wrong type":                   `{"tool_name":"Agent","tool_input":{"prompt":5,"subagent_type":"sdd-apply"}}`,
		"input over the bound":                        sdApplyCallInput + strings.Repeat(" ", hookwire.MaxAgentCallBytes),
		"input over the bound that holds a good call": strings.Replace(sdApplyCallInput, `"do it"`, `"`+strings.Repeat("x", hookwire.MaxAgentCallBytes)+`"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if got := string(agentGateAnswer([]byte(raw), sdApplyGateConfig)); got != "{}\n" {
				t.Errorf("agentGateAnswer() = %q, want the pass-through {}", clip(got))
			}
		})
	}
}

// A call that was rewritten and could not be written down is let through as it came: the
// encoder of the answer cannot fail for text, but if it did the hook would still never block.
func TestAnAnswerThatCannotBeEncodedIsThePassThrough(t *testing.T) {
	failing := func(hookwire.AgentCall, string) ([]byte, error) { return nil, errors.New("cannot encode") }
	if out, ok := rewrittenAgentCall([]byte(sdApplyCallInput), sdApplyGateConfig, failing); ok || out != nil {
		t.Errorf("rewrittenAgentCall with an encoder that fails = %q, %v, want no rewrite", out, ok)
	}
	var got hookwire.AgentCall
	var gotPrompt string
	recording := func(call hookwire.AgentCall, prompt string) ([]byte, error) {
		got, gotPrompt = call, prompt
		return []byte("encoded\n"), nil
	}
	out, ok := rewrittenAgentCall([]byte(sdApplyCallInput), sdApplyGateConfig, recording)
	if !ok || string(out) != "encoded\n" {
		t.Fatalf("rewrittenAgentCall = %q, %v, want what the encoder wrote", out, ok)
	}
	if want := (hookwire.AgentCall{Description: "d", Prompt: "do it", SubagentType: "sdd-apply"}); !reflect.DeepEqual(got, want) {
		t.Errorf("the encoder was given the call %+v, want %+v", got, want)
	}
	if !strings.Contains(gotPrompt, absoluteContractPath) || !strings.HasPrefix(gotPrompt, "do it") {
		t.Errorf("the encoder was given the prompt %q, want the gate's rewrite of the call's", gotPrompt)
	}
}

func clip(s string) string {
	if len(s) > 80 {
		return s[:80] + "..."
	}
	return s
}
