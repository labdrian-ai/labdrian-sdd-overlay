package guard

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/archguard"
)

// rings declares the layer of every package of this module (docs/architecture/
// hexagonal-target.md, section 1, row "Memory (longterm-mem)"). A package missing
// here fails the guard, so a new package starts with a decision about where it
// sits; a row whose package is gone fails it too.
//
// Where the document leaves a package open the choice is made here:
//
//   - query and ops are application packages: use cases that own the ports they
//     read through (L1 gave query its memory ports; L3 and L4 give the rest).
//   - ingest, projectid, promote, skillstale and staleness are domain packages:
//     their rules are the part that stays; the file system, git and SQLite they
//     reach today are debt below.
//   - vaultreg is an adapter. L9 makes its Resolve pure, but the registry file
//     store stays in the package, so it cannot be a domain package without a
//     debt that L9 does not clear.
//   - memory is the domain model of what longterm-mem reads (the observation, the
//     search shapes, the standing, the tokenizer and the snippet rule); engram is
//     an adapter that maps Engram's rows to it (L1). Only the composition root may
//     import engram (engram_adapter_guard_test.go).
//   - the module root (this guard) and internal/ops/testdata (a Go package the
//     other tests import) are test support.
var rings = map[string]archguard.Ring{
	".":                       archguard.Support,
	"cmd/longterm-mem":        archguard.Root,
	"internal/durable":        archguard.Adapter,
	"internal/embed":          archguard.Adapter,
	"internal/engram":         archguard.Adapter,
	"internal/identityledger": archguard.Adapter,
	"internal/ingest":         archguard.Domain,
	"internal/memory":         archguard.Domain,
	"internal/mcpserver":      archguard.Adapter,
	"internal/ops":            archguard.Application,
	"internal/ops/testdata":   archguard.Support,
	"internal/projectid":      archguard.Domain,
	"internal/promote":        archguard.Domain,
	"internal/query":          archguard.Application,
	"internal/register":       archguard.Adapter,
	"internal/repohistory":    archguard.Adapter,
	"internal/skillstale":     archguard.Domain,
	"internal/staleness":      archguard.Domain,
	"internal/vault":          archguard.Adapter,
	"internal/vaultreg":       archguard.Adapter,
	"internal/vecindex":       archguard.Adapter,
}

// knownDebt is every violation of the rule that exists today, each owed to the
// work unit that removes it: package, then the import or member it should not
// use, then the unit. It only shrinks: a unit deletes its lines when it lands, and
// a line whose violation is gone fails the guard, so none can be left behind. A
// new violation is not added here to make the guard pass; it is fixed.
//
// The guard reads imports and the members selected from a few standard packages.
// It cannot see package-level mutable variables used as test seams, so these
// known ones are not listed below and are owed to their units all the same:
//
//   - register: the saveInstallState seam (L10)
//   - vecindex: the global dirLocks map (L10), and its own flock helper,
//     acquireFileLock, which L10 should settle together with it
var knownDebt = archguard.Debt{
	// ingest: the canonical origin of a file reads the working directory. The C17
	// audit read ingest as pure from its imports; the call is the debt. L11 makes
	// the origin a value the source adapter resolves.
	"internal/ingest": {
		"path/filepath.Abs": "L11",
	},

	// ops: Doctor and Status read the vault and the vector index directly. L3 puts
	// the vault behind a port. L4 owns the index repository port; the C17 table
	// names no unit for the embed and vecindex edges of ops, and L4 is assigned
	// because ops would consume the same port and typed errors as query.
	"internal/ops": {
		"internal/embed":    "L4",
		"internal/vecindex": "L4",
		"os":                "L3",
	},

	// projectid: the git and file system reads move to a RepositoryInspector
	// adapter.
	"internal/projectid": {
		"os":                         "L7",
		"path/filepath.Abs":          "L7",
		"path/filepath.EvalSymlinks": "L7",
	},

	// promote: the clock is injected (L2, done). The address allocator becomes
	// injected in L2 as well and the vault file system moves to vaultfs (L3).
	"internal/promote": {
		"internal/durable": "L3",
		"internal/vault":   "L2",
		"os":               "L3",
	},

	// query: reads through ports it owns instead of the concrete adapters.
	"internal/query": {
		"internal/embed":    "L4",
		"internal/vault":    "L4",
		"internal/vecindex": "L4",
	},

	// skillstale: pure Detect over its inputs, with the lock, the facts, the
	// command resolver and the clock supplied by the caller. The repohistory
	// states it reads become its own facts type.
	"internal/skillstale": {
		"internal/repohistory":       "L5",
		"os":                         "L5",
		"path/filepath.EvalSymlinks": "L5",
		"time.Now":                   "L5",
	},

	// staleness: reads the repository through a RepoEvidence port.
	"internal/staleness": {
		"internal/repohistory":  "L6",
		"os":                    "L6",
		"path/filepath.WalkDir": "L6",
	},
}

// pureModules are the modules of this repository that the domain may import: the rules both this
// module and the engine need (the identity of a project) live in a module of the standard library
// alone, whose own test holds it to that (Phase 9, D2).
var pureModules = []string{"github.com/labdrian-ai/labdrian-sdd-overlay/identity"}

// TestArchitectureFollowsTheDependencyRule is the fitness function: it fails on
// a package without a ring, on a violation that is not known debt, and on debt
// that is no longer a violation.
func TestArchitectureFollowsTheDependencyRule(t *testing.T) {
	problems, err := archguard.CheckWith(".", rings, knownDebt, archguard.Options{PureModules: pureModules})
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}
