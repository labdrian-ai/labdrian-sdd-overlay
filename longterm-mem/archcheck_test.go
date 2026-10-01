package guard

// The architecture fitness checker (Phase 9, H1). It enforces the dependency
// rule of docs/architecture/hexagonal-target.md, section 1:
//
//	composition root -> adapters -> application -> domain
//
// Imports point inward only. A domain package imports the pure standard library
// and other domain packages, nothing else: no operating system, process, network
// or file system access, no wall clock, no entropy, no third-party module and no
// adapter. This file is the checker; architecture_test.go declares the ring of
// every package and the debt that still violates the rule. The file is the same
// in engine and in longterm-mem, so a rule changes in both or in neither.
//
// How it works. It parses every non-test .go file under the module root, with
// go/parser and no go tool, so a build constraint cannot hide an import: a file
// for another platform counts. It reads two things from each file:
//
//   - its imports, judged against the ring of the importing package;
//   - for the few standard packages that are pure to import but not to use
//     (time, path/filepath, io/fs, fmt), the members it selects from them, so
//     that `time.Duration` passes and `time.Now` does not.
//
// Limits, stated so nobody mistakes the guard for more than it is:
//
//   - Granularity is the edge between a package and one import or member, not
//     the file and not the use. Once a package is allowed a debt edge, a second
//     use of the same import in that package passes until the work unit that owns
//     the debt lands and deletes the line.
//   - Member detection is syntactic, by the name the file gives the import. It
//     catches `time.Now()`, `var clock = time.Now` and an aliased import, and it
//     refuses a dot import of a guarded package because a dot import would hide
//     the member. A local identifier that shadows the import name is read as the
//     package, which fails closed: rename the identifier.
//   - It sees what a package selects, not what reaches it: a func value passed in
//     from another package is invisible, and so are ambient reads that live in a
//     pure-looking API (time.Local follows the TZ variable, for example). Review
//     is still needed for those.
//   - Only direct imports are judged. Transitive impurity needs an impure package
//     in the chain, and every package in the chain is judged in turn.
//   - Test files are not production code and are not read.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ring is the layer a package declares for itself.
type ring int

const (
	// ringDomain is the model and its rules. It imports the pure standard library
	// and other domain packages only.
	ringDomain ring = iota + 1
	// ringApplication holds use cases and the ports they own. Like the domain it is
	// pure, and it may also import domain packages.
	ringApplication
	// ringAdapter is code that talks to the world: files, processes, the network,
	// a database, a settings format. It may import anything inside the module
	// except the composition root and test support.
	ringAdapter
	// ringRoot is a composition root: a main package that builds the adapters and
	// wires them to the use cases.
	ringRoot
	// ringSupport is test-only code (a harness, a guard like this one). It is not
	// production code and has no rule.
	ringSupport
)

func (r ring) String() string {
	switch r {
	case ringDomain:
		return "domain"
	case ringApplication:
		return "application"
	case ringAdapter:
		return "adapter"
	case ringRoot:
		return "root"
	case ringSupport:
		return "support"
	}
	return "unknown ring"
}

// pure reports whether packages of the ring are held to the pure-standard-library
// rule.
func (r ring) pure() bool { return r == ringDomain || r == ringApplication }

// importable lists, for each ring, the rings inside the module that its packages
// may import.
var importable = map[ring][]ring{
	ringDomain:      {ringDomain},
	ringApplication: {ringDomain, ringApplication},
	ringAdapter:     {ringDomain, ringApplication, ringAdapter},
	ringRoot:        {ringDomain, ringApplication, ringAdapter},
	ringSupport:     {ringDomain, ringApplication, ringAdapter, ringRoot, ringSupport},
}

func (r ring) mayImport(target ring) bool {
	for _, allowed := range importable[r] {
		if allowed == target {
			return true
		}
	}
	return false
}

// describeImportable names what a ring may import, for a failure message.
func (r ring) describeImportable() string {
	names := make([]string, 0, len(importable[r]))
	for _, allowed := range importable[r] {
		names = append(names, allowed.String())
	}
	return strings.Join(names, " and ")
}

