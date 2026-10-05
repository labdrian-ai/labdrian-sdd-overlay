package hookwire

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// --- what the guards read ---------------------------------------------------------------

// ToolCall is a PreToolUse input as the hooks that guard a tool call read it: the tool's name,
// the command a shell tool is about to run, and the paths the file tools are about to write.
// What a guard decides from is the text of these; it reads no other field.
type ToolCall struct {
	// Name is tool_name, or empty when the input has none.
	Name string
	// Command is tool_input.command: the command of a shell tool.
	Command string
	// FilePath is tool_input.file_path: the path of Write, Edit and MultiEdit.
	FilePath string
	// NotebookPath is tool_input.notebook_path: the path of NotebookEdit.
	NotebookPath string
}

// guardHookInput is the view of a PreToolUse input the guards decode: the tool's name and the
// three fields of its input that a guard reads. A field of these of the wrong type, and a
// tool_input that is not an object, fail the decoding; every other field is ignored, of
// whatever type.
//
// The shape of tool_input (the order of its fields and their tags) is part of what the decoder
// says when it fails, and that is part of what the model reads in a denial
// (ClearanceGuardDetail), so it is not to be changed while that wording is kept. The type's
// name is not: the denial names it by constants, found in the decoder's words through the type
// itself.
type guardHookInput struct {
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		Command      string `json:"command"`
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
	} `json:"tool_input"`
}

// DecodeToolCall reads a PreToolUse input for a guard. It is the decoding of encoding/json, so it
// takes one JSON value (null is an input with nothing in it), matches a key without regard to
// case, and takes the last of a repeated key. The input is at most MaxToolCallBytes: a longer one
// is refused, wrapping ErrTooLarge, whatever it holds, and what to do about a call that was not
// judged is the guard's to answer (both guards deny it). It fails for text that is not JSON, for a value that is not an object, for a tool_name,
// command, file_path or notebook_path that is not text and for a tool_input that is not an
// object.
//
// The error of a value that could not be decoded wraps the one of the JSON decoder, in its
// words. The clearance guard fails closed and shows the model why; the words it has always
// shown are ClearanceGuardDetail's.
func DecodeToolCall(data []byte) (ToolCall, error) {
	if len(data) > MaxToolCallBytes {
		return ToolCall{}, tooLarge("decode tool call", len(data), MaxToolCallBytes)
	}
	var in guardHookInput
	if err := json.Unmarshal(data, &in); err != nil {
		return ToolCall{}, &toolCallError{cause: err}
	}
	return ToolCall{Name: in.ToolName, Command: in.ToolInput.Command, FilePath: in.ToolInput.FilePath, NotebookPath: in.ToolInput.NotebookPath}, nil
}

// toolCallError is the failure of DecodeToolCall to decode its input as JSON.
type toolCallError struct{ cause error }

func (e *toolCallError) Error() string { return "decode tool call: " + e.cause.Error() }
func (e *toolCallError) Unwrap() error { return e.cause }

// The wording of the clearance guard's denial for an input it could not decode is the decoder's
// own, as it has always been, and it names the type that was decoded into as it was named
// when that type was the guard's own, in engine/shaper: "shaper.guardHookInput" for a value
// that is not an object, and "guardHookInput" for a field of one. Both are written here and
// nowhere else, and are found in the decoder's words by the type itself (guardViewType), not
// by a copy of its name, so renaming the type does not change a byte of what the guard says.
const (
	legacyViewPackage = "shaper"
	legacyViewName    = "guardHookInput"
)

// guardViewType is the type DecodeToolCall decodes into, whose name encoding/json puts in the
// error it returns.
var guardViewType = reflect.TypeOf(guardHookInput{})

// ClearanceGuardDetail is the text of the error err of DecodeToolCall as the clearance guard has
// always printed it in a denial: the JSON decoder's own words, with the name of the type that was
// decoded into as it was before the hook format had one home. It is for that guard alone, which
// is the only reader of the error that shows it. An error that is not the decoder's, such as the
// refusal of an input that is too large, is told in its own words.
//
// Deleting this function and printing err.Error() in its place is a change to the text of a
// denial, and an owner's to make.
func ClearanceGuardDetail(err error) string {
	var failure *toolCallError
	if !errors.As(err, &failure) {
		return err.Error()
	}
	text := strings.Replace(failure.cause.Error(), guardViewType.String(), legacyViewPackage+"."+legacyViewName, 1)
	return strings.Replace(text, "struct field "+guardViewType.Name()+".", "struct field "+legacyViewName+".", 1)
}

// commandHookInput is the view of a PreToolUse input the review-receipt hook decodes: the tool's
// name, which it does not use, and the command. It reads no other field of tool_input, so a
// file_path of the wrong type beside the command is not an error here as it is for a guard: the
// receipts an acknowledgement burns must be captured whatever else the call carries.
type commandHookInput struct {
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		Command string `json:"command"`
	} `json:"tool_input"`
}

// DecodeCommand reads the command of a PreToolUse input, whatever the tool: a tool_input with no
// command, or a null one, has the empty command. It decodes like DecodeToolCall (one JSON value,
// keys without regard to case, no bound on the size) and fails for text that is not JSON, for a
// value that is not an object, for a tool_name or a command that is not text and for a
// tool_input that is not an object.
func DecodeCommand(data []byte) (string, error) {
	var in commandHookInput
	if err := json.Unmarshal(data, &in); err != nil {
		return "", fmt.Errorf("decode command: %w", err)
	}
	return in.ToolInput.Command, nil
}

// --- answering by exit status -------------------------------------------------------------

// The exit statuses of a hook that answers by them.
const (
	// ExitAllow lets the call go ahead.
	ExitAllow = 0
	// ExitBlock blocks it: Claude Code reads exit status 2 as a denial, and feeds what the hook
	// wrote to stderr to the model.
	ExitBlock = 2
)

// ExitReply is what a hook that answers by exit status tells Claude Code: whether to block the
// call, and the message that says why.
type ExitReply struct {
	Block   bool
	Message string
}

// Code is the exit status of the hook.
func (r ExitReply) Code() int {
	if r.Block {
		return ExitBlock
	}
	return ExitAllow
}

// Stderr is what the hook writes to stderr: the message and a line break, or nothing for no
// message.
func (r ExitReply) Stderr() []byte {
	if r.Message == "" {
		return nil
	}
	return []byte(r.Message + "\n")
}
