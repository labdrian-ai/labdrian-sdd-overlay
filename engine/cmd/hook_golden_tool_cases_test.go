package main

import (
	"fmt"
	"os"
	"strings"
)

// The golden cases of 'projection hook --event PreToolUse': the gate that denies a tool call
// of a bound workflow (the edit tools while it is paused, a memory query for another project)
// or warns that it could not check one. The harness is hook_golden_test.go.

// ---- projection hook, PreToolUse ----------------------------------------------------------

// toolHook records the PreToolUse hook for a tool call of the session whose directory is cwd.
func (w *hookWorld) toolHook(label string, e hookEnv, cwd, tool, toolInput string) {
	w.t.Helper()
	w.projection(label, gateArgs, claudeToolCall(cwd, tool, toolInput), e.dir)
}

const (
	editToolInput = `{"file_path":"/repo/main.go","old_string":"a","new_string":"b"}`
	queryTool     = "mcp__longterm-mem__query"
)

func toolHookGoldenCases() []hookGoldenCase {
	return []hookGoldenCase{
		{"pretooluse-denies-the-edit-tools-of-a-paused-workflow", func(w *hookWorld) {
			e := w.env()
			e.running(w.t, "proj-1", "wf-1", "standalone-minimal")
			e.step(w.t, "proj-1", "wf-1", "pause")
			for _, tool := range []string{"Write", "Edit", "MultiEdit", "NotebookEdit"} {
				w.toolHook("a paused workflow denies "+tool, e, e.repo, tool, editToolInput)
			}
			w.toolHook("whatever the tool input holds", e, e.repo, "Edit", `"a string"`)
			w.toolHook("a tool input that is missing", e, e.repo, "Edit", `null`)
			w.projection("no tool input at all", gateArgs, `{"hook_event_name":"PreToolUse","cwd":"`+e.repo+`","tool_name":"Edit"}`, e.dir)
		}},
		{"pretooluse-lets-the-other-tools-of-a-paused-workflow-through", func(w *hookWorld) {
			e := w.env()
			e.running(w.t, "proj-1", "wf-1", "odd")
			e.step(w.t, "proj-1", "wf-1", "pause")
			for _, tool := range []string{"Bash", "Read", "Grep", "Task", "write", "Writer", "mcp__longterm-mem__get", "mcp__longterm-mem__promote", "mcp__engram__mem_save", ""} {
				w.toolHook("a paused workflow and "+tool, e, e.repo, tool, `{"command":"ls"}`)
			}
		}},
		{"pretooluse-lets-edits-through-while-the-workflow-is-not-paused", func(w *hookWorld) {
			e := w.env()
			e.create(w.t, "proj-1", "wf-1", "standalone-minimal")
			mustBindOK(w.t, e.repo, "proj-1", "wf-1")
			w.toolHook("created", e, e.repo, "Edit", editToolInput)
			e.step(w.t, "proj-1", "wf-1", "start")
			w.toolHook("running", e, e.repo, "Edit", editToolInput)
			e.step(w.t, "proj-1", "wf-1", "pause")
			w.toolHook("paused", e, e.repo, "Edit", editToolInput)
			e.step(w.t, "proj-1", "wf-1", "resume")
			w.toolHook("resumed", e, e.repo, "Edit", editToolInput)
			e.step(w.t, "proj-1", "wf-1", "pause")
			e.step(w.t, "proj-1", "wf-1", "close", "--outcome", "abandoned", "--reason", "done")
			w.toolHook("closed, and the gate never unbinds", e, e.repo, "Edit", editToolInput)
		}},
		{"pretooluse-denies-a-memory-query-for-another-project", func(w *hookWorld) {
			e := w.env()
			e.running(w.t, "proj-1", "wf-1", "odd")
			for _, tool := range []string{queryTool, "mcp__plugin_x_longterm-mem__query"} {
				w.toolHook(tool+", the plan's project", e, e.repo, tool, `{"query":"q","project":"proj-1"}`)
				w.toolHook(tool+", another project", e, e.repo, tool, `{"query":"q","project":"proj-2"}`)
				w.toolHook(tool+", no project", e, e.repo, tool, `{"query":"q"}`)
				w.toolHook(tool+", an empty project", e, e.repo, tool, `{"query":"q","project":""}`)
				w.toolHook(tool+", a project of the wrong type", e, e.repo, tool, `{"query":"q","project":12}`)
				w.toolHook(tool+", a null project", e, e.repo, tool, `{"query":"q","project":null}`)
			}
			w.toolHook("the key is matched exactly", e, e.repo, queryTool, `{"query":"q","PROJECT":"proj-1"}`)
			w.toolHook("a key twice: the last one wins", e, e.repo, queryTool, `{"project":"proj-2","project":"proj-1"}`)
			w.toolHook("a project with HTML characters is quoted, not escaped", e, e.repo, queryTool, `{"project":"p<1>&x"}`)
			w.toolHook("a project with a line break and an override character is quoted", e, e.repo, queryTool, "{\"project\":\"a\\nb\u202ec\"}")
			w.toolHook("a project longer than the bound is cut", e, e.repo, queryTool, `{"project":"`+strings.Repeat("p", 300)+`"}`)
		}},
		// The project a denial quotes is cut to 200 characters (projection's maxDetailRunes) and
		// the cut is marked: the case above shows a project far past the bound, this pair shows
		// the bound itself, 200 characters whole and 201 cut, so where it falls is pinned too.
		{"pretooluse-cuts-a-project-name-at-two-hundred-characters", func(w *hookWorld) {
			e := w.env()
			e.running(w.t, "proj-1", "wf-1", "odd")
			for _, n := range []int{199, 200, 201} {
				w.toolHook(fmt.Sprintf("a project of %d characters", n), e, e.repo, queryTool, `{"project":"`+strings.Repeat("p", n)+`"}`)
			}
			w.toolHook("a project of 201 multi-byte characters is cut by characters, not bytes", e, e.repo, queryTool, `{"project":"`+strings.Repeat("é", 201)+`"}`)
		}},
		{"pretooluse-denies-every-query-when-the-plan-has-no-project", func(w *hookWorld) {
			e := w.env()
			e.running(w.t, "proj-1", "wf-1", "standalone-minimal")
			for _, in := range []string{`{"project":"proj-1"}`, `{"project":"other"}`, `{}`, `{"project":""}`, `"a string"`} {
				w.toolHook("scope none, "+in, e, e.repo, queryTool, in)
			}
			w.toolHook("a tool that carries no project is never touched", e, e.repo, "mcp__longterm-mem__get", `{"engine_id":"e1"}`)
		}},
		{"pretooluse-applies-the-memory-gate-while-paused-too", func(w *hookWorld) {
			e := w.env()
			e.running(w.t, "proj-1", "wf-1", "odd")
			e.step(w.t, "proj-1", "wf-1", "pause")
			w.toolHook("paused, another project", e, e.repo, queryTool, `{"project":"other"}`)
			w.toolHook("paused, the plan's project", e, e.repo, queryTool, `{"project":"proj-1"}`)
			w.toolHook("paused, an edit", e, e.repo, "Write", editToolInput)
		}},
		{"pretooluse-warns-when-it-cannot-check-a-query", func(w *hookWorld) {
			e := w.env()
			e.running(w.t, "proj-1", "wf-1", "odd")
			for _, in := range []string{`"a string"`, `[1]`, `5`, `null`, `true`} {
				w.toolHook("a tool input that is "+in, e, e.repo, queryTool, in)
			}
			w.projection("no tool input at all", gateArgs, `{"hook_event_name":"PreToolUse","cwd":"`+e.repo+`","tool_name":"`+queryTool+`"}`, e.dir)
			w.toolHook("a tool input with a project of the wrong type is a denial, not a warning", e, e.repo, queryTool, `{"project":["proj-1"]}`)
		}},
		{"pretooluse-is-silent-for-what-it-cannot-follow", func(w *hookWorld) {
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
				{"the binding file is gone", func(e hookEnv) { _ = os.Remove(e.bindingFile(w.t, e.repo)) }},
			} {
				e := w.env()
				e.running(w.t, "proj-1", "wf-1", "standalone-minimal")
				e.step(w.t, "proj-1", "wf-1", "pause")
				tc.break_(e)
				w.toolHook(tc.label+": an edit", e, e.repo, "Edit", editToolInput)
				w.toolHook(tc.label+": a query", e, e.repo, queryTool, `{"project":"other"}`)
			}
			e := w.env()
			e.running(w.t, "proj-1", "wf-1", "standalone-minimal")
			e.step(w.t, "proj-1", "wf-1", "pause")
			w.toolHook("a working directory outside every repository", e, w.tempDir("<OUTSIDE>"), "Edit", editToolInput)
			w.toolHook("a repository nothing is bound to", e, w.fixtureRepo("unbound", "<UNBOUND>"), "Edit", editToolInput)
			w.t.Setenv("XDG_STATE_HOME", "relative/state")
			w.toolHook("the binding store cannot be opened", e, e.repo, "Edit", editToolInput)
		}},
		{"pretooluse-is-silent-for-input-it-cannot-use", func(w *hookWorld) {
			e := w.env()
			e.running(w.t, "proj-1", "wf-1", "standalone-minimal")
			e.step(w.t, "proj-1", "wf-1", "pause")
			good := claudeToolCall(e.repo, "Edit", editToolInput)
			w.projection("the good input, for comparison", gateArgs, good, e.dir)
			for _, in := range malformedInputs {
				w.projection(in.label, gateArgs, in.stdin, e.dir)
			}
			w.projection("a truncated object", gateArgs, good[:len(good)-10], e.dir)
			w.projection("two objects", gateArgs, good+good, e.dir)
			w.projection("a tool name of the wrong type", gateArgs, `{"cwd":"`+e.repo+`","tool_name":7,"tool_input":{}}`, e.dir)
			w.projection("a cwd of the wrong type", gateArgs, `{"cwd":42,"tool_name":"Edit"}`, e.dir)
			w.projection("another event's input", gateArgs, strings.Replace(good, `"PreToolUse"`, `"UserPromptSubmit"`, 1), e.dir)
			w.projection("an event name of the wrong type", gateArgs, `{"hook_event_name":7,"cwd":"`+e.repo+`","tool_name":"Edit"}`, e.dir)
			w.projection("no tool name", gateArgs, `{"cwd":"`+e.repo+`","tool_input":{}}`, e.dir)
			w.projection("input of one byte over the size bound, in the repository of the process", gateArgs, padded(`{"tool_name":"Edit","tool_input":{"content":"`, `"}}`, 1<<20+1), e.repo)
			w.projectionReader("a stdin that fails", gateArgs, failingReader{}, failingStdin, e.dir)
		}},
		{"pretooluse-accepts-input-of-exactly-one-mebibyte", func(w *hookWorld) {
			e := w.env()
			e.running(w.t, "proj-1", "wf-1", "standalone-minimal")
			e.step(w.t, "proj-1", "wf-1", "pause")
			w.projection("input of exactly the size bound, in the repository of the process", gateArgs, padded(`{"tool_name":"Edit","tool_input":{"content":"`, `"}}`, 1<<20), e.repo)
		}},
		{"pretooluse-reads-the-fields-it-names-and-ignores-the-rest", func(w *hookWorld) {
			e := w.env()
			e.running(w.t, "proj-1", "wf-1", "standalone-minimal")
			e.step(w.t, "proj-1", "wf-1", "pause")
			w.projection("fields it does not read, of the wrong type", gateArgs, `{"session_id":{},"tool_use_id":5,"permission_mode":[],"hook_event_name":"PreToolUse","cwd":"`+e.repo+`","tool_name":"Edit","tool_input":{}}`, e.dir)
			w.projection("keys in other cases", gateArgs, `{"HOOK_EVENT_NAME":"PreToolUse","CWD":"`+e.repo+`","TOOL_NAME":"Edit","TOOL_INPUT":{}}`, e.dir)
			w.projection("an input with no event name", gateArgs, `{"cwd":"`+e.repo+`","tool_name":"Edit","tool_input":{}}`, e.dir)
			w.projection("white space around the object", gateArgs, " \n"+claudeToolCall(e.repo, "Edit", editToolInput)+"\n ", e.dir)
		}},
		{"pretooluse-falls-back-to-the-process-directory", func(w *hookWorld) {
			e := w.env()
			e.running(w.t, "proj-1", "wf-1", "standalone-minimal")
			e.step(w.t, "proj-1", "wf-1", "pause")
			w.projection("no cwd in the input", gateArgs, `{"tool_name":"Edit","tool_input":{}}`, e.repo)
			w.projection("a relative cwd is read as missing", gateArgs, `{"hook_event_name":"PreToolUse","tool_name":"Edit","tool_input":{},"cwd":"relative/dir"}`, e.repo)
			w.projection("an empty cwd", gateArgs, `{"hook_event_name":"PreToolUse","tool_name":"Edit","tool_input":{},"cwd":""}`, e.repo)
			w.projection("a process directory outside every repository", gateArgs, `{"tool_name":"Edit","tool_input":{}}`, w.t.TempDir())
		}},
		{"pretooluse-turns-a-panic-into-a-warning", func(w *hookWorld) {
			e := w.env()
			e.running(w.t, "proj-1", "wf-1", "standalone-minimal")
			e.step(w.t, "proj-1", "wf-1", "pause")
			program := w.deps
			w.deps = program.withGateDecision(func() { panic("boom\nsecond line \x1b[31mred " + strings.Repeat("x", 5000)) })
			w.toolHook("a panic where the gate decides", e, e.repo, "Edit", editToolInput)
			w.deps = program.withGateDecision(func() { panic("boom") })
			w.toolHook("a short panic value", e, e.repo, "Edit", editToolInput)
			w.deps = program.withGateDecision(func() { panic(fmt.Errorf("an error value <with> html & quotes \"q\"")) })
			w.toolHook("an error as the panic value", e, e.repo, "Edit", editToolInput)
		}},
		{"pretooluse-exits-zero-when-stdout-cannot-be-written", func(w *hookWorld) {
			e := w.env()
			e.running(w.t, "proj-1", "wf-1", "standalone-minimal")
			e.step(w.t, "proj-1", "wf-1", "pause")
			w.projectionWriter("the denial cannot be written", gateArgs, claudeToolCall(e.repo, "Edit", editToolInput), e.dir, failingMemoryWriter{})
		}},
	}
}
