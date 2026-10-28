package review_test

import (
	"slices"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

// T1 looks for the names a diff removed (own-rdd T6b2, S6 b): a name declared on a removed line of a .go or .sh file
// that no added line of the diff declares again.
func TestRemovedNamesAreTheDeclarationsOfRemovedLines(t *testing.T) {
	for _, tc := range []struct {
		name           string
		removed, added []review.ChangedLines
		want           []string
	}{
		{"a Go function", goLines("func Old(x int) int {"), nil, []string{"Old"}},
		{"a Go method", goLines("func (r *Repo) Fetch(id string) error {"), nil, []string{"Fetch"}},
		{"a Go generic function", goLines("func Map[T any](xs []T) []T {"), nil, []string{"Map"}},
		{"a Go type", goLines("type Thing struct {"), nil, []string{"Thing"}},
		{"a Go generic type", goLines("type Set[T comparable] map[T]bool"), nil, []string{"Set"}},
		{"a Go constant", goLines("const maxBytes = 10"), nil, []string{"maxBytes"}},
		{"a Go variable", goLines("var ErrGone = errors.New(\"gone\")"), nil, []string{"ErrGone"}},
		{"a Go variable list", goLines("var a, b int"), nil, []string{"a", "b"}},
		{"an indented declaration", goLines("\tfunc inner() {"), nil, []string{"inner"}},
		{"a shell function with parentheses", shLines("cleanup() {"), nil, []string{"cleanup"}},
		{"a shell function with the keyword", shLines("function build_all {"), nil, []string{"build_all"}},
		{"a shell function with both", shLines("function run_it() {"), nil, []string{"run_it"}},
		{"a line that declares nothing", goLines("\treturn Old(x)", "// func Commented()", "x := 1"), nil, nil},
		{"the blank name", goLines("var _ = unused", "func _() {"), nil, nil},
		{"a file that is neither Go nor shell", []review.ChangedLines{{Path: "README.md", Lines: []string{"func Old() {"}}}, nil, nil},
		{"a name an added line declares again", goLines("func Moved() {"), goLines("func Moved() {"), nil},
		{"a name an added line of another file declares again", goLines("func Moved() {"), []review.ChangedLines{{Path: "b/other.go", Lines: []string{"func Moved(x int) {"}}}, nil},
		{"only the name declared again is dropped", goLines("func Kept() {", "func Gone() {"), goLines("func Kept() {"), []string{"Gone"}},
		{"a name once, however many lines remove it", goLines("func Dup() {", "func Dup() {"), nil, []string{"Dup"}},
		{"the names in order of the sorted result", goLines("func Zed() {", "func Alpha() {"), nil, []string{"Alpha", "Zed"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := review.RemovedNames(tc.removed, tc.added); !slices.Equal(got, tc.want) {
				t.Errorf("RemovedNames() = %q, want %q", got, tc.want)
			}
		})
	}
}

func goLines(lines ...string) []review.ChangedLines {
	return []review.ChangedLines{{Path: "a/file.go", Lines: lines}}
}

func shLines(lines ...string) []review.ChangedLines {
	return []review.ChangedLines{{Path: "bin/tool.sh", Lines: lines}}
}

// A finding of a tool names the copy it ran on and the line it found, and neither is part of what it found: the same
// finding at the base and at the head is the same line once the root of the copy and the position are taken out.
func TestNormalizeFindingRemovesTheRootAndThePosition(t *testing.T) {
	const root = "/home/u/.local/state/labdrian/review-sweep/0123456789abcdef/head"
	for _, tc := range []struct{ name, line, want string }{
		{"a line and a column", root + "/engine/a.go:12:6: unreachable func: Old", "engine/a.go: unreachable func: Old"},
		{"a line only", root + "/engine/a.go:12: unused", "engine/a.go: unused"},
		{"the root inside the message", root + "/a.go:3:1: " + root + "/b.go is unused", "a.go: b.go is unused"},
		{"no position", "engine/a.go: unused", "engine/a.go: unused"},
		{"a number in the message stays", root + "/a.go:3:1: 12:30 is not a position", "a.go: 12:30 is not a position"},
		{"another root is left", "/other/a.go:3:1: x", "/other/a.go: x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := review.NormalizeFinding(root, tc.line); got != tc.want {
				t.Errorf("NormalizeFinding(%q) = %q, want %q", tc.line, got, tc.want)
			}
		})
	}
	if got, want := review.NormalizeFinding("", "a.go:3:1: x"), "a.go: x"; got != want {
		t.Errorf("NormalizeFinding with no root = %q, want %q", got, want)
	}
	if got, want := review.NormalizeFinding(root+"/", root+"/a.go:3:1: x"), "a.go: x"; got != want {
		t.Errorf("NormalizeFinding with a root that ends in a slash = %q, want %q", got, want)
	}
}

// A line is new when its normalized form occurs more times at the head than at the base (S6 b): a finding that moved
// is not new, a finding that was already there twice and is there thrice is.
func TestNewFindingsAreTheNormalizedLinesThatOccurMoreAtTheHead(t *testing.T) {
	const baseRoot, headRoot = "/s/base", "/s/head"
	for _, tc := range []struct {
		name       string
		base, head []string
		want       []string
	}{
		{"nothing", nil, nil, nil},
		{"the same finding on another line", []string{baseRoot + "/a.go:10:2: unused x"}, []string{headRoot + "/a.go:40:2: unused x"}, nil},
		{"a new finding", []string{baseRoot + "/a.go:10:2: unused x"}, []string{headRoot + "/a.go:10:2: unused x", headRoot + "/b.go:1:1: unused y"}, []string{"b.go: unused y"}},
		{"one more of a finding that was there", []string{baseRoot + "/a.go:1:1: u", baseRoot + "/a.go:2:1: u"}, []string{headRoot + "/a.go:1:1: u", headRoot + "/a.go:2:1: u", headRoot + "/a.go:3:1: u"}, []string{"a.go: u"}},
		{"fewer than at the base", []string{baseRoot + "/a.go:1:1: u", baseRoot + "/a.go:2:1: u"}, []string{headRoot + "/a.go:1:1: u"}, nil},
		{"each new line once, sorted", nil, []string{headRoot + "/z.go:1:1: z", headRoot + "/a.go:1:1: a", headRoot + "/a.go:9:1: a"}, []string{"a.go: a", "z.go: z"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := review.NewFindings(baseRoot, tc.base, headRoot, tc.head); !slices.Equal(got, tc.want) {
				t.Errorf("NewFindings() = %q, want %q", got, tc.want)
			}
		})
	}
}
