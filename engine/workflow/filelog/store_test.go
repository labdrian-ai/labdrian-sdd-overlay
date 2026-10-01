package filelog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/statestore"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// validCreatedEvent is the seq-0 created event of proj-1/wf-1; validStartedEvent
// is the started event that follows it.
func validCreatedEvent() workflow.WorkflowEvent {
	return workflow.WorkflowEvent{
		Version:    1,
		WorkflowID: "wf-1",
		ProjectID:  "proj-1",
		Seq:        0,
		PrevDigest: "",
		Kind:       workflow.KindCreated,
		At:         "2026-09-28T10:00:00Z",
		Provenance: workflow.Provenance{
			WorktreeRoot: "/home/labdrian/labdrian-sdd-overlay",
			GitHead:      "e1218c2f00000000000000000000000000000000",
		},
		Observations: []workflow.Observation{},
		GoalID:       "goal-1",
		GoalDigest:   strings.Repeat("a", 64),
		Profile:      "odd",
	}
}

func validStartedEvent() workflow.WorkflowEvent {
	created := validCreatedEvent()
	prevDigest, err := workflow.EventDigest(created)
	if err != nil {
		panic(err)
	}
	e := created
	e.Seq = 1
	e.PrevDigest = prevDigest
	e.Kind = workflow.KindStarted
	e.GoalID, e.GoalDigest, e.Profile = "", "", ""
	return e
}

// setStoreEnv points XDG_STATE_HOME at a fresh temp dir and an unusable HOME,
// so NewStore always resolves the same isolated root and every test in this
// file is guaranteed never to touch the real user's state home.
func setStoreEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", root)
	return root
}

func newTestStore(t *testing.T) Store {
	t.Helper()
	setStoreEnv(t)
	s, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() = %v, want nil", err)
	}
	return s
}

func storeFilePath(t *testing.T, root, projectID, workflowID string) string {
	t.Helper()
	return filepath.Join(root, "labdrian", "workflows", projectID, workflowID+".jsonl")
}

// Which platforms have a store is decided where the code is built: statestore
// compiles Supported to true on linux and darwin and to false everywhere else.
// This is the mapping from the running platform to the answer NewStore acts on, so
// the expectation is computed from the platform and the input is what NewStore
// passes; neither is a value the test chooses to make itself pass.
func TestTheRunningPlatformIsSupportedOnlyOnLinuxAndDarwin(t *testing.T) {
	want := runtime.GOOS == "linux" || runtime.GOOS == "darwin"
	if statestore.Supported != want {
		t.Fatalf("statestore.Supported = %v on %s, want %v: only linux and darwin have a no-follow read", statestore.Supported, runtime.GOOS, want)
	}
	err := checkPlatform(statestore.Supported, runtime.GOOS)
	if want != (err == nil) {
		t.Fatalf("checkPlatform on %s = %v, want it to accept exactly the platforms that have a store", runtime.GOOS, err)
	}
}

// A platform without a store is refused with the sentinel and named in the message.
func TestCheckPlatformRefusesAnUnsupportedPlatformAndNamesIt(t *testing.T) {
	for _, goos := range []string{"windows", "freebsd", "plan9"} {
		err := checkPlatform(false, goos)
		if !errors.Is(err, ErrUnsupportedPlatform) || !strings.Contains(err.Error(), goos) {
			t.Errorf("checkPlatform(false, %q) = %v, want ErrUnsupportedPlatform naming the platform", goos, err)
		}
	}
	if err := checkPlatform(true, "linux"); err != nil {
		t.Errorf("checkPlatform(true, linux) = %v, want nil", err)
	}
}

// The platforms that run these tests are the ones that have a store.
func TestThisPlatformHasAStore(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("the workflow store is only implemented on linux and darwin")
	}
	setStoreEnv(t)
	if _, err := NewStore(); err != nil {
		t.Fatalf("NewStore() = %v, want nil on %s", err, runtime.GOOS)
	}
}

func TestNewStoreRejectsRelativeXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "relative/path")
	if _, err := NewStore(); err == nil {
		t.Fatalf("NewStore() = nil, want error for relative XDG_STATE_HOME")
	}
}

func TestNewStoreRejectsUnusableHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "relative/home")
	if _, err := NewStore(); err == nil {
		t.Fatalf("NewStore() = nil, want error for relative HOME fallback")
	}

	t.Setenv("HOME", "")
	if _, err := NewStore(); err == nil {
		t.Fatalf("NewStore() = nil, want error for unset HOME")
	}
}

