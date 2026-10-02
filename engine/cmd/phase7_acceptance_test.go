package main

// Phase 7 (runtime adapters) acceptance fixtures. Each criterion of
// odd/tasks/runtime-adapters.md's "Acceptance criteria" is proved end to end
// against the BUILT engine binary: separate OS processes, realistic Claude Code
// hook JSON on stdin, a hand-made repository (never the git binary), and an
// isolated HOME and XDG_STATE_HOME under t.TempDir(). No test here can reach
// ~/.claude, ~/.pi, ~/.codex, ~/.local/state/labdrian, or Engram: every child
// process gets an explicit, minimal environment (HOME, XDG_STATE_HOME, and a PATH
// that is a directory of the test's own).
//
// Criterion -> test. The subtests share one build of the binary, so they live
// under one top-level test, TestPhase7Acceptance.
//
//	typed capability declarations     /Capabilities_AreTypedDeclarationsWithLimits
//	no os/exec or network in Phase 7  (static, not repeated here)
//	                                  engine/runtime: TestPhase7PackagesImportNoExecOrNetwork,
//	                                  TestRuntimeExecAllowlist
//	unbound sessions pass through     /Unbound_HooksAreASilentPassthrough
//	bound and running                 /Running_ProjectsTheWorkflowWithinTheBound
//	bound and paused, closed          /Paused_DeniesEditToolsAndClosedNeverGates
//	memory gate                       /MemoryGate_ChecksTheProjectOfQueriesOnly
//	restart, from any worktree        /Restart_AFreshProcessInAnyWorktreeProjectsTheSameState
//	                                  (the built binary against the in-process hook:
//	                                  TestProjectionHookOutputIsIdenticalAcrossProcesses)
//	unusable state                    /UnusableState_ProjectsNothingWarnsOnceDeniesNothing
//	presence prober                   /PresenceProber_RecordsPresenceByStatOnly
//	                                  (its source opens no file, static:
//	                                  capability/presence: TestPresenceProberSourceOnlyStats,
//	                                  TestRuntimeProbeSourceOpensNoFile)
//	settings family                   /Settings_MergeIsIdempotentAndPreservesForeignEntries
//	live session (RA8)                not a fixture: one authorized real prompt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capabilitytest"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"
)

// --- harness ----------------------------------------------------------------

// phase7World is one isolated world: a home and a state home, a PATH of its own,
// a scratch directory, and a hand-made repository the session works in.
type phase7World struct {
	t      *testing.T
	binary string
	home   string // HOME of every child process
	state  string // XDG_STATE_HOME of every child process
	bin    string // PATH of every child process: empty unless a criterion puts a file on it
	dir    string // scratch directory: Goal files, and the working directory of most processes
	repo   string // the repository the session works in
}

func newPhase7World(t *testing.T, binary string) phase7World {
	t.Helper()
	home, state := phase6IsolatedHome(t)
	return phase7World{t: t, binary: binary, home: home, state: state, bin: t.TempDir(), dir: t.TempDir(), repo: fixtureRepo(t, "repo")}
}

// phase7Run is one finished process.
type phase7Run struct {
	code   int
	stdout string
	stderr string
}

// asHook adapts a finished process to the hook assertions of the in-process
// tests, which take a list of exit calls; a real process exits once.
func (r phase7Run) asHook() hookRun {
	return hookRun{codes: []int{r.code}, stdout: r.stdout, stderr: r.stderr}
}

// must fails the test unless the process exited 0.
func (r phase7Run) must(t *testing.T, label string) phase7Run {
	t.Helper()
	if r.code != 0 {
		t.Fatalf("%s: exit %d, stdout %q, stderr %q, want exit 0", label, r.code, r.stdout, r.stderr)
	}
	return r
}

func (w phase7World) env() []string {
	return []string{"HOME=" + w.home, "XDG_STATE_HOME=" + w.state, "PATH=" + w.bin}
}

// run starts one process with the world's environment, stdin as given, and cwd
// as its working directory, and waits for it (at most a minute).
func (w phase7World) run(cwd, stdin, name string, args ...string) phase7Run {
	w.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = cwd
	cmd.Env = w.env()
	cmd.Stdin = strings.NewReader(stdin)
	var out, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errBuf
	code := 0
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			w.t.Fatalf("run %s %v: %v", name, args, err)
		}
		code = exitErr.ExitCode()
	}
	return phase7Run{code: code, stdout: out.String(), stderr: errBuf.String()}
}

// engine runs the built engine binary.
func (w phase7World) engine(cwd, stdin string, args ...string) phase7Run {
	w.t.Helper()
	return w.run(cwd, stdin, w.binary, args...)
}

// verb runs 'workflow <verb>' for the workflow, with more flags, and requires
// exit 0.
func (w phase7World) verb(project, wf, verb string, flags ...string) phase7Run {
	w.t.Helper()
	args := append([]string{"workflow", verb, "--project", project, "--workflow", wf}, flags...)
	return w.engine(w.dir, "", args...).must(w.t, "workflow "+verb)
}

func (w phase7World) createWorkflow(project, wf, profile string) {
	w.t.Helper()
	goal := writeMemoryTestFile(w.t, w.dir, project+"-"+wf+"-goal.json", memoryTestGoalJSON(project, "goal-1"))
	w.verb(project, wf, "create", "--goal", goal, "--profile", profile)
}

func (w phase7World) goalPath(project, wf string) string {
	return filepath.Join(w.dir, project+"-"+wf+"-goal.json")
}

// bindRunning creates a workflow, starts it, records the stages, and binds the
// repository to it, every step in its own process.
func (w phase7World) bindRunning(project, wf, profile string, stages ...string) {
	w.t.Helper()
	w.createWorkflow(project, wf, profile)
	w.verb(project, wf, "start")
	for _, stage := range stages {
		w.verb(project, wf, "stage", "--stage", stage)
	}
	w.engine(w.repo, "", "workflow", "bind", "--project", project, "--workflow", wf).must(w.t, "workflow bind")
}

// promptHook is what Claude Code runs when the user submits a prompt from cwd.
func (w phase7World) promptHook(cwd string) phase7Run {
	w.t.Helper()
	return w.engine(w.dir, hookInput(w.t, cwd, "session-1", "please continue with the task"), "projection", "hook", "--event", "UserPromptSubmit")
}

