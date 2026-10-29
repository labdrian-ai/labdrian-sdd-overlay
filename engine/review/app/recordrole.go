package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

// RoleRecorded is what RecordSweeper and RecordPolisher stored: a record of a role that comes after the builder, which
// has no skeleton for the role chain (own-rdd T6b4, S6 c).
type RoleRecorded struct {
	Candidate review.Candidate
	// Path is where the record is stored, relative to the root of the working tree and written with '/'.
	Path string
	// Number is the number it takes for the head: the <n> of the file name, which the results, the verdicts, the
	// exemptions and the other role records of the head share.
	Number int
	// SHA256 is the digest of the bytes of the stored file.
	SHA256 string
	// NotFlushed, when it is not nil, wraps ErrNotFlushed: the record is stored, but the directory could not be flushed
	// after it. The record is not stored again.
	NotFlushed error
}

// RecordSweeper holds the report of the sweeper to the sweep it answers for and to the commit it names, binds the one
// commit from base to head as Exempt does, and stores the report as the sweeper record of the head (own-rdd T6b4, S6 c).
//
// The rules are applied in this order and the first one broken is the refusal, with nothing stored:
//
//   - the feature is a single safe path segment (review.ErrInvalidFeature);
//   - the report is at most review.MaxReplyBytes (review.ErrReplyTooLarge) and the object of review.ParseSweeperReport:
//     a refused one is a *ReplyRefusedError with the named error inside it, before git is asked anything;
//   - the revisions bind and are one commit: base is the first parent of head (review.ErrNotOneCommit);
//   - a stored sweep of the head fired and binds it. Every stored sweep of the head is read back and held to git, and a
//     file that does not read back or does not hold is an error, never skipped; the sweep that counts is the latest
//     fired one, and none is a *ReplyRefusedError that wraps review.ErrInvalidSweeperReport;
//   - the report answers for every candidate of that sweep once and for no other, and names a commit when a candidate is
//     removed or corrected (review.CheckSweeperReport);
//   - the commit it names, when it names one, has head as its first parent and a subject that starts "chore(sweep)"
//     (review.CheckSweepCommit).
//
// The record is the report with the binding of the candidate and the name of the sweep it answers for, in the next free
// number of the head. The candidate is computed from git, never taken from the report.
func (s Service) RecordSweeper(dir, feature, base, head string, report []byte) (RoleRecorded, error) {
	if err := review.CheckFeature(feature); err != nil {
		return RoleRecorded{}, err
	}
	parsed, err := review.ParseSweeperReport(report)
	if err != nil {
		return RoleRecorded{}, &ReplyRefusedError{Err: err}
	}
	repo, candidate, err := s.candidate(dir, base, head)
	if err != nil {
		return RoleRecorded{}, err
	}
	if err := s.oneCommit(repo, candidate.Base, candidate.Head); err != nil {
		return RoleRecorded{}, err
	}
	sweep, stored, err := s.firedSweep(repo, feature, candidate)
	if err != nil {
		return RoleRecorded{}, err
	}
	if err := review.CheckSweeperReport(parsed, stored); err != nil {
		return RoleRecorded{}, &ReplyRefusedError{Err: err}
	}
	if parsed.Commit != nil {
		if err := s.sweepCommit(repo, candidate.Head, *parsed.Commit); err != nil {
			return RoleRecorded{}, err
		}
	}
	file, err := review.EncodeSweeper(candidate, sweep, parsed)
	if err != nil {
		return RoleRecorded{}, err
	}
	return s.storeRole(repo.Root, feature, candidate, review.SweeperName, file)
}

