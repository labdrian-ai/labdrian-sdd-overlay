package review

import (
	"regexp"
	"slices"
	"strings"
)

// This file is what the sweep of a commit computes from text, without git or any tool (own-rdd T6b2, S6 b): the names a
// diff removed, and the findings of an analyzer that a diff added. `review sweep` hands it the lines and the output
// of the tools; the checks themselves (T1-T3 of the sweeper) run in the app.

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
