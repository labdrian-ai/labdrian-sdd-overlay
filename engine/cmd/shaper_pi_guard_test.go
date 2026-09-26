package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/shaper"
)

// runPiGateScript copies the embedded labdrian-gate.ts bytes to a scratch
// .mjs module, as engine/runtime/pi_test.go does, and runs script under node
// with the module path substituted for %[1]q. It never loads a real Pi.
func runPiGateScript(t *testing.T, script string) string {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node unavailable; skipping labdrian-gate.ts behavior test: %v", err)
	}
	dir := t.TempDir()
	gate := filepath.Join(dir, "extensions", "labdrian-gate.mjs")
	if err := os.MkdirAll(filepath.Dir(gate), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gate, []byte(pipkg.GateExtensionSource()), 0o600); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(dir, "script.mjs")
	if err := os.WriteFile(scriptPath, []byte(fmt.Sprintf(script, gate)), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("node", scriptPath).CombinedOutput()
	if err != nil {
		t.Fatalf("node script failed: %v\n%s", err, out)
	}
	return string(out)
}

var piGuardTexts = []string{
	"gentle-ai-overlay shaper clearance record --stdin --root /r",
	"echo '{}' | ~/.claude/bin/gentle-ai-overlay shaper clearance record --stdin",
	`sh -c "gentle-ai-overlay shaper clearance record --stdin"`,
	"gentle-ai-overlay shaper   clearance\trecord --stdin",
	"gentle-ai-overlay shaper \\\n clearance \\\n record --stdin",
	"echo x > ~/.local/state/labdrian/shaper-clearance/p/g/a.json",
	"gentle-ai-overlay shaper assess --root /r --handoff h.json --goal g.json",
	"git checkout feat/shaper-clearance",
	"go test ./...",
	"",
}

// TestPiGateGuardMatchesTheGoGuard keeps the Pi tool_call guard's text
// matching identical to shaper.GuardMatches.
func TestPiGateGuardMatchesTheGoGuard(t *testing.T) {
	texts, _ := json.Marshal(piGuardTexts)
	out := runPiGateScript(t, `
const mod = await import(%[1]q);
const texts = `+string(texts)+`;
console.log(JSON.stringify(texts.map((t) => mod.matchesShaperClearanceGuard(t))));
`)
	var got []bool
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	for i, text := range piGuardTexts {
		if want := shaper.GuardMatches(text); got[i] != want {
			t.Errorf("Pi guard(%q) = %v, Go guard = %v", text, got[i], want)
		}
	}
}

