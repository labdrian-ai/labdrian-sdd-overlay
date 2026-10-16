package shelltest

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// The overlay forwards the verbs of memory, roles, shaper and workflow to the engine binary
// verbatim: `labdrian <command> <verb> ...` runs `gentle-ai-overlay <command> <verb> ...`. Each
// command is a row of the table, and every row is held to the same four things, each run through the
// real entrypoint under a throwaway HOME whose engine binary is a fake that records its argv, so
// nothing real is touched:
//
//   - its arguments reach the engine verbatim (spaces inside an argument included);
//   - the exit status of the engine is the exit status of the overlay;
//   - a home with no engine binary is refused with a clear message;
//   - --help lists the command with its own verbs and not the verbs of another command.
//
// The rows that say more of their help (workflow lists the verbs that bind a repository, and every
// refusal they exit 2 for) carry it as helpBlocks.

// fakeEngine echoes its arguments, one in brackets, and exits with the status named by
// FAKE_ENGINE_STATUS (0 when it is not set).
const fakeEngine = `#!/usr/bin/env bash
printf 'engine-argv:'
printf ' [%s]' "$@"
printf '\n'
exit "${FAKE_ENGINE_STATUS:-0}"
`

// helpBlock is a set of things that must all be in the lines `--help` gives to one command.
type helpBlock struct {
	name  string
	wants []string
}

type dispatchRow struct {
	command string
	// forwarded is an invocation whose arguments must reach the engine as they are.
	forwarded []string
	// failing is an invocation to run with an engine that ends with wantStatus.
	failing    []string
	wantStatus int
	// noEngine is an invocation to run in a home that has no engine.
	noEngine []string
	// The line --help prints under "  <command> <verb>" must contain helpLine, and says it of this
	// command and not of otherCommand (a verb of which it would be if the help were attached to the
	// wrong entry).
	helpLine     string
	otherCommand string
	helpBlocks   []helpBlock
}

var dispatchRows = []dispatchRow{
	{
		command:      "memory",
		forwarded:    []string{"memory", "plan", "--profile", "odd", "--goal", "goal 1.json"},
		failing:      []string{"memory", "plan", "--profile", "odd"},
		wantStatus:   2,
		noEngine:     []string{"memory", "plan", "--profile", "odd"},
		helpLine:     "Read-only",
		otherCommand: "roles",
	},
	{
		command:      "roles",
		forwarded:    []string{"roles", "append", "--project", "p 1", "--goal", "g1", "--chain", "c1", "--stdin"},
		failing:      []string{"roles", "resume", "--project", "p", "--goal", "g", "--chain", "c"},
		wantStatus:   1,
		noEngine:     []string{"roles", "validate", "--file", "/tmp/nonexistent.json"},
		helpLine:     "data only",
		otherCommand: "shaper",
	},
	{
		command:      "shaper",
		forwarded:    []string{"shaper", "clearance", "record", "--root", "/w t", "--stdin"},
		failing:      []string{"shaper", "assess", "--root", "/w"},
		wantStatus:   3, // the exit status of a draft
		noEngine:     []string{"shaper", "assess"},
		helpLine:     "assess",
		otherCommand: "skills",
	},
	{
		command:      "workflow",
		forwarded:    []string{"workflow", "create", "--project", "proj-1", "--workflow", "wf-1", "--goal", "goal 1.json", "--profile", "odd"},
		failing:      []string{"workflow", "status", "--project", "proj-1", "--workflow", "wf-1"},
		wantStatus:   2,
		noEngine:     []string{"workflow", "status", "--project", "proj-1", "--workflow", "wf-1"},
		helpLine:     "Local bookkeeping",
		otherCommand: "memory",
		helpBlocks: []helpBlock{
			{
				// Each verb is matched with its own help wording: the bare word "binding" already appears in
				// the description of create.
				name:  "lists the workflow binding verbs",
				wants: []string{"bind   --project ID --workflow ID", "unbind    remove this repository's binding", "binding   read-only"},
			},
			{
				// Exit 2 covers every refusal of the binding verbs, not only a binding file that is not ours:
				// an unusable file, a repository whose lock is taken, and a binding another process changed.
				// Each phrase sits on one help line.
				name:  "states every refusal the binding verbs exit 2 for",
				wants: []string{"not ours or cannot be used", "(foreign, malformed, unavailable)", "in progress (busy)", "another process changed"},
			},
		},
	},
}