// pureStd is the standard library a domain or application package may import. It
// is a list to widen on purpose, never by accident: anything not here (os,
// os/exec, syscall, net and net/http, crypto/rand, math/rand, log, flag, runtime,
// embed, unsafe, the go/* parsers) acts on the process or its surroundings.
// Packages that are pure to import but not to use have their ambient members
// listed in ambientMembers.
var pureStd = map[string]bool{
	"bufio": true, "bytes": true, "cmp": true, "container/list": true, "context": true,
	"crypto/sha256": true, "crypto/sha512": true,
	"encoding/base64": true, "encoding/binary": true, "encoding/hex": true, "encoding/json": true,
	"errors": true, "fmt": true, "hash": true, "hash/fnv": true,
	"io": true, "io/fs": true, "maps": true, "math": true, "math/bits": true,
	"net/url": true, "path": true, "path/filepath": true, "reflect": true, "regexp": true,
	"slices": true, "sort": true, "strconv": true, "strings": true,
	"sync": true, "sync/atomic": true, "time": true,
	"unicode": true, "unicode/utf16": true, "unicode/utf8": true,
}

// ambientMembers lists, for the standard packages that are pure to import, the
// members that are not pure to use.
var ambientMembers = []struct {
	pkg   string
	why   string
	names []string
}{
	{"time", "reads the wall clock or sleeps; take the time as a value or inject a clock",
		[]string{"Now", "Since", "Until", "Sleep", "After", "AfterFunc", "Tick", "NewTicker", "NewTimer"}},
	{"path/filepath", "reaches the file system; walking and resolving paths belongs in an adapter",
		[]string{"Abs", "EvalSymlinks", "Glob", "Walk", "WalkDir"}},
	{"io/fs", "reads through a file system; a port, not the domain, walks one",
		[]string{"Glob", "ReadDir", "ReadFile", "Stat", "WalkDir"}},
	{"fmt", "uses the process's standard streams; take an io.Writer or io.Reader",
		[]string{"Print", "Printf", "Println", "Scan", "Scanf", "Scanln"}},
}

// ambientReason returns why pkg.member is not pure, or "" when it is.
func ambientReason(pkg, member string) string {
	for _, group := range ambientMembers {
		if group.pkg != pkg {
			continue
		}
		for _, name := range group.names {
			if name == member {
				return group.why
			}
		}
	}
	return ""
}

func isAmbientPackage(pkg string) bool {
	for _, group := range ambientMembers {
		if group.pkg == pkg {
			return true
		}
	}
	return false
}

type refKind int

const (
	refImport    refKind = iota // the file imports pkg
	refMember                   // the file selects member from pkg
	refDotImport                // the file dot-imports pkg
)

// reference is one thing a file takes from another package.
type reference struct {
	kind   refKind
	pkg    string // the import path as written
	member string // refMember only
	file   string // slash-separated, relative to the module root
}

// sourcePackage is the directory of a package and everything its production files
// reference.
type sourcePackage struct {
	dir  string // slash-separated, relative to the module root; "." is the root
	refs []reference
}

// violation is one edge of the dependency rule that is broken.
type violation struct {
	from   string   // the package directory
	target string   // an in-module directory, an import path, or pkg.Member
	rule   string   // what the edge breaks
	files  []string // the files that make the edge, sorted
}

func (v violation) edge() string { return v.from + " -> " + v.target }

// checker judges the production code of one module against the declared rings.
type checker struct {
	modulePath string
	rings      map[string]ring
	packages   []sourcePackage
}

func newChecker(root string, rings map[string]ring) (*checker, error) {
	modulePath, err := readModulePath(root)
	if err != nil {
		return nil, err
	}
	packages, err := loadPackages(root)
	if err != nil {
		return nil, err
	}
	return &checker{modulePath: modulePath, rings: rings, packages: packages}, nil
}

