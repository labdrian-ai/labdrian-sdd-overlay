package review

import (
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// maxFeatureBytes bounds the name of a feature: a directory name well inside the 255 bytes a file system
// allows, with room for nothing else being added to it.
const maxFeatureBytes = 100

// CheckFeature refuses a feature that is not a single safe path segment, and returns nil for one that is:
// 1 to 100 characters, each a letter or digit of ASCII, '.', '_' or '-', starting with a letter or a digit,
// and holding no "..". That is every name the feature documents of odd/tasks/ have, and nothing that is a
// separator, a way up, an absolute path, a hidden or option-like name, or a character a shell or a
// terminal reads as something other than itself. A name it refuses is named in the error, which wraps
// ErrInvalidFeature.
func CheckFeature(name string) error {
	refuse := func(why string) error {
		return fmt.Errorf("%w: %q is not a single safe path segment: %s", ErrInvalidFeature, name, why)
	}
	switch {
	case name == "":
		return refuse("it is empty")
	case len(name) > maxFeatureBytes:
		return refuse(fmt.Sprintf("it is longer than %d bytes", maxFeatureBytes))
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !isAlnum(c) && c != '.' && c != '_' && c != '-' {
			return refuse("it holds a character other than a letter, a digit, '.', '_' or '-'")
		}
	}
	switch {
	case !isAlnum(name[0]):
		return refuse("it must start with a letter or a digit")
	case strings.Contains(name, ".."):
		return refuse(`it holds ".."`)
	}
	return nil
}

func isAlnum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// CheckRev refuses a revision the adapter must not hand to git: an empty one, one that starts with '-' (git
// would read an option, and `--output=<file>` writes that file), and one with a control character (NUL
// cannot be in an argument, and a newline or a tab is not part of any name a person types). Any other text
// is a revision for git to resolve, or to refuse itself. The error wraps ErrInvalidRev.
func CheckRev(rev string) error {
	if rev == "" {
		return fmt.Errorf("%w: a revision is empty", ErrInvalidRev)
	}
	if rev[0] == '-' {
		return fmt.Errorf("%w: %q starts with '-' and would be read as an option", ErrInvalidRev, rev)
	}
	for i := 0; i < len(rev); i++ {
		if c := rev[i]; c < 0x20 || c == 0x7f {
			return fmt.Errorf("%w: %q holds a control character", ErrInvalidRev, rev)
		}
	}
	return nil
}

// ResultName is the name of the file that holds the nth launch stored for the head of c: <head12>-<n>.json.
// n counts from 1; NextNumber says which one is next.
func ResultName(c Candidate, n int) string {
	return c.Short() + "-" + strconv.Itoa(n) + ".json"
}

// ResultPath is the path, relative to the root of the repository and written with '/', of the file called
// name in the results directory of the feature. It does not check either part: CheckFeature does, and the
// name comes from ResultName.
func ResultPath(feature, name string) string {
	return path.Join(ResultsDir, feature, name)
}

// maxNumberDigits bounds a launch number that NextNumber counts, so that a name a person or another tool
// put in the directory cannot make the next number overflow: nine digits is a billion launches of one head.
const maxNumberDigits = 9

// NextNumber is the number of the next launch stored for the head of c, given the names in the results
// directory of the feature: one more than the highest number any of them holds, and 1 when none does. A
// name holds a number when it is <head12>-<n> followed by '.', where <n> is written the way ResultName
// writes it (digits, no leading zero), so a result and the verifier report stored beside it
// (<head12>-<n>.verifier.md) both do, and the number of a deleted launch is not given again as long as a
// later one is there. Any other name, a temporary file included, holds none.
func NextNumber(names []string, c Candidate) int {
	prefix := c.Short() + "-"
	highest := 0
	for _, name := range names {
		rest, ok := strings.CutPrefix(name, prefix)
		if !ok {
			continue
		}
		digits, _, ok := strings.Cut(rest, ".")
		if !ok || !canonicalNumber(digits) {
			continue
		}
		if n, err := strconv.Atoi(digits); err == nil && n > highest {
			highest = n
		}
	}
	return highest + 1
}

// canonicalNumber reports whether s is a positive number written in digits with no leading zero and no
// more than maxNumberDigits of them.
func canonicalNumber(s string) bool {
	if s == "" || len(s) > maxNumberDigits || s[0] == '0' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// WorktreePath is where the detached worktree of the review of c is: review-<head12> in the directory
// <repo-name>-worktrees that sits beside the main checkout at mainRoot, so for ~/labdrian-sdd-overlay it is
// ~/labdrian-sdd-overlay-worktrees/review-<head12>. It is never inside the repository, whichever worktree
// of it the command runs in, because mainRoot is the root of the main one.
func WorktreePath(mainRoot string, c Candidate) string {
	return filepath.Join(filepath.Dir(mainRoot), filepath.Base(mainRoot)+"-worktrees", "review-"+c.Short())
}