// toolHook is what Claude Code runs before the session uses a tool from cwd.
func (w phase7World) toolHook(cwd, tool string, input any) phase7Run {
	w.t.Helper()
	return w.engine(w.dir, gateInput(w.t, cwd, "session-1", tool, input), "projection", "hook", "--event", "PreToolUse")
}

// binding runs 'workflow binding' from cwd and decodes its report.
func (w phase7World) binding(cwd string) bindingReportJSON {
	w.t.Helper()
	r := w.engine(cwd, "", "workflow", "binding").must(w.t, "workflow binding")
	var report bindingReportJSON
	if err := json.Unmarshal([]byte(r.stdout), &report); err != nil {
		w.t.Fatalf("workflow binding printed %q: %v", r.stdout, err)
	}
	return report
}

// stateSnapshot is the content of every file under the state home.
func (w phase7World) stateSnapshot() map[string]string { return snapshotContents(w.t, w.state) }

// tool inputs as Claude Code sends them.
var (
	phase7EditInputs = map[string]any{
		"Write":        map[string]any{"file_path": "/work/repo/main.go", "content": "package main\n"},
		"Edit":         map[string]any{"file_path": "/work/repo/main.go", "old_string": "a", "new_string": "b"},
		"MultiEdit":    map[string]any{"file_path": "/work/repo/main.go", "edits": []any{map[string]any{"old_string": "a", "new_string": "b"}}},
		"NotebookEdit": map[string]any{"notebook_path": "/work/repo/analysis.ipynb", "new_source": "print(1)"},
	}
	phase7OrderedEditTools = []string{"Write", "Edit", "MultiEdit", "NotebookEdit"}

	phase7OtherInputs = map[string]any{
		"Bash":      map[string]any{"command": "go test ./..."},
		"Read":      map[string]any{"file_path": "/work/repo/main.go"},
		"Grep":      map[string]any{"pattern": "TODO"},
		"Glob":      map[string]any{"pattern": "**/*.go"},
		"Task":      map[string]any{"description": "explore", "prompt": "look around"},
		"WebFetch":  map[string]any{"url": "https://example.com", "prompt": "summarize"},
		"TodoWrite": map[string]any{"todos": []any{}},
	}
	phase7OrderedOtherTools = []string{"Bash", "Read", "Grep", "Glob", "Task", "WebFetch", "TodoWrite"}
)

// assertToolsSilent requires each of the tools to be allowed without a word:
// exit 0, no output on either stream.
func assertToolsSilent(t *testing.T, w phase7World, label string, tools []string, inputs map[string]any) {
	t.Helper()
	for _, tool := range tools {
		assertSilent(t, label+" "+tool, w.toolHook(w.repo, tool, inputs[tool]).asHook())
	}
}

// --- the acceptance test ----------------------------------------------------

// TestPhase7Acceptance builds the engine binary once and drives it through every
// acceptance criterion of Phase 7. Safety: every process runs with HOME and
// XDG_STATE_HOME set to this test's own temporary directories.
func TestPhase7Acceptance(t *testing.T) {
	binary := phase6BuildEngineBinary(t)
	for _, c := range []struct {
		name string
		run  func(*testing.T, string)
	}{
		{"Capabilities_AreTypedDeclarationsWithLimits", phase7Capabilities},
		{"Unbound_HooksAreASilentPassthrough", phase7Unbound},
		{"Running_ProjectsTheWorkflowWithinTheBound", phase7Running},
		{"Paused_DeniesEditToolsAndClosedNeverGates", phase7Paused},
		{"MemoryGate_ChecksTheProjectOfQueriesOnly", phase7MemoryGate},
		{"Restart_AFreshProcessInAnyWorktreeProjectsTheSameState", phase7Restart},
		{"UnusableState_ProjectsNothingWarnsOnceDeniesNothing", phase7UnusableState},
		{"PresenceProber_RecordsPresenceByStatOnly", phase7PresenceProber},
		{"Settings_MergeIsIdempotentAndPreservesForeignEntries", phase7Settings},
	} {
		t.Run(c.name, func(t *testing.T) { c.run(t, binary) })
	}
}

// --- typed capability declarations -------------------------------------------

func phase7DecodeReport(t *testing.T, stdout string) capability.Report {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(stdout))
	dec.DisallowUnknownFields()
	var report capability.Report
	if err := dec.Decode(&report); err != nil {
		t.Fatalf("stdout is not a capability report: %v\n%s", err, stdout)
	}
	if dec.More() {
		t.Fatalf("stdout holds more than one JSON value:\n%s", stdout)
	}
	return report
}

