package roles

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// ChainRecord pairs one parsed RoleHandoff with the exact raw bytes it was
// parsed from. The raw bytes are what the next record's prev_sha256 chains
// to, so they are kept alongside the parsed value rather than re-derived.
type ChainRecord struct {
	Raw     []byte
	Handoff RoleHandoff
}

// sha256Hex returns the lowercase hex SHA-256 of data.
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// VerifyChain checks one chain's records, in seq order starting at 1: seq 1
// has prev_sha256 equal to EmptyChainDigest and from_role prototyper or
// shaper; each later record's prev_sha256 equals the SHA-256 of the previous
// record's raw bytes; seq is contiguous; from_role equals the previous
// record's to_role, except that a record following an interrupted record
// must repeat that record's exact from_role and to_role; and nothing follows
// a completed record whose to_role is delivery (terminal). It does not
// re-check each record's own field shapes; ParseRoleHandoff already does
// that. It is first-error-wins and performs no I/O.
func VerifyChain(records []ChainRecord) error {
	for i, r := range records {
		h := r.Handoff
		if h.Seq != i+1 {
			return fmt.Errorf("role chain: record at index %d has seq %d, want %d", i, h.Seq, i+1)
		}
		if i == 0 {
			if h.PrevSHA256 != EmptyChainDigest {
				return fmt.Errorf("role chain: seq 1 prev_sha256 must be the empty-chain digest, got %q", h.PrevSHA256)
			}
			if h.FromRole != RolePrototyper && h.FromRole != RoleShaper {
				return fmt.Errorf("role chain: seq 1 from_role must be %q or %q, got %q", RolePrototyper, RoleShaper, h.FromRole)
			}
			continue
		}
		prev := records[i-1]
		wantPrevSHA := sha256Hex(prev.Raw)
		if h.PrevSHA256 != wantPrevSHA {
			return fmt.Errorf("role chain: seq %d prev_sha256 %q does not match seq %d bytes digest %q", h.Seq, h.PrevSHA256, prev.Handoff.Seq, wantPrevSHA)
		}
		if prev.Handoff.Status == StatusInterrupted {
			if h.FromRole != prev.Handoff.FromRole || h.ToRole != prev.Handoff.ToRole {
				return fmt.Errorf("role chain: seq %d must repeat the interrupted seq %d transition %s -> %s, got %s -> %s",
					h.Seq, prev.Handoff.Seq, prev.Handoff.FromRole, prev.Handoff.ToRole, h.FromRole, h.ToRole)
			}
			continue
		}
		if h.FromRole != prev.Handoff.ToRole {
			return fmt.Errorf("role chain: seq %d from_role %q must equal seq %d to_role %q", h.Seq, h.FromRole, prev.Handoff.Seq, prev.Handoff.ToRole)
		}
		if prev.Handoff.ToRole == RoleDelivery {
			return fmt.Errorf("role chain: seq %d is terminal (delivery); seq %d must not follow", prev.Handoff.Seq, h.Seq)
		}
	}
	return nil
}

// ResumeState describes where a chain currently stands: which role holds the
// baton, whether that role's work was interrupted, and whether the chain is
// terminal. It is a derivation of data only and grants no authority.
type ResumeState struct {
	Role         Role
	Interrupted  bool
	ResumeReason string
	// Terminal is true when the chain's last record is a completed handoff
	// to delivery; no further record may follow.
	Terminal bool
}

// Resume derives the current ResumeState from records, ordered seq 1..N. It
// does not itself verify the chain; call VerifyChain first when records come
// from an untrusted source.
func Resume(records []ChainRecord) (ResumeState, error) {
	if len(records) == 0 {
		return ResumeState{}, fmt.Errorf("role chain: resume requires at least one record")
	}
	last := records[len(records)-1].Handoff
	state := ResumeState{Role: last.ToRole}
	if last.Status == StatusInterrupted {
		state.Interrupted = true
		if last.ResumeReason != nil {
			state.ResumeReason = *last.ResumeReason
		}
	} else if last.ToRole == RoleDelivery {
		state.Terminal = true
	}
	return state, nil
}

// Next returns, in canonical order, every role NextRoles allows from the
// chain's current ResumeState. It returns nil for a terminal chain.
func Next(records []ChainRecord) ([]Role, error) {
	state, err := Resume(records)
	if err != nil {
		return nil, err
	}
	if state.Terminal {
		return nil, nil
	}
	return NextRoles(state.Role), nil
}