// TestPiGateToolCallBlocksClearanceRecordingAndStoreWrites drives the
// default export with a fake pi and asserts the tool_call handler blocks bash
// calls naming the record entry point or the store path, and write/edit calls
// whose input.path is inside the store, mirroring shaper.RunGuardHook.
func TestPiGateToolCallBlocksClearanceRecordingAndStoreWrites(t *testing.T) {
	out := runPiGateScript(t, `
const mod = await import(%[1]q);
const handlers = {};
mod.default({ on(name, fn) { handlers[name] = fn; } });
const call = (toolName, input) => handlers.tool_call ? handlers.tool_call({ type: "tool_call", toolName, toolCallId: "1", input }, {}) : "no handler";
const results = {
  record: await call("bash", { command: "gentle-ai-overlay shaper clearance record --stdin" }),
  store: await call("bash", { command: "cat ~/.local/state/labdrian/shaper-clearance/p/g/x.json" }),
  other: await call("bash", { command: "ls" }),
  write: await call("write", { path: "/s/labdrian/shaper-clearance/p/g/x.json", content: "{}" }),
  edit: await call("edit", { path: "~/.local/state/labdrian/shaper-clearance/p/g/x.json", edits: [] }),
  writeElsewhere: await call("write", { path: "/r/notes.md", content: "labdrian/shaper-clearance" }),
  read: await call("read", { path: "/s/labdrian/shaper-clearance/x.json" }),
  noCommand: await call("bash", {}),
  beforeAgentStart: typeof handlers.before_agent_start,
};
console.log(JSON.stringify(results));
`)
	var got map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	for _, key := range []string{"record", "store", "write", "edit"} {
		var res struct {
			Block  bool   `json:"block"`
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal(got[key], &res); err != nil || !res.Block {
			t.Fatalf("%s: result %s, want {block:true}", key, got[key])
		}
		for _, want := range []string{"speed bump", "same OS user", "not a signature"} {
			if !strings.Contains(res.Reason, want) {
				t.Errorf("%s: reason %q lacks %q", key, res.Reason, want)
			}
		}
	}
	for _, key := range []string{"other", "writeElsewhere", "read", "noCommand"} {
		if raw, ok := got[key]; ok && string(raw) != "null" {
			t.Errorf("%s: result %s, want undefined", key, raw)
		}
	}
	if string(got["beforeAgentStart"]) != `"function"` {
		t.Errorf("before_agent_start handler lost: %s", got["beforeAgentStart"])
	}
}

// ---------------------------------------------------------------------------
// P3-S6: the Pi /shaper-clear dialog. The gate is driven under node with a
// fake pi and ctx; gentle-ai-overlay is a POSIX sh stub at the resolved
// $HOME/.claude/bin path that replays in-process assess output and captures
// the record stdin, which the real record core then has to accept. No real
// pi, claude, or gentle-ai-overlay binary runs.
// ---------------------------------------------------------------------------

// piClearScript drives the registered /shaper-clear handler with a fake ctx
// configured by the SHAPER_CLEAR_SCENARIO JSON environment variable, and
// prints the handler result plus every UI call in order.
const piClearScript = `
const mod = await import(%[1]q);
const sc = JSON.parse(process.env.SHAPER_CLEAR_SCENARIO);
if (sc.child) process.env.GENTLE_PI_AGENTS_CHILD = "1";
const calls = [];
let sessionId = "session-1";
let sessionFile = "/sessions/1.jsonl";
let mgr = { getSessionId() { return sessionId; }, getSessionFile() { return sessionFile; } };
if (sc.session === "none") mgr = undefined;
if (sc.session === "noGetId") mgr = { getSessionFile() { return sessionFile; } };
if (sc.session === "throws") mgr = { getSessionId() { throw new Error("no session"); }, getSessionFile() { return sessionFile; } };
if (sc.session === "emptyId") sessionId = "";
const swap = () => {
  if (sc.swapSession === "id") sessionId = "session-2";
  if (sc.swapSession === "file") sessionFile = "/sessions/2.jsonl";
  if (sc.swapSession === "manager") mgr = { getSessionId() { return sessionId; }, getSessionFile() { return sessionFile; } };
};
const inputs = sc.inputs || [];
const ctx = {
  mode: sc.mode || "tui",
  hasUI: sc.hasUI === undefined ? true : sc.hasUI,
  cwd: sc.cwd,
  get sessionManager() { return mgr; },
  ui: {
    editor: async (title, prefill) => { calls.push({ kind: "editor", title, text: prefill }); return sc.editorCancel ? undefined : prefill; },
    input: async (title) => { calls.push({ kind: "input", title }); const v = inputs.shift(); return v === null ? undefined : v; },
    confirm: async (title, message) => { calls.push({ kind: "confirm", title, text: message }); return sc.confirm === undefined ? true : sc.confirm; },
    select: async (title, options) => { calls.push({ kind: "select", title, options }); swap(); return sc.select === null ? undefined : sc.select; },
    notify: (message, type) => { calls.push({ kind: "notify", text: message, type }); },
  },
};
const handlers = {};
const commands = {};
mod.default({ on(name, fn) { handlers[name] = fn; }, registerCommand(name, opts) { commands[name] = opts; } });
const result = await commands["shaper-clear"].handler(sc.args, ctx);
console.log(JSON.stringify({ result, calls, registered: Object.keys(commands), events: Object.keys(handlers).sort() }));
`

type piClearCall struct {
	Kind    string   `json:"kind"`
	Title   string   `json:"title"`
	Text    string   `json:"text"`
	Options []string `json:"options"`
	Type    string   `json:"type"`
}

type piClearOutput struct {
	Result struct {
		Status   string `json:"status"`
		Message  string `json:"message"`
		ExitCode *int   `json:"exitCode"`
	} `json:"result"`
	Calls      []piClearCall `json:"calls"`
	Registered []string      `json:"registered"`
	Events     []string      `json:"events"`
}

// piClearFixture is one worktree plus a stub gentle-ai-overlay replaying the
// real in-process assess output for it.
type piClearFixture struct {
	root, stubDir string
	view          string
	assess        assessOutput
}

const piClearStub = `#!/bin/sh
d="$SHAPER_STUB_DIR"
{ for a in "$@"; do printf '%s\n' "$a"; done; printf '%s\n' '--end--'; } >> "$d/argv"
case "$1 $2" in
"shaper assess")
  for a in "$@"; do
    if [ "$a" = "--view" ]; then cat "$d/view"; cat "$d/view_stderr" >&2; exit "$(cat "$d/view_code")"; fi
  done
  cat "$d/assess.json"
  exit "$(cat "$d/assess_code")"
  ;;
"shaper clearance")
  cat > "$d/record.json"
  cat "$d/record_stdout"
  cat "$d/record_stderr" >&2
  exit "$(cat "$d/record_code")"
  ;;
esac
exit 64
`

func newPiClearFixture(t *testing.T, nonGoals string) piClearFixture {
	t.Helper()
	f, jsonCode, viewCode := newPiClearFixtureForGoal(t, shaperTestGoal("standalone-shaper-handoff", nonGoals))
	if jsonCode != 3 || viewCode != 3 {
		t.Fatalf("fixture assess exits %d/%d, want 3 (draft)", jsonCode, viewCode)
	}
	return f
}

// newPiClearFixtureForGoal builds a fixture for goal whose stub replays the
// real in-process assess outputs, exit codes, and --view stderr, and returns
// the real assess and assess --view exit codes.
func newPiClearFixtureForGoal(t *testing.T, goal string) (piClearFixture, int, int) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh unavailable: %v", err)
	}
	root, _ := shaperWorktree(t, shaperTestHandoff, goal)
	jsonRun := runShaperTest(assessArgs(root), "")
	viewRun := runShaperTest(assessArgs(root, "--view"), "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	stubDir := t.TempDir()
	t.Setenv("SHAPER_STUB_DIR", stubDir)
	bin := filepath.Join(home, ".claude", "bin", "gentle-ai-overlay")
	if err := os.MkdirAll(filepath.Dir(bin), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"assess.json":   jsonRun.stdout,
		"view":          viewRun.stdout,
		"view_stderr":   viewRun.stderr,
		"assess_code":   fmt.Sprint(jsonRun.code),
		"view_code":     fmt.Sprint(viewRun.code),
		"record_stdout": "shaper clearance record: stored record\n",
		"record_stderr": "",
		"record_code":   "0",
	} {
		if err := os.WriteFile(filepath.Join(stubDir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(bin, []byte(piClearStub), 0o700); err != nil {
		t.Fatal(err)
	}
	return piClearFixture{root: root, stubDir: stubDir, view: viewRun.stdout, assess: decodeAssess(t, jsonRun)}, jsonRun.code, viewRun.code
}

func (f piClearFixture) set(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.stubDir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// invocations returns each stub invocation's argv.
func (f piClearFixture) invocations(t *testing.T) [][]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.stubDir, "argv"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out [][]string
	var cur []string
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		if line == "--end--" {
			out = append(out, cur)
			cur = nil
			continue
		}
		cur = append(cur, line)
	}
	return out
}

func (f piClearFixture) recorded(t *testing.T) (string, bool) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.stubDir, "record.json"))
	if os.IsNotExist(err) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data), true
}