// TestNewStoreErrorMessagesAreStable characterizes what NewStore reports for an
// unusable environment, so the state home resolution (statestore.Home)
// cannot change the text a person sees.
func TestNewStoreErrorMessagesAreStable(t *testing.T) {
	tests := []struct {
		name string
		xdg  string
		home string
		want string
	}{
		{"relative XDG_STATE_HOME", "relative/path", "/home/someone", `workflow store: XDG_STATE_HOME "relative/path" is not absolute`},
		{"relative HOME fallback", "", "relative/home", `workflow store: XDG_STATE_HOME is unset and HOME "relative/home" is not an absolute path`},
		{"unset HOME fallback", "", "", `workflow store: XDG_STATE_HOME is unset and HOME "" is not an absolute path`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", tt.xdg)
			t.Setenv("HOME", tt.home)
			_, err := NewStore()
			if err == nil || err.Error() != tt.want {
				t.Fatalf("NewStore() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestStoreLoadAbsentWhenNoFile(t *testing.T) {
	s := newTestStore(t)
	loaded, err := s.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if loaded.Classification != workflow.ClassificationAbsent {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, workflow.ClassificationAbsent)
	}
}

func TestStoreLoadRejectsInvalidIdentifiers(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Load("../etc", "wf-1"); err == nil {
		t.Fatalf("Load() = nil, want error for unsafe project_id")
	}
	if _, err := s.Load("proj-1", "../etc"); err == nil {
		t.Fatalf("Load() = nil, want error for unsafe workflow_id")
	}
}

func TestStoreAppendCreatesOwnedWorkflow(t *testing.T) {
	s := newTestStore(t)
	created := validCreatedEvent()
	if err := s.Append(created.ProjectID, created.WorkflowID, created); err != nil {
		t.Fatalf("Append() = %v, want nil", err)
	}
	loaded, err := s.Load(created.ProjectID, created.WorkflowID)
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if loaded.Classification != workflow.ClassificationOwned {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, workflow.ClassificationOwned)
	}
	if len(loaded.Events) != 1 || loaded.Events[0].Kind != workflow.KindCreated {
		t.Fatalf("Load() events = %+v, want one created event", loaded.Events)
	}
	if loaded.State.Status != workflow.StatusCreated {
		t.Fatalf("Load() state.Status = %q, want %q", loaded.State.Status, workflow.StatusCreated)
	}
}

func TestStoreAppendRejectsCreateWithWrongSeqOrKind(t *testing.T) {
	s := newTestStore(t)
	started := validStartedEvent()
	started.Seq = 0
	started.PrevDigest = ""
	if err := s.Append(started.ProjectID, started.WorkflowID, started); err == nil {
		t.Fatalf("Append() = nil, want error for non-created first event")
	}
	loaded, err := s.Load(started.ProjectID, started.WorkflowID)
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if loaded.Classification != workflow.ClassificationAbsent {
		t.Fatalf("Load() classification = %q, want %q (a rejected append must not create the file)", loaded.Classification, workflow.ClassificationAbsent)
	}
}

func TestStoreAppendChainOfEvents(t *testing.T) {
	s := newTestStore(t)
	created := validCreatedEvent()
	if err := s.Append(created.ProjectID, created.WorkflowID, created); err != nil {
		t.Fatalf("Append(created) = %v, want nil", err)
	}
	createdDigest, err := workflow.EventDigest(created)
	if err != nil {
		t.Fatalf("workflow.EventDigest() = %v, want nil", err)
	}
	started := created
	started.Seq = 1
	started.PrevDigest = createdDigest
	started.Kind = workflow.KindStarted
	started.GoalID, started.GoalDigest, started.Profile = "", "", ""
	if err := s.Append(started.ProjectID, started.WorkflowID, started); err != nil {
		t.Fatalf("Append(started) = %v, want nil", err)
	}
	startedDigest, err := workflow.EventDigest(started)
	if err != nil {
		t.Fatalf("workflow.EventDigest() = %v, want nil", err)
	}
	stage := started
	stage.Seq = 2
	stage.PrevDigest = startedDigest
	stage.Kind = workflow.KindStageRecorded
	stage.Stage = "explore"
	if err := s.Append(stage.ProjectID, stage.WorkflowID, stage); err != nil {
		t.Fatalf("Append(stage) = %v, want nil", err)
	}

	loaded, err := s.Load(created.ProjectID, created.WorkflowID)
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if loaded.Classification != workflow.ClassificationOwned {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, workflow.ClassificationOwned)
	}
	if len(loaded.Events) != 3 {
		t.Fatalf("Load() events = %d, want 3", len(loaded.Events))
	}
	if loaded.State.Status != workflow.StatusRunning {
		t.Fatalf("Load() state.Status = %q, want %q", loaded.State.Status, workflow.StatusRunning)
	}
	if len(loaded.State.Stages) != 1 || loaded.State.Stages[0] != "explore" {
		t.Fatalf("Load() state.Stages = %v, want [explore]", loaded.State.Stages)
	}

	// Wrong seq/prev_digest linkage must be refused without mutating the file.
	before, err := os.ReadFile(storeFilePath(t, s.stateHome, created.ProjectID, created.WorkflowID))
	if err != nil {
		t.Fatalf("os.ReadFile() = %v, want nil", err)
	}
	badNext := stage
	badNext.Seq = 5
	if err := s.Append(badNext.ProjectID, badNext.WorkflowID, badNext); err == nil {
		t.Fatalf("Append() = nil, want error for wrong seq")
	}
	after, err := os.ReadFile(storeFilePath(t, s.stateHome, created.ProjectID, created.WorkflowID))
	if err != nil {
		t.Fatalf("os.ReadFile() = %v, want nil", err)
	}
	if string(before) != string(after) {
		t.Fatalf("Append() with bad seq mutated the stored file")
	}
}

func TestStoreAppendRejectsIllegalTransition(t *testing.T) {
	s := newTestStore(t)
	created := validCreatedEvent()
	if err := s.Append(created.ProjectID, created.WorkflowID, created); err != nil {
		t.Fatalf("Append(created) = %v, want nil", err)
	}
	createdDigest, err := workflow.EventDigest(created)
	if err != nil {
		t.Fatalf("workflow.EventDigest() = %v, want nil", err)
	}
	// paused is illegal directly from created.
	paused := created
	paused.Seq = 1
	paused.PrevDigest = createdDigest
	paused.Kind = workflow.KindPaused
	paused.GoalID, paused.GoalDigest, paused.Profile = "", "", ""
	if err := s.Append(paused.ProjectID, paused.WorkflowID, paused); !errors.Is(err, workflow.ErrInvalidTransition) {
		t.Fatalf("Append() err = %v, want workflow.ErrInvalidTransition", err)
	}
}

func writeRawLog(t *testing.T, root, projectID, workflowID, content string) {
	t.Helper()
	path := storeFilePath(t, root, projectID, workflowID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("os.MkdirAll() = %v, want nil", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("os.WriteFile() = %v, want nil", err)
	}
}

// A rejected first append against an absent workflow must be validated the
// same way the store validates every later append against an owned
// workflow: identifiers must match the requested workflow, and the linkage
// fields (here, prev_digest at seq 0) must be legal for the state being
// appended to (here, the zero state before any event exists).
func TestStoreAppendRejectsMismatchedIdentifiersOnFirstEvent(t *testing.T) {
	s := newTestStore(t)
	created := validCreatedEvent()
	created.ProjectID = "other-project"
	if err := s.Append("proj-1", "wf-1", created); err == nil {
		t.Fatalf("Append() = nil, want error for a first event whose project_id does not match the requested workflow")
	}
	loaded, err := s.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if loaded.Classification != workflow.ClassificationAbsent {
		t.Fatalf("Load() classification = %q, want %q (a rejected append must not create the file)", loaded.Classification, workflow.ClassificationAbsent)
	}
}

// TestStoreAppendRejectsNonZeroSeqOnFirstEvent guards the explicit,
// non-delegated seq/kind check in Append's absent-classification branch:
// CheckTransition(State{}, next) enforces the same rule against the zero
// State, but a first append must be rejected by Append's own local check
// too, as an independent line of defense.
func TestStoreAppendRejectsNonZeroSeqOnFirstEvent(t *testing.T) {
	s := newTestStore(t)
	created := validCreatedEvent()
	created.Seq = 1
	created.PrevDigest = strings.Repeat("a", 64)
	if err := s.Append(created.ProjectID, created.WorkflowID, created); err == nil {
		t.Fatalf("Append() = nil, want error for a first event with a non-zero seq")
	}
	loaded, err := s.Load(created.ProjectID, created.WorkflowID)
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if loaded.Classification != workflow.ClassificationAbsent {
		t.Fatalf("Load() classification = %q, want %q (a rejected append must not create the file)", loaded.Classification, workflow.ClassificationAbsent)
	}
}

func TestStoreAppendRejectsNonEmptyPrevDigestOnFirstEvent(t *testing.T) {
	s := newTestStore(t)
	created := validCreatedEvent()
	created.PrevDigest = strings.Repeat("a", 64)
	if err := s.Append(created.ProjectID, created.WorkflowID, created); err == nil {
		t.Fatalf("Append() = nil, want error for a first event with a non-empty prev_digest")
	}
	loaded, err := s.Load(created.ProjectID, created.WorkflowID)
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if loaded.Classification != workflow.ClassificationAbsent {
		t.Fatalf("Load() classification = %q, want %q (a rejected append must not create the file)", loaded.Classification, workflow.ClassificationAbsent)
	}
}

// ClassifyLog's Detail messages report 1-based line numbers, matching
// how a human reading the file (or an editor's line gutter) would count
// lines; the previous 0-based index made "line 0" point at the file's first
// line, which is confusing to a person debugging a malformed log by hand.
func TestStoreLoadMalformedDetailUsesOneBasedLineNumbers(t *testing.T) {
	root := setStoreEnv(t)
	s, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() = %v, want nil", err)
	}
	writeRawLog(t, root, "proj-1", "wf-1", "not json at all\n")
	loaded, err := s.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if loaded.Classification != workflow.ClassificationMalformed {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, workflow.ClassificationMalformed)
	}
	if !strings.Contains(loaded.Detail, "line 1 ") {
		t.Fatalf("Load() detail = %q, want it to reference the 1-based %q", loaded.Detail, "line 1")
	}
}

