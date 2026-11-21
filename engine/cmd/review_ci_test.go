package main

// `review ci --landing <rev> --head <rev> [--cwd <repo>]` (labdrian-orchestrator-opencode T46, S3 L123): the engine says whether a
// repository has no CI, so that the foreman does not. Exit 0 with one line that starts `CI: none configured` only when the
// landing holds the head, `git remote` lists no remote and the tree of the landing commit holds no CI marker. Every other
// case is a refusal or a failure: non-zero, one line on stderr, nothing on stdout. Nothing is written. The tests run the verb
// over repositories they make in a temporary directory.

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// ciMarkerFiles is one file for each entry of the closed list of S3 L123, root paths only.
var ciMarkerFiles = []string{
	".github/workflows/ci.yml", ".gitea/workflows/ci.yml", ".forgejo/workflows/ci.yml", ".buildkite/pipeline.yml",
	".woodpecker/build.yml", ".gitlab-ci.yml", ".circleci/config.yml", ".travis.yml", "azure-pipelines.yml",
	"Jenkinsfile", "bitbucket-pipelines.yml", ".drone.yml", ".woodpecker.yml", "appveyor.yml", ".appveyor.yml",
}

// ciState is everything the verb must leave as it found it: the status of the repository (untracked files too), the
// refs, the remotes and a digest of every file of the repository and of the home the process runs under.
func (w *reviewWorld) ciState() string {
	w.t.Helper()
	var b strings.Builder
	b.WriteString(w.git("status", "--porcelain", "--untracked-files=all") + w.git("for-each-ref") + w.git("remote", "-v"))
	for _, root := range []string{w.parent, os.Getenv("HOME")} {
		walked := filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
			if err != nil {
				fmt.Fprintf(&b, "path error %v\n", err)
				return nil
			}
			if path == filepath.Join(w.root, ".git", "index.lock") {
				return nil
			}
			if e.IsDir() {
				fmt.Fprintf(&b, "%s/\n", path)
				return nil
			}
			data, _ := os.ReadFile(path)
			fmt.Fprintf(&b, "%s %x\n", path, sha256.Sum256(data))
			return nil
		})
		if walked != nil {
			w.t.Fatalf("walking %s: %v", root, walked)
		}
	}
	return b.String()
}

// ci runs `review ci` and holds the repository and the state home to what they were.
func (w *reviewWorld) ci(args ...string) (stdout, stderr string, exits []int) {
	w.t.Helper()
	before := w.ciState()
	stdout, stderr, exits = w.run("", append([]string{"ci"}, args...)...)
	if after := w.ciState(); after != before {
		w.t.Errorf("review ci changed the repository or the state home:\n--- before\n%s\n--- after\n%s", before, after)
	}
	return stdout, stderr, exits
}

// land commits file on top of the world and returns the commit.
func (w *reviewWorld) land(file string) string {
	w.t.Helper()
	overlaytest.WriteFile(w.t, w.root, file, "x\n")
	w.git("add", "-A")
	w.git("commit", "-q", "-m", "land "+file)
	return w.name("HEAD")
}

func noneConfigured(landing, head string) string {
	return "CI: none configured: landing " + landing + " holds head " + head + ", no remote, no CI marker in its tree\n"
}

func TestReviewCIWithNoRemoteAndNoMarkerIsNoneConfiguredNamingTheFullIds(t *testing.T) {
	w := newReviewWorld(t)
	landing := w.land("d.txt")

	stdout, stderr, exits := w.ci("--landing", "HEAD", "--head", w.head)

	if stdout != noneConfigured(landing, w.head) || stderr != "" || !equalInts(exits, []int{0}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want %q, nothing, [0]", stdout, stderr, exits, noneConfigured(landing, w.head))
	}
}

func TestReviewCIRefusesEachMarkerOfTheListAndNamesIt(t *testing.T) {
	for _, marker := range ciMarkerFiles {
		t.Run(marker, func(t *testing.T) {
			w := newReviewWorld(t)
			landing := w.land(marker)

			stdout, stderr, exits := w.ci("--landing", "HEAD", "--head", w.head)

			want := "error: review ci: the tree of the landing " + landing + " holds CI markers: " + marker + "; the no-CI closure does not apply\n"
			if stdout != "" || stderr != want || !equalInts(exits, []int{1}) {
				t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q, [1]", stdout, stderr, exits, want)
			}
		})
	}
}

func TestReviewCIDoesNotReadAMarkerBelowTheRoot(t *testing.T) {
	w := newReviewWorld(t)
	w.land("sub/.github/workflows/x.yml")
	landing := w.land("sub/Jenkinsfile")

	stdout, stderr, exits := w.ci("--landing", "HEAD", "--head", w.head)

	if stdout != noneConfigured(landing, w.head) || stderr != "" || !equalInts(exits, []int{0}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want %q, nothing, [0]", stdout, stderr, exits, noneConfigured(landing, w.head))
	}
}

