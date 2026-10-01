package main

// Tests for 'projection hook --event UserPromptSubmit'. Workflows are created
// and advanced through the real 'workflow' verbs, repositories are hand-made
// fixtures (never the git binary), and every test runs in its own state home
// (bindEnv sets XDG_STATE_HOME), so nothing here can reach real state.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"
)

// --- harness ----------------------------------------------------------------

type hookRun struct {
	codes  []int
	stdout string
	stderr string
}

var hookArgs = []string{"hook", "--event", "UserPromptSubmit"}

// runProjectionArgs runs the projection verb in process with stdin as the hook
// input and processCwd as the working directory of the process. Every exit call
// is recorded, because the core is handed a non-terminating exit.
func runProjectionArgs(args []string, stdin, processCwd string) hookRun {
	var out, errBuf bytes.Buffer
	var codes []int
	runProjectionCore(args, processCwd, strings.NewReader(stdin), &out, &errBuf, func(c int) { codes = append(codes, c) })
	return hookRun{codes: codes, stdout: out.String(), stderr: errBuf.String()}
}

func runHook(stdin, processCwd string) hookRun { return runProjectionArgs(hookArgs, stdin, processCwd) }

// hookInput is the JSON Claude Code sends a UserPromptSubmit hook.
func hookInput(t *testing.T, cwd, session, prompt string) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"session_id":      session,
		"transcript_path": "/tmp/transcript.jsonl",
		"cwd":             cwd,
		"permission_mode": "default",
		"hook_event_name": "UserPromptSubmit",
		"prompt":          prompt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// assertSilent requires the hook to have done nothing visible: no stdout, no
// stderr, exit 0.
func assertSilent(t *testing.T, label string, r hookRun) {
	t.Helper()
	if r.stdout != "" || r.stderr != "" || !reflect.DeepEqual(r.codes, []int{0}) {
		t.Errorf("%s: exits %v, stdout %q, stderr %q, want a silent exit 0", label, r.codes, r.stdout, r.stderr)
	}
}

// hookMessage is the parsed output of a hook that spoke.
type hookMessage struct {
	Context string // hookSpecificOutput.additionalContext
	Warning string // systemMessage
	Keys    []string
}

// decodeHookOutput requires stdout to be exactly one JSON object of the shape
// Claude Code reads, and returns its parts. Exit 0 and an empty stderr are part
// of that contract.
func decodeHookOutput(t *testing.T, r hookRun) hookMessage {
	t.Helper()
	if !reflect.DeepEqual(r.codes, []int{0}) || r.stderr != "" {
		t.Fatalf("exits %v, stderr %q, want exactly one exit 0 and no stderr", r.codes, r.stderr)
	}
	if !strings.HasPrefix(r.stdout, "{") || !strings.HasSuffix(r.stdout, "}\n") || strings.Count(r.stdout, "\n") != 1 || !json.Valid([]byte(r.stdout)) {
		t.Fatalf("stdout %q is not exactly one JSON object on one line", r.stdout)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(r.stdout), &top); err != nil {
		t.Fatal(err)
	}
	var msg hookMessage
	for k := range top {
		msg.Keys = append(msg.Keys, k)
	}
	if _, misplaced := top["additionalContext"]; misplaced {
		t.Fatalf("additionalContext is at the top level, where Claude Code ignores it: %s", r.stdout)
	}
	for k := range top {
		if k != "hookSpecificOutput" && k != "systemMessage" {
			t.Fatalf("stdout carries the unexpected key %q: %s", k, r.stdout)
		}
	}
	if raw, ok := top["hookSpecificOutput"]; ok {
		var specific map[string]string
		if err := json.Unmarshal(raw, &specific); err != nil {
			t.Fatalf("hookSpecificOutput = %s: %v", raw, err)
		}
		if specific["hookEventName"] != "UserPromptSubmit" || len(specific) != 2 {
			t.Fatalf("hookSpecificOutput = %v, want exactly hookEventName UserPromptSubmit and additionalContext", specific)
		}
		msg.Context = specific["additionalContext"]
	}
	if raw, ok := top["systemMessage"]; ok {
		if err := json.Unmarshal(raw, &msg.Warning); err != nil {
			t.Fatalf("systemMessage = %s: %v", raw, err)
		}
	}
	return msg
}

// hookEnv is a bindEnv that can also create workflows of any profile.
type hookEnv struct{ bindEnv }

func newHookEnv(t *testing.T) hookEnv { return hookEnv{newBindEnv(t)} }

func (e hookEnv) goalPath(project, wf string) string {
	return filepath.Join(e.dir, project+"-"+wf+"-goal.json")
}

// create creates the workflow through the CLI (status created); its goal is
// goal-1 of the project.
func (e hookEnv) create(t *testing.T, project, wf, profile string) {
	t.Helper()
	goal := writeMemoryTestFile(t, e.dir, project+"-"+wf+"-goal.json", memoryTestGoalJSON(project, "goal-1"))
	phase6MustExitZero(t, "create", []string{"create", "--project", project, "--workflow", wf, "--goal", goal, "--profile", profile}, e.dir)
}

// step runs one workflow verb (with its own flags) against the workflow.
func (e hookEnv) step(t *testing.T, project, wf string, verbAndFlags ...string) {
	t.Helper()
	phase6MustExitZero(t, verbAndFlags[0], append(append([]string(nil), verbAndFlags...), "--project", project, "--workflow", wf), e.dir)
}

// running creates a workflow, starts it, records the given stages, and binds the
// repository to it.
func (e hookEnv) running(t *testing.T, project, wf, profile string, stages ...string) {
	t.Helper()
	e.create(t, project, wf, profile)
	e.step(t, project, wf, "start")
	for _, stage := range stages {
		e.step(t, project, wf, "stage", "--stage", stage)
	}
	mustBindOK(t, e.repo, project, wf)
}

