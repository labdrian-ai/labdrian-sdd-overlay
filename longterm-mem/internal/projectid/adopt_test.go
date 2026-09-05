package projectid_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/projectid"
)

// established builds a projectid.Established backed by a fixed set of names.
func established(names ...string) projectid.Established {
	have := map[string]bool{}
	for _, n := range names {
		have[n] = true
	}
	return func(name string) (bool, error) { return have[name], nil }
}

// The cheapest form of integrating fragmented memory is not creating the
// second pile. A repository whose remote normalizes to
// "github.com/acme/widgets" also derives the plain "widgets", and if THAT
// is the name the memory already lives under, minting the URL-shaped one
// is fragmentation performed by the resolver itself.
func TestAdopt_EstablishedAliasWinsOverTheChainsAnswer(t *testing.T) {
	tmp := t.TempDir()
	root := newRepo(t, tmp, "widgets")
	git(t, root, "remote", "add", "origin", "https://github.com/acme/widgets.git")

	got, err := projectid.Adopt(root, established("widgets"))
	if err != nil {
		t.Fatalf("Adopt: %v", err)
	}
	if got.Identity.Project != "widgets" {
		t.Fatalf("an established alias must be adopted, not re-minted: got %q", got.Identity.Project)
	}
	if !got.Adopted {
		t.Fatal("Adopted must report that the identity came from storage, not from the chain")
	}
	if got.Identity.Rule != projectid.RuleRemote {
		t.Fatalf("the adopted name must still name the rule that derived it: got %q", got.Identity.Rule)
	}
}

// With nothing established there is nothing to integrate WITH, so the chain
// decides exactly as before. Adoption must never invent a name that no rule
// derives.
func TestAdopt_NothingEstablishedFallsBackToTheChain(t *testing.T) {
	tmp := t.TempDir()
	root := newRepo(t, tmp, "widgets")
	git(t, root, "remote", "add", "origin", "https://github.com/acme/widgets.git")

	got, err := projectid.Adopt(root, established())
	if err != nil {
		t.Fatalf("Adopt: %v", err)
	}
	want := resolve(t, root)
	if got.Identity.Project != want.Project || got.Identity.Rule != want.Rule {
		t.Fatalf("with nothing established Adopt must equal Resolve: got %q (%s), want %q (%s)",
			got.Identity.Project, got.Identity.Rule, want.Project, want.Rule)
	}
	if got.Adopted {
		t.Fatal("Adopted must be false when no name was established")
	}
	if len(got.PendingIntegration) != 0 {
		t.Fatalf("nothing is pending when nothing is established: got %v", got.PendingIntegration)
	}
}

// Two derivable names both holding memory is the fragmentation itself, and
// it is PROVABLE rather than guessed: both names came out of this one
// repository's own metadata in a single read. The higher-ranked spelling
// becomes canonical and the other is reported as owed an integration --
// reported, because merging Engram's own store is not this module's to do
// (R-002 keeps its connection read-only).
func TestAdopt_OtherEstablishedAliasesArePendingIntegration(t *testing.T) {
	tmp := t.TempDir()
	root := newRepo(t, tmp, "widgets")
	write(t, filepath.Join(root, projectid.DeclaredFileName), "acme-widgets\n")
	git(t, root, "remote", "add", "origin", "https://github.com/acme/widgets.git")

	got, err := projectid.Adopt(root, established("acme-widgets", "widgets"))
	if err != nil {
		t.Fatalf("Adopt: %v", err)
	}
	if got.Identity.Project != "acme-widgets" {
		t.Fatalf("the highest-ranked established name is canonical: got %q", got.Identity.Project)
	}
	if len(got.PendingIntegration) != 1 || got.PendingIntegration[0] != "widgets" {
		t.Fatalf("every other established derivable name is owed an integration: got %v", got.PendingIntegration)
	}
}

// The property Resolve guarantees must survive adoption: a main checkout
// and a linked worktree adopt the same name, or adoption has reintroduced
// exactly what it exists to prevent.
func TestAdopt_MainCheckoutAndWorktreeAdoptIdentically(t *testing.T) {
	tmp := t.TempDir()
	root := newRepo(t, tmp, "widgets")
	git(t, root, "remote", "add", "origin", "https://github.com/acme/widgets.git")
	wt := addWorktree(t, root, filepath.Join(tmp, "widgets-feature"))

	e := established("widgets")
	fromMain, err := projectid.Adopt(root, e)
	if err != nil {
		t.Fatalf("Adopt(main): %v", err)
	}
	fromWorktree, err := projectid.Adopt(wt, e)
	if err != nil {
		t.Fatalf("Adopt(worktree): %v", err)
	}
	if fromMain.Identity.Project != fromWorktree.Identity.Project {
		t.Fatalf("adoption fragmented the repository: main=%q worktree=%q",
			fromMain.Identity.Project, fromWorktree.Identity.Project)
	}
}

// A storage that cannot be consulted must not silently become "nothing is
// established" -- that answer is indistinguishable from a genuinely fresh
// repository, and acting on it mints the second pile this whole mechanism
// exists to avoid. The error is surfaced so the caller decides.
func TestAdopt_UnreadableStorageIsReportedNotAssumedEmpty(t *testing.T) {
	tmp := t.TempDir()
	root := newRepo(t, tmp, "widgets")
	boom := errors.New("storage unavailable")

	_, err := projectid.Adopt(root, func(string) (bool, error) { return false, boom })
	if !errors.Is(err, boom) {
		t.Fatalf("an unreadable storage must surface, not read as empty: got %v", err)
	}
}
