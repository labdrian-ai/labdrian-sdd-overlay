package gitexec_test

// The three answers `review anchor` asks of git (own-rdd T7a, S7 g), on real repositories: every parent of a commit,
// the merge bases of two commits, and the tree of a commit.

import (
	"reflect"
	"strings"
	"testing"
)

func TestParentsListsEveryParentOfACommitFirstParentFirst(t *testing.T) {
	f := newFixture(t, "sha1")
	f.write("a.txt", "one\n")
	f.stage()
	first := f.snapshot("first")
	f.write("a.txt", "two\n")
	f.stage()
	second := f.snapshot("second")
	f.git("checkout", "-q", "-b", "side", first)
	f.write("b.txt", "side\n")
	f.stage()
	side := f.snapshot("side")
	f.git("checkout", "-q", "main")
	f.git("merge", "-q", "--no-ff", "-m", "merge", "side")
	g := f.adapter()

	for _, tc := range []struct {
		name, commit string
		want         []string
	}{
		{"a root commit", first, nil},
		{"a commit", second, []string{first}},
		{"a merge", f.name("HEAD"), []string{second, side}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := g.Parents(f.repo(), tc.commit)

			if err != nil || len(got) != len(tc.want) || (len(got) > 0 && !reflect.DeepEqual(got, tc.want)) {
				t.Errorf("Parents(%s) = %q, %v; want %q", tc.commit, got, err, tc.want)
			}
		})
	}
	if got, err := g.Parents(f.repo(), strings.Repeat("0", 40)); err == nil {
		t.Errorf("Parents() of an object that is not there = %q, nil; want an error", got)
	}
}

func TestMergeBasesAreNoneForUnrelatedHistoriesOneForABranchAndTwoForACrissCross(t *testing.T) {
	f := newFixture(t, "sha1")
	f.write("a.txt", "root\n")
	f.stage()
	root := f.snapshot("root")
	f.git("checkout", "-q", "-b", "left")
	f.write("l.txt", "l\n")
	f.stage()
	left := f.snapshot("left")
	f.git("checkout", "-q", "-b", "right", root)
	f.write("r.txt", "r\n")
	f.stage()
	right := f.snapshot("right")
	f.git("checkout", "-q", "left")
	f.git("merge", "-q", "--no-ff", "-m", "left takes right", right)
	leftTip := f.name("HEAD")
	f.git("checkout", "-q", "right")
	f.git("merge", "-q", "--no-ff", "-m", "right takes left", left)
	rightTip := f.name("HEAD")
	f.git("checkout", "-q", "--orphan", "unrelated")
	f.git("rm", "-q", "-rf", ".")
	f.write("u.txt", "u\n")
	f.stage()
	unrelated := f.snapshot("unrelated")
	g := f.adapter()

	for _, tc := range []struct {
		name, a, b string
		want       []string
	}{
		{"unrelated histories", leftTip, unrelated, nil},
		{"a branch and main", left, right, []string{root}},
		{"a criss-cross", leftTip, rightTip, []string{left, right}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := g.MergeBases(f.repo(), tc.a, tc.b)

			if err != nil {
				t.Fatalf("MergeBases() error = %v; want nil", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("MergeBases() = %q; want %q", got, tc.want)
			}
			for _, want := range tc.want {
				found := false
				for _, name := range got {
					found = found || name == want
				}
				if !found {
					t.Errorf("MergeBases() = %q; want it to hold %s", got, want)
				}
			}
		})
	}
	if got, err := g.MergeBases(f.repo(), strings.Repeat("0", 40), left); err == nil {
		t.Errorf("MergeBases() of an object that is not there = %q, nil; want an error", got)
	}
}

func TestTreeIsTheTreeOfTheCommit(t *testing.T) {
	for _, format := range []string{"sha1", "sha256"} {
		t.Run(format, func(t *testing.T) {
			f := newFixture(t, format)
			f.write("a.txt", "one\n")
			f.stage()
			commit := f.snapshot("first")

			got, err := f.adapter().Tree(f.repo(), commit)

			if want := f.name(commit + "^{tree}"); err != nil || got != want {
				t.Errorf("Tree() = %q, %v; want %q", got, err, want)
			}
		})
	}
	f := newFixture(t, "sha1")
	if got, err := f.adapter().Tree(f.repo(), strings.Repeat("0", 40)); err == nil {
		t.Errorf("Tree() of an object that is not there = %q, nil; want an error", got)
	}
}

// Parent is untouched by Parents: of a merge it still gives the first parent only.
func TestParentOfAMergeStillGivesOnlyTheFirstParent(t *testing.T) {
	f := newFixture(t, "sha1")
	f.write("a.txt", "one\n")
	f.stage()
	first := f.snapshot("first")
	f.git("checkout", "-q", "-b", "side")
	f.write("b.txt", "side\n")
	f.stage()
	f.snapshot("side")
	f.git("checkout", "-q", "main")
	f.write("c.txt", "main\n")
	f.stage()
	main := f.snapshot("main")
	f.git("merge", "-q", "--no-ff", "-m", "merge", "side")

	got, err := f.adapter().Parent(f.repo(), f.name("HEAD"))

	if err != nil || got != main || got == first {
		t.Errorf("Parent() = %q, %v; want %q", got, err, main)
	}
}