// hook runs the hook for the repository at cwd, as Claude Code would.
func (e hookEnv) hook(t *testing.T, cwd string) hookRun {
	t.Helper()
	return runHook(hookInput(t, cwd, "session-1", "a prompt"), e.dir)
}

// snapshotContents lists every path under root with the SHA-256 of its content
// (or "dir"), so a test can prove nothing was written, not even bytes that
// leave the size and the time alone.
func snapshotContents(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if info.IsDir() {
			out[rel] = "dir"
			return nil
		}
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrPermission) {
			// A file a test made unreadable on purpose: its content cannot be
			// compared, and snapshotTree still watches its mode, size, and time.
			out[rel] = "unreadable"
			return nil
		}
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		out[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

// --- silence ----------------------------------------------------------------

func TestProjectionHookIsSilentWithoutABinding(t *testing.T) {
	e := newHookEnv(t)
	e.create(t, "proj-1", "wf-1", "standalone-minimal") // a workflow exists, but nothing is bound to it
	e.step(t, "proj-1", "wf-1", "start")

	assertSilent(t, "cwd from the input", e.hook(t, e.repo))
	assertSilent(t, "cwd from the process", runHook(`{"hook_event_name":"UserPromptSubmit"}`, e.repo))
	if _, err := os.Lstat(filepath.Join(e.state, "labdrian", "bindings")); err == nil {
		t.Error("an unbound repository made the hook create the bindings directory")
	}
}

func TestProjectionHookIsSilentOutsideARepository(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "standalone-minimal")
	elsewhere := t.TempDir() // no .git above it

	assertSilent(t, "input cwd outside a repository", e.hook(t, elsewhere))
	assertSilent(t, "process cwd outside a repository", runHook(`{"hook_event_name":"UserPromptSubmit"}`, elsewhere))
	// The input's directory wins over the process's: a bound repository as the
	// process directory does not rescue an input that points elsewhere.
	assertSilent(t, "input cwd elsewhere, process cwd in the bound repository", runHook(hookInput(t, elsewhere, "s", "p"), e.repo))
}

// TestProjectionHookIsSilentForInputItCannotUse: while the repository is bound
// (so a good input would speak), input that is not a usable hook input passes
// through without a word.
func TestProjectionHookIsSilentForInputItCannotUse(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "standalone-minimal")
	good := hookInput(t, e.repo, "s", "p")
	if decodeHookOutput(t, runHook(good, e.dir)).Context == "" {
		t.Fatal("test bug: the good input does not produce a context")
	}

	oversized := `{"cwd":"` + e.repo + `","prompt":"` + strings.Repeat("x", projection.MaxHookInputBytes) + `"}`
	for name, stdin := range map[string]string{
		"empty stdin":                 "",
		"whitespace":                  " \n ",
		"not JSON":                    "hello",
		"an array":                    "[" + good + "]",
		"null":                        "null",
		"a truncated object":          good[:len(good)-10],
		"two objects":                 good + good,
		"a cwd of the wrong type":     `{"cwd":42}`,
		"input over the size cap":     oversized,
		"another event's input":       strings.Replace(good, "UserPromptSubmit", "PreToolUse", 1),
		"an event name of wrong type": `{"hook_event_name":7,"cwd":"` + e.repo + `"}`,
	} {
		assertSilent(t, name, runHook(stdin, e.dir))
	}
}

// endlessReader yields the byte 'x' for ever and counts what it was asked for.
type endlessReader struct{ read int }

func (r *endlessReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	r.read += len(p)
	return len(p), nil
}

// TestProjectionHookReadsNoMoreThanTheCapPlusOneByte: input that never ends must
// not make the hook read for ever, or hold it all in memory. One byte past the
// cap is enough to know the input is too large.
func TestProjectionHookReadsNoMoreThanTheCapPlusOneByte(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "standalone-minimal")
	stdin := &endlessReader{}
	var out, errBuf bytes.Buffer
	var codes []int
	runProjectionCore(hookArgs, e.repo, stdin, &out, &errBuf, func(c int) { codes = append(codes, c) })
	if out.Len() != 0 || errBuf.Len() != 0 || !reflect.DeepEqual(codes, []int{0}) {
		t.Errorf("exits %v, stdout %q, stderr %q, want a silent exit 0", codes, out.String(), errBuf.String())
	}
	// io.LimitReader may ask the source for less than it is given room for, but
	// never for more than the cap and the one byte that proves it was passed.
	if stdin.read > projection.MaxHookInputBytes+1 {
		t.Errorf("the hook read %d bytes of an endless input, want at most %d", stdin.read, projection.MaxHookInputBytes+1)
	}
}

func TestProjectionHookIgnoresFieldsItDoesNotKnow(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "standalone-minimal")
	minimal := decodeHookOutput(t, runHook(`{"cwd":"`+e.repo+`"}`, e.dir))
	if minimal.Context == "" {
		t.Fatal("test bug: the minimal input does not produce a context")
	}
	rich := `{"cwd":"` + e.repo + `","hook_event_name":"UserPromptSubmit","session_id":"s","prompt":"p","a_future_field":{"nested":[1,{"deep":null}]},"another":true}`
	if got := decodeHookOutput(t, runHook(rich, e.dir)); got.Context != minimal.Context {
		t.Errorf("unknown fields changed the context:\n%s\nvs\n%s", got.Context, minimal.Context)
	}
}

