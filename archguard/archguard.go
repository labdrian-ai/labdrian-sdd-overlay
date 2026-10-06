// Package archguard enforces the dependency rule of a hexagonal architecture on a
// Go module, from its tests:
//
//	composition root -> adapters -> application -> domain
//
// Imports point inward only. A domain package imports the pure standard library
// and other domain packages, nothing else: no operating system, process, network
// or file system access, no wall clock, no entropy, no third-party module and no
// adapter. An application package (use cases and the ports they own) is held to
// the same purity and may also import domain packages. An adapter may import
// anything inside the module except the composition root and test support.
//
// A module uses it with one test. The test declares the ring of every package in
// a table, and lists in a second table the violations that exist today, each owed
// to the work unit that removes it:
//
//	func TestArchitecture(t *testing.T) {
//		problems, err := archguard.Check(".", rings, knownDebt)
//		if err != nil {
//			t.Fatal(err)
//		}
//		for _, p := range problems {
//			t.Error(p)
//		}
//	}
//
// Check fails on a package with no ring, on a ring row with no package, on a
// violation that is not known debt, and on debt that is no longer a violation, so
// the debt table only shrinks: the unit that removes a violation deletes its line.
// The messages call the two tables rings and knownDebt, the names the callers use.
//
// How it works. It parses every Go file under the module root with go/parser and
// no go tool, so a build constraint cannot hide an import: a file for another
// platform counts. It reads two things from each production file:
//
//   - its imports, judged against the ring of the importing package;
//   - for the few standard packages that are pure to import but not to use (time,
//     path/filepath, io/fs, fmt), the members it selects from them, so that
//     time.Duration passes and time.Now does not.
//
// Limits, stated so nobody mistakes the guard for more than it is:
//
//   - Granularity is the edge between a package and one import or member, not the
//     file and not the use. Once a package is allowed a debt edge, a second use of
//     the same import in that package passes until the work unit that owns the debt
//     lands and deletes the line.
//   - Member detection is syntactic, by the name the file gives the import. It
//     catches time.Now(), var clock = time.Now and an aliased import, and it
//     refuses a dot import of a guarded package because a dot import would hide the
//     member. A local identifier that shadows the import name is read as the
//     package, which fails closed: rename the identifier.
//   - It sees what a package selects, not what reaches it: a func value passed in
//     from another package is invisible, and so are ambient reads that live in a
//     pure-looking API (time.Local follows the TZ variable, for example). Package
//     level mutable variables used as test seams are invisible too, because the
//     guard reads imports and selected members, not declarations. Review is still
//     needed for those.
//   - Only direct imports are judged. Transitive impurity needs an impure package
//     in the chain, and every package in the chain is judged in turn.
//   - Test files are not production code and are not read. A directory that holds
//     only test files is still a package and must declare a ring (Support).
//
// The module depends on the standard library only, so that a module can import it
// from its tests without taking anything else on.
package archguard

import (
	"fmt"
	"strings"
)

// Ring is the layer a package declares for itself.
type Ring int

const (
	// Domain is the model and its rules. It imports the pure standard library and
	// other domain packages only.
	Domain Ring = iota + 1
	// Application holds use cases and the ports they own. Like the domain it is
	// pure, and it may also import domain packages.
	Application
	// Adapter is code that talks to the world: files, processes, the network, a
	// database, a settings format. It may import anything inside the module except
	// the composition root and test support.
	Adapter
	// Root is a composition root: a main package that builds the adapters and
	// wires them to the use cases.
	Root
	// Support is test-only code (a harness, a guard like the architecture test
	// itself). It is not production code and has no rule.
	Support
)

func (r Ring) String() string {
	switch r {
	case Domain:
		return "domain"
	case Application:
		return "application"
	case Adapter:
		return "adapter"
	case Root:
		return "root"
	case Support:
		return "support"
	}
	return "unknown ring"
}

// Debt lists the violations that exist today: package directory, then the edge it
// breaks, then the id of the work unit that removes it. The edge is an import path
// (a directory relative to the module root for a package of the module), or
// "pkg.Member" for a member of a standard package, for example:
//
//	archguard.Debt{
//		"shaper":  {"gitprov": "H4", "os": "H10"},
//		"prespec": {"time.Now": "H11"},
//	}
//
// A map cannot list the same edge twice. Only a domain or application package can
// owe debt, and a unit id looks like H4, L3, T1 or B2.
type Debt map[string]map[string]string

// Check judges the module at root against the declared rings and the known debt.
// It returns one message per problem, in a stable order, and none when the module
// follows the rule. An error means the guard could not run: no go.mod at root, a
// Go file that does not parse, or no Go package under root at all.
func Check(root string, rings map[string]Ring, debt Debt) ([]string, error) {
	return CheckWith(root, rings, debt, Options{})
}

// Options tune a check beyond the rings and the debt.
type Options struct {
	// PureModules are modules outside the one being judged, named by module path, that the
	// caller vouches hold themselves to the pure standard library (the identity module of this
	// repository, whose own test says so): a domain or application package may import them and
	// any package of them, where any other module is third-party. Like the debt, the list says
	// only what exists: a module no package imports is reported, so the permission goes with the
	// last import.
	PureModules []string
}

// CheckWith is Check with options.
func CheckWith(root string, rings map[string]Ring, debt Debt, opts Options) ([]string, error) {
	c, err := newChecker(root, rings, opts.PureModules...)
	if err != nil {
		return nil, err
	}
	if len(c.packages) == 0 {
		return nil, fmt.Errorf("archguard: no Go packages under %s; the walk may be broken", root)
	}

	var problems []string
	for _, dir := range c.undeclared() {
		problems = append(problems, fmt.Sprintf("package %s has no ring: declare it in rings (domain, application, adapter, root or support)", dir))
	}
	for _, dir := range c.missing() {
		problems = append(problems, fmt.Sprintf("rings declares %s but no such package exists: delete the row", dir))
	}
	lines := debt.lines()
	problems = append(problems, validateDebts(lines, rings)...)

	unlisted, stale := reconcile(c.violations(), lines)
	for _, v := range unlisted {
		problems = append(problems, fmt.Sprintf("%s breaks the dependency rule: %s (in %s). "+
			"Move the code behind a port instead; if it is existing debt owed to a work unit, "+
			"add it to knownDebt with that unit's id",
			v.edge(), v.rule, strings.Join(v.files, ", ")))
	}
	for _, module := range c.unusedPureModules() {
		problems = append(problems, fmt.Sprintf("pure module %s is not imported by any package: delete it from Options.PureModules", module))
	}
	for _, d := range stale {
		problems = append(problems, fmt.Sprintf("known debt %s (%s) is no longer a violation: delete the line from knownDebt", d.edge(), d.unit))
	}
	return problems, nil
}
