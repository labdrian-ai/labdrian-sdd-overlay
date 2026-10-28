package gitexec_test

// What the stop hook asks git (own-rdd T6a, S6, L18): which branch HEAD is on, which branch is the default one, and
// the commits of HEAD that the default branch does not have. The tests run the git of the machine on repositories
// they make in temporary directories, as realgit_test.go does.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// commit adds a file to the repository, commits it and returns the full name of the commit.
func (f *fixture) commit(file string) string {
	f.t.Helper()
	f.write(file, file+"\n")
	f.stage()
	return f.snapshot(file)
}

func TestStandingSaysWhereHeadStandsInItsBranch(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(f *fixture) app.Standing // makes the history and returns what Standing must say of it
	}{
		{"a branch with two commits of its own", func(f *fixture) app.Standing {
			f.commit("m0.txt")
			f.git("checkout", "-q", "-b", "feat")
			c1, c2 := f.commit("c1.txt"), f.commit("c2.txt")
			return app.Standing{Branch: "feat", Default: "main", Commits: []string{c1, c2}}
		}},
		{"the default branch has no commit that it lacks itself", func(f *fixture) app.Standing {
			f.commit("m0.txt")
			f.commit("m1.txt")
			return app.Standing{Branch: "main", Default: "main"}
		}},
		{"a branch that merged the default branch lists none of the commits of the default branch", func(f *fixture) app.Standing {
			f.commit("m0.txt")
			f.git("checkout", "-q", "-b", "feat")
			c1, c2 := f.commit("c1.txt"), f.commit("c2.txt")
			f.git("checkout", "-q", "main")
			f.commit("m1.txt")
			f.git("checkout", "-q", "feat")
			f.git("merge", "-q", "--no-ff", "-m", "merge main", "main")
			merge := f.name("HEAD")
			c3 := f.commit("c3.txt")
			return app.Standing{Branch: "feat", Default: "main", Commits: []string{c1, c2, merge, c3}}
		}},
		{"a branch that merged another branch lists the merge and not the commits merged in", func(f *fixture) app.Standing {
			f.commit("m0.txt")
			f.git("checkout", "-q", "-b", "other")
			f.commit("o1.txt")
			f.git("checkout", "-q", "main")
			f.git("checkout", "-q", "-b", "feat")
			c1 := f.commit("c1.txt")
			f.git("merge", "-q", "--no-ff", "-m", "merge other", "other")
			merge := f.name("HEAD")
			return app.Standing{Branch: "feat", Default: "main", Commits: []string{c1, merge}}
		}},
		{"a detached head says nothing else", func(f *fixture) app.Standing {
			f.commit("m0.txt")
			f.git("checkout", "-q", "--detach")
			f.commit("c1.txt")
			return app.Standing{}
		}},
		{"master is the default branch when there is no main", func(f *fixture) app.Standing {
			f.commit("m0.txt")
			f.git("branch", "-m", "master")
			f.git("checkout", "-q", "-b", "feat")
			c1 := f.commit("c1.txt")
			return app.Standing{Branch: "feat", Default: "master", Commits: []string{c1}}
		}},
		{"a repository that names no default branch says so and lists no commit", func(f *fixture) app.Standing {
			f.commit("m0.txt")
			f.git("branch", "-m", "trunk")
			f.git("checkout", "-q", "-b", "feat")
			f.commit("c1.txt")
			return app.Standing{Branch: "feat"}
		}},
		{"the branch origin/HEAD names is the default branch, before main", func(f *fixture) app.Standing {
			f.commit("m0.txt")
			f.git("checkout", "-q", "-b", "develop")
			develop := f.commit("m1.txt")
			f.git("checkout", "-q", "-b", "feat")
			c1 := f.commit("c1.txt")
			f.git("update-ref", "refs/remotes/origin/develop", develop)
			f.git("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/develop")
			f.git("branch", "-D", "develop")
			return app.Standing{Branch: "feat", Default: "develop", Commits: []string{c1}}
		}},
		{"commits that the remote-tracking default branch has are not the branch's own", func(f *fixture) app.Standing {
			f.commit("m0.txt")
			f.git("checkout", "-q", "-b", "feat")
			ahead := f.commit("m1.txt") // the remote has it; the local main has not
			f.git("update-ref", "refs/remotes/origin/main", ahead)
			c1 := f.commit("c1.txt")
			return app.Standing{Branch: "feat", Default: "main", Commits: []string{c1}}
		}},
		{"commits that only the local default branch has are not the branch's own either", func(f *fixture) app.Standing {
			m0 := f.commit("m0.txt")
			f.git("update-ref", "refs/remotes/origin/main", m0)
			f.commit("m1.txt") // the local main has it; the remote has not
			f.git("checkout", "-q", "-b", "feat")
			c1 := f.commit("c1.txt")
			return app.Standing{Branch: "feat", Default: "main", Commits: []string{c1}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, "sha1")
			want := tc.setup(f)

			got, err := f.adapter().Standing(f.repo())

			if err != nil || !reflect.DeepEqual(got, want) {
				t.Errorf("Standing() = %+v, %v; want %+v and no error", got, err, want)
			}
		})
	}
}

// A line of what git printed that is not the full name of a commit is not an answer to the question: the use case
// hands every commit it is given to git again, as a revision.
func TestStandingRefusesWhatGitPrintedThatIsNotACommitName(t *testing.T) {
	r := &fakeRunner{stdout: []byte("feat\n")}

	_, err := newGit(r).Standing(repo)

	if err == nil || !strings.Contains(err.Error(), "full object name") {
		t.Errorf("Standing() error = %v, want it to say git printed something that is not a full object name", err)
	}
}
