package review_test

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

// The candidate every test binds: two full commit names and the diff between them. The digest of the
// diff below is the SHA-256 of the three bytes "abc", written out so that no test computes what it
// checks.
const (
	baseSHA = "1111111111111111111111111111111111111111"
	headSHA = "2222222222222222222222222222222222222222"
	// diffSHA is the SHA-256 of diffText.
	diffText = "abc"
	diffSHA  = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
)

// changedPaths are the paths of the diff, in the order git prints them.
func changedPaths() []string { return []string{"engine/guard.go", "engine/guard_test.go"} }

// candidate is the candidate of the constants above.
func candidate(t *testing.T) review.Candidate {
	t.Helper()
	c, err := review.NewCandidate(baseSHA, headSHA, []byte(diffText), changedPaths())
	if err != nil {
		t.Fatalf("setup: NewCandidate(%s, %s) error = %v, want nil", baseSHA, headSHA, err)
	}
	return c
}

// sha256Names are the full object names of a SHA-256 repository: 64 hexadecimal characters.
func sha256Names() (base, head string) {
	return strings.Repeat("a", 64), strings.Repeat("b", 64)
}