// TestProjectionHookTreatsARelativeWorkingDirectoryAsMissing: a relative cwd is
// not a place, so the hook falls back to the process's directory, as it does
// when the input names none.
func TestProjectionHookTreatsARelativeWorkingDirectoryAsMissing(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "standalone-minimal")
	want := decodeHookOutput(t, e.hook(t, e.repo)).Context

	for _, cwd := range []string{"relative/dir", ".", "..", ""} {
		got := decodeHookOutput(t, runHook(`{"hook_event_name":"UserPromptSubmit","cwd":"`+cwd+`"}`, e.repo))
		if got.Context != want {
			t.Errorf("cwd %q: the context differs from the one for the absolute directory", cwd)
		}
		// Not a repository at the fallback: silent, however the relative path reads.
		assertSilent(t, "relative cwd "+cwd+" with the process outside a repository", runHook(`{"cwd":"`+cwd+`"}`, t.TempDir()))
	}
}

// --- what is projected ------------------------------------------------------

func TestProjectionHookOutputShape(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "standalone-minimal", "authorize")
	r := e.hook(t, e.repo)

	msg := decodeHookOutput(t, r)
	if msg.Context == "" || msg.Warning != "" {
		t.Fatalf("output %s, want a context and no warning", r.stdout)
	}
	if !reflect.DeepEqual(msg.Keys, []string{"hookSpecificOutput"}) {
		t.Errorf("top-level keys = %v, want only hookSpecificOutput", msg.Keys)
	}
	if len(msg.Context) > projection.MaxContextBytes {
		t.Errorf("context is %d bytes, over the bound", len(msg.Context))
	}
	if strings.Contains(r.stdout, `\u003c`) {
		t.Errorf("stdout escapes HTML characters: %s", r.stdout)
	}
}

func TestProjectionHookProjectsEachOpenStatus(t *testing.T) {
	goal := "goal: goal-1 (digest "
	t.Run("created", func(t *testing.T) {
		e := newHookEnv(t)
		e.create(t, "proj-1", "wf-1", "standalone-minimal")
		mustBindOK(t, e.repo, "proj-1", "wf-1")
		ctx := decodeHookOutput(t, e.hook(t, e.repo)).Context
		for _, want := range []string{"workflow: wf-1 (project: proj-1)", "profile: standalone-minimal", "current stage: none yet", "next stage: authorize", "stages recorded (0): none"} {
			if !hasContextLine(ctx, want) {
				t.Errorf("no line %q in:\n%s", want, ctx)
			}
		}
		if line := contextLineWithPrefix(ctx, "status:"); !strings.Contains(line, "created") || !strings.Contains(line, "labdrian workflow start --project proj-1 --workflow wf-1") {
			t.Errorf("status line %q does not say the workflow is created and how to start it", line)
		}
		if line := contextLineWithPrefix(ctx, "goal:"); !strings.HasPrefix(line, goal) || len(line) != len(goal)+12+1 {
			t.Errorf("goal line %q does not carry the goal id and 12 digest characters", line)
		}
	})
	t.Run("running with stages recorded", func(t *testing.T) {
		e := newHookEnv(t)
		e.running(t, "proj-1", "wf-1", "standalone-minimal", "authorize", "bound-scope")
		ctx := decodeHookOutput(t, e.hook(t, e.repo)).Context
		for _, want := range []string{"status: running", "current stage: bound-scope", "next stage: execute-one-bounded-sequential-path", "stages recorded (2): authorize, bound-scope"} {
			if !hasContextLine(ctx, want) {
				t.Errorf("no line %q in:\n%s", want, ctx)
			}
		}
		if contextLineWithPrefix(ctx, "PAUSED") != "" {
			t.Errorf("a running workflow carries a paused notice:\n%s", ctx)
		}
	})
	t.Run("paused", func(t *testing.T) {
		e := newHookEnv(t)
		e.running(t, "proj-1", "wf-1", "standalone-minimal", "authorize")
		e.step(t, "proj-1", "wf-1", "pause")
		ctx := decodeHookOutput(t, e.hook(t, e.repo)).Context
		if !hasContextLine(ctx, "status: paused") {
			t.Errorf("no paused status line in:\n%s", ctx)
		}
		notice := contextLineWithPrefix(ctx, "PAUSED")
		if !strings.Contains(notice, "do not advance") || !strings.Contains(notice, "labdrian workflow resume --project proj-1 --workflow wf-1") {
			t.Errorf("paused notice %q does not tell the session to stop advancing and how to resume", notice)
		}
	})
	t.Run("every declared stage recorded", func(t *testing.T) {
		e := newHookEnv(t)
		profile, err := workflowprofile.Resolve("standalone-minimal")
		if err != nil {
			t.Fatal(err)
		}
		var stages []string
		for _, s := range profile.Stages {
			stages = append(stages, s.Name)
		}
		e.running(t, "proj-1", "wf-1", "standalone-minimal", stages...)
		ctx := decodeHookOutput(t, e.hook(t, e.repo)).Context
		for _, want := range []string{"current stage: report", "next stage: none: every declared stage is recorded"} {
			if !hasContextLine(ctx, want) {
				t.Errorf("no line %q in:\n%s", want, ctx)
			}
		}
	})
}

func hasContextLine(ctx, line string) bool {
	for _, l := range strings.Split(ctx, "\n") {
		if l == line {
			return true
		}
	}
	return false
}

// contextLineWithPrefix returns the first line of ctx with the prefix, or "".
func contextLineWithPrefix(ctx, prefix string) string {
	for _, l := range strings.Split(ctx, "\n") {
		if strings.HasPrefix(l, prefix) {
			return l
		}
	}
	return ""
}

