package main

// The place where the hook protocol meets the policies. engine/hookwire reads and writes what
// Claude Code reads and writes, and knows no policy; engine/projection, engine/gate and the
// other policies decide what a hook says, and know no wire. This file is the translation
// between the two, the one thing the composition root adds: which tools the engine gates, and
// the mapping of each decoded value to the value a policy takes and of each decision to the
// reply that says it.

import (
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gate"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
)

// gatedEditTools are the tools that edit a file, which the PreToolUse gate denies while the
// workflow of a bound repository is paused, matched exactly. It is the one list: README, the
// help text and the Claude Code cancellation declaration retype it in prose, and the settings
// matcher that routes these tools to the hook retypes it (settings.ProjectionEditToolMatcher);
// tests check each copy against it. A tool added here is gated only once the matcher installed
// in Claude Code names it too, which is what the test of the matcher is for.
//
// Each call returns a list of its own, as the gate has always been given one, so that nothing
// that holds the list can change it for the next hook call of the process.
func gatedEditTools() []string { return []string{"Write", "Edit", "MultiEdit", "NotebookEdit"} }

// occasionOf is what the projection hook is doing when it answers event, for the wording of a
// warning after a panic.
func occasionOf(event string) projection.Occasion {
	switch event {
	case hookwire.EventUserPromptSubmit:
		return projection.OccasionPrompt
	case hookwire.EventPreToolUse:
		return projection.OccasionToolCall
	default:
		return projection.OccasionUnknown
	}
}

// gateCall is a tool call of a PreToolUse input as the projection gate reads it. The arguments
// are read as those of a memory query, and only when the gate asks for them (projection.ToolCall):
// reading them decodes the whole input of the call, which for a file edit is the file, and the
// gate wants them for a memory query alone.
func gateCall(in hookwire.PreToolUse) projection.ToolCall {
	return projection.ToolCall{Name: in.Tool, ReadQuery: func() projection.QueryArguments {
		q := in.Query()
		return projection.QueryArguments{Named: q.Named, Project: q.Project}
	}}
}

// gateReply is what a decision of the projection gate says to Claude Code. Only a denial has a
// reason to give: an allow says nothing, or its warning.
func gateReply(r projection.GateResult) hookwire.PreToolUseReply {
	reply := hookwire.PreToolUseReply{Deny: r.Deny, Warning: r.Warning}
	if r.Deny {
		reply.Reason = r.Explanation()
	}
	return reply
}

// promptReply is what a projection says to Claude Code.
func promptReply(r projection.ProjectionResult) hookwire.PromptReply {
	return hookwire.PromptReply{Context: r.Context, Warning: r.Warning}
}

// agentGateAnswer is what 'gate-task' prints for the hook input raw: the call with the prompt
// the gate gives it, or, for everything the gate leaves alone and everything it cannot read, the
// pass-through that leaves the call as it is. It never fails, and never denies.
func agentGateAnswer(raw []byte, cfg gate.Config) []byte {
	if out, ok := rewrittenAgentCall(raw, cfg, hookwire.AgentCall.UpdatedInput); ok {
		return out
	}
	return hookwire.PassThrough()
}

// rewrittenAgentCall is the answer that carries the call of raw with the prompt the gate gives it,
// written by encode, and whether there is one: it is not when the input cannot be read, when the
// gate leaves the call alone, and when the answer cannot be written. The caller says the same
// thing in each of the three cases, the pass-through, which is why they are one. encode is the
// writing of the answer, a parameter so that its failure, which cannot happen for text, is
// tested instead of read.
func rewrittenAgentCall(raw []byte, cfg gate.Config, encode func(hookwire.AgentCall, string) ([]byte, error)) ([]byte, bool) {
	call, err := hookwire.DecodeAgentCall(raw)
	if err != nil {
		return nil, false
	}
	prompt, changed := gate.Rewrite(gate.Call{SubagentType: call.SubagentType, Prompt: call.Prompt}, cfg)
	if !changed {
		return nil, false
	}
	out, err := encode(call, prompt)
	if err != nil {
		return nil, false
	}
	return out, true
}
