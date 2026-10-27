package app

import (
	"errors"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

// Exemption is what Exempt stored.
type Exemption struct {
	Candidate review.Candidate
	// Path is where the exemption is stored, relative to the root of the working tree and written with '/'.
	Path string
	// Number is the number it takes for the head: the <n> of the file name, which the results and the verdicts of
	// the head share.
	Number int
	// SHA256 is the digest of the bytes of the stored file.
	SHA256 string
	// Handoff is the skeleton of the record of the role chain that closes the exemption.
	Handoff roles.RoleHandoff
	// NotFlushed, when it is not nil, wraps ErrNotFlushed: the exemption is stored, but the directory could not be
	// flushed after it, so it may not survive a crash of the machine. The exemption is not stored again.
	NotFlushed error
}

// Exempt exempts the head from review (own-rdd T4, S5, L16): the work took no review, the agent declared its risk
// and the verifier checked it. It holds the declared risk and the report of the verifier to their rules, binds the
// diff from base to head to the candidate git gives now, in the repository that holds dir (absolute), and stores the
// exemption in the next free number of the head in the results directory of the feature. The file is the report
// and the risk with the binding they were held to; no file is replaced.
//
// The rules are applied in this order and the first one broken is the refusal, before git or the store is asked
// anything they are not needed for, with nothing stored:
//
//   - the feature is a single safe path segment (review.ErrInvalidFeature);
//   - the declared risk is review.NoRisk: a declaration that names an item of the high-risk list is
//     review.ErrHighRisk and one the engine cannot read is review.ErrInvalidRisk, and neither is a reply;
//   - the report is at most review.MaxReplyBytes (review.ErrReplyTooLarge), the object of reviewverdict.ParseReport
//     and clear: a missing, malformed or not clear report is refused with a *ReplyRefusedError, the named error
//     inside it, exactly as a reply is, and a check that did not run never passes;
//   - the revisions are ones git cannot read as an option (review.ErrInvalidRev) and are two commits
//     (review.ErrInvalidCandidate);
//   - the exemption is for one commit: the base is the first parent of the head, as git gives it
//     (review.ErrNotOneCommit), so a walk of the chain from a delivery base meets the review of every commit.
//
// The candidate is computed from git in the same way as Bind and Record compute it, and is never taken from the
// report. Whether the head also has a stored result is not looked at here: the check of the delivery lets a result
// that blocks win over an exemption (review.Cleared).
func (s Service) Exempt(dir, feature, base, head, risk string, report []byte) (Exemption, error) {
	if err := review.CheckFeature(feature); err != nil {
		return Exemption{}, err
	}
	if err := review.CheckRisk(risk); err != nil {
		return Exemption{}, err
	}
	if len(report) > review.MaxReplyBytes {
		return Exemption{}, &ReplyRefusedError{Err: fmt.Errorf("%w: %d bytes, the maximum is %d", review.ErrReplyTooLarge, len(report), review.MaxReplyBytes)}
	}
	if err := review.CheckReport(report); err != nil {
		return Exemption{}, &ReplyRefusedError{Err: err}
	}
	repo, candidate, err := s.candidate(dir, base, head)
	if err != nil {
		return Exemption{}, err
	}
	if err := s.oneCommit(repo, candidate.Base, candidate.Head); err != nil {
		return Exemption{}, err
	}
	file, err := review.EncodeExemption(candidate, risk, report)
	if err != nil {
		return Exemption{}, err
	}
	number, name, err := s.store(repo.Root, feature, candidate, review.ExemptionName, file)
	var notFlushed error
	switch {
	case errors.Is(err, ErrNotFlushed):
		notFlushed = err
	case err != nil:
		return Exemption{}, err
	}
	path, sum := review.ResultPath(feature, name), sha256hex.Sum(file)
	return Exemption{Candidate: candidate, Path: path, Number: number, SHA256: sum, Handoff: review.ExemptionHandoff(candidate, path, sum), NotFlushed: notFlushed}, nil
}

// oneCommit holds the range of an exemption, base and head full names, to the one commit it is for: git is asked for
// the first parent of head and review.CheckOneCommit compares it with base. It is what Exempt asks before it stores and
// what Check asks of each stored exemption, so a range of several commits is never made and never read as one. Whatever
// keeps git from saying the parent is a refusal.
func (s Service) oneCommit(repo Repository, base, head string) error {
	parent, err := s.git.Parent(repo, head)
	if err != nil {
		return fmt.Errorf("find the first parent of %s: %w", head, err)
	}
	return review.CheckOneCommit(base, head, parent)
}