func TestProjectionHookStatesTheMemoryPlanOfTheProfile(t *testing.T) {
	const preface = "memory plan (read-only: it executes no query and grants no memory write): "
	for _, tt := range []struct{ profile, want string }{
		{"odd", "scope=project sources=engram,longterm-mem,procedural-skills project_id=proj-1 goal_id=none write=none"},
		{"sdd", "scope=project sources=engram project_id=proj-1 goal_id=none write=none"},
		{"standalone-minimal", "scope=none sources=none project_id=none goal_id=none write=none"},
	} {
		t.Run(tt.profile, func(t *testing.T) {
			e := newHookEnv(t)
			e.running(t, "proj-1", "wf-1", tt.profile)
			ctx := decodeHookOutput(t, e.hook(t, e.repo)).Context
			if !hasContextLine(ctx, preface+tt.want) {
				t.Errorf("no memory plan line %q in:\n%s", preface+tt.want, ctx)
			}
			// The projection is the profile's ceiling for the workflow's project,
			// whatever the process's environment holds.
			if tt.profile == "standalone-minimal" && contextLineWithPrefix(ctx, "omitted filters:") != "omitted filters: project_id, goal_id" {
				t.Errorf("a scope-none plan must say it omitted both filters:\n%s", ctx)
			}
		})
	}
}

// TestProjectionHookNamesTheUnavailableDependenciesAndTheCapabilityLimits: with
// a prober that confirms nothing (the seam installs UnavailableProber, so the
// test does not depend on what the machine running it has on PATH), every
// dependency of a workflow is recorded unavailable, and the context says so; the
// capability line is the declaration's.
func TestProjectionHookNamesTheUnavailableDependenciesAndTheCapabilityLimits(t *testing.T) {
	useUnavailableProber(t)
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "odd")
	ctx := decodeHookOutput(t, e.hook(t, e.repo)).Context

	line := contextLineWithPrefix(ctx, "unavailable dependencies at the last recorded event:")
	for _, want := range []string{"memory:engram", "memory:longterm-mem", "memory:procedural-skills", "gentle-ai-review"} {
		if !strings.Contains(line, want) {
			t.Errorf("unavailable-dependencies line %q does not name %q", line, want)
		}
	}
	d, err := capability.Declare(capability.TargetClaude)
	if err != nil {
		t.Fatal(err)
	}
	limits := contextLineWithPrefix(ctx, "capability limits (claude):")
	for _, c := range d.Claims {
		if mentioned := strings.Contains(limits, string(c.Capability)+"="); mentioned == (c.Status == capability.Supported) {
			t.Errorf("claim %s is %s but the capability line %q says otherwise", c.Capability, c.Status, limits)
		}
	}
}

// --- closed workflows -------------------------------------------------------

func TestProjectionHookAnnouncesAClosedWorkflowAndUnbindsIt(t *testing.T) {
	for _, outcome := range []string{"completed", "abandoned"} {
		t.Run(outcome, func(t *testing.T) {
			e := newHookEnv(t)
			e.running(t, "proj-1", "wf-1", "standalone-minimal")
			if outcome == "completed" {
				e.step(t, "proj-1", "wf-1", "verify", "--goal", e.goalPath("proj-1", "wf-1"))
				e.step(t, "proj-1", "wf-1", "close", "--outcome", "completed")
			} else {
				e.step(t, "proj-1", "wf-1", "close", "--outcome", "abandoned", "--reason", "not needed")
			}
			log, err := os.ReadFile(e.workflowLog("proj-1", "wf-1"))
			if err != nil {
				t.Fatal(err)
			}

			first := decodeHookOutput(t, e.hook(t, e.repo))
			if first.Warning != "" || !strings.Contains(first.Context, "closed ("+outcome+")") || strings.Contains(first.Context, "\n") {
				t.Fatalf("first call: %+v, want a one-line note that the workflow closed (%s)", first, outcome)
			}
			if !strings.Contains(first.Context, "was removed") || strings.Contains(first.Context, "failed") {
				t.Fatalf("first call: %q, want the note to say the binding was removed, because it was", first.Context)
			}
			if loaded := loadStoredBinding(t, e.repo); loaded.Classification != projection.ClassificationAbsent {
				t.Fatalf("the binding to a closed workflow is still there: %+v", loaded)
			}
			assertSilent(t, "the call after the unbind", e.hook(t, e.repo))

			if after, err := os.ReadFile(e.workflowLog("proj-1", "wf-1")); err != nil || !bytes.Equal(after, log) {
				t.Errorf("the hook changed the workflow log (%v)", err)
			}
		})
	}
}

// TestProjectionHookLeavesAFreshBindingAlone: between the hook reading a closed
// workflow and removing its binding, the user binds the next workflow. The hook
// has no right to remove that binding, so it keeps it.
func TestProjectionHookLeavesAFreshBindingAlone(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "standalone-minimal") // the repository is bound to wf-1
	e.create(t, "proj-2", "wf-2", "standalone-minimal")  // the next workflow, not bound yet
	e.step(t, "proj-2", "wf-2", "start")
	e.step(t, "proj-1", "wf-1", "close", "--outcome", "abandoned", "--reason", "done")

	store, err := newBindingStore()
	if err != nil {
		t.Fatal(err)
	}
	key := mustRepoKey(t, e.repo)
	seamRuns := 0
	beforeHookUnbind = func() {
		seamRuns++
		if err := store.Bind(key, "proj-2", "wf-2", time.Now(), true); err != nil {
			t.Errorf("Bind() from the other process = %v", err)
		}
	}
	t.Cleanup(func() { beforeHookUnbind = nil })

	r := e.hook(t, e.repo)
	if seamRuns != 1 {
		t.Fatalf("the seam ran %d times, want once: the hook did not try to unbind", seamRuns)
	}
	if !reflect.DeepEqual(r.codes, []int{0}) || r.stderr != "" {
		t.Errorf("exits %v, stderr %q, want a plain exit 0", r.codes, r.stderr)
	}
	if loaded := loadStoredBinding(t, e.repo); loaded.Classification != projection.ClassificationOwned || loaded.Binding.WorkflowID != "wf-2" {
		t.Fatalf("stored binding = %+v, want the fresh binding to wf-2 left alone", loaded)
	}
	// Nothing was removed and nothing failed: the note must claim neither.
	got := decodeHookOutput(t, r)
	if !strings.Contains(got.Context, "left alone") || strings.Contains(got.Context, "was removed") || strings.Contains(got.Context, "failed") {
		t.Errorf("note %q, want it to say the binding was left alone", got.Context)
	}
}

