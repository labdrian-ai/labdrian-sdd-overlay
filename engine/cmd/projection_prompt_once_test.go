package main

// Tests for S19 part (b): the UserPromptSubmit hook adds the workflow block to a session only when it
// differs from the last block it added to that session, and always on the session's first prompt, and
// the SessionStart hook makes the next prompt add it again. Every case goes through the command, over a
// state home of the test's own (bindEnv sets XDG_STATE_HOME).

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// promptIn runs the UserPromptSubmit hook for the repository at cwd as the session sends it.
func (e hookEnv) promptIn(t *testing.T, session string) hookRun {
	t.Helper()
	return runProjectionArgsWith(e.deps, hookArgs, hookInput(t, e.repo, session, "a prompt"), e.dir)
}

// startIn runs the SessionStart hook of the session.
func (e hookEnv) startIn(t *testing.T, session, source string) hookRun {
	t.Helper()
	input := strings.Replace(sessionStartInput(t, e.repo, source), `"session-1"`, `"`+session+`"`, 1)
	return runProjectionArgsWith(e.deps, []string{"hook", "--event", "SessionStart"}, input, e.dir)
}

func (e hookEnv) promptDir() string { return filepath.Join(e.state, "labdrian", "projection-prompt") }

func requireBlock(t *testing.T, label string, r hookRun) {
	t.Helper()
	if msg := decodeHookOutput(t, r); msg.Context == "" {
		t.Fatalf("%s: stdout %q has no block, want one", label, r.stdout)
	}
}

func TestTheSecondPromptOfASessionWithTheSameBlockIsSilent(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "standalone-minimal", "authorize")
	first := e.promptIn(t, "session-1")
	requireBlock(t, "first prompt", first)
	assertSilent(t, "the same block again", e.promptIn(t, "session-1"))
	assertSilent(t, "and again", e.promptIn(t, "session-1"))
	other := e.promptIn(t, "session-2")
	if !reflect.DeepEqual(other, first) {
		t.Errorf("first prompt of another session = %+v, want the first prompt's %+v", other, first)
	}
}

func TestAChangedBlockIsAdded(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "standalone-minimal", "authorize")
	a := e.promptIn(t, "session-1")
	requireBlock(t, "A", a)
	e.step(t, "proj-1", "wf-1", "stage", "--stage", "bound-scope")
	b := e.promptIn(t, "session-1")
	requireBlock(t, "B", b)
	if b.stdout == a.stdout {
		t.Fatalf("test bug: the stage did not change the block")
	}
	assertSilent(t, "B again", e.promptIn(t, "session-1"))
}

// A, then B, then A: the third prompt adds its block, because the last block added was B's, whatever
// the session saw before it. The two blocks are those of two workflows the repository is bound to in turn.
func TestABlockThatReturnsAfterAnotherIsAddedAgain(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "standalone-minimal", "authorize")
	e.create(t, "proj-1", "wf-2", "standalone-minimal")
	e.step(t, "proj-1", "wf-2", "start")
	bind := func(wf string) {
		t.Helper()
		phase6MustExitZeroWith(t, e.deps, "unbind", []string{"unbind"}, e.repo)
		mustBindOK(t, e.repo, "proj-1", wf)
	}
	a := e.promptIn(t, "session-1")
	requireBlock(t, "A", a)
	bind("wf-2")
	b := e.promptIn(t, "session-1")
	requireBlock(t, "B", b)
	if b.stdout == a.stdout {
		t.Fatalf("test bug: the two workflows gave the same block")
	}
	bind("wf-1")
	if third := e.promptIn(t, "session-1"); !reflect.DeepEqual(third, a) {
		t.Errorf("third prompt (A again) = %+v, want the block %+v printed again", third, a)
	}
	assertSilent(t, "A once more", e.promptIn(t, "session-1"))
}

