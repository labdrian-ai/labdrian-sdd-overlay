package filechain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
)

const (
	validDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	otherDigest = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// recordJSON is one RoleHandoff v1 record of proj-1/goal-1/chain-1, in the layout
// the golden chain was recorded in.
func recordJSON(seq int, prevSHA, from, to, status, resumeReason string) string {
	statusField := `"status": "` + status + `"`
	if resumeReason != "" {
		statusField += `, "resume_reason": "` + resumeReason + `"`
	}
	return `{
  "version": 1,
  "project_id": "proj-1",
  "goal_id": "goal-1",
  "chain_id": "chain-1",
  "seq": ` + strconv.Itoa(seq) + `,
  "from_role": "` + from + `",
  "to_role": "` + to + `",
  "prev_sha256": "` + prevSHA + `",
  "payload_kind": "diff",
  "payload_sha256": "` + validDigest + `",
  "evidence": [],
  "context": {"summary": "s", "decisions": [], "open_questions": []},
  ` + statusField + `
}`
}

// newTestStore builds the store over a fresh temporary directory, so no test can
// reach the real user's state, and returns the store and the state home.
func newTestStore(t *testing.T) (Store, string) {
	t.Helper()
	root := t.TempDir()
	s, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore() = %v, want nil", err)
	}
	return s, root
}

// chainDir is where proj-1/goal-1/chain-1 lives under a state home.
func chainDir(root string) string {
	return filepath.Join(root, "labdrian", "role-chains", "proj-1", "goal-1", "chain-1")
}

func TestChainStoreAppendFirstRecord(t *testing.T) {
	s, _ := newTestStore(t)
	data := recordJSON(1, roles.EmptyChainDigest, "shaper", "estimator", "completed", "")
	path, err := s.Append([]byte(data))
	if err != nil {
		t.Fatalf("Append() = %v, want nil", err)
	}
	if filepath.Base(path) != "000001.json" {
		t.Fatalf("Append() path = %q, want basename 000001.json", path)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) = %v", path, err)
	}
	if string(got) != data {
		t.Fatalf("stored bytes = %q, want %q", got, data)
	}
}

func TestChainStoreRejectsFirstRecordWithWrongFromRole(t *testing.T) {
	s, _ := newTestStore(t)
	data := recordJSON(1, roles.EmptyChainDigest, "builder", "sweeper", "completed", "")
	if _, err := s.Append([]byte(data)); err == nil {
		t.Fatalf("Append() = nil, want error for a first record whose from_role is not prototyper or shaper")
	}
}

func TestChainStoreAppendsChainedRecordsAndLoads(t *testing.T) {
	s, _ := newTestStore(t)
	r1 := recordJSON(1, roles.EmptyChainDigest, "shaper", "estimator", "completed", "")
	if _, err := s.Append([]byte(r1)); err != nil {
		t.Fatalf("Append(seq1) = %v, want nil", err)
	}
	r2 := recordJSON(2, sha256Hex([]byte(r1)), "estimator", "builder", "completed", "")
	if _, err := s.Append([]byte(r2)); err != nil {
		t.Fatalf("Append(seq2) = %v, want nil", err)
	}

	records, err := s.LoadChain("proj-1", "goal-1", "chain-1")
	if err != nil {
		t.Fatalf("LoadChain() = %v, want nil", err)
	}
	if len(records) != 2 {
		t.Fatalf("LoadChain() returned %d records, want 2", len(records))
	}
	if records[0].Handoff.Seq != 1 || records[1].Handoff.Seq != 2 {
		t.Fatalf("LoadChain() seqs = %d, %d, want 1, 2", records[0].Handoff.Seq, records[1].Handoff.Seq)
	}
	if string(records[0].Raw) != r1 || string(records[1].Raw) != r2 {
		t.Fatalf("LoadChain() raw bytes differ from what was appended: the next record chains to them")
	}
}

