package archguard

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// The tests in this file prove the checker on throwaway modules, so the rules are
// pinned by what they flag and what they let through rather than by the real tree
// of any module that uses archguard, whose violations change as work units land.

const fixtureModule = "example.test/m"

// writeFixtureModule writes a Go module under a temporary directory and returns
// its root. Keys are slash-separated paths relative to the root.
func writeFixtureModule(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	files["go.mod"] = "module " + fixtureModule + "\n\ngo 1.21\n"
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// fixtureEdges loads a fixture module and returns the sorted "from -> target"
// edges the checker reports for it.
func fixtureEdges(t *testing.T, rings map[string]Ring, files map[string]string) []string {
	t.Helper()
	root := writeFixtureModule(t, files)
	c, err := newChecker(root, rings)
	if err != nil {
		t.Fatalf("newChecker: %v", err)
	}
	var edges []string
	for _, v := range c.violations() {
		edges = append(edges, v.edge())
	}
	sort.Strings(edges)
	return edges
}

func TestCheckerFlagsTheDependencyRule(t *testing.T) {
	tests := []struct {
		name  string
		rings map[string]Ring
		files map[string]string
		want  []string
	}{
		{
			name:  "a domain package may use the pure standard library",
			rings: map[string]Ring{"dom": Domain},
			files: map[string]string{"dom/a.go": "package dom\nimport (\"fmt\"; \"strings\"; \"encoding/json\")\nvar _ = fmt.Sprint(strings.ToUpper(\"x\"))\nvar _ json.Marshaler\n"},
			want:  nil,
		},
		{
			name:  "a domain package must not import os, os/exec, syscall or net",
			rings: map[string]Ring{"dom": Domain},
			files: map[string]string{"dom/a.go": "package dom\nimport (\"net\"; \"os\"; \"os/exec\"; \"syscall\")\nvar _ = []any{net.IPv4len, os.Args, exec.ErrDot, syscall.EINVAL}\n"},
			want:  []string{"dom -> net", "dom -> os", "dom -> os/exec", "dom -> syscall"},
		},
		{
			name:  "a domain package must not import crypto/rand",
			rings: map[string]Ring{"dom": Domain},
			files: map[string]string{"dom/a.go": "package dom\nimport \"crypto/rand\"\nvar _ = rand.Reader\n"},
			want:  []string{"dom -> crypto/rand"},
		},
		{
			name:  "a domain package may import another domain package",
			rings: map[string]Ring{"dom": Domain, "other": Domain},
			files: map[string]string{
				"dom/a.go":   "package dom\nimport \"" + fixtureModule + "/other\"\nvar _ = other.X\n",
				"other/b.go": "package other\nvar X = 1\n",
			},
			want: nil,
		},
		{
			name:  "a domain package must not import an adapter",
			rings: map[string]Ring{"dom": Domain, "ad": Adapter},
			files: map[string]string{
				"dom/a.go": "package dom\nimport \"" + fixtureModule + "/ad\"\nvar _ = ad.X\n",
				"ad/b.go":  "package ad\nvar X = 1\n",
			},
			want: []string{"dom -> ad"},
		},
		{
			name:  "a domain package must not import a third-party module",
			rings: map[string]Ring{"dom": Domain},
			files: map[string]string{"dom/a.go": "package dom\nimport _ \"github.com/someone/lib\"\n"},
			want:  []string{"dom -> github.com/someone/lib"},
		},
		{
			name:  "an application package may use domain packages but not adapters",
			rings: map[string]Ring{"app": Application, "dom": Domain, "ad": Adapter},
			files: map[string]string{
				"app/a.go": "package app\nimport (\"" + fixtureModule + "/dom\"; \"" + fixtureModule + "/ad\")\nvar _ = []int{dom.X, ad.X}\n",
				"dom/b.go": "package dom\nvar X = 1\n",
				"ad/c.go":  "package ad\nvar X = 1\n",
			},
			want: []string{"app -> ad"},
		},
		{
			name:  "a domain package must not import an application package",
			rings: map[string]Ring{"dom": Domain, "app": Application},
			files: map[string]string{
				"dom/a.go": "package dom\nimport \"" + fixtureModule + "/app\"\nvar _ = app.X\n",
				"app/b.go": "package app\nvar X = 1\n",
			},
			want: []string{"dom -> app"},
		},
		{
			name:  "an adapter may use the operating system, third-party modules and the inner rings",
			rings: map[string]Ring{"ad": Adapter, "dom": Domain},
			files: map[string]string{
				"ad/a.go":  "package ad\nimport (\"os\"; \"os/exec\"; _ \"github.com/someone/lib\"; \"" + fixtureModule + "/dom\")\nvar _ = []any{os.Args, exec.ErrDot, dom.X}\n",
				"dom/b.go": "package dom\nvar X = 1\n",
			},
			want: nil,
		},
		{
			name:  "an adapter must not import a support package",
			rings: map[string]Ring{"ad": Adapter, "sup": Support},
			files: map[string]string{
				"ad/a.go":  "package ad\nimport \"" + fixtureModule + "/sup\"\nvar _ = sup.X\n",
				"sup/b.go": "package sup\nvar X = 1\n",
			},
			want: []string{"ad -> sup"},
		},
		{
			name:  "the composition root may build adapters",
			rings: map[string]Ring{"cmd": Root, "ad": Adapter, "dom": Domain},
			files: map[string]string{
				"cmd/main.go": "package main\nimport (\"os\"; \"" + fixtureModule + "/ad\"; \"" + fixtureModule + "/dom\")\nfunc main() { _ = []any{os.Args, ad.X, dom.X} }\n",
				"ad/a.go":     "package ad\nvar X = 1\n",
				"dom/b.go":    "package dom\nvar X = 1\n",
			},
			want: nil,
		},
		{
			name:  "a support package has no rule",
			rings: map[string]Ring{"sup": Support, "ad": Adapter},
			files: map[string]string{
				"sup/a.go": "package sup\nimport (\"os\"; \"" + fixtureModule + "/ad\")\nvar _ = []any{os.Args, ad.X}\n",
				"ad/b.go":  "package ad\nvar X = 1\n",
			},
			want: nil,
		},
		{
			name:  "test files are not production code",
			rings: map[string]Ring{"dom": Domain},
			files: map[string]string{
				"dom/a.go":      "package dom\nvar X = 1\n",
				"dom/a_test.go": "package dom\nimport \"os\"\nvar _ = os.Args\n",
			},
			want: nil,
		},
		{
			name:  "a build-tagged file counts on every platform",
			rings: map[string]Ring{"dom": Domain},
			files: map[string]string{
				"dom/a.go":         "package dom\nvar X = 1\n",
				"dom/a_windows.go": "//go:build windows\n\npackage dom\nimport \"os\"\nvar _ = os.Args\n",
			},
			want: []string{"dom -> os"},
		},
		{
			name:  "a nested module is not part of the module under test",
			rings: map[string]Ring{"dom": Domain},
			files: map[string]string{
				"dom/a.go":          "package dom\nvar X = 1\n",
				"nested/go.mod":     "module example.test/nested\n",
				"nested/n/other.go": "package n\nimport \"os\"\nvar _ = os.Args\n",
			},
			want: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := fixtureEdges(t, tc.rings, tc.files); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("violations = %q, want %q", got, tc.want)
			}
		})
	}
}

