package settings

import "strings"

// The recognizers below are the one place that decides whether a hook command mentions a word,
// so every family, status helper, query and the Codex document read an entry the same way.

// CommandOwnedBy reports whether one hook command is ours: it mentions the binary, bare or
// shell-quoted, and the family's identity token. An empty binary path is mentioned by no command,
// so it owns nothing.
func CommandOwnedBy(command, binary, identity string) bool {
	return commandMentions(command, binary) && commandMentions(command, identity)
}

// commandMentions reports whether command holds word as a substring, bare (as entries written
// before the commands were quoted have it) or shell-quoted (as they are written now). An empty
// word, which every string contains, is mentioned by no command: a caller handed an empty binary
// path must find nothing, not answer on the family's token alone.
func commandMentions(command, word string) bool {
	if word == "" {
		return false
	}
	return strings.Contains(command, word) || strings.Contains(command, shellQuote(word))
}

// entryMentions reports whether any of the inner hooks[].command strings of a hook entry (an
// outer object) mentions word. An entry not shaped as Claude Code expects mentions nothing.
func entryMentions(e interface{}, word string) bool {
	em, ok := e.(map[string]interface{})
	if !ok {
		return false
	}
	innerHooks, ok := em["hooks"].([]interface{})
	if !ok {
		return false
	}
	for _, ih := range innerHooks {
		ihm, ok := ih.(map[string]interface{})
		if !ok {
			continue
		}
		if cmdStr, ok := ihm["command"].(string); ok && commandMentions(cmdStr, word) {
			return true
		}
	}
	return false
}
