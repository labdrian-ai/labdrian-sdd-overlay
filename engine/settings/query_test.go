package settings

import "testing"

// A settings object as Claude Code reads it, decoded the way Parse decodes it, with the given
// entries under the events.
func rootWith(events map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"hooks": events}
}

func TestHasHooksObjectNeedsAnObjectUnderHooks(t *testing.T) {
	cases := []struct {
		name string
		root map[string]interface{}
		want bool
	}{
		{"a nil root", nil, false},
		{"no hooks key", map[string]interface{}{}, false},
		{"hooks is null", map[string]interface{}{"hooks": nil}, false},
		{"hooks is an array", map[string]interface{}{"hooks": []interface{}{}}, false},
		{"hooks is a string", map[string]interface{}{"hooks": "x"}, false},
		{"hooks is a nil object", map[string]interface{}{"hooks": map[string]interface{}(nil)}, false},
		{"hooks is an empty object: the key is there, the entries are not", map[string]interface{}{"hooks": map[string]interface{}{}}, true},
		{"hooks holds events", rootWith(map[string]interface{}{"PreToolUse": list()}), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HasHooksObject(c.root); got != c.want {
				t.Errorf("HasHooksObject = %v, want %v", got, c.want)
			}
		})
	}
}

func TestHasHookContainingAsksEveryEntryOfTheEventForEveryWord(t *testing.T) {
	root := rootWith(map[string]interface{}{
		"SessionEnd": list(
			command("echo mine"),
			command("/bin/overlay sync-trigger --event session-end"),
		),
		"UserPromptSubmit": list(command("/bin/overlay propagate", "any matcher at all")),
	})
	cases := []struct {
		name  string
		event string
		words []string
		want  bool
	}{
		{"one word, found in the second entry", "SessionEnd", []string{"overlay"}, true},
		{"two words in the same command", "SessionEnd", []string{"overlay", "sync-trigger"}, true},
		{"two words, one of them nowhere", "SessionEnd", []string{"overlay", "review-receipt"}, false},
		{"a word of the other event", "SessionEnd", []string{"propagate"}, false},
		{"the matcher is not looked at", "UserPromptSubmit", []string{"propagate"}, true},
		{"an event the settings do not have", "PreToolUse", []string{"overlay"}, false},
		{"the word must be a substring, not the whole command", "SessionEnd", []string{"sync-trig"}, true},
		{"the empty word is in every command", "SessionEnd", []string{""}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HasHookContaining(root, c.event, c.words...); got != c.want {
				t.Errorf("HasHookContaining(%s, %q) = %v, want %v", c.event, c.words, got, c.want)
			}
		})
	}
}

func TestHasHookContainingTakesTheWordsFromAnyInnerHookOfOneEntry(t *testing.T) {
	split := map[string]interface{}{"hooks": list(
		map[string]interface{}{"type": "command", "command": "/bin/overlay"},
		map[string]interface{}{"type": "command", "command": "sync-trigger"},
	)}
	root := rootWith(map[string]interface{}{"SessionEnd": list(split)})
	if !HasHookContaining(root, "SessionEnd", "/bin/overlay", "sync-trigger") {
		t.Error("two inner hooks of one entry that hold a word each did not answer for both words")
	}
	apart := rootWith(map[string]interface{}{"SessionEnd": list(command("/bin/overlay"), command("sync-trigger"))})
	if HasHookContaining(apart, "SessionEnd", "/bin/overlay", "sync-trigger") {
		t.Error("two entries that hold a word each answered for both words")
	}
}

func TestHasHookContainingReadsNothingThatIsNotShapedLikeAnEntry(t *testing.T) {
	odd := []map[string]interface{}{
		{},
		{"hooks": nil},
		{"hooks": "x"},
		{"hooks": map[string]interface{}{}},
		{"hooks": []interface{}{}},
		{"hooks": []interface{}{"x", 7, nil, map[string]interface{}{}, map[string]interface{}{"command": 7}}},
	}
	for _, entry := range odd {
		root := rootWith(map[string]interface{}{"SessionEnd": list("x", 1, nil, entry)})
		if HasHookContaining(root, "SessionEnd", "overlay") {
			t.Errorf("an entry %v answered for a word it does not hold", entry)
		}
	}
	for _, events := range []map[string]interface{}{
		{"SessionEnd": "x"}, {"SessionEnd": map[string]interface{}{}}, {"SessionEnd": nil}, {"SessionEnd": 3},
	} {
		if HasHookContaining(rootWith(events), "SessionEnd", "overlay") {
			t.Errorf("an event that is not an array (%v) answered for a word", events)
		}
	}
	if HasHookContaining(nil, "SessionEnd", "overlay") || HasHookContaining(map[string]interface{}{"hooks": "x"}, "SessionEnd", "overlay") {
		t.Error("settings without a hooks object answered for a word")
	}
}

func TestHasMatchedHookContainingNeedsTheMatcherToBeThatStringAndTheWordsToBeThere(t *testing.T) {
	root := rootWith(map[string]interface{}{
		"PreToolUse": list(
			command("/bin/overlay gate-task", "Agent"),
			command("/bin/overlay review-receipt hook", "Bash"),
			command("/bin/overlay other", "agent"),
			command("/bin/overlay numeric", 5),
			command("/bin/overlay none"),
			command("echo mine", "Agent"),
		),
	})
	cases := []struct {
		name    string
		matcher string
		words   []string
		want    bool
	}{
		{"the matcher and the word", "Agent", []string{"gate-task"}, true},
		{"the matcher and both words", "Bash", []string{"/bin/overlay", "review-receipt"}, true},
		{"the matcher, and a word of an entry under another matcher", "Agent", []string{"review-receipt"}, false},
		{"the case of the matcher counts", "agent", []string{"other"}, true},
		{"the case of the matcher counts, the other way", "Agent", []string{"other"}, false},
		{"a matcher that is a number is not the string of it", "5", []string{"numeric"}, false},
		{"an entry with no matcher has none to compare", "", []string{"none"}, false},
		{"a matcher nobody has", "Write", []string{"overlay"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HasMatchedHookContaining(root, "PreToolUse", c.matcher, c.words...); got != c.want {
				t.Errorf("HasMatchedHookContaining(%q, %q) = %v, want %v", c.matcher, c.words, got, c.want)
			}
		})
	}
	if HasMatchedHookContaining(rootWith(map[string]interface{}{"PreToolUse": list("x", 1, nil)}), "PreToolUse", "Agent", "overlay") {
		t.Error("entries that are not objects answered for a matcher")
	}
}