func phase7Capabilities(t *testing.T, binary string) {
	w := newPhase7World(t, binary)
	run := w.engine(w.dir, "", "runtime", "capabilities").must(t, "runtime capabilities")
	if run.stderr != "" {
		t.Errorf("stderr = %q, want none", run.stderr)
	}
	all := phase7DecodeReport(t, run.stdout)
	wantTargets := []string{capability.TargetClaude, capability.TargetCodex, capability.TargetPi, capability.TargetOpenCode}
	var gotTargets []string
	for _, d := range all.Declarations {
		gotTargets = append(gotTargets, d.Target)
	}
	if all.Version != 1 || !reflect.DeepEqual(gotTargets, wantTargets) {
		t.Fatalf("report version %d, targets %v, want version 1 and %v", all.Version, gotTargets, wantTargets)
	}

	// Typed and evidenced: every declaration the binary prints is valid, and every
	// supported or partial claim names a test that exists in this repository.
	for _, d := range all.Declarations {
		if err := capability.Validate(d); err != nil {
			t.Errorf("%s: printed declaration is invalid: %v", d.Target, err)
		}
		if err := capability.CheckEvidence(capabilitytest.NewCatalog(os.DirFS("..")), d); err != nil {
			t.Errorf("%s: printed declaration cites a test that does not exist: %v", d.Target, err)
		}
	}

	// --target selects exactly one of them.
	for _, d := range all.Declarations {
		single := phase7DecodeReport(t, w.engine(w.dir, "", "runtime", "capabilities", "--target", d.Target).must(t, "--target "+d.Target).stdout)
		if len(single.Declarations) != 1 || !reflect.DeepEqual(single.Declarations[0], d) {
			t.Errorf("--target %s printed %+v, want the declaration `all` prints", d.Target, single.Declarations)
		}
	}
	if r := w.engine(w.dir, "", "runtime", "capabilities", "--target", "vscode"); r.code != 2 || r.stdout != "" {
		t.Errorf("--target vscode: exit %d, stdout %q, want exit 2 and no report", r.code, r.stdout)
	}

	phase7 := []capability.Capability{capability.Projection, capability.Dispatch, capability.Cancellation, capability.Persistence, capability.Restart, capability.Authentication, capability.MemoryEnforcement}
	claims := func(d capability.Declaration) map[capability.Capability]capability.Claim {
		m := map[capability.Capability]capability.Claim{}
		for _, c := range d.Claims {
			m[c.Capability] = c
		}
		return m
	}
	for _, d := range all.Declarations {
		byName := claims(d)
		switch d.Target {
		case capability.TargetClaude:
			// The reference runtime states every capability; a supported or partial
			// claim names its proof, and a partial one states its limit.
			for _, name := range append([]capability.Capability{capability.Installation}, phase7...) {
				c, ok := byName[name]
				if !ok {
					t.Errorf("claude has no %s claim", name)
					continue
				}
				switch c.Status {
				case capability.Supported:
					if len(c.Tests) == 0 {
						t.Errorf("claude %s is supported and names no test", name)
					}
				case capability.Partial:
					if len(c.Tests) == 0 || c.Detail == "" {
						t.Errorf("claude %s is partial with %d tests and detail %q, want named tests and a written limit", name, len(c.Tests), c.Detail)
					}
				default:
					t.Errorf("claude %s is %s, want supported or partial", name, c.Status)
				}
			}
			if d.Untested != "" {
				t.Errorf("claude is marked untested (%q)", d.Untested)
			}
		default:
			// Declared only: an honest unsupported with the limit written.
			for _, name := range phase7 {
				c, ok := byName[name]
				if !ok {
					t.Errorf("%s has no %s claim", d.Target, name)
					continue
				}
				if c.Status != capability.Unsupported || c.Detail == "" || len(c.Tests) != 0 {
					t.Errorf("%s %s = %s with %d tests and detail %q, want unsupported, no tests, and a written limit", d.Target, name, c.Status, len(c.Tests), c.Detail)
				}
			}
			if untested := d.Untested != ""; untested != (d.Target == capability.TargetOpenCode) {
				t.Errorf("%s untested = %q: only opencode is marked untested", d.Target, d.Untested)
			}
		}
	}

	// Phase 8 adds the skills capability, last in the closed set. Every runtime the
	// binary prints states it, and none claims more than partial: a written limit, and
	// named tests (their existence was checked above, with the rest of the claims).
	for _, d := range all.Declarations {
		last := d.Claims[len(d.Claims)-1]
		if last.Capability != capability.Skills {
			t.Errorf("%s: the last printed claim is %s, want skills", d.Target, last.Capability)
			continue
		}
		if last.Status != capability.Partial || len(last.Tests) == 0 || last.Detail == "" {
			t.Errorf("%s skills = %s with %d tests and detail %q, want partial, named tests, and a written limit", d.Target, last.Status, len(last.Tests), last.Detail)
		}
	}
}

// --- unbound sessions --------------------------------------------------------

func phase7Unbound(t *testing.T, binary string) {
	w := newPhase7World(t, binary)
	// A paused workflow exists, so a hook that guessed from the directory, or that
	// denied without a binding, would show it; but nothing is bound to it.
	w.createWorkflow("proj-1", "wf-1", "odd")
	w.verb("proj-1", "wf-1", "start")
	w.verb("proj-1", "wf-1", "pause")
	another := fixtureRepo(t, "another")
	before := w.stateSnapshot()

	events := map[string][]string{
		"UserPromptSubmit": {hookInput(t, w.repo, "s", "p"), hookInput(t, another, "s", "p"), hookInput(t, w.dir, "s", "p")},
		"PreToolUse": {
			gateInput(t, w.repo, "s", "Edit", phase7EditInputs["Edit"]),
			gateInput(t, w.repo, "s", "mcp__longterm-mem__query", queryArgs("other")),
			gateInput(t, another, "s", "Write", phase7EditInputs["Write"]),
			gateInput(t, w.dir, "s", "Edit", phase7EditInputs["Edit"]),
		},
	}
	// Whatever else arrives on stdin is not a usable hook input.
	malformed := []string{"", "not json", "{", "[]", "null", `"cwd"`, `{"cwd":5}`, `{"cwd":"relative/path"}`, "\xff\xfe\x00\x01", strings.Repeat("{", 4096)}
	for event, valid := range events {
		for i, input := range append(append([]string(nil), valid...), malformed...) {
			r := w.engine(w.dir, input, "projection", "hook", "--event", event)
			assertSilent(t, event+" input #"+strconv.Itoa(i), r.asHook())
		}
	}
	if after := w.stateSnapshot(); !reflect.DeepEqual(after, before) {
		t.Errorf("an unbound hook changed the state directory:\nbefore %v\nafter  %v", before, after)
	}
}

// --- bound and running -------------------------------------------------------

