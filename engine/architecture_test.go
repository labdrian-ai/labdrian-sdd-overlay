package guard

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/archguard"
)

// rings declares the layer of every package of this module (docs/architecture/
// hexagonal-target.md, section 1). A package missing here fails the guard, so a
// new package starts with a decision about where it sits; a row whose package is
// gone fails it too.
//
// Where the document leaves a package open the choice is made here:
//
//   - runtime is an adapter: today it holds the concrete runtime adapters, and
//     the pure core (Target vocabulary, Adapter port, prompt rules) becomes
//     runtime/core, a domain package, in H26.
//   - pathguard, capability, projection, workflow and the like are domain
//     packages because their pure half is the part that stays; the file system
//     half is debt below.
//   - assets, gadu, synctrigger, filelock, gitprov, statestore and atomicfile are
//     adapters: infrastructure the domain reaches only through a port.
//   - capability/presence is the adapter of the workflow's DependencyProber port:
//     it stats files and the PATH, and capability itself stays pure.
//   - shaper/fsadapter is the adapter of the shaper's file-facing ports: the
//     ClearanceStore and the ContainedSource (H10).
//   - reviewreceipt/fsstore is the adapter of the four ports of the review receipt
//     capture (H12): the transaction stores found through gitprov, the receipts, the
//     persisted receipts and the changes, all of them files.
//   - installer, shelltest, capabilitytest, shaper/shapertest (the documents the shaper's
//     tests share), reviewreceipt/receipttest (the review documents the receipt capture's
//     tests share) and this guard (the module root) are test-only.
var rings = map[string]archguard.Ring{
	".":                         archguard.Support,
	"assets":                    archguard.Adapter,
	"atomicfile":                archguard.Adapter,
	"capability":                archguard.Domain,
	"capability/presence":       archguard.Adapter,
	"capabilitytest":            archguard.Support,
	"cmd":                       archguard.Root,
	"filelock":                  archguard.Adapter,
	"gadu":                      archguard.Adapter,
	"gate":                      archguard.Domain,
	"gitprov":                   archguard.Adapter,
	"goal":                      archguard.Domain,
	"installer":                 archguard.Support,
	"jsonstrict":                archguard.Domain,
	"memoryscope":               archguard.Domain,
	"pathguard":                 archguard.Domain,
	"pipkg":                     archguard.Adapter,
	"prespec":                   archguard.Domain,
	"projection":                archguard.Domain,
	"projection/fsstore":        archguard.Adapter,
	"propagator":                archguard.Domain,
	"reviewreceipt":             archguard.Domain,
	"reviewreceipt/fsstore":     archguard.Adapter,
	"reviewreceipt/receipttest": archguard.Support,
	"roles":                     archguard.Domain,
	"roles/filechain":           archguard.Adapter,
	"runtime":                   archguard.Adapter,
	"settings":                  archguard.Adapter,
	"shaper":                    archguard.Domain,
	"shaper/fsadapter":          archguard.Adapter,
	"shaper/shapertest":         archguard.Support,
	"shelltest":                 archguard.Support,
	"skills":                    archguard.Domain,
	"statestore":                archguard.Adapter,
	"synctrigger":               archguard.Adapter,
	"workflow":                  archguard.Domain,
	"workflow/filelog":          archguard.Adapter,
	"workflowprofile":           archguard.Domain,
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
//   - cmd: skillsLockWait and the other global seams of main.go (H31)
var knownDebt = archguard.Debt{
	// pathguard: the file system half (symlink resolution) moves to
	// pathguard/fsresolve, leaving the pure containment rules.
	"pathguard": {
		"os":                         "H22",
		"path/filepath.EvalSymlinks": "H22",
	},

	// skills: the concrete file system moves to skills/skillsfs.
	"skills": {
		"os":                    "H17",
		"path/filepath.WalkDir": "H17",
	},
}

// TestArchitectureFollowsTheDependencyRule is the fitness function: it fails on
// a package without a ring, on a violation that is not known debt, and on debt
// that is no longer a violation.
func TestArchitectureFollowsTheDependencyRule(t *testing.T) {
	problems, err := archguard.Check(".", rings, knownDebt)
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}
