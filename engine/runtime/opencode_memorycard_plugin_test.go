package runtime_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// The memorycard plugin (S11) is run under node against a fake `memorycard` program: the fake records
// each call (its arguments, its working directory and what it read on stdin) so that a test sees what
// the plugin asked of memorycard, and nothing reaches a real memory store.

func memorycardPluginSource(t *testing.T) []byte {
	t.Helper()
	_, callerPath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(callerPath), "memorycard-opencode-plugin.mjs"))
	if err != nil {
		t.Fatalf("read the memorycard plugin source: %v", err)
	}
	return source
}

// memorycardFixture is a config root laid out as the adapter lays it out, with the memorycard plugin
// at its v2 path, the stub @opencode/plugin package above it and a fake memorycard.
type memorycardFixture struct {
	root       string
	pluginPath string
	calls      string // the file the fake appends one line to per call
	stdin      string // the file the fake writes what `capture` read to
	fake       string // the fake program
	project    string // the directory OpenCode reports for the session
}

const fakeMemorycardScript = `#!/bin/sh
printf '%s|%s\n' "$1" "$(pwd)" >> "$FAKE_CALLS"
case "$1" in
context)
	case "$FAKE_MODE" in
	fail) echo "broken" >&2; exit 3 ;;
	hang) sleep 30 ;;
	empty) exit 0 ;;
	esac
	printf 'FAKE-CONTEXT %s\n' "$*"
	;;
capture)
	case "$FAKE_MODE" in
	fail) echo "broken" >&2; exit 3 ;;
	hang) sleep 30 ;;
	esac
	cat > "$FAKE_STDIN"
	;;
esac
`

