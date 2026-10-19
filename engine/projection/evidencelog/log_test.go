package evidencelog_test

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection/evidencelog"
)

func record(callID string) projection.ReadRecord {
	return projection.ReadRecord{
		Evidence: projection.ReadEvidence{ProjectID: "proj-1", WorkflowID: "wf-1", Tool: "mcp__engram__mem_get_observation",
			Source: memoryscope.SourceEngram, IDs: []string{"4216"}},
		At:     time.Date(2026, 10, 19, 22, 40, 0, 0, time.FixedZone("x", 3600)),
		CallID: callID, SessionID: "s-1", ResponseSHA256: strings.Repeat("a", 64), ResponseBytes: 5120,
	}
}

func newLog(t *testing.T) (evidencelog.Log, string) {
	t.Helper()
	home := t.TempDir()
	log, err := evidencelog.New(home)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return log, filepath.Join(home, "labdrian", "workflows", "proj-1", "wf-1.memory.jsonl")
}

func lines(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []map[string]any
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("line %q: %v", sc.Text(), err)
		}
		out = append(out, m)
	}
	return out
}

func TestAppendWritesOneLinePerReadBesideTheWorkflowLog(t *testing.T) {
	log, path := newLog(t)
	for _, id := range []string{"toolu_1", "toolu_2"} {
		if err := log.Append(record(id)); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	got := lines(t, path)
	if len(got) != 2 {
		t.Fatalf("%d lines, want 2", len(got))
	}
	want := `{"version":1,"at":"2026-10-19T21:40:00Z","tool":"mcp__engram__mem_get_observation","source":"engram","project":"","goal":"","ids":["4216"],"skill":"","call_id":"toolu_1","session":"s-1","response_sha256":"` +
		strings.Repeat("a", 64) + `","response_bytes":5120}`
	data, _ := os.ReadFile(path)
	if first := strings.SplitN(string(data), "\n", 2)[0]; first != want {
		t.Errorf("line =\n%s\nwant\n%s", first, want)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, %v; want 0600", info.Mode().Perm(), err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "wf-1.jsonl")); !os.IsNotExist(err) {
		t.Errorf("the workflow log was touched: %v", err)
	}
}

func TestAppendWritesAnEmptyIDListAsAList(t *testing.T) {
	log, path := newLog(t)
	rec := record("toolu_1")
	rec.Evidence.IDs = nil
	if err := log.Append(rec); err != nil {
		t.Fatal(err)
	}
	if ids, ok := lines(t, path)[0]["ids"].([]any); !ok || len(ids) != 0 {
		t.Errorf("ids = %#v, want []", lines(t, path)[0]["ids"])
	}
}

func TestConcurrentAppendsKeepEveryLineWhole(t *testing.T) {
	log, path := newLog(t)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := log.Append(record("toolu")); err != nil {
				t.Errorf("Append: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := len(lines(t, path)); got != 16 {
		t.Errorf("%d lines, want 16", got)
	}
}

func TestAppendRefusesASymlinkAndAnUnsafeIdentifier(t *testing.T) {
	log, path := newLog(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if err := log.Append(record("toolu_1")); err == nil {
		t.Error("appended through a symlink")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("the symlink's target was written")
	}
	bad := record("toolu_1")
	bad.Evidence.WorkflowID = "../x"
	if err := log.Append(bad); err == nil {
		t.Error("an unsafe workflow id was accepted")
	}
}

func TestNewRefusesARelativeStateHome(t *testing.T) {
	if _, err := evidencelog.New("relative"); err == nil {
		t.Error("a relative state home was accepted")
	}
}
