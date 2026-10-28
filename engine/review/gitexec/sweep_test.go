package gitexec_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

// The tests of this file run the real git on repositories made in temporary directories, as the other tests of the
// adapter do. They pin the questions the sweep puts (own-rdd T6b3, S6 b).

// snapshotAll stages everything in the working tree and commits it.
func (f *fixture) snapshotAll(message string) string {
	f.t.Helper()
	f.stage()
	return f.snapshot(message)
}

func TestDeletedOrRenamedListsTheOldPathOfARemovalAndOfARename(t *testing.T) {
	f := newFixture(t, "sha1")
	f.write("gone.go", "package a\n")
	f.write("moved.go", "package a\n\nfunc keep() {}\n")
	f.write("kept.go", "package a\n")
	base := f.snapshotAll("feat: base")
	f.git("rm", "-q", "gone.go")
	f.git("mv", "moved.go", "renamed.go")
	f.write("kept.go", "package a\n\nfunc added() {}\n")
	f.write("fresh.go", "package a\n")
	f.stage()
	head := f.snapshot("feat: head")

	got, err := f.adapter().DeletedOrRenamed(f.repo(), base, head)

	want := []string{"gone.go", "moved.go"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("DeletedOrRenamed = %q, %v, want %q, nil", got, err, want)
	}
}

func TestDeletedOrRenamedOfACommitThatDeletesNothingIsEmpty(t *testing.T) {
	f := newFixture(t, "sha1")
	f.write("a.go", "package a\n")
	base := f.snapshotAll("feat: base")
	f.write("b.go", "package a\n")
	f.stage()
	head := f.snapshot("feat: head")

	got, err := f.adapter().DeletedOrRenamed(f.repo(), base, head)

	if err != nil || len(got) != 0 {
		t.Errorf("DeletedOrRenamed = %q, %v, want none, nil", got, err)
	}
}

func TestChangedLinesSplitsTheDiffOfTheGoAndShellFilesIntoRemovedAndAddedLines(t *testing.T) {
	f := newFixture(t, "sha1")
	f.write("pkg/a.go", "package a\n\nfunc old() {}\n\nfunc keep() {}\n")
	f.write("my file.sh", "old() {\n  :\n}\n")
	f.write("café.go", "package a\n\nfunc accent() {}\n")
	f.write("notes.txt", "func ignored() {}\n")
	base := f.snapshotAll("feat: base")
	f.write("pkg/a.go", "package a\n\nfunc fresh() {}\n\nfunc keep() {}\n")
	f.write("my file.sh", "fresh() {\n  :\n}\n")
	f.write("café.go", "package a\n\nfunc accented() {}\n")
	f.write("notes.txt", "func ignoredToo() {}\n")
	f.stage()
	head := f.snapshot("feat: head")

	removed, added, err := f.adapter().ChangedLines(f.repo(), base, head)

	wantRemoved := []review.ChangedLines{
		{Path: "café.go", Lines: []string{"func accent() {}"}},
		{Path: "my file.sh", Lines: []string{"old() {"}},
		{Path: "pkg/a.go", Lines: []string{"func old() {}"}},
	}
	wantAdded := []review.ChangedLines{
		{Path: "café.go", Lines: []string{"func accented() {}"}},
		{Path: "my file.sh", Lines: []string{"fresh() {"}},
		{Path: "pkg/a.go", Lines: []string{"func fresh() {}"}},
	}
	if err != nil || !reflect.DeepEqual(removed, wantRemoved) || !reflect.DeepEqual(added, wantAdded) {
		t.Errorf("ChangedLines = %q, %q, %v, want %q, %q, nil", removed, added, err, wantRemoved, wantAdded)
	}
}

func TestChangedLinesKeepsALineThatStartsWithTheMarkersOfAHeader(t *testing.T) {
	f := newFixture(t, "sha1")
	f.write("a.sh", "echo one\n")
	base := f.snapshotAll("feat: base")
	f.write("a.sh", "-- not a header\n++ nor this\n")
	f.stage()
	head := f.snapshot("feat: head")

	removed, added, err := f.adapter().ChangedLines(f.repo(), base, head)

	wantRemoved := []review.ChangedLines{{Path: "a.sh", Lines: []string{"echo one"}}}
	wantAdded := []review.ChangedLines{{Path: "a.sh", Lines: []string{"-- not a header", "++ nor this"}}}
	if err != nil || !reflect.DeepEqual(removed, wantRemoved) || !reflect.DeepEqual(added, wantAdded) {
		t.Errorf("ChangedLines = %q, %q, %v, want %q, %q, nil", removed, added, err, wantRemoved, wantAdded)
	}
}

