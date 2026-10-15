package installer_test

// bootstrap makes its first commit only in a repository that has none (cmd_bootstrap, step 5). It
// used to ask `git log --oneline 2>/dev/null || true` and read any failure as "no commit yet", so a
// repository git could not read was committed to as if it were empty. "No commit yet" is a HEAD that
// names a branch that does not exist; anything else that keeps git from reading HEAD stops bootstrap
// with an error, and nothing is committed (routing_pins_test.go has the world).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// gitState is what a command that committed, or moved a branch, would have changed in the
// repository of the world: HEAD and every ref, as files.
func (w *pinWorld) gitState() string {
	w.t.Helper()
	gitDir := filepath.Join(w.overlay, ".git")
	head, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		w.t.Fatal(err)
	}
	return "HEAD: " + string(head) + w.tree(filepath.Join(gitDir, "refs"))
}

func TestBootstrapOfARepositoryWithNoCommitYetMakesTheFirstOne(t *testing.T) {
	for _, tc := range []struct {
		name   string
		branch string // the branch HEAD names, "" for the one git init chooses
		want   string
	}{
		{"on the branch git init chose", "", "[master (root-commit) <SHA>] chore: initial overlay scaffold\n"},
		{"on a branch of another name", "trunk", "[trunk (root-commit) <SHA>] chore: initial overlay scaffold\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newBootstrapWorld(t)
			w.git("init")
			if tc.branch != "" {
				w.git("symbolic-ref", "HEAD", "refs/heads/"+tc.branch)
			}

			run := w.run("bootstrap")

			if run.Exit != 0 {
				t.Fatalf("bootstrap = %s", pinRunText(run))
			}
			if !strings.Contains(run.Stdout, tc.want) {
				t.Errorf("bootstrap did not make the first commit, want %q in\n%s", tc.want, run.Stdout)
			}
			if got := strings.Count(w.git("log", "--oneline", "--all"), "\n"); got != 3 {
				t.Errorf("the repository has %d commits, want the scaffold, the baseline and the overlay", got)
			}
		})
	}
}

// A repository that has a commit is not one that has none because `git log` cannot run in it (here
// a date format git refuses): the scaffold is not committed a second time. The script used to
// ask `git log` and read its failure as an empty repository.
func TestBootstrapDoesNotCommitTheScaffoldAgainWhenGitLogCannotRun(t *testing.T) {
	w := newBootstrapWorld(t)
	if first := w.run("bootstrap"); first.Exit != 0 {
		t.Fatalf("the first bootstrap = %s", pinRunText(first))
	}
	w.git("config", "log.date", "bogus")
	w.write(w.overlay, "README.md", "# changed after the scaffold was committed\n")

	// Its last line, `git log`, fails on the configuration; what it committed is the question, and
	// that it did not take the repository for one git cannot read.
	run := w.run("bootstrap")

	if strings.Contains(run.Stderr, "cannot read the history") {
		t.Errorf("bootstrap took a repository that has commits for one git cannot read:\n%s", pinRunText(run))
	}
	subjects := w.git("-c", "log.date=iso", "log", "--all", "--format=%s")
	if got := strings.Count(subjects, "chore: initial overlay scaffold\n"); got != 1 {
		t.Errorf("the scaffold was committed %d times, want once:\n%s", got, subjects)
	}
}

func TestBootstrapStopsWhenGitCannotReadTheHistoryAndCommitsNothing(t *testing.T) {
	const gone = "0123456789012345678901234567890123456789\n"
	for _, tc := range []struct {
		name    string
		corrupt func(t *testing.T, w *pinWorld)
	}{
		{"the branch points at a commit that is gone", func(t *testing.T, w *pinWorld) {
			w.write(w.overlay, ".git/refs/heads/main", gone)
		}},
		{"the file of the branch is not a ref", func(t *testing.T, w *pinWorld) {
			w.write(w.overlay, ".git/refs/heads/main", "not a ref\n")
		}},
		{"HEAD is a commit that is gone", func(t *testing.T, w *pinWorld) {
			w.write(w.overlay, ".git/HEAD", gone)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newBootstrapWorld(t)
			if first := w.run("bootstrap"); first.Exit != 0 {
				t.Fatalf("the first bootstrap = %s", pinRunText(first))
			}
			tc.corrupt(t, w)
			before := w.gitState()

			run := w.run("bootstrap")

			if run.Exit != 1 || !strings.Contains(run.Stderr, "ERROR: cannot read the history of $OVERLAY: ") {
				t.Errorf("bootstrap = %s, want exit 1 and an error that says git cannot read the history", pinRunText(run))
			}
			if after := w.gitState(); after != before {
				t.Errorf("bootstrap moved HEAD or a branch of a repository it could not read:\n--- before\n%s--- after\n%s", before, after)
			}
			if strings.Contains(run.Stdout, "root-commit") || strings.Contains(run.Stdout, "baseline from gentle-ai") {
				t.Errorf("bootstrap committed in a repository it could not read:\n%s", run.Stdout)
			}
		})
	}
}
