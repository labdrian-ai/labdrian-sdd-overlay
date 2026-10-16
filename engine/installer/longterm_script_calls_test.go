package installer_test

// The calls the overlay script makes to the engine's installed-targets command, found in its text
// (longterm_engine_calls_test.go runs the script; this reads it). A call can sit on a continuation line
// (`longtermmem_targets retire \` and the flags on the next), after a `;`, a `&&`, a `||` or a `|`, or
// hand its flags over in an array (`retire_args+=(--purge)`), and a check that looks at one line of text
// for one shape of call misses the rest. So the text is read the way the shell reads it as far as this
// needs: continuation lines joined, comments dropped, the statements split at the separators outside
// quotes, the words of each statement taken apart, and the arrays a call expands read from their
// assignments. It does not read command substitutions, heredocs or functions that wrap the helper, and
// the script has none around these calls; a test below fails if a call is hidden in one.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// engineCall is a call of the helper longtermmem_targets: the verb and the flags it hands over, the
// flags of the arrays it expands included.
type engineCall struct {
	verb  string
	flags []string
}

// scriptWords reads the text of a script the way the shell splits it, as far as finding calls needs:
// the statements, each a list of words. A backslash at the end of a line joins it to the next, a whole
// word that begins with # is a comment to the end of the line, and a newline, `;`, `&` (`&&`) or `|`
// (`||`) outside quotes ends a statement. Quotes are taken off the words they enclose.
func scriptWords(script string) [][]string {
	text := strings.ReplaceAll(script, "\\\n", " ")
	var statements [][]string
	var words []string
	var word strings.Builder
	started := false // a word is being read, even if it is empty (`""`)
	inSingle, inDouble := false, false
	endWord := func() {
		if started {
			words = append(words, word.String())
		}
		word.Reset()
		started = false
	}
	endStatement := func() {
		endWord()
		if len(words) > 0 {
			statements = append(statements, words)
		}
		words = nil
	}
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
			} else {
				word.WriteByte(c)
			}
		case inDouble:
			switch {
			case c == '"':
				inDouble = false
			case c == '\\' && i+1 < len(text):
				word.WriteByte(c)
				i++
				word.WriteByte(text[i])
			default:
				word.WriteByte(c)
			}
		case c == '\'':
			inSingle, started = true, true
		case c == '"':
			inDouble, started = true, true
		case c == '\\' && i+1 < len(text):
			word.WriteByte(c)
			i++
			word.WriteByte(text[i])
			started = true
		case c == '#' && !started:
			for i < len(text) && text[i] != '\n' {
				i++
			}
			endStatement()
		case c == '\n' || c == ';' || c == '&' || c == '|':
			endStatement()
		case c == ' ' || c == '\t':
			endWord()
		default:
			word.WriteByte(c)
			started = true
		}
	}
	endStatement()
	return statements
}

var (
	flagWord      = regexp.MustCompile(`^--[a-z][a-z-]*$`)
	arrayAssigned = regexp.MustCompile(`(\w+)\+?=\(([^)]*)\)`)
	arrayExpanded = regexp.MustCompile(`\$\{(\w+)\[@\]`)
)

// arrayFlags maps the name of each array the script assigns to the flags it puts in it, wherever it
// assigns it (`name=(...)` and `name+=(...)` alike).
func arrayFlags(script string) map[string][]string {
	flags := map[string][]string{}
	var kept []string
	for _, line := range strings.Split(strings.ReplaceAll(script, "\\\n", " "), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			kept = append(kept, line)
		}
	}
	text := strings.Join(kept, "\n")
	for _, m := range arrayAssigned.FindAllStringSubmatch(text, -1) {
		for _, word := range strings.Fields(m[2]) {
			word = strings.Trim(word, `"'`)
			if flagWord.MatchString(word) {
				flags[m[1]] = append(flags[m[1]], word)
			}
		}
	}
	return flags
}

// engineCalls finds every call of longtermmem_targets in the text of a script.
func engineCalls(script string) []engineCall {
	arrays := arrayFlags(script)
	var calls []engineCall
	for _, statement := range scriptWords(script) {
		for i, word := range statement {
			if word != "longtermmem_targets" || i+1 >= len(statement) {
				continue
			}
			call := engineCall{verb: statement[i+1]}
			for _, arg := range statement[i+2:] {
				if flagWord.MatchString(arg) {
					call.flags = append(call.flags, arg)
				}
				expanded := map[string]bool{}
				for _, m := range arrayExpanded.FindAllStringSubmatch(arg, -1) {
					if !expanded[m[1]] {
						expanded[m[1]] = true
						call.flags = append(call.flags, arrays[m[1]]...)
					}
				}
			}
			calls = append(calls, call)
		}
	}
	return calls
}