// TestStoreLoadMalformedBlankLineDetailUsesOneBasedLineNumbers covers the
// blank-line malformed branch specifically: TestStoreLoadMalformedDetailUsesOneBasedLineNumbers
// above only exercises the "not valid JSON" branch, and the blank-line
// branch has its own, separate fmt.Sprintf call site.
func TestStoreLoadMalformedBlankLineDetailUsesOneBasedLineNumbers(t *testing.T) {
	root := setStoreEnv(t)
	s, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() = %v, want nil", err)
	}
	created := validCreatedEvent()
	line, err := created.MarshalLine()
	if err != nil {
		t.Fatalf("MarshalLine() = %v, want nil", err)
	}
	// A blank second line: valid first line, then an empty line.
	writeRawLog(t, root, created.ProjectID, created.WorkflowID, string(line)+"\n")
	loaded, err := s.Load(created.ProjectID, created.WorkflowID)
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if loaded.Classification != workflow.ClassificationMalformed {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, workflow.ClassificationMalformed)
	}
	if !strings.Contains(loaded.Detail, "line 2 ") {
		t.Fatalf("Load() detail = %q, want it to reference the 1-based %q", loaded.Detail, "line 2")
	}
}

func TestStoreLoadForeignWhenValidJSONNotOurs(t *testing.T) {
	root := setStoreEnv(t)
	s, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() = %v, want nil", err)
	}
	writeRawLog(t, root, "proj-1", "wf-1", `{"hello":"world"}`+"\n")
	loaded, err := s.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if loaded.Classification != workflow.ClassificationForeign {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, workflow.ClassificationForeign)
	}
	if !strings.Contains(loaded.Detail, "line 1 ") {
		t.Fatalf("Load() detail = %q, want it to reference the 1-based %q", loaded.Detail, "line 1")
	}
}

