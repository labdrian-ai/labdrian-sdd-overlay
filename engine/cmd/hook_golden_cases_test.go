package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ---- inputs ----------------------------------------------------------------------------

// claudeToolCall is the PreToolUse input Claude Code sends for a tool call, with the fields
// it really carries around the tool input given as raw JSON.
func claudeToolCall(cwd, tool, toolInput string) string {
	return fmt.Sprintf(`{"session_id":"s1","transcript_path":"/t.jsonl","cwd":%q,"permission_mode":"default","hook_event_name":"PreToolUse","tool_name":%q,"tool_input":%s,"tool_use_id":"toolu_01"}`,
		cwd, tool, toolInput)
}

// claudePrompt is the UserPromptSubmit input Claude Code sends for a prompt.
func claudePrompt(cwd string) string {
	return fmt.Sprintf(`{"session_id":"s1","transcript_path":"/t.jsonl","cwd":%q,"permission_mode":"default","hook_event_name":"UserPromptSubmit","prompt":"a prompt"}`, cwd)
}

// agentCall is the PreToolUse input for the Agent tool with the given tool input (raw JSON).
func agentCall(toolInput string) string { return claudeToolCall("/work", "Agent", toolInput) }

// malformedInputs are the inputs no hook can decode as a hook input, or that are not the
// object it needs: what each hook does with them is part of its contract.
var malformedInputs = []struct{ label, stdin string }{
	{"empty", ""},
	{"white space only", " \n\t "},
	{"not JSON", "hello"},
	{"a truncated object", `{"tool_name":"Bash","tool_input":{"command":"ls"`},
	{"an array", `[{"tool_name":"Bash","tool_input":{"command":"ls"}}]`},
	{"a string", `"Bash"`},
	{"a number", `7`},
	{"true", `true`},
	{"null", `null`},
	{"two objects", `{"tool_name":"Bash"}{"tool_name":"Bash"}`},
	{"text after the object", `{"tool_name":"Bash"} trailing`},
	{"an empty object", `{}`},
}

// padded is a hook input of exactly total bytes: prefix, as many "x" as it takes, and suffix,
// which closes the string the prefix ended in and the objects it opened. A case that tests a
// size bound gives the size, so the bytes are the same wherever it runs.
func padded(prefix, suffix string, total int) string {
	filler := total - len(prefix) - len(suffix)
	if filler < 0 {
		panic(fmt.Sprintf("the prefix of %d bytes does not fit in %d", len(prefix), total))
	}
	return prefix + strings.Repeat("x", filler) + suffix
}

// ---- the cases -------------------------------------------------------------------------

func hookGoldenCases() []hookGoldenCase {
	var cases []hookGoldenCase
	cases = append(cases, gateTaskGoldenCases()...)
	cases = append(cases, promptHookGoldenCases()...)
	cases = append(cases, toolHookGoldenCases()...)
	cases = append(cases, profileHookGoldenCases()...)
	cases = append(cases, approveGuardGoldenCases()...)
	cases = append(cases, shaperGuardGoldenCases()...)
	return cases
}

// ---- gate-task -------------------------------------------------------------------------

