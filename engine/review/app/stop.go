package app

import (
	"errors"
	"fmt"
)

// Asked is the store of the questions the stop hook has put, by session: what lets it ask each question once.
type Asked interface {
	// Has says whether the question named key was put in the session.
	Has(session, key string) (bool, error)
	// Add records that the question named key was put in the session. Recording it again changes nothing.
	Add(session, key string) error
}

// noDefaultKey names the question that the repository does not tell its default branch: one for the session, whichever
// branch the agent is on.
const noDefaultKey = "no-default-branch"

// Ruling is what the stop hook decides for a stop of the agent (own-rdd T6a, S6).
type Ruling struct {
	// Block says that the stop is blocked, to put Question. It is blocked once for each session and question.
	Block bool
	// Question is what is put when Block. With NoDefault it names the branch and nothing else.
	Question Question
	// NoDefault says that the question is that the repository does not tell its default branch, so the commits of the
	// branch that need a review cannot be told.
	NoDefault bool
}

// Stop decides whether the stop of the agent of session is blocked, in the repository that holds dir (absolute):
// once for each session and question, when a commit of the branch has no review (NextUnreviewed) or the repository
// does not tell its default branch, which is a question of its own (own-rdd T6a, S6). continuing is the stop_hook_active
// of the hook input, true while the agent is continuing because a stop hook blocked its stop: the stop is never blocked
// then, and nothing is asked of git or of the stores, because a hook that keeps blocking keeps the agent from stopping.
//
// A question is asked once: when the store of the questions has it for the session the stop is not blocked, and when a
// question is put it is recorded before the stop is blocked. A stop is not blocked on what cannot be told or cannot be
// remembered: the error is returned with no block, since a block that could not be recorded would be asked again at
// every stop. The questions are the Asked port's: a Service made without one (WithAsked) cannot run Stop.
func (s Service) Stop(dir, session string, continuing bool) (Ruling, error) {
	if s.asked == nil {
		return Ruling{}, errors.New("review: the stop hook has no store of the questions it has put")
	}
	if continuing {
		return Ruling{}, nil
	}
	question, ask, err := s.NextUnreviewed(dir)
	var noDefault *NoDefaultBranchError
	switch {
	case errors.As(err, &noDefault):
		return s.once(session, noDefaultKey, Ruling{Block: true, NoDefault: true, Question: Question{Branch: noDefault.Branch}})
	case err != nil || !ask:
		return Ruling{}, err
	}
	return s.once(session, short(question.Base)+"-"+short(question.Head), Ruling{Block: true, Question: question})
}

// once is ruling when the question named key was not put in the session yet, and records that it is put, and no block
// when it was.
func (s Service) once(session, key string, ruling Ruling) (Ruling, error) {
	before, err := s.asked.Has(session, key)
	switch {
	case err != nil:
		return Ruling{}, fmt.Errorf("read the questions asked in the session: %w", err)
	case before:
		return Ruling{}, nil
	}
	if err := s.asked.Add(session, key); err != nil {
		return Ruling{}, fmt.Errorf("record the question asked in the session: %w", err)
	}
	return ruling, nil
}

// short is the first 12 characters of a full object name, as the names of the stored files use them.
func short(name string) string {
	return name[:min(len(name), 12)]
}
