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
// The package imports nothing of the module. Its values are its own, and engine/cmd maps them
// to and from the values of each policy.
package hookwire

import "errors"

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

// ErrTooLarge is wrapped by the error of a decoder that was given more than MaxEnvelopeBytes.
var ErrTooLarge = errors.New("hookwire: hook input exceeds the maximum size")