func TestStoreLoadForeignWhenIDsMismatch(t *testing.T) {
	root := setStoreEnv(t)
	s, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() = %v, want nil", err)
	}
	other := validCreatedEvent()
	other.WorkflowID = "other-workflow"
	line, err := other.MarshalLine()
	if err != nil {
		t.Fatalf("MarshalLine() = %v, want nil", err)
	}
	// Stored under proj-1/wf-1 but the event inside declares a different
	// workflow_id.
	writeRawLog(t, root, "proj-1", "wf-1", string(line))
	loaded, err := s.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if loaded.Classification != workflow.ClassificationForeign {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, workflow.ClassificationForeign)
	}
	if !strings.Contains(loaded.Detail, "line 1 ") {
		t.Fatalf("Load() detail = %q, want it to reference the 1-based %q", loaded.Detail, "line 1")
	}
}

func TestStoreLoadMalformedWhenNotValidJSON(t *testing.T) {
	root := setStoreEnv(t)
	s, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() = %v, want nil", err)
	}
	writeRawLog(t, root, "proj-1", "wf-1", "not json at all\n")
	loaded, err := s.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if loaded.Classification != workflow.ClassificationMalformed {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, workflow.ClassificationMalformed)
	}
}

func TestStoreLoadMalformedWhenNotUTF8(t *testing.T) {
	root := setStoreEnv(t)
	s, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() = %v, want nil", err)
	}
	writeRawLog(t, root, "proj-1", "wf-1", string([]byte{0xff, 0xfe, 0xfd})+"\n")
	loaded, err := s.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if loaded.Classification != workflow.ClassificationMalformed {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, workflow.ClassificationMalformed)
	}
}

