package workflow

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

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

func TestStoreLoadAbsentWhenNoFile(t *testing.T) {
	s := newTestStore(t)
	loaded, err := s.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if loaded.Classification != ClassificationAbsent {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, ClassificationAbsent)
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
	if loaded.Classification != ClassificationOwned {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, ClassificationOwned)
	}
	if len(loaded.Events) != 1 || loaded.Events[0].Kind != KindCreated {
		t.Fatalf("Load() events = %+v, want one created event", loaded.Events)
	}
	if loaded.State.Status != StatusCreated {
		t.Fatalf("Load() state.Status = %q, want %q", loaded.State.Status, StatusCreated)
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
	if loaded.Classification != ClassificationAbsent {
		t.Fatalf("Load() classification = %q, want %q (a rejected append must not create the file)", loaded.Classification, ClassificationAbsent)
	}
}

func TestStoreAppendChainOfEvents(t *testing.T) {
	s := newTestStore(t)
	created := validCreatedEvent()
	if err := s.Append(created.ProjectID, created.WorkflowID, created); err != nil {
		t.Fatalf("Append(created) = %v, want nil", err)
	}
	createdDigest, err := EventDigest(created)
	if err != nil {
		t.Fatalf("EventDigest() = %v, want nil", err)
	}
	started := created
	started.Seq = 1
	started.PrevDigest = createdDigest
	started.Kind = KindStarted
	started.GoalID, started.GoalDigest, started.Profile = "", "", ""
	if err := s.Append(started.ProjectID, started.WorkflowID, started); err != nil {
		t.Fatalf("Append(started) = %v, want nil", err)
	}
	startedDigest, err := EventDigest(started)
	if err != nil {
		t.Fatalf("EventDigest() = %v, want nil", err)
	}
	stage := started
	stage.Seq = 2
	stage.PrevDigest = startedDigest
	stage.Kind = KindStageRecorded
	stage.Stage = "explore"
	if err := s.Append(stage.ProjectID, stage.WorkflowID, stage); err != nil {
		t.Fatalf("Append(stage) = %v, want nil", err)
	}

	loaded, err := s.Load(created.ProjectID, created.WorkflowID)
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if loaded.Classification != ClassificationOwned {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, ClassificationOwned)
	}
	if len(loaded.Events) != 3 {
		t.Fatalf("Load() events = %d, want 3", len(loaded.Events))
	}
	if loaded.State.Status != StatusRunning {
		t.Fatalf("Load() state.Status = %q, want %q", loaded.State.Status, StatusRunning)
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
	createdDigest, err := EventDigest(created)
	if err != nil {
		t.Fatalf("EventDigest() = %v, want nil", err)
	}
	// paused is illegal directly from created.
	paused := created
	paused.Seq = 1
	paused.PrevDigest = createdDigest
	paused.Kind = KindPaused
	paused.GoalID, paused.GoalDigest, paused.Profile = "", "", ""
	if err := s.Append(paused.ProjectID, paused.WorkflowID, paused); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Append() err = %v, want ErrInvalidTransition", err)
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
	if loaded.Classification != ClassificationForeign {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, ClassificationForeign)
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
	if loaded.Classification != ClassificationForeign {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, ClassificationForeign)
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
	if loaded.Classification != ClassificationMalformed {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, ClassificationMalformed)
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
	if loaded.Classification != ClassificationMalformed {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, ClassificationMalformed)
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
	if loaded.Classification != ClassificationMalformed {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, ClassificationMalformed)
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
	if loaded.Classification != ClassificationMalformed {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, ClassificationMalformed)
	}
}

func TestStoreLoadMalformedWhenOversized(t *testing.T) {
	root := setStoreEnv(t)
	s, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() = %v, want nil", err)
	}
	padding := strings.Repeat("x", maxWorkflowLogBytes+1)
	writeRawLog(t, root, "proj-1", "wf-1", padding+"\n")
	loaded, err := s.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if loaded.Classification != ClassificationMalformed {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, ClassificationMalformed)
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
	if loaded.Classification != ClassificationDrifted {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, ClassificationDrifted)
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
	e.Kind = KindStarted
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
	if loaded.Classification != ClassificationDrifted {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, ClassificationDrifted)
	}
}

func TestStoreAppendRefusesNonOwnedStatesWithoutMutation(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr error
	}{
		{"foreign", `{"hello":"world"}` + "\n", ErrRefuseForeignState},
		{"malformed", "not json\n", ErrRefuseMalformedState},
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
	if loaded.Classification != ClassificationOwned || len(loaded.Events) != 1 {
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
	if loaded.Classification != ClassificationUnavailable {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, ClassificationUnavailable)
	}
}

func TestStoreConcurrentAppendExactlyOneWins(t *testing.T) {
	s := newTestStore(t)
	created := validCreatedEvent()
	if err := s.Append(created.ProjectID, created.WorkflowID, created); err != nil {
		t.Fatalf("Append(created) = %v, want nil", err)
	}
	createdDigest, err := EventDigest(created)
	if err != nil {
		t.Fatalf("EventDigest() = %v, want nil", err)
	}
	started := created
	started.Seq = 1
	started.PrevDigest = createdDigest
	started.Kind = KindStarted
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
		case errors.Is(err, ErrAppendConflict):
			conflicts++
		default:
			t.Fatalf("Append() err = %v, want nil or ErrAppendConflict", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent Append() successes=%d conflicts=%d, want 1 and 1", successes, conflicts)
	}

	loaded, err := s.Load(created.ProjectID, created.WorkflowID)
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if loaded.Classification != ClassificationOwned {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, ClassificationOwned)
	}
	if len(loaded.Events) != 2 {
		t.Fatalf("Load() events = %d, want 2 (exactly one append should have won)", len(loaded.Events))
	}
}

// setupStoreWithCreated appends the seq-0 created event and returns the
// store, its lock file path, and the next legal (seq-1) event.
func setupStoreWithCreated(t *testing.T) (Store, string, WorkflowEvent) {
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
	createdDigest, err := EventDigest(created)
	if err != nil {
		t.Fatalf("EventDigest() = %v, want nil", err)
	}
	started := created
	started.Seq = 1
	started.PrevDigest = createdDigest
	started.Kind = KindStarted
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
	if err := s.Append(started.ProjectID, started.WorkflowID, started); !errors.Is(err, ErrAppendConflict) {
		t.Fatalf("Append() = %v, want ErrAppendConflict (a live lock must never be stolen)", err)
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
