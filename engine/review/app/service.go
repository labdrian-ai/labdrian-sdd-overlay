package app

import (
	"errors"
	"fmt"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

// maxNumberAttempts is how many times Record lists the directory and tries to take the next number before it
// gives up. Each try that fails means another writer took that number a moment before; sixteen in a row is
// not a race.
const maxNumberAttempts = 16

// The wait between two tries: numberBackoff after the first lost race, doubled after each further one up to
// maxNumberBackoff. Nothing is waited before the first try or after the last, so the sixteen tries of a
// directory with no free number wait 513ms in all: long enough for the writers that collide to spread out,
// short enough that a person never sees it.
const (
	numberBackoff    = time.Millisecond
	maxNumberBackoff = 50 * time.Millisecond
)

// Service is the use cases of `engine review` over the ports.
type Service struct {
	git     Git
	results Results
	sleep   func(time.Duration)
}

// New is the Service over git and results. It never waits between two tries for a result number: the clock
// is not the application's to read (the architecture guard refuses time.Sleep in this ring), so the one who
// builds the Service hands it in with WithSleep.
func New(git Git, results Results) Service { return Service{git: git, results: results} }

// WithSleep is the Service that waits with sleep between two tries for a result number. A nil sleep is no
// waiting at all, as New makes it.
func (s Service) WithSleep(sleep func(time.Duration)) Service {
	s.sleep = sleep
	return s
}

// Bound is what Bind made: the candidate, and the worktree that is at its head.
type Bound struct {
	Candidate review.Candidate
	// Worktree is the absolute path of the detached worktree of the review.
	Worktree string
	// Created says that Bind made the worktree; false means it was there, at the head, and was reused.
	Created bool
}

// Bind computes the candidate of the diff from base to head, in the repository that holds dir (absolute),
// and makes sure the detached worktree of the review is at its head. base and head are revisions as a
// person types them; the candidate holds their full names. A diff over review.DiffLimitBytes is bound all
// the same (Candidate.OverLimit says so).
//
// The worktree is made last, once the candidate is bound, so a candidate that cannot be bound leaves no
// directory behind. One that is already there at the head is reused and changed in nothing. One at another
// commit (ErrWorktreeElsewhere), and anything else at that path (ErrWorktreeForeign), is refused and left as
// it is.
//
// Two binds of one head at the same moment can both find no worktree; the one whose add loses to the other's
// looks at the path once more, and when the worktree of the repository is there at the head it is reused
// (Created false) as if it had been there from the start. In any other case the error of the add is the
// answer.
func (s Service) Bind(dir, base, head string) (Bound, error) {
	repo, candidate, err := s.candidate(dir, base, head)
	if err != nil {
		return Bound{}, err
	}
	path := review.WorktreePath(repo.Main, candidate)
	state, err := s.git.Worktree(repo, path)
	if err != nil {
		return Bound{}, fmt.Errorf("inspect %s: %w", path, err)
	}
	switch {
	case !state.Present:
		if err := s.git.AddWorktree(repo, path, candidate.Head); err != nil {
			if s.isWorktreeAt(repo, path, candidate.Head) {
				return Bound{Candidate: candidate, Worktree: path}, nil
			}
			return Bound{}, fmt.Errorf("make the worktree %s: %w", path, err)
		}
		return Bound{Candidate: candidate, Worktree: path, Created: true}, nil
	case !state.OfRepository:
		return Bound{}, fmt.Errorf("%w: %s exists and is not a worktree of this repository; move it away or remove it", ErrWorktreeForeign, path)
	case state.Head != candidate.Head:
		return Bound{}, fmt.Errorf("%w: %s has %s checked out, not %s; remove it with 'git worktree remove %s' or check the head out there",
			ErrWorktreeElsewhere, path, state.Head, candidate.Head, path)
	}
	return Bound{Candidate: candidate, Worktree: path}, nil
}

// isWorktreeAt says whether what is at path is, now, a worktree of the repository with head checked out. Any
// other answer, and a path git cannot say anything about, is no.
func (s Service) isWorktreeAt(repo Repository, path, head string) bool {
	state, err := s.git.Worktree(repo, path)
	return err == nil && state.Present && state.OfRepository && state.Head == head
}

// Recorded is what Record stored.
type Recorded struct {
	Candidate review.Candidate
	// Path is where the result is stored, relative to the root of the working tree and written with '/'.
	Path string
	// Number is the launch it is for the head: the <n> of the file name.
	Number int
	// SHA256 is the digest of the bytes of the stored file.
	SHA256 string
	// Handoff is the skeleton of the record of the role chain that closes the review.
	Handoff roles.RoleHandoff
}

// Record holds the reply of a reviewer to the candidate of the diff from base to head, recomputed from git
// in the repository that holds dir (absolute) and not taken from the reply, and stores it. The file is the
// reply with the binding it was held to, in the next free number of the head in the results directory of the
// feature; no file is replaced. A reply that does not hold, or is too large, is refused with a
// *ReplyRefusedError, the named error inside it, and nothing is written.
func (s Service) Record(dir, feature, base, head string, reply []byte) (Recorded, error) {
	if err := review.CheckFeature(feature); err != nil {
		return Recorded{}, err
	}
	if len(reply) > review.MaxReplyBytes {
		return Recorded{}, &ReplyRefusedError{Err: fmt.Errorf("%w: %d bytes, the maximum is %d", review.ErrReplyTooLarge, len(reply), review.MaxReplyBytes)}
	}
	repo, candidate, err := s.candidate(dir, base, head)
	if err != nil {
		return Recorded{}, err
	}
	if _, err := reviewresult.Parse(reply, candidate.Binding()); err != nil {
		return Recorded{}, &ReplyRefusedError{Err: err}
	}
	file, err := review.EncodeResult(candidate, reply)
	if err != nil {
		return Recorded{}, err
	}
	number, name, err := s.store(repo.Root, feature, candidate, file)
	if err != nil {
		return Recorded{}, err
	}
	path, sum := review.ResultPath(feature, name), sha256hex.Sum(file)
	return Recorded{Candidate: candidate, Path: path, Number: number, SHA256: sum, Handoff: review.Handoff(candidate, path, sum)}, nil
}

// store takes the next free number of the head in the directory of the feature. A name that is taken by a
// writer that raced this one is not an error: after a short wait, longer each time, it lists the directory
// again and takes the next number, up to maxNumberAttempts tries.
func (s Service) store(root, feature string, c review.Candidate, file []byte) (int, string, error) {
	wait := numberBackoff
	for attempt := 1; attempt <= maxNumberAttempts; attempt++ {
		names, err := s.results.Names(root, feature)
		if err != nil {
			return 0, "", fmt.Errorf("list %s/%s: %w", review.ResultsDir, feature, err)
		}
		number := review.NextNumber(names, c)
		name := review.ResultName(c, number)
		err = s.results.Create(root, feature, name, file)
		switch {
		case err == nil:
			return number, name, nil
		case !errors.Is(err, ErrExists):
			return 0, "", fmt.Errorf("store %s: %w", review.ResultPath(feature, name), err)
		}
		if attempt < maxNumberAttempts {
			s.pause(wait)
			wait = min(wait*2, maxNumberBackoff)
		}
	}
	return 0, "", fmt.Errorf("%w: %d tries in %s/%s", ErrNoFreeNumber, maxNumberAttempts, review.ResultsDir, feature)
}

// pause waits for d with the clock the Service was given, and not at all when it was given none.
func (s Service) pause(d time.Duration) {
	if s.sleep != nil {
		s.sleep(d)
	}
}

// candidate locates the repository that holds dir and binds the diff from base to head, both revisions as
// a person types them. A revision git could read as an option is refused before git is asked anything.
func (s Service) candidate(dir, base, head string) (Repository, review.Candidate, error) {
	for _, rev := range []string{base, head} {
		if err := review.CheckRev(rev); err != nil {
			return Repository{}, review.Candidate{}, err
		}
	}
	repo, err := s.git.Locate(dir)
	if err != nil {
		return Repository{}, review.Candidate{}, fmt.Errorf("locate the repository of %s: %w", dir, err)
	}
	baseName, err := s.git.Commit(repo, base)
	if err != nil {
		return Repository{}, review.Candidate{}, fmt.Errorf("resolve base %q: %w", base, err)
	}
	headName, err := s.git.Commit(repo, head)
	if err != nil {
		return Repository{}, review.Candidate{}, fmt.Errorf("resolve head %q: %w", head, err)
	}
	diff, err := s.git.Diff(repo, baseName, headName)
	if err != nil {
		return Repository{}, review.Candidate{}, fmt.Errorf("diff %s..%s: %w", baseName, headName, err)
	}
	paths, err := s.git.Paths(repo, baseName, headName)
	if err != nil {
		return Repository{}, review.Candidate{}, fmt.Errorf("list the paths of %s..%s: %w", baseName, headName, err)
	}
	c, err := review.NewCandidate(baseName, headName, diff, paths)
	if err != nil {
		return Repository{}, review.Candidate{}, err
	}
	return repo, c, nil
}