func TestModulesAreTheDirectoriesThatHoldAGoModAtTheCommit(t *testing.T) {
	f := newFixture(t, "sha1")
	f.write("go.mod", "module a\n")
	f.write("tools/x/go.mod", "module x\n")
	f.write("tools/x/go.mod.bak", "not a module\n")
	f.write("docs/go.mod.md", "not a module\n")
	f.snapshotAll("feat: head")

	got, err := f.adapter().Modules(f.repo(), f.name("HEAD"))

	want := []string{".", "tools/x"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Modules = %q, %v, want %q, nil", got, err, want)
	}
}

func TestMentionsAreThePathsOfTheCommitThatHoldTheNameAsAWholeWord(t *testing.T) {
	f := newFixture(t, "sha1")
	f.write("a.go", "package a\n\nvar _ = oldName()\n")
	f.write("b.go", "package a\n\nvar _ = oldNameLonger()\n")
	f.write("c.txt", "see oldName here\n")
	f.write("d.txt", "a.b and axb\n")
	f.write("e.txt", "-flag is here\n")
	f.write("g.txt", "axb only\n")
	f.snapshotAll("feat: head")
	head := f.name("HEAD")
	// the working tree must not matter: the question is put to the commit
	f.write("f.go", "package a\n\nvar _ = oldName()\n")

	got, err := f.adapter().Mentions(f.repo(), head, []string{"oldName", "a.b", "-flag", "absent"})

	want := map[string][]string{"oldName": {"a.go", "c.txt"}, "a.b": {"d.txt"}, "-flag": {"e.txt"}}
	for _, paths := range got {
		slices.Sort(paths)
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Mentions = %q, %v, want %q, nil: several names in one call, and none for a name nothing mentions", got, err, want)
	}
}

func TestMentionsKeepsAPathWithASpaceWholeAndNamesAFileOnceHoweverOftenItHoldsTheName(t *testing.T) {
	f := newFixture(t, "sha1")
	f.write("my dir/a file.go", "package a\n\nvar _ = twice(twice(1))\n")
	f.snapshotAll("feat: head")

	got, err := f.adapter().Mentions(f.repo(), f.name("HEAD"), []string{"twice", "once"})

	want := map[string][]string{"twice": {"my dir/a file.go"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Mentions = %q, %v, want %q, nil", got, err, want)
	}
}

func TestMentionsOfNoNameStartsNothingAndAnswersNothing(t *testing.T) {
	r := &fakeRunner{}

	got, err := newGit(r).Mentions(repo, headSHA, nil)

	if err != nil || len(got) != 0 || len(r.calls) != 0 {
		t.Errorf("Mentions = %q, %v after %d starts, want none, nil and no start", got, err, len(r.calls))
	}
}

func TestMentionsPutsAtMost256NamesToOneStartOfGit(t *testing.T) {
	names := make([]string, 257)
	for i := range names {
		names[i] = fmt.Sprintf("name%d", i)
	}
	r := &fakeRunner{}

	got, err := newGit(r).Mentions(repo, headSHA, names)

	if err != nil || len(got) != 0 {
		t.Fatalf("Mentions = %q, %v, want none, nil", got, err)
	}
	if len(r.calls) != 2 {
		t.Fatalf("git was started %d times for 257 names, want 2", len(r.calls))
	}
	if n := strings.Count(strings.Join(r.calls[0].args, " "), " -e "); n != 256 {
		t.Errorf("the first start holds %d names, want 256", n)
	}
	if args := r.calls[1].args; !slices.Contains(args, "name256") || slices.Contains(args, "name0") {
		t.Errorf("the second start is %q, want only the last name", args)
	}
}

func TestMentionsRefusesAnAnswerThatNamesAThingNobodyAskedFor(t *testing.T) {
	for name, stdout := range map[string]string{
		"a name not asked":    headSHA + ":a.go\x00other\n",
		"a path of no commit": "a.go\x00oldName\n",
		"a record cut short":  headSHA + ":a.go\x00oldName",
	} {
		t.Run(name, func(t *testing.T) {
			r := &fakeRunner{stdout: []byte(stdout)}

			_, err := newGit(r).Mentions(repo, headSHA, []string{"oldName"})

			if err == nil {
				t.Error("Mentions gave no error, want one: an answer that is not whole is not an answer")
			}
		})
	}
}

func TestMentionsRefusesACommitGitDoesNotHave(t *testing.T) {
	f := newFixture(t, "sha1")
	f.write("a.go", "package a\n")
	f.snapshotAll("feat: head")

	if _, err := f.adapter().Mentions(f.repo(), strings.Repeat("a", 40), []string{"x"}); err == nil {
		t.Error("Mentions of an unknown commit gave no error, want one: a grep that did not run is not an empty answer")
	}
}

func TestMentionsRefusesAnEmptyNameAndAnyNameWithALineBreak(t *testing.T) {
	r := &fakeRunner{}
	for _, name := range []string{"", "a\nb"} {
		if _, err := newGit(r).Mentions(repo, headSHA, []string{name}); err == nil {
			t.Errorf("Mentions(%q) gave no error, want one: git would read it as several patterns or as every line", name)
		}
	}
	if len(r.calls) != 0 {
		t.Errorf("git was started %d times, want none", len(r.calls))
	}
}

func TestIsCommitIsTrueForACommitAndFalseForAWellFormedNameGitDoesNotHaveAndForATree(t *testing.T) {
	f := newFixture(t, "sha1")
	f.write("a.go", "package a\n")
	f.snapshotAll("feat: head")
	head := f.name("HEAD")
	tree := f.name("HEAD^{tree}")
	for name, c := range map[string]struct {
		object string
		want   bool
	}{
		"a commit":                 {head, true},
		"a name git does not have": {strings.Repeat("a", 40), false},
		"a tree":                   {tree, false},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := f.adapter().IsCommit(f.repo(), c.object)

			if err != nil || got != c.want {
				t.Errorf("IsCommit = %v, %v, want %v, nil", got, err, c.want)
			}
		})
	}
}

