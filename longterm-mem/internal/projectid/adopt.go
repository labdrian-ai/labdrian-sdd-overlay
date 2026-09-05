package projectid

import "fmt"

// Established reports whether name already holds memory. It is the seam
// between this package, which knows what a repository DERIVES, and storage,
// which knows what is actually stored -- neither of which can answer the
// question alone.
type Established func(name string) (bool, error)

// Adoption is a resolved identity together with what integrating it owes.
type Adoption struct {
	// Identity is the name to use.
	Identity Identity
	// Adopted is true when Identity came from storage rather than from the
	// chain: a derivable alias already held memory, so that alias IS this
	// repository's established identity.
	Adopted bool
	// PendingIntegration lists the other derivable names that also hold
	// memory. Each is provably the same repository as Identity, because
	// every one of them came out of this one repository's own metadata in
	// a single read -- not out of a similarity score on their spellings.
	// They are REPORTED rather than merged: Engram's store is read-only to
	// this module (R-002), and merging is its owner's to perform.
	PendingIntegration []string
	// Derived is every name the repository derives right now, in rank
	// order, for the caller to persist so these names stay findable once
	// nothing derives them any more.
	Derived []DerivedName
}

// Adopt resolves dir's identity the way Resolve does, then lets storage
// override WHICH of the derivable names is used.
//
// The rule it implements: fragmented memory must be made one with the
// source of truth, and anything derivable must be integrated rather than
// left beside it. The cheapest form of that -- and the only one available
// before any merge -- is not creating the second pile. A repository whose
// remote normalizes to "github.com/acme/widgets" also derives the plain
// "widgets"; if the memory already lives under "widgets", then minting the
// URL-shaped name is the resolver fragmenting the repository itself. This
// is not a hypothetical: it is what shipped, and what this fixes.
//
// Adoption is deliberately narrow. It only ever selects a name this
// repository DERIVES -- never a name that merely looks similar to one --
// so it cannot merge two repositories whose names happen to resemble each
// other, which is the one mistake this package must never make. What it
// cannot prove, it reports.
func Adopt(dir string, established Established) (Adoption, error) {
	return AdoptWith(dir, AdoptOptions{Established: established})
}

// AdoptOptions carries what Adopt consults besides the repository itself.
type AdoptOptions struct {
	// Established reports whether a name already holds memory.
	Established Established
	// Remembered are names this repository was known by before, most
	// recently used first. They exist because derivation alone has a hole
	// in it: a repository that MOVES stops deriving its old path, and
	// memory stored under that path becomes unreachable -- silently. A name
	// the repository was known by is still its name.
	//
	// They rank BELOW everything the repository still derives. What it
	// looks like now is better evidence than what it looked like once, and
	// inverting that would let a stale name outrank a live declaration.
	Remembered []string
}

// AdoptWith is Adopt, plus the names the repository was known by before.
func AdoptWith(dir string, opts AdoptOptions) (Adoption, error) {
	repo, err := discover(dir)
	if err != nil {
		return Adoption{}, err
	}
	derived, err := derivableNames(repo)
	if err != nil {
		return Adoption{}, err
	}

	names := derived
	seen := make(map[string]bool, len(derived))
	for _, n := range derived {
		seen[n.Name] = true
	}
	for _, r := range opts.Remembered {
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		names = append(names, DerivedName{Name: r, Rule: RuleRemembered, Strict: true})
	}
	established := opts.Established

	var found []Identity
	for _, n := range names {
		ok, err := established(n.Name)
		if err != nil {
			// "Could not ask" must never be recorded as "nothing is
			// there": that answer is indistinguishable from a genuinely
			// fresh repository, and acting on it mints the second pile.
			return Adoption{}, fmt.Errorf("projectid: checking whether %q already holds memory: %w", n.Name, err)
		}
		if ok {
			found = append(found, repo.identity(n.Name, n.Rule))
		}
	}

	if len(found) == 0 {
		id, err := Resolve(dir)
		if err != nil {
			return Adoption{}, err
		}
		return Adoption{Identity: id, Derived: derived}, nil
	}

	a := Adoption{Identity: found[0], Adopted: true, Derived: derived}
	for _, other := range found[1:] {
		a.PendingIntegration = append(a.PendingIntegration, other.Project)
	}
	return a, nil
}

// DerivedName is one name a repository derives, and the rule that produced
// it. Callers persist these (see internal/identityledger) so a name stays
// findable once nothing derives it any more.
type DerivedName struct {
	Name string
	Rule Rule
	// Strict is whether this name can realistically name only THIS
	// repository -- a declared name, a normalized remote, an absolute
	// common-dir path. It is false for the looser spellings, a bare
	// directory name above all, which can equally name somebody else's
	// repository. Only strict names may later be adopted on a ledger's word
	// alone: adopting a loose one from memory would bind this repository to
	// another project's, silently. Refusing costs a reunion that stays
	// visible and is fixable with a declared file.
	Strict bool
}

// CommonDir returns the absolute, symlink-resolved git common directory of
// dir's repository: the one directory a main checkout and every linked
// worktree share, and the only sane place to keep a record that belongs to
// the repository rather than to one checkout of it.
func CommonDir(dir string) (string, error) {
	repo, err := discover(dir)
	if err != nil {
		return "", err
	}
	return repo.commonDir, nil
}

// DerivableNames returns every name dir's repository derives, in the rank
// order that decides which becomes canonical when several hold memory.
func DerivableNames(dir string) ([]DerivedName, error) {
	repo, err := discover(dir)
	if err != nil {
		return nil, err
	}
	return derivableNames(repo)
}

// derivableNames lists every name this repository derives, in the order
// that decides which of them becomes canonical when several are
// established: the chain's own order, and within each rule the strict
// identity before the looser spellings that correspondingNames already
// accepts as naming it.
//
// The looser spellings are here for the same reason CheckCorrespondence
// accepts them: two of the three rules produce identities no operator would
// ever type, so the name the memory actually lives under is far more often
// the plain one. Leaving them out would make adoption miss the only case it
// exists for.
func derivableNames(repo repository) ([]DerivedName, error) {
	var names []DerivedName
	seen := map[string]bool{}
	add := func(name string, rule Rule, strict bool) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		names = append(names, DerivedName{Name: name, Rule: rule, Strict: strict})
	}

	declaredName, ok, err := declared(repo)
	if err != nil {
		return nil, err
	}
	if ok {
		add(declaredName, RuleDeclared, true)
	}

	if remoteName, ok := remote(repo.commonDir); ok {
		add(remoteName, RuleRemote, true)
		add(lastSegment(remoteName), RuleRemote, false)
	}

	commonDirID := repo.identity(repo.commonDir, RuleCommonDir)
	for i, n := range correspondingNames(commonDirID) {
		// correspondingNames puts the strict identity -- the absolute
		// common-dir path -- first, and the loose directory name after it.
		add(n, RuleCommonDir, i == 0)
	}
	return names, nil
}
