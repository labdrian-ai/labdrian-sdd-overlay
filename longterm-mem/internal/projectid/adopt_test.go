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

// The hole derivation alone cannot close: a repository that MOVES stops
// deriving its old path, so memory stored under that path becomes
// unreachable -- silently, and forever. A name the repository was known by
// before is still its name, and adopting it is the reunion.
func TestAdoptWith_RemembersANameNothingDerivesAnyMore(t *testing.T) {
	tmp := t.TempDir()
	root := newRepo(t, tmp, "widgets") // no declaration, no remote: path-derived only

	got, err := projectid.AdoptWith(root, projectid.AdoptOptions{
		Established: established("/somewhere/else/.git"),
		Remembered:  []string{"/somewhere/else/.git"},
	})
	if err != nil {
		t.Fatalf("AdoptWith: %v", err)
	}
	if got.Identity.Project != "/somewhere/else/.git" {
		t.Fatalf("a remembered name that still holds memory must be adopted: got %q", got.Identity.Project)
	}
	if !got.Adopted {
		t.Fatal("Adopted must report that the identity came from storage")
	}
}

// Remembered names rank BELOW everything the repository still derives. What
// it looks like now is better evidence than what it looked like once, and
// inverting that would let a stale name outrank a live declaration.
func TestAdoptWith_LiveDerivationOutranksMemory(t *testing.T) {
	tmp := t.TempDir()
	root := newRepo(t, tmp, "widgets")
	write(t, filepath.Join(root, projectid.DeclaredFileName), "current-name\n")

	got, err := projectid.AdoptWith(root, projectid.AdoptOptions{
		Established: established("current-name", "former-name"),
		Remembered:  []string{"former-name"},
	})
	if err != nil {
		t.Fatalf("AdoptWith: %v", err)
	}
	if got.Identity.Project != "current-name" {
		t.Fatalf("a live declaration must outrank a remembered name: got %q", got.Identity.Project)
	}
	if len(got.PendingIntegration) != 1 || got.PendingIntegration[0] != "former-name" {
		t.Fatalf("the remembered name still holds memory and is owed an integration: got %v", got.PendingIntegration)
	}
}

// DerivableNames is what the caller writes to the ledger, so it must report
// the rule and whether each name may later be adopted on the ledger's word
// alone. The loose spellings may not: a bare directory name can name
// somebody else's repository.
func TestDerivableNames_MarksLooseSpellingsUnadoptable(t *testing.T) {
	tmp := t.TempDir()
	root := newRepo(t, tmp, "widgets")
	git(t, root, "remote", "add", "origin", "https://github.com/acme/widgets.git")

	names, err := projectid.DerivableNames(root)
	if err != nil {
		t.Fatalf("DerivableNames: %v", err)
	}

	strict := map[string]bool{}
	for _, n := range names {
		strict[n.Name] = n.Strict
	}
	if !strict["github.com/acme/widgets"] {
		t.Error("a normalized remote is a strict identity")
	}
	if strict["widgets"] {
		t.Error("a bare last segment can name another repository and must not be adoptable from memory alone")
	}
}
