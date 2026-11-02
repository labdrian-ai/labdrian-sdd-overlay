package app

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
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

// WithFrozenContext is the Service whose Launch hands the reviewer, after the brief, the function-context diff of the
// candidate (C25 T3, S3 a): for a reviewer with no tool, the whole of what it can inspect. Without it Launch hands the
// brief alone.
func (s Service) WithFrozenContext() Service {
	s.frozenContext = true
	return s
}

// Launch binds the diff from base to head in the repository that holds dir (Bind, which makes or reuses the detached
// worktree), starts the reviewer with model in that worktree over brief, and records its reply (Record) over the
// full names of the bound candidate and rows. Everything that can be refused is refused before the reviewer starts, in
// this order: the feature, the rows, the model, the binding, a diff over review.DiffLimitBytes, a brief that is not
// the brief of the candidate, and, with WithFrozenContext, a brief and context over review.MaxReplyBytes. An error of
// the reviewer is returned as it is and nothing is stored; a reply Record refuses for its shape is asked for again
// once (reAsk), and a reply Record refuses after that, or for anything else, is the *ReplyRefusedError of Record; a
// reviewer that does not answer the re-ask leaves the first refusal.
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
	if s.frozenContext {
		if brief, err = s.withContext(dir, c, brief); err != nil {
			return Recorded{}, err
		}
	}
	reply, err := s.reviewer.Review(bound.Worktree, model, brief)
	if err != nil {
		return Recorded{}, err
	}
	recorded, refusal := s.Record(dir, feature, c.Base, c.Head, rows, reply)
	again, ok := reAsk(brief, refusal)
	if !ok {
		return recorded, refusal
	}
	if reply, err = s.reviewer.Review(bound.Worktree, model, again); err != nil {
		// A reviewer that does not answer the re-ask leaves the first refusal as the outcome of the launch.
		return Recorded{}, refusal
	}
	return s.Record(dir, feature, c.Base, c.Head, rows, reply)
}

// maxQuotedRefusal bounds the refusal a re-ask quotes: its text can echo a member name of the reply, of any length.
const maxQuotedRefusal = 1024

// shapeRefusals are the refusals of a reply that is not in the shape of the contract, which a reviewer asked again
// can mend: a member missing, unknown or of the wrong type, an id missing or repeated, a value outside its
// enumeration, a severe finding with no proof. A reply refused for its binding or its inspection is not asked again.
var shapeRefusals = []error{
	reviewresult.ErrUnparseable, reviewresult.ErrUnknownMember, reviewresult.ErrMissingID,
	reviewresult.ErrDuplicateID, reviewresult.ErrOutsideEnum, reviewresult.ErrMissingProof,
}

// reAsk is the brief of the one time Launch asks the reviewer again (phase9-c25-review-hunt S7): brief followed by a
// heading that quotes the refusal err of the first reply, its first maxQuotedRefusal bytes. It is false when err is not
// a *ReplyRefusedError of a shapeRefusals reason, or when the re-ask would be over review.MaxReplyBytes.
func reAsk(brief []byte, err error) ([]byte, bool) {
	var refused *ReplyRefusedError
	if !errors.As(err, &refused) || !slices.ContainsFunc(shapeRefusals, func(reason error) bool { return errors.Is(refused.Err, reason) }) {
		return nil, false
	}
	quoted := refused.Err.Error()
	if len(quoted) > maxQuotedRefusal {
		quoted = strings.ToValidUTF8(quoted[:maxQuotedRefusal], "") + " ..."
	}
	again := fmt.Sprintf("%s\n\n## Your previous reply was refused\n\n%s\n\nReply again with the whole JSON object of the contract: every member of every object present, none null.\n", brief, quoted)
	if len(again) > review.MaxReplyBytes {
		return nil, false
	}
	return []byte(again), true
}

// withContext is brief followed by a heading that names the base and the head of c and their function-context diff in
// one fenced block, whose fence is longer than any run of backticks in the diff. Brief and context together over
// review.MaxReplyBytes are refused with an error that wraps review.ErrInvalidBrief.
func (s Service) withContext(dir string, c review.Candidate, brief []byte) ([]byte, error) {
	repo, err := s.git.Locate(dir)
	if err != nil {
		return nil, err
	}
	diff, err := s.git.FunctionContextDiff(repo, c.Base, c.Head)
	if err != nil {
		return nil, fmt.Errorf("read the frozen context: %w", err)
	}
	fence := strings.Repeat("`", max(3, longestBacktickRun(diff)+1))
	var b strings.Builder
	b.Write(brief)
	fmt.Fprintf(&b, "\n\n## Frozen context (base %s, head %s)\n\n%sdiff\n", c.Base, c.Head, fence)
	b.Write(diff)
	if len(diff) > 0 && diff[len(diff)-1] != '\n' {
		b.WriteByte('\n')
	}
	b.WriteString(fence + "\n")
	if b.Len() > review.MaxReplyBytes {
		return nil, fmt.Errorf("%w: the brief and its frozen context are %d bytes, the maximum is %d", review.ErrInvalidBrief, b.Len(), review.MaxReplyBytes)
	}
	return []byte(b.String()), nil
}

// longestBacktickRun is the length of the longest run of backticks in text.
func longestBacktickRun(text []byte) int {
	longest, run := 0, 0
	for _, ch := range text {
		if ch != '`' {
			run = 0
			continue
		}
		run++
		longest = max(longest, run)
	}
	return longest
}