func gateTaskGoldenCases() []hookGoldenCase {
	fileArgs := []string{"--contract-file", contractFile, "--contract-path", contractEntry}
	withContract := func(w *hookWorld) { w.files[contractFile] = contractDoc(fmApplies, fmExcluded, fmInject) }
	return []hookGoldenCase{
		{"gate-task-lets-the-prompt-through", func(w *hookWorld) {
			withContract(w)
			w.gateTask("a phase the contract names nowhere", agentInput("sdd-explore", promptWithEntry), fileArgs...)
			w.gateTask("an excluded phase that does not carry the contract", agentInput("sdd-propose", promptPlain), fileArgs...)
			w.gateTask("an applies-to phase that already carries it", agentInput("sdd-apply", promptWithEntry), fileArgs...)
			w.gateTask("a sub-agent type no contract knows", agentInput("general-purpose", promptPlain), fileArgs...)
			w.gateTask("the entry is padded with white space", agentInput("sdd-apply", "Do the phase.\n\n## Skills to load before work\n   "+contractEntry+"   \n"), fileArgs...)
		}},
		{"gate-task-injects-the-contract", func(w *hookWorld) {
			withContract(w)
			w.gateTask("no skills section, prompt without a line break at its end", agentInput("sdd-apply", promptPlain), fileArgs...)
			w.gateTask("no skills section, prompt ending in one line break", agentInput("sdd-apply", "Do the phase.\n"), fileArgs...)
			w.gateTask("no skills section, prompt ending in two line breaks", agentInput("sdd-apply", "Do the phase.\n\n"), fileArgs...)
			w.gateTask("a skills section with another skill", agentInput("sdd-tasks", promptWithHeader), fileArgs...)
			w.gateTask("a heading that only starts like the skills section", agentInput("sdd-apply", "Do it.\n\n## Skills to load before work (extra)\n/skills/x/SKILL.md\n"), fileArgs...)
			w.gateTask("a prompt with carriage returns", agentInput("sdd-apply", "Do it.\r\n\r\n## Skills to load before work\r\n/skills/x/SKILL.md\r\n"), fileArgs...)
		}},
		{"gate-task-strips-the-contract", func(w *hookWorld) {
			withContract(w)
			w.gateTask("an excluded phase carrying the contract", agentInput("sdd-propose", promptWithEntry), fileArgs...)
			w.gateTask("the entry twice", agentInput("sdd-spec", promptWithEntry+contractEntry+"\n"), fileArgs...)
			w.gateTask("the entry padded with white space", agentInput("sdd-design", "Do it.\n\n## Skills to load before work\n  "+contractEntry+"\t\n/skills/x/SKILL.md\n"), fileArgs...)
			w.gateTask("a line that only contains the path", agentInput("sdd-verify", "see "+contractEntry+" for the rules\n"), fileArgs...)
		}},
		{"gate-task-echoes-the-tool-input", func(w *hookWorld) {
			withContract(w)
			w.gateTask("a model is echoed", agentCall(`{"description":"d","subagent_type":"sdd-apply","prompt":"p","model":"sonnet"}`), fileArgs...)
			w.gateTask("a null model is left out", agentCall(`{"description":"d","subagent_type":"sdd-apply","prompt":"p","model":null}`), fileArgs...)
			w.gateTask("an empty model is echoed", agentCall(`{"description":"d","subagent_type":"sdd-apply","prompt":"p","model":""}`), fileArgs...)
			w.gateTask("a missing description is echoed empty", agentCall(`{"subagent_type":"sdd-apply","prompt":"p"}`), fileArgs...)
			w.gateTask("fields the Agent tool has besides the four are dropped", agentCall(`{"description":"d","subagent_type":"sdd-apply","prompt":"p","run_in_background":true,"isolation":"worktree","name":"x"}`), fileArgs...)
			w.gateTask("the order of the input is not the order of the output", agentCall(`{"prompt":"p","model":"opus","subagent_type":"sdd-apply","description":"d"}`), fileArgs...)
			w.gateTask("non-ASCII text is written as it is", agentCall(`{"description":"é 日本 😀","subagent_type":"sdd-apply","prompt":"café 日本"}`), fileArgs...)
		}},
		{"gate-task-writes-html-characters-as-they-are", func(w *hookWorld) {
			withContract(w)
			w.gateTask("angle brackets and an ampersand", agentCall(`{"description":"d <x> & y","subagent_type":"sdd-apply","prompt":"a<b>&c"}`), fileArgs...)
			w.gateTask("the separators U+2028 and U+2029", agentCall("{\"description\":\"d\",\"subagent_type\":\"sdd-apply\",\"prompt\":\"line\u2028sep\u2029end\"}"), fileArgs...)
			w.gateTask("quotes, a backslash, a tab and a control character", agentCall(`{"description":"d","subagent_type":"sdd-apply","prompt":"quote \" and \\ backslash\ttab \u0001 ctl"}`), fileArgs...)
			w.gateTask("a line break in the description", agentCall(`{"description":"two\nlines","subagent_type":"sdd-apply","prompt":"p"}`), fileArgs...)
		}},
		{"gate-task-lets-through-input-it-cannot-decode", func(w *hookWorld) {
			withContract(w)
			for _, in := range malformedInputs {
				w.gateTask(in.label, in.stdin, fileArgs...)
			}
		}},
		{"gate-task-reads-the-fields-it-names", func(w *hookWorld) {
			withContract(w)
			good := `"description":"d","subagent_type":"sdd-apply","prompt":"p"`
			w.gateTask("tool_name of the wrong type", `{"tool_name":7,`+`"tool_input":{`+good+`}}`, fileArgs...)
			w.gateTask("tool_name an object", `{"tool_name":{},`+`"tool_input":{`+good+`}}`, fileArgs...)
			w.gateTask("tool_name missing: the name is not read", `{"tool_input":{`+good+`}}`, fileArgs...)
			w.gateTask("tool_name of another tool: the name is not read", `{"tool_name":"Bash","tool_input":{`+good+`}}`, fileArgs...)
			w.gateTask("fields of the envelope it does not read, of the wrong type", `{"cwd":5,"hook_event_name":7,"session_id":{},"tool_name":"Agent","tool_input":{`+good+`}}`, fileArgs...)
			w.gateTask("tool_input missing", `{"tool_name":"Agent"}`, fileArgs...)
			w.gateTask("tool_input null", `{"tool_name":"Agent","tool_input":null}`, fileArgs...)
			w.gateTask("tool_input a string", `{"tool_name":"Agent","tool_input":"x"}`, fileArgs...)
			w.gateTask("tool_input a number", `{"tool_name":"Agent","tool_input":5}`, fileArgs...)
			w.gateTask("tool_input an array", `{"tool_name":"Agent","tool_input":[1]}`, fileArgs...)
			w.gateTask("description of the wrong type", `{"tool_name":"Agent","tool_input":{"description":5,"subagent_type":"sdd-apply","prompt":"p"}}`, fileArgs...)
			w.gateTask("prompt of the wrong type", `{"tool_name":"Agent","tool_input":{"description":"d","subagent_type":"sdd-apply","prompt":5}}`, fileArgs...)
			w.gateTask("prompt null", `{"tool_name":"Agent","tool_input":{"description":"d","subagent_type":"sdd-apply","prompt":null}}`, fileArgs...)
			w.gateTask("subagent_type of the wrong type", `{"tool_name":"Agent","tool_input":{"description":"d","subagent_type":5,"prompt":"p"}}`, fileArgs...)
			w.gateTask("model of the wrong type", `{"tool_name":"Agent","tool_input":{"description":"d","subagent_type":"sdd-apply","prompt":"p","model":5}}`, fileArgs...)
			w.gateTask("model an object", `{"tool_name":"Agent","tool_input":{"description":"d","subagent_type":"sdd-apply","prompt":"p","model":{}}}`, fileArgs...)
			w.gateTask("a field it does not read, of the wrong type", `{"tool_name":"Agent","tool_input":{`+good+`,"command":5,"run_in_background":"yes"}}`, fileArgs...)
			w.gateTask("an empty sub-agent type", `{"tool_name":"Agent","tool_input":{"description":"d","subagent_type":"","prompt":"p"}}`, fileArgs...)
			w.gateTask("an empty prompt", `{"tool_name":"Agent","tool_input":{"description":"d","subagent_type":"sdd-apply","prompt":""}}`, fileArgs...)
			w.gateTask("a prompt of white space", `{"tool_name":"Agent","tool_input":{"description":"d","subagent_type":"sdd-apply","prompt":"  "}}`, fileArgs...)
		}},
		{"gate-task-matches-keys-without-regard-to-case", func(w *hookWorld) {
			withContract(w)
			w.gateTask("keys of the tool input in other cases", agentCall(`{"Description":"d","SUBAGENT_TYPE":"sdd-apply","Prompt":"p","MODEL":"opus"}`), fileArgs...)
			w.gateTask("TOOL_INPUT in capitals", `{"TOOL_NAME":"Agent","TOOL_INPUT":{"description":"d","subagent_type":"sdd-apply","prompt":"p"}}`, fileArgs...)
			w.gateTask("a key twice: the last one wins", agentCall(`{"description":"d","subagent_type":"sdd-explore","subagent_type":"sdd-apply","prompt":"p"}`), fileArgs...)
		}},
		{"gate-task-reads-at-most-four-mebibytes", func(w *hookWorld) {
			withContract(w)
			const mebibyte = 1 << 20
			const prefix = `{"tool_name":"Agent","tool_input":{"description":"d","subagent_type":"sdd-apply","prompt":"`
			w.gateTask("an input of 3 MiB is rewritten", padded(prefix, `"}}`, 3*mebibyte), fileArgs...)
			w.gateTask("an input of exactly 4 MiB is read whole and rewritten", padded(prefix, `"}}`, 4*mebibyte), fileArgs...)
			w.gateTask("an input of 4 MiB and one byte is cut off at 4 MiB, and what is left is passed through", padded(prefix, `"}}`, 4*mebibyte+1), fileArgs...)
		}},
		{"gate-task-lets-the-prompt-through-when-stdin-fails", func(w *hookWorld) {
			withContract(w)
			w.gateTaskReader("a stdin that fails", failingReader{}, failingStdin, fileArgs...)
		}},
	}
}