func phase7Running(t *testing.T, binary string) {
	w := newPhase7World(t, binary)
	w.bindRunning("proj-1", "wf-1", "odd", "authorize", "explore")
	odd, err := workflowprofile.Resolve("odd")
	if err != nil {
		t.Fatal(err)
	}
	before := w.stateSnapshot()

	r := w.promptHook(w.repo)
	// decodeHookOutput requires exit 0, no stderr, exactly one JSON object that
	// begins with '{', only hookSpecificOutput and systemMessage keys, and no
	// top-level additionalContext (where Claude Code ignores it).
	msg := decodeHookOutput(t, r.asHook())
	if !reflect.DeepEqual(msg.Keys, []string{"hookSpecificOutput"}) || msg.Warning != "" {
		t.Fatalf("output %s, want hookSpecificOutput alone: a running workflow needs no warning", r.stdout)
	}
	// The literal is the documented 16 KiB bound; the second operand follows the
	// constant. Both are kept so that raising projection.MaxContextBytes cannot
	// loosen this end-to-end check (the constant's own value is pinned by
	// projection.TestHookLimits).
	if len(msg.Context) == 0 || len(msg.Context) > 16<<10 || len(msg.Context) > projection.MaxContextBytes {
		t.Errorf("context is %d bytes, want between 1 and the 16 KiB bound", len(msg.Context))
	}
	for _, want := range []string{
		"workflow: wf-1 (project: proj-1)",
		"profile: odd",
		"status: running",
		"current stage: explore",
		"next stage: " + odd.Stages[2].Name,
		"memory plan (read-only: it executes no query and grants no memory write): scope=project sources=engram,longterm-mem,procedural-skills project_id=proj-1 goal_id=none write=none",
		"stages recorded (2): authorize, explore",
	} {
		if !hasContextLine(msg.Context, want) {
			t.Errorf("no line %q in the projected context:\n%s", want, msg.Context)
		}
	}
	if !strings.HasPrefix(contextLineWithPrefix(msg.Context, "goal:"), "goal: goal-1 (digest ") {
		t.Errorf("no goal line in the projected context:\n%s", msg.Context)
	}
	if !strings.HasPrefix(r.stdout, `{"hookSpecificOutput":{"hookEventName":"UserPromptSubmit","additionalContext":`) {
		t.Errorf("stdout begins %.80q, want the documented hookSpecificOutput object", r.stdout)
	}
	if after := w.stateSnapshot(); !reflect.DeepEqual(after, before) {
		t.Errorf("the prompt hook changed the state directory")
	}
}

// --- bound and paused, closed --------------------------------------------------

func phase7Paused(t *testing.T, binary string) {
	t.Run("paused denies the file-edit tools and nothing else", func(t *testing.T) {
		w := newPhase7World(t, binary)
		w.bindRunning("proj-1", "wf-1", "odd", "authorize")
		assertToolsSilent(t, w, "running", phase7OrderedEditTools, phase7EditInputs)

		w.verb("proj-1", "wf-1", "pause")
		before := w.stateSnapshot()
		for _, tool := range phase7OrderedEditTools {
			assertDenied(t, "paused "+tool, w.toolHook(w.repo, tool, phase7EditInputs[tool]).asHook(),
				"wf-1", "proj-1", "is paused", tool+" is denied", "labdrian workflow resume --project proj-1 --workflow wf-1", "labdrian workflow unbind")
		}
		assertToolsSilent(t, w, "paused", phase7OrderedOtherTools, phase7OtherInputs)
		assertSilent(t, "paused mem_save", w.toolHook(w.repo, "mcp__plugin_engram_engram__mem_save", map[string]any{"title": "t", "content": "c"}).asHook())
		if line := contextLineWithPrefix(decodeHookOutput(t, w.promptHook(w.repo).asHook()).Context, "PAUSED"); !strings.Contains(line, "labdrian workflow resume") {
			t.Errorf("the prompt hook does not tell the session the workflow is paused: %q", line)
		}
		if after := w.stateSnapshot(); !reflect.DeepEqual(after, before) {
			t.Errorf("the gate or the prompt hook changed the state directory while paused")
		}

		w.verb("proj-1", "wf-1", "resume")
		assertToolsSilent(t, w, "resumed", phase7OrderedEditTools, phase7EditInputs)
	})

	// closed: the workflow was paused, so an open one would deny; a closed one
	// never gates, and the next prompt unbinds the repository.
	for _, outcome := range []string{"abandoned", "completed"} {
		t.Run(outcome+" workflows unbind and never gate", func(t *testing.T) {
			w := newPhase7World(t, binary)
			profile := "standalone-minimal"
			w.createWorkflow("proj-1", "wf-1", profile)
			w.verb("proj-1", "wf-1", "start")
			resolved, err := workflowprofile.Resolve(profile)
			if err != nil {
				t.Fatal(err)
			}
			for _, stage := range resolved.Stages {
				w.verb("proj-1", "wf-1", "stage", "--stage", stage.Name)
			}
			w.engine(w.repo, "", "workflow", "bind", "--project", "proj-1", "--workflow", "wf-1").must(t, "bind")
			w.verb("proj-1", "wf-1", "pause")
			assertDenied(t, "control: paused Edit", w.toolHook(w.repo, "Edit", phase7EditInputs["Edit"]).asHook(), "is paused")

			if outcome == "abandoned" {
				w.verb("proj-1", "wf-1", "close", "--outcome", "abandoned", "--reason", "acceptance")
			} else {
				w.verb("proj-1", "wf-1", "resume")
				w.verb("proj-1", "wf-1", "verify", "--goal", w.goalPath("proj-1", "wf-1"))
				w.verb("proj-1", "wf-1", "close", "--outcome", "completed")
			}

			// The gate never gates a closed workflow, and it never unbinds: it is
			// read-only.
			assertToolsSilent(t, w, "closed", phase7OrderedEditTools, phase7EditInputs)
			assertToolsSilent(t, w, "closed", phase7OrderedOtherTools, phase7OtherInputs)
			if b := w.binding(w.repo); b.Classification != "owned" {
				t.Fatalf("the gate changed the binding: %+v", b)
			}

			// The next prompt says so once and unbinds.
			note := decodeHookOutput(t, w.promptHook(w.repo).asHook())
			if !strings.Contains(note.Context, "is closed ("+outcome+")") || !strings.Contains(note.Context, "was removed") || note.Warning != "" {
				t.Errorf("closed-workflow note %+v, want one note saying it is closed (%s) and the binding was removed", note, outcome)
			}
			if b := w.binding(w.repo); b.Classification != "absent" {
				t.Errorf("binding after the prompt = %+v, want it unbound", b)
			}
			assertSilent(t, "prompt after unbind", w.promptHook(w.repo).asHook())
			assertToolsSilent(t, w, "unbound", phase7OrderedEditTools, phase7EditInputs)
		})
	}
}

// --- memory gate ---------------------------------------------------------------

