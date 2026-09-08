package projectid

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// This file exercises AdoptWith and the unexported derivableNames directly
// (white-box, package projectid) rather than through the now-deleted Adopt
// and DerivableNames wrappers. Every assertion below is the one the deleted
// wrappers' tests used to pin; only the entry point changed.
//
// Every test here builds its own throwaway repositories under t.TempDir()
// and runs git inside those. Nothing touches the repository this module
// lives in. Production code never shells out (exec_allowlist_test.go), but
// a _test.go file may, and building a real linked worktree is the only
// honest way to prove the property under test.

// established builds an Established backed by a fixed set of names.
func established(names ...string) Established {
	have := map[string]bool{}
	for _, n := range names {
		have[n] = true
	}
	return func(name string) (bool, error) { return have[name], nil }
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newRepo creates an initialized repository with one commit at parent/name
// and returns its path.
func newRepo(t *testing.T, parent, name string) string {
	t.Helper()
	root := filepath.Join(parent, name)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", root, err)
	}
	git(t, root, "init", "-q", "-b", "main")
	git(t, root, "commit", "-q", "--allow-empty", "-m", "init")
	return root
}

// addWorktree creates a linked worktree of repo at path and returns path.
func addWorktree(t *testing.T, repo, path string) string {
	t.Helper()
	git(t, repo, "worktree", "add", "-q", "-b", filepath.Base(path), path)
	return path
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func resolveIdentity(t *testing.T, dir string) Identity {
	t.Helper()
	id, err := Resolve(dir)
	if err != nil {
		t.Fatalf("Resolve(%s): unexpected error: %v", dir, err)
	}
	return id
}

// derivableNamesFor is the DerivableNames wrapper's body, inlined for
// tests now that the exported wrapper is gone: discover dir's repository,
// then list what it derives.
func derivableNamesFor(t *testing.T, dir string) []DerivedName {
	t.Helper()
	repo, err := discover(dir)
	if err != nil {
		t.Fatalf("discover(%s): %v", dir, err)
	}
	names, err := derivableNames(repo)
	if err != nil {
		t.Fatalf("derivableNames(%s): %v", dir, err)
	}
	return names
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

	got, err := AdoptWith(root, AdoptOptions{Established: established("widgets")})
	if err != nil {
		t.Fatalf("AdoptWith: %v", err)
	}
	if got.Identity.Project != "widgets" {
		t.Fatalf("an established alias must be adopted, not re-minted: got %q", got.Identity.Project)
	}
	if !got.Adopted {
		t.Fatal("Adopted must report that the identity came from storage, not from the chain")
	}
	if got.Identity.Rule != RuleRemote {
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

	got, err := AdoptWith(root, AdoptOptions{Established: established()})
	if err != nil {
		t.Fatalf("AdoptWith: %v", err)
	}
	want := resolveIdentity(t, root)
	if got.Identity.Project != want.Project || got.Identity.Rule != want.Rule {
		t.Fatalf("with nothing established AdoptWith must equal Resolve: got %q (%s), want %q (%s)",
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
	write(t, filepath.Join(root, DeclaredFileName), "acme-widgets\n")
	git(t, root, "remote", "add", "origin", "https://github.com/acme/widgets.git")

	got, err := AdoptWith(root, AdoptOptions{Established: established("acme-widgets", "widgets")})
	if err != nil {
		t.Fatalf("AdoptWith: %v", err)
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
	fromMain, err := AdoptWith(root, AdoptOptions{Established: e})
	if err != nil {
		t.Fatalf("AdoptWith(main): %v", err)
	}
	fromWorktree, err := AdoptWith(wt, AdoptOptions{Established: e})
	if err != nil {
		t.Fatalf("AdoptWith(worktree): %v", err)
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

	_, err := AdoptWith(root, AdoptOptions{Established: func(string) (bool, error) { return false, boom }})
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

	got, err := AdoptWith(root, AdoptOptions{
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
	write(t, filepath.Join(root, DeclaredFileName), "current-name\n")

	got, err := AdoptWith(root, AdoptOptions{
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

// derivableNames is what the caller writes to the ledger, so it must report
// the rule and whether each name may later be adopted on the ledger's word
// alone. The loose spellings may not: a bare directory name can name
// somebody else's repository.
func TestDerivableNames_MarksLooseSpellingsUnadoptable(t *testing.T) {
	tmp := t.TempDir()
	root := newRepo(t, tmp, "widgets")
	git(t, root, "remote", "add", "origin", "https://github.com/acme/widgets.git")

	names := derivableNamesFor(t, root)

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