// TestProjectionHookStillSpeaksWhenTheBindingCannotBeRemoved: unbinding is best
// effort. A failure to remove the file never fails the hook.
func TestProjectionHookStillSpeaksWhenTheBindingCannotBeRemoved(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "standalone-minimal")
	e.step(t, "proj-1", "wf-1", "close", "--outcome", "abandoned", "--reason", "done")

	dir := filepath.Dir(e.bindingFile(t, e.repo))
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if f, err := os.Create(filepath.Join(dir, "probe")); err == nil {
		f.Close()
		os.Remove(filepath.Join(dir, "probe"))
		t.Skip("directory permissions are not enforced for this process (root or an equivalent capability); the removal cannot be made to fail")
	}

	got := decodeHookOutput(t, e.hook(t, e.repo))
	if !strings.Contains(got.Context, "closed (abandoned)") {
		t.Errorf("context = %q, want the closed note even though the binding could not be removed", got.Context)
	}
	// The note must not claim a removal that did not happen, and must name the
	// command that finishes the job by hand.
	if strings.Contains(got.Context, "was removed") || strings.Contains(got.Context, "being removed") ||
		!strings.Contains(got.Context, "failed") || !strings.Contains(got.Context, "labdrian workflow unbind") || strings.Contains(got.Context, "\n") {
		t.Errorf("context = %q, want a one-line note that says the removal failed and names 'labdrian workflow unbind'", got.Context)
	}
	if loaded := loadStoredBinding(t, e.repo); loaded.Classification != projection.ClassificationOwned {
		t.Errorf("binding = %+v, want it still there: the removal was not possible", loaded)
	}
}

// --- states it cannot follow ------------------------------------------------

// warningOnly requires the hook to have printed one warning and no context,
// exited 0, and left the whole state directory exactly as it was.
func warningOnly(t *testing.T, e hookEnv, wantIn ...string) {
	t.Helper()
	before, beforeTree := snapshotContents(t, e.state), snapshotTree(t, e.state)
	r := e.hook(t, e.repo)
	msg := decodeHookOutput(t, r)
	if msg.Context != "" || msg.Warning == "" || !reflect.DeepEqual(msg.Keys, []string{"systemMessage"}) {
		t.Fatalf("output %s, want a systemMessage only: one warning and nothing projected", r.stdout)
	}
	for _, want := range wantIn {
		if !strings.Contains(msg.Warning, want) {
			t.Errorf("warning %q does not mention %q", msg.Warning, want)
		}
	}
	if strings.Contains(msg.Warning, "\n") {
		t.Errorf("warning %q spans more than one line", msg.Warning)
	}
	if after := snapshotContents(t, e.state); !reflect.DeepEqual(after, before) {
		t.Errorf("the hook changed the state directory:\nbefore %v\nafter  %v", before, after)
	}
	if after := snapshotTree(t, e.state); !reflect.DeepEqual(after, beforeTree) {
		t.Errorf("the hook changed a mode, size, or time in the state directory:\nbefore %v\nafter  %v", beforeTree, after)
	}
}

func TestProjectionHookWarnsAboutAWorkflowItCannotFollow(t *testing.T) {
	tests := []struct {
		name   string
		break_ func(t *testing.T, e hookEnv)
		wantIn []string
	}{
		{"the workflow log is gone", func(t *testing.T, e hookEnv) {
			if err := os.Remove(e.workflowLog("proj-1", "wf-1")); err != nil {
				t.Fatal(err)
			}
		}, []string{"does not exist"}},
		{"the workflow log is foreign", func(t *testing.T, e hookEnv) {
			writeFixtureFile(t, e.workflowLog("proj-1", "wf-1"), `{"hello":"world"}`+"\n")
		}, []string{"is foreign"}},
		{"the workflow log is malformed", func(t *testing.T, e hookEnv) {
			writeFixtureFile(t, e.workflowLog("proj-1", "wf-1"), "not json\n")
		}, []string{"is malformed"}},
		{"the workflow log drifted", func(t *testing.T, e hookEnv) {
			driftWorkflowLog(t, e.workflowLog("proj-1", "wf-1"))
		}, []string{"is drifted"}},
		{"the workflow log cannot be read", func(t *testing.T, e hookEnv) {
			log := e.workflowLog("proj-1", "wf-1")
			if err := os.Chmod(log, 0); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(log, 0o600) })
			if f, err := os.Open(log); err == nil {
				f.Close()
				t.Skip("chmod 0 is not enforced for this process (root or an equivalent capability); the unreadable-log fault cannot be injected")
			}
		}, []string{"is unavailable"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newHookEnv(t)
			e.running(t, "proj-1", "wf-1", "standalone-minimal")
			tt.break_(t, e)
			warningOnly(t, e, append([]string{"wf-1", "proj-1", "labdrian workflow status --project proj-1 --workflow wf-1", "labdrian workflow unbind"}, tt.wantIn...)...)
			// The binding is untouched: an unknown state is not a closed workflow.
			if loaded := loadStoredBinding(t, e.repo); loaded.Classification != projection.ClassificationOwned {
				t.Errorf("binding = %+v, want it kept", loaded)
			}
			// And it is the same on the next prompt: still a warning, never a
			// silent unbind.
			warningOnly(t, e, "wf-1")
		})
	}
}

