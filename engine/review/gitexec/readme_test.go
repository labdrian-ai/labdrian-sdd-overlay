package gitexec_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/gitexec"
)

// The README of the review results states the command whose output diff_sha256 is the digest of, the one that
// lists the paths, and the environment both run under: the delivery check and any other program that
// recomputes a digest read the definition there. The text of the README and the argument vectors of the code
// must be the same text, so that a flag changed in one and not in the other, which changes every digest, cannot
// pass.
func TestTheReadmeOfTheReviewResultsStatesTheCommandsAndTheEnvironmentTheDigestIsMadeFrom(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(overlaytest.RepoRoot(t), "odd", "review-results", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	readme := string(data)

	// The documented commands name /dev/null where the code names the null device of the platform.
	onUnix := func(s string) string { return strings.ReplaceAll(s, os.DevNull, "/dev/null") }
	for _, tc := range []struct {
		name string
		want string
	}{
		{"the canonical diff", "git " + onUnix(strings.Join(gitexec.DiffArgs("<base>", "<head>"), " "))},
		{"the list of the paths", "git " + strings.Join(gitexec.PathsArgs("<base>", "<head>"), " ")},
		{"the empty tree of a SHA-1 repository", gitexec.EmptyTreeSHA1},
		{"the empty tree of a SHA-256 repository", gitexec.EmptyTreeSHA256},
		{"the variable that keeps the attributes of the system out", "GIT_ATTR_NOSYSTEM=1"},
		{"the locale", "LC_ALL=C"},
		{"the variables of git that are removed", "without any variable whose name begins with `GIT_`"},
	} {
		if !strings.Contains(readme, tc.want) {
			t.Errorf("odd/review-results/README.md does not state %s: want it to contain\n%s", tc.name, tc.want)
		}
	}
}