func TestChainStoreLoadChainOnMissingChainReturnsEmpty(t *testing.T) {
	s, _ := newTestStore(t)
	records, err := s.LoadChain("proj-none", "goal-none", "chain-none")
	if err != nil {
		t.Fatalf("LoadChain() = %v, want nil for a chain that does not exist yet", err)
	}
	if len(records) != 0 {
		t.Fatalf("LoadChain() = %d records, want 0", len(records))
	}
}

func TestChainStoreAppendRejectsWrongPrevSHA(t *testing.T) {
	s, _ := newTestStore(t)
	r1 := recordJSON(1, roles.EmptyChainDigest, "shaper", "estimator", "completed", "")
	if _, err := s.Append([]byte(r1)); err != nil {
		t.Fatalf("Append(seq1) = %v, want nil", err)
	}
	r2 := recordJSON(2, otherDigest, "estimator", "builder", "completed", "")
	if _, err := s.Append([]byte(r2)); err == nil {
		t.Fatalf("Append(seq2) = nil, want error for a wrong prev_sha256")
	}
}

func TestChainStoreAppendRejectsSeqGap(t *testing.T) {
	s, _ := newTestStore(t)
	r1 := recordJSON(1, roles.EmptyChainDigest, "shaper", "estimator", "completed", "")
	if _, err := s.Append([]byte(r1)); err != nil {
		t.Fatalf("Append(seq1) = %v, want nil", err)
	}
	r3 := recordJSON(3, sha256Hex([]byte(r1)), "estimator", "builder", "completed", "")
	_, err := s.Append([]byte(r3))
	want := "role chain store: append: seq 3 is neither the next seq (2) nor an existing one (chain has 1 records)"
	if err == nil || err.Error() != want {
		t.Fatalf("Append(seq3) = %v, want %q", err, want)
	}
}

func TestChainStoreAppendIdenticalBytesIsIdempotent(t *testing.T) {
	s, _ := newTestStore(t)
	r1 := recordJSON(1, roles.EmptyChainDigest, "shaper", "estimator", "completed", "")
	path1, err := s.Append([]byte(r1))
	if err != nil {
		t.Fatalf("Append() first = %v, want nil", err)
	}
	path2, err := s.Append([]byte(r1))
	if err != nil {
		t.Fatalf("Append() re-append identical bytes = %v, want nil (idempotent)", err)
	}
	if path1 != path2 {
		t.Fatalf("Append() paths differ: %q vs %q", path1, path2)
	}
}

func TestChainStoreAppendDifferentBytesAtSameKeyRefused(t *testing.T) {
	s, root := newTestStore(t)
	r1 := recordJSON(1, roles.EmptyChainDigest, "shaper", "estimator", "completed", "")
	if _, err := s.Append([]byte(r1)); err != nil {
		t.Fatalf("Append() first = %v, want nil", err)
	}
	r1different := recordJSON(1, roles.EmptyChainDigest, "prototyper", "shaper", "completed", "")
	_, err := s.Append([]byte(r1different))
	want := fmt.Sprintf("role chain store: refusing to replace immutable record %q with different bytes", filepath.Join(chainDir(root), "000001.json"))
	if err == nil || err.Error() != want {
		t.Fatalf("Append() = %v, want %q", err, want)
	}
	if got, readErr := os.ReadFile(filepath.Join(chainDir(root), "000001.json")); readErr != nil || string(got) != r1 {
		t.Fatalf("stored record = %q, %v, want it left as it was", got, readErr)
	}
}

func TestChainStoreAppendRejectsAfterTerminalDelivery(t *testing.T) {
	s, _ := newTestStore(t)
	r1 := recordJSON(1, roles.EmptyChainDigest, "shaper", "estimator", "completed", "")
	if _, err := s.Append([]byte(r1)); err != nil {
		t.Fatalf("Append(seq1) = %v, want nil", err)
	}
	r2 := recordJSON(2, sha256Hex([]byte(r1)), "estimator", "builder", "completed", "")
	if _, err := s.Append([]byte(r2)); err != nil {
		t.Fatalf("Append(seq2) = %v, want nil", err)
	}
	r3 := recordJSON(3, sha256Hex([]byte(r2)), "builder", "reviewer", "completed", "")
	if _, err := s.Append([]byte(r3)); err != nil {
		t.Fatalf("Append(seq3) = %v, want nil", err)
	}
	r4 := recordJSON(4, sha256Hex([]byte(r3)), "reviewer", "delivery", "completed", "")
	if _, err := s.Append([]byte(r4)); err != nil {
		t.Fatalf("Append(seq4) = %v, want nil", err)
	}
	r5 := recordJSON(5, sha256Hex([]byte(r4)), "reviewer", "builder", "completed", "")
	if _, err := s.Append([]byte(r5)); err == nil {
		t.Fatalf("Append(seq5) = nil, want error: chain is terminal at delivery")
	}
}