func newMemorycardFixture(t *testing.T) memorycardFixture {
	t.Helper()
	requireNode(t)
	scratch := t.TempDir()
	root := filepath.Join(scratch, "config")
	pluginPath := filepath.Join(root, "plugins", "memorycard", "index.js")
	if err := os.MkdirAll(filepath.Dir(pluginPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pluginPath, memorycardPluginSource(t), 0o644); err != nil {
		t.Fatal(err)
	}
	stub := filepath.Join(root, "node_modules", "@opencode", "plugin")
	if err := os.MkdirAll(stub, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range stubPluginPackage {
		if err := os.WriteFile(filepath.Join(stub, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bin := filepath.Join(scratch, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(bin, "memorycard")
	if err := os.WriteFile(fake, []byte(fakeMemorycardScript), 0o755); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(scratch, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	return memorycardFixture{root: root, pluginPath: pluginPath, calls: filepath.Join(scratch, "calls"), stdin: filepath.Join(scratch, "stdin"), fake: fake, project: project}
}

// writeRecord writes the record the adapter writes beside the plugin, naming the program to run.
func (f memorycardFixture) writeRecord(t *testing.T, program string) {
	t.Helper()
	record, err := json.Marshal(map[string]any{
		"installed_version":   "test-version",
		"installed_hash":      "test-hash",
		"plugin_path":         f.pluginPath,
		"plugin_config_root":  f.root,
		"memorycard_command":  program,
		"activation_marker":   filepath.Join(f.root, "memorycard-opencode.active.json"),
		"plugin_config_scope": "global-opencode-config",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.root, "memorycard-opencode.json"), record, 0o644); err != nil {
		t.Fatal(err)
	}
}

// run loads the plugin under node with a stub context whose location is the project directory, runs its
// setup and then body, which sees plugin, hooks (name to handler) and ctx. The environment has the
// fake's files and extra; the fake is not on PATH unless extra puts it there.
func (f memorycardFixture) run(t *testing.T, extra []string, body string) string {
	t.Helper()
	script := fmt.Sprintf(`const plugin = (await import(%q)).default;
const hooks = new Map();
const hook = async (name, handler) => { hooks.set(name, handler); return { dispose: async () => {} }; };
const ctx = { location: { directory: %q }, tool: { hook }, session: { hook } };
await plugin.setup(ctx);
%s`, "file://"+f.pluginPath, f.project, body)
	scriptPath := filepath.Join(t.TempDir(), "memorycard-plugin-behavior.mjs")
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("node", scriptPath)
	command.Env = append(os.Environ(), "FAKE_CALLS="+f.calls, "FAKE_STDIN="+f.stdin)
	command.Env = append(command.Env, extra...)
	out, err := command.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("node exited with error: %v\n%s", err, ee.Stderr)
		}
		t.Fatalf("node exec failed: %v", err)
	}
	return string(out)
}

// callsMade are the fake's calls, one "<command>|<working directory>" per line.
func (f memorycardFixture) callsMade(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(f.calls)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func (f memorycardFixture) resolvedProject(t *testing.T) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(f.project)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func TestMemorycardPluginIsAV2PluginCalledMemorycard(t *testing.T) {
	f := newMemorycardFixture(t)
	f.writeRecord(t, f.fake)
	out := f.run(t, nil, `console.log(JSON.stringify({ id: plugin.id, hooks: [...hooks.keys()].sort() }));`)
	if want := `{"id":"memorycard","hooks":["context","execute.after"]}`; strings.TrimSpace(out) != want {
		t.Errorf("plugin = %s, want %s", strings.TrimSpace(out), want)
	}
}

func TestMemorycardPluginPutsTheMemorycardContextOnTheSystemPromptOfANewSession(t *testing.T) {
	f := newMemorycardFixture(t)
	f.writeRecord(t, f.fake)
	out := f.run(t, nil, `const event = { sessionID: "ses_1", system: [] };
await hooks.get("context")(event);
console.log(JSON.stringify(event.system));`)
	want := `[{"type":"text","text":"FAKE-CONTEXT context"}]`
	if strings.TrimSpace(out) != want {
		t.Errorf("system = %s, want %s", strings.TrimSpace(out), want)
	}
	calls := f.callsMade(t)
	if len(calls) != 1 || calls[0] != "context|"+f.resolvedProject(t) {
		t.Errorf("calls = %v, want one context call run in the session's directory %s", calls, f.resolvedProject(t))
	}
}

func TestMemorycardPluginAsksOncePerSessionAndPushesBeforeEveryModelCall(t *testing.T) {
	f := newMemorycardFixture(t)
	f.writeRecord(t, f.fake)
	out := f.run(t, nil, `const first = { sessionID: "ses_1", system: [] };
const again = { sessionID: "ses_1", system: [] };
const other = { sessionID: "ses_2", system: [] };
await hooks.get("context")(first);
await hooks.get("context")(again);
await hooks.get("context")(other);
console.log(JSON.stringify([first.system.length, again.system.length, other.system.length]));`)
	if strings.TrimSpace(out) != "[1,1,1]" {
		t.Errorf("pushed = %s, want one text on every model call", strings.TrimSpace(out))
	}
	if calls := f.callsMade(t); len(calls) != 2 {
		t.Errorf("calls = %v, want one context call per session (two sessions)", calls)
	}
}

func TestMemorycardPluginPushesNothingWhenMemorycardHasNoContext(t *testing.T) {
	f := newMemorycardFixture(t)
	f.writeRecord(t, f.fake)
	out := f.run(t, []string{"FAKE_MODE=empty"}, `const event = { sessionID: "ses_1", system: [] };
await hooks.get("context")(event);
console.log(JSON.stringify(event.system));`)
	if strings.TrimSpace(out) != "[]" {
		t.Errorf("system = %s, want nothing pushed", strings.TrimSpace(out))
	}
}

func TestMemorycardPluginCapturesTheFinalTextOfASubagentOnStdin(t *testing.T) {
	f := newMemorycardFixture(t)
	f.writeRecord(t, f.fake)
	f.run(t, nil, `await hooks.get("execute.after")({ id: "call_1", tool: "subagent", sessionID: "ses_1", status: "completed", input: { agent: "builder", prompt: "go" }, result: { output: "done\n\n## Key Learnings\n1. A learning that is long enough to be kept by memorycard.\n", content: [] } });`)
	got, err := os.ReadFile(f.stdin)
	if err != nil {
		t.Fatalf("capture read nothing: %v", err)
	}
	if want := "done\n\n## Key Learnings\n1. A learning that is long enough to be kept by memorycard.\n"; string(got) != want {
		t.Errorf("capture read %q, want the subagent output exactly %q", got, want)
	}
	if calls := f.callsMade(t); len(calls) != 1 || calls[0] != "capture|"+f.resolvedProject(t) {
		t.Errorf("calls = %v, want one capture call run in the session's directory", calls)
	}
}

func TestMemorycardPluginCapturesTheTextPartsOfASubagentResultWithNoOutput(t *testing.T) {
	f := newMemorycardFixture(t)
	f.writeRecord(t, f.fake)
	f.run(t, nil, `await hooks.get("execute.after")({ id: "call_1", tool: "subagent", sessionID: "ses_1", status: "completed", input: {}, result: { content: [{ type: "text", text: "first" }, { type: "image", data: "x" }, { type: "text", text: "second" }] } });`)
	got, err := os.ReadFile(f.stdin)
	if err != nil {
		t.Fatalf("capture read nothing: %v", err)
	}
	if string(got) != "first\nsecond" {
		t.Errorf("capture read %q, want the text parts joined by a newline", got)
	}
}

func TestMemorycardPluginIgnoresEveryCallThatIsNotACompletedSubagentWithText(t *testing.T) {
	f := newMemorycardFixture(t)
	f.writeRecord(t, f.fake)
	f.run(t, nil, `const after = hooks.get("execute.after");
await after({ id: "a", tool: "shell", sessionID: "s", status: "completed", input: {}, result: { output: "## Key Learnings\n1. not from a subagent at all, so never captured." } });
await after({ id: "b", tool: "subagent", sessionID: "s", status: "error", input: {}, error: new Error("boom"), result: undefined });
await after({ id: "c", tool: "subagent", sessionID: "s", status: "completed", input: {}, result: { output: "", content: [] } });
await after({ id: "d", tool: "subagent", sessionID: "s", status: "completed", input: {}, result: { output: "   \n" } });
await after(undefined);
await after({ tool: "subagent", status: "completed", result: { output: 7 } });`)
	if calls := f.callsMade(t); len(calls) != 0 {
		t.Errorf("calls = %v, want none", calls)
	}
}

func TestMemorycardPluginSwallowsEveryFailureOfMemorycard(t *testing.T) {
	cases := map[string]struct {
		mode    string
		program func(f memorycardFixture) string
		env     []string
	}{
		"binary absent":  {program: func(f memorycardFixture) string { return filepath.Join(f.root, "no-memorycard") }},
		"non-zero exit":  {mode: "fail", program: func(f memorycardFixture) string { return f.fake }},
		"timeout":        {mode: "hang", program: func(f memorycardFixture) string { return f.fake }, env: []string{"LABDRIAN_MEMORYCARD_TIMEOUT_MS=300"}},
		"not executable": {program: func(f memorycardFixture) string { return f.root }},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newMemorycardFixture(t)
			f.writeRecord(t, c.program(f))
			env := append([]string{"FAKE_MODE=" + c.mode}, c.env...)
			out := f.run(t, env, `const event = { sessionID: "ses_1", system: [] };
await hooks.get("context")(event);
await hooks.get("execute.after")({ id: "call_1", tool: "subagent", sessionID: "ses_1", status: "completed", input: {}, result: { output: "text" } });
console.log(JSON.stringify(event.system));`)
			if strings.TrimSpace(out) != "[]" {
				t.Errorf("system = %s, want nothing pushed and no failure thrown", strings.TrimSpace(out))
			}
		})
	}
}

func TestMemorycardPluginDoesNotAskAFailingMemorycardAgainAtOnce(t *testing.T) {
	f := newMemorycardFixture(t)
	f.writeRecord(t, f.fake)
	f.run(t, []string{"FAKE_MODE=fail"}, `for (let i = 0; i < 3; i++) await hooks.get("context")({ sessionID: "ses_1", system: [] });`)
	if calls := f.callsMade(t); len(calls) != 1 {
		t.Errorf("calls = %v, want one: a failure pauses the question for the session", calls)
	}
}

func TestMemorycardPluginRunsTheBareNameFromThePathWhenThereIsNoRecord(t *testing.T) {
	f := newMemorycardFixture(t)
	path := "PATH=" + filepath.Dir(f.fake) + string(os.PathListSeparator) + os.Getenv("PATH")
	out := f.run(t, []string{path}, `const event = { sessionID: "ses_1", system: [] };
await hooks.get("context")(event);
console.log(JSON.stringify(event.system));`)
	if want := `[{"type":"text","text":"FAKE-CONTEXT context"}]`; strings.TrimSpace(out) != want {
		t.Errorf("system = %s, want %s", strings.TrimSpace(out), want)
	}
}

func TestMemorycardPluginWritesItsActiveMarker(t *testing.T) {
	f := newMemorycardFixture(t)
	f.writeRecord(t, f.fake)
	f.run(t, nil, ``)
	data, err := os.ReadFile(filepath.Join(f.root, "memorycard-opencode.active.json"))
	if err != nil {
		t.Fatalf("the plugin wrote no active marker: %v", err)
	}
	var marker map[string]string
	if err := json.Unmarshal(data, &marker); err != nil {
		t.Fatalf("active marker is not JSON: %v\n%s", err, data)
	}
	wantHash := regexp.MustCompile(`^[0-9a-f]{64}$`)
	if marker["active_version"] != "test-version" || !wantHash.MatchString(marker["active_hash"]) || marker["plugin_path"] != f.pluginPath || marker["config_root"] != f.root {
		t.Errorf("active marker = %v, want the record's version, the sha256 of the plugin file, its path and the config root", marker)
	}
}

func TestMemorycardPluginSourceImportsOnlyWhatTheRealPluginModuleExportsAndNamesNoEngram(t *testing.T) {
	source := string(memorycardPluginSource(t))
	for _, line := range strings.Split(source, "\n") {
		if strings.Contains(line, "@opencode/") && !strings.HasPrefix(line, "import { Plugin } from \"@opencode/plugin\";") {
			t.Errorf("the plugin imports %q; OpenCode 2.2.7 exports Plugin from @opencode/plugin, and the tool module only CallID and Error", line)
		}
	}
	if strings.Contains(strings.ToLower(source), "engram") {
		t.Error("the memorycard plugin names Engram; S11 wants no Engram call")
	}
}