// takenByTheCommand is what the command takes: each verb the script may call and the flags of each.
// The list is the one the engine holds (engine/cmd/installed_targets.go), without --state-dir, which
// the helper puts in front of every call; list is a verb of the command the script does not call.
var takenByTheCommand = map[string]map[string]bool{
	"record":  {"--target": true},
	"release": {"--target": true},
	"assess":  {"--purge": true, "--unregister-failed": true},
	"retire":  {"--binary": true, "--purge": true},
}

// problemsWithEngineCalls says what is wrong with the calls a script makes: a verb the command does not
// take, a flag the verb does not take, and a verb of the four the script never calls.
func problemsWithEngineCalls(script string) []string {
	var problems []string
	seen := map[string]bool{}
	for _, call := range engineCalls(script) {
		allowed, ok := takenByTheCommand[call.verb]
		if !ok {
			problems = append(problems, "the script calls the verb "+call.verb+", which the command does not take")
			continue
		}
		seen[call.verb] = true
		for _, flag := range call.flags {
			if !allowed[flag] {
				problems = append(problems, "the script sends "+flag+" to "+call.verb+", which does not take it")
			}
		}
	}
	for verb := range takenByTheCommand {
		if !seen[verb] {
			problems = append(problems, "the script never calls "+verb)
		}
	}
	sort.Strings(problems)
	return problems
}