// The one thing the hook writes for a prompt with a session is its own store, and a session is hashed there.
func TestAPromptWithASessionWritesOnlyUnderItsOwnStore(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "standalone-minimal")
	before := snapshotContents(t, e.state)
	requireBlock(t, "first", e.promptIn(t, "a-session-name"))
	after := snapshotContents(t, e.state)
	for path, sum := range after {
		if before[path] == sum {
			continue
		}
		if !strings.HasPrefix(path, filepath.Join("labdrian", "projection-prompt")) {
			t.Errorf("the hook changed %s outside its store", path)
		}
		if strings.Contains(path, "a-session-name") {
			t.Errorf("the session name reached the path %s", path)
		}
	}
	if len(after) <= len(before) {
		t.Errorf("the hook made no store entry")
	}
}

func TestAPromptWithoutASessionPrintsEveryTimeAndRecordsNothing(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "standalone-minimal")
	run := func() hookRun {
		return runProjectionArgsWith(e.deps, hookArgs, `{"hook_event_name":"UserPromptSubmit","cwd":`+quote(e.repo)+`,"prompt":"p"}`, e.dir)
	}
	first, second := run(), run()
	requireBlock(t, "first", first)
	if !reflect.DeepEqual(first, second) {
		t.Errorf("second = %+v, want the first's %+v", second, first)
	}
	numeric := runProjectionArgsWith(e.deps, hookArgs, `{"session_id":7,"hook_event_name":"UserPromptSubmit","cwd":`+quote(e.repo)+`}`, e.dir)
	if !reflect.DeepEqual(numeric, first) {
		t.Errorf("a session id that is not text = %+v, want the same as no session %+v", numeric, first)
	}
	if _, err := os.Stat(e.promptDir()); err == nil {
		t.Errorf("a store was made for prompts with no session")
	}
}

func TestAStoreThatCannotBeReadOrWrittenPrintsAsBefore(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "standalone-minimal")
	want := e.promptIn(t, "session-1")
	requireBlock(t, "reference", want)
	if err := os.RemoveAll(e.promptDir()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(e.promptDir(), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if got := e.promptIn(t, "session-1"); !reflect.DeepEqual(got, want) {
			t.Fatalf("prompt %d over a broken store = %+v, want %+v", i+1, got, want)
		}
	}
	if got := e.startIn(t, "session-1", "startup"); got.stderr != "" || !reflect.DeepEqual(got.codes, []int{0}) {
		t.Errorf("SessionStart over a broken store = %+v, want exit 0 and no stderr", got)
	}
}

func TestAWarningIsAlwaysPrintedAndNeverRecorded(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "standalone-minimal")
	driftWorkflowLog(t, e.workflowLog("proj-1", "wf-1"))
	first, second := e.promptIn(t, "session-1"), e.promptIn(t, "session-1")
	if msg := decodeHookOutput(t, first); !strings.Contains(msg.Warning, "is drifted") || msg.Context != "" {
		t.Fatalf("first = %+v, want the drift warning with no block", msg)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("second = %+v, want the same warning as the first %+v", second, first)
	}
	if _, err := os.Stat(e.promptDir()); err == nil {
		t.Errorf("a warning was recorded")
	}
}

func TestAWarningDoesNotHideTheBlockThatFollowsIt(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "standalone-minimal")
	requireBlock(t, "first", e.promptIn(t, "session-1"))
	log := e.workflowLog("proj-1", "wf-1")
	good, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	driftWorkflowLog(t, log)
	if msg := decodeHookOutput(t, e.promptIn(t, "session-1")); msg.Warning == "" {
		t.Fatalf("want the warning")
	}
	if err := os.WriteFile(log, good, 0o600); err != nil {
		t.Fatal(err)
	}
	assertSilent(t, "the same block after the warning", e.promptIn(t, "session-1"))
}

