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
//   - hookwire is the adapter of the Claude Code hook protocol (H14): it decodes what a hook is
//     handed and encodes what it answers, imports nothing of the module, and is the one home of
//     the hook's JSON tags; the policies take and return plain values.
//   - skills/registryyaml is the adapter of the skills domain's RegistryRepository (H15): the
//     YAML file of the registry, its reader and its writer, with the policy for what the reader
//     does not understand (H16). It never calls the domain's Validate: the domain judges what
//     the adapter returns.
//   - skills/skillsfs is the adapter of the skills domain's file-facing ports (H17): the tree of
//     skills an overlay keeps, and the files of a project and the staged writes of an overlay.
//     It is the one place engine/skills reaches the operating system through.
//   - skills/app holds the use cases of `engine skills` (H20): typed inputs and results over the
//     ports of the skills domain, no argument vector, no printing, no exit. The command line and
//     the words a person reads are the CLI adapter in cmd.
//   - skills/projectidentity is the adapter of the skills domain's ProjectIdentity port (H18): the
//     sources that name the project a directory is (the id the person gave, the origin remote read
//     from .git/config without running git, the name of the directory) and the chain that asks them
//     in order. The rule that reduces a remote url to a name is the identity module's, shared with
//     longterm-mem (D2); the order of the chain is the composition root's.
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
	"contract":                  archguard.Domain,
	"filelock":                  archguard.Adapter,
	"gadu":                      archguard.Adapter,
	"gate":                      archguard.Domain,
	"gitprov":                   archguard.Adapter,
	"goal":                      archguard.Domain,
	"hookwire":                  archguard.Adapter,
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
	"skills/app":                archguard.Application,
	"skills/projectidentity":    archguard.Adapter,
	"skills/registryyaml":       archguard.Adapter,
	"skills/skillsfs":           archguard.Adapter,
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
