package hookwire

import (
	"bytes"
	"encoding/json"
	"strings"
)

// PromptText is the text of the prompt of a UserPromptSubmit hook's stdin: the value of "prompt" when
// the input is one JSON object no larger than MaxEnvelopeBytes whose "prompt" is text. Anything else,
// an input over the bound included, is "no prompt" (false), never an error: the caller is a hook that
// must not stop because it could not read the input. The text is returned as it is, so a
// caller that asks IsTaskNotification sees the characters the model sees and not their JSON escapes.
func PromptText(data []byte) (string, bool) {
	if len(data) > MaxEnvelopeBytes {
		return "", false
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return "", false
	}
	var wire struct {
		Prompt json.RawMessage `json:"prompt"`
	}
	if err := json.Unmarshal(trimmed, &wire); err != nil {
		return "", false
	}
	if len(wire.Prompt) == 0 || wire.Prompt[0] != '"' {
		return "", false
	}
	var text string
	if err := json.Unmarshal(wire.Prompt, &text); err != nil {
		return "", false
	}
	return text, true
}

const (
	// taskNotificationTag opens the prompt Claude Code hands a session when a background task ends.
	taskNotificationTag = "<task-notification>"
	// systemReminderTag opens the wrapper Claude Code puts around an automated event.
	systemReminderTag = "<system-reminder>"
	// systemNotificationMark is the line that heads such a wrapper; the text says it is not user input.
	systemNotificationMark = "[SYSTEM NOTIFICATION - NOT USER INPUT]"
)

// IsTaskNotification reports whether a prompt is an automated notification and not something the user
// wrote: it starts with the task-notification tag, or it starts with the system-reminder tag and the mark
// is the first non-empty line after it. Both are read at the head only, so a prompt that merely quotes
// either one is not skipped.
func IsTaskNotification(prompt string) bool {
	if strings.HasPrefix(prompt, taskNotificationTag) {
		return true
	}
	body, wrapped := strings.CutPrefix(prompt, systemReminderTag)
	if !wrapped {
		return false
	}
	for _, line := range strings.Split(body, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line == systemNotificationMark
		}
	}
	return false
}