func runPiClear(t *testing.T, scenario map[string]any) piClearOutput {
	t.Helper()
	data, err := json.Marshal(scenario)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHAPER_CLEAR_SCENARIO", string(data))
	out := runPiGateScript(t, piClearScript)
	var got piClearOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	return got
}

func clearArgs() string { return "--handoff handoff.json --goal goal.json" }

func callKinds(calls []piClearCall) []string {
	var out []string
	for _, c := range calls {
		out = append(out, c.Kind)
	}
	return out
}

// TestPiGateShaperClearRefusesOutsideAHumanTUI: RPC, print, json, a missing
// UI, and a gentle-pi agent child are refused before any dialog or process.
func TestPiGateShaperClearRefusesOutsideAHumanTUI(t *testing.T) {
	f := newPiClearFixture(t, `["Extract jsonstrict."]`)
	for name, sc := range map[string]map[string]any{
		"rpc":   {"mode": "rpc"},
		"print": {"mode": "print", "hasUI": false},
		"json":  {"mode": "json", "hasUI": false},
		"noUI":  {"mode": "tui", "hasUI": false},
		"child": {"child": true},
	} {
		t.Run(name, func(t *testing.T) {
			sc["args"] = clearArgs()
			sc["cwd"] = f.root
			got := runPiClear(t, sc)
			if got.Result.Status != "refused" {
				t.Fatalf("status %q (%s), want refused", got.Result.Status, got.Result.Message)
			}
			for _, c := range got.Calls {
				if c.Kind != "notify" {
					t.Errorf("dialog %q opened despite refusal", c.Kind)
				}
			}
			if inv := f.invocations(t); len(inv) != 0 {
				t.Errorf("gentle-ai-overlay ran despite refusal: %v", inv)
			}
		})
	}
	if got := runPiClear(t, map[string]any{"mode": "rpc", "args": clearArgs(), "cwd": f.root}); !strings.Contains(got.Result.Message, "tui") {
		t.Errorf("rpc refusal %q does not name the TUI requirement", got.Result.Message)
	}
	if got := runPiClear(t, map[string]any{"child": true, "args": clearArgs(), "cwd": f.root}); !strings.Contains(got.Result.Message, "GENTLE_PI_AGENTS_CHILD") {
		t.Errorf("child refusal %q does not name GENTLE_PI_AGENTS_CHILD", got.Result.Message)
	}
}