func TestStoreLoadMalformedWhenMissingTrailingNewline(t *testing.T) {
	root := setStoreEnv(t)
	s, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() = %v, want nil", err)
	}
	e := validCreatedEvent()
	line, err := e.MarshalLine()
	if err != nil {
		t.Fatalf("MarshalLine() = %v, want nil", err)
	}
	writeRawLog(t, root, "proj-1", "wf-1", strings.TrimSuffix(string(line), "\n"))
	loaded, err := s.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if loaded.Classification != workflow.ClassificationMalformed {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, workflow.ClassificationMalformed)
	}
}

func TestStoreLoadMalformedWhenEmpty(t *testing.T) {
	root := setStoreEnv(t)
	s, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() = %v, want nil", err)
	}
	writeRawLog(t, root, "proj-1", "wf-1", "")
	loaded, err := s.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if loaded.Classification != workflow.ClassificationMalformed {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, workflow.ClassificationMalformed)
	}
}

func TestStoreLoadMalformedWhenOversized(t *testing.T) {
	root := setStoreEnv(t)
	s, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() = %v, want nil", err)
	}
	padding := strings.Repeat("x", workflow.MaxLogBytes+1)
	writeRawLog(t, root, "proj-1", "wf-1", padding+"\n")
	loaded, err := s.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if loaded.Classification != workflow.ClassificationMalformed {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, workflow.ClassificationMalformed)
	}
}

// writeSparseLog makes a log file of size bytes that occupies almost no disk: the
// size is real to every reader, the content is zeros.
func writeSparseLog(t *testing.T, root, projectID, workflowID string, size int64) {
	t.Helper()
	writeRawLog(t, root, projectID, workflowID, "")
	if err := os.Truncate(storeFilePath(t, root, projectID, workflowID), size); err != nil {
		t.Fatalf("os.Truncate() = %v, want nil", err)
	}
}

// A log larger than workflow.MaxLogBytes is malformed and is never read in full: the
// documented bound is on what the store reads, so a huge file costs the bound, not
// its size. The detail names the real size, as it always has.
func TestStoreLoadNeverReadsMoreThanTheDocumentedBound(t *testing.T) {
	const size = 512 << 20
	root := setStoreEnv(t)
	s, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() = %v, want nil", err)
	}
	writeSparseLog(t, root, "proj-1", "wf-1", size)

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	loaded, err := s.Load("proj-1", "wf-1")
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}

	if loaded.Classification != workflow.ClassificationMalformed {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, workflow.ClassificationMalformed)
	}
	if want := fmt.Sprintf("workflow log is %d bytes, exceeding the maximum of %d", size, workflow.MaxLogBytes); loaded.Detail != want {
		t.Errorf("Load() detail = %q, want %q", loaded.Detail, want)
	}
	// Reading the bound allocates a few times the bound while the buffer grows;
	// reading the whole file allocates several times its size.
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 8*workflow.MaxLogBytes {
		t.Errorf("Load() of a %d byte log allocated %d bytes, want at most %d: the log must not be read past the bound", size, allocated, 8*workflow.MaxLogBytes)
	}
}

// The bound is exact: a log of MaxLogBytes is judged on its content like any other,
// one byte more is judged on its size.
func TestStoreLoadJudgesSizeOnlyBeyondTheBound(t *testing.T) {
	tests := []struct {
		name       string
		size       int64
		wantOnSize bool
	}{
		{"at the bound", workflow.MaxLogBytes, false},
		{"one byte over", workflow.MaxLogBytes + 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := setStoreEnv(t)
			s, err := NewStore()
			if err != nil {
				t.Fatalf("NewStore() = %v, want nil", err)
			}
			writeSparseLog(t, root, "proj-1", "wf-1", tt.size)
			loaded, err := s.Load("proj-1", "wf-1")
			if err != nil {
				t.Fatalf("Load() = %v, want nil", err)
			}
			if loaded.Classification != workflow.ClassificationMalformed {
				t.Fatalf("Load() classification = %q, want malformed", loaded.Classification)
			}
			if onSize := strings.Contains(loaded.Detail, "exceeding the maximum"); onSize != tt.wantOnSize {
				t.Errorf("Load() detail = %q, size judged = %v, want %v", loaded.Detail, onSize, tt.wantOnSize)
			}
		})
	}
}

