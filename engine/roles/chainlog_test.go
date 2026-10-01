package roles

import (
	"strings"
	"testing"
)

// AdmitRecord is the rule for joining a chain, on records alone: these tests build
// chains in memory and never touch a file.
func TestAdmitRecord(t *testing.T) {
	c1, c2 := chainToBuilder(t)
	delivered := chainToDelivery(t)

	next := func(t *testing.T, prev ChainRecord, seq int, from, to string) ChainRecord {
		t.Helper()
		return chainRecord(t, recordJSON(seq, sha256Hex(prev.Raw), from, to, "completed", ""))
	}

	tests := []struct {
		name      string
		chain     []ChainRecord
		candidate ChainRecord
		wantErr   string // "" admits
	}{
		{
			name:      "the first record of an empty chain",
			candidate: c1,
		},
		{
			name:      "a first record that does not come from the prototyper or the shaper",
			candidate: chainRecord(t, recordJSON(1, EmptyChainDigest, "builder", "sweeper", "completed", "")),
			wantErr:   "role chain store: append: role chain: seq 1 from_role must be",
		},
		{
			name:      "a first record that does not chain to the empty digest",
			candidate: chainRecord(t, recordJSON(1, validDigest, "shaper", "estimator", "completed", "")),
			wantErr:   "role chain store: append: role chain: seq 1 prev_sha256 must be the empty-chain digest",
		},
		{
			name:      "the next record of a chain",
			chain:     []ChainRecord{c1},
			candidate: c2,
		},
		{
			name:      "a next record with the wrong prev_sha256",
			chain:     []ChainRecord{c1},
			candidate: chainRecord(t, recordJSON(2, otherDigest, "estimator", "builder", "completed", "")),
			wantErr:   "role chain store: append: role chain: seq 2 prev_sha256",
		},
		{
			name:      "a next record whose role does not follow",
			chain:     []ChainRecord{c1},
			candidate: next(t, c1, 2, "builder", "reviewer"),
			wantErr:   "role chain store: append: role chain: seq 2 from_role",
		},
		{
			name:      "a record that skips a seq",
			chain:     []ChainRecord{c1},
			candidate: next(t, c1, 3, "estimator", "builder"),
			wantErr:   "role chain store: append: seq 3 is neither the next seq (2) nor an existing one (chain has 1 records)",
		},
		{
			name:      "a record for seq 1 of an empty chain that starts at 2",
			candidate: next(t, c1, 2, "estimator", "builder"),
			wantErr:   "role chain store: append: seq 2 is neither the next seq (1) nor an existing one (chain has 0 records)",
		},
		{
			name:      "a record after the chain was delivered",
			chain:     delivered,
			candidate: next(t, delivered[3], 5, "reviewer", "builder"),
			wantErr:   "role chain store: append: role chain: seq 5",
		},
		{
			// Which record holds a seq is the store's immutability rule, decided on the
			// bytes where they are kept; the chain only says the seq is taken.
			name:      "a seq the chain already has",
			chain:     []ChainRecord{c1, c2},
			candidate: c1,
		},
		{
			name:      "a different record for a seq the chain already has",
			chain:     []ChainRecord{c1, c2},
			candidate: chainRecord(t, recordJSON(1, EmptyChainDigest, "prototyper", "shaper", "completed", "")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := AdmitRecord(tt.chain, tt.candidate)
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("AdmitRecord() = %v, want nil", err)
			case tt.wantErr != "" && (err == nil || !strings.HasPrefix(err.Error(), tt.wantErr)):
				t.Fatalf("AdmitRecord() = %v, want an error starting %q", err, tt.wantErr)
			}
		})
	}
}

// The chain a caller hands in is not changed by a record that would extend it.
func TestAdmitRecordDoesNotChangeTheChain(t *testing.T) {
	c1, c2 := chainToBuilder(t)
	chain := make([]ChainRecord, 1, 4)
	chain[0] = c1
	if err := AdmitRecord(chain, c2); err != nil {
		t.Fatalf("AdmitRecord() = %v, want nil", err)
	}
	if len(chain) != 1 || chain[:2][1].Handoff.Seq != 0 {
		t.Fatalf("chain = %+v, want it untouched, including the spare capacity behind it", chain[:2])
	}
}