// firedSweep is the latest stored sweep of the head that fired, with its name. It reads back every sweep of the head
// and holds it to git as a reader does (review.CheckRecordBinding), so a record that does not hold is an error and not
// a sweep that is passed over.
func (s Service) firedSweep(repo Repository, feature string, c review.Candidate) (string, review.StoredSweep, error) {
	names, err := s.results.Names(repo.Root, feature)
	if err != nil {
		return "", review.StoredSweep{}, fmt.Errorf("list %s/%s: %w", review.ResultsDir, feature, err)
	}
	var latest string
	var found review.StoredSweep
	for _, name := range review.SweepsOfHead(names, c.Head) {
		path := review.ResultPath(feature, name)
		file, err := s.results.Read(repo.Root, feature, name)
		if err != nil {
			return "", review.StoredSweep{}, fmt.Errorf("read %s: %w", path, err)
		}
		stored, err := review.DecodeSweep(file)
		if err != nil {
			return "", review.StoredSweep{}, fmt.Errorf("%s: %w", path, err)
		}
		if stored.Binding.Head != c.Head {
			continue
		}
		if err := review.CheckRecordBinding(stored.Binding, c); err != nil {
			return "", review.StoredSweep{}, fmt.Errorf("%s: %w", path, err)
		}
		if stored.Fired() {
			latest, found = name, stored
		}
	}
	if latest == "" {
		return "", review.StoredSweep{}, &ReplyRefusedError{Err: fmt.Errorf("%w: no stored sweep of %s fired and binds it, so there is nothing for a sweeper to answer",
			review.ErrInvalidSweeperReport, c.Head)}
	}
	return latest, found, nil
}

// sweepCommit holds the commit the sweeper names to the facts git gives about it.
func (s Service) sweepCommit(repo Repository, head, commit string) error {
	isCommit, err := s.git.IsCommit(repo, commit)
	if err != nil {
		return fmt.Errorf("find out whether %s is a commit: %w", commit, err)
	}
	if !isCommit {
		return &ReplyRefusedError{Err: fmt.Errorf("%w: the commit %s is not a commit that git has", review.ErrInvalidSweeperReport, commit)}
	}
	parent, err := s.git.Parent(repo, commit)
	if err != nil {
		return fmt.Errorf("find the first parent of %s: %w", commit, err)
	}
	message, err := s.git.Message(repo, commit)
	if err != nil {
		return fmt.Errorf("read the message of %s: %w", commit, err)
	}
	subject, _, _ := strings.Cut(message, "\n")
	if err := review.CheckSweepCommit(head, commit, parent, subject); err != nil {
		return &ReplyRefusedError{Err: err}
	}
	return nil
}

// RecordPolisher holds the findings of one polisher judge to the object of review.ParsePolisherFindings, binds the one
// commit from base to head as Exempt does, and stores them as the polisher record of the head (own-rdd T6b4, S6 c). The
// rules are applied in this order and the first one broken is the refusal, with nothing stored: the feature
// (review.ErrInvalidFeature); the findings, at most review.MaxReplyBytes and the object the contract describes, a
// refused one being a *ReplyRefusedError with the named error inside it, before git is asked anything; the revisions bind
// and base is the first parent of head (review.ErrNotOneCommit); and the stored file is at most review.MaxStoredBytes
// (review.EncodePolisher), so a record that is stored always reads back. No finding blocks: the engine never judges what a
// polisher found.
func (s Service) RecordPolisher(dir, feature, base, head string, findings []byte) (RoleRecorded, error) {
	if err := review.CheckFeature(feature); err != nil {
		return RoleRecorded{}, err
	}
	if _, err := review.ParsePolisherFindings(findings); err != nil {
		return RoleRecorded{}, &ReplyRefusedError{Err: err}
	}
	repo, candidate, err := s.candidate(dir, base, head)
	if err != nil {
		return RoleRecorded{}, err
	}
	if err := s.oneCommit(repo, candidate.Base, candidate.Head); err != nil {
		return RoleRecorded{}, err
	}
	file, err := review.EncodePolisher(candidate, findings)
	if err != nil {
		if errors.Is(err, review.ErrReplyTooLarge) {
			return RoleRecorded{}, &ReplyRefusedError{Err: err}
		}
		return RoleRecorded{}, err
	}
	return s.storeRole(repo.Root, feature, candidate, review.PolisherName, file)
}

// storeRole stores file as the record of a role in the next free number of the head and says what it stored.
func (s Service) storeRole(root, feature string, c review.Candidate, nameOf func(review.Candidate, int) string, file []byte) (RoleRecorded, error) {
	number, name, err := s.store(root, feature, c, nameOf, file)
	var notFlushed error
	switch {
	case errors.Is(err, ErrNotFlushed):
		notFlushed = err
	case err != nil:
		return RoleRecorded{}, err
	}
	return RoleRecorded{Candidate: c, Path: review.ResultPath(feature, name), Number: number, SHA256: sha256hex.Sum(file), NotFlushed: notFlushed}, nil
}