// The same bound holds for an append: an oversized log is refused as malformed, and
// left exactly as it was.
func TestStoreAppendRefusesAnOversizedLogWithoutReadingItAll(t *testing.T) {
	const size = 64 << 20
	root := setStoreEnv(t)
	s, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() = %v, want nil", err)
	}
	writeSparseLog(t, root, "proj-1", "wf-1", size)

	err = s.Append("proj-1", "wf-1", validCreatedEvent())
	if !errors.Is(err, workflow.ErrRefuseMalformedState) {
		t.Fatalf("Append() = %v, want ErrRefuseMalformedState", err)
	}
	info, statErr := os.Stat(storeFilePath(t, root, "proj-1", "wf-1"))
	if statErr != nil || info.Size() != size {
		t.Fatalf("log after the refused append: size %v, err %v, want %d bytes untouched", info, statErr, size)
	}
}

func TestStoreLoadDriftedWhenChainBroken(t *testing.T) {
	root := setStoreEnv(t)
	s, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() = %v, want nil", err)
	}
	created := validCreatedEvent()
	createdLine, err := created.MarshalLine()
	if err != nil {
		t.Fatalf("MarshalLine() = %v, want nil", err)
	}
	started := validStartedEvent()
	started.PrevDigest = strings.Repeat("f", 64) // wrong: breaks the chain
	startedLine, err := started.MarshalLine()
	if err != nil {
		t.Fatalf("MarshalLine() = %v, want nil", err)
	}
	writeRawLog(t, root, created.ProjectID, created.WorkflowID, string(createdLine)+string(startedLine))
	loaded, err := s.Load(created.ProjectID, created.WorkflowID)
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if loaded.Classification != workflow.ClassificationDrifted {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, workflow.ClassificationDrifted)
	}
}

func TestStoreLoadDriftedWhenTransitionIllegal(t *testing.T) {
	root := setStoreEnv(t)
	s, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() = %v, want nil", err)
	}
	// A structurally valid, well-chained single event whose kind (started)
	// is illegal as the first event of a workflow: VerifyEvents accepts it
	// (chain/seq are fine), but Replay rejects it.
	e := validCreatedEvent()
	e.Kind = workflow.KindStarted
	e.GoalID, e.GoalDigest, e.Profile = "", "", ""
	line, err := e.MarshalLine()
	if err != nil {
		t.Fatalf("MarshalLine() = %v, want nil", err)
	}
	writeRawLog(t, root, e.ProjectID, e.WorkflowID, string(line))
	loaded, err := s.Load(e.ProjectID, e.WorkflowID)
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if loaded.Classification != workflow.ClassificationDrifted {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, workflow.ClassificationDrifted)
	}
}

func TestStoreAppendRefusesNonOwnedStatesWithoutMutation(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr error
	}{
		{"foreign", `{"hello":"world"}` + "\n", workflow.ErrRefuseForeignState},
		{"malformed", "not json\n", workflow.ErrRefuseMalformedState},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := setStoreEnv(t)
			s, err := NewStore()
			if err != nil {
				t.Fatalf("NewStore() = %v, want nil", err)
			}
			writeRawLog(t, root, "proj-1", "wf-1", tt.content)
			before, err := os.ReadFile(storeFilePath(t, root, "proj-1", "wf-1"))
			if err != nil {
				t.Fatalf("os.ReadFile() = %v, want nil", err)
			}
			next := validCreatedEvent()
			next.ProjectID, next.WorkflowID = "proj-1", "wf-1"
			if err := s.Append("proj-1", "wf-1", next); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Append() err = %v, want %v", err, tt.wantErr)
			}
			after, err := os.ReadFile(storeFilePath(t, root, "proj-1", "wf-1"))
			if err != nil {
				t.Fatalf("os.ReadFile() = %v, want nil", err)
			}
			if string(before) != string(after) {
				t.Fatalf("Append() mutated a refused (%s) state", tt.name)
			}
		})
	}
}

func TestStoreRoundTripAfterRestart(t *testing.T) {
	root := setStoreEnv(t)
	s1, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() = %v, want nil", err)
	}
	created := validCreatedEvent()
	if err := s1.Append(created.ProjectID, created.WorkflowID, created); err != nil {
		t.Fatalf("Append() = %v, want nil", err)
	}

	// Simulate a process restart: a brand new Store value resolved fresh
	// from the same environment must see the same state.
	s2, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() = %v, want nil", err)
	}
	if s2.stateHome != filepath.Clean(root) {
		t.Fatalf("NewStore() stateHome = %q, want %q", s2.stateHome, root)
	}
	loaded, err := s2.Load(created.ProjectID, created.WorkflowID)
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if loaded.Classification != workflow.ClassificationOwned || len(loaded.Events) != 1 {
		t.Fatalf("Load() = %+v, want one owned event after restart", loaded)
	}
}

func TestStoreDirectoryAndFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on windows")
	}
	root := setStoreEnv(t)
	s, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() = %v, want nil", err)
	}
	created := validCreatedEvent()
	if err := s.Append(created.ProjectID, created.WorkflowID, created); err != nil {
		t.Fatalf("Append() = %v, want nil", err)
	}

	dir := filepath.Join(root, "labdrian", "workflows", created.ProjectID)
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("os.Stat(dir) = %v, want nil", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode = %o, want 0700", info.Mode().Perm())
	}

	file := storeFilePath(t, root, created.ProjectID, created.WorkflowID)
	finfo, err := os.Stat(file)
	if err != nil {
		t.Fatalf("os.Stat(file) = %v, want nil", err)
	}
	if finfo.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %o, want 0600", finfo.Mode().Perm())
	}
}

func TestStoreLoadUnavailableOnPermissionDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	root := setStoreEnv(t)
	s, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() = %v, want nil", err)
	}
	created := validCreatedEvent()
	if err := s.Append(created.ProjectID, created.WorkflowID, created); err != nil {
		t.Fatalf("Append() = %v, want nil", err)
	}
	dir := filepath.Join(root, "labdrian", "workflows", created.ProjectID)
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatalf("os.Chmod() = %v, want nil", err)
	}
	defer os.Chmod(dir, 0o700)

	loaded, err := s.Load(created.ProjectID, created.WorkflowID)
	if err != nil {
		t.Fatalf("Load() = %v, want nil (unavailable is reported via Classification, not error)", err)
	}
	if loaded.Classification != workflow.ClassificationUnavailable {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, workflow.ClassificationUnavailable)
	}
}

func TestStoreConcurrentAppendExactlyOneWins(t *testing.T) {
	s := newTestStore(t)
	created := validCreatedEvent()
	if err := s.Append(created.ProjectID, created.WorkflowID, created); err != nil {
		t.Fatalf("Append(created) = %v, want nil", err)
	}
	createdDigest, err := workflow.EventDigest(created)
	if err != nil {
		t.Fatalf("workflow.EventDigest() = %v, want nil", err)
	}
	started := created
	started.Seq = 1
	started.PrevDigest = createdDigest
	started.Kind = workflow.KindStarted
	started.GoalID, started.GoalDigest, started.Profile = "", "", ""

	var wg sync.WaitGroup
	errs := make([]error, 2)
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = s.Append(started.ProjectID, started.WorkflowID, started)
		}(i)
	}
	close(start)
	wg.Wait()

	successes, conflicts := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			successes++
		// The loser is refused one of two ways, depending on timing: it
		// finds the append lock held (workflow.ErrAppendConflict), or it takes the
		// lock after the winner released it and finds its seq already
		// stored (workflow.ErrStaleSeq). Both leave the log unchanged.
		case errors.Is(err, workflow.ErrAppendConflict), errors.Is(err, workflow.ErrStaleSeq):
			conflicts++
		default:
			t.Fatalf("Append() err = %v, want nil, workflow.ErrAppendConflict, or workflow.ErrStaleSeq", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent Append() successes=%d refusals=%d, want 1 and 1", successes, conflicts)
	}

	loaded, err := s.Load(created.ProjectID, created.WorkflowID)
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if loaded.Classification != workflow.ClassificationOwned {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, workflow.ClassificationOwned)
	}
	if len(loaded.Events) != 2 {
		t.Fatalf("Load() events = %d, want 2 (exactly one append should have won)", len(loaded.Events))
	}
}

// setupStoreWithCreated appends the seq-0 created event and returns the
// store, its lock file path, and the next legal (seq-1) event.
func setupStoreWithCreated(t *testing.T) (Store, string, workflow.WorkflowEvent) {
	t.Helper()
	root := setStoreEnv(t)
	s, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() = %v, want nil", err)
	}
	created := validCreatedEvent()
	if err := s.Append(created.ProjectID, created.WorkflowID, created); err != nil {
		t.Fatalf("Append(created) = %v, want nil", err)
	}
	createdDigest, err := workflow.EventDigest(created)
	if err != nil {
		t.Fatalf("workflow.EventDigest() = %v, want nil", err)
	}
	started := created
	started.Seq = 1
	started.PrevDigest = createdDigest
	started.Kind = workflow.KindStarted
	started.GoalID, started.GoalDigest, started.Profile = "", "", ""
	lockPath := filepath.Join(root, "labdrian", "workflows", created.ProjectID, created.WorkflowID+".lock")
	return s, lockPath, started
}