// TestPiGateShaperClearAffirmRecordsWhatGoAccepts runs the whole dialog: the
// view is displayed verbatim, every flag gets a reason and evidence, the
// forgery disclosure precedes the decision, and the record piped to
// 'shaper clearance record --stdin' is accepted by the real record core.
func TestPiGateShaperClearAffirmRecordsWhatGoAccepts(t *testing.T) {
	f := newPiClearFixture(t, `["Extract jsonstrict."]`)
	if len(f.assess.Flags) == 0 {
		t.Fatal("fixture must raise at least one flag")
	}
	got := runPiClear(t, map[string]any{
		"args":   clearArgs(),
		"cwd":    f.root,
		"inputs": []string{"Intended overlap.", "Read the view."},
		"select": "Affirm",
	})
	if got.Result.Status != "recorded" || got.Result.ExitCode == nil || *got.Result.ExitCode != 0 {
		t.Fatalf("result %+v, want recorded exit 0", got.Result)
	}
	if !containsString(got.Registered, "shaper-clear") || strings.Join(got.Events, ",") != "before_agent_start,tool_call" {
		t.Errorf("registration: commands %v events %v", got.Registered, got.Events)
	}
	kinds := strings.Join(callKinds(got.Calls), ",")
	if !strings.HasPrefix(kinds, "editor,input,input,confirm,select") {
		t.Fatalf("dialog order %s, want editor,input,input,confirm,select,...", kinds)
	}
	if got.Calls[0].Text != f.view {
		t.Errorf("displayed view differs from the Go-rendered bytes:\n got %q\nwant %q", got.Calls[0].Text, f.view)
	}
	if !strings.Contains(got.Calls[1].Title, f.assess.Flags[0].ID) {
		t.Errorf("flag prompt %q does not name flag %s", got.Calls[1].Title, f.assess.Flags[0].ID)
	}
	if !strings.Contains(got.Calls[3].Text, shaper.ForgeryDisclosure) {
		t.Errorf("confirm before the decision lacks the forgery disclosure: %q", got.Calls[3].Text)
	}
	if opts := got.Calls[4].Options; len(opts) != 2 || opts[0] != "Decline" || opts[1] != "Affirm" {
		t.Errorf("decision options %v, want [Decline Affirm]", opts)
	}
	last := got.Calls[len(got.Calls)-1]
	if last.Kind != "notify" || !strings.Contains(last.Text, "exit 0") || !strings.Contains(last.Text, "stored record") {
		t.Errorf("final notification %+v does not surface the record exit status and output", last)
	}

	inv := f.invocations(t)
	if len(inv) != 3 {
		t.Fatalf("invocations %v, want assess, assess --view, record", inv)
	}
	wantSource := []string{"--root", f.root, "--handoff", "handoff.json", "--goal", "goal.json"}
	for i, want := range [][]string{
		append([]string{"shaper", "assess"}, wantSource...),
		append(append([]string{"shaper", "assess"}, wantSource...), "--view"),
		append(append([]string{"shaper", "clearance", "record"}, wantSource...), "--stdin"),
	} {
		if strings.Join(inv[i], "\x00") != strings.Join(want, "\x00") {
			t.Errorf("invocation %d argv %q, want %q", i, inv[i], want)
		}
	}

	rec, ok := f.recorded(t)
	if !ok {
		t.Fatal("no record reached the record command's stdin")
	}
	for _, forbidden := range []string{"session-1", "user", "human"} {
		if strings.Contains(rec, forbidden) {
			t.Errorf("record claims an identity (%q): %s", forbidden, rec)
		}
	}
	r := runShaperTest(recordArgs(f.root, "--stdin"), rec)
	if r.code != 0 {
		t.Fatalf("real record core refused the Pi record: exit %d stderr %q\n%s", r.code, r.stderr, rec)
	}
	after := decodeAssess(t, runShaperTest(assessArgs(f.root), ""))
	if after.Clearance.Status != "verified" || after.State != "draft" {
		t.Errorf("after the Pi record: clearance %+v state %q, want verified and draft (handoff v1 never ready)", after.Clearance, after.State)
	}
}

// TestPiGateShaperClearDeclineIsRecorded: an explicit Decline is recorded,
// and the real record core accepts it.
func TestPiGateShaperClearDeclineIsRecorded(t *testing.T) {
	f := newPiClearFixture(t, `[]`)
	got := runPiClear(t, map[string]any{"args": clearArgs(), "cwd": f.root, "select": "Decline"})
	if got.Result.Status != "recorded" {
		t.Fatalf("result %+v, want recorded", got.Result)
	}
	rec, ok := f.recorded(t)
	if !ok || !strings.Contains(rec, `"decision":"decline"`) {
		t.Fatalf("decline record %q", rec)
	}
	if r := runShaperTest(recordArgs(f.root, "--stdin"), rec); r.code != 0 {
		t.Fatalf("real record core refused the decline: exit %d stderr %q", r.code, r.stderr)
	}
}

