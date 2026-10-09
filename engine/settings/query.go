package settings

// The queries below read a decoded settings.json for the status check, which asks the questions
// the Has and Missing helpers do not: whether the file has a hooks object at all, and whether an
// event holds an entry whose commands mention some words, without the binary path and the family's
// token that identify an entry of a family. They answer from the generic values Parse decodes, so
// a caller that has read the file needs no second parse and no knowledge of the shape of an entry.

// HasHooksObject reports whether root holds an object under "hooks". An empty object counts: it is
// a settings file that has the key and no entries. A key that holds null, an array or any other
// value does not.
func HasHooksObject(root map[string]interface{}) bool {
	hooks, _ := root["hooks"].(map[string]interface{})
	return hooks != nil
}

// HasHookContaining reports whether some entry under the event key holds, in the commands of its
// inner hooks, every one of words. Each word is looked for in any of the entry's commands, so the
// words may be in different inner hooks of one entry but not in different entries. Entries and
// events that are not shaped as Claude Code expects hold nothing.
func HasHookContaining(root map[string]interface{}, event string, words ...string) bool {
	hooks, _ := root["hooks"].(map[string]interface{})
	return hasEntryMatching(hooks, event, func(e interface{}) bool { return entryHoldsWords(e, words) })
}

// HasMatchedHookContaining is HasHookContaining for the entries whose matcher is exactly the
// given string. An entry with no matcher, or one whose matcher is not a string, has none to
// compare and does not count.
func HasMatchedHookContaining(root map[string]interface{}, event, matcher string, words ...string) bool {
	hooks, _ := root["hooks"].(map[string]interface{})
	return hasEntryMatching(hooks, event, func(e interface{}) bool {
		em, ok := e.(map[string]interface{})
		return ok && em["matcher"] == matcher && entryHoldsWords(e, words)
	})
}

// entryHoldsWords reports whether the commands of the entry mention every word.
func entryHoldsWords(e interface{}, words []string) bool {
	for _, word := range words {
		if !entryContainsBinary(e, word) {
			return false
		}
	}
	return true
}
