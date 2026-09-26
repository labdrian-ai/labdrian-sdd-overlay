package roles

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestChainStore(t *testing.T) ChainStore {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s, err := NewChainStore()
	if err != nil {
		t.Fatalf("NewChainStore() = %v, want nil", err)
	}
	return s
}

func TestChainStoreAppendFirstRecord(t *testing.T) {
	s := newTestChainStore(t)
	data := recordJSON(1, EmptyChainDigest, "shaper", "estimator", "completed", "")
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
	s := newTestChainStore(t)
	data := recordJSON(1, EmptyChainDigest, "builder", "sweeper", "completed", "")
	if _, err := s.Append([]byte(data)); err == nil {
		t.Fatalf("Append() = nil, want error for a first record whose from_role is not prototyper or shaper")
	}
}

func TestChainStoreAppendsChainedRecordsAndLoads(t *testing.T) {
	s := newTestChainStore(t)
	r1 := recordJSON(1, EmptyChainDigest, "shaper", "estimator", "completed", "")
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
}

func TestChainStoreLoadChainOnMissingChainReturnsEmpty(t *testing.T) {
	s := newTestChainStore(t)
	records, err := s.LoadChain("proj-none", "goal-none", "chain-none")
	if err != nil {
		t.Fatalf("LoadChain() = %v, want nil for a chain that does not exist yet", err)
	}
	if len(records) != 0 {
		t.Fatalf("LoadChain() = %d records, want 0", len(records))
	}
}

func TestChainStoreAppendRejectsWrongPrevSHA(t *testing.T) {
	s := newTestChainStore(t)
	r1 := recordJSON(1, EmptyChainDigest, "shaper", "estimator", "completed", "")
	if _, err := s.Append([]byte(r1)); err != nil {
		t.Fatalf("Append(seq1) = %v, want nil", err)
	}
	r2 := recordJSON(2, otherDigest, "estimator", "builder", "completed", "")
	if _, err := s.Append([]byte(r2)); err == nil {
		t.Fatalf("Append(seq2) = nil, want error for a wrong prev_sha256")
	}
}

func TestChainStoreAppendRejectsSeqGap(t *testing.T) {
	s := newTestChainStore(t)
	r1 := recordJSON(1, EmptyChainDigest, "shaper", "estimator", "completed", "")
	if _, err := s.Append([]byte(r1)); err != nil {
		t.Fatalf("Append(seq1) = %v, want nil", err)
	}
	r3 := recordJSON(3, sha256Hex([]byte(r1)), "estimator", "builder", "completed", "")
	if _, err := s.Append([]byte(r3)); err == nil {
		t.Fatalf("Append(seq3) = nil, want error for a seq gap")
	}
}

func TestChainStoreAppendIdenticalBytesIsIdempotent(t *testing.T) {
	s := newTestChainStore(t)
	r1 := recordJSON(1, EmptyChainDigest, "shaper", "estimator", "completed", "")
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
	s := newTestChainStore(t)
	r1 := recordJSON(1, EmptyChainDigest, "shaper", "estimator", "completed", "")
	if _, err := s.Append([]byte(r1)); err != nil {
		t.Fatalf("Append() first = %v, want nil", err)
	}
	r1different := recordJSON(1, EmptyChainDigest, "prototyper", "shaper", "completed", "")
	if _, err := s.Append([]byte(r1different)); err == nil {
		t.Fatalf("Append() = nil, want refusal to overwrite seq 1 with different bytes")
	}
}

func TestChainStoreAppendRejectsAfterTerminalDelivery(t *testing.T) {
	s := newTestChainStore(t)
	r1 := recordJSON(1, EmptyChainDigest, "shaper", "estimator", "completed", "")
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
	s := newTestChainStore(t)
	dir, err := s.chainDir("proj-1", "goal-1", "chain-1")
	if err != nil {
		t.Fatalf("chainDir() = %v, want nil", err)
	}
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
	r1 := recordJSON(1, EmptyChainDigest, "shaper", "estimator", "completed", "")
	if _, err := s.Append([]byte(r1)); err == nil {
		t.Fatalf("Append() = nil, want refusal of a symlinked chain directory")
	}
}

func TestChainStoreRejectsUnsafePathComponent(t *testing.T) {
	s := newTestChainStore(t)
	if _, err := s.chainDir("../escape", "goal-1", "chain-1"); err == nil {
		t.Fatalf("chainDir() = nil, want error for an unsafe project_id component")
	}
}