func TestChainStoreRefusesSymlinkedChainDirectory(t *testing.T) {
	s, root := newTestStore(t)
	dir := chainDir(root)
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		t.Fatalf("MkdirAll(%q) = %v", filepath.Dir(dir), err)
	}
	realDir := dir + "-real"
	if err := os.MkdirAll(realDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(%q) = %v", realDir, err)
	}
	if err := os.Symlink(realDir, dir); err != nil {
		t.Fatalf("Symlink() = %v", err)
	}
	r1 := recordJSON(1, roles.EmptyChainDigest, "shaper", "estimator", "completed", "")
	_, err := s.Append([]byte(r1))
	want := fmt.Sprintf("role chain store: append: role chain store: refusing symlinked store component %q", dir)
	if err == nil || err.Error() != want {
		t.Fatalf("Append() = %v, want %q", err, want)
	}
	if entries, _ := os.ReadDir(realDir); len(entries) != 0 {
		t.Fatalf("the symlink target holds %d entries, want none: nothing may be written through the link", len(entries))
	}
}

func TestChainStoreRejectsUnsafePathComponent(t *testing.T) {
	s, _ := newTestStore(t)
	tests := []struct {
		name, project, goal, chain, want string
	}{
		{"parent directory as project", "../escape", "goal-1", "chain-1", `role chain store: project_id "../escape" contains a path separator or NUL`},
		{"backslash in goal", "proj-1", `a\b`, "chain-1", `role chain store: goal_id "a\\b" contains a path separator or NUL`},
		{"NUL in chain", "proj-1", "goal-1", "a\x00b", `role chain store: chain_id "a\x00b" contains a path separator or NUL`},
		{"empty project", "", "goal-1", "chain-1", `role chain store: project_id "" is not a usable path component`},
		{"dot goal", "proj-1", ".", "chain-1", `role chain store: goal_id "." is not a usable path component`},
		{"dot-dot chain", "proj-1", "goal-1", "..", `role chain store: chain_id ".." is not a usable path component`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.LoadChain(tt.project, tt.goal, tt.chain); err == nil || err.Error() != tt.want {
				t.Fatalf("LoadChain() = %v, want %q", err, tt.want)
			}
		})
	}
}

// A zero Store was not built by NewStore and has no state home to write under.
func TestZeroStoreIsRefused(t *testing.T) {
	_, err := Store{}.LoadChain("proj-1", "goal-1", "chain-1")
	want := "role chain store: store is not initialized; use NewStore"
	if err == nil || err.Error() != want {
		t.Fatalf("LoadChain() = %v, want %q", err, want)
	}
}

// The store is built over the state home it is handed, which must be an absolute
// path. Resolving it from the environment, and the text that prints for an unusable
// environment, are the composition root's (cmd pins them).
func TestNewStoreRefusesAStateHomeThatIsNotAbsolute(t *testing.T) {
	for _, home := range []string{"", "relative/path", "./state"} {
		_, err := NewStore(home)
		want := fmt.Sprintf("role chain store: state home %q is not an absolute path", home)
		if err == nil || err.Error() != want {
			t.Errorf("NewStore(%q) = %v, want %q", home, err, want)
		}
	}
}

// A platform without a no-follow open is refused when the store is built, with a
// sentinel callers can match and the message every store of this kind prints.
func TestTheUnsupportedPlatformSentinelKeepsItsMessage(t *testing.T) {
	if got, want := ErrUnsupportedPlatform.Error(), "role chain store: unsupported platform"; got != want {
		t.Errorf("ErrUnsupportedPlatform = %q, want %q", got, want)
	}
}