// ---- projection hook, UserPromptSubmit ---------------------------------------------------

// promptHook records the UserPromptSubmit hook for the session whose directory is cwd.
func (w *hookWorld) promptHook(label string, e hookEnv, cwd string) {
	w.t.Helper()
	w.projection(label, hookArgs, claudePrompt(cwd), e.dir)
}

func promptHookGoldenCases() []hookGoldenCase {
	return []hookGoldenCase{
		{"prompt-projects-an-open-workflow", func(w *hookWorld) {
			e := w.env()
			e.create(w.t, "proj-1", "wf-1", "standalone-minimal")
			mustBindOK(w.t, e.repo, "proj-1", "wf-1")
			w.promptHook("created", e, e.repo)
			e.step(w.t, "proj-1", "wf-1", "start")
			e.step(w.t, "proj-1", "wf-1", "stage", "--stage", "authorize")
			e.step(w.t, "proj-1", "wf-1", "stage", "--stage", "bound-scope")
			w.promptHook("running with two stages", e, e.repo)
			e.step(w.t, "proj-1", "wf-1", "pause")
			w.promptHook("paused", e, e.repo)
		}},
		{"prompt-projects-the-odd-profile", func(w *hookWorld) {
			e := w.env()
			e.running(w.t, "proj-1", "wf-1", "odd", "authorize")
			w.promptHook("odd, running: the memory plan and the dependencies it could not confirm", e, e.repo)
		}},
		{"prompt-removes-the-binding-of-a-closed-workflow", func(w *hookWorld) {
			e := w.env()
			e.running(w.t, "proj-1", "wf-1", "standalone-minimal")
			e.step(w.t, "proj-1", "wf-1", "close", "--outcome", "abandoned", "--reason", "done")
			w.promptHook("the prompt after the workflow was closed", e, e.repo)
			w.promptHook("the next prompt, nothing is bound any more", e, e.repo)
		}},
		{"prompt-leaves-a-binding-that-changed-meanwhile", func(w *hookWorld) {
			e := w.env()
			e.running(w.t, "proj-1", "wf-1", "standalone-minimal")
			e.create(w.t, "proj-2", "wf-2", "standalone-minimal")
			e.step(w.t, "proj-2", "wf-2", "start")
			e.step(w.t, "proj-1", "wf-1", "close", "--outcome", "abandoned", "--reason", "done")
			store, err := newBindingStore()
			if err != nil {
				w.t.Fatal(err)
			}
			key := mustRepoKey(w.t, e.repo)
			w.deps = w.deps.withBindingSeams(bindingSeams{beforeHookUnbind: func() {
				if err := store.Bind(key, "proj-2", "wf-2", time.Now(), true); err != nil {
					w.t.Errorf("Bind() from the other process = %v", err)
				}
			}})
			w.promptHook("another process binds the next workflow just before the removal", e, e.repo)
			w.promptHook("the next prompt follows the workflow bound meanwhile", e, e.repo)
		}},
		{"prompt-says-when-it-cannot-remove-the-binding", func(w *hookWorld) {
			e := w.env()
			e.running(w.t, "proj-1", "wf-1", "standalone-minimal")
			e.step(w.t, "proj-1", "wf-1", "close", "--outcome", "abandoned", "--reason", "done")
			dir := filepath.Dir(e.bindingFile(w.t, e.repo))
			if err := os.Chmod(dir, 0o500); err != nil {
				w.t.Fatal(err)
			}
			w.t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
			if f, err := os.Create(filepath.Join(dir, "probe")); err == nil {
				f.Close()
				os.Remove(filepath.Join(dir, "probe"))
				w.t.Skip("directory permissions are not enforced for this process (root or an equivalent capability); the removal cannot be made to fail")
			}
			w.promptHook("the binding cannot be removed", e, e.repo)
			w.promptHook("and the note repeats on the next prompt", e, e.repo)
		}},
		{"prompt-warns-about-a-binding-or-workflow-it-cannot-follow", func(w *hookWorld) {
			unbound := w.env()
			w.promptHook("no repository is bound: nothing to say", unbound, unbound.repo)
			for _, tc := range []struct {
				label  string
				break_ func(e hookEnv)
			}{
				{"the workflow log is gone", func(e hookEnv) { _ = os.Remove(e.workflowLog("proj-1", "wf-1")) }},
				{"the workflow log is foreign", func(e hookEnv) { writeFixtureFile(w.t, e.workflowLog("proj-1", "wf-1"), `{"hello":"world"}`+"\n") }},
				{"the workflow log is malformed", func(e hookEnv) { writeFixtureFile(w.t, e.workflowLog("proj-1", "wf-1"), "not json\n") }},
				{"the workflow log drifted", func(e hookEnv) { driftWorkflowLog(w.t, e.workflowLog("proj-1", "wf-1")) }},
				{"the binding file is foreign", func(e hookEnv) { writeFixtureFile(w.t, e.bindingFile(w.t, e.repo), `{"hello":"world"}`+"\n") }},
				{"the binding file is malformed", func(e hookEnv) { writeFixtureFile(w.t, e.bindingFile(w.t, e.repo), "hand-written notes\n") }},
			} {
				e := w.env()
				e.running(w.t, "proj-1", "wf-1", "standalone-minimal")
				tc.break_(e)
				w.promptHook(tc.label, e, e.repo)
			}
			e := w.env()
			e.running(w.t, "proj-1", "wf-1", "standalone-minimal")
			w.t.Setenv("XDG_STATE_HOME", "relative/state")
			w.promptHook("the binding store cannot be opened", e, e.repo)
		}},
		{"prompt-is-silent-for-what-it-has-nothing-to-say-about", func(w *hookWorld) {
			e := w.env()
			e.running(w.t, "proj-1", "wf-1", "standalone-minimal")
			w.promptHook("a working directory outside every repository", e, w.tempDir("<OUTSIDE>"))
			w.promptHook("a repository nothing is bound to", e, w.fixtureRepo("unbound", "<UNBOUND>"))
			w.projection("another event's input", hookArgs, strings.Replace(claudePrompt(e.repo), `"UserPromptSubmit"`, `"PreToolUse"`, 1), e.dir)
			w.projection("an event name of the wrong type", hookArgs, `{"hook_event_name":7,"cwd":"`+e.repo+`"}`, e.dir)
			w.projection("an input with no event name", hookArgs, `{"cwd":"`+e.repo+`"}`, e.dir)
		}},
		{"prompt-is-silent-for-input-it-cannot-use", func(w *hookWorld) {
			e := w.env()
			e.running(w.t, "proj-1", "wf-1", "standalone-minimal")
			for _, in := range malformedInputs {
				w.projection(in.label, hookArgs, in.stdin, e.dir)
			}
			w.projection("a cwd of the wrong type", hookArgs, `{"hook_event_name":"UserPromptSubmit","cwd":42}`, e.dir)
			w.projection("input of one byte over the size bound, in the repository of the process", hookArgs, padded(`{"hook_event_name":"UserPromptSubmit","prompt":"`, `"}`, 1<<20+1), e.repo)
			w.projectionReader("a stdin that fails", hookArgs, failingReader{}, failingStdin, e.dir)
		}},
		{"prompt-accepts-input-of-exactly-one-mebibyte", func(w *hookWorld) {
			e := w.env()
			e.running(w.t, "proj-1", "wf-1", "standalone-minimal")
			w.projection("input of exactly the size bound, in the repository of the process", hookArgs, padded(`{"hook_event_name":"UserPromptSubmit","prompt":"`, `"}`, 1<<20), e.repo)
		}},
		{"prompt-reads-the-fields-it-names-and-ignores-the-rest", func(w *hookWorld) {
			e := w.env()
			e.running(w.t, "proj-1", "wf-1", "standalone-minimal")
			w.projection("fields it does not read, of the wrong type", hookArgs, `{"session_id":{},"prompt":5,"permission_mode":[],"hook_event_name":"UserPromptSubmit","cwd":"`+e.repo+`"}`, e.dir)
			w.projection("keys in other cases", hookArgs, `{"HOOK_EVENT_NAME":"UserPromptSubmit","CWD":"`+e.repo+`"}`, e.dir)
			w.projection("an unclean cwd is cleaned", hookArgs, `{"cwd":"`+e.repo+`/./sub/../"}`, e.dir)
			w.projection("white space around the object", hookArgs, " \n"+claudePrompt(e.repo)+"\n ", e.dir)
		}},
		{"prompt-falls-back-to-the-process-directory", func(w *hookWorld) {
			e := w.env()
			e.running(w.t, "proj-1", "wf-1", "standalone-minimal")
			w.projection("no cwd in the input", hookArgs, `{"hook_event_name":"UserPromptSubmit"}`, e.repo)
			w.projection("a relative cwd is read as missing", hookArgs, `{"hook_event_name":"UserPromptSubmit","cwd":"relative/dir"}`, e.repo)
			w.projection("an empty cwd", hookArgs, `{"hook_event_name":"UserPromptSubmit","cwd":""}`, e.repo)
			w.projection("no cwd and a process directory outside every repository", hookArgs, `{"hook_event_name":"UserPromptSubmit"}`, w.t.TempDir())
		}},
		{"prompt-turns-a-panic-into-a-warning", func(w *hookWorld) {
			e := w.env()
			e.running(w.t, "proj-1", "wf-1", "standalone-minimal")
			e.step(w.t, "proj-1", "wf-1", "close", "--outcome", "abandoned", "--reason", "done")
			w.deps = w.deps.withBindingSeams(bindingSeams{beforeHookUnbind: func() { panic("boom\nsecond line \x1b[31mred " + strings.Repeat("x", 5000)) }})
			w.promptHook("a panic where the hook removes the binding", e, e.repo)
		}},
		{"prompt-exits-zero-when-stdout-cannot-be-written", func(w *hookWorld) {
			e := w.env()
			e.running(w.t, "proj-1", "wf-1", "standalone-minimal")
			w.projectionWriter("the context cannot be written", hookArgs, claudePrompt(e.repo), e.dir, failingMemoryWriter{})
		}},
		{"projection-refuses-a-command-line-it-does-not-understand", func(w *hookWorld) {
			e := w.env()
			for _, c := range []struct {
				label string
				args  []string
			}{
				{"no action at all", nil},
				{"an event flag with no action", []string{"--event", "UserPromptSubmit"}},
				{"an action that is not hook", []string{"status"}},
				{"the action with no event", []string{"hook"}},
				{"an event flag with no value", []string{"hook", "--event"}},
				{"an event the hook does not handle", []string{"hook", "--event", "Stop"}},
				{"an event written in lower case", []string{"hook", "--event", "userpromptsubmit"}},
				{"a flag the hook does not know", []string{"hook", "--bogus"}},
				{"a word after the action", []string{"hook", "extra"}},
				{"a word after the event", []string{"hook", "--event", "UserPromptSubmit", "extra"}},
			} {
				w.projection(c.label, c.args, claudePrompt(e.repo), e.dir)
			}
			w.projection("the last of a repeated event flag decides", []string{"hook", "--event", "UserPromptSubmit", "--event", "PreToolUse"},
				claudeToolCall(e.repo, "Edit", `{}`), e.dir)
		}},
	}
}
