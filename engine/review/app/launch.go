package app

import (
	"errors"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

// Reviewer is the port that starts a reviewer over a worktree and answers what it replied. The adapter wraps
// ErrReviewerDidNotAnswer in every error it returns for a reviewer that did not give a reply.
type Reviewer interface {
	// Review runs the reviewer with model in worktree, given brief, and answers its reply.
	Review(worktree, model string, brief []byte) ([]byte, error)
}

var (
	// ErrReviewerDidNotAnswer is a reviewer that was not found, exited with an error, ran out of time or left no reply.
	ErrReviewerDidNotAnswer = errors.New("review: the reviewer did not answer")
	// ErrNoReviewer is a Service that was given no Reviewer, so Launch refuses to run.
	ErrNoReviewer = errors.New("review: no reviewer is configured")
)

// WithReviewer is the Service that starts reviewers with r, which Launch needs. Without one Launch refuses to run.
func (s Service) WithReviewer(r Reviewer) Service {
	s.reviewer = r
	return s
}

// Launch binds the diff from base to head in the repository that holds dir (Bind, which makes or reuses the detached
// worktree), starts the reviewer with model in that worktree over brief, and records its reply (Record) over the
// full names of the bound candidate and rows. Everything that can be refused is refused before the reviewer starts, in
// this order: the feature, the rows, the model, the binding, a diff over review.DiffLimitBytes, and a brief that is not
// the brief of the candidate. An error of the reviewer is returned as it is and nothing is stored; a reply Record refuses is
// the *ReplyRefusedError of Record.
func (s Service) Launch(dir, feature, base, head, rows, model string, brief []byte) (Recorded, error) {
	if s.reviewer == nil {
		return Recorded{}, ErrNoReviewer
	}
	if err := review.CheckFeature(feature); err != nil {
		return Recorded{}, err
	}
	if _, err := review.ParseRows(rows); err != nil {
		return Recorded{}, err
	}
	if err := review.CheckModel(model); err != nil {
		return Recorded{}, err
	}
	bound, err := s.Bind(dir, base, head)
	if err != nil {
		return Recorded{}, err
	}
	c := bound.Candidate
	if c.OverLimit() {
		return Recorded{}, review.OverLimitError(c)
	}
	if err := review.CheckBrief(c, bound.Worktree, brief); err != nil {
		return Recorded{}, err
	}
	reply, err := s.reviewer.Review(bound.Worktree, model, brief)
	if err != nil {
		return Recorded{}, err
	}
	return s.Record(dir, feature, c.Base, c.Head, rows, reply)
}