// TestPiGateShaperClearAbortsWithoutARecord: cancelling any step, a blank
// reason or evidence, an unrecognised decision, a changed session identity,
// and a view whose digest does not match assess all end with no record.
func TestPiGateShaperClearAbortsWithoutARecord(t *testing.T) {
	cases := map[string]struct {
		scenario map[string]any
		prep     func(f piClearFixture)
		status   string
		want     string
	}{
		"viewCancelled":   {scenario: map[string]any{"editorCancel": true, "inputs": []string{"r", "e"}, "select": "Affirm"}, status: "aborted", want: "view was closed"},
		"reasonCancelled": {scenario: map[string]any{"inputs": []any{nil}, "select": "Affirm"}, status: "aborted"},
		"blankReason":     {scenario: map[string]any{"inputs": []string{"   ", "e"}, "select": "Affirm"}, status: "aborted", want: "reason"},
		"blankEvidence":   {scenario: map[string]any{"inputs": []string{"r", ""}, "select": "Affirm"}, status: "aborted", want: "evidence"},
		"disclosureNo":    {scenario: map[string]any{"inputs": []string{"r", "e"}, "confirm": false, "select": "Affirm"}, status: "aborted"},
		"decisionNone":    {scenario: map[string]any{"inputs": []string{"r", "e"}, "select": nil}, status: "aborted"},
		"decisionOther":   {scenario: map[string]any{"inputs": []string{"r", "e"}, "select": "affirm"}, status: "aborted"},
		"sessionIDSwap":   {scenario: map[string]any{"inputs": []string{"r", "e"}, "select": "Affirm", "swapSession": "id"}, status: "refused", want: "session"},
		"managerSwap":     {scenario: map[string]any{"inputs": []string{"r", "e"}, "select": "Affirm", "swapSession": "manager"}, status: "refused", want: "session"},
		"sessionFileSwap": {scenario: map[string]any{"inputs": []string{"r", "e"}, "select": "Affirm", "swapSession": "file"}, status: "refused", want: "session identity changed"},
		"viewDigestDrift": {scenario: map[string]any{"inputs": []string{"r", "e"}, "select": "Affirm"}, prep: func(f piClearFixture) {
			f.set(t, "view", f.view+"tampered\n")
		}, status: "refused", want: "view"},
		"invalidAssess": {scenario: map[string]any{"select": "Affirm"}, prep: func(f piClearFixture) {
			f.set(t, "assess_code", "2")
		}, status: "refused", want: "exit 2"},
		"missingGoalArg": {scenario: map[string]any{"args": "--handoff handoff.json", "select": "Affirm"}, status: "refused", want: "--goal"},
		"relativeRoot":   {scenario: map[string]any{"args": clearArgs() + " --root rel", "select": "Affirm"}, status: "refused", want: "absolute"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newPiClearFixture(t, `["Extract jsonstrict."]`)
			if tc.prep != nil {
				tc.prep(f)
			}
			sc := tc.scenario
			if _, ok := sc["args"]; !ok {
				sc["args"] = clearArgs()
			}
			sc["cwd"] = f.root
			got := runPiClear(t, sc)
			if got.Result.Status != tc.status {
				t.Fatalf("status %q (%s), want %q", got.Result.Status, got.Result.Message, tc.status)
			}
			if tc.want != "" && !strings.Contains(got.Result.Message, tc.want) {
				t.Errorf("message %q lacks %q", got.Result.Message, tc.want)
			}
			if rec, ok := f.recorded(t); ok {
				t.Errorf("a record was piped despite %s: %s", name, rec)
			}
		})
	}
}

// TestPiGateShaperClearSurfacesRecordFailure: a non-zero record exit is
// reported with its status and stderr, never as success.
func TestPiGateShaperClearSurfacesRecordFailure(t *testing.T) {
	f := newPiClearFixture(t, `[]`)
	f.set(t, "record_code", "1")
	f.set(t, "record_stderr", "error: shaper clearance record: immutable record differs\n")
	got := runPiClear(t, map[string]any{"args": clearArgs(), "cwd": f.root, "select": "Affirm"})
	if got.Result.Status != "failed" || got.Result.ExitCode == nil || *got.Result.ExitCode != 1 {
		t.Fatalf("result %+v, want failed exit 1", got.Result)
	}
	last := got.Calls[len(got.Calls)-1]
	if last.Kind != "notify" || last.Type != "error" || !strings.Contains(last.Text, "exit 1") || !strings.Contains(last.Text, "immutable record differs") {
		t.Errorf("final notification %+v does not surface the failure", last)
	}
}

// TestPiGateShaperClearEscapesRecordOutput: raw control, C1, and bidi bytes
// the record command writes to stdout or stderr never reach a notification
// or the result message unescaped.
func TestPiGateShaperClearEscapesRecordOutput(t *testing.T) {
	const hostile = "error: bad \x1b[8m hidden \u009b2J \u202e\n"
	for name, tc := range map[string]struct{ stdout, stderr, code string }{
		"stderrOnFailure": {"", hostile, "1"},
		"stdoutOnSuccess": {hostile, "", "0"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newPiClearFixture(t, `[]`)
			f.set(t, "record_stdout", tc.stdout)
			f.set(t, "record_stderr", tc.stderr)
			f.set(t, "record_code", tc.code)
			got := runPiClear(t, map[string]any{"args": clearArgs(), "cwd": f.root, "select": "Affirm"})
			if _, ok := f.recorded(t); !ok {
				t.Fatalf("record command did not run: %+v", got.Result)
			}
			texts := []string{got.Result.Message}
			for _, c := range got.Calls {
				if c.Kind == "notify" {
					texts = append(texts, c.Title, c.Text)
				}
			}
			var sawEscaped bool
			for _, text := range texts {
				for _, r := range text {
					if piUnpresentable(r) {
						t.Errorf("unpresentable %U reached a notification or the result: %q", r, text)
						break
					}
				}
				if strings.Contains(text, "U+001B") {
					sawEscaped = true
				}
			}
			if !sawEscaped {
				t.Errorf("no notification or result names the escaped U+001B: %q", texts)
			}
		})
	}
}