func TestProjectionHookWarnsAboutABindingItCannotUse(t *testing.T) {
	tests := []struct {
		name  string
		plant func(t *testing.T, path string)
		want  string
	}{
		{"a foreign binding file", func(t *testing.T, path string) { writeFixtureFile(t, path, `{"hello":"world"}`+"\n") }, "foreign"},
		{"a malformed binding file", func(t *testing.T, path string) { writeFixtureFile(t, path, "hand-written notes\n") }, "malformed"},
		{"a directory in its place", func(t *testing.T, path string) {
			if err := os.MkdirAll(path, 0o700); err != nil {
				t.Fatal(err)
			}
		}, "unavailable"},
		{"a binding file that cannot be read", func(t *testing.T, path string) {
			writeFixtureFile(t, path, "{}\n")
			if err := os.Chmod(path, 0); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
			if f, err := os.Open(path); err == nil {
				f.Close()
				t.Skip("chmod 0 is not enforced for this process (root or an equivalent capability); the unreadable-file fault cannot be injected")
			}
		}, "unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newHookEnv(t)
			e.running(t, "proj-1", "wf-1", "standalone-minimal") // a workflow the binding could have named
			path := e.bindingFile(t, e.repo)
			if err := os.RemoveAll(path); err != nil {
				t.Fatal(err)
			}
			tt.plant(t, path)
			warningOnly(t, e, tt.want, "labdrian workflow binding")
		})
	}
}

// --- read-only, deterministic, restartable ----------------------------------

// TestProjectionHookChangesNothingForAnOpenWorkflow is the read-only proof: for a
// created, running, or paused workflow the whole state directory (contents,
// modes, sizes, and times) is the same after the hook as before it.
func TestProjectionHookChangesNothingForAnOpenWorkflow(t *testing.T) {
	for _, status := range []string{"created", "running", "paused"} {
		t.Run(status, func(t *testing.T) {
			e := newHookEnv(t)
			e.create(t, "proj-1", "wf-1", "standalone-minimal")
			if status != "created" {
				e.step(t, "proj-1", "wf-1", "start")
				e.step(t, "proj-1", "wf-1", "stage", "--stage", "authorize")
			}
			if status == "paused" {
				e.step(t, "proj-1", "wf-1", "pause")
			}
			mustBindOK(t, e.repo, "proj-1", "wf-1")

			contents, tree := snapshotContents(t, e.state), snapshotTree(t, e.state)
			for i := 0; i < 3; i++ {
				if msg := decodeHookOutput(t, e.hook(t, e.repo)); msg.Context == "" {
					t.Fatalf("call %d: no context for a %s workflow", i, status)
				}
			}
			if after := snapshotContents(t, e.state); !reflect.DeepEqual(after, contents) {
				t.Errorf("the hook changed file contents:\nbefore %v\nafter  %v", contents, after)
			}
			if after := snapshotTree(t, e.state); !reflect.DeepEqual(after, tree) {
				t.Errorf("the hook changed a mode, size, or time:\nbefore %v\nafter  %v", tree, after)
			}
		})
	}
}

