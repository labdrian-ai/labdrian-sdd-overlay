package main

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// The golden cases of the two hooks that guard a tool call against the agent that makes it:
// 'skills guard-hook' (a deny in JSON, exit 0; input it cannot decode is let through, input over the bound is denied) and
// 'shaper guard-hook' (a deny by exit 2 and a message on stderr; input it cannot read is
// denied). The harness is hook_golden_test.go.

// jsonText is s as a JSON string.
func jsonText(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// skillsGuard records one run of 'skills guard-hook' with the given arguments after it.
func (w *hookWorld) skillsGuard(label, stdin string, extraArgs ...string) {
	w.t.Helper()
	w.skillsGuardReader(label, strings.NewReader(stdin), stdin, extraArgs...)
}

// skillsGuardReader records one run of 'skills guard-hook' over a stdin that is not a string.
func (w *hookWorld) skillsGuardReader(label string, stdin io.Reader, stdinNote string, extraArgs ...string) {
	w.t.Helper()
	var stdout, stderr bytes.Buffer
	var exits []int
	runSkillsWithStdin(testDeps(), append([]string{skillsGuardVerb}, extraArgs...), stdin, &stdout, &stderr, func(c int) { exits = append(exits, c) })
	w.record("skills "+strings.Join(append([]string{skillsGuardVerb}, extraArgs...), " "), label, stdinNote, exits, stdout.String(), stderr.String())
}

// skillsGuardPanicking records one run of 'skills guard-hook' whose decision panics with value.
func (w *hookWorld) skillsGuardPanicking(label, stdin string, value any) {
	w.t.Helper()
	var stdout, stderr bytes.Buffer
	var exits []int
	runSkillsGuardHook(nil, strings.NewReader(stdin), &stdout, &stderr, func(c int) { exits = append(exits, c) }, func() { panic(value) })
	w.record("skills "+skillsGuardVerb, label, stdin, exits, stdout.String(), stderr.String())
}

// skillsGuardWriter records one run of 'skills guard-hook' whose stdout cannot be written.
func (w *hookWorld) skillsGuardWriter(label, stdin string, stdout io.Writer) {
	w.t.Helper()
	var stderr bytes.Buffer
	var exits []int
	runSkillsWithStdin(testDeps(), []string{skillsGuardVerb}, strings.NewReader(stdin), stdout, &stderr, func(c int) { exits = append(exits, c) })
	w.record("skills "+skillsGuardVerb, label, stdin, exits, "<stdout cannot be written>\n", stderr.String())
}

// shaperGuard records one run of 'shaper guard-hook'.
func (w *hookWorld) shaperGuard(label, stdin string) {
	w.t.Helper()
	w.shaperGuardReader(label, strings.NewReader(stdin), stdin)
}

// shaperGuardReader records one run of 'shaper guard-hook' over a stdin that is not a string.
func (w *hookWorld) shaperGuardReader(label string, stdin io.Reader, stdinNote string) {
	w.t.Helper()
	var stdout, stderr bytes.Buffer
	var exits []int
	runShaperCore(testDeps(), []string{"guard-hook"}, stdin, &stdout, &stderr, func(c int) { exits = append(exits, c) })
	w.record("shaper guard-hook", label, stdinNote, exits, stdout.String(), stderr.String())
}

// bashToolCall is the hook input of a Bash call of command, in /work, with the tool's
// description beside it when there is one. The input of both guards is built here, so a field
// the envelope gains is added in one place.
func bashToolCall(command, description string) string {
	input := `{"command":` + jsonText(command)
	if description != "" {
		input += `,"description":` + jsonText(description)
	}
	return claudeToolCall("/work", "Bash", input+"}")
}

// ---- skills guard-hook -----------------------------------------------------------------

func approveGuardGoldenCases() []hookGoldenCase {
	// The approve guard's calls carry the description Claude Code sends beside a command.
	bash := func(command string) string { return bashToolCall(command, "d") }
	return []hookGoldenCase{
		{"approve-guard-denies-the-agent-running-skills-approve", func(w *hookWorld) {
			for _, command := range []string{
				"labdrian skills approve --id my-skill --approver me",
				"/usr/local/bin/labdrian-overlay skills approve --id x",
				`"gentle-ai-overlay" 'skills' "approve"`,
				`sh -c 'labdrian skills approve --id x'`,
				"echo x | xargs labdrian skills approve",
				"cd /repo && labdrian skills approve --id x",
				"ls\nlabdrian skills approve --id x",
				"rg --pre ./x labdrian skills approve",
				"git commit -m 'run labdrian skills approve'",
			} {
				w.skillsGuard("Bash: "+command, bash(command))
			}
		}},
		{"approve-guard-denies-writing-the-approval-record", func(w *hookWorld) {
			for _, tc := range []struct{ tool, input string }{
				{"Write", `{"file_path":"/repo/skills/my-skill/.approval.json","content":"{}"}`},
				{"Edit", `{"file_path":"/repo/skills/my-skill/.approval.json","old_string":"a","new_string":"b"}`},
				{"MultiEdit", `{"file_path":"/repo/skills/my-skill/.approval.json","edits":[]}`},
				{"NotebookEdit", `{"notebook_path":"/repo/skills/my-skill/.approval.json","new_source":"x"}`},
				{"Write", `{"file_path":"/repo/skills/my-skill/.APPROVAL.JSON","content":"{}"}`},
				{"Write", `{"file_path":"C:\\repo\\skills\\x\\.approval.json","content":"{}"}`},
				{"Write", `{"file_path":".approval.json","content":"{}"}`},
			} {
				w.skillsGuard(tc.tool+" "+tc.input, claudeToolCall("/work", tc.tool, tc.input))
			}
		}},
		{"approve-guard-lets-the-rest-through", func(w *hookWorld) {
			for _, command := range []string{
				"ls -la",
				"rg 'labdrian skills approve' docs",
				"grep -rn 'labdrian skills approve' .",
				"rg --pretty 'labdrian skills approve'",
				"labdrian skills approved",
				"labdrian skills approve-all",
				"labdrian skills list",
				"echo labdrian skills",
			} {
				w.skillsGuard("Bash: "+command, bash(command))
			}
			for _, tc := range []struct{ tool, input string }{
				{"Write", `{"file_path":"/repo/skills/my-skill/SKILL.md","content":"labdrian skills approve --id x"}`},
				{"Write", `{"file_path":"/repo/README.md","content":"Run labdrian skills approve."}`},
				{"Read", `{"file_path":"/repo/skills/my-skill/.approval.json"}`},
				{"Grep", `{"pattern":".approval.json","path":"/repo"}`},
				{"Bash", `{"file_path":"/repo/skills/x/.approval.json"}`},
				{"Read", `{"command":"labdrian skills approve"}`},
				{"SomethingNew", `{"command":"labdrian skills approve"}`},
				{"bash", `{"command":"labdrian skills approve"}`},
				{"Write", `{}`},
				{"Bash", `{}`},
			} {
				w.skillsGuard(tc.tool+" "+tc.input, claudeToolCall("/work", tc.tool, tc.input))
			}
		}},
		{"approve-guard-lets-input-it-cannot-decode-through", func(w *hookWorld) {
			for _, in := range malformedInputs {
				w.skillsGuard(in.label, in.stdin)
			}
		}},
		{"approve-guard-reads-the-fields-it-names", func(w *hookWorld) {
			const denied = `"command":"labdrian skills approve"`
			w.skillsGuard("tool_name of the wrong type with a denied command", `{"tool_name":7,"tool_input":{`+denied+`}}`)
			w.skillsGuard("tool_name an object", `{"tool_name":{},"tool_input":{`+denied+`}}`)
			w.skillsGuard("tool_name missing", `{"tool_input":{`+denied+`}}`)
			w.skillsGuard("tool_input a string", `{"tool_name":"Bash","tool_input":"labdrian skills approve"}`)
			w.skillsGuard("tool_input an array", `{"tool_name":"Bash","tool_input":[1]}`)
			w.skillsGuard("tool_input a number", `{"tool_name":"Bash","tool_input":5}`)
			w.skillsGuard("tool_input null", `{"tool_name":"Bash","tool_input":null}`)
			w.skillsGuard("command of the wrong type", `{"tool_name":"Bash","tool_input":{"command":5}}`)
			w.skillsGuard("command null", `{"tool_name":"Bash","tool_input":{"command":null}}`)
			w.skillsGuard("file_path of the wrong type beside a denied command", `{"tool_name":"Bash","tool_input":{`+denied+`,"file_path":5}}`)
			w.skillsGuard("notebook_path of the wrong type", `{"tool_name":"NotebookEdit","tool_input":{"notebook_path":["x"]}}`)
			w.skillsGuard("fields of the envelope it does not read, of the wrong type", `{"cwd":5,"hook_event_name":7,"session_id":{},"tool_name":"Bash","tool_input":{`+denied+`}}`)
			w.skillsGuard("keys in other cases", `{"TOOL_NAME":"Bash","TOOL_INPUT":{"COMMAND":"labdrian skills approve"}}`)
			w.skillsGuard("a key twice: the last one wins", `{"tool_name":"Bash","tool_input":{"command":"ls","command":"labdrian skills approve"}}`)
			w.skillsGuard("a command with an escaped line break", `{"tool_name":"Bash","tool_input":{"command":"ls\nlabdrian skills approve"}}`)
		}},
		{"approve-guard-denies-input-over-eight-mebibytes", func(w *hookWorld) {
			const mebibyte = 1 << 20
			const prefix = `{"tool_name":"Bash","tool_input":{"command":"labdrian skills approve","padding":"`
			w.skillsGuard("a denied command in an input of exactly 8 MiB is judged", padded(prefix, `"}}`, 8*mebibyte))
			w.skillsGuard("an unrelated command in an input of 8 MiB and one byte is denied without being judged", padded(`{"tool_name":"Bash","tool_input":{"command":"ls","padding":"`, `"}}`, 8*mebibyte+1))
			w.skillsGuard("the same command in an input of 8 MiB and one byte is denied for the size", padded(prefix, `"}}`, 8*mebibyte+1))
		}},
		{"approve-guard-lets-the-call-through-when-stdin-fails", func(w *hookWorld) {
			w.skillsGuardReader("a stdin that fails", failingReader{}, failingStdin)
		}},
		{"approve-guard-turns-a-panic-into-a-warning", func(w *hookWorld) {
			w.skillsGuardPanicking("a panic where the guard decides", bash("ls"), "boom\nsecond line \x1b[31mred "+strings.Repeat("x", 5000))
			w.skillsGuardPanicking("a short panic value", bash("labdrian skills approve"), "boom")
		}},
		{"approve-guard-exits-zero-when-stdout-cannot-be-written", func(w *hookWorld) {
			w.skillsGuardWriter("the denial cannot be written", bash("labdrian skills approve"), failingMemoryWriter{})
		}},
		{"approve-guard-refuses-arguments", func(w *hookWorld) {
			w.skillsGuard("an argument", bash("ls"), "extra")
			w.skillsGuard("a flag", bash("ls"), "--event", "PreToolUse")
		}},
	}
}

// ---- shaper guard-hook -----------------------------------------------------------------

func shaperGuardGoldenCases() []hookGoldenCase {
	bash := func(command string) string { return bashToolCall(command, "") }
	return []hookGoldenCase{
		{"shaper-guard-denies-recording-a-clearance", func(w *hookWorld) {
			for _, command := range []string{
				"gentle-ai-overlay shaper clearance record --stdin --root /r",
				"echo '{}' | ~/.claude/bin/gentle-ai-overlay shaper clearance record --stdin",
				`sh -c "gentle-ai-overlay shaper clearance record --stdin"`,
				"gentle-ai-overlay shaper   clearance\trecord --stdin",
				"gentle-ai-overlay shaper \\\n clearance \\\n record --stdin",
				"echo x > ~/.local/state/labdrian/shaper-clearance/p/g/a.json",
				"ls $XDG_STATE_HOME/labdrian/shaper-clearance",
			} {
				w.shaperGuard("Bash: "+command, bash(command))
			}
		}},
		{"shaper-guard-denies-touching-the-clearance-store", func(w *hookWorld) {
			for _, tc := range []struct{ tool, input string }{
				{"Write", `{"file_path":"/home/u/.local/state/labdrian/shaper-clearance/p/g/x.json","content":"{}"}`},
				{"Edit", `{"file_path":"/s/labdrian/shaper-clearance/p/g/x.json","old_string":"a","new_string":"b"}`},
				{"NotebookEdit", `{"notebook_path":"/s/labdrian/shaper-clearance/n.ipynb","new_source":"x"}`},
				{"Read", `{"file_path":"/s/labdrian/shaper-clearance/p/g/x.json"}`},
				{"SomethingNew", `{"file_path":"/s/labdrian/shaper-clearance/p/g/x.json"}`},
			} {
				w.shaperGuard(tc.tool+" "+tc.input, claudeToolCall("/work", tc.tool, tc.input))
			}
		}},
		{"shaper-guard-lets-the-rest-through", func(w *hookWorld) {
			for _, command := range []string{
				"go test ./...",
				"gentle-ai-overlay shaper assess --root /r --handoff h.json --goal g.json",
				"git checkout feat/shaper-clearance",
				"echo shaper clearance",
			} {
				w.shaperGuard("Bash: "+command, bash(command))
			}
			for _, tc := range []struct{ tool, input string }{
				{"Write", `{"file_path":"/repo/doc.md","content":"run shaper clearance record"}`},
				// Not a case of "lets through": the file name only starts like the store (the
				// marker is a substring of the path), so the guard denies it. It is recorded here
				// as the over-match it is, a false positive of a guard that matches text only (the
				// golden file shows exit 2 for this call, not an allow).
				{"Write", `{"file_path":"/repo/labdrian/shaper-clearance-notes.md","content":"x"}`},
				{"Write", `{"file_path":"/repo/doc.md","content":"labdrian/shaper-clearance"}`},
				{"Edit", `{"file_path":"/repo/main.go","old_string":"a","new_string":"b"}`},
				{"Write", `{}`},
				{"Bash", `{}`},
				{"Bash", `null`},
			} {
				w.shaperGuard(tc.tool+" "+tc.input, claudeToolCall("/work", tc.tool, tc.input))
			}
			w.shaperGuard("an object with no fields it reads", `{}`)
			w.shaperGuard("an object with only tool_name", `{"tool_name":"Bash"}`)
			w.shaperGuard("null: nothing to decode into", `null`)
			w.shaperGuard("fields of the envelope it does not read, of the wrong type", `{"cwd":5,"hook_event_name":7,"session_id":{},"tool_name":"Bash","tool_input":{"command":"ls"}}`)
			w.shaperGuard("keys in other cases", `{"TOOL_NAME":"Bash","TOOL_INPUT":{"COMMAND":"gentle-ai-overlay shaper clearance record --stdin"}}`)
			w.shaperGuard("a key twice: the last one wins", `{"tool_name":"Bash","tool_input":{"command":"ls","command":"gentle-ai-overlay shaper clearance record"}}`)
		}},
		{"shaper-guard-denies-input-it-cannot-decode", func(w *hookWorld) {
			for _, in := range malformedInputs {
				w.shaperGuard(in.label, in.stdin)
			}
		}},
		{"shaper-guard-denies-a-field-of-the-wrong-type", func(w *hookWorld) {
			w.shaperGuard("tool_name a number", `{"tool_name":7,"tool_input":{"command":"ls"}}`)
			w.shaperGuard("tool_name an object", `{"tool_name":{},"tool_input":{"command":"ls"}}`)
			w.shaperGuard("tool_name a boolean", `{"tool_name":true}`)
			w.shaperGuard("tool_input a string", `{"tool_name":"Bash","tool_input":"ls"}`)
			w.shaperGuard("tool_input an array", `{"tool_name":"Bash","tool_input":[1]}`)
			w.shaperGuard("tool_input a number", `{"tool_name":"Bash","tool_input":5}`)
			w.shaperGuard("command a number", `{"tool_name":"Bash","tool_input":{"command":5}}`)
			w.shaperGuard("command an array", `{"tool_name":"Bash","tool_input":{"command":["ls"]}}`)
			w.shaperGuard("file_path an object", `{"tool_name":"Write","tool_input":{"file_path":{}}}`)
			w.shaperGuard("notebook_path a boolean", `{"tool_name":"NotebookEdit","tool_input":{"notebook_path":false}}`)
			w.shaperGuard("two fields of the wrong type: the first is named", `{"tool_name":7,"tool_input":{"command":5}}`)
			w.shaperGuard("a field of the wrong type beside a command that would be denied", `{"tool_name":7,"tool_input":{"command":"gentle-ai-overlay shaper clearance record"}}`)
		}},
		// The guard reads at most the bound of every guard, 8 MiB, and one byte more. Up to the
		// bound it judges what it was given, however large; over it, it denies without judging,
		// as it denies any input it cannot read: it fails closed, and the approve guard, which
		// runs on the same calls, denies an input over the bound too (it lets through only the
		// input it cannot decode). (Until Phase 9 batch 10 it read without
		// any bound, and these were its first two cases, which pass unchanged.)
		{"shaper-guard-judges-input-up-to-eight-mebibytes", func(w *hookWorld) {
			const mebibyte = 1 << 20
			const unrelated = `{"tool_name":"Write","tool_input":{"file_path":"/tmp/big.txt","content":"`
			const naming = `{"tool_name":"Bash","tool_input":{"command":"gentle-ai-overlay shaper clearance record --stdin `
			w.shaperGuard("a large unrelated write is allowed", padded(unrelated, `"}}`, 5*mebibyte))
			w.shaperGuard("a large command that names the record verb is denied", padded(naming, `"}}`, 5*mebibyte))
			w.shaperGuard("an unrelated write of exactly 8 MiB is judged and allowed", padded(unrelated, `"}}`, 8*mebibyte))
			w.shaperGuard("a command that names the record verb, in an input of exactly 8 MiB, is judged and denied", padded(naming, `"}}`, 8*mebibyte))
			w.shaperGuard("an unrelated write of 8 MiB and one byte is denied without being judged", padded(unrelated, `"}}`, 8*mebibyte+1))
			w.shaperGuard("a command that names the record verb, in an input of 8 MiB and one byte, is denied for the size", padded(naming, `"}}`, 8*mebibyte+1))
		}},
		{"shaper-guard-denies-when-stdin-fails", func(w *hookWorld) {
			w.shaperGuardReader("a stdin that fails", failingReader{}, failingStdin)
		}},
	}
}
