package review

import (
	"path"
	"regexp"
	"slices"
	"strings"
)

// This file is what the sweep of a commit computes from text, without git or any tool (own-rdd T6b2, S6 b): the names a
// diff removed, and the findings of an analyzer that a diff added. `review sweep` hands it the lines and the output
// of the tools; the checks themselves (T1-T4 of the sweeper) run in the app.

// ChangedLines are the lines of one file that a diff removed, or that it added: the path as git prints it and the
// lines without the leading '-' or '+'.
type ChangedLines struct {
	Path  string
	Lines []string
}

// The declarations of a name that RemovedNames reads, one per line. They are the shapes S6 (b) lists, nothing more.
// A Go declaration is read only when it starts the line (column 0, where gofmt writes a package-level declaration):
// a declaration local to a function (`var err error` in a body) and a name inside a grouped `const (` or `var (`
// block are indented, so they are not read (a known limit of S6 b, L52: a removed grouped name is not swept). A
// shell declaration may be indented, since a function defined inside another is global once run. A form these do
// not match is not found either (an assumption of S6 b: the skill's "removed name" is not defined).
var (
	goFunc  = regexp.MustCompile(`^func\s+(?:\([^)]*\)\s*)?([A-Za-z_]\w*)`)
	goType  = regexp.MustCompile(`^type\s+([A-Za-z_]\w*)`)
	goNames = regexp.MustCompile(`^(?:const|var)\s+([A-Za-z_]\w*(?:\s*,\s*[A-Za-z_]\w*)*)`)
	shKey   = regexp.MustCompile(`^\s*function\s+([A-Za-z_]\w*)`)
	shParen = regexp.MustCompile(`^\s*([A-Za-z_]\w*)\s*\(\)`)
)

// RemovedNames is the names declared on a removed line of a .go or .sh file that no added line of the diff declares
// again, once each, in alphabetical order: a function (or method), a type, a constant or a variable of Go declared at
// column 0 (not a local or a grouped entry), and a function of shell (`N()` or `function N`, possibly indented). The
// blank name `_` is not one. A file of another kind declares nothing.
func RemovedNames(removed, added []ChangedLines) []string {
	again := map[string]bool{}
	for _, name := range declaredNames(added) {
		again[name] = true
	}
	var gone []string
	for _, name := range declaredNames(removed) {
		if !again[name] && !slices.Contains(gone, name) {
			gone = append(gone, name)
		}
	}
	slices.Sort(gone)
	return gone
}

// declaredNames is every name the lines of the .go and .sh files declare.
func declaredNames(files []ChangedLines) []string {
	var names []string
	for _, f := range files {
		var patterns []*regexp.Regexp
		switch {
		case strings.HasSuffix(f.Path, ".go"):
			patterns = []*regexp.Regexp{goFunc, goType, goNames}
		case strings.HasSuffix(f.Path, ".sh"):
			patterns = []*regexp.Regexp{shKey, shParen}
		}
		for _, line := range f.Lines {
			for _, p := range patterns {
				m := p.FindStringSubmatch(line)
				if m == nil {
					continue
				}
				for _, name := range strings.Split(m[1], ",") {
					if name = strings.TrimSpace(name); name != "_" {
						names = append(names, name)
					}
				}
			}
		}
	}
	return names
}

// RemovedFlags is the flags held by a removed line of a .go or .sh file that no added line of a .go or .sh file holds,
// once each, in alphabetical order. A flag is `--` and a lowercase letter, then lowercase letters, digits and single
// hyphens, not ending with a hyphen, with no letter, digit, `_` or `-` right before or after it (S15 d).
func RemovedFlags(removed, added []ChangedLines) []string {
	again := map[string]bool{}
	for _, flag := range flagsOf(added) {
		again[flag] = true
	}
	var gone []string
	for _, flag := range flagsOf(removed) {
		if !again[flag] && !slices.Contains(gone, flag) {
			gone = append(gone, flag)
		}
	}
	slices.Sort(gone)
	return gone
}

// flagsOf is every flag on the lines of the .go and .sh files.
func flagsOf(files []ChangedLines) []string {
	var flags []string
	for _, f := range files {
		if !strings.HasSuffix(f.Path, ".go") && !strings.HasSuffix(f.Path, ".sh") {
			continue
		}
		for _, line := range f.Lines {
			for i := 0; i+2 < len(line); i++ {
				if line[i] != '-' || line[i+1] != '-' || (i > 0 && inFlagWord(line[i-1])) {
					continue
				}
				if end := flagEnd(line, i+2); end > 0 {
					flags = append(flags, line[i:end])
					i = end - 1
				}
			}
		}
	}
	return flags
}