// Importing time is not impure; asking it for the wall clock is. The same holds
// for the file system helpers of path/filepath and io/fs and the standard
// streams of fmt: the package stays importable, its ambient members do not.
func TestCheckerFlagsAmbientMembersOfPartlyPurePackages(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{"time values are pure", "import \"time\"\nvar _ = time.Duration(1) + time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Sub(time.Time{})\n", nil},
		{"time.Now reads the wall clock", "import \"time\"\nvar _ = time.Now()\n", []string{"dom -> time.Now"}},
		{"time.Now as a function value is caught too", "import \"time\"\nvar clock = time.Now\n", []string{"dom -> time.Now"}},
		{"time.Since and time.Sleep are ambient", "import \"time\"\nvar _ = time.Since(time.Time{})\nfunc f() { time.Sleep(1) }\n", []string{"dom -> time.Since", "dom -> time.Sleep"}},
		{"an aliased import is followed", "import clock \"time\"\nvar _ = clock.Now()\n", []string{"dom -> time.Now"}},
		{"a dot import hides its members", "import . \"time\"\nvar _ = Now()\n", []string{"dom -> time (dot import)"}},
		{"filepath string functions are pure", "import \"path/filepath\"\nvar _ = filepath.Join(\"a\", filepath.Base(\"b\"))\n", nil},
		{"filepath walking and resolving are not", "import \"path/filepath\"\nvar _ = filepath.WalkDir\nvar _, _ = filepath.Abs(\"x\")\nvar _, _ = filepath.EvalSymlinks(\"x\")\n", []string{"dom -> path/filepath.Abs", "dom -> path/filepath.EvalSymlinks", "dom -> path/filepath.WalkDir"}},
		{"io/fs types are pure", "import \"io/fs\"\nvar _ fs.FileMode\nvar _ = fs.ErrNotExist\n", nil},
		{"io/fs reading and walking are not", "import \"io/fs\"\nvar _ = fs.WalkDir\nvar _ = fs.ReadFile\n", []string{"dom -> io/fs.ReadFile", "dom -> io/fs.WalkDir"}},
		{"fmt formatting is pure, fmt printing is not", "import \"fmt\"\nvar _ = fmt.Sprintf(\"%d\", 1)\nfunc f() { fmt.Println(1) }\n", []string{"dom -> fmt.Println"}},
		{"a repeated use is reported once", "import \"time\"\nvar _ = time.Now()\nvar _ = time.Now()\n", []string{"dom -> time.Now"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{"dom/a.go": "package dom\n" + tc.body}
			if got := fixtureEdges(t, map[string]Ring{"dom": Domain}, files); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("violations = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCheckerAppliesTheMemberRulesToApplicationPackagesAndSparesAdapters(t *testing.T) {
	body := "package p\nimport \"time\"\nvar _ = time.Now()\n"
	files := func() map[string]string { return map[string]string{"p/a.go": body} }

	if got := fixtureEdges(t, map[string]Ring{"p": Application}, files()); !reflect.DeepEqual(got, []string{"p -> time.Now"}) {
		t.Errorf("application: violations = %q, want the wall clock flagged", got)
	}
	if got := fixtureEdges(t, map[string]Ring{"p": Adapter}, files()); got != nil {
		t.Errorf("adapter: violations = %q, want none", got)
	}
}

func TestCheckerNamesTheFilesOfAViolation(t *testing.T) {
	root := writeFixtureModule(t, map[string]string{
		"dom/b.go": "package dom\nimport \"os\"\nvar _ = os.Args\n",
		"dom/a.go": "package dom\nimport \"os\"\nvar _ = os.Args\n",
		"dom/c.go": "package dom\nvar X = 1\n",
	})
	c, err := newChecker(root, map[string]Ring{"dom": Domain})
	if err != nil {
		t.Fatal(err)
	}
	got := c.violations()
	if len(got) != 1 {
		t.Fatalf("violations = %v, want exactly one", got)
	}
	if want := []string{"dom/a.go", "dom/b.go"}; !reflect.DeepEqual(got[0].files, want) {
		t.Errorf("files = %v, want %v (sorted, only the importing files)", got[0].files, want)
	}
	if !strings.Contains(got[0].rule, "pure standard library") {
		t.Errorf("rule = %q, want it to name the pure standard library", got[0].rule)
	}
}

func TestCheckerReportsDeclarationDrift(t *testing.T) {
	root := writeFixtureModule(t, map[string]string{
		"declared/a.go": "package declared\n",
		"newcomer/b.go": "package newcomer\n",
		"main.go":       "package root\n",
	})
	c, err := newChecker(root, map[string]Ring{"declared": Domain, "gone": Domain})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := c.undeclared(), []string{".", "newcomer"}; !reflect.DeepEqual(got, want) {
		t.Errorf("undeclared = %q, want %q", got, want)
	}
	if got, want := c.missing(), []string{"gone"}; !reflect.DeepEqual(got, want) {
		t.Errorf("missing = %q, want %q", got, want)
	}
}

// A directory with only tests (a harness, a guard like this one) is a package: it
// must say it is support, and its row does not go stale for lack of production
// code.
func TestCheckerTreatsATestOnlyDirectoryAsAPackage(t *testing.T) {
	root := writeFixtureModule(t, map[string]string{
		"harness/h_test.go": "package harness\nimport \"os\"\nvar _ = os.Args\n",
		"dom/a.go":          "package dom\nvar X = 1\n",
	})
	c, err := newChecker(root, map[string]Ring{"dom": Domain})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := c.undeclared(), []string{"harness"}; !reflect.DeepEqual(got, want) {
		t.Errorf("undeclared = %q, want %q", got, want)
	}
	if got := c.violations(); len(got) != 0 {
		t.Errorf("violations = %v, want none: test files are not read", got)
	}

	c, err = newChecker(root, map[string]Ring{"dom": Domain, "harness": Support})
	if err != nil {
		t.Fatal(err)
	}
	if got := c.missing(); len(got) != 0 {
		t.Errorf("missing = %q, want none: a test-only package exists", got)
	}
}

func TestCheckerRefusesAnImportOfAPackageWithoutARing(t *testing.T) {
	got := fixtureEdges(t, map[string]Ring{"dom": Domain}, map[string]string{
		"dom/a.go":    "package dom\nimport \"" + fixtureModule + "/orphan\"\nvar _ = orphan.X\n",
		"orphan/b.go": "package orphan\nvar X = 1\n",
	})
	if want := []string{"dom -> orphan"}; !reflect.DeepEqual(got, want) {
		t.Errorf("violations = %q, want %q", got, want)
	}
}
