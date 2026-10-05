// Package hookwire is the Claude Code hook protocol: the JSON a hook is handed on stdin and the
// JSON, or the exit status, it answers with. It is an adapter. The policies that decide what a
// hook says (engine/projection, engine/gate, engine/skills, engine/shaper, engine/reviewreceipt)
// take and return plain values and know nothing of this format; engine/cmd reads stdin, asks
// hookwire to decode it, hands the policy its own values, and asks hookwire to encode the
// answer. A runtime with another hook format needs another adapter and no change to a policy.
//
// Every name of the wire, which is every JSON tag of the hook format, is written here and
// nowhere else (TestNoHookWireFormatOutsideHookwire reads the module to check).
//
// There is a decoder for each thing a hook needs to read, and each reads exactly the fields its
// consumer has always read and no others, with the type it has always expected. That is
// deliberate and is part of the contract: a field a decoder does not read may be of any type,
// one it reads must be of the type it expects, and what is refused is refused as it always was,
// because each hook decides for itself what to do with an input it cannot use (one lets the call
// go through, one denies it) and the hooks' golden files pin it. A single decoder that read the
// union of the fields would refuse inputs one of the hooks accepts, such as a Bash call with a
// file_path of the wrong type beside the command of an acknowledgement the receipt hook guards.
//
// The decoders do not all match a key the same way, because each keeps the rule of the code it
// replaced and the golden files of the hooks pin it. DecodeAgentCall, DecodeToolCall and
// DecodeCommand decode through encoding/json's struct tags, which match a key without regard to
// case and take the last of a repeated key. PreToolUse.Query reads the arguments of a memory
// query through a map, so it matches "project" exactly (a key in another case is another key,
// which names no project) and also takes the last of a repeated key. Each is pinned in its own
// test, and a decoder is not changed to agree with another without an owner's decision.
//
// Each decoder also bounds its input (MaxEnvelopeBytes for the two events, MaxToolCallBytes
// for the guards, MaxAgentCallBytes for the Agent tool), refusing a larger one with an error
// wrapping ErrTooLarge. DecodeCommand, the review-receipt hook's, is the exception: the hook
// must see the command of an acknowledgement whatever else the call carries, and what it
// should do with an input it cannot bear to read is an owner's decision.
//
// The package imports nothing of the module. Its values are its own, and engine/cmd maps them
// to and from the values of each policy.
package hookwire

import (
	"errors"
	"fmt"
)

// The hook events the engine answers.
const (
	// EventPreToolUse runs before a tool call and can deny it or rewrite its input.
	EventPreToolUse = "PreToolUse"
	// EventUserPromptSubmit runs when the user submits a prompt; what it prints becomes part of
	// the session's context.
	EventUserPromptSubmit = "UserPromptSubmit"
)

// MaxEnvelopeBytes bounds the input DecodeUserPromptSubmit and DecodePreToolUse accept. A
// prompt can be long, and Claude Code sends it in the input, so the bound is generous; it
// exists so that a hostile or runaway writer cannot make the hook read without end. A caller
// that reads stdin reads one byte more than this, which is enough to see the bound was passed.
const MaxEnvelopeBytes = 1 << 20

// MaxToolCallBytes bounds the input DecodeToolCall accepts. The guards that read it run on every
// Bash and file-edit call, and the input of a call carries what it writes, so the bound is
// generous; it exists so that a hostile or runaway writer cannot make a guard read without end.
// A caller that reads stdin reads one byte more than this, which is enough to see the bound was
// passed. What a guard does with a call it was not given to judge is its own decision: the
// approve guard fails open, the clearance guard fails closed.
const MaxToolCallBytes = 8 << 20

// MaxAgentCallBytes bounds the input DecodeAgentCall accepts. The prompt of a sub-agent can be
// long. 'gate-task' reads exactly this many bytes, so an input over the bound reaches the decoder
// cut short, which is not valid JSON, and is let through like every input it cannot read.
const MaxAgentCallBytes = 4 << 20

// ErrTooLarge is wrapped by the error of a decoder that was given more than its bound
// (MaxEnvelopeBytes, MaxToolCallBytes, MaxAgentCallBytes).
var ErrTooLarge = errors.New("hookwire: hook input exceeds the maximum size")

// tooLarge is the refusal of a decoder, named by what it was decoding, for an input of size
// bytes over its bound of limit.
func tooLarge(what string, size, limit int) error {
	return fmt.Errorf("%s: %w: %d bytes exceeds the maximum of %d", what, ErrTooLarge, size, limit)
}
