package archguard

import (
	"fmt"
	"sort"
	"strings"
)

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
	rings      map[string]Ring
	packages   []sourcePackage
}

func newChecker(root string, rings map[string]Ring) (*checker, error) {
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

// violations returns every broken edge in the packages that declare a ring, in a
// stable order. Packages without a ring are reported by undeclared, not here.
func (c *checker) violations() []violation {
	var out []violation
	for _, pkg := range c.packages {
		r, declared := c.rings[pkg.dir]
		if !declared || r == Support {
			continue
		}
		out = append(out, c.judge(pkg, r)...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].edge() < out[j].edge() })
	return out
}

func (c *checker) judge(pkg sourcePackage, r Ring) []violation {
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
func (c *checker) judgeReference(r Ring, ref reference) (target, rule string) {
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

func (c *checker) judgeInternalImport(r Ring, dir string) string {
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

// undeclared lists the package directories that have Go code and no ring.
func (c *checker) undeclared() []string {
	var dirs []string
	for _, pkg := range c.packages {
		if _, ok := c.rings[pkg.dir]; !ok {
			dirs = append(dirs, pkg.dir)
		}
	}
	return dirs
}

// missing lists the declared directories that have no Go code: a row that outlived
// its package.
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
