package app

import (
	"bytes"
	"errors"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/contract"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator"
)

// MaxAttempts bounds the loop of PropagateVerified. It defends against a foreign, uncoordinated
// writer (the `gentle-ai skill-registry refresh` binary, which regenerates the registry whole and
// knows nothing of the block convention or of the lock) tearing a read, removing the registry for
// a moment, or replacing it between a write and its read-back. A few attempts win the race against
// a single competing write. The number is a bound on purpose: a registry that something else
// rewrites in a tight loop fails loudly instead of keeping the run spinning.
const MaxAttempts = 3

// Request is one run: which registry, which contract, how the row is scoped, and whether a
// missing registry is an error.
type Request struct {
	// Registry is the path of the registry, as the store is to be asked for it.
	Registry string
	// Contract is where the text of the contract is had, once per pass.
	Contract ContractSource
	// Config scopes the row: the path it names and the markers and label that delimit it.
	Config propagator.Config
	// RequireRegistry makes a registry that is absent an error. By default it is Absent, a clean
	// no-op, because a project without a registry does not use the overlay.
	RequireRegistry bool
}

// Attempt is what one pass did.
type Attempt struct {
	Outcome Outcome
	// Written holds the bytes written when the Outcome is Written, for the read-back to compare
	// with; it is nil otherwise.
	Written []byte
}

// Service runs the use case over the registry store it is given.
type Service struct{ store RegistryStore }

// New returns the use case over store.
func New(store RegistryStore) *Service { return &Service{store: store} }

// Propagate makes one pass: it reads the contract, then the registry, decides and, when the row is
// missing or stale, writes the registry. It does not read the write back and does not retry; that
// is PropagateVerified. The contract is read first and the registry second, so a contract that
// cannot be had is reported even when the registry is absent.
func (s *Service) Propagate(req Request) (Attempt, error) {
	text, err := req.Contract.Text()
	if err != nil {
		return Attempt{}, &ContractReadError{Err: err}
	}
	doc, err := contract.Parse(text)
	if err != nil {
		return Attempt{}, &ContractParseError{Err: err}
	}

	current, err := s.store.Read(req.Registry)
	if err != nil {
		var missing *NotFoundError
		if !errors.As(err, &missing) {
			return Attempt{}, &RegistryReadError{Path: req.Registry, Err: err}
		}
		if req.RequireRegistry {
			return Attempt{}, &RegistryRequiredError{Path: req.Registry}
		}
		return Attempt{Outcome: Absent}, nil
	}
	if strings.TrimSpace(string(current)) == "" {
		return Attempt{Outcome: Empty}, nil
	}

	out, changed, err := propagator.Propagate(string(current), req.Config, doc)
	if err != nil {
		return Attempt{}, &RewriteError{Err: err}
	}
	if !changed {
		return Attempt{Outcome: Unchanged}, nil
	}
	written := []byte(out)
	if err := s.store.Write(req.Registry, written); err != nil {
		return Attempt{}, &RegistryWriteError{Path: req.Registry, Err: err}
	}
	return Attempt{Outcome: Written, Written: written}, nil
}

// evidence is what the passes of one run have shown so far about the registry, kept across all of
// them and not only the last: a run whose first pass wrote the registry (it exists) and whose last
// two found it absent must not conclude that there is none.
type evidence struct {
	sawEmpty, sawAbsent, sawWrite bool
}

// PropagateVerified is Propagate under a bounded loop. Propagate trusts the write it made, and
// that is safe against another run of the same command, which the caller serializes with a lock,
// but not against a writer outside the lock. So after each write the registry is read back and
// compared byte for byte with what was written; when it differs the whole pass starts over, not
// just the write, because what the right registry is may have changed with the foreign write.
//
// Three kinds of pass are not an answer yet: one that found the registry Empty, one that found
// it Absent, and one whose write did not stay. They are retried up to MaxAttempts passes. A pass
// that found the registry Unchanged is the answer at once, and so is any error: neither has a
// race to wait out. What the run concludes when the budget is spent depends on every pass, not the
// last (see evidence): Absent on every pass is Absent, a clean no-op; Empty on every pass is an
// EmptyRegistryError; absent and empty in turn, or either after a write landed, is an
// InconsistentRegistryError; a write that never stayed is a LostWriteError, or an
// UnverifiedWriteError when the read-back itself failed.
//
// The Outcome returned is the one of the last pass, which is Absent, Unchanged or Written.
func (s *Service) PropagateVerified(req Request) (Outcome, error) {
	var seen evidence
	for attempt := 1; attempt <= MaxAttempts; attempt++ {
		last := attempt == MaxAttempts
		got, err := s.Propagate(req)
		if err != nil {
			return 0, err
		}
		switch got.Outcome {
		case Unchanged:
			return Unchanged, nil
		case Empty:
			seen.sawEmpty = true
			if !last {
				continue
			}
			if seen.sawAbsent || seen.sawWrite {
				return 0, &InconsistentRegistryError{Path: req.Registry, Attempts: MaxAttempts}
			}
			return 0, &EmptyRegistryError{Path: req.Registry, Attempts: MaxAttempts}
		case Absent:
			seen.sawAbsent = true
			if !last {
				continue
			}
			if seen.sawEmpty || seen.sawWrite {
				return 0, &InconsistentRegistryError{Path: req.Registry, Attempts: MaxAttempts}
			}
			return Absent, nil
		}

		seen.sawWrite = true
		onDisk, readErr := s.store.Read(req.Registry)
		if readErr == nil && bytes.Equal(onDisk, got.Written) {
			return Written, nil
		}
		if last {
			if readErr != nil {
				return 0, &UnverifiedWriteError{Path: req.Registry, Attempts: MaxAttempts, Err: readErr}
			}
			return 0, &LostWriteError{Path: req.Registry, Attempts: MaxAttempts}
		}
	}
	panic("app: PropagateVerified left its loop without an answer")
}