// A state home that is itself a symlink is the user's own choice and is followed;
// only the components below it must be plain directories.
func TestChainStoreFollowsASymlinkedStateHome(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "state")
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("Symlink() = %v", err)
	}
	s, err := NewStore(link)
	if err != nil {
		t.Fatalf("NewStore() = %v, want nil", err)
	}
	r1 := recordJSON(1, roles.EmptyChainDigest, "shaper", "estimator", "completed", "")
	if _, err := s.Append([]byte(r1)); err != nil {
		t.Fatalf("Append() = %v, want nil", err)
	}
	if _, err := os.Stat(filepath.Join(chainDir(real), "000001.json")); err != nil {
		t.Fatalf("the record is not under the link's target: %v", err)
	}
}

// Every way a chain on disk can be unusable is reported in the words the store has
// always used, and never repaired or rewritten.
func TestChainStoreReportsAnUnusableChainOnDisk(t *testing.T) {
	r1 := recordJSON(1, roles.EmptyChainDigest, "shaper", "estimator", "completed", "")
	r2 := recordJSON(2, sha256Hex([]byte(r1)), "estimator", "builder", "completed", "")

	tests := []struct {
		name  string
		setup func(t *testing.T, root string)
		do    func(s Store) error
		want  func(root string) string
	}{
		{
			name: "a symlinked record",
			setup: func(t *testing.T, root string) {
				mustWrite(t, filepath.Join(chainDir(root), "real.txt"), r1)
				mustSymlink(t, filepath.Join(chainDir(root), "real.txt"), filepath.Join(chainDir(root), "000001.json"))
			},
			do: func(s Store) error { _, err := s.LoadChain("proj-1", "goal-1", "chain-1"); return err },
			want: func(root string) string {
				return fmt.Sprintf("role chain store: refusing symlinked record %q", filepath.Join(chainDir(root), "000001.json"))
			},
		},
		{
			name: "a record whose name is not its seq",
			setup: func(t *testing.T, root string) {
				mustWrite(t, filepath.Join(chainDir(root), "000001.json"), r2)
			},
			do: func(s Store) error { _, err := s.LoadChain("proj-1", "goal-1", "chain-1"); return err },
			want: func(root string) string {
				return `role chain store: record "000001.json" declares seq 2, want filename "000002.json"`
			},
		},
		{
			name: "a record that is not a role handoff",
			setup: func(t *testing.T, root string) {
				mustWrite(t, filepath.Join(chainDir(root), "000001.json"), "{}")
			},
			do:   func(s Store) error { _, err := s.LoadChain("proj-1", "goal-1", "chain-1"); return err },
			want: func(root string) string { return `role chain store: record "000001.json": parse role handoff: ` },
		},
		{
			name: "a chain that does not verify",
			setup: func(t *testing.T, root string) {
				mustWrite(t, filepath.Join(chainDir(root), "000001.json"), r1)
				mustWrite(t, filepath.Join(chainDir(root), "000002.json"), recordJSON(2, otherDigest, "estimator", "builder", "completed", ""))
			},
			do:   func(s Store) error { _, err := s.LoadChain("proj-1", "goal-1", "chain-1"); return err },
			want: func(root string) string { return `role chain store: role chain: seq 2 prev_sha256 ` },
		},
		{
			name: "a store component that is a file",
			setup: func(t *testing.T, root string) {
				component := filepath.Join(root, "labdrian", "role-chains", "proj-1")
				if err := os.RemoveAll(component); err != nil {
					t.Fatalf("RemoveAll() = %v", err)
				}
				mustWrite(t, component, "not a directory")
			},
			do: func(s Store) error { _, err := s.LoadChain("proj-1", "goal-1", "chain-1"); return err },
			want: func(root string) string {
				return fmt.Sprintf("role chain store: store component %q is not a directory", filepath.Join(root, "labdrian", "role-chains", "proj-1"))
			},
		},
		{
			name: "a record slot taken by a directory",
			setup: func(t *testing.T, root string) {
				mustWrite(t, filepath.Join(chainDir(root), "000001.json"), r1)
				if err := os.Mkdir(filepath.Join(chainDir(root), "000002.json"), 0o700); err != nil {
					t.Fatalf("Mkdir() = %v", err)
				}
			},
			do: func(s Store) error { _, err := s.Append([]byte(r2)); return err },
			want: func(root string) string {
				return fmt.Sprintf("role chain store: record %q is not a regular file", filepath.Join(chainDir(root), "000002.json"))
			},
		},
		{
			name: "a record slot taken by a symlink",
			setup: func(t *testing.T, root string) {
				mustWrite(t, filepath.Join(chainDir(root), "000001.json"), r1)
				mustSymlink(t, filepath.Join(chainDir(root), "000001.json"), filepath.Join(chainDir(root), "000002.json"))
			},
			do: func(s Store) error { _, err := s.Append([]byte(r2)); return err },
			want: func(root string) string {
				return fmt.Sprintf("role chain store: append: role chain store: refusing symlinked record %q", filepath.Join(chainDir(root), "000002.json"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, root := newTestStore(t)
			if err := os.MkdirAll(chainDir(root), 0o700); err != nil {
				t.Fatalf("MkdirAll() = %v", err)
			}
			tt.setup(t, root)
			before := snapshot(t, root)
			err := tt.do(s)
			want := tt.want(root)
			if err == nil || !strings.HasPrefix(err.Error(), want) {
				t.Fatalf("error = %v, want it to start %q", err, want)
			}
			if after := snapshot(t, root); strings.Join(after, "\n") != strings.Join(before, "\n") {
				t.Fatalf("the refusal changed the store:\nbefore:\n%s\nafter:\n%s", strings.Join(before, "\n"), strings.Join(after, "\n"))
			}
		})
	}
}

// An append leaves nothing but its record: no temporary file survives, whether the
// record was stored, stored again, or refused.
func TestChainStoreLeavesNoTemporaryFile(t *testing.T) {
	s, root := newTestStore(t)
	r1 := recordJSON(1, roles.EmptyChainDigest, "shaper", "estimator", "completed", "")
	other := recordJSON(1, roles.EmptyChainDigest, "prototyper", "shaper", "completed", "")
	for _, data := range []string{r1, r1, other} {
		_, _ = s.Append([]byte(data))
	}
	entries, err := os.ReadDir(chainDir(root))
	if err != nil {
		t.Fatalf("ReadDir() = %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "000001.json" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("chain directory holds %v, want only 000001.json", names)
	}
}

// Writers racing for one name cannot lose each other's record: with identical bytes
// they all succeed and the record is whole; with different bytes exactly one wins,
// every other is refused, and what is stored is the winner's, complete.
func TestChainStoreRacingWritersNeverLoseARecord(t *testing.T) {
	first := recordJSON(1, roles.EmptyChainDigest, "shaper", "estimator", "completed", "")
	second := recordJSON(1, roles.EmptyChainDigest, "prototyper", "shaper", "completed", "")

	t.Run("identical bytes", func(t *testing.T) {
		s, root := newTestStore(t)
		errs := race(8, func(int) error { _, err := s.Append([]byte(first)); return err })
		for i, err := range errs {
			if err != nil {
				t.Errorf("writer %d: %v, want nil: identical bytes are idempotent", i, err)
			}
		}
		if got, _ := os.ReadFile(filepath.Join(chainDir(root), "000001.json")); string(got) != first {
			t.Errorf("stored record = %q, want %q", got, first)
		}
	})

	t.Run("different bytes", func(t *testing.T) {
		s, root := newTestStore(t)
		data := []string{first, second}
		errs := race(8, func(i int) error { _, err := s.Append([]byte(data[i%2])); return err })
		stored, _ := os.ReadFile(filepath.Join(chainDir(root), "000001.json"))
		if string(stored) != first && string(stored) != second {
			t.Fatalf("stored record = %q, want exactly one of the two offered", stored)
		}
		for i, err := range errs {
			offered := data[i%2]
			switch {
			case offered == string(stored) && err != nil:
				t.Errorf("writer %d offered the stored record and got %v, want nil", i, err)
			case offered != string(stored) && (err == nil || !strings.Contains(err.Error(), "refusing to replace immutable record")):
				t.Errorf("writer %d offered a different record and got %v, want a refusal to replace", i, err)
			}
		}
	})
}

func race(n int, fn func(i int) error) []error {
	errs := make([]error, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = fn(i)
		}(i)
	}
	close(start)
	wg.Wait()
	return errs
}

// paddedRecord is the first record of proj-1/goal-1/chain-1 grown to exactly size
// bytes with blanks before the closing brace: still the same one valid document.
func paddedRecord(size int) string {
	record := recordJSON(1, roles.EmptyChainDigest, "shaper", "estimator", "completed", "")
	return strings.Replace(record, "\n}", strings.Repeat(" ", size-len(record))+"\n}", 1)
}

// A record has no length of its own, so the store documents and keeps a bound on the
// whole file (roles.MaxRecordBytes) and judges the size, not the content, beyond it.
// The bound is exact: a record of that size is stored and read back like any other.
func TestARecordAtTheBoundIsStoredAndReadBack(t *testing.T) {
	s, _ := newTestStore(t)
	record := paddedRecord(roles.MaxRecordBytes)
	if _, err := s.Append([]byte(record)); err != nil {
		t.Fatalf("Append(a record of MaxRecordBytes) = %v, want nil", err)
	}
	chain, err := s.LoadChain("proj-1", "goal-1", "chain-1")
	if err != nil || len(chain) != 1 || string(chain[0].Raw) != record {
		t.Fatalf("LoadChain() = %d records, %v, want the one record read back whole", len(chain), err)
	}
}

// A record over the bound is refused before anything is written.
func TestAppendRefusesAnOversizedRecordAndStoresNothing(t *testing.T) {
	s, root := newTestStore(t)
	_, err := s.Append([]byte(paddedRecord(roles.MaxRecordBytes + 1)))
	want := fmt.Sprintf("role chain store: append: parse role handoff: roles: handoff record is too large: %d bytes, the maximum is %d", roles.MaxRecordBytes+1, roles.MaxRecordBytes)
	if err == nil || err.Error() != want {
		t.Fatalf("Append() = %v, want %q", err, want)
	}
	if _, statErr := os.Stat(filepath.Join(root, "labdrian")); !os.IsNotExist(statErr) {
		t.Errorf("the refused record created %s (%v), want nothing written", root, statErr)
	}
}

// A record file over the bound is never read in full: the bound is on what the store
// reads, so a huge file costs the bound and not its size, and the refusal names the
// size the file reported.
func TestLoadChainRefusesAnOversizedRecordWithoutReadingItAll(t *testing.T) {
	tests := []struct {
		name string
		size int64
	}{
		{"one byte over", roles.MaxRecordBytes + 1},
		{"far over", 256 << 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, root := newTestStore(t)
			path := filepath.Join(chainDir(root), "000001.json")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
			if err != nil {
				t.Fatal(err)
			}
			if err := f.Truncate(tt.size); err != nil { // sparse: it occupies almost no disk
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}

			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			_, err = s.LoadChain("proj-1", "goal-1", "chain-1")
			runtime.ReadMemStats(&after)

			want := fmt.Sprintf("role chain store: record %q is %d bytes, exceeding the maximum of %d", path, tt.size, roles.MaxRecordBytes)
			if err == nil || err.Error() != want {
				t.Fatalf("LoadChain() = %v, want %q", err, want)
			}
			if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 8*roles.MaxRecordBytes {
				t.Errorf("LoadChain() of a %d byte record allocated %d bytes, want at most %d: the record must not be read past the bound", tt.size, allocated, 8*roles.MaxRecordBytes)
			}
		})
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll(%q) = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) = %v", path, err)
	}
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink(%q, %q) = %v", target, link, err)
	}
}
