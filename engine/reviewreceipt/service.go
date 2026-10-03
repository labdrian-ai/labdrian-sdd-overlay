package reviewreceipt

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// What a name is for, as an UnsafeNameError says it.
const (
	kindChange      = "change name"
	kindReceiptFile = "receipt file name"
)

// Service captures the review receipts of one project: it reads them from the stores the
// review tool keeps its transactions in and persists them where the project versions them,
// before the acknowledgement that burns them. It is built over the four ports and knows no
// file, process or git.
//
// A Service is built for one project, the one its ports answer for. The zero value has no
// ports and refuses to do anything.
type Service struct {
	ports Ports
}

// NewService builds the service over its ports. A missing port is not guessed around: every
// operation of the service refuses, and says which port is missing.
func NewService(p Ports) *Service {
	return &Service{ports: p}
}

// ready fails when a port is missing.
func (s *Service) ready() error {
	var missing string
	switch {
	case s == nil || s.ports.Stores == nil:
		missing = "TransactionStores"
	case s.ports.Source == nil:
		missing = "ReceiptSource"
	case s.ports.Sink == nil:
		missing = "ReceiptSink"
	case s.ports.Changes == nil:
		missing = "ChangeCatalog"
	default:
		return nil
	}
	return fmt.Errorf("reviewreceipt: the service has no %s port", missing)
}

// Capture persists every approved review receipt that survives in the project's stores into
// openspec/changes/<change>/review-receipts/, under the name its shape gives it
// (ApprovedReceipt.FileName). It returns the receipts captured (or already present
// byte-for-byte) this run, and on failure the ones that came before it.
//
// It never replaces a receipt file with different bytes: that is an error, not an overwrite.
// A lineage is persisted once per shape, however many stores hold it. The change name and
// every file name are checked to be one path component (CheckPathComponent) before the sink
// is asked anything, so a receipt is never put outside the change's folder.
func (s *Service) Capture(change string) ([]Captured, error) {
	if strings.TrimSpace(change) == "" {
		return nil, errors.New("reviewreceipt: change name is required")
	}
	if err := CheckPathComponent(kindChange, change); err != nil {
		return nil, err
	}
	if err := s.ready(); err != nil {
		return nil, err
	}

	var captured []Captured
	err := s.eachApproved(func(sv surviving) error {
		if err := s.persist(change, sv); err != nil {
			return err
		}
		captured = append(captured, Captured{LineageID: sv.Receipt.Lineage, Path: s.ports.Sink.Location(change, sv.Receipt.FileName())})
		return nil
	})
	return captured, err
}

// persist keeps one receipt: nothing to do when exactly these bytes are already persisted,
// a refusal when other bytes are, and a write when none are.
func (s *Service) persist(change string, sv surviving) error {
	name := sv.Receipt.FileName()
	existing, err := s.ports.Sink.Read(change, name)
	switch {
	case err == nil:
		if bytes.Equal(existing, sv.Data) {
			return nil
		}
		return fmt.Errorf("reviewreceipt: %s already exists with different content", s.ports.Sink.Location(change, name))
	case !errors.Is(err, ErrNotPersisted):
		return fmt.Errorf("reviewreceipt: %w", err)
	}
	if err := s.ports.Sink.Write(change, name, sv.Data); err != nil {
		return fmt.Errorf("reviewreceipt: %w", err)
	}
	return nil
}

// eachApproved calls visit for every currently-approved receipt across the project's
// stores (either shape), once per lineage and shape, in the order the stores list them. A
// store is read when the walk gets to it, and what it holds is visited before the next is
// read, so a caller that writes as it goes has written what came before a failure. It
// stops at the first error a store or a visit returns. An approved receipt that cannot be
// kept (an *UnusableReceiptError) does not stop the walk: every other receipt is still
// visited, and the walk then fails naming each one that could not be kept, so one bad
// document never keeps the others from being persisted, and none is skipped in silence.
func (s *Service) eachApproved(visit func(surviving) error) error {
	stores, err := s.ports.Stores.Stores()
	if err != nil {
		return fmt.Errorf("reviewreceipt: %w", err)
	}
	var unusable []error
	seen := map[seenReceipt]bool{}
	for _, store := range stores {
		documents, err := s.ports.Source.Documents(store)
		if err != nil {
			return fmt.Errorf("reviewreceipt: %w", err)
		}
		for _, doc := range documents {
			r, ok := approvedIn(doc.Shape, doc.Data)
			key := seenReceipt{doc.Shape, r.Lineage}
			if !ok || seen[key] {
				continue
			}
			seen[key] = true
			// The lineage id was read from a document another program wrote; the file name
			// it makes is checked before anything is asked of the sink with it.
			if err := CheckPathComponent(kindReceiptFile, r.FileName()); err != nil {
				unusable = append(unusable, &UnusableReceiptError{Origin: doc.Origin, Err: err})
				continue
			}
			if err := visit(surviving{Receipt: r, Data: doc.Data}); err != nil {
				// The refusals collected so far are kept: a later failure drops none.
				return errors.Join(append(unusable, err)...)
			}
		}
	}
	return errors.Join(unusable...)
}

