package review_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

// An exemption binds exactly one commit (own-rdd T4, S5, L21): the base of its range is the first parent of its head,
// the parent git gives, "" when the head has none. Anything else is refused with an error that wraps ErrNotOneCommit
// and says both commits, so that a chain from a delivery base meets every commit an exemption stands for.
func TestCheckOneCommitAcceptsOnlyTheFirstParentOfTheHeadAsTheBase(t *testing.T) {
	older := strings.Repeat("3", 40)

	if err := review.CheckOneCommit(baseSHA, headSHA, baseSHA); err != nil {
		t.Errorf("CheckOneCommit() of the first parent as the base: error = %v, want nil", err)
	}

	for _, tc := range []struct{ name, base, parent, say string }{
		{"the base is further back", older, baseSHA, "the base is " + older + " and the first parent of " + headSHA + " is " + baseSHA},
		{"the head is the base", headSHA, baseSHA, "the base is " + headSHA + " and the first parent of " + headSHA + " is " + baseSHA},
		{"the head has no parent", baseSHA, "", headSHA + " has no parent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := review.CheckOneCommit(tc.base, headSHA, tc.parent)

			if !errors.Is(err, review.ErrNotOneCommit) || !strings.Contains(err.Error(), tc.say) {
				t.Errorf("CheckOneCommit() error = %v, want one that wraps ErrNotOneCommit and says %q", err, tc.say)
			}
		})
	}
}
