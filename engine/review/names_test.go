package review_test

import (
	"errors"
	"path/filepath"
	"slices"
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

// The sweep, the sweeper and the polisher store a record per head beside the results (own-rdd T6b2, S6 c): the
// ending is the only thing that tells the kinds of file apart, and each ending is one the others do not end in.
func TestRoleRecordNamesAreTheHeadTheNumberAndTheEnding(t *testing.T) {
	c := candidate(t)
	for _, tc := range []struct{ kind, got, want string }{
		{"sweep", review.SweepName(c, 3), "222222222222-3.sweep.json"},
		{"sweeper", review.SweeperName(c, 3), "222222222222-3.sweeper.json"},
		{"polisher", review.PolisherName(c, 12), "222222222222-12.polisher.json"},
	} {
		if tc.got != tc.want {
			t.Errorf("%sName = %q, want %q", tc.kind, tc.got, tc.want)
		}
	}
}

func TestEachRoleRecordCheckAcceptsItsOwnNameOnly(t *testing.T) {
	names := map[string]string{
		"sweep":    "222222222222-1.sweep.json",
		"sweeper":  "222222222222-1.sweeper.json",
		"polisher": "222222222222-1.polisher.json",
	}
	checks := map[string]func(string) error{
		"sweep": review.CheckSweepName, "sweeper": review.CheckSweeperName, "polisher": review.CheckPolisherName,
	}
	errs := map[string]error{
		"sweep": review.ErrInvalidSweepName, "sweeper": review.ErrInvalidSweeperName, "polisher": review.ErrInvalidPolisherName,
	}
	for kind, check := range checks {
		for other, name := range names {
			err := check(name)
			if kind == other {
				if err != nil {
					t.Errorf("Check%sName(%q) error = %v, want nil", kind, name, err)
				}
				continue
			}
			if !errors.Is(err, errs[kind]) {
				t.Errorf("Check%sName(%q) error = %v, want one that is %v", kind, name, err, errs[kind])
			}
		}
		for _, bad := range []string{"222222222222-1.json", "222222222222-1.exempt.json", "222222222222-01." + kind + ".json", "22222222222-1." + kind + ".json", "dir/222222222222-1." + kind + ".json"} {
			if err := check(bad); !errors.Is(err, errs[kind]) {
				t.Errorf("Check%sName(%q) error = %v, want one that is %v", kind, bad, err, errs[kind])
			}
		}
	}
}

func TestRoleRecordsOfAHeadAreThoseOfThatHeadByLaunchNumber(t *testing.T) {
	names := []string{
		"222222222222-10.sweep.json", "222222222222-2.sweep.json", "111111111111-1.sweep.json", "222222222222-1.json",
		"222222222222-3.sweeper.json", "222222222222-1.sweeper.json", "222222222222-01.sweeper.json",
		"222222222222-2.polisher.json", "222222222222-1.polisher.json", "222222222222-1.polisher.json.tmp",
	}
	for _, tc := range []struct {
		kind      string
		got, want []string
	}{
		{"sweeps", review.SweepsOfHead(names, headSHA), []string{"222222222222-2.sweep.json", "222222222222-10.sweep.json"}},
		{"sweepers", review.SweepersOfHead(names, headSHA), []string{"222222222222-1.sweeper.json", "222222222222-3.sweeper.json"}},
		{"polishers", review.PolishersOfHead(names, headSHA), []string{"222222222222-1.polisher.json", "222222222222-2.polisher.json"}},
	} {
		if !slices.Equal(tc.got, tc.want) {
			t.Errorf("%sOfHead = %q, want %q", tc.kind, tc.got, tc.want)
		}
	}
}

// The numbers are shared: a head with a result, a verdict, an exemption, a sweep, a sweeper and a polisher record
// takes the next number after the highest of them, so no record is given a number another one holds.
func TestNextNumberCountsTheRoleRecords(t *testing.T) {
	c := candidate(t)
	for _, tc := range []struct {
		name  string
		names []string
		want  int
	}{
		{"a sweep", []string{"222222222222-4.sweep.json"}, 5},
		{"a sweeper", []string{"222222222222-6.sweeper.json"}, 7},
		{"a polisher", []string{"222222222222-8.polisher.json"}, 9},
		{"every kind", []string{"222222222222-1.json", "222222222222-2.verifier.json", "222222222222-3.exempt.json", "222222222222-4.sweep.json", "222222222222-5.sweeper.json", "222222222222-6.polisher.json"}, 7},
		{"another head's records", []string{"333333333333-9.sweep.json"}, 1},
	} {
		if got := review.NextNumber(tc.names, c); got != tc.want {
			t.Errorf("%s: NextNumber(%q) = %d, want %d", tc.name, tc.names, got, tc.want)
		}
	}
}

// What a role record is not: a result. The existing readers of results and exemptions keep reading only their own.
func TestResultReadersNeverTakeARoleRecordForAResult(t *testing.T) {
	records := []string{"222222222222-1.sweep.json", "222222222222-1.sweeper.json", "222222222222-1.polisher.json"}
	for _, name := range records {
		if err := review.CheckResultName(name); !errors.Is(err, review.ErrInvalidResultName) {
			t.Errorf("CheckResultName(%q) error = %v, want one that is %v", name, err, review.ErrInvalidResultName)
		}
		if err := review.CheckExemptionName(name); !errors.Is(err, review.ErrInvalidExemptionName) {
			t.Errorf("CheckExemptionName(%q) error = %v, want one that is %v", name, err, review.ErrInvalidExemptionName)
		}
	}
	names := append([]string{"222222222222-1.json", "222222222222-2.exempt.json"}, records...)
	if got, want := review.ResultsOfHead(names, headSHA), []string{"222222222222-1.json"}; !slices.Equal(got, want) {
		t.Errorf("ResultsOfHead = %q, want %q", got, want)
	}
	if got, want := review.ExemptionsOfHead(names, headSHA), []string{"222222222222-2.exempt.json"}; !slices.Equal(got, want) {
		t.Errorf("ExemptionsOfHead = %q, want %q", got, want)
	}
}
