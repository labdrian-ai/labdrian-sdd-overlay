package app

import (
	"errors"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewverdict"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

var (
	// ErrAlreadyVerified is a result that already has a verdict stored beside it.
	ErrAlreadyVerified = errors.New("review: the result already has a verdict")
	// ErrCorrectionSpent is a verdict that blocks a candidate whose correction round was spent: the review
	// of another head of the same base was blocked by a stored verdict, and the builder was sent back once.
	ErrCorrectionSpent = errors.New("review: the correction round of the candidate was spent")
)

// Verified is what Verify stored.
type Verified struct {
	// Path is where the verdict is stored, relative to the root of the working tree and written with '/'.
	Path string
	// SHA256 is the digest of the bytes of the stored verdict.
	SHA256 string
	// Decision is what the verdicts make of the findings of the result: whether the review blocks.
	Decision reviewverdict.Decision
	// Handoff is the skeleton of the record of the role chain that closes the review: the one Record printed
	// for the result, with the verdict bound as evidence and who is next filled from the decision.
	Handoff roles.RoleHandoff
	// NotFlushed, when it is not nil, wraps ErrNotFlushed: the verdict is stored, but the directory could not
	// be flushed after it, so it may not survive a crash of the machine. The verdict is not stored again.
	NotFlushed error
}

// Verify holds the reply of a verifier to the stored result called stored, in the directory of the feature
// of the repository that holds dir (absolute), decides from the two whether the review blocks, and stores the
// reply beside the result, under the same <head12>-<n> and the ending .verifier.json. The candidate is the
// one the result was stored with and not one computed again: the commits are named in full, so what the
// verifier checked is what the result says it was asked about. The stored result is read as Record wrote it
// and held to its binding again; one that is not a result, or whose reply no longer holds, is refused.
//
// A reply that does not hold to the contract of reviewverdict, or is too large, is refused with a
// *ReplyRefusedError, and nothing is stored. A result has one verdict: a verdict that is already there is
// ErrAlreadyVerified and stays as it is.
//
// A candidate is corrected at most once. A verdict that blocks sends the review back to the builder, and the
// fix is reviewed against the same base; a verdict that blocks on another head of a base whose stored
// verdicts already hold one that blocked (review.IsCorrection) is ErrCorrectionSpent, and nothing is stored.
// The round is recorded by the stored verdict itself: no other file says it, and no counter can drift from
// the verdicts. What cannot be read to tell is not taken to mean the round is unspent. A verdict that blocks
// is checked and stored under the lock of the directory of the feature (Results.Lock), so two of them on two
// heads of one base, verified at the same time, cannot both find the round unspent.
func (s Service) Verify(dir, feature, stored string, reply []byte) (Verified, error) {
	if err := review.CheckFeature(feature); err != nil {
		return Verified{}, err
	}
	verdictName, err := review.VerdictName(stored)
	if err != nil {
		return Verified{}, err
	}
	if len(reply) > review.MaxReplyBytes {
		return Verified{}, &ReplyRefusedError{Err: fmt.Errorf("%w: %d bytes, the maximum is %d", review.ErrReplyTooLarge, len(reply), review.MaxReplyBytes)}
	}
	repo, err := s.git.Locate(dir)
	if err != nil {
		return Verified{}, fmt.Errorf("locate the repository of %s: %w", dir, err)
	}
	resultPath := review.ResultPath(feature, stored)
	resultFile, err := s.results.Read(repo.Root, feature, stored)
	if err != nil {
		return Verified{}, fmt.Errorf("read %s: %w", resultPath, err)
	}
	binding, result, err := judged(resultFile)
	if err != nil {
		return Verified{}, fmt.Errorf("%s: %w", resultPath, err)
	}
	verdicts, err := reviewverdict.Parse(reply, result)
	if err != nil {
		return Verified{}, &ReplyRefusedError{Err: err}
	}
	decision := reviewverdict.Decide(result, verdicts)
	if decision.Outcome == reviewverdict.Blocks {
		unlock, err := s.results.Lock(repo.Root, feature)
		if err != nil {
			return Verified{}, fmt.Errorf("lock %s/%s: %w", review.ResultsDir, feature, err)
		}
		defer unlock()
		spentBy, err := s.spentRound(repo.Root, feature, stored, binding)
		if err != nil {
			return Verified{}, err
		}
		if spentBy != "" {
			return Verified{}, fmt.Errorf("%w: %s blocked another head of the same base and the builder was sent back once; this verdict blocks again, so nothing was stored and the owner decides",
				ErrCorrectionSpent, review.ResultPath(feature, spentBy))
		}
	}
	file, err := review.EncodeVerdict(reply)
	if err != nil {
		return Verified{}, err
	}
	verdictPath := review.ResultPath(feature, verdictName)
	var notFlushed error
	switch err := s.results.Create(repo.Root, feature, verdictName, file); {
	case err == nil:
	case errors.Is(err, ErrNotFlushed):
		notFlushed = err
	case errors.Is(err, ErrExists):
		return Verified{}, fmt.Errorf("%w: %s", ErrAlreadyVerified, verdictPath)
	default:
		return Verified{}, fmt.Errorf("store %s: %w", verdictPath, err)
	}
	sum := sha256hex.Sum(file)
	handoff := review.VerifiedHandoff(binding, resultPath, sha256hex.Sum(resultFile), verdictPath, sum, decision.Outcome)
	return Verified{Path: verdictPath, SHA256: sum, Decision: decision, Handoff: handoff, NotFlushed: notFlushed}, nil
}

// spentRound is the name of the stored verdict that spent the correction round of the candidate that binding
// names: one that decides to block and is for another head of the same base. It is "" when none did. Every
// verdict of the feature is read and decided again from its own result, so the answer is the one the stored
// files give and not a record kept beside them. The verdict of stored itself is left out, for it is the one
// being made.
func (s Service) spentRound(root, feature, stored string, binding reviewresult.Binding) (string, error) {
	names, err := s.results.Names(root, feature)
	if err != nil {
		return "", fmt.Errorf("list %s/%s: %w", review.ResultsDir, feature, err)
	}
	for _, name := range names {
		result, isVerdict := review.ResultOfVerdict(name)
		if !isVerdict || result == stored {
			continue
		}
		earlier, decision, err := s.decided(root, feature, result, name)
		if err != nil {
			return "", fmt.Errorf("cannot tell whether the correction round was spent: %s: %w", review.ResultPath(feature, name), err)
		}
		if decision.Outcome == reviewverdict.Blocks && review.IsCorrection(earlier, binding) {
			return name, nil
		}
	}
	return "", nil
}

// decided reads the stored result and the stored verdict beside it, and decides again from the two: the
// binding of the result and the decision of its verdict.
func (s Service) decided(root, feature, result, verdict string) (reviewresult.Binding, reviewverdict.Decision, error) {
	resultFile, err := s.results.Read(root, feature, result)
	if err != nil {
		return reviewresult.Binding{}, reviewverdict.Decision{}, fmt.Errorf("read %s: %w", result, err)
	}
	verdictFile, err := s.results.Read(root, feature, verdict)
	if err != nil {
		return reviewresult.Binding{}, reviewverdict.Decision{}, fmt.Errorf("read %s: %w", verdict, err)
	}
	binding, parsed, err := judged(resultFile)
	if err != nil {
		return reviewresult.Binding{}, reviewverdict.Decision{}, fmt.Errorf("%s: %w", result, err)
	}
	verdicts, err := reviewverdict.Parse(verdictFile, parsed)
	if err != nil {
		return reviewresult.Binding{}, reviewverdict.Decision{}, err
	}
	return binding, reviewverdict.Decide(parsed, verdicts), nil
}

// judged reads a stored result and holds its reply to the binding it was stored with, as Record did. A file
// that is not a result, or whose reply no longer holds, is review.ErrInvalidStored.
func judged(file []byte) (reviewresult.Binding, reviewresult.Result, error) {
	stored, err := review.DecodeResult(file)
	if err != nil {
		return reviewresult.Binding{}, reviewresult.Result{}, err
	}
	result, err := reviewresult.Parse(stored.Reply, stored.Binding)
	if err != nil {
		return reviewresult.Binding{}, reviewresult.Result{}, fmt.Errorf("%w: its reply does not hold to its binding: %w", review.ErrInvalidStored, err)
	}
	return stored.Binding, result, nil
}
