package projectidfs_test

import (
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/projectid"
)

// The rules of adoption are tested over a table in internal/projectid. These cases are the two things that only a
// real repository can show: that the names it derives are the ones its real metadata gives, and that a main
// checkout and a linked worktree built by git itself adopt the same one.

// established builds an Established backed by a fixed set of names.
func established(names ...string) projectid.Established {
	have := map[string]bool{}
	for _, n := range names {
		have[n] = true
	}
	return func(name string) (bool, error) { return have[name], nil }
}

// The cheapest form of integrating fragmented memory is not creating the second pile. A repository whose remote
// normalizes to "github.com/acme/widgets" also derives the plain "widgets", and if THAT is the name the memory
// already lives under, minting the URL-shaped one is fragmentation performed by the resolver itself.
func TestAdopt_EstablishedAliasWinsOverTheChainsAnswerInARealRepository(t *testing.T) {
	tmp := t.TempDir()
	root := newRepo(t, tmp, "widgets")
	git(t, root, "remote", "add", "origin", "https://github.com/acme/widgets.git")

	got, err := resolver().AdoptWith(root, projectid.AdoptOptions{Established: established("widgets")})

	if err != nil {
		t.Fatalf("AdoptWith: %v", err)
	}
	if got.Identity.Project != "widgets" || !got.Adopted || got.Identity.Rule != projectid.RuleRemote {
		t.Fatalf("AdoptWith = %+v, want the established alias \"widgets\" adopted, derived by the remote rule", got)
	}
	want := resolve(t, root)
	if got.Identity.WorktreeRoot != want.WorktreeRoot || got.Identity.CommonDir != want.CommonDir {
		t.Fatalf("the adopted identity is for %s / %s, want %s / %s", got.Identity.WorktreeRoot, got.Identity.CommonDir, want.WorktreeRoot, want.CommonDir)
	}
}

// The property Resolve guarantees must survive adoption: a main checkout and a linked worktree adopt the same
// name, or adoption has reintroduced exactly what it exists to prevent.
func TestAdopt_MainCheckoutAndWorktreeAdoptIdentically(t *testing.T) {
	tmp := t.TempDir()
	root := newRepo(t, tmp, "widgets")
	git(t, root, "remote", "add", "origin", "https://github.com/acme/widgets.git")
	wt := addWorktree(t, root, filepath.Join(tmp, "widgets-feature"))

	opts := projectid.AdoptOptions{Established: established("widgets")}
	fromMain, err := resolver().AdoptWith(root, opts)
	if err != nil {
		t.Fatalf("AdoptWith(main): %v", err)
	}
	fromWorktree, err := resolver().AdoptWith(wt, opts)
	if err != nil {
		t.Fatalf("AdoptWith(worktree): %v", err)
	}

	if fromMain.Identity.Project != fromWorktree.Identity.Project {
		t.Fatalf("adoption fragmented the repository: main=%q worktree=%q", fromMain.Identity.Project, fromWorktree.Identity.Project)
	}
	if len(fromMain.Derived) != len(fromWorktree.Derived) {
		t.Fatalf("the two derive different names: main=%v worktree=%v", fromMain.Derived, fromWorktree.Derived)
	}
	for i := range fromMain.Derived {
		if fromMain.Derived[i] != fromWorktree.Derived[i] {
			t.Fatalf("the two derive different names: main=%v worktree=%v", fromMain.Derived, fromWorktree.Derived)
		}
	}
}

// The common directory is the one a linked worktree shares with its main checkout, absolute and free of links.
func TestCommonDir_IsTheOneAMainCheckoutAndItsWorktreesShare(t *testing.T) {
	tmp := t.TempDir()
	root := newRepo(t, tmp, "widgets")
	wt := addWorktree(t, root, filepath.Join(tmp, "widgets-feature"))

	fromMain, err := resolver().CommonDir(root)
	if err != nil {
		t.Fatalf("CommonDir(main): %v", err)
	}
	fromWorktree, err := resolver().CommonDir(wt)
	if err != nil {
		t.Fatalf("CommonDir(worktree): %v", err)
	}

	if fromMain != fromWorktree || !filepath.IsAbs(fromMain) || filepath.Base(fromMain) != ".git" {
		t.Errorf("CommonDir = %q from the main checkout, %q from the worktree; want one absolute .git directory", fromMain, fromWorktree)
	}
}