// TestProjectionHookOutputIsDeterministicAcrossCallsAndSessions: the same state
// gives the same bytes on every call, whatever the session id and the prompt
// are, because the projection depends on neither.
func TestProjectionHookOutputIsDeterministicAcrossCallsAndSessions(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "odd", "authorize", "explore")
	want := e.hook(t, e.repo)
	decodeHookOutput(t, want)

	for i := 0; i < 20; i++ {
		got := runHook(hookInput(t, e.repo, "session-"+strings.Repeat("x", i), "prompt number "+strings.Repeat("y", i*50)), e.dir)
		if got.stdout != want.stdout {
			t.Fatalf("call %d (another session and prompt) differs:\n%s\nvs\n%s", i, got.stdout, want.stdout)
		}
	}
	for _, noSession := range []string{`{"cwd":"` + e.repo + `"}`, `{"cwd":"` + e.repo + `","session_id":null,"prompt":""}`} {
		if got := runHook(noSession, e.dir); got.stdout != want.stdout {
			t.Errorf("input %s: differs from the input with a session and a prompt", noSession)
		}
	}
	// Nothing that varies between calls is in the text: no session, no prompt,
	// no timestamp.
	for _, varying := range []string{"session-", "prompt number", "a prompt"} {
		if strings.Contains(want.stdout, varying) {
			t.Errorf("stdout contains %q, which varies between calls", varying)
		}
	}
	if stamp := regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`).FindString(want.stdout); stamp != "" {
		t.Errorf("stdout contains the timestamp %q", stamp)
	}
}

// TestProjectionHookSeesTheSameWorkflowFromEveryWorktree: the binding is keyed by
// the repository, not the checkout, so a session in any worktree, or in any
// subdirectory of one, is told the same thing.
func TestProjectionHookSeesTheSameWorkflowFromEveryWorktree(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "sdd", "dispatcher-selected-planning-phases")
	first := fixtureLinkedWorktree(t, e.repo, "wt1")
	second := fixtureLinkedWorktree(t, e.repo, "wt2")
	sub := filepath.Join(second, "some", "deeper", "dir")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}

	want := e.hook(t, e.repo)
	if decodeHookOutput(t, want).Context == "" {
		t.Fatal("test bug: no context for the main checkout")
	}
	for name, cwd := range map[string]string{"the first worktree": first, "the second worktree": second, "a subdirectory of it": sub} {
		if got := e.hook(t, cwd); got.stdout != want.stdout {
			t.Errorf("%s: the output differs from the main checkout's:\n%s\nvs\n%s", name, got.stdout, want.stdout)
		}
	}
}

// TestProjectionHookOutputIsIdenticalAcrossProcesses runs the built binary as
// separate processes, each a restart: no memory is shared between them, only the
// files. The output matches, byte for byte, the in-process hook's, whatever the
// session, and a repository that is not bound stays silent.
//
// Safety: HOME and XDG_STATE_HOME of every process are this test's own
// temporary directories.
func TestProjectionHookOutputIsIdenticalAcrossProcesses(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "odd", "authorize")
	other := fixtureRepo(t, "unbound")
	binary := phase6BuildEngineBinary(t)
	home := t.TempDir()

	run := func(cwd, stdin string) (string, string, int) {
		t.Helper()
		cmd := exec.Command(binary, "projection", "hook", "--event", "UserPromptSubmit")
		cmd.Dir = cwd
		cmd.Env = []string{"HOME=" + home, "XDG_STATE_HOME=" + e.state, "PATH=" + os.Getenv("PATH")}
		cmd.Stdin = strings.NewReader(stdin)
		var out, errBuf bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errBuf
		code := 0
		if err := cmd.Run(); err != nil {
			exitErr, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("run the binary: %v", err)
			}
			code = exitErr.ExitCode()
		}
		return out.String(), errBuf.String(), code
	}

	inProcess := e.hook(t, e.repo)
	decodeHookOutput(t, inProcess)
	for i, session := range []string{"session-a", "session-b", "session-c"} {
		stdout, stderr, code := run(e.dir, hookInput(t, e.repo, session, "prompt "+session))
		if code != 0 || stderr != "" || stdout != inProcess.stdout {
			t.Fatalf("process %d: exit %d, stderr %q, stdout\n%s\nwant exit 0 and the in-process output\n%s", i, code, stderr, stdout, inProcess.stdout)
		}
	}
	// The same, started from inside a worktree and told nothing else.
	worktree := fixtureLinkedWorktree(t, e.repo, "wt")
	if stdout, _, code := run(worktree, `{"hook_event_name":"UserPromptSubmit"}`); code != 0 || stdout != inProcess.stdout {
		t.Errorf("process in a worktree: exit %d, stdout\n%s\nwant the in-process output", code, stdout)
	}
	// Silent for a repository nothing is bound to, and usage errors are exit 1.
	if stdout, stderr, code := run(other, hookInput(t, other, "s", "p")); stdout != "" || stderr != "" || code != 0 {
		t.Errorf("unbound repository: exit %d, stdout %q, stderr %q, want a silent exit 0", code, stdout, stderr)
	}
	cmd := exec.Command(binary, "projection", "hook")
	cmd.Env = []string{"HOME=" + home, "XDG_STATE_HOME=" + e.state, "PATH=" + os.Getenv("PATH")}
	if err := cmd.Run(); err == nil {
		t.Error("projection hook without --event exited 0")
	} else if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
		t.Errorf("projection hook without --event: %v, want exit 1", err)
	}
}

// --- output failures and the command line -----------------------------------

// TestProjectionHookTurnsAPanicIntoExitZero: a Go panic ends a process with
// status 2, which Claude Code reads as "block this prompt". The seam panics
// where a bug in the hook would, and the prompt must still go through: exit 0,
// the cause on stderr for whoever debugs it, and one visible, sanitized,
// bounded systemMessage so the user is not left guessing why nothing was
// projected. The panic value is hostile on purpose: multi-line, an escape
// sequence, and very long.
func TestProjectionHookTurnsAPanicIntoExitZero(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "standalone-minimal")
	e.step(t, "proj-1", "wf-1", "close", "--outcome", "abandoned", "--reason", "done")
	beforeHookUnbind = func() { panic("boom\nsecond line \x1b[31mred " + strings.Repeat("x", 5000)) }
	t.Cleanup(func() { beforeHookUnbind = nil })

	r := e.hook(t, e.repo)
	if !reflect.DeepEqual(r.codes, []int{0}) || !strings.Contains(r.stderr, "internal error: boom") {
		t.Fatalf("exits %v, stderr %q, want exit 0 and the panic reported on stderr", r.codes, r.stderr)
	}
	// decodeHookOutput insists on an empty stderr, so check the stdout half by hand.
	stdout := hookRun{codes: r.codes, stdout: r.stdout}
	got := decodeHookOutput(t, stdout)
	if got.Context != "" || !strings.Contains(got.Warning, "boom second line") || !strings.Contains(got.Warning, "internal error") {
		t.Fatalf("output %+v, want only a systemMessage naming the internal error", got)
	}
	if strings.ContainsAny(got.Warning, "\n\x1b") || len(got.Warning) > 700 {
		t.Errorf("warning %q (%d bytes) is not one short clean line", got.Warning, len(got.Warning))
	}
}

// TestProjectionHookWarnsWhenTheStoreCannotBeOpened: a real Go error from the
// binding store (here a relative XDG_STATE_HOME, which the store refuses) is not
// "no binding". The hook cannot know whether the repository is bound, so it says
// it could not check, once, and projects nothing. It still exits 0.
func TestProjectionHookWarnsWhenTheStoreCannotBeOpened(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "standalone-minimal")
	t.Setenv("XDG_STATE_HOME", "relative/state")

	got := decodeHookOutput(t, e.hook(t, e.repo))
	if got.Context != "" || !strings.Contains(got.Warning, "XDG_STATE_HOME") || !strings.Contains(got.Warning, "no workflow is projected") {
		t.Fatalf("output %+v, want only a warning that carries the reason and says nothing is projected", got)
	}
	if strings.Contains(got.Warning, "\n") || len(got.Warning) > 700 {
		t.Errorf("warning %q is not one short line", got.Warning)
	}
}

// TestProjectionHookStaysSilentWhenTheReaderFails: a stdin that cannot be read
// gives the hook nothing to decide from and no binding to be loyal to, so it is
// the same silent exit 0 as input it cannot use.
func TestProjectionHookStaysSilentWhenTheReaderFails(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "standalone-minimal")
	var out, errBuf bytes.Buffer
	var codes []int
	runProjectionCore(hookArgs, e.dir, failingReader{}, &out, &errBuf, func(c int) { codes = append(codes, c) })
	assertSilent(t, "a failing stdin", hookRun{codes: codes, stdout: out.String(), stderr: errBuf.String()})
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestProjectionHookExitsZeroEvenWhenStdoutCannotBeWritten(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "standalone-minimal")
	var errBuf bytes.Buffer
	var codes []int
	runProjectionCore(hookArgs, e.dir, strings.NewReader(hookInput(t, e.repo, "s", "p")), failingMemoryWriter{}, &errBuf, func(c int) { codes = append(codes, c) })
	if !reflect.DeepEqual(codes, []int{0}) || errBuf.String() != "" {
		t.Errorf("exits %v, stderr %q, want exit 0 and no stderr: a hook must never fail the prompt over its own output", codes, errBuf.String())
	}
}

func TestProjectionHookRefusesBadCommandLinesWithExit1(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{"no action", nil, "requires an action"},
		{"an unknown action", []string{"status"}, `unknown action "status"`},
		{"an action given as a flag", []string{"--event", "UserPromptSubmit"}, "requires an action"},
		{"no event", []string{"hook"}, "--event is required"},
		{"an event flag without its value", []string{"hook", "--event"}, "--event requires a value"},
		{"an empty event", []string{"hook", "--event", ""}, "--event is required"},
		{"an event in the wrong case for the gate", []string{"hook", "--event", "pretooluse"}, `unsupported --event "pretooluse"`},
		{"two events joined", []string{"hook", "--event", "UserPromptSubmit|PreToolUse"}, `unsupported --event "UserPromptSubmit|PreToolUse"`},
		{"an event in the wrong case", []string{"hook", "--event", "userpromptsubmit"}, `unsupported --event "userpromptsubmit"`},
		{"an unknown event", []string{"hook", "--event", "Stop"}, `unsupported --event "Stop"`},
		{"an unknown flag", []string{"hook", "--event", "UserPromptSubmit", "--json"}, `unknown flag "--json"`},
		{"the flag of another command", []string{"hook", "--event", "UserPromptSubmit", "--cwd", "/tmp"}, `unknown flag "--cwd"`},
		{"the flag joined to its value", []string{"hook", "--event=UserPromptSubmit"}, `unknown flag "--event=UserPromptSubmit"`},
		{"a stray argument", []string{"hook", "--event", "UserPromptSubmit", "extra"}, `unexpected argument "extra"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Stdin holds a valid input for a bound repository; a usage error
			// must not answer it.
			e := newHookEnv(t)
			e.running(t, "proj-1", "wf-1", "standalone-minimal")
			r := runProjectionArgs(tt.args, hookInput(t, e.repo, "s", "p"), e.dir)
			if !reflect.DeepEqual(r.codes, []int{1}) || r.stdout != "" || !strings.Contains(r.stderr, tt.want) || !strings.HasPrefix(r.stderr, "error: projection") {
				t.Fatalf("exits %v, stdout %q, stderr %q, want exit 1, no stdout, and 'error: projection ...' containing %q", r.codes, r.stdout, r.stderr, tt.want)
			}
		})
	}
}