// TestPiGateShaperClearMissingBinaryFailsClosed: no gentle-ai-overlay at the
// resolved path refuses without a dialog.
func TestPiGateShaperClearMissingBinaryFailsClosed(t *testing.T) {
	f := newPiClearFixture(t, `[]`)
	if err := os.Remove(filepath.Join(os.Getenv("HOME"), ".claude", "bin", "gentle-ai-overlay")); err != nil {
		t.Fatal(err)
	}
	got := runPiClear(t, map[string]any{"args": clearArgs(), "cwd": f.root, "select": "Affirm"})
	if got.Result.Status != "refused" || !strings.Contains(got.Result.Message, "gentle-ai-overlay") {
		t.Fatalf("result %+v, want refused naming gentle-ai-overlay", got.Result)
	}
	for _, c := range got.Calls {
		if c.Kind != "notify" {
			t.Errorf("dialog %q opened without a binary", c.Kind)
		}
	}
}

// TestPiGateShaperClearDisclosureMatchesGo keeps the gate's disclosure text
// identical to shaper.ForgeryDisclosure.
func TestPiGateShaperClearDisclosureMatchesGo(t *testing.T) {
	out := runPiGateScript(t, `
const mod = await import(%[1]q);
console.log(JSON.stringify(mod.SHAPER_FORGERY_DISCLOSURE));
`)
	var got string
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if got != shaper.ForgeryDisclosure {
		t.Errorf("gate disclosure %q\nwant %q", got, shaper.ForgeryDisclosure)
	}
}

// TestPiGateShaperClearRefusesWithoutASessionIdentity: a missing session
// manager, a missing or throwing getSessionId, or an empty id is refused
// before any dialog opens or any process runs.
func TestPiGateShaperClearRefusesWithoutASessionIdentity(t *testing.T) {
	for _, mode := range []string{"none", "noGetId", "throws", "emptyId"} {
		t.Run(mode, func(t *testing.T) {
			f := newPiClearFixture(t, `["Extract jsonstrict."]`)
			got := runPiClear(t, map[string]any{"args": clearArgs(), "cwd": f.root, "session": mode, "inputs": []string{"r", "e"}, "select": "Affirm"})
			if got.Result.Status != "refused" || !strings.Contains(got.Result.Message, "no session identity") {
				t.Fatalf("result %+v, want refused for a missing session identity", got.Result)
			}
			for _, c := range got.Calls {
				if c.Kind != "notify" {
					t.Errorf("dialog %q opened without a session identity", c.Kind)
				}
			}
			if inv := f.invocations(t); len(inv) != 0 {
				t.Errorf("gentle-ai-overlay ran %d times without a session identity: %v", len(inv), inv)
			}
		})
	}
}

// TestPiGateShaperClearPassesArgumentsWithoutAShell: /shaper-clear tokens
// carrying shell metacharacters reach gentle-ai-overlay as single literal
// argv entries and never run as commands, which pins spawn's shell: false.
func TestPiGateShaperClearPassesArgumentsWithoutAShell(t *testing.T) {
	f := newPiClearFixture(t, `[]`)
	marker := func(name string) string { return filepath.Join(f.stubDir, name) }
	root := f.root + "/`touch${IFS}" + marker("tick") + "`"
	handoff := "handoff.json;touch${IFS}" + marker("semi")
	goal := "goal.json$(touch${IFS}" + marker("subst") + ")"
	got := runPiClear(t, map[string]any{
		"args":   "--root " + root + " --handoff " + handoff + " --goal " + goal,
		"cwd":    f.root,
		"select": "Affirm",
	})
	for _, name := range []string{"tick", "semi", "subst"} {
		if _, err := os.Stat(marker(name)); err == nil {
			t.Errorf("side-effect file %s exists: an argument ran as a shell command", name)
		}
	}
	if got.Result.Status != "recorded" {
		t.Fatalf("result %+v, want recorded from the stub", got.Result)
	}
	inv := f.invocations(t)
	if len(inv) != 3 {
		t.Fatalf("stub ran %d times, want 3 (assess, assess --view, record): %v", len(inv), inv)
	}
	for i, argv := range inv {
		for _, want := range [][2]string{{"--root", root}, {"--handoff", handoff}, {"--goal", goal}} {
			if !containsPair(argv, want[0], want[1]) {
				t.Errorf("invocation %d argv %q lacks the literal pair %s %q", i, argv, want[0], want[1])
			}
		}
	}
}

