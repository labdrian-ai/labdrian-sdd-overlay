package hookwire

// --- the Stop event ---------------------------------------------------------------------------

// Stop is what is kept of the JSON a Stop hook receives: the event's name, the working directory of the session, the
// session and whether a stop hook is already making the agent continue. Nothing else is read, in particular not the
// transcript.
type Stop struct {
	// Event is hook_event_name, or empty when the input has none.
	Event string
	// Cwd is the session's working directory as a cleaned absolute path, or empty when the input has none or it is
	// not an absolute path.
	Cwd string
	// SessionID is session_id, or empty when the input has none.
	SessionID string
	// Continuing is stop_hook_active: true while the agent is continuing because a stop hook blocked its stop. A
	// hook that blocks again then keeps the agent from ever stopping, so Claude Code hands the flag to the hook.
	Continuing bool
}

// DecodeStop reads a Stop hook's stdin by the rules DecodeUserPromptSubmit reads its own: one JSON object no larger
// than MaxEnvelopeBytes, the fields it does not name ignored, a relative cwd read as missing, and a field it reads of
// the wrong type an error.
func DecodeStop(data []byte) (Stop, error) {
	var wire struct {
		HookEventName  string `json:"hook_event_name"`
		Cwd            string `json:"cwd"`
		SessionID      string `json:"session_id"`
		StopHookActive bool   `json:"stop_hook_active"`
	}
	if err := decodeEnvelope(data, &wire); err != nil {
		return Stop{}, err
	}
	return Stop{Event: wire.HookEventName, Cwd: cleanCwd(wire.Cwd), SessionID: wire.SessionID, Continuing: wire.StopHookActive}, nil
}

// StopFallbackReason is the reason of a block that has none, so a block never reaches the agent without a word.
const StopFallbackReason = "a stop hook blocked this stop."

// StopReply is what a Stop hook tells Claude Code: to block the stop, with the reason the agent reads, or to let it
// happen.
type StopReply struct {
	Block  bool
	Reason string
}

// stopOutput is the JSON object a Stop hook prints to block: the decision and its reason at the top level, and the
// hook exits 0 (the JSON decides).
type stopOutput struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// Encode renders r as the one JSON object a Stop hook writes to stdout, followed by a line break:
// {"decision":"block","reason":"..."}. A reply that does not block yields no output at all, which Claude Code reads
// as "let the agent stop". The JSON keeps <, > and & as they are.
func (r StopReply) Encode() ([]byte, error) {
	if !r.Block {
		return nil, nil
	}
	reason := r.Reason
	if reason == "" {
		reason = StopFallbackReason
	}
	return encodeLine(stopOutput{Decision: "block", Reason: reason})
}
