package review_test

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

// The feature is a directory name under odd/review-results/ that a command line gives, so it is one
// path segment of plain characters and nothing that leads anywhere else.
func TestCheckFeatureAcceptsTheNamesOfFeatureDocuments(t *testing.T) {
	for _, name := range []string{
		"own-rdd", "skill-lifecycle", "consolidation-phase9", "a", "Own-RDD", "v1.2_x", "9lives",
		strings.Repeat("f", 100),
	} {
		t.Run(name, func(t *testing.T) {
			if err := review.CheckFeature(name); err != nil {
				t.Errorf("CheckFeature(%q) error = %v, want nil", name, err)
			}
		})
	}
}

func TestCheckFeatureRefusesWhatIsNotASingleSafePathSegment(t *testing.T) {
	for _, tc := range []struct{ name, feature string }{
		{"nothing", ""},
		{"the current directory", "."},
		{"the parent directory", ".."},
		{"a way up", "../x"},
		{"a way up inside", "a/../b"},
		{"a separator", "a/b"},
		{"a trailing separator", "a/"},
		{"a windows separator", `a\b`},
		{"an absolute path", "/etc"},
		{"two dots inside a name", "a..b"},
		{"a hidden name", ".hidden"},
		{"a name that git or a shell reads as an option", "-rf"},
		{"a space", "a b"},
		{"a newline", "a\nb"},
		{"a NUL byte", "a\x00b"},
		{"a colon", "c:x"},
		{"a letter outside ASCII", "café"},
		{"a name past 100 bytes", strings.Repeat("f", 101)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := review.CheckFeature(tc.feature)
			if !errors.Is(err, review.ErrInvalidFeature) {
				t.Fatalf("CheckFeature(%q) error = %v, want one that is %v", tc.feature, err, review.ErrInvalidFeature)
			}
			if !strings.Contains(err.Error(), strconv.Quote(tc.feature)) {
				t.Errorf("CheckFeature(%q) error = %q, want it to name the feature as %s", tc.feature, err, strconv.Quote(tc.feature))
			}
		})
	}
}

// A revision is handed to git by the adapter, which cannot tell an option from a name: an option, or
// something that is not text, never reaches it.
func TestCheckRevAcceptsWhatNamesACommit(t *testing.T) {
	for _, rev := range []string{"HEAD", "main~2", "v1.0", "origin/main", "HEAD@{2 days ago}", baseSHA, "feat/own-rdd-t2^{commit}"} {
		if err := review.CheckRev(rev); err != nil {
			t.Errorf("CheckRev(%q) error = %v, want nil", rev, err)
		}
	}
}

func TestCheckRevRefusesWhatGitWouldReadAsAnOptionOrCannotReceive(t *testing.T) {
	for _, tc := range []struct{ name, rev string }{
		{"nothing", ""},
		{"an option", "-x"},
		{"an option that writes a file", "--output=/tmp/x"},
		{"a newline", "HEAD\n--output=x"},
		{"a NUL byte", "HEAD\x00"},
		{"a tab", "HEAD\t"},
		{"a DEL character", "HEAD\x7f"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := review.CheckRev(tc.rev)
			if !errors.Is(err, review.ErrInvalidRev) {
				t.Fatalf("CheckRev(%q) error = %v, want one that is %v", tc.rev, err, review.ErrInvalidRev)
			}
		})
	}
}

// The stored result of a launch is <head12>-<n>.json, in the directory of its feature.
func TestResultNameIsTheFirstTwelveCharactersOfTheHeadThenTheNumber(t *testing.T) {
	c := candidate(t)
	for _, tc := range []struct {
		n    int
		want string
	}{{1, "222222222222-1.json"}, {2, "222222222222-2.json"}, {12, "222222222222-12.json"}} {
		if got := review.ResultName(c, tc.n); got != tc.want {
			t.Errorf("ResultName(_, %d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestResultPathIsUnderTheResultsDirectoryOfTheFeature(t *testing.T) {
	if review.ResultsDir != "odd/review-results" {
		t.Fatalf("ResultsDir = %q, want %q", review.ResultsDir, "odd/review-results")
	}
	if got, want := review.ResultPath("own-rdd", "222222222222-1.json"), "odd/review-results/own-rdd/222222222222-1.json"; got != want {
		t.Errorf("ResultPath() = %q, want %q", got, want)
	}
}

// The number of a launch is the next one after the highest the head already has, from 1: a second
// blind launch of a head takes the next number, and a launch that was deleted never makes a number
// come back. A verifier report stored beside a result holds its number too.
func TestNextNumber(t *testing.T) {
	c := candidate(t)
	for _, tc := range []struct {
		name  string
		names []string
		want  int
	}{
		{"no file", nil, 1},
		{"a directory with other heads only", []string{"333333333333-1.json", "333333333333-2.json"}, 1},
		{"one result", []string{"222222222222-1.json"}, 2},
		{"two results", []string{"222222222222-1.json", "222222222222-2.json"}, 3},
		{"the highest, not the count", []string{"222222222222-3.json"}, 4},
		{"a number with two digits", []string{"222222222222-9.json", "222222222222-10.json"}, 11},
		{"a verifier report holds its number", []string{"222222222222-1.json", "222222222222-2.verifier.md"}, 3},
		{"other heads do not count", []string{"222222222222-1.json", "333333333333-7.json"}, 2},
		{"a name that is not a launch of the head", []string{"222222222222-x.json", "222222222222-.json", "222222222222.json", "222222222222-1x.json"}, 1},
		{"a number with a leading zero is not one the command wrote", []string{"222222222222-01.json", "222222222222-0.json"}, 1},
		{"a temporary file of a write in progress", []string{".review-123.tmp", "222222222222-1.json"}, 2},
		{"a number too large to count", []string{"222222222222-99999999999999999999999.json"}, 1},
		{"nine digits are a number the command could write", []string{"222222222222-999999999.json"}, 1000000000},
		{"ten digits are more than the command counts", []string{"222222222222-1000000000.json"}, 1},
		{"a name without the dot after the number", []string{"222222222222-3", "222222222222-2.json"}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := review.NextNumber(tc.names, c); got != tc.want {
				t.Errorf("NextNumber(%q) = %d, want %d", tc.names, got, tc.want)
			}
		})
	}
}

// The detached worktree of a review is a sibling of the directory of the main checkout, called
// <repo-name>-worktrees, and is named for the head. It is never inside the repository.
func TestWorktreePathIsInTheWorktreesDirectoryBesideTheMainCheckout(t *testing.T) {
	c := candidate(t)
	main := filepath.FromSlash("/home/someone/labdrian-sdd-overlay")
	want := filepath.FromSlash("/home/someone/labdrian-sdd-overlay-worktrees/review-222222222222")
	if got := review.WorktreePath(main, c); got != want {
		t.Errorf("WorktreePath(%q) = %q, want %q", main, got, want)
	}
	other := filepath.FromSlash("/tmp/x/repo")
	if got, want := review.WorktreePath(other, c), filepath.FromSlash("/tmp/x/repo-worktrees/review-222222222222"); got != want {
		t.Errorf("WorktreePath(%q) = %q, want %q", other, got, want)
	}
}