func phase7MemoryGate(t *testing.T, binary string) {
	// The two spellings of the tool name a session really sees: a bare server
	// and a plugin-provided one.
	queryTools := []string{"mcp__longterm-mem__query", "mcp__plugin_longterm-mem_longterm-mem__query"}
	// Tools the gate must never touch: they carry no project it could check (get
	// carries only an id), or they are not longterm-mem at all.
	untouched := map[string]any{
		"mcp__longterm-mem__get":                        map[string]any{"id": "obs-1"},
		"mcp__plugin_longterm-mem_longterm-mem__get":    map[string]any{"id": "obs-1"},
		"mcp__longterm-mem__promote":                    map[string]any{"id": "obs-1", "project": "other"},
		"mcp__plugin_engram_engram__mem_search":         map[string]any{"query": "q", "project": "other"},
		"mcp__plugin_engram_engram__mem_save":           map[string]any{"title": "t", "content": "c", "project": "other"},
		"mcp__engram__mem_context":                      map[string]any{"project": "other"},
		"mcp__plugin_longterm-mem_longterm-mem__query2": map[string]any{"query": "q", "project": "other"},
	}
	assertUntouched := func(t *testing.T, w phase7World) {
		t.Helper()
		for tool, input := range untouched {
			assertSilent(t, tool, w.toolHook(w.repo, tool, input).asHook())
		}
	}

	t.Run("a project scope permits only the workflow's project", func(t *testing.T) {
		w := newPhase7World(t, binary)
		w.bindRunning("proj-1", "wf-1", "odd", "authorize")
		before := w.stateSnapshot()
		for _, tool := range queryTools {
			assertDenied(t, tool+" other project", w.toolHook(w.repo, tool, queryArgs("other")).asHook(),
				`project "other"`, `permits only project "proj-1"`, "wf-1", "labdrian workflow unbind")
			assertDenied(t, tool+" no project", w.toolHook(w.repo, tool, queryArgs(nil)).asHook(), "names no project", `"proj-1"`)
			assertSilent(t, tool+" the workflow's project", w.toolHook(w.repo, tool, queryArgs("proj-1")).asHook())
		}
		assertUntouched(t, w)
		if after := w.stateSnapshot(); !reflect.DeepEqual(after, before) {
			t.Errorf("the memory gate changed the state directory")
		}
	})

	t.Run("scope none denies every query", func(t *testing.T) {
		w := newPhase7World(t, binary)
		w.bindRunning("proj-1", "wf-1", "standalone-minimal", "authorize")
		for _, tool := range queryTools {
			for label, input := range map[string]any{"the workflow's project": queryArgs("proj-1"), "another project": queryArgs("other"), "no project": queryArgs(nil)} {
				assertDenied(t, tool+" "+label, w.toolHook(w.repo, tool, input).asHook(), "scope none", "every longterm-mem query is denied", "labdrian workflow unbind")
			}
		}
		assertUntouched(t, w)
	})
}

// --- restart -------------------------------------------------------------------