func TestProjectionHookTakesTheLastOfARepeatedEventFlag(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "standalone-minimal")
	r := runProjectionArgs([]string{"hook", "--event", "Stop", "--event", "UserPromptSubmit"}, hookInput(t, e.repo, "s", "p"), e.dir)
	if decodeHookOutput(t, r).Context == "" {
		t.Errorf("output %q, want the context: the last --event is the one that counts", r.stdout)
	}
}

// --- help -------------------------------------------------------------------

func TestUsageDocumentsTheProjectionHook(t *testing.T) {
	text := strings.Join(strings.Fields(captureUsage(t)), " ")
	for _, want := range []string{
		"engine projection hook --event UserPromptSubmit",
		"internal Claude Code hook command",
		"install-hooks installs it",
		"re-run install-hooks and restart Claude Code",
		`"hookSpecificOutput"`,
		"silent",
		"exit 0 always",
		"1 usage error",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("usage does not contain %q", want)
		}
	}
}

// The hook family is installed by install-hooks, so no user-facing text may
// still say it is not, and each says what a user must do to get it: re-run
// install-hooks on an existing install and restart Claude Code.
func TestDocsSayInstallHooksInstallsTheProjectionHooks(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	for name, raw := range map[string]string{"README": string(readme), "usage()": captureUsage(t)} {
		text := strings.Join(strings.Fields(strings.ReplaceAll(raw, "`", "")), " ")
		for _, stale := range []string{"does not install it into Claude Code settings yet", "install-hooks does not install it yet", "no session receives its output today"} {
			if strings.Contains(text, stale) {
				t.Errorf("%s still says %q", name, stale)
			}
		}
		for _, want := range []string{"install-hooks installs", "re-run install-hooks", "restart Claude Code"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s does not say %q", name, want)
			}
		}
	}
}
