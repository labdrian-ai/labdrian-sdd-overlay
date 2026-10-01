package roles

import "fmt"

// This file is the domain's half of keeping a role handoff chain: the port that
// says what a place for chains must do (ChainLog) and the rule for when a record may
// join a chain (AdmitRecord). The rule is pure: it takes the chain as records and
// the new record as a value, and knows nothing of where chains live. A ChainLog
// adapter owns that (see engine/roles/filechain) and applies the rule in the same
// order every time: read the chain, AdmitRecord the new record, store it.
//
// The messages keep the wording the store has always printed ("role chain store:
// append: ..."); callers show them to a person, and they do not change with where
// the code lives.

// ChainLog is where role handoff chains are kept. The domain owns this port and
// adapters implement it; nothing here grants execution authority, it only persists
// and verifies data.
type ChainLog interface {
	// LoadChain returns every record of one chain in seq order, verified with
	// VerifyChain. A chain that does not exist yet is empty and not an error:
	// appending to it is simply the first record.
	LoadChain(projectID, goalID, chainID string) ([]ChainRecord, error)
	// Append strictly parses data as a RoleHandoff, admits it to the chain it
	// declares (AdmitRecord) and stores it, immutably: storing identical bytes
	// again at the same seq changes nothing, and different bytes there are
	// refused with the stored record left as it was. It returns where the record
	// is kept, as a person would read it (a path, for a file adapter).
	Append(data []byte) (location string, err error)
}

// AdmitRecord decides whether candidate may join the chain whose records are chain,
// already verified and in seq order from 1. It is admitted when its seq is the
// next one (len(chain)+1) and the chain extended by it still verifies (VerifyChain),
// or when its seq is one the chain already has: whether that is the same record
// again or a different one claiming the seq is the store's immutability rule, which
// compares the bytes where they are kept. Every other seq is refused, and so is a
// record that breaks the chain (a wrong prev_sha256, a role that does not follow,
// anything after delivery). It performs no I/O.
func AdmitRecord(chain []ChainRecord, candidate ChainRecord) error {
	seq := candidate.Handoff.Seq
	switch {
	case seq >= 1 && seq <= len(chain):
		// A stored seq. The chain was verified when it was loaded.
		return nil
	case seq == len(chain)+1:
		extended := make([]ChainRecord, 0, len(chain)+1)
		extended = append(extended, chain...)
		extended = append(extended, candidate)
		if err := VerifyChain(extended); err != nil {
			return fmt.Errorf("role chain store: append: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("role chain store: append: seq %d is neither the next seq (%d) nor an existing one (chain has %d records)", seq, len(chain)+1, len(chain))
	}
}
