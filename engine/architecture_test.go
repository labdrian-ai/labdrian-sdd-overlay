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
//   - installer, shelltest and this guard (the module root) are test-only.
var rings = map[string]archguard.Ring{
	".":               archguard.Support,
	"assets":          archguard.Adapter,
	"atomicfile":      archguard.Adapter,
	"capability":      archguard.Domain,
	"cmd":             archguard.Root,
	"filelock":        archguard.Adapter,
	"gadu":            archguard.Adapter,
	"gate":            archguard.Domain,
	"gitprov":         archguard.Adapter,
	"goal":            archguard.Domain,
	"installer":       archguard.Support,
	"jsonstrict":      archguard.Domain,
	"memoryscope":     archguard.Domain,
	"pathguard":       archguard.Domain,
	"pipkg":           archguard.Adapter,
	"prespec":         archguard.Domain,
	"projection":      archguard.Domain,
	"propagator":      archguard.Domain,
	"reviewreceipt":   archguard.Domain,
	"roles":           archguard.Domain,
	"runtime":         archguard.Adapter,
	"settings":        archguard.Adapter,
	"shaper":          archguard.Domain,
	"shelltest":       archguard.Support,
	"skills":          archguard.Domain,
	"statestore":      archguard.Adapter,
	"synctrigger":     archguard.Adapter,
	"workflow":        archguard.Domain,
	"workflowprofile": archguard.Domain,
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
//   - projection.lockWait (H8)
//   - cmd: skillsLockWait and the other global seams of main.go (H31)
//   - shaper: the global open hook of the clearance store (H10)
//   - reviewreceipt: the global store path variable (H12)
var knownDebt = archguard.Debt{
	// capability: the presence prober and the evidence check read the file system
	// and the platform; they move to capability/presence and capabilitytest.
	"capability": {
		"go/ast":    "H9",
		"go/parser": "H9",
		"go/token":  "H9",
		"os":        "H9",
		"runtime":   "H9",
	},

	// pathguard: the file system half (symlink resolution) moves to
	// pathguard/fsresolve, leaving the pure containment rules.
	"pathguard": {
		"os":                         "H22",
		"path/filepath.EvalSymlinks": "H22",
	},

	// prespec: the clock and entropy are injected, the CLI handler moves to cmd.
	"prespec": {
		"crypto/rand": "H11",
		"time.Now":    "H11",
	},

	// projection: the binding store moves to projection/fsstore behind a
	// BindingStore port.
	"projection": {
		"os":         "H8",
		"runtime":    "H8",
		"syscall":    "H8",
		"time.Now":   "H8",
		"time.Sleep": "H8",
	},

	// reviewreceipt: the receipt scan and the git resolution move behind ports
	// and a reviewreceipt/fsstore adapter.
	"reviewreceipt": {
		"os":      "H12",
		"os/exec": "H12",
	},

	// roles: the chain store moves to roles/filechain.
	"roles": {
		"os":      "H7",
		"runtime": "H7",
		"syscall": "H7",
	},

	// shaper: owns WorktreeProvenance instead of importing gitprov (H4); the
	// contained reads and the clearance store move to shaper/fsadapter (H10).
	"shaper": {
		"gitprov": "H4",
		"os":      "H10",
		"runtime": "H10",
		"syscall": "H10",
		"unsafe":  "H10",
	},

	// skills: the concrete file system moves to skills/skillsfs.
	"skills": {
		"os":                    "H17",
		"path/filepath.WalkDir": "H17",
	},

	// workflow: the event log store moves to workflow/filelog.
	"workflow": {
		"os":      "H6",
		"runtime": "H6",
		"syscall": "H6",
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