func containsPair(argv []string, flag, value string) bool {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == flag && argv[i+1] == value {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// P3-S6 hardening: the dialog never shows unpresentable bytes. Go refuses
// such views (P3-S6a); the gate aborts on that refusal and repeats the rune
// check itself before any dialog, and it labels prompts only with the
// Go-generated flag id and kind, never with model-authored text.
// ---------------------------------------------------------------------------

// piUnpresentable mirrors engine/shaper's unpresentable rune set for the
// assertions below.
func piUnpresentable(r rune) bool {
	switch {
	case r == '\n' || r == '\t':
		return false
	case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
		return true
	}
	return unicode.Is(unicode.Cf, r)
}

// assertNothingShownOrRecorded fails when any dialog opened, when any
// notification or the result message carries an unpresentable rune, or when
// a record reached the record command.
func assertNothingShownOrRecorded(t *testing.T, f piClearFixture, got piClearOutput) {
	t.Helper()
	for _, c := range got.Calls {
		if c.Kind != "notify" {
			t.Errorf("dialog %q opened (title %q)", c.Kind, c.Title)
		}
	}
	texts := []string{got.Result.Message}
	for _, c := range got.Calls {
		texts = append(texts, c.Title, c.Text)
	}
	for _, text := range texts {
		for _, r := range text {
			if piUnpresentable(r) {
				t.Errorf("unpresentable %U reached the UI or the result: %q", r, text)
				break
			}
		}
	}
	if rec, ok := f.recorded(t); ok {
		t.Errorf("a record was piped: %s", rec)
	}
	for _, argv := range f.invocations(t) {
		if len(argv) > 2 && argv[1] == "clearance" {
			t.Errorf("the record command ran: %q", argv)
		}
	}
}

// TestPiGateShaperClearRefusesAnESCBearingGoal: a Goal whose scope decodes to
// ESC[8m (SGR conceal) makes the real assess refuse the view; the gate replays
// that and refuses with a message naming view_unpresentable, shows no dialog,
// and records nothing.
func TestPiGateShaperClearRefusesAnESCBearingGoal(t *testing.T) {
	goal := strings.Replace(shaperTestGoal("standalone-shaper-handoff", `["Extract jsonstrict."]`),
		`"scope":"One project."`, `"scope":"One\u001b[8m hidden project."`, 1)
	f, _, viewCode := newPiClearFixtureForGoal(t, goal)
	if viewCode == 0 || f.view != "" {
		t.Fatalf("fixture: real --view exit %d stdout %q, want a refusal", viewCode, f.view)
	}
	got := runPiClear(t, map[string]any{"args": clearArgs(), "cwd": f.root, "inputs": []string{"r", "e"}, "select": "Affirm"})
	if got.Result.Status != "refused" {
		t.Fatalf("status %q (%s), want refused", got.Result.Status, got.Result.Message)
	}
	for _, want := range []string{"view_unpresentable", "nothing"} {
		if !strings.Contains(got.Result.Message, want) {
			t.Errorf("message %q lacks %q", got.Result.Message, want)
		}
	}
	assertNothingShownOrRecorded(t, f, got)
}

// TestPiGateShaperClearAbortsWhenAssessViewRefuses: when 'assess --view'
// prints nothing and names view_unpresentable, or exits with a code that
// carries no view, the gate refuses before any dialog.
func TestPiGateShaperClearAbortsWhenAssessViewRefuses(t *testing.T) {
	for name, tc := range map[string]struct {
		view, stderr, code, want string
	}{
		"refusedView":        {"", "shaper assess: refusing to print the view: view_unpresentable: view section goal_scope at rune offset 3 holds U+001B\n", "3", "view_unpresentable"},
		"exit2":              {"", "error: shaper assess: invalid\n", "2", "exit 2"},
		"exit1WithBytes":     {"# view\n", "", "1", "exit 1"},
		"stderrNamesRefusal": {"# view\n", "shaper assess: refusing to print the view: view_unpresentable: x\n", "3", "view_unpresentable"},
		"rawESCInStderr":     {"", "error: bad \x1b[8m hidden\u009b\u202e\n", "2", "exit 2"},
		"rawESCInStdout":     {"# view \x1b[8m hidden\n", "", "1", "exit 1"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newPiClearFixture(t, `["Extract jsonstrict."]`)
			f.set(t, "view", tc.view)
			f.set(t, "view_stderr", tc.stderr)
			f.set(t, "view_code", tc.code)
			got := runPiClear(t, map[string]any{"args": clearArgs(), "cwd": f.root, "inputs": []string{"r", "e"}, "select": "Affirm"})
			if got.Result.Status != "refused" || !strings.Contains(got.Result.Message, tc.want) || !strings.Contains(got.Result.Message, "nothing") {
				t.Fatalf("result %+v, want refused naming %q and that nothing was shown or recorded", got.Result, tc.want)
			}
			assertNothingShownOrRecorded(t, f, got)
		})
	}
}

// TestPiGateShaperClearRechecksViewRunesInTheGate: defense in depth. A view
// holding an unpresentable rune whose digest matches the assessed
// view_sha256 (as if Go's refusal regressed) is still refused by the gate
// before any dialog.
func TestPiGateShaperClearRechecksViewRunesInTheGate(t *testing.T) {
	for name, tc := range map[string]struct{ inject, want string }{
		"esc":         {"\x1b[8m", "unpresentable"},
		"cr":          {"\r", "unpresentable"},
		"c1csi":       {"\u009b", "unpresentable"},
		"bidi":        {"\u202e", "unpresentable"},
		"zeroWidth":   {"\u200b", "unpresentable"},
		"del":         {"\x7f", "unpresentable"},
		"invalidUTF8": {"\xff\xfe", "not valid UTF-8"},
	} {
		inject := tc.inject
		t.Run(name, func(t *testing.T) {
			f := newPiClearFixture(t, `["Extract jsonstrict."]`)
			tampered := strings.Replace(f.view, "\n", "\n"+inject+"hidden", 1)
			sum := sha256.Sum256([]byte(tampered))
			assessJSON, err := os.ReadFile(filepath.Join(f.stubDir, "assess.json"))
			if err != nil {
				t.Fatal(err)
			}
			f.set(t, "assess.json", strings.Replace(string(assessJSON), f.assess.Subject.ViewSHA256, hex.EncodeToString(sum[:]), 1))
			f.set(t, "view", tampered)
			got := runPiClear(t, map[string]any{"args": clearArgs(), "cwd": f.root, "inputs": []string{"r", "e"}, "select": "Affirm"})
			if got.Result.Status != "refused" || !strings.Contains(got.Result.Message, tc.want) {
				t.Fatalf("result %+v, want refused naming %q", got.Result, tc.want)
			}
			assertNothingShownOrRecorded(t, f, got)
		})
	}
}

// TestPiGateShaperClearLabelsPromptsOnlyWithGoFlagIDAndKind: flag prompts are
// titled with the Go-generated flag id and kind only, and no dialog title
// carries the model-authored flag item, project_id, or goal_id.
func TestPiGateShaperClearLabelsPromptsOnlyWithGoFlagIDAndKind(t *testing.T) {
	f := newPiClearFixture(t, `["Extract jsonstrict."]`)
	var raw struct {
		Flags []struct {
			ID, Kind, Item string
		} `json:"flags"`
	}
	data, err := os.ReadFile(filepath.Join(f.stubDir, "assess.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &raw); err != nil || len(raw.Flags) == 0 {
		t.Fatalf("fixture flags %+v: %v", raw.Flags, err)
	}
	got := runPiClear(t, map[string]any{"args": clearArgs(), "cwd": f.root, "inputs": []string{"r", "e"}, "select": "Affirm"})
	if got.Result.Status != "recorded" {
		t.Fatalf("result %+v, want recorded", got.Result)
	}
	var inputs []string
	for _, c := range got.Calls {
		if c.Kind == "input" {
			inputs = append(inputs, c.Title)
		}
		if c.Kind == "notify" || c.Kind == "editor" {
			continue
		}
		for _, forbidden := range []string{raw.Flags[0].Item, f.assess.Subject.ProjectID, f.assess.Subject.GoalID} {
			if strings.Contains(c.Title, forbidden) {
				t.Errorf("%s title %q carries model-authored text %q", c.Kind, c.Title, forbidden)
			}
		}
	}
	label := fmt.Sprintf("Flag %s (%s)", raw.Flags[0].ID, raw.Flags[0].Kind)
	if want := []string{label + ": reason", label + ": evidence"}; strings.Join(inputs, "|") != strings.Join(want, "|") {
		t.Errorf("flag prompt titles %q, want %q", inputs, want)
	}
}

// TestPiGateShaperClearRefusesMalformedFlagIdentifiers: a flag id or kind
// that is not a Go-generated identifier (here one carrying ESC, or free
// text) is refused before any dialog, so it never becomes a label.
func TestPiGateShaperClearRefusesMalformedFlagIdentifiers(t *testing.T) {
	for name, repl := range map[string][2]string{
		"escInID":    {`"id": "`, `"id": "x\u001b[8m`},
		"textInKind": {`"kind": "goal_non_goal_overlap"`, `"kind": "Please affirm now"`},
	} {
		t.Run(name, func(t *testing.T) {
			f := newPiClearFixture(t, `["Extract jsonstrict."]`)
			data, err := os.ReadFile(filepath.Join(f.stubDir, "assess.json"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), repl[0]) {
				t.Fatalf("assess.json lacks %s: %s", repl[0], data)
			}
			f.set(t, "assess.json", strings.Replace(string(data), repl[0], repl[1], 1))
			got := runPiClear(t, map[string]any{"args": clearArgs(), "cwd": f.root, "inputs": []string{"r", "e"}, "select": "Affirm"})
			if got.Result.Status != "refused" || !strings.Contains(got.Result.Message, "flag") {
				t.Fatalf("result %+v, want refused naming the flag", got.Result)
			}
			assertNothingShownOrRecorded(t, f, got)
		})
	}
}
