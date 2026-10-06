package hookwire

import (
	"encoding/json"
	"fmt"
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
// is refused, wrapping ErrTooLarge, whatever it holds. What to do about a call that was not judged
// is the guard's to answer, and both guards deny it. It fails for text that is not JSON, for a
// value that is not an object, for a tool_name, command, file_path or notebook_path that is not
// text and for a tool_input that is not an object.
//
// The error of a value that could not be decoded wraps the one of the JSON decoder, in its words.
// Those words are for logs and tests, never for the model: a guard that denies such a call writes
// the denial itself, so that what the model reads does not depend on the decoder or on the type it
// decodes into.
func DecodeToolCall(data []byte) (ToolCall, error) {
	if len(data) > MaxToolCallBytes {
		return ToolCall{}, tooLarge("decode tool call", len(data), MaxToolCallBytes)
	}
	var in guardHookInput
	if err := json.Unmarshal(data, &in); err != nil {
		return ToolCall{}, fmt.Errorf("decode tool call: %w", err)
	}
	return ToolCall{Name: in.ToolName, Command: in.ToolInput.Command, FilePath: in.ToolInput.FilePath, NotebookPath: in.ToolInput.NotebookPath}, nil
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