func TestReviewCIRefusesARemoteAndNamesIt(t *testing.T) {
	w := newReviewWorld(t)
	w.land("d.txt")
	w.git("remote", "add", "origin", "https://example.invalid/repo.git")

	stdout, stderr, exits := w.ci("--landing", "HEAD", "--head", w.head)

	want := "error: review ci: the repository has remotes: origin; the no-CI closure does not apply\n"
	if stdout != "" || stderr != want || !equalInts(exits, []int{1}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q, [1]", stdout, stderr, exits, want)
	}
}

func TestReviewCIRefusesALandingThatDoesNotHoldTheHead(t *testing.T) {
	w := newReviewWorld(t)

	stdout, stderr, exits := w.ci("--landing", w.base, "--head", w.head)

	want := "error: review ci: the landing " + w.base + " does not hold the head " + w.head + "; the no-CI closure does not apply\n"
	if stdout != "" || stderr != want || !equalInts(exits, []int{1}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q, [1]", stdout, stderr, exits, want)
	}
}

// A marker that is only in the working tree is not in the landing commit: the commit has no CI marker, so no CI is
// configured for it, whether the file is untracked or staged.
func TestReviewCIDoesNotReadTheWorkingTree(t *testing.T) {
	w := newReviewWorld(t)
	landing := w.land("d.txt")
	overlaytest.WriteFile(t, w.root, ".github/workflows/ci.yml", "on: push\n")
	overlaytest.WriteFile(t, w.root, "Jenkinsfile", "pipeline {}\n")
	w.git("add", "Jenkinsfile")

	stdout, stderr, exits := w.ci("--landing", "HEAD", "--head", w.head)

	if stdout != noneConfigured(landing, w.head) || stderr != "" || !equalInts(exits, []int{0}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want %q, nothing, [0]", stdout, stderr, exits, noneConfigured(landing, w.head))
	}
}

// Deleting a workflow file in the working tree changes nothing: the landing commit still holds it.
func TestReviewCIStillRefusesAMarkerCommittedAndDeletedLocally(t *testing.T) {
	w := newReviewWorld(t)
	landing := w.land(".github/workflows/ci.yml")
	if err := os.Remove(filepath.Join(w.root, ".github", "workflows", "ci.yml")); err != nil {
		t.Fatalf("setup: %v", err)
	}

	stdout, stderr, exits := w.ci("--landing", landing, "--head", w.head)

	want := "error: review ci: the tree of the landing " + landing + " holds CI markers: .github/workflows/ci.yml; the no-CI closure does not apply\n"
	if stdout != "" || stderr != want || !equalInts(exits, []int{1}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q, [1]", stdout, stderr, exits, want)
	}
}

func TestReviewCIIsNeverNoneConfiguredForWhatItCannotRead(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"an unknown landing", []string{"--landing", "no-such-rev", "--head", "HEAD"}, "error: review ci: resolve landing \"no-such-rev\": "},
		{"an unknown head", []string{"--landing", "HEAD", "--head", "no-such-rev"}, "error: review ci: resolve head \"no-such-rev\": "},
		{"no landing", []string{"--head", "HEAD"}, "error: review ci: --landing is required\n"},
		{"no head", []string{"--landing", "HEAD"}, "error: review ci: --head is required\n"},
		{"a flag it does not take", []string{"--landing", "HEAD", "--head", "HEAD", "--onto", "main"}, "error: review ci: unknown flag \"--onto\"\n"},
		{"a landing that reads as an option", []string{"--landing", "--all", "--head", "HEAD"}, "error: review ci: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newReviewWorld(t)

			stdout, stderr, exits := w.ci(tc.args...)

			if stdout != "" || !strings.HasPrefix(stderr, tc.want) || strings.Count(stderr, "\n") != 1 || len(exits) != 1 || exits[0] == 0 {
				t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, one line that starts %q, a non-zero exit", stdout, stderr, exits, tc.want)
			}
		})
	}
}

func TestReviewCIRunsInTheRepositoryOfCwd(t *testing.T) {
	w := newReviewWorld(t)
	landing := w.land("d.txt")
	w.cwd = t.TempDir()

	stdout, stderr, exits := w.ci("--landing", "HEAD", "--head", w.head, "--cwd", w.root)

	if stdout != noneConfigured(landing, w.head) || stderr != "" || !equalInts(exits, []int{0}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want %q, nothing, [0]", stdout, stderr, exits, noneConfigured(landing, w.head))
	}
}