func TestIsCommitIsAnErrorWhenGitDoesNotAnswer(t *testing.T) {
	for name, r := range map[string]*fakeRunner{
		"a status and a message":    {stderr: []byte("fatal: boom"), err: exitStatus(t)},
		"a failure to start":        {err: errors.New("no git")},
		"an answer that is no kind": {stdout: []byte("banana\n")},
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := newGit(r).IsCommit(repo, headSHA); err == nil || got {
				t.Errorf("IsCommit = %v, %v, want false and an error", got, err)
			}
		})
	}
}

func TestStatusIsThePorcelainLinesOfTheWorkingTreeWithEveryUntrackedFile(t *testing.T) {
	f := newFixture(t, "sha1")
	f.write("a.txt", "a\n")
	f.write("b.txt", "b\n")
	f.snapshotAll("feat: head")
	f.write("a.txt", "changed\n")
	f.write("new/deep/c.txt", "c\n")
	f.write("café.txt", "x\n")

	got, err := f.adapter().Status(f.repo())

	want := []string{" M a.txt", "?? café.txt", "?? new/deep/c.txt"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Status = %q, %v, want %q, nil", got, err, want)
	}
}

func TestStatusOfACleanTreeIsEmpty(t *testing.T) {
	f := newFixture(t, "sha1")
	f.write("a.txt", "a\n")
	f.snapshotAll("feat: head")

	got, err := f.adapter().Status(f.repo())

	if err != nil || len(got) != 0 {
		t.Errorf("Status = %q, %v, want none, nil", got, err)
	}
}

func TestArchiveFillsTheDirectoryWithTheTreeOfTheCommit(t *testing.T) {
	f := newFixture(t, "sha1")
	f.write("a.go", "package a\n")
	f.write("sub/dir/b.go", "package b\n")
	f.write("run.sh", "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(f.root, "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a.go", filepath.Join(f.root, "link.go")); err != nil {
		t.Fatal(err)
	}
	base := f.snapshotAll("feat: base")
	f.git("rm", "-q", "a.go")
	f.write("later.go", "package later\n")
	f.stage()
	f.snapshot("feat: later")
	dir := t.TempDir()

	err := f.adapter().Archive(f.repo(), base, dir)

	if err != nil {
		t.Fatalf("Archive error = %v", err)
	}
	for rel, want := range map[string]string{"a.go": "package a\n", "sub/dir/b.go": "package b\n", "run.sh": "#!/bin/sh\n"} {
		got, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil || string(got) != want {
			t.Errorf("%s = %q, %v, want %q", rel, got, err, want)
		}
	}
	if info, err := os.Stat(filepath.Join(dir, "run.sh")); err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Errorf("run.sh mode = %v, %v, want it executable", info, err)
	}
	if target, err := os.Readlink(filepath.Join(dir, "link.go")); err != nil || target != "a.go" {
		t.Errorf("link.go = %q, %v, want a link to a.go", target, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "later.go")); err == nil {
		t.Error("later.go is in the archive of the base, which does not have it")
	}
}

func TestArchiveRefusesADirectoryThatIsNotAbsoluteAndACommitGitDoesNotHave(t *testing.T) {
	f := newFixture(t, "sha1")
	f.write("a.go", "package a\n")
	head := f.snapshotAll("feat: head")

	if err := f.adapter().Archive(f.repo(), head, "relative/dir"); err == nil {
		t.Error("Archive into a relative directory gave no error")
	}
	if err := f.adapter().Archive(f.repo(), strings.Repeat("a", 40), t.TempDir()); err == nil {
		t.Error("Archive of an unknown commit gave no error")
	}
}
