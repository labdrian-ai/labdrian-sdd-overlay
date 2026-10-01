package fsstore_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection/fsstore"
)

// The on-disk bindings are a contract with every earlier version of the program: a
// state written by one must be read, extended and rewritten by the next, byte for
// byte, and the other way round. testdata/golden-v1 is a sequence of states recorded
// from the version that kept this store in the projection package (commit 4320ab7):
// a throwaway program built on that version ran the steps below, one after the
// other, against a fresh state home, copied the binding after each step that wrote
// one, and listed the whole state home after each step (every directory with its
// permission bits, every file with its permission bits, SHA-256 and size) in
// manifest-N.txt.
//
// The steps are the ones a session goes through: bind a repository, bind the same
// workflow again (a no-op that keeps the first bound_at), replace the binding of a
// workflow it judged stale, bind a second repository, and unbind the first, which
// leaves its lock file behind by design.
var (
	goldenA  = hex64("a")
	goldenB  = hex64("b")
	goldenT1 = time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	goldenT2 = time.Date(2026, 9, 29, 11, 0, 0, 0, time.UTC)
	goldenT3 = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
)

// goldenSteps are the recorded steps; step n leaves the state of manifest-n.txt.
var goldenSteps = []func(t *testing.T, s fsstore.Store){
	func(t *testing.T, s fsstore.Store) {
		mustBind(t, s, goldenA, "proj-1", "wf-1", goldenT1)
		// Binding the same workflow again changes nothing, whatever the time.
		mustBind(t, s, goldenA, "proj-1", "wf-1", goldenT2)
	},
	func(t *testing.T, s fsstore.Store) {
		read := mustLoad(t, s, goldenA)
		if err := s.BindIfUnchanged(goldenA, "proj-1", "wf-2", goldenT2, read.Binding); err != nil {
			t.Fatalf("BindIfUnchanged() = %v, want nil", err)
		}
	},
	func(t *testing.T, s fsstore.Store) { mustBind(t, s, goldenB, "proj-2", "wf-9", goldenT3) },
	func(t *testing.T, s fsstore.Store) {
		if removed, err := s.Unbind(goldenA); err != nil || !removed {
			t.Fatalf("Unbind() = %v, %v, want true, nil", removed, err)
		}
	},
}

func requirePOSIX(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("POSIX permission bits are not meaningful here")
	}
}

func goldenBytes(t *testing.T, name string) []byte {
	t.Helper()
	return readBytes(t, filepath.Join("testdata", "golden-v1", name))
}

// requireState checks that the state home is the one recorded after step n.
func requireState(t *testing.T, root string, n int) {
	t.Helper()
	want := string(goldenBytes(t, fmt.Sprintf("manifest-%d.txt", n)))
	if got := strings.Join(snapshot(t, root), "\n") + "\n"; got != want {
		t.Fatalf("state home after step %d is not the one the earlier version wrote:\n got:\n%swant:\n%s", n, got, want)
	}
}

// snapshot lists a state home the way manifest-N.txt does: every directory with its
// permission bits, every file with its permission bits, SHA-256 and size, sorted.
func snapshot(t *testing.T, root string) []string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
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
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			lines = append(lines, fmt.Sprintf("d %04o %s", info.Mode().Perm(), rel))
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		lines = append(lines, fmt.Sprintf("f %04o %s %s %d", info.Mode().Perm(), rel, hex.EncodeToString(sum[:]), len(data)))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return lines
}

// install lays down the state recorded after step n as the earlier version left
// it: directories 0700, files 0600, each binding with the recorded bytes and each
// lock file empty.
func install(t *testing.T, root string, n int) {
	t.Helper()
	known := map[string][]byte{}
	for _, name := range []string{"a-1.json", "a-2.json", "b-1.json"} {
		data := goldenBytes(t, name)
		sum := sha256.Sum256(data)
		known[hex.EncodeToString(sum[:])] = data
	}
	for _, line := range strings.Split(strings.TrimSpace(string(goldenBytes(t, fmt.Sprintf("manifest-%d.txt", n)))), "\n") {
		fields := strings.Fields(line)
		path := filepath.Join(root, filepath.FromSlash(fields[2]))
		if fields[0] == "d" {
			if err := os.MkdirAll(path, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, 0o700); err != nil {
				t.Fatal(err)
			}
			continue
		}
		var data []byte // a lock file is empty
		if strings.HasSuffix(path, ".json") {
			var found bool
			if data, found = known[fields[3]]; !found {
				t.Fatalf("manifest-%d.txt lists %s with a digest no recorded binding has", n, fields[2])
			}
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// A whole sequence written by this version is the sequence the earlier one wrote:
// the same paths, the same modes and the same bytes after every step, and nothing
// left behind.
func TestWritingTheRecordedSequenceFromScratchProducesTheRecordedStates(t *testing.T) {
	requirePOSIX(t)
	s, root := isolatedStore(t)
	for i, step := range goldenSteps {
		step(t, s)
		requireState(t, root, i+1)
	}
}

// The state the earlier version left is read as it always was, and extended without
// being rewritten: from the state after every step, the steps that follow produce
// the recorded states to the byte.
func TestExtendingARecordedStateProducesTheRecordedStates(t *testing.T) {
	requirePOSIX(t)
	for have := 1; have < len(goldenSteps); have++ {
		t.Run(fmt.Sprintf("after step %d", have), func(t *testing.T) {
			s, root := isolatedStore(t)
			install(t, root, have)
			requireState(t, root, have)
			for i := have; i < len(goldenSteps); i++ {
				goldenSteps[i](t, s)
				requireState(t, root, i+1)
			}
		})
	}
}

func TestARecordedBindingIsReadAsOwned(t *testing.T) {
	requirePOSIX(t)
	s, root := isolatedStore(t)
	install(t, root, 3)

	a := mustLoad(t, s, goldenA)
	wantA := projection.Binding{Version: 1, RepoKey: goldenA, ProjectID: "proj-1", WorkflowID: "wf-2", BoundAt: "2026-09-29T11:00:00Z"}
	if a.Classification != projection.ClassificationOwned || a.Binding != wantA || a.Detail != "" {
		t.Errorf("Load(a) = %+v, want owned with %+v", a, wantA)
	}
	b := mustLoad(t, s, goldenB)
	wantB := projection.Binding{Version: 1, RepoKey: goldenB, ProjectID: "proj-2", WorkflowID: "wf-9", BoundAt: "2026-09-29T12:00:00Z"}
	if b.Classification != projection.ClassificationOwned || b.Binding != wantB || b.Detail != "" {
		t.Errorf("Load(b) = %+v, want owned with %+v", b, wantB)
	}
	requireState(t, root, 3)
}