// AllSurvivingApprovedPersisted reports whether every currently-approved receipt is already
// persisted byte-for-byte under some change in changes (the explicit capture remedy already
// ran); zero surviving receipts is true.
func (s *Service) AllSurvivingApprovedPersisted(changes []string) (bool, error) {
	if err := s.ready(); err != nil {
		return false, err
	}
	var all []surviving
	if err := s.eachApproved(func(sv surviving) error {
		all = append(all, sv)
		return nil
	}); err != nil {
		return false, err
	}
	for _, sv := range all {
		found := false
		for _, change := range changes {
			if err := CheckPathComponent(kindChange, change); err != nil {
				return false, err
			}
			persisted, err := s.ports.Sink.Read(change, sv.Receipt.FileName())
			if errors.Is(err, ErrNotPersisted) {
				continue
			}
			if err != nil {
				return false, fmt.Errorf("reviewreceipt: %w", err)
			}
			if bytes.Equal(persisted, sv.Data) {
				found = true
				break
			}
		}
		if !found {
			return false, nil
		}
	}
	return true, nil
}

// DetectActiveChange returns the single non-archive change under openspec/changes/ that
// carries at least one recognized SDD artifact file (see activeChangeMarkers). It returns
// ("", nil) when there is no openspec/changes/ directory or no active change -- callers
// treat that as pass-through, not an error. More than one active change is reported as
// *MultipleActiveChangesError so callers can fail closed instead of guessing which change a
// captured receipt belongs to.
func (s *Service) DetectActiveChange() (string, error) {
	if err := s.ready(); err != nil {
		return "", err
	}
	names, err := s.ports.Changes.Changes()
	if err != nil {
		return "", fmt.Errorf("reviewreceipt: %w", err)
	}

	var active []string
	for _, name := range names {
		if name == "archive" {
			continue
		}
		for _, marker := range activeChangeMarkers() {
			if s.ports.Changes.HasArtifact(name, marker) {
				active = append(active, name)
				break
			}
		}
	}

	switch len(active) {
	case 0:
		return "", nil
	case 1:
		return active[0], nil
	default:
		sort.Strings(active)
		return "", &MultipleActiveChangesError{Changes: active}
	}
}

// Verdict is what the service decides about a command a tool call is about to run: whether to
// deny it, and why. The zero value is an allow.
type Verdict struct {
	Deny   bool
	Reason string
}

func deny(reason string) Verdict { return Verdict{Deny: true, Reason: reason} }

// CheckCommand is the fail-closed check that runs before every shell command: only when the
// command contains acknowledgeMarker does it resolve the single active change and capture every
// surviving approved receipt before it allows the command, which burns them. How the command
// reaches it, and what the runtime is told of the verdict, is the hook adapter's.
//
// The verdict is:
//   - an allow: the command does not match, there is no openspec/changes/ directory, or
//     there is no active change to attach a receipt to.
//   - an allow: exactly one active change existed and Capture succeeded.
//   - an allow: more than one active change exists, but every surviving approved receipt is
//     already persisted under some active change (the `capture --change <name>` remedy
//     already ran) -- a fix-forward path for a retried acknowledgement.
//   - a denial, with its reason: more than one active change exists and at least one surviving
//     approved receipt is unpersisted, or Capture itself failed. Fail-closed.
//
// A command that is not an acknowledgement is allowed without the service asking anything, and so
// is the empty command that a call with no command has. A service that is missing a port denies
// an acknowledgement it cannot guard.
func (s *Service) CheckCommand(command string) Verdict {
	if !strings.Contains(command, acknowledgeMarker) {
		return Verdict{}
	}
	if err := s.ready(); err != nil {
		return deny(fmt.Sprintf("review-receipt: %v", err))
	}

	change, err := s.DetectActiveChange()
	if err != nil {
		var multi *MultipleActiveChangesError
		if errors.As(err, &multi) {
			allPersisted, perr := s.AllSurvivingApprovedPersisted(multi.Changes)
			if perr != nil {
				return deny(fmt.Sprintf("review-receipt: %v", perr))
			}
			if allPersisted {
				return Verdict{}
			}
			return deny(multi.Error())
		}
		return deny(fmt.Sprintf("review-receipt: %v", err))
	}
	if change == "" {
		return Verdict{}
	}

	if _, err := s.Capture(change); err != nil {
		return deny(fmt.Sprintf("review-receipt: capture failed: %v", err))
	}
	return Verdict{}
}
