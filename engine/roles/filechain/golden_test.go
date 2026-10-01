package filechain

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

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
)

// The on-disk chain is a contract with every earlier version of the program: a chain
// written by one must be read, extended and rewritten by the next, byte for byte, and
// the other way round. testdata/golden-v1 is a chain recorded from the version that
// kept this store in the roles package (commit a355c64): a throwaway program built on
// that version appended the five records below, one after the other, to a fresh state
// home, copied every record file, and listed the whole state home (each directory and
// file with its permission bits, and each file with its SHA-256 and size) in
// manifest.txt.
//
// The chain it records, proj-1/goal-1/chain-1, goes shaper -> estimator, then an
// interrupted estimator -> builder, its repeat completed, builder -> reviewer, and
// reviewer -> delivery, which closes it.
const goldenRecords = 5

// goldenRecord returns the bytes of the n-th recorded record, from 1.
func goldenRecord(t *testing.T, n int) []byte {
	t.Helper()
	return mustRead(t, filepath.Join("testdata", "golden-v1", recordFileName(n)))
}

// goldenManifest returns the listing the earlier version produced.
func goldenManifest(t *testing.T) string {
	t.Helper()
	return string(mustRead(t, filepath.Join("testdata", "golden-v1", "manifest.txt")))
}

// install lays down the first n recorded records as the earlier version left them:
// directories 0700, record files 0600.
func install(t *testing.T, root string, n int) {
	t.Helper()
	dir := chainDir(root)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for p := dir; p != root; p = filepath.Dir(p) {
		if err := os.Chmod(p, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= n; i++ {
		if err := os.WriteFile(filepath.Join(dir, recordFileName(i)), goldenRecord(t, i), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// snapshot lists a state home the way manifest.txt does: every directory with its
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
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			lines = append(lines, fmt.Sprintf("l %s", rel))
		case d.IsDir():
			lines = append(lines, fmt.Sprintf("d %04o %s", info.Mode().Perm(), rel))
		default:
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			lines = append(lines, fmt.Sprintf("f %04o %s %s %d", info.Mode().Perm(), rel, hex.EncodeToString(sum[:]), len(data)))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return lines
}

func requireListing(t *testing.T, root string) {
	t.Helper()
	if got := strings.Join(snapshot(t, root), "\n") + "\n"; got != goldenManifest(t) {
		t.Fatalf("state home is not the one the earlier version wrote:\n got:\n%swant:\n%s", got, goldenManifest(t))
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

func requirePOSIX(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("POSIX permission bits are not meaningful here")
	}
}

// The chain the earlier version wrote is read as it always was: whole, in order,
// each record with the exact bytes the next one chains to, and unchanged by the read.
func TestARecordedChainIsReadWholeAndLeftAlone(t *testing.T) {
	requirePOSIX(t)
	s, root := newTestStore(t)
	install(t, root, goldenRecords)

	records, err := s.LoadChain("proj-1", "goal-1", "chain-1")
	if err != nil {
		t.Fatalf("LoadChain() = %v, want nil", err)
	}
	if len(records) != goldenRecords {
		t.Fatalf("LoadChain() = %d records, want %d", len(records), goldenRecords)
	}
	for i, r := range records {
		if string(r.Raw) != string(goldenRecord(t, i+1)) || r.Handoff.Seq != i+1 {
			t.Errorf("record %d: seq %d, bytes differ from the recorded ones", i+1, r.Handoff.Seq)
		}
	}
	state, err := roles.Resume(records)
	if err != nil || state.Role != roles.RoleDelivery || !state.Terminal {
		t.Errorf("Resume() = %+v, %v, want the chain closed at delivery", state, err)
	}
	if records[1].Handoff.Status != roles.StatusInterrupted || records[2].Handoff.FromRole != records[1].Handoff.FromRole {
		t.Errorf("the interrupted record and its repeat were not read as recorded")
	}
	requireListing(t, root)
}

// An append extends what is there and nothing else: the recorded chain, at every
// length, plus the records that follow it, is the recorded state home, to the byte,
// with the same paths and modes. The bytes already stored are never rewritten.
func TestAppendingToARecordedChainProducesTheRecordedBytes(t *testing.T) {
	requirePOSIX(t)
	for have := 0; have < goldenRecords; have++ {
		t.Run(fmt.Sprintf("after %d records", have), func(t *testing.T) {
			s, root := newTestStore(t)
			if have > 0 {
				install(t, root, have)
			}
			for n := have + 1; n <= goldenRecords; n++ {
				path, err := s.Append(goldenRecord(t, n))
				if err != nil {
					t.Fatalf("Append(record %d) = %v, want nil", n, err)
				}
				if want := filepath.Join(chainDir(root), recordFileName(n)); path != want {
					t.Fatalf("Append(record %d) path = %q, want %q", n, path, want)
				}
			}
			requireListing(t, root)
		})
	}
}

// A whole chain written by this version is the whole chain the earlier one wrote:
// byte-identical at every step, in the same layout, with the same modes and nothing
// left behind. Storing a record again, at any point, changes nothing.
func TestWritingAChainFromScratchProducesTheRecordedBytesAndLayout(t *testing.T) {
	requirePOSIX(t)
	s, root := newTestStore(t)
	for n := 1; n <= goldenRecords; n++ {
		if _, err := s.Append(goldenRecord(t, n)); err != nil {
			t.Fatalf("Append(record %d) = %v, want nil", n, err)
		}
		for i := 1; i <= n; i++ {
			if got := mustRead(t, filepath.Join(chainDir(root), recordFileName(i))); string(got) != string(goldenRecord(t, i)) {
				t.Fatalf("record %d after %d appends is not the recorded one", i, n)
			}
		}
		if _, err := s.Append(goldenRecord(t, n)); err != nil {
			t.Fatalf("Append(record %d) again = %v, want nil: identical bytes are idempotent", n, err)
		}
	}
	requireListing(t, root)
}
