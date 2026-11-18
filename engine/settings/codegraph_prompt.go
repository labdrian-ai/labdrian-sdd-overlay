package settings

import (
	"fmt"
	"strings"
)

// The codegraph 1.2.0 installer (/codegraph) writes a UserPromptSubmit hook whose command is exactly
// `codegraph prompt-hook`. That command has no option to skip task notifications, which are most of the
// prompts of a long session and add nothing to codegraph's exploration (S19, L87), so Merge replaces it,
// in place, with a labdrian wrapper that passes everything else to codegraph unchanged, and Remove writes
// the bare command back. The entry belongs to codegraph, not to the overlay: it is not a hookFamily (a
// family owns whole entries by the overlay's binary and an identity token and appends the ones that are
// missing); this step changes one exact command string and changes it back, and touches nothing else.
// A command that merely contains the text, such as the wrapper itself, is never matched. The wrapper is
// recognised by its shape whatever engine path it names: an old wrapper is rewritten in place to the current
// one, restored by uninstall, and read as wrapped by status.

const (
	// codegraphPromptEvent is the event the entry sits under.
	codegraphPromptEvent = "UserPromptSubmit"
	// codegraphPromptBare is the command the codegraph installer writes.
	codegraphPromptBare = "codegraph prompt-hook"
)

// codegraphPromptWrapper is the command that runs codegraph through the engine at hookCommand. When the
// engine is not there it runs codegraph directly, so a missing binary never silences codegraph.
func codegraphPromptWrapper(hookCommand string) string {
	bin := shellQuote(hookCommand)
	return "command -v " + bin + " >/dev/null 2>&1 || exec " + codegraphPromptBare + "; exec " + bin + " codegraph-prompt-hook"
}

// isCodegraphPromptWrapper reports whether cmd has the shape of the wrapper for some engine path:
// `command -v P >/dev/null 2>&1 || exec codegraph prompt-hook; exec P codegraph-prompt-hook`, with the same
// shell word P in both places. It parses the word, so it does not depend on the path of the current engine:
// the path can change between installs, and an old wrapper must still be found.
func isCodegraphPromptWrapper(cmd string) bool {
	rest, ok := strings.CutPrefix(cmd, "command -v ")
	if !ok {
		return false
	}
	word, rest := shellWord(rest)
	return word != "" && rest == " >/dev/null 2>&1 || exec "+codegraphPromptBare+"; exec "+word+" codegraph-prompt-hook"
}

// shellWord splits from the front of s the one shell word shellQuote writes: a run of safe characters, or a
// single-quoted string in which an embedded quote is written quote, backslash, quote, quote. It returns the
// word as written and the rest, and an empty word when s does not begin with one.
func shellWord(s string) (word, rest string) {
	if s == "" {
		return "", s
	}
	if s[0] != '\'' {
		i := 0
		for i < len(s) && isShellSafe(rune(s[i])) {
			i++
		}
		return s[:i], s[i:]
	}
	for i := 1; i < len(s); i++ {
		if s[i] != '\'' {
			continue
		}
		if strings.HasPrefix(s[i:], `'\''`) {
			i += 3
			continue
		}
		return s[:i+1], s[i+1:]
	}
	return "", s
}

// commandOf is the command of a hook object, empty when it has none.
func commandOf(h map[string]interface{}) string {
	cmd, _ := h["command"].(string)
	return cmd
}

// CodegraphPromptState is what the settings say about the codegraph prompt hook.
type CodegraphPromptState int

const (
	// CodegraphPromptAbsent: no hook object runs the bare command or the wrapper.
	CodegraphPromptAbsent CodegraphPromptState = iota
	// CodegraphPromptWrapped: the wrapper is there and no bare command runs beside it.
	CodegraphPromptWrapped
	// CodegraphPromptBare: a hook object runs the bare command, so codegraph explores every notification.
	CodegraphPromptBare
)