func TestSessionStartMakesTheNextPromptAddTheBlockAgain(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "odd")
	first := e.promptIn(t, "session-1")
	requireBlock(t, "first", first)
	assertSilent(t, "same block", e.promptIn(t, "session-1"))
	other := e.promptIn(t, "session-2")
	requireBlock(t, "other session", other)

	for _, source := range []string{"startup", "resume", "clear", "compact"} {
		if r := e.startIn(t, "session-1", source); r.stdout == "" || r.stderr != "" {
			t.Fatalf("%s: SessionStart = %+v, want its block", source, r)
		}
		if got := e.promptIn(t, "session-1"); !reflect.DeepEqual(got, first) {
			t.Fatalf("%s: prompt after SessionStart = %+v, want the block again %+v", source, got, first)
		}
		assertSilent(t, source+": then silent", e.promptIn(t, "session-1"))
	}
	assertSilent(t, "the other session was not reset", e.promptIn(t, "session-2"))
}

func TestSessionStartPrintsTheSameWhateverTheSession(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "odd")
	withoutSession := runProjectionArgsWith(e.deps, []string{"hook", "--event", "SessionStart"},
		`{"hook_event_name":"SessionStart","cwd":`+quote(e.repo)+`,"source":"startup"}`, e.dir)
	requireSessionBlock(t, withoutSession)
	_ = e.promptIn(t, "session-1")
	for _, s := range []string{"session-1", "session-never-seen"} {
		if got := e.startIn(t, s, "startup"); !reflect.DeepEqual(got, withoutSession) {
			t.Errorf("SessionStart of %s = %+v, want %+v", s, got, withoutSession)
		}
	}
}

func requireSessionBlock(t *testing.T, r hookRun) {
	t.Helper()
	if r.stdout == "" || r.stderr != "" || !reflect.DeepEqual(r.codes, []int{0}) {
		t.Fatalf("SessionStart = %+v, want a block and exit 0", r)
	}
}

// A block is recorded as the session's last only once it was written: when stdout fails the prompt goes
// through without it, and the next prompt must add it.
func TestABlockThatWasNotWrittenIsAddedByTheNextPrompt(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "standalone-minimal")
	input := hookInput(t, e.repo, "session-1", "a prompt")
	var codes []int
	var errBuf strings.Builder
	runProjectionCore(e.deps, hookArgs, e.dir, strings.NewReader(input), brokenStdout{errors.New("broken pipe")}, &errBuf, func(c int) { codes = append(codes, c) })
	if !reflect.DeepEqual(codes, []int{0}) {
		t.Fatalf("exits %v, want [0] whatever stdout does", codes)
	}
	requireBlock(t, "the prompt after a failed write", e.promptIn(t, "session-1"))
	assertSilent(t, "and then the same block is not added again", e.promptIn(t, "session-1"))
}

// The record is of a source and a session together: the same session id sent by two runtimes is two sessions.
func TestTheSameSessionIdUnderTwoSourcesIsTwoSessions(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "odd")
	run := func(event, source string, input string) hookRun {
		return runProjectionArgsWith(e.deps, []string{"hook", "--event", event, "--source", source}, input, e.dir)
	}
	prompt := hookInput(t, e.repo, "shared-id", "a prompt")
	start := strings.Replace(sessionStartInput(t, e.repo, "startup"), `"session-1"`, `"shared-id"`, 1)
	first := run("UserPromptSubmit", "claude", prompt)
	requireBlock(t, "claude", first)
	// OpenCode's block names OpenCode's capability limits (S10, L117), so it is its own block, not Claude's.
	other := run("UserPromptSubmit", "opencode", prompt)
	requireBlock(t, "opencode", other)
	assertSilent(t, "claude again", run("UserPromptSubmit", "claude", prompt))
	assertSilent(t, "opencode again", run("UserPromptSubmit", "opencode", prompt))
	if r := run("SessionStart", "opencode", start); r.stdout == "" {
		t.Fatalf("SessionStart = %+v, want its block", r)
	}
	if got := run("UserPromptSubmit", "opencode", prompt); !reflect.DeepEqual(got, other) {
		t.Errorf("opencode after its SessionStart = %+v, want the block again", got)
	}
	assertSilent(t, "a SessionStart of opencode does not reset claude", run("UserPromptSubmit", "claude", prompt))
}

// quote is s as a JSON string.
func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"` }
