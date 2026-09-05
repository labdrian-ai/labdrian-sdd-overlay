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
	repo, err := discover(dir)
	if err != nil {
		return Adoption{}, err
	}
	names, err := derivableNames(repo)
	if err != nil {
		return Adoption{}, err
	}

	var found []Identity
	for _, n := range names {
		ok, err := established(n.name)
		if err != nil {
			// "Could not ask" must never be recorded as "nothing is
			// there": that answer is indistinguishable from a genuinely
			// fresh repository, and acting on it mints the second pile.
			return Adoption{}, fmt.Errorf("projectid: checking whether %q already holds memory: %w", n.name, err)
		}
		if ok {
			found = append(found, repo.identity(n.name, n.rule))
		}
	}

	if len(found) == 0 {
		id, err := Resolve(dir)
		if err != nil {
			return Adoption{}, err
		}
		return Adoption{Identity: id}, nil
	}

	a := Adoption{Identity: found[0], Adopted: true}
	for _, other := range found[1:] {
		a.PendingIntegration = append(a.PendingIntegration, other.Project)
	}
	return a, nil
}

// derivedName is one name this repository derives, and the rule it came
// from.
type derivedName struct {
	name string
	rule Rule
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
func derivableNames(repo repository) ([]derivedName, error) {
	var names []derivedName
	seen := map[string]bool{}
	add := func(name string, rule Rule) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		names = append(names, derivedName{name: name, rule: rule})
	}

	declaredName, ok, err := declared(repo)
	if err != nil {
		return nil, err
	}
	if ok {
		add(declaredName, RuleDeclared)
	}

	if remoteName, ok := remote(repo.commonDir); ok {
		add(remoteName, RuleRemote)
		add(lastSegment(remoteName), RuleRemote)
	}

	commonDirID := repo.identity(repo.commonDir, RuleCommonDir)
	for _, n := range correspondingNames(commonDirID) {
		add(n, RuleCommonDir)
	}
	return names, nil
}
