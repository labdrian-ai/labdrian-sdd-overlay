package fsadapter

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

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/shaper"
)

// The on-disk clearance store is a contract with every earlier version of the program: a
// state written by one must be read, extended and rewritten by the next, byte for byte,
// and the other way round. testdata/golden-v1 is a sequence of states recorded from the
// version that kept this store in the shaper package (commit eea7845): a throwaway test
// built on that version stored the four records below, one after the other, in a fresh
// state home (each stored twice, the second time a no-op), copied the record bytes it was
// given, and listed the whole state home after each (every directory with its permission
// bits, every file with its permission bits, SHA-256 and size) in manifest-N.txt.
//
// The records are two handoffs of one goal (an affirmation with a resolved flag and a
// host time, then a decline), a second goal of the same project, and a second project.
const goldenRecords = 4

func goldenRecord(t *testing.T, n int) []byte {
	t.Helper()
	return mustRead(t, filepath.Join("testdata", "golden-v1", fmt.Sprintf("record-%d.json", n)))
}

// goldenKey is the key record n names, read from the record itself.
func goldenKey(t *testing.T, n int) [3]string {
	t.Helper()
	r, err := shaper.ParseRecord(goldenRecord(t, n))
	if err != nil {
		t.Fatalf("record %d does not parse: %v", n, err)
	}
	return [3]string{r.Subject.ProjectID, r.Subject.GoalID, r.Subject.HandoffSHA256}
}

// install lays down the first n recorded records as the earlier version left them:
// directories 0700, record files 0600.
func install(t *testing.T, root string, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		path := recordPath(root, goldenKey(t, i))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		for p := filepath.Dir(path); p != root; p = filepath.Dir(p) {
			if err := os.Chmod(p, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(path, goldenRecord(t, i), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// requireState checks that the state home is the one recorded after record n (none for 0).
func requireState(t *testing.T, root string, n int) {
	t.Helper()
	want := ""
	if n > 0 {
		want = string(mustRead(t, filepath.Join("testdata", "golden-v1", fmt.Sprintf("manifest-%d.txt", n))))
	}
	if got := strings.Join(snapshot(t, root), "\n"); got != strings.TrimSuffix(want, "\n") {
		t.Fatalf("state home after record %d is not the one the earlier version wrote:\n got:\n%s\nwant:\n%s", n, got, want)
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

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func requirePOSIX(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("POSIX permission bits are not meaningful here")
	}
}

// The state the earlier version wrote is read as it always was: every record with the exact
// bytes it was stored with, at the path it was stored at, and unchanged by the read.
func TestARecordedStateIsReadWholeAndLeftAlone(t *testing.T) {
	requirePOSIX(t)
	s, root := newTestStore(t)
	install(t, root, goldenRecords)
	for n := 1; n <= goldenRecords; n++ {
		key := goldenKey(t, n)
		got, err := s.Get(key[0], key[1], key[2])
		if err != nil || string(got) != string(goldenRecord(t, n)) {
			t.Errorf("Get(record %d) = %d bytes, %v, want the recorded bytes", n, len(got), err)
		}
		if path, err := s.Path(key[0], key[1], key[2]); err != nil || path != recordPath(root, key) {
			t.Errorf("Path(record %d) = %q, %v, want %q", n, path, err, recordPath(root, key))
		}
	}
	requireState(t, root, goldenRecords)
}

// A store extends what is there and nothing else: the recorded state at every length, plus
// the records that follow it, is the recorded state home, to the byte, with the same paths
// and modes. The bytes already stored are never rewritten.
func TestStoringOntoARecordedStateProducesTheRecordedBytes(t *testing.T) {
	requirePOSIX(t)
	for have := 0; have < goldenRecords; have++ {
		t.Run(fmt.Sprintf("after %d records", have), func(t *testing.T) {
			s, root := newTestStore(t)
			install(t, root, have)
			requireState(t, root, have)
			for n := have + 1; n <= goldenRecords; n++ {
				path, err := s.Put(goldenRecord(t, n))
				if err != nil {
					t.Fatalf("Put(record %d) = %v, want nil", n, err)
				}
				if want := recordPath(root, goldenKey(t, n)); path != want {
					t.Fatalf("Put(record %d) path = %q, want %q", n, path, want)
				}
				requireState(t, root, n)
			}
		})
	}
}

// A whole state written by this version is the whole state the earlier one wrote:
// byte-identical at every step, in the same layout, with the same modes and nothing left
// behind. Storing a record again, at any point, changes nothing.
func TestWritingFromScratchProducesTheRecordedBytesAndLayout(t *testing.T) {
	requirePOSIX(t)
	s, root := newTestStore(t)
	for n := 1; n <= goldenRecords; n++ {
		if _, err := s.Put(goldenRecord(t, n)); err != nil {
			t.Fatalf("Put(record %d) = %v, want nil", n, err)
		}
		requireState(t, root, n)
		if _, err := s.Put(goldenRecord(t, n)); err != nil {
			t.Fatalf("Put(record %d) again = %v, want nil: identical bytes are idempotent", n, err)
		}
		requireState(t, root, n)
	}
}