func TestStoreLiveLockIsNeverStolenRegardlessOfAge(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("flock-based locking is only implemented on linux/darwin")
	}
	s, lockPath, started := setupStoreWithCreated(t)
	release, err := acquireLock(lockPath)
	if err != nil {
		t.Fatalf("acquireLock() = %v, want nil", err)
	}
	defer release()
	// Age alone must never reclaim a live lock: only flock ownership matters.
	old := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatalf("os.Chtimes() = %v, want nil", err)
	}
	if err := s.Append(started.ProjectID, started.WorkflowID, started); !errors.Is(err, workflow.ErrAppendConflict) {
		t.Fatalf("Append() = %v, want workflow.ErrAppendConflict (a live lock must never be stolen)", err)
	}
}

func TestStoreReleaseNeverDeletesALockItDoesNotHold(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("flock-based locking is only implemented on linux/darwin")
	}
	s, lockPath, started := setupStoreWithCreated(t)
	release, err := acquireLock(lockPath)
	if err != nil {
		t.Fatalf("acquireLock() = %v, want nil", err)
	}
	release()
	if _, err := os.Lstat(lockPath); err != nil {
		t.Fatalf("os.Lstat(lockPath) = %v, want nil (release must not delete the lock file)", err)
	}
	if err := s.Append(started.ProjectID, started.WorkflowID, started); err != nil {
		t.Fatalf("Append() = %v, want nil (a released lock must allow the next Append)", err)
	}
}

// The append lock is private like the rest of the store, and a contended append is
// refused at once, not queued: the caller gets workflow.ErrAppendConflict and decides.
func TestStoreAppendLockFileIsPrivateAndAContendedAppendIsRefusedAtOnce(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("flock-based locking is only implemented on linux/darwin")
	}
	s, lockPath, started := setupStoreWithCreated(t)

	info, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("os.Stat(lockPath) = %v, want nil", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Errorf("lock file mode = %v, want a regular file with mode 0600", info.Mode())
	}

	release, err := acquireLock(lockPath)
	if err != nil {
		t.Fatalf("acquireLock() = %v, want nil", err)
	}
	defer release()
	begin := time.Now()
	err = s.Append(started.ProjectID, started.WorkflowID, started)
	if !errors.Is(err, workflow.ErrAppendConflict) || !strings.Contains(err.Error(), lockPath) {
		t.Fatalf("Append() = %v, want workflow.ErrAppendConflict naming %s", err, lockPath)
	}
	if took := time.Since(begin); took > 500*time.Millisecond {
		t.Errorf("a contended Append took %v, want an immediate refusal", took)
	}
}

// TestStoreAppendOfAnAlreadyStoredSeqIsErrStaleSeq pins the refusal a caller
// gets when the log advanced past the seq it built its event for: the same
// outcome the loser of a concurrent append sees once the winner has finished.
func TestStoreAppendOfAnAlreadyStoredSeqIsErrStaleSeq(t *testing.T) {
	s := newTestStore(t)
	created := validCreatedEvent()
	if err := s.Append(created.ProjectID, created.WorkflowID, created); err != nil {
		t.Fatalf("Append(created) = %v, want nil", err)
	}
	createdDigest, err := workflow.EventDigest(created)
	if err != nil {
		t.Fatalf("workflow.EventDigest() = %v, want nil", err)
	}
	started := created
	started.Seq = 1
	started.PrevDigest = createdDigest
	started.Kind = workflow.KindStarted
	started.GoalID, started.GoalDigest, started.Profile = "", "", ""
	if err := s.Append(started.ProjectID, started.WorkflowID, started); err != nil {
		t.Fatalf("first Append(started) = %v, want nil", err)
	}

	err = s.Append(started.ProjectID, started.WorkflowID, started)
	if !errors.Is(err, workflow.ErrStaleSeq) {
		t.Fatalf("second Append(started) err = %v, want workflow.ErrStaleSeq", err)
	}
	if errors.Is(err, workflow.ErrAppendConflict) {
		t.Fatalf("second Append(started) err = %v, must not be workflow.ErrAppendConflict: no append was in progress", err)
	}
	loaded, err := s.Load(created.ProjectID, created.WorkflowID)
	if err != nil || len(loaded.Events) != 2 {
		t.Fatalf("Load() = %d events, err %v; want 2 events and nil", len(loaded.Events), err)
	}
}