// CodegraphPrompt reports the state of the codegraph prompt hook in root. A wrapper of any engine path
// counts as wrapped. A bare command anywhere wins: it is what a codegraph reinstall leaves, and it is what
// is to be fixed.
func CodegraphPrompt(root map[string]interface{}) CodegraphPromptState {
	state := CodegraphPromptAbsent
	for _, h := range promptHookObjects(root) {
		cmd := commandOf(h)
		switch {
		case cmd == codegraphPromptBare:
			return CodegraphPromptBare
		case isCodegraphPromptWrapper(cmd):
			state = CodegraphPromptWrapped
		}
	}
	return state
}

// CodegraphPromptReplacedNotice is the sentence an install says when it replaced the bare codegraph
// command in the settings file at location.
func CodegraphPromptReplacedNotice(location string) string {
	return fmt.Sprintf("%s ran %q on every prompt; install-hooks replaced it with a wrapper that skips task notifications and runs it for every other prompt (a codegraph reinstall puts the bare command back: run install-hooks again; uninstall-hooks restores it)",
		location, codegraphPromptBare)
}

// promptHookObjects are the hook objects of every UserPromptSubmit entry of root, in file order.
func promptHookObjects(root map[string]interface{}) []map[string]interface{} {
	hooks, _ := root["hooks"].(map[string]interface{})
	entries, _ := hooks[codegraphPromptEvent].([]interface{})
	var out []map[string]interface{}
	for _, e := range entries {
		em, _ := e.(map[string]interface{})
		inner, _ := em["hooks"].([]interface{})
		for _, h := range inner {
			if hm, ok := h.(map[string]interface{}); ok {
				out = append(out, hm)
			}
		}
	}
	return out
}

// wrapCodegraphPrompt puts the wrapper in place of the first bare command, keeping every other field of its
// hook object and entry, and takes out any other bare command, so that codegraph runs once per prompt:
// a bare command beside a wrapper is removed whether the wrapper was there or has just been put in place.
// It reports whether it changed anything.
func wrapCodegraphPrompt(hooks map[string]interface{}, hookCommand string) bool {
	wrapper := codegraphPromptWrapper(hookCommand)
	return rewriteCodegraphPrompt(hooks, func(cmd string) bool {
		return cmd == codegraphPromptBare || (cmd != wrapper && isCodegraphPromptWrapper(cmd))
	}, wrapper)
}

// restoreCodegraphPrompt writes the bare command back in place of a wrapper of any engine path, and reports
// whether it changed anything.
func restoreCodegraphPrompt(hooks map[string]interface{}) bool {
	return rewriteCodegraphPrompt(hooks, isCodegraphPromptWrapper, codegraphPromptBare)
}

// rewriteCodegraphPrompt replaces a command that isFrom accepts with to in the first hook object that holds
// one, in place, and removes every other hook object that holds one while a hook object that holds to stands,
// so the list ends with one command to and none that isFrom accepts. An entry left with no hook object is
// removed.
func rewriteCodegraphPrompt(hooks map[string]interface{}, isFrom func(string) bool, to string) bool {
	entries, ok := hooks[codegraphPromptEvent].([]interface{})
	if !ok {
		return false
	}
	kept := false // a hook object that holds to already stands in the list
	for _, h := range promptHookObjects(map[string]interface{}{"hooks": hooks}) {
		if h["command"] == to {
			kept = true
		}
	}
	changed := false
	var outEntries []interface{}
	for _, e := range entries {
		em, isMap := e.(map[string]interface{})
		inner, hasInner := em["hooks"].([]interface{})
		if !isMap || !hasInner {
			outEntries = append(outEntries, e)
			continue
		}
		var outInner []interface{}
		dropped := false
		for _, h := range inner {
			hm, isHook := h.(map[string]interface{})
			if !isHook || !isFrom(commandOf(hm)) {
				outInner = append(outInner, h)
				continue
			}
			changed = true
			if kept {
				dropped = true
				continue
			}
			hm["command"] = to
			kept = true
			outInner = append(outInner, hm)
		}
		switch {
		case !dropped:
			outEntries = append(outEntries, e)
		case len(outInner) > 0:
			em["hooks"] = outInner
			outEntries = append(outEntries, em)
		}
	}
	if changed {
		hooks[codegraphPromptEvent] = outEntries
	}
	return changed
}