func phase7Restart(t *testing.T, binary string) {
	w := newPhase7World(t, binary)
	// Every step below is its own process; only files carry state between them.
	w.bindRunning("proj-1", "wf-1", "odd", "authorize", "explore")
	first := w.promptHook(w.repo)
	decodeHookOutput(t, first.asHook())

	worktree := fixtureLinkedWorktree(t, w.repo, "wt")
	deeper := filepath.Join(worktree, "pkg", "deeper")
	if err := os.MkdirAll(deeper, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, cwd := range map[string]string{"the main checkout": w.repo, "a linked worktree": worktree, "a subdirectory of it": deeper} {
		for _, session := range []string{"session-b", "session-c"} {
			r := w.engine(w.dir, hookInput(t, cwd, session, "a later prompt in "+session), "projection", "hook", "--event", "UserPromptSubmit")
			if r.stdout != first.stdout || r.code != 0 || r.stderr != "" {
				t.Errorf("a new process in %s (%s): exit %d, stderr %q, stdout\n%s\nwant the bytes of the first process\n%s", name, session, r.code, r.stderr, r.stdout, first.stdout)
			}
		}
	}
	if b := w.binding(worktree); b.Classification != "owned" || b.Binding == nil || b.Binding.WorkflowID != "wf-1" {
		t.Errorf("binding seen from the worktree = %+v, want the one made in the main checkout", b)
	}

	// The projection follows the log on disk: a change made by another process
	// reaches the next fresh process in the worktree, and undoing it restores the
	// first bytes.
	w.verb("proj-1", "wf-1", "pause")
	paused := w.engine(w.dir, hookInput(t, worktree, "session-d", "p"), "projection", "hook", "--event", "UserPromptSubmit")
	if !hasContextLine(decodeHookOutput(t, paused.asHook()).Context, "status: paused") {
		t.Errorf("a new process in the worktree does not see the workflow paused:\n%s", paused.stdout)
	}
	assertDenied(t, "worktree Edit while paused", w.toolHook(worktree, "Edit", phase7EditInputs["Edit"]).asHook(), "is paused")
	w.verb("proj-1", "wf-1", "resume")
	if again := w.engine(w.dir, hookInput(t, worktree, "session-e", "p"), "projection", "hook", "--event", "UserPromptSubmit"); again.stdout != first.stdout {
		t.Errorf("after resume, a new process differs from the first:\n%s\nvs\n%s", again.stdout, first.stdout)
	}
}

// --- unusable state -------------------------------------------------------------

func phase7UnusableState(t *testing.T, binary string) {
	logPath := func(w phase7World) string { return phase6WorkflowLogPath(w.state, "proj-1", "wf-1") }
	bindingPath := func(t *testing.T, w phase7World) string {
		return filepath.Join(w.state, "labdrian", "bindings", mustRepoKey(t, w.repo)+".json")
	}
	cases := []struct {
		name   string
		break_ func(t *testing.T, w phase7World)
	}{
		{"the workflow log drifted", func(t *testing.T, w phase7World) { driftWorkflowLog(t, logPath(w)) }},
		{"the workflow log is foreign", func(t *testing.T, w phase7World) {
			writeFixtureFile(t, logPath(w), `{"hello":"world"}`+"\n")
		}},
		{"the workflow log is malformed", func(t *testing.T, w phase7World) { writeFixtureFile(t, logPath(w), "not json\n") }},
		{"the workflow log is gone", func(t *testing.T, w phase7World) {
			if err := os.Remove(logPath(w)); err != nil {
				t.Fatal(err)
			}
		}},
		{"the workflow log cannot be read", func(t *testing.T, w phase7World) {
			path := logPath(w)
			if err := os.Chmod(path, 0); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
			if f, err := os.Open(path); err == nil {
				f.Close()
				t.Skip("chmod 0 is not enforced for this process (root or an equivalent capability); the unreadable-log fault cannot be injected")
			}
		}},
		{"the binding file is foreign", func(t *testing.T, w phase7World) {
			writeFixtureFile(t, bindingPath(t, w), `{"hello":"world"}`+"\n")
		}},
		{"the binding file is malformed", func(t *testing.T, w phase7World) {
			writeFixtureFile(t, bindingPath(t, w), "hand-written notes\n")
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := newPhase7World(t, binary)
			w.bindRunning("proj-1", "wf-1", "odd", "authorize")
			w.verb("proj-1", "wf-1", "pause")
			// While the state is intact the gate is live, so the silence below is
			// the state's doing.
			assertDenied(t, "control: Edit", w.toolHook(w.repo, "Edit", phase7EditInputs["Edit"]).asHook(), "is paused")
			assertDenied(t, "control: query", w.toolHook(w.repo, "mcp__longterm-mem__query", queryArgs("other")).asHook(), `permits only project "proj-1"`)

			c.break_(t, w)
			before, tree := w.stateSnapshot(), snapshotTree(t, w.state)

			// Nothing projected, one visible warning that says so and says what to do.
			r := w.promptHook(w.repo)
			msg := decodeHookOutput(t, r.asHook())
			if msg.Context != "" || !reflect.DeepEqual(msg.Keys, []string{"systemMessage"}) {
				t.Fatalf("output %s, want a systemMessage alone: nothing projected", r.stdout)
			}
			if strings.Contains(msg.Warning, "\n") || !strings.Contains(msg.Warning, "projected") || !strings.Contains(msg.Warning, "labdrian workflow") {
				t.Errorf("warning %q, want one line saying nothing is projected and naming a labdrian workflow command", msg.Warning)
			}

			// No tool denied.
			for _, tool := range phase7OrderedEditTools {
				assertSilent(t, tool, w.toolHook(w.repo, tool, phase7EditInputs[tool]).asHook())
			}
			for _, project := range []any{"other", nil} {
				assertSilent(t, "query", w.toolHook(w.repo, "mcp__plugin_longterm-mem_longterm-mem__query", queryArgs(project)).asHook())
			}

			// Read-only: the hooks neither repaired nor removed anything.
			if after := w.stateSnapshot(); !reflect.DeepEqual(after, before) {
				t.Errorf("the hooks changed the state directory")
			}
			if after := snapshotTree(t, w.state); !reflect.DeepEqual(after, tree) {
				t.Errorf("the hooks changed a mode, size, or time in the state directory")
			}
		})
	}

	// The evidence limit is on record: the prompt hook's claim says that when the
	// binding or the workflow cannot be followed it projects nothing and warns,
	// and the gate's claim says that it then denies nothing.
	t.Run("the declarations record the limit", func(t *testing.T) {
		w := newPhase7World(t, binary)
		report := phase7DecodeReport(t, w.engine(w.dir, "", "runtime", "capabilities", "--target", capability.TargetClaude).must(t, "runtime capabilities").stdout)
		byName := map[capability.Capability]string{}
		for _, c := range report.Declarations[0].Claims {
			byName[c.Capability] = c.Detail
		}
		for _, name := range []capability.Capability{capability.Projection, capability.Cancellation} {
			if !strings.Contains(byName[name], "cannot be followed") {
				t.Errorf("claude %s detail %q does not state what happens when the binding or the workflow cannot be followed", name, byName[name])
			}
		}
	})
}

// --- presence prober -------------------------------------------------------------

func phase7PresenceProber(t *testing.T, binary string) {
	w := newPhase7World(t, binary)
	const canary = "PHASE7-CREDENTIAL-CANARY"
	writeFile := func(mode os.FileMode, rel ...string) {
		t.Helper()
		path := filepath.Join(append([]string{w.home}, rel...)...)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(canary), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	}
	// Claude Code's credentials file cannot be read by its owner, so a prober that
	// opened it would fail; a stat still sees it. Pi's and Engram's are ordinary.
	writeFile(0, ".claude", ".credentials.json")
	writeFile(0o600, ".pi", "agent", "auth.json")
	writeFile(0o600, ".engram", "engram.db")
	// gentle-ai is on PATH, and would leave a marker if anything ran it.
	marker := filepath.Join(t.TempDir(), "gentle-ai-was-run")
	if err := os.WriteFile(filepath.Join(w.bin, "gentle-ai"), []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	r := w.engine(w.dir, "", "runtime", "probe").must(t, "runtime probe")
	if r.stderr != "" {
		t.Errorf("stderr = %q, want none", r.stderr)
	}
	var report probeReportJSON
	dec := json.NewDecoder(strings.NewReader(r.stdout))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&report); err != nil {
		t.Fatalf("stdout is not a probe report: %v\n%s", err, r.stdout)
	}

	type expectation struct{ status, detail string }
	want := map[string]expectation{
		"credentials:claude-code":  {"available", "the Claude Code credentials file is present (not opened; this does not prove the runtime is authenticated)"},
		"credentials:codex":        {"unavailable", "was not found"},
		"credentials:pi":           {"available", "the Pi credentials file is present (not opened; this does not prove the runtime is authenticated)"},
		"memory:engram":            {"available", "the Engram database file is present (not opened, so its contents and health are unverified)"},
		"memory:longterm-mem":      {"unavailable", "was not found"},
		"memory:procedural-skills": {"unavailable", "no presence check exists"},
		"gentle-ai-review":         {"available", "not executed; review mode and consent are unverified"},
	}
	if len(report.Observations) != len(want) {
		t.Fatalf("observations = %+v, want %d", report.Observations, len(want))
	}
	for _, o := range report.Observations {
		exp, ok := want[o.Capability]
		if !ok {
			t.Errorf("unexpected observation %+v", o)
			continue
		}
		// The status is one of two words; never "authenticated" or "healthy".
		if string(o.Status) != exp.status || !strings.Contains(o.Detail, exp.detail) {
			t.Errorf("%s = %s %q, want %s with %q", o.Capability, o.Status, o.Detail, exp.status, exp.detail)
		}
	}
	// "authenticated" appears only inside the disclaimer that presence does not
	// prove it, never as a claim; and no path or content is printed.
	claimed := strings.ReplaceAll(strings.ToLower(r.stdout), "does not prove the runtime is authenticated", "")
	if strings.Contains(claimed, "authenticated") {
		t.Errorf("the probe report says something is authenticated outside a disclaimer:\n%s", r.stdout)
	}
	for _, leak := range []string{canary, w.home, w.bin} {
		if strings.Contains(r.stdout, leak) {
			t.Errorf("the probe report leaks %q", leak)
		}
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("gentle-ai was executed by the probe")
	}

	// The workflow verbs use the same prober: the observations a workflow records
	// are the probe's, and the projected context lists as unavailable only what
	// the probe did not find.
	w.bindRunning("proj-1", "wf-1", "odd", "authorize")
	events := phase6LoadOwned(t, w.state, "proj-1", "wf-1").Events
	seen := map[string]string{}
	for _, o := range events[len(events)-1].Observations {
		seen[o.Capability] = o.Status
	}
	if seen["memory:engram"] != "available" || seen["gentle-ai-review"] != "available" || seen["memory:longterm-mem"] != "unavailable" {
		t.Errorf("observations the last workflow event recorded = %v", seen)
	}
	line := contextLineWithPrefix(decodeHookOutput(t, w.promptHook(w.repo).asHook()).Context, "unavailable dependencies at the last recorded event:")
	if strings.Contains(line, "memory:engram") || strings.Contains(line, "gentle-ai-review") || !strings.Contains(line, "memory:longterm-mem") {
		t.Errorf("unavailable-dependencies line %q, want only what the probe did not find", line)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("gentle-ai was executed by a workflow verb")
	}
}

// --- settings family -------------------------------------------------------------

func phase7ReadSettings(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("%s is not JSON: %v", path, err)
	}
	return doc
}

