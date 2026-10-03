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
var gatedEditTools = []string{"Write", "Edit", "MultiEdit", "NotebookEdit"}

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

// gateCall is a tool call of a PreToolUse input as the projection gate reads it, with the
// arguments read as those of a memory query. They are read only for a bound repository, where
// the gate may need them.
func gateCall(in hookwire.PreToolUse) projection.ToolCall {
	q := in.Query()
	return projection.ToolCall{Name: in.Tool, Query: projection.QueryArguments{Named: q.Named, Project: q.Project}}
}

// gateReply is what a decision of the projection gate says to Claude Code.
func gateReply(r projection.GateResult) hookwire.PreToolUseReply {
	return hookwire.PreToolUseReply{Deny: r.Deny, Reason: r.Explanation(), Warning: r.Warning}
}

// promptReply is what a projection says to Claude Code.
func promptReply(r projection.ProjectionResult) hookwire.PromptReply {
	return hookwire.PromptReply{Context: r.Context, Warning: r.Warning}
}

// agentGateAnswer is what 'gate-task' prints for the hook input raw: the call with the prompt
// the gate gives it, or, for everything the gate leaves alone and everything it cannot read, the
// pass-through that leaves the call as it is. It never fails, and never denies.
func agentGateAnswer(raw []byte, cfg gate.Config) []byte {
	call, err := hookwire.DecodeAgentCall(raw)
	if err != nil {
		return hookwire.PassThrough()
	}
	prompt, changed := gate.Rewrite(gate.Call{SubagentType: call.SubagentType, Prompt: call.Prompt}, cfg)
	if !changed {
		return hookwire.PassThrough()
	}
	out, err := call.UpdatedInput(prompt)
	if err != nil {
		return hookwire.PassThrough()
	}
	return out
}
