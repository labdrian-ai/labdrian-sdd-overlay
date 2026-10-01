package filelog

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// The on-disk log is a contract with every earlier version of the program: a log
// written by one must be read, extended and rewritten by the next, byte for byte,
// and the other way round. testdata/golden-v1.jsonl is a log recorded from the
// version that kept this store in the workflow package (commit 4293a52): a
// throwaway program built on that version appended the eight events below, one
// after the other, to a fresh state home, and copied the log after each append.
// Every one of those eight copies is the first k lines of the file, which is what
// an append-only log is, so the one file holds all of them.
//
// The workflow it records, wf-golden of proj-golden on the odd profile, uses every
// event kind and every optional field: created (with a role chain and two
// observations), started, two stage_recorded, paused, resumed, verified (with its
// checked digests) and closed (completed).
const (
	goldenProject  = "proj-golden"
	goldenWorkflow = "wf-golden"
	goldenEvents   = 8
)

// goldenLog returns the recorded log and its lines, each with its newline.
func goldenLog(t *testing.T) ([]byte, []string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "golden-v1.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.SplitAfter(string(data), "\n")
	lines = lines[:len(lines)-1] // SplitAfter leaves an empty tail after the last newline
	if len(lines) != goldenEvents {
		t.Fatalf("the recorded log has %d lines, want %d", len(lines), goldenEvents)
	}
	return data, lines
}

// goldenEvent is the k-th recorded event, read back from its recorded line.
func goldenEvent(t *testing.T, lines []string, k int) workflow.WorkflowEvent {
	t.Helper()
	e, err := workflow.ParseWorkflowEvent([]byte(strings.TrimSuffix(lines[k], "\n")))
	if err != nil {
		t.Fatalf("recorded event %d: %v", k, err)
	}
	return e
}

func logPath(root string) string {
	return filepath.Join(root, "labdrian", "workflows", goldenProject, goldenWorkflow+".jsonl")
}

// The log the earlier version wrote is read as it always was: ours, whole, and
// replayed to the state it ended in.
func TestARecordedLogIsReadAsOwnedAndReplayedToItsFinalState(t *testing.T) {
	data, _ := goldenLog(t)
	root := setStoreEnv(t)
	s, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	writeRawLog(t, root, goldenProject, goldenWorkflow, string(data))

	loaded, err := s.Load(goldenProject, goldenWorkflow)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Classification != workflow.ClassificationOwned || loaded.Detail != "" {
		t.Fatalf("Load() = %q (%s), want owned", loaded.Classification, loaded.Detail)
	}
	wantKinds := []workflow.Kind{
		workflow.KindCreated, workflow.KindStarted, workflow.KindStageRecorded, workflow.KindStageRecorded,
		workflow.KindPaused, workflow.KindResumed, workflow.KindVerified, workflow.KindClosed,
	}
	if len(loaded.Events) != len(wantKinds) {
		t.Fatalf("Load() events = %d, want %d", len(loaded.Events), len(wantKinds))
	}
	for i, want := range wantKinds {
		if loaded.Events[i].Kind != want {
			t.Errorf("event %d kind = %q, want %q", i, loaded.Events[i].Kind, want)
		}
	}
	st := loaded.State
	if st.Status != workflow.StatusClosed || st.CloseOutcome != workflow.OutcomeCompleted ||
		st.Profile != "odd" || st.GoalID != "goal-golden" || st.RoleChainID != "chain-golden" ||
		strings.Join(st.Stages, ",") != "explore,design" || st.LastVerifiedSeq != 6 {
		t.Errorf("replayed state = %+v, want the closed odd workflow with stages explore and design, verified at seq 6", st)
	}
	if !bytes.Equal(data, mustRead(t, logPath(root))) {
		t.Error("reading the log changed it")
	}
}

// An append extends what is there and nothing else: the recorded log, at every
// length, plus the next recorded event, is the recorded log one line longer, to the
// byte. The bytes already stored are never re-encoded.
func TestAppendingToARecordedLogProducesTheRecordedBytes(t *testing.T) {
	_, lines := goldenLog(t)
	for have := 1; have < goldenEvents; have++ {
		have := have
		t.Run(fmt.Sprintf("after %d events", have), func(t *testing.T) {
			root := setStoreEnv(t)
			s, err := NewStore()
			if err != nil {
				t.Fatal(err)
			}
			writeRawLog(t, root, goldenProject, goldenWorkflow, strings.Join(lines[:have], ""))

			if err := s.Append(goldenProject, goldenWorkflow, goldenEvent(t, lines, have)); err != nil {
				t.Fatalf("Append() = %v, want nil", err)
			}
			want := strings.Join(lines[:have+1], "")
			if got := string(mustRead(t, logPath(root))); got != want {
				t.Errorf("log after the append is not the recorded one:\n got %q\nwant %q", got, want)
			}
		})
	}
}

// A whole workflow written by this version is the whole workflow the earlier one
// wrote: byte-identical at every step, in the same layout, with the same modes and
// nothing left behind.
func TestWritingAWorkflowFromScratchProducesTheRecordedBytesAndLayout(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("POSIX permission bits are not meaningful here")
	}
	data, lines := goldenLog(t)
	root := setStoreEnv(t)
	s, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	for k := range lines {
		if err := s.Append(goldenProject, goldenWorkflow, goldenEvent(t, lines, k)); err != nil {
			t.Fatalf("Append(event %d) = %v, want nil", k, err)
		}
		want := strings.Join(lines[:k+1], "")
		if got := string(mustRead(t, logPath(root))); got != want {
			t.Fatalf("log after %d appends is not the recorded one:\n got %q\nwant %q", k+1, got, want)
		}
	}
	if !bytes.Equal(mustRead(t, logPath(root)), data) {
		t.Error("the finished log is not the recorded log")
	}

	// The layout is part of the contract too: these paths and these modes, and no
	// temporary file left in the directory.
	got := map[string]fs.FileMode{}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		got[filepath.ToSlash(rel)] = info.Mode()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]fs.FileMode{
		"labdrian":                       fs.ModeDir | 0o700,
		"labdrian/workflows":             fs.ModeDir | 0o700,
		"labdrian/workflows/proj-golden": fs.ModeDir | 0o700,
		"labdrian/workflows/proj-golden/wf-golden.jsonl": 0o600,
		"labdrian/workflows/proj-golden/wf-golden.lock":  0o600,
	}
	if len(got) != len(want) {
		t.Errorf("state home holds %v, want exactly %v", sortedKeys(got), sortedKeys(want))
	}
	for path, mode := range want {
		if got[path] != mode {
			t.Errorf("%s has mode %v, want %v", path, got[path], mode)
		}
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func sortedKeys(m map[string]fs.FileMode) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