// phase7Canonical renders a parsed JSON value in one canonical form, so two
// values compare as the bytes they would be written as.
func phase7Canonical(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// phase7InstalledHook is one entry of the projection family read back from
// settings.
type phase7InstalledHook struct{ event, matcher, command string }

// phase7ProjectionEntries lists the entries of doc that run the projection hook
// of the binary at hookCommand, in file order.
func phase7ProjectionEntries(t *testing.T, doc map[string]any, hookCommand string) []phase7InstalledHook {
	t.Helper()
	var out []phase7InstalledHook
	hooks, _ := doc["hooks"].(map[string]any)
	for _, event := range []string{"UserPromptSubmit", "PreToolUse"} {
		entries, _ := hooks[event].([]any)
		for _, e := range entries {
			entry, _ := e.(map[string]any)
			inner, _ := entry["hooks"].([]any)
			for _, h := range inner {
				command, _ := h.(map[string]any)["command"].(string)
				if strings.Contains(command, hookCommand+" projection hook --event ") {
					matcher, _ := entry["matcher"].(string)
					out = append(out, phase7InstalledHook{event: event, matcher: matcher, command: command})
				}
			}
		}
	}
	return out
}

func phase7Settings(t *testing.T, binary string) {
	w := newPhase7World(t, binary)
	claudeDir := filepath.Join(w.home, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.json")
	hookCommand := filepath.Join(claudeDir, "bin", "gentle-ai-overlay")
	if err := os.MkdirAll(filepath.Dir(hookCommand), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(binary, hookCommand); err != nil {
		t.Fatal(err)
	}

	// Foreign entries share our event keys and even our matchers; one runs another
	// program with a similar verb, and one shares our binary path.
	entry := func(matcher, command string, extra map[string]any) map[string]any {
		hook := map[string]any{"type": "command", "command": command}
		for k, v := range extra {
			hook[k] = v
		}
		e := map[string]any{"hooks": []any{hook}}
		if matcher != "" {
			e["matcher"] = matcher
		}
		return e
	}
	foreign := map[string]any{
		"model":       "opus",
		"permissions": map[string]any{"allow": []any{"Bash(ls:*)"}},
		"hooks": map[string]any{
			"Notification": []any{entry("", "/opt/other/notify --loud && true", nil)},
			"UserPromptSubmit": []any{
				entry("", "/opt/other/ups --flag", nil),
				entry("", "/opt/other/tool projection hook --event UserPromptSubmit", nil),
			},
			"PreToolUse": []any{
				entry("Write|Edit|MultiEdit|NotebookEdit", "/opt/other/edit-guard", map[string]any{"timeout": 5}),
				entry(`^mcp__([A-Za-z0-9-]+_)*longterm-mem__query$`, "/opt/other/mem-guard", nil),
				entry("Bash", hookCommand+" some other verb", nil),
			},
		},
	}
	original, err := json.MarshalIndent(foreign, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	merge := func() {
		w.engine(w.dir, "", "merge-settings", "--settings", settingsPath, "--hook-command", hookCommand).must(t, "merge-settings")
	}
	statusLine := func() string {
		t.Helper()
		out := w.engine(w.dir, "", "status").stdout
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, "hooks: projection (") {
				return line
			}
		}
		t.Fatalf("status prints no projection line:\n%s", out)
		return ""
	}
	assertForeignKept := func(stage string) {
		t.Helper()
		doc := phase7ReadSettings(t, settingsPath)
		hooks, _ := doc["hooks"].(map[string]any)
		for event, entries := range foreign["hooks"].(map[string]any) {
			have := map[string]bool{}
			list, _ := hooks[event].([]any)
			for _, e := range list {
				have[phase7Canonical(t, e)] = true
			}
			for _, e := range entries.([]any) {
				if !have[phase7Canonical(t, e)] {
					t.Errorf("%s: foreign %s entry lost or changed: %s", stage, event, phase7Canonical(t, e))
				}
			}
		}
		permissions, _ := doc["permissions"].(map[string]any)
		if doc["model"] != "opus" || phase7Canonical(t, permissions["allow"]) != `["Bash(ls:*)"]` {
			t.Errorf("%s: foreign top-level keys changed: %s", stage, phase7Canonical(t, doc))
		}
	}

	// Before: the family is missing, and status says so, with the way out.
	if line := statusLine(); !strings.HasPrefix(line, "[WARN]") || !strings.Contains(line, "missing or drifted") || !strings.Contains(line, "install-hooks") {
		t.Errorf("status before the merge: %q", line)
	}

	merge()
	assertForeignKept("after the merge")
	installed := phase7ProjectionEntries(t, phase7ReadSettings(t, settingsPath), hookCommand)
	wantEntries := []phase7InstalledHook{
		{"UserPromptSubmit", "", ""},
		{"PreToolUse", "Write|Edit|MultiEdit|NotebookEdit", ""},
		{"PreToolUse", `^mcp__([A-Za-z0-9-]+_)*longterm-mem__query$`, ""},
	}
	if len(installed) != len(wantEntries) {
		t.Fatalf("projection entries = %+v, want %d", installed, len(wantEntries))
	}
	for i, want := range wantEntries {
		got := installed[i]
		if got.event != want.event || got.matcher != want.matcher || !strings.HasPrefix(got.command, "command -v "+hookCommand+" >/dev/null 2>&1 && "+hookCommand+" projection hook --event "+want.event) || !strings.HasSuffix(got.command, " || true") {
			t.Errorf("entry %d = %+v, want the guarded never-blocking %s command with matcher %q", i, got, want.event, want.matcher)
		}
	}
	if line := statusLine(); !strings.HasPrefix(line, "[OK  ]") || !strings.Contains(line, "installed") {
		t.Errorf("status after the merge: %q", line)
	}

	// The installed commands work as Claude Code would run them: through sh, with
	// the hook JSON on stdin.
	if _, err := exec.LookPath("sh"); err == nil {
		w.bindRunning("proj-1", "wf-1", "odd", "authorize")
		ctx := decodeHookOutput(t, w.run(w.dir, hookInput(t, w.repo, "s", "p"), "sh", "-c", installed[0].command).asHook()).Context
		if !hasContextLine(ctx, "workflow: wf-1 (project: proj-1)") {
			t.Errorf("the installed UserPromptSubmit command did not project the workflow:\n%s", ctx)
		}
		w.verb("proj-1", "wf-1", "pause")
		assertDenied(t, "installed edit gate", w.run(w.dir, gateInput(t, w.repo, "s", "Edit", phase7EditInputs["Edit"]), "sh", "-c", installed[1].command).asHook(), "is paused")
		assertSilent(t, "installed edit gate, Bash", w.run(w.dir, gateInput(t, w.repo, "s", "Bash", phase7OtherInputs["Bash"]), "sh", "-c", installed[1].command).asHook())
		assertDenied(t, "installed memory gate", w.run(w.dir, gateInput(t, w.repo, "s", "mcp__plugin_longterm-mem_longterm-mem__query", queryArgs("other")), "sh", "-c", installed[2].command).asHook(), `permits only project "proj-1"`)
		// A missing binary is a no-op, not a failure.
		gone := strings.Replace(installed[0].command, hookCommand, filepath.Join(claudeDir, "bin", "missing"), -1)
		assertSilent(t, "installed command, binary missing", w.run(w.dir, hookInput(t, w.repo, "s", "p"), "sh", "-c", gone).asHook())
	}
	// The matchers the settings hold cover the tool names the gate checks.
	memoryMatcher := regexp.MustCompile(installed[2].matcher)
	for tool, want := range map[string]bool{
		"mcp__longterm-mem__query": true, "mcp__plugin_longterm-mem_longterm-mem__query": true,
		"mcp__longterm-mem__get": false, "mcp__longterm-mem__promote": false, "mcp__plugin_engram_engram__mem_search": false,
	} {
		if memoryMatcher.MatchString(tool) != want {
			t.Errorf("installed memory matcher matches %s = %v, want %v", tool, !want, want)
		}
	}
	if got := strings.Split(installed[1].matcher, "|"); !reflect.DeepEqual(got, phase7OrderedEditTools) {
		t.Errorf("installed edit matcher lists %v, want the gated tools %v", got, phase7OrderedEditTools)
	}

	// Idempotent: a second merge changes nothing at all.
	first, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	merge()
	if second, _ := os.ReadFile(settingsPath); !bytes.Equal(first, second) {
		t.Errorf("a second merge changed the settings file:\n%s\nvs\n%s", second, first)
	}

	// Drift: an edited entry is reported, and the next merge repairs it to the
	// same bytes, without touching a foreign entry.
	doc := phase7ReadSettings(t, settingsPath)
	for _, e := range doc["hooks"].(map[string]any)["UserPromptSubmit"].([]any) {
		inner := e.(map[string]any)["hooks"].([]any)[0].(map[string]any)
		if strings.Contains(inner["command"].(string), " projection hook --event ") && strings.Contains(inner["command"].(string), hookCommand) {
			inner["command"] = inner["command"].(string) + " --extra"
		}
	}
	drifted, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, drifted, 0o600); err != nil {
		t.Fatal(err)
	}
	if line := statusLine(); !strings.HasPrefix(line, "[WARN]") || !strings.Contains(line, "drifted") {
		t.Errorf("status with a drifted entry: %q", line)
	}
	merge()
	if repaired, _ := os.ReadFile(settingsPath); !bytes.Equal(repaired, first) {
		t.Errorf("the merge did not repair the drift to the installed bytes:\n%s\nvs\n%s", repaired, first)
	}
	assertForeignKept("after the repair")

	// Uninstall removes ours and only ours: the document is the foreign one again,
	// and a second uninstall changes nothing.
	w.engine(w.dir, "", "uninstall-hooks", "--settings", settingsPath, "--hook-command", hookCommand).must(t, "uninstall-hooks")
	after := phase7ReadSettings(t, settingsPath)
	if left := phase7ProjectionEntries(t, after, hookCommand); len(left) != 0 {
		t.Errorf("projection entries survive uninstall: %+v", left)
	}
	if got, want := phase7Canonical(t, after), phase7Canonical(t, foreign); got != want {
		t.Errorf("after uninstall the settings are\n%s\nwant the foreign document\n%s", got, want)
	}
	uninstalled, _ := os.ReadFile(settingsPath)
	w.engine(w.dir, "", "uninstall-hooks", "--settings", settingsPath, "--hook-command", hookCommand).must(t, "second uninstall-hooks")
	if again, _ := os.ReadFile(settingsPath); !bytes.Equal(again, uninstalled) {
		t.Errorf("a second uninstall changed the settings file")
	}
	if line := statusLine(); !strings.HasPrefix(line, "[WARN]") {
		t.Errorf("status after uninstall: %q, want the family reported missing", line)
	}
}
