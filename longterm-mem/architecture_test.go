package guard

import (
	"strings"
	"testing"
)

// rings declares the layer of every package of this module (docs/architecture/
// hexagonal-target.md, section 1, row "Memory (longterm-mem)"). A package missing
// here fails the guard, so a new package starts with a decision about where it
// sits; a row whose package is gone fails it too.
//
// Where the document leaves a package open the choice is made here:
//
//   - query and ops are application packages: use cases that own the ports they
//     read through (L1, L3, L4).
//   - ingest, projectid, promote, skillstale and staleness are domain packages:
//     their rules are the part that stays; the file system, git and SQLite they
//     reach today are debt below.
//   - vaultreg is an adapter. L9 makes its Resolve pure, but the registry file
//     store stays in the package, so it cannot be a domain package without a
//     debt that L9 does not clear.
//   - engram is an adapter even though the domain types still live in it (L1
//     moves them to internal/memory).
//   - the module root (this guard) and internal/ops/testdata (a Go package the
//     other tests import) are test support.
var rings = map[string]ring{
	".":                       ringSupport,
	"cmd/longterm-mem":        ringRoot,
	"internal/durable":        ringAdapter,
	"internal/embed":          ringAdapter,
	"internal/engram":         ringAdapter,
	"internal/identityledger": ringAdapter,
	"internal/ingest":         ringDomain,
	"internal/mcpserver":      ringAdapter,
	"internal/ops":            ringApplication,
	"internal/ops/testdata":   ringSupport,
	"internal/projectid":      ringDomain,
	"internal/promote":        ringDomain,
	"internal/query":          ringApplication,
	"internal/register":       ringAdapter,
	"internal/repohistory":    ringAdapter,
	"internal/skillstale":     ringDomain,
	"internal/staleness":      ringDomain,
	"internal/vault":          ringAdapter,
	"internal/vaultreg":       ringAdapter,
	"internal/vecindex":       ringAdapter,
}

// knownDebt is every violation of the rule that exists today, each owed to the
// work unit that removes it. It only shrinks: a unit deletes its lines when it
// lands, and a line whose violation is gone fails the guard, so none can be left
// behind. A new violation is not added here to make the guard pass; it is fixed.
var knownDebt = []debt{
	// ingest: the canonical origin of a file reads the working directory. The C17
	// audit read ingest as pure from its imports; the call is the debt. L11 makes
	// the origin a value the source adapter resolves.
	{"internal/ingest", "path/filepath.Abs", "L11"},

	// ops: Doctor and Status read the vault and the vector index directly. L3 puts
	// the vault behind a port. L4 owns the index repository port; the C17 table
	// names no unit for the embed and vecindex edges of ops, and L4 is assigned
	// because ops would consume the same port and typed errors as query.
	{"internal/ops", "internal/embed", "L4"},
	{"internal/ops", "internal/vecindex", "L4"},
	{"internal/ops", "os", "L3"},

	// projectid: the git and file system reads move to a RepositoryInspector
	// adapter.
	{"internal/projectid", "os", "L7"},
	{"internal/projectid", "path/filepath.Abs", "L7"},
	{"internal/projectid", "path/filepath.EvalSymlinks", "L7"},

	// promote: the engram types move to internal/memory (L1), the address
	// allocator and the clock become injected (L2), the vault file system moves to
	// vaultfs (L3).
	{"internal/promote", "internal/durable", "L3"},
	{"internal/promote", "internal/engram", "L1"},
	{"internal/promote", "internal/vault", "L2"},
	{"internal/promote", "os", "L3"},
	{"internal/promote", "time.Now", "L2"},

	// query: reads through ports it owns instead of the concrete adapters.
	{"internal/query", "internal/embed", "L4"},
	{"internal/query", "internal/engram", "L1"},
	{"internal/query", "internal/vault", "L4"},
	{"internal/query", "internal/vecindex", "L4"},

	// skillstale: pure Detect over its inputs, with the lock, the facts, the
	// command resolver and the clock supplied by the caller. The repohistory
	// states it reads become its own facts type.
	{"internal/skillstale", "internal/engram", "L1"},
	{"internal/skillstale", "internal/repohistory", "L5"},
	{"internal/skillstale", "os", "L5"},
	{"internal/skillstale", "path/filepath.EvalSymlinks", "L5"},
	{"internal/skillstale", "time.Now", "L5"},

	// staleness: reads the repository through a RepoEvidence port.
	{"internal/staleness", "internal/engram", "L1"},
	{"internal/staleness", "internal/repohistory", "L6"},
	{"internal/staleness", "os", "L6"},
	{"internal/staleness", "path/filepath.WalkDir", "L6"},
}

// TestArchitectureFollowsTheDependencyRule is the fitness function: it fails on
// a package without a ring, on a violation that is not known debt, and on debt
// that is no longer a violation.
func TestArchitectureFollowsTheDependencyRule(t *testing.T) {
	c, err := newChecker(".", rings)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.packages) == 0 {
		t.Fatal("no production packages found under the module root; the walk may be broken")
	}

	for _, dir := range c.undeclared() {
		t.Errorf("package %s has no ring: declare it in rings (domain, application, adapter, root or support)", dir)
	}
	for _, dir := range c.missing() {
		t.Errorf("rings declares %s but no such package exists: delete the row", dir)
	}
	for _, problem := range validateDebts(knownDebt, rings) {
		t.Error(problem)
	}

	unlisted, stale := reconcile(c.violations(), knownDebt)
	for _, v := range unlisted {
		t.Errorf("%s breaks the dependency rule: %s (in %s). "+
			"Move the code behind a port instead; if it is existing debt owed to a work unit of "+
			"docs/architecture/hexagonal-target.md, add it to knownDebt with that unit's id",
			v.edge(), v.rule, strings.Join(v.files, ", "))
	}
	for _, d := range stale {
		t.Errorf("known debt %s (%s) is no longer a violation: delete the line from knownDebt", d.edge(), d.unit)
	}
}