func readModulePath(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`), nil
		}
	}
	return "", fmt.Errorf("no module line in %s", filepath.Join(root, "go.mod"))
}

// loadPackages parses every production .go file under root. A directory with Go
// files of any kind is a package, so a test-only directory must declare a ring
// too. It skips what the go tool skips (hidden and underscore directories,
// vendor) and a nested module. It does not skip testdata: a Go package that
// lives there is code like any other.
func loadPackages(root string) ([]sourcePackage, error) {
	fset := token.NewFileSet()
	byDir := map[string]*sourcePackage{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirectory(root, p, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		dir := path.Dir(rel)
		pkg := byDir[dir]
		if pkg == nil {
			pkg = &sourcePackage{dir: dir}
			byDir[dir] = pkg
		}
		if strings.HasSuffix(p, "_test.go") {
			return nil // a test file makes its directory a package, but is not read
		}
		file, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		pkg.refs = append(pkg.refs, collectReferences(file, rel)...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	packages := make([]sourcePackage, 0, len(byDir))
	for _, pkg := range byDir {
		packages = append(packages, *pkg)
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].dir < packages[j].dir })
	return packages, nil
}

func skipDirectory(root, p, name string) bool {
	if p == root {
		return false
	}
	if name == "vendor" || name == "node_modules" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
		return true
	}
	_, err := os.Stat(filepath.Join(p, "go.mod"))
	return err == nil
}

// collectReferences reads the imports of a file and, for the guarded standard
// packages, the members it selects.
func collectReferences(file *ast.File, rel string) []reference {
	var refs []reference
	localNames := map[string]string{} // the name a file gives a guarded import -> its path
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		refs = append(refs, reference{kind: refImport, pkg: importPath, file: rel})
		name := importName(importPath)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		switch {
		case name == "_":
		case name == ".":
			if isAmbientPackage(importPath) {
				refs = append(refs, reference{kind: refDotImport, pkg: importPath, file: rel})
			}
		case isAmbientPackage(importPath):
			localNames[name] = importPath
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok {
			if importPath, ok := localNames[id.Name]; ok {
				refs = append(refs, reference{kind: refMember, pkg: importPath, member: sel.Sel.Name, file: rel})
			}
		}
		return true
	})
	return refs
}

var majorVersion = regexp.MustCompile(`^v[0-9]+$`)

// importName is the name a package gives itself when the import does not rename
// it: the last path element, or the one before a /vN major-version suffix.
func importName(importPath string) string {
	elements := strings.Split(importPath, "/")
	name := elements[len(elements)-1]
	if len(elements) > 1 && majorVersion.MatchString(name) {
		name = elements[len(elements)-2]
	}
	return name
}

// violations returns every broken edge in the packages that declare a ring, in a
// stable order. Packages without a ring are reported by undeclared, not here.
func (c *checker) violations() []violation {
	var out []violation
	for _, pkg := range c.packages {
		r, declared := c.rings[pkg.dir]
		if !declared || r == ringSupport {
			continue
		}
		out = append(out, c.judge(pkg, r)...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].edge() < out[j].edge() })
	return out
}

func (c *checker) judge(pkg sourcePackage, r ring) []violation {
	byTarget := map[string]*violation{}
	for _, ref := range pkg.refs {
		target, rule := c.judgeReference(r, ref)
		if rule == "" {
			continue
		}
		v := byTarget[target]
		if v == nil {
			v = &violation{from: pkg.dir, target: target, rule: rule}
			byTarget[target] = v
		}
		if !containsString(v.files, ref.file) {
			v.files = append(v.files, ref.file)
		}
	}
	out := make([]violation, 0, len(byTarget))
	for _, v := range byTarget {
		sort.Strings(v.files)
		out = append(out, *v)
	}
	return out
}

// judgeReference returns the display target of a reference and the rule it
// breaks, or an empty rule when it breaks none.
func (c *checker) judgeReference(r ring, ref reference) (target, rule string) {
	switch ref.kind {
	case refMember:
		target = ref.pkg + "." + ref.member
		if why := ambientReason(ref.pkg, ref.member); r.pure() && why != "" {
			rule = fmt.Sprintf("%s packages must be pure, and %s %s", r, target, why)
		}
		return target, rule
	case refDotImport:
		target = ref.pkg + " (dot import)"
		if r.pure() {
			rule = fmt.Sprintf("%s packages must not dot-import %s: a dot import hides which of its ambient members are used", r, ref.pkg)
		}
		return target, rule
	}
	if dir, inside := c.moduleDir(ref.pkg); inside {
		return dir, c.judgeInternalImport(r, dir)
	}
	if !r.pure() {
		return ref.pkg, ""
	}
	if isStandard(ref.pkg) {
		if pureStd[ref.pkg] {
			return ref.pkg, ""
		}
		return ref.pkg, fmt.Sprintf("%s packages may import only the pure standard library, and %s is not in pureStd", r, ref.pkg)
	}
	return ref.pkg, fmt.Sprintf("%s packages may import only the pure standard library and %s packages, not a third-party module", r, r.describeImportable())
}

func (c *checker) judgeInternalImport(r ring, dir string) string {
	got, declared := c.rings[dir]
	switch {
	case !declared:
		return fmt.Sprintf("%s declares no ring, so the rule cannot judge importing it", dir)
	case !r.mayImport(got):
		return fmt.Sprintf("%s packages may import only %s packages inside the module, and %s is %s", r, r.describeImportable(), dir, got)
	}
	return ""
}

// moduleDir maps an import path of this module to its directory under the root.
func (c *checker) moduleDir(importPath string) (string, bool) {
	if importPath == c.modulePath {
		return ".", true
	}
	return strings.CutPrefix(importPath, c.modulePath+"/")
}

// isStandard reports whether an import path belongs to the standard library: its
// first element has no dot, which is how the go tool tells them apart.
func isStandard(importPath string) bool {
	first, _, _ := strings.Cut(importPath, "/")
	return !strings.Contains(first, ".")
}

// undeclared lists the package directories that have production code and no ring.
func (c *checker) undeclared() []string {
	var dirs []string
	for _, pkg := range c.packages {
		if _, ok := c.rings[pkg.dir]; !ok {
			dirs = append(dirs, pkg.dir)
		}
	}
	return dirs
}

// missing lists the declared directories that have no production code: a row that
// outlived its package.
func (c *checker) missing() []string {
	present := map[string]bool{}
	for _, pkg := range c.packages {
		present[pkg.dir] = true
	}
	var dirs []string
	for dir := range c.rings {
		if !present[dir] {
			dirs = append(dirs, dir)
		}
	}
	sort.Strings(dirs)
	return dirs
}

func containsString(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

// debt is a known violation, owed to the work unit that removes it. The line is
// deleted by that unit; a line whose violation is gone fails the guard.
type debt struct {
	from   string // the package directory
	target string // as violation.target
	unit   string // the work unit id in docs/architecture/hexagonal-target.md
}

func (d debt) edge() string { return d.from + " -> " + d.target }

// reconcile splits the violations found into those with no debt line (new, and
// a failure) and returns the debt lines whose violation no longer exists (stale,
// also a failure).
func reconcile(found []violation, debts []debt) (unlisted []violation, stale []debt) {
	listed := map[string]bool{}
	for _, d := range debts {
		listed[d.edge()] = true
	}
	live := map[string]bool{}
	for _, v := range found {
		live[v.edge()] = true
		if !listed[v.edge()] {
			unlisted = append(unlisted, v)
		}
	}
	for _, d := range debts {
		if !live[d.edge()] {
			stale = append(stale, d)
		}
	}
	return unlisted, stale
}

var workUnitID = regexp.MustCompile(`^[HLTB][0-9]+$`)

// validateDebts returns a message for every debt line that cannot be trusted:
// no work unit, a package without a ring, a package whose ring has no rule to
// owe against, or a line listed twice.
func validateDebts(debts []debt, rings map[string]ring) []string {
	var problems []string
	seen := map[string]bool{}
	for _, d := range debts {
		r, declared := rings[d.from]
		switch {
		case !workUnitID.MatchString(d.unit):
			problems = append(problems, fmt.Sprintf("known debt %s: unit %q is not a work unit id such as H4 or L3", d.edge(), d.unit))
		case !declared:
			problems = append(problems, fmt.Sprintf("known debt %s: %s is not declared in rings", d.edge(), d.from))
		case !r.pure():
			problems = append(problems, fmt.Sprintf("known debt %s: only a domain or application package can owe debt, and %s is %s", d.edge(), d.from, r))
		case seen[d.edge()]:
			problems = append(problems, fmt.Sprintf("known debt %s is listed twice", d.edge()))
		}
		seen[d.edge()] = true
	}
	return problems
}
