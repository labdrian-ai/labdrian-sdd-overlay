package review_test

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
)

// A candidate is two commits, the digest and the size of the bytes of their diff, and the paths the diff
// changes, in the order git gave them. The digest is the SHA-256 of the bytes it is handed and nothing
// else; the git that makes those bytes is the adapter's.
func TestNewCandidateBindsTheDiffByItsDigestAndItsSize(t *testing.T) {
	got, err := review.NewCandidate(baseSHA, headSHA, []byte(diffText), changedPaths())
	if err != nil {
		t.Fatalf("NewCandidate() error = %v, want nil", err)
	}
	want := review.Candidate{Base: baseSHA, Head: headSHA, DiffSHA256: diffSHA, DiffBytes: 3, Paths: changedPaths()}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NewCandidate() = %+v, want %+v", got, want)
	}
}

// A repository that hashes with SHA-256 names its commits with 64 characters; both are then 64.
func TestNewCandidateAcceptsTheFullNamesOfASHA256Repository(t *testing.T) {
	base, head := sha256Names()
	got, err := review.NewCandidate(base, head, []byte(diffText), nil)
	if err != nil {
		t.Fatalf("NewCandidate(64-character names) error = %v, want nil", err)
	}
	if got.Base != base || got.Head != head {
		t.Errorf("NewCandidate() names %s and %s, want %s and %s", got.Base, got.Head, base, head)
	}
}

// A diff that changes no path has an empty list, which is written as an array and not as null: the
// binding of a reviewer prompt and the stored file both say "no path" with [].
func TestNewCandidateKeepsAnEmptyListOfPathsEmptyAndNotNil(t *testing.T) {
	got, err := review.NewCandidate(baseSHA, headSHA, nil, nil)
	if err != nil {
		t.Fatalf("NewCandidate(no diff, no paths) error = %v, want nil", err)
	}
	if got.Paths == nil || len(got.Paths) != 0 {
		t.Errorf("Paths = %#v, want an empty non-nil list", got.Paths)
	}
	if got.DiffBytes != 0 || got.DiffSHA256 != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("an empty diff has size %d and digest %s, want 0 and the digest of no bytes", got.DiffBytes, got.DiffSHA256)
	}
}

// Everything a reviewer is held to is the candidate, so a candidate that names nothing, or names a path
// that JSON cannot carry, is refused before any reviewer is launched on it.
func TestNewCandidateRefusesAnythingThatDoesNotNameACandidate(t *testing.T) {
	mixedBase, mixedHead := baseSHA, strings.Repeat("b", 64)
	upperBase, upperHead := strings.Repeat("A", 40), strings.Repeat("B", 40)
	// The mention of a commit that is not a full name is the commit with its value, so that the refusal of the
	// head is told from the refusal of two names of different lengths, which also says "head".
	for _, tc := range []struct {
		name       string
		base, head string
		paths      []string
		mention    string
	}{
		{"a base that is an abbreviation", "1111111", headSHA, nil, `base "1111111" is not a full object name`},
		{"a head that is an abbreviation", baseSHA, "2222222", nil, `head "2222222" is not a full object name`},
		{"a base in upper case", upperBase, headSHA, nil, `base "` + upperBase + `" is not a full object name`},
		{"a head in upper case, as long as the base", baseSHA, upperHead, nil, `head "` + upperHead + `" is not a full object name`},
		{"a head that is empty", baseSHA, "", nil, `head "" is not a full object name`},
		{"a base of 40 characters and a head of 64", mixedBase, mixedHead, nil, "the same length"},
		{"an empty path", baseSHA, headSHA, []string{"a.go", ""}, "empty path"},
		{"a path listed twice", baseSHA, headSHA, []string{"a.go", "b.go", "a.go"}, `"a.go"`},
		{"a path that is not UTF-8", baseSHA, headSHA, []string{"caf\xe9.txt"}, "UTF-8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := review.NewCandidate(tc.base, tc.head, []byte(diffText), tc.paths)
			if !errors.Is(err, review.ErrInvalidCandidate) {
				t.Fatalf("NewCandidate() error = %v, want one that is %v", err, review.ErrInvalidCandidate)
			}
			if !strings.Contains(err.Error(), tc.mention) {
				t.Errorf("NewCandidate() error = %q, want it to mention %q", err, tc.mention)
			}
			if !reflect.DeepEqual(got, review.Candidate{}) {
				t.Errorf("NewCandidate() = %+v alongside an error, want the zero Candidate", got)
			}
		})
	}
}

// A reviewer is held to the binding of the candidate as reviewresult states it: the same four values.
func TestCandidateBindingIsWhatAReviewerReplyIsHeldTo(t *testing.T) {
	want := reviewresult.Binding{Base: baseSHA, Head: headSHA, DiffSHA256: diffSHA, Paths: changedPaths()}
	if got := candidate(t).Binding(); !reflect.DeepEqual(got, want) {
		t.Errorf("Binding() = %+v, want %+v", got, want)
	}
}

// The record of the chain names the range as <base>..<head> (the ref of its review-diff evidence), and
// the files of a head are named by its first twelve characters.
func TestCandidateRefAndShortName(t *testing.T) {
	c := candidate(t)
	if got, want := c.Ref(), baseSHA+".."+headSHA; got != want {
		t.Errorf("Ref() = %q, want %q", got, want)
	}
	if got, want := c.Short(), "222222222222"; got != want {
		t.Errorf("Short() = %q, want %q", got, want)
	}
}

// The commands document the limit of a reply as 1 MiB (README.md and odd/review-results/README.md), so the
// constant is held to the number they print.
func TestTheLimitOfAReplyIsTheOneMebibyteTheDocumentationStates(t *testing.T) {
	if review.MaxReplyBytes != 1<<20 {
		t.Fatalf("MaxReplyBytes = %d, want %d (1 MiB, as README.md and odd/review-results/README.md say)", review.MaxReplyBytes, 1<<20)
	}
}

// A diff is sent to a reviewer whole, up to 150 KB, which is 150,000 bytes; above that the parent
// launches one reviewer per commit. The limit is part of the binding and does not make it fail.
func TestOverLimitIsTrueOnlyAboveOneHundredFiftyThousandBytes(t *testing.T) {
	if review.DiffLimitBytes != 150000 {
		t.Fatalf("DiffLimitBytes = %d, want 150000 (150 KB, as skills/role-chain/SKILL.md and the README say)", review.DiffLimitBytes)
	}
	for _, tc := range []struct {
		size int
		want bool
	}{{0, false}, {149999, false}, {150000, false}, {150001, true}} {
		diff := bytes.Repeat([]byte("x"), tc.size)
		c, err := review.NewCandidate(baseSHA, headSHA, diff, nil)
		if err != nil {
			t.Fatalf("NewCandidate(%d bytes) error = %v, want nil", tc.size, err)
		}
		if c.DiffBytes != tc.size {
			t.Errorf("DiffBytes = %d, want %d", c.DiffBytes, tc.size)
		}
		if got := c.OverLimit(); got != tc.want {
			t.Errorf("OverLimit() for %d bytes = %v, want %v", tc.size, got, tc.want)
		}
	}
}

// The zero Candidate names nothing: its short name is empty instead of a panic, for a caller that used
// the zero value NewCandidate returns with its error.
func TestTheZeroCandidateHasAnEmptyShortName(t *testing.T) {
	if got := (review.Candidate{}).Short(); got != "" {
		t.Errorf("Candidate{}.Short() = %q, want \"\"", got)
	}
}
