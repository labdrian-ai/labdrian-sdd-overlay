package review

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The reasons the checks of a launch refuse (the first one of the verb `review launch`). An error they return
// wraps exactly one of them.
var (
	// ErrInvalidModel is a model that is empty, that starts with '-' (so codex could read it as an option) or
	// that holds a control character.
	ErrInvalidModel = errors.New("review: invalid model")
	// ErrOverLimit is a candidate whose diff is over DiffLimitBytes, which one launch does not take whole.
	ErrOverLimit = errors.New("review: diff over the limit of one launch")
	// ErrInvalidBrief is a brief that is empty, not valid UTF-8, too large, or not the brief of the candidate.
	ErrInvalidBrief = errors.New("review: invalid brief")
)

// CheckModel refuses a model name the launcher would hand to codex as an argument: an empty one, one that starts
// with '-' (codex would read it as an option) and one that holds a control character (a line break, a NUL).
// The error wraps ErrInvalidModel.
func CheckModel(model string) error {
	switch {
	case model == "":
		return fmt.Errorf("%w: the model is empty", ErrInvalidModel)
	case strings.HasPrefix(model, "-"):
		return fmt.Errorf("%w: %q starts with '-', so codex could read it as an option", ErrInvalidModel, model)
	case strings.IndexFunc(model, unicode.IsControl) >= 0:
		return fmt.Errorf("%w: %q holds a control character", ErrInvalidModel, model)
	}
	return nil
}

// CheckBrief refuses a brief that is not the brief of the candidate c reviewed in worktree: one that is empty, that
// is not valid UTF-8, that is over MaxReplyBytes, or that does not hold, as text, the base, the head, the
// diff_sha256 and the worktree path of the binding, the first of them it lacks, in that order, being the one the
// error names. The check is textual: a brief that names the binding and asks something else passes it. The error
// wraps ErrInvalidBrief.
func CheckBrief(c Candidate, worktree string, brief []byte) error {
	switch {
	case len(brief) == 0:
		return fmt.Errorf("%w: the brief is empty", ErrInvalidBrief)
	case !utf8.Valid(brief):
		return fmt.Errorf("%w: the brief is not valid UTF-8", ErrInvalidBrief)
	case len(brief) > MaxReplyBytes:
		return fmt.Errorf("%w: the brief is %d bytes, the maximum is %d", ErrInvalidBrief, len(brief), MaxReplyBytes)
	}
	for _, part := range []struct{ name, value string }{
		{"base", c.Base}, {"head", c.Head}, {"diff_sha256", c.DiffSHA256}, {"worktree", worktree},
	} {
		if !bytes.Contains(brief, []byte(part.value)) {
			return reasoned{reason: ErrInvalidBrief, text: fmt.Sprintf("the brief does not name the %s %s of the binding, so it is not the brief of this candidate", part.name, part.value)}
		}
	}
	return nil
}

// OverLimitError is the refusal of a candidate over DiffLimitBytes, telling the parent to split it. Its text is
// the one sentence below and it wraps ErrOverLimit.
func OverLimitError(c Candidate) error {
	return reasoned{reason: ErrOverLimit, text: fmt.Sprintf("the diff is %d bytes, over the %d-byte limit of one launch; split the candidate into one launch per commit (skills/role-chain/SKILL.md)", c.DiffBytes, DiffLimitBytes)}
}

// The members `review bind` prints the base and the head of a binding in, each one a full object name (40 or 64
// lowercase hexadecimal digits): `"base": "<name>"` and `"head": "<name>"`.
var (
	briefBase = regexp.MustCompile(`"base":\s*"([0-9a-f]{40}|[0-9a-f]{64})"`)
	briefHead = regexp.MustCompile(`"head":\s*"([0-9a-f]{40}|[0-9a-f]{64})"`)
)

// BriefPair is the one base and head a brief names, in the form `review bind` prints them: a `"base"` member and a
// `"head"` member, each with a full object name. A name repeated is the same name. It refuses, with an error that
// wraps ErrInvalidBrief, a brief that is empty, one that names no base or no head in that form, and one that names
// more than one base or more than one head, since it is then not the brief of one candidate.
func BriefPair(brief []byte) (base, head string, err error) {
	if len(brief) == 0 {
		return "", "", fmt.Errorf("%w: the brief is empty", ErrInvalidBrief)
	}
	bases, heads := distinctNames(briefBase, brief), distinctNames(briefHead, brief)
	switch {
	case len(bases) == 0 || len(heads) == 0:
		return "", "", fmt.Errorf(`%w: the brief names no base and head pair: it needs one "base" and one "head" member with a full object name, as review bind prints them`, ErrInvalidBrief)
	case len(bases) > 1 || len(heads) > 1:
		return "", "", fmt.Errorf("%w: the brief names more than one base and head pair, so it is not the brief of one candidate", ErrInvalidBrief)
	}
	return bases[0], heads[0], nil
}

// distinctNames is the first capture of every match of re in text, each name once, in the order they first appear.
func distinctNames(re *regexp.Regexp, text []byte) []string {
	var names []string
	for _, m := range re.FindAllSubmatch(text, -1) {
		if name := string(m[1]); !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

// reasoned is an error whose text is exactly text and that wraps reason, for the refusals whose words are
// fixed sentences a person reads as they are.
type reasoned struct {
	reason error
	text   string
}

func (e reasoned) Error() string { return e.text }
func (e reasoned) Unwrap() error { return e.reason }