// engineArgv is what the fake engine prints for the arguments it was given.
func engineArgv(args []string) string {
	var b strings.Builder
	b.WriteString("engine-argv:")
	for _, a := range args {
		b.WriteString(" [" + a + "]")
	}
	return b.String()
}

// hasLine says whether one of the lines of out is exactly want: the engine got these arguments and no
// others (an extra argument the overlay added would be at the end of the line).
func hasLine(out, want string) bool {
	for _, line := range strings.Split(out, "\n") {
		if line == want {
			return true
		}
	}
	return false
}

// helpLineAfter is the line of help under the header that begins with prefix: the line after the last
// one that does, or that line itself when it is the last of the text. No such header gives "".
func helpLineAfter(help, prefix string) string {
	lines := strings.Split(help, "\n")
	found := ""
	for i, line := range lines {
		if strings.HasPrefix(line, prefix) {
			found = line
			if i+1 < len(lines) {
				found = lines[i+1]
			}
		}
	}
	return found
}

// nextHeader begins the next command in the help of the overlay: two spaces then a letter. The lines
// of a command's own entry are indented much deeper.
var nextHeader = regexp.MustCompile(`^  [a-z]`)

// helpBlockOf is the lines of help that belong to the command whose header begins with prefix: from the
// line after it to the next command header.
func helpBlockOf(help, prefix string) string {
	var block []string
	on := false
	for _, line := range strings.Split(help, "\n") {
		switch {
		case !on && strings.HasPrefix(line, prefix):
			on = true
		case on && nextHeader.MatchString(line):
			return strings.Join(block, "\n")
		case on:
			block = append(block, line)
		}
	}
	return strings.Join(block, "\n")
}

func TestOverlayDispatch(t *testing.T) {
	for _, row := range dispatchRows {
		t.Run(row.command, func(t *testing.T) {
			overlay := overlayScript(t)
			// run is the overlay under a home of its own whose engine is the fake, which ends with status
			// when it is not empty.
			run := func(t *testing.T, home string, status int, args ...string) (string, int) {
				t.Helper()
				env := append(os.Environ(), "HOME="+home)
				if status > 0 {
					env = append(env, "FAKE_ENGINE_STATUS="+strconv.Itoa(status))
				}
				out, err := overlaytest.Combined(t, overlay, env, "", args...)
				return out, overlaytest.ExitCode(err)
			}
			homeWithEngine := func(t *testing.T) string {
				t.Helper()
				home := t.TempDir()
				overlaytest.StubEngine(t, home, fakeEngine)
				return home
			}

			t.Run("forwards the verb and its arguments verbatim", func(t *testing.T) {
				out, exit := run(t, homeWithEngine(t), 0, row.forwarded...)

				if want := engineArgv(row.forwarded); exit != 0 || !hasLine(out, want) {
					t.Errorf("%s: exit %d and output %q, want exit 0 and a line that is exactly %q", row.command, exit, out, want)
				}
			})

			t.Run("preserves the exit status of the engine", func(t *testing.T) {
				_, exit := run(t, homeWithEngine(t), row.wantStatus, row.failing...)

				if exit != row.wantStatus {
					t.Errorf("%s: exit status %d, want %d", row.command, exit, row.wantStatus)
				}
			})

			t.Run("refuses a home with no engine binary with a clear message", func(t *testing.T) {
				out, exit := run(t, t.TempDir(), 0, row.noEngine...)

				if exit == 0 || !strings.Contains(out, "engine binary not found") {
					t.Errorf("%s: exit %d and output %q, want a non-zero exit and \"engine binary not found\"", row.command, exit, out)
				}
			})

			t.Run("help lists the command with its own verbs", func(t *testing.T) {
				help, _ := run(t, t.TempDir(), 0, "--help")

				header := "  " + row.command + " <verb>"
				if line := helpLineAfter(help, header); !strings.Contains(line, row.helpLine) {
					t.Errorf("--help: the line under %q is %q, want one with %q, not the verbs of %s", header, line, row.helpLine, row.otherCommand)
				}
			})

			for _, block := range row.helpBlocks {
				t.Run("help "+block.name, func(t *testing.T) {
					help, _ := run(t, t.TempDir(), 0, "--help")

					got := helpBlockOf(help, "  "+row.command+" <verb>")
					var missing []string
					for _, want := range block.wants {
						if !strings.Contains(got, want) {
							missing = append(missing, want)
						}
					}
					if len(missing) > 0 {
						t.Errorf("--help for %s leaves out %q; its entry is:\n%s", row.command, missing, got)
					}
				})
			}
		})
	}
}

