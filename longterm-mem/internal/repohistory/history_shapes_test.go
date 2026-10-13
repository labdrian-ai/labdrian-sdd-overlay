package repohistory_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/repohistory"
)

// accentedName is a file name with a letter outside ASCII (written as its code point, so that no tool rewrites it).
const accentedName = "se" + string(rune(0xF1)) + "al.go"

// shapesRepo is a repository whose history has the files these cases ask about, in a project directory of its
// own (so that a file "beside the project" is the test's and nobody else's): kept.go and "has space.go" are in
// the tree, and "gone away.go", `it's "q".go` and an accented name were removed by a commit. It returns the project
// and the absolute path of a file beside it.
func shapesRepo(t *testing.T) (root, beside string) {
	t.Helper()
	parent := t.TempDir()
	root = filepath.Join(parent, "project")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "-q", "-b", "main")
	write(t, root, "kept.go", "package a\n")
	write(t, root, "has space.go", "package a\n")
	write(t, root, "gone away.go", "package a\n// a distinctive body, so that it is no rename\n")
	write(t, root, `it's "q".go`, "package a\n// another distinctive body, so that it is no rename\n")
	write(t, root, accentedName, "package a\n// a third distinctive body, so that it is no rename\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "first")
	git(t, root, "rm", "-q", "gone away.go", `it's "q".go`, accentedName)
	git(t, root, "commit", "-q", "-m", "remove three files")

	beside = filepath.Join(parent, "beside.go")
	if err := os.WriteFile(beside, []byte("package beside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, beside
}

// Shapes of a path that the detectors' pattern keeps out of what they ask (an empty text, a leading "./" or
// "../", an absolute path, a space, a quote, a letter outside ASCII) are still a question this package can be
// asked, and so is a climb in the middle of a path ("a/../../x.go"), which the pattern does admit: what it
// answers is pinned here as it is today, so that rework of what is around it cannot change it unseen.
//
// Every answer follows from two plain facts: it first asks the file system about the path joined to the
// root (anything that resolves to a file or a directory is "present", the root itself included), and it then
// asks git for the history of the text as given, and compares it with the names git prints, which git writes
// without a leading "./", and quoted when they hold a quote or a letter outside ASCII. So a removed file asked
// about by a different spelling of its name is "unknown" (a commit is found, and it is not recognised as that
// path's deletion), never "deleted".
func TestInspect_PinsWhatItAnswersForTheShapesOfAPath(t *testing.T) {
	root, beside := shapesRepo(t)

	type answer struct {
		state     repohistory.State
		hasCommit bool
	}
	for _, tc := range []struct {
		name, path string
		want       answer
	}{
		{"an empty text is the root, which is there", "", answer{repohistory.StatePresent, false}},
		{"a dot is the root, which is there", ".", answer{repohistory.StatePresent, false}},
		{"a leading ./ of a name with a space, which is there", "./has space.go", answer{repohistory.StatePresent, false}},
		{"a leading ./ of a removed file is not its deletion", "./gone away.go", answer{repohistory.StateUnknown, true}},
		{"a climb that comes back to a file that is there", "a/../kept.go", answer{repohistory.StatePresent, false}},
		{"a climb that comes back to a removed file is not its deletion", "a/../gone away.go", answer{repohistory.StateUnknown, true}},
		{"a removed file whose name holds a quote is not its deletion", `it's "q".go`, answer{repohistory.StateUnknown, true}},
		{"a removed file whose name holds a letter outside ASCII is not its deletion", accentedName, answer{repohistory.StateUnknown, true}},
		{"a removed file whose name holds a space is its deletion", "gone away.go", answer{repohistory.StateDeleted, true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts, err := repohistory.Inspect(root, []string{tc.path})
			if err != nil {
				t.Fatalf("Inspect(%q): %v", tc.path, err)
			}
			got := facts[tc.path]
			if got.Path != tc.path || got.State != tc.want.state || (got.Commit != "") != tc.want.hasCommit {
				t.Errorf("Inspect(%q) = %+v, want state %q and a commit: %v", tc.path, got, tc.want.state, tc.want.hasCommit)
			}
		})
	}

	t.Run("a climb out of the project to a file that is there is present", func(t *testing.T) {
		facts, err := repohistory.Inspect(root, []string{"../beside.go"})
		if err != nil || facts["../beside.go"].State != repohistory.StatePresent {
			t.Errorf("Inspect(../beside.go) = %+v, %v; want present, for the file beside the project is there", facts, err)
		}
	})

	// Git refuses a path outside the repository, and the refusal is the answer for all the paths asked about
	// at once: not a guess for this path, and no fact for the others. An absolute path is refused even when the
	// file is there, because the file system is asked about the path joined to the root, not about the path.
	for _, tc := range []struct{ name, path string }{
		{"a climb out of the project to a file that is not there", "../nothing.go"},
		{"a climb out of the project from below", "a/../../x.go"},
		{"an absolute path outside the project, though the file is there", beside},
		{"an absolute path that is not there", "/nowhere/at/all.go"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts, err := repohistory.Inspect(root, []string{"kept.go", tc.path})
			if err == nil || facts != nil || !strings.Contains(err.Error(), "repohistory: reading the history of "+tc.path+": ") {
				t.Errorf("Inspect(kept.go, %q) = %v, %v; want git's refusal of the path and no facts", tc.path, facts, err)
			}
		})
	}
}
