package hookwire

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ScopedToolCall is a PreToolUse input as the classifier guard reads it: tool_name, cwd (the working directory of
// the agent whose call is judged) and tool_input's command, file_path, notebook_path, path (the directory Grep and
// Glob search) and pattern. A key the input lacks reads as "".
type ScopedToolCall struct {
	Name, Cwd                       string
	Command, FilePath, NotebookPath string
	Path, Pattern                   string
}

var (
	scopedTopKeys  = []string{"tool_name", "cwd", "tool_input"}
	scopedToolKeys = []string{"command", "file_path", "notebook_path", "path", "pattern"}
)

// DecodeScopedToolCall reads a PreToolUse input for the classifier guard, strictly. DecodeToolCall stays lenient
// for the guards that run on every call of every session; encoding/json takes the last of a repeated key, matches
// keys without regard to case (the Kelvin sign included) and turns invalid text into U+FFFD, so the text a guard
// judges could differ from the text the tool acts on. This decoder refuses, with an error that names the key and
// never its value: input over MaxToolCallBytes (wrapping ErrTooLarge); anything but exactly one JSON object; a
// repeated key, at the top level or in tool_input; a key that equals an expected one only without regard to case;
// an expected key whose value is not a JSON string; a tool_input that is not an object; and an expected string
// holding U+FFFD. Every other key is skipped, whatever its value.
func DecodeScopedToolCall(data []byte) (ScopedToolCall, error) {
	if len(data) > MaxToolCallBytes {
		return ScopedToolCall{}, tooLarge("decode scoped tool call", len(data), MaxToolCallBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	var call ScopedToolCall
	err := scanObject(dec, "the input", scopedTopKeys, func(key string) error {
		switch key {
		case "tool_name":
			return stringValue(dec, key, &call.Name)
		case "cwd":
			return stringValue(dec, key, &call.Cwd)
		}
		return scanObject(dec, "tool_input", scopedToolKeys, func(key string) error {
			switch key {
			case "command":
				return stringValue(dec, key, &call.Command)
			case "file_path":
				return stringValue(dec, key, &call.FilePath)
			case "notebook_path":
				return stringValue(dec, key, &call.NotebookPath)
			case "path":
				return stringValue(dec, key, &call.Path)
			}
			return stringValue(dec, key, &call.Pattern)
		})
	})
	if err != nil {
		return ScopedToolCall{}, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return ScopedToolCall{}, errors.New("decode scoped tool call: the input holds more than one JSON value")
	}
	return call, nil
}

// scanObject reads one JSON object from dec: each key is checked, then handle reads the value of an expected key
// and any other value is skipped.
func scanObject(dec *json.Decoder, what string, expected []string, handle func(key string) error) error {
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("decode scoped tool call: %s is not readable JSON: %w", what, err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return fmt.Errorf("decode scoped tool call: %s is not a JSON object", what)
	}
	seen := map[string]bool{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("decode scoped tool call: %s is not readable JSON: %w", what, err)
		}
		key, _ := tok.(string)
		if seen[key] {
			return fmt.Errorf("decode scoped tool call: the key %q appears twice in %s", key, what)
		}
		seen[key] = true
		isExpected := false
		for _, e := range expected {
			if key == e {
				isExpected = true
			} else if strings.EqualFold(key, e) {
				return fmt.Errorf("decode scoped tool call: the key %q in %s differs from %q only in case", key, what, e)
			}
		}
		if !isExpected {
			var skipped json.RawMessage
			if err := dec.Decode(&skipped); err != nil {
				return fmt.Errorf("decode scoped tool call: the value of %q in %s is not readable JSON: %w", key, what, err)
			}
			continue
		}
		if err := handle(key); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("decode scoped tool call: %s is not readable JSON: %w", what, err)
	}
	return nil
}

// stringValue reads the next value as a JSON string into dst; another type, or U+FFFD in it (what invalid UTF-8
// and a lone surrogate decode to), is refused.
func stringValue(dec *json.Decoder, key string, dst *string) error {
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("decode scoped tool call: the value of %q is not readable JSON: %w", key, err)
	}
	s, ok := tok.(string)
	if !ok {
		return fmt.Errorf("decode scoped tool call: the value of %q is not a JSON string", key)
	}
	if strings.ContainsRune(s, '�') {
		return fmt.Errorf("decode scoped tool call: the value of %q holds U+FFFD (invalid text)", key)
	}
	*dst = s
	return nil
}