// flagEnd is where the flag whose name starts at start ends in line, or 0 when no flag starts there.
func flagEnd(line string, start int) int {
	if !isLower(line[start]) {
		return 0
	}
	end := start + 1
	for end < len(line) {
		c := line[end]
		if isLower(c) || isDigit(c) || (c == '-' && end+1 < len(line) && (isLower(line[end+1]) || isDigit(line[end+1]))) {
			end++
			continue
		}
		break
	}
	if end < len(line) && inFlagWord(line[end]) {
		return 0
	}
	return end
}

func isLower(c byte) bool { return c >= 'a' && c <= 'z' }
func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// inFlagWord is a character that may not touch a flag: a letter, a digit, `_` or `-`.
func inFlagWord(c byte) bool {
	return isLower(c) || isDigit(c) || (c >= 'A' && c <= 'Z') || c == '_' || c == '-'
}

// Leftovers is the changed paths whose base name ends .orig or .bak and that the commit does not delete, sorted, once
// each (S15 b). The slice is empty, never nil.
func Leftovers(paths, deleted []string) []string {
	left := []string{}
	for _, p := range paths {
		name := p[strings.LastIndex(p, "/")+1:]
		if (strings.HasSuffix(name, ".orig") || strings.HasSuffix(name, ".bak")) && !slices.Contains(deleted, p) && !slices.Contains(left, p) {
			left = append(left, p)
		}
	}
	slices.Sort(left)
	return left
}

// FixturePackages is the packages of a commit for the fixture check (S15 c): the directory of each changed .go path and,
// for a changed path with a testdata element, the directory above the first testdata ("." at the root). Sorted, once
// each. A path that is neither gives none.
func FixturePackages(paths []string) []string {
	var packages []string
	for _, p := range paths {
		pkg, ok := "", false
		if dir, _, found := strings.Cut(p, "/testdata/"); found {
			pkg, ok = dir, true
		} else if strings.HasPrefix(p, "testdata/") {
			pkg, ok = ".", true
		} else if strings.HasSuffix(p, ".go") {
			pkg, ok = path.Dir(p), true
		}
		if ok && !slices.Contains(packages, pkg) {
			packages = append(packages, pkg)
		}
	}
	slices.Sort(packages)
	return packages
}

// FixtureNames is the names of a fixture, a path of the form <package>/testdata/...: its base name, then each directory
// between testdata and it, outermost first (S15 c).
func FixtureNames(fixture string) []string {
	below, found := strings.CutPrefix(fixture, "testdata/")
	if !found {
		_, below, _ = strings.Cut(fixture, "/testdata/")
	}
	parts := strings.Split(below, "/")
	names := []string{parts[len(parts)-1]}
	return append(names, parts[:len(parts)-1]...)
}

// OrphanedFixtures is the fixtures of packages among files, the paths of a tree, that no .go file directly in their
// package names (S15 c): mentioned says, for a name, the paths of the tree that hold it as a whole word. Sorted, once
// each. The slice is empty, never nil.
func OrphanedFixtures(files, packages []string, mentioned map[string][]string) []string {
	orphaned := []string{}
	for _, pkg := range packages {
		prefix := "testdata/"
		if pkg != "." {
			prefix = pkg + "/testdata/"
		}
		for _, f := range files {
			if !strings.HasPrefix(f, prefix) || slices.Contains(orphaned, f) {
				continue
			}
			read := false
			for _, name := range FixtureNames(f) {
				for _, hit := range mentioned[name] {
					if strings.HasSuffix(hit, ".go") && path.Dir(hit) == pkg {
						read = true
					}
				}
			}
			if !read {
				orphaned = append(orphaned, f)
			}
		}
	}
	slices.Sort(orphaned)
	return orphaned
}

// position is the `:<line>` or `:<line>:<col>` that follows the path at the start of a finding.
var position = regexp.MustCompile(`^([^\s:]+):\d+(?::\d+)?`)

// NormalizeFinding is a line of an analyzer without what belongs to the copy it ran on and to where it found
// something: the root of the copy is removed wherever it occurs, and the line and the column that follow the path at
// the start of the line are removed. The same finding in the copy of the base and in the copy of the head then reads
// the same.
func NormalizeFinding(root, line string) string {
	if root = strings.TrimRight(root, "/"); root != "" {
		line = strings.ReplaceAll(line, root+"/", "")
		line = strings.ReplaceAll(line, root, "")
	}
	return position.ReplaceAllString(line, "$1")
}

// NewFindings is the findings of an analyzer that the head added: the lines of its output on the copy of the base (at
// baseRoot) and on the copy of the head (at headRoot) are normalized (NormalizeFinding), and a line is new when it
// occurs more times at the head than at the base. Each new line is given once, in alphabetical order.
func NewFindings(baseRoot string, base []string, headRoot string, head []string) []string {
	count := map[string]int{}
	for _, line := range base {
		count[NormalizeFinding(baseRoot, line)]--
	}
	for _, line := range head {
		count[NormalizeFinding(headRoot, line)]++
	}
	var added []string
	for line, n := range count {
		if n > 0 {
			added = append(added, line)
		}
	}
	slices.Sort(added)
	return added
}
