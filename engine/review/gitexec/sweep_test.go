package gitexec_test

import (
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

	cases := []struct {
		name string
		want []string
	}{
		{"oldName", []string{"a.go", "c.txt"}},
		{"a.b", []string{"d.txt"}},
		{"-flag", []string{"e.txt"}},
		{"absent", nil},
	}
	for _, c := range cases {
		got, err := f.adapter().Mentions(f.repo(), head, c.name)
		slices.Sort(got)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("Mentions(%q) = %q, %v, want %q, nil", c.name, got, err, c.want)
		}
	}
}

func TestMentionsRefusesACommitGitDoesNotHave(t *testing.T) {
	f := newFixture(t, "sha1")
	f.write("a.go", "package a\n")
	f.snapshotAll("feat: head")

	if _, err := f.adapter().Mentions(f.repo(), strings.Repeat("a", 40), "x"); err == nil {
		t.Error("Mentions of an unknown commit gave no error, want one: a grep that did not run is not an empty answer")
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
