package guard

import (
	"strings"
	"testing"
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
var rings = map[string]ring{
	".":               ringSupport,
	"assets":          ringAdapter,
	"capability":      ringDomain,
	"cmd":             ringRoot,
	"filelock":        ringAdapter,
	"gadu":            ringAdapter,
	"gate":            ringDomain,
	"gitprov":         ringAdapter,
	"goal":            ringDomain,
	"installer":       ringSupport,
	"jsonstrict":      ringDomain,
	"memoryscope":     ringDomain,
	"pathguard":       ringDomain,
	"pipkg":           ringAdapter,
	"prespec":         ringDomain,
	"projection":      ringDomain,
	"propagator":      ringDomain,
	"reviewreceipt":   ringDomain,
	"roles":           ringDomain,
	"runtime":         ringAdapter,
	"settings":        ringAdapter,
	"shaper":          ringDomain,
	"shelltest":       ringSupport,
	"skills":          ringDomain,
	"synctrigger":     ringAdapter,
	"workflow":        ringDomain,
	"workflowprofile": ringDomain,
}

// knownDebt is every violation of the rule that exists today, each owed to the
// work unit that removes it. It only shrinks: a unit deletes its lines when it
// lands, and a line whose violation is gone fails the guard, so none can be left
// behind. A new violation is not added here to make the guard pass; it is fixed.
var knownDebt = []debt{
	// capability: the presence prober and the evidence check read the file system
	// and the platform; they move to capability/presence and capabilitytest.
	{"capability", "go/ast", "H9"},
	{"capability", "go/parser", "H9"},
	{"capability", "go/token", "H9"},
	{"capability", "os", "H9"},
	{"capability", "runtime", "H9"},

	// pathguard: the file system half (symlink resolution) moves to
	// pathguard/fsresolve, leaving the pure containment rules.
	{"pathguard", "os", "H22"},
	{"pathguard", "path/filepath.EvalSymlinks", "H22"},

	// prespec: the clock and entropy are injected, the CLI handler moves to cmd.
	{"prespec", "crypto/rand", "H11"},
	{"prespec", "time.Now", "H11"},

	// projection: the binding store moves to projection/fsstore behind a
	// BindingStore port.
	{"projection", "os", "H8"},
	{"projection", "runtime", "H8"},
	{"projection", "syscall", "H8"},
	{"projection", "time.Now", "H8"},
	{"projection", "time.Sleep", "H8"},

	// reviewreceipt: the receipt scan and the git resolution move behind ports
	// and a reviewreceipt/fsstore adapter.
	{"reviewreceipt", "os", "H12"},
	{"reviewreceipt", "os/exec", "H12"},

	// roles: the chain store moves to roles/filechain.
	{"roles", "os", "H7"},
	{"roles", "runtime", "H7"},
	{"roles", "syscall", "H7"},

	// shaper: owns WorktreeProvenance instead of importing gitprov (H4); the
	// contained reads and the clearance store move to shaper/fsadapter (H10).
	{"shaper", "gitprov", "H4"},
	{"shaper", "os", "H10"},
	{"shaper", "runtime", "H10"},
	{"shaper", "syscall", "H10"},
	{"shaper", "unsafe", "H10"},

	// skills: the concrete file system moves to skills/skillsfs.
	{"skills", "os", "H17"},
	{"skills", "path/filepath.WalkDir", "H17"},

	// workflow: the event log store moves to workflow/filelog.
	{"workflow", "os", "H6"},
	{"workflow", "runtime", "H6"},
	{"workflow", "syscall", "H6"},
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