// The three readers of text the table uses are held on small texts of their own, so that a mistake in
// one of them does not make a row pass or fail for a reason the script has no part in.

func TestEngineArgvBracketsEachArgumentAsTheFakeEnginePrintsIt(t *testing.T) {
	if got, want := engineArgv([]string{"a", "b c", ""}), "engine-argv: [a] [b c] []"; got != want {
		t.Errorf("engineArgv = %q, want %q", got, want)
	}
	if got, want := engineArgv(nil), "engine-argv:"; got != want {
		t.Errorf("engineArgv() = %q, want %q", got, want)
	}
}

func TestHasLineWantsTheWholeLine(t *testing.T) {
	for _, tc := range []struct {
		name, out, want string
		ok              bool
	}{
		{"a line of its own", "x\nengine-argv: [a]\ny\n", "engine-argv: [a]", true},
		{"a line with something after it", "engine-argv: [a] []\n", "engine-argv: [a]", false},
		{"a line with something before it", "see engine-argv: [a]\n", "engine-argv: [a]", false},
		{"the last line, with no line end", "x\nengine-argv: [a]", "engine-argv: [a]", true},
		{"nothing", "", "engine-argv: [a]", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasLine(tc.out, tc.want); got != tc.ok {
				t.Errorf("hasLine(%q, %q) = %v, want %v", tc.out, tc.want, got, tc.ok)
			}
		})
	}
}

func TestHelpLineAfterIsTheLineUnderTheLastMatchingHeader(t *testing.T) {
	for _, tc := range []struct {
		name, help, want string
	}{
		{"a header with a line under it", "  a <verb>  one\n    under a\n  b <verb>  two\n    under b\n", "    under a"},
		{"the header is the last line", "  a <verb>  one\n  b <verb>  two", "  b <verb>  two"},
		{"the header twice", "  a <verb>  one\n    first\n  a <verb>  again\n    second\n", "    second"},
		{"a header that only contains the prefix", "x  a <verb>\n    no\n", ""},
		{"no such header", "  b <verb>  two\n    under b\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := helpLineAfter(tc.help, "  a <verb>"); got != tc.want {
				t.Errorf("helpLineAfter = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHelpBlockOfRunsFromTheLineAfterTheHeaderToTheNextHeader(t *testing.T) {
	for _, tc := range []struct {
		name, help, want string
	}{
		{"between two headers", "  a <verb>  one\n      detail 1\n      detail 2\n  b <verb>  two\n      other\n", "      detail 1\n      detail 2"},
		{"the last entry", "  b <verb>  two\n  a <verb>  one\n      detail\n", "      detail\n"},
		{"a header with nothing under it", "  a <verb>  one\n  b <verb>  two\n", ""},
		{"deeper lines are not headers", "  a <verb>  one\n    Two spaces deeper\n   x\n  b <verb>\n", "    Two spaces deeper\n   x"},
		{"no such header", "  b <verb>  two\n      other\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := helpBlockOf(tc.help, "  a <verb>"); got != tc.want {
				t.Errorf("helpBlockOf = %q, want %q", got, tc.want)
			}
		})
	}
}