func TestScriptEngineCallsAreFoundInEveryShapeTheShellCanWriteThem(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
		want   []engineCall
	}{
		{"one line, with a failure arm", `longtermmem_targets record --target "$t" || exit 1`,
			[]engineCall{{"record", []string{"--target"}}}},
		{"flags on a continuation line", "longtermmem_targets retire \\\n    --binary \"$B\" --bogus || exit 1",
			[]engineCall{{"retire", []string{"--binary", "--bogus"}}}},
		{"after a semicolon", `true; longtermmem_targets release --target "$t" --bogus`,
			[]engineCall{{"release", []string{"--target", "--bogus"}}}},
		{"after &&", `[[ -n "$t" ]] && longtermmem_targets release --target "$t" --bogus`,
			[]engineCall{{"release", []string{"--target", "--bogus"}}}},
		{"after ||", `false || longtermmem_targets assess --bogus`,
			[]engineCall{{"assess", []string{"--bogus"}}}},
		{"after a pipe", `echo x | longtermmem_targets assess --bogus`,
			[]engineCall{{"assess", []string{"--bogus"}}}},
		{"in the condition of an if", `if ! longtermmem_targets assess --bogus; then echo no; fi`,
			[]engineCall{{"assess", []string{"--bogus"}}}},
		{"two calls on one line", `longtermmem_targets release --target a; longtermmem_targets release --target b --bogus`,
			[]engineCall{{"release", []string{"--target"}}, {"release", []string{"--target", "--bogus"}}}},
		{"flags in an array, set and added to", "args=(--purge)\nargs+=(--bogus)\nlongtermmem_targets assess \"${args[@]}\"",
			[]engineCall{{"assess", []string{"--purge", "--bogus"}}}},
		{"flags in an array, expanded the portable way", "args=()\nargs+=(--bogus)\nlongtermmem_targets assess ${args[@]+\"${args[@]}\"} || rc=$?",
			[]engineCall{{"assess", []string{"--bogus"}}}},
		{"a semicolon inside quotes does not end the call", `longtermmem_targets record --target "a;b" --bogus`,
			[]engineCall{{"record", []string{"--target", "--bogus"}}}},
		{"a call named in a comment is not a call", "# longtermmem_targets <verb> [args...] runs a verb\n  # longtermmem_targets record --bogus\ntrue # longtermmem_targets record --bogus",
			nil},
		{"a flag in a quoted word is a word of the call all the same", `longtermmem_targets record "--bogus"`,
			[]engineCall{{"record", []string{"--bogus"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := engineCalls(tc.script)
			if len(got) != len(tc.want) {
				t.Fatalf("calls = %+v, want %+v", got, tc.want)
			}
			for i := range got {
				if got[i].verb != tc.want[i].verb || strings.Join(got[i].flags, " ") != strings.Join(tc.want[i].flags, " ") {
					t.Errorf("call %d = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestTheScriptSendsOnlyTheVerbsAndFlagsTheCommandTakes(t *testing.T) {
	script, err := os.ReadFile(overlaytest.Script(t))
	if err != nil {
		t.Fatal(err)
	}
	if problems := problemsWithEngineCalls(string(script)); len(problems) != 0 {
		t.Errorf("the calls of the script to the command:\n%s", strings.Join(problems, "\n"))
	}
	// The helper hands the state directory over once, in front of the arguments.
	if !strings.Contains(string(script), `"$ENGINE_BINARY" installed-targets "$verb" --state-dir "$STATE_DIR" "$@"`) {
		t.Error("the helper does not call the engine with the state directory in front of the arguments")
	}
	// The engine is called through the helper and nowhere else: a call written out would escape the check. The
	// one call that is not the helper's is the probe of an engine that may be too old, which asks the bare
	// command (it is one word of a command substitution, not a statement of its own).
	const helper = "$ENGINE_BINARY installed-targets $verb --state-dir $STATE_DIR $@"
	for _, statement := range scriptWords(string(script)) {
		for i, word := range statement {
			if word == "installed-targets" && i > 0 && strings.HasSuffix(statement[i-1], "ENGINE_BINARY") && strings.Join(statement, " ") != helper {
				t.Errorf("a call of the engine's installed-targets that does not go through longtermmem_targets: %q", strings.Join(statement, " "))
			}
		}
	}
}

// A bad flag hidden in each shape of call is found. The mutants are the script itself with a flag the
// command does not take put in, one shape at a time, and the check has to name it; a shape it cannot
// see would pass the mutant, and the test would fail.
func TestAFlagTheCommandDoesNotTakeIsFoundWhereverTheScriptHidesIt(t *testing.T) {
	script, err := os.ReadFile(overlaytest.Script(t))
	if err != nil {
		t.Fatal(err)
	}
	text := string(script)
	if problems := problemsWithEngineCalls(text); len(problems) != 0 {
		t.Fatalf("the script has problems before it is mutated:\n%s", strings.Join(problems, "\n"))
	}
	for _, tc := range []struct{ name, old, new string }{
		{"on the line of the call", `longtermmem_targets record --target "$t" || exit 1`, `longtermmem_targets record --target "$t" --bogus || exit 1`},
		{"on a continuation line", `longtermmem_targets record --target "$t" || exit 1`, "longtermmem_targets record \\\n          --target \"$t\" --bogus || exit 1"},
		{"after a semicolon", `longtermmem_targets record --target "$t" || exit 1`, `true; longtermmem_targets record --target "$t" --bogus || exit 1`},
		{"after &&", `longtermmem_targets record --target "$t" || exit 1`, `[[ -n "$t" ]] && longtermmem_targets record --target "$t" --bogus || exit 1`},
		{"after a pipe", `longtermmem_targets record --target "$t" || exit 1`, `echo | longtermmem_targets record --target "$t" --bogus || exit 1`},
		{"in an array of assess", `[[ -n "$purge" ]] && assess_args+=(--purge)`, `[[ -n "$purge" ]] && assess_args+=(--purge --bogus)`},
		{"in an array of retire", `local retire_args=(--binary "$LONGTERM_MEM_BINARY")`, `local retire_args=(--binary "$LONGTERM_MEM_BINARY" --bogus)`},
		{"in an array added to later", `[[ -n "$purge" ]] && retire_args+=(--purge)`, `[[ -n "$purge" ]] && retire_args+=(--purge --bogus)`},
		{"a verb the command does not take", `longtermmem_targets assess ${assess_args`, `longtermmem_targets assess-all ${assess_args`},
		{"a verb the script never calls again", `longtermmem_targets retire "${retire_args[@]}" || exit 1`, `true`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if strings.Count(text, tc.old) == 0 {
				t.Fatalf("the script has no %q to mutate", tc.old)
			}
			mutant := strings.Replace(text, tc.old, tc.new, 1)
			problems := strings.Join(problemsWithEngineCalls(mutant), "\n")
			if problems == "" {
				t.Fatalf("the check passes a script in which %s", tc.name)
			}
			if strings.HasPrefix(tc.name, "a verb") {
				return
			}
			if !strings.Contains(problems, "--bogus") {
				t.Errorf("the check names no --bogus: %s", problems)
			}
		})
	}
}
