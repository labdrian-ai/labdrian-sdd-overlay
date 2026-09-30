package capability_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
)

// evidenceFixture builds a small engine-shaped tree under a temporary
// directory and returns the engine root. Only parseability matters, so the
// files are never compiled. Next to the root sits a sibling directory holding
// a real test, which no reference may ever reach.
//
//	alpha/present_test.go   TestPresent, TestAlsoPresent (valid tests)
//	alpha/shapes_test.go    functions that look like tests but are not
//	alpha/helper.go         a Test-shaped function in a non-test file
//	alpha/notes.txt         test-shaped text in a non-Go file
//	alpha/dir_test.go/      a directory whose name ends in _test.go
//	beta/aliased_test.go    a test declared with an aliased testing import
//	broken/broken_test.go   a syntax error
func evidenceFixture(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "engine")
	files := map[string]string{
		"engine/alpha/present_test.go": "package alpha\n\nimport \"testing\"\n\n" +
			"func TestPresent(t *testing.T) {}\n\nfunc TestAlsoPresent(t *testing.T) {}\n",
		"engine/alpha/shapes_test.go": "package alpha\n\nimport \"testing\"\n\n" +
			"func TestNoParams() {}\n" +
			"func TestBenchShaped(b *testing.B) {}\n" +
			"func TestIntParam(x int) {}\n" +
			"func TestTwoParams(t *testing.T, extra int) {}\n" +
			"func TestReturnsValue(t *testing.T) error { return nil }\n" +
			"func Testlowercase(t *testing.T) {}\n" +
			"func TestMain(m *testing.M) {}\n" +
			"func TestGeneric[T any](t *testing.T) {}\n" +
			"type suite struct{}\n" +
			"func (suite) TestMethod(t *testing.T) {}\n",
		"engine/alpha/helper.go":       "package alpha\n\nimport \"testing\"\n\nfunc TestOnlyInNonTestFile(t *testing.T) {}\n",
		"engine/alpha/notes.txt":       "func TestInText(t *testing.T) {}\n",
		"engine/beta/aliased_test.go":  "package beta\n\nimport tt \"testing\"\n\nfunc TestAliased(t *tt.T) {}\n",
		"engine/broken/broken_test.go": "package broken\n\nfunc TestBroken(t *testing.T) {\n",
		"escape/escaped_test.go":       "package escape\n\nimport \"testing\"\n\nfunc TestEscaped(t *testing.T) {}\n",
	}
	for name, content := range files {
		path := filepath.Join(base, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "alpha", "dir_test.go"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// declarationWithRefs returns a declaration whose installation claim names
// refs; every other claim is unsupported and names none.
func declarationWithRefs(refs ...string) capability.Declaration {
	d := validDeclaration()
	d.Claims[0].Tests = refs
	return d
}

func TestCheckEvidenceAcceptsRealTests(t *testing.T) {
	root := evidenceFixture(t)
	tests := []struct {
		name string
		refs []string
	}{
		{"one present test", []string{"alpha:TestPresent"}},
		{"several tests across directories", []string{"alpha:TestAlsoPresent", "alpha:TestPresent", "beta:TestAliased"}},
		{"a test declared through an aliased testing import", []string{"beta:TestAliased"}},
		{"a declaration that names no tests has nothing to prove", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := capability.CheckEvidence(root, declarationWithRefs(tt.refs...)); err != nil {
				t.Fatalf("CheckEvidence() = %v, want nil", err)
			}
		})
	}
}

func TestCheckEvidenceRefusesAnythingThatIsNotARunnableTest(t *testing.T) {
	root := evidenceFixture(t)
	tests := []struct {
		name    string
		ref     string
		wantErr string
	}{
		{"a test name that does not exist", "alpha:TestAbsent", `test "alpha:TestAbsent" not found`},
		{"a directory that does not exist", "nowhere:TestPresent", `directory "nowhere" not found under the engine root`},
		{"a test-shaped function in a non-test file", "alpha:TestOnlyInNonTestFile", `test "alpha:TestOnlyInNonTestFile" not found`},
		{"a test-shaped function in a non-Go file", "alpha:TestInText", `test "alpha:TestInText" not found`},
		{"a function without parameters", "alpha:TestNoParams", `test "alpha:TestNoParams" not found`},
		{"a benchmark-shaped function", "alpha:TestBenchShaped", `test "alpha:TestBenchShaped" not found`},
		{"a function whose parameter is not a pointer", "alpha:TestIntParam", `test "alpha:TestIntParam" not found`},
		{"a function with two parameters", "alpha:TestTwoParams", `test "alpha:TestTwoParams" not found`},
		{"a function with a result", "alpha:TestReturnsValue", `test "alpha:TestReturnsValue" not found`},
		{"a name go test would not run because a lowercase letter follows Test", "alpha:Testlowercase", `test "alpha:Testlowercase" not found`},
		{"TestMain, which takes a *testing.M", "alpha:TestMain", `test "alpha:TestMain" not found`},
		{"a generic function", "alpha:TestGeneric", `test "alpha:TestGeneric" not found`},
		{"a method", "alpha:TestMethod", `test "alpha:TestMethod" not found`},
		{"a test in a sibling directory", "beta:TestPresent", `test "beta:TestPresent" not found`},
		{"a file that does not parse", "broken:TestBroken", "parse broken/broken_test.go"},
		{"a reference that climbs out of the engine root", "../escape:TestEscaped", `test reference "../escape:TestEscaped" must match`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := capability.CheckEvidence(root, declarationWithRefs(tt.ref))
			if err == nil {
				t.Fatalf("CheckEvidence(%q) = nil, want an error containing %q", tt.ref, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("CheckEvidence(%q) = %q, want it to contain %q", tt.ref, err.Error(), tt.wantErr)
			}
			if !strings.Contains(err.Error(), "target claude, capability installation") {
				t.Errorf("error %q does not name the target and capability", err.Error())
			}
		})
	}
}

func TestCheckEvidenceReportsEveryFailureAtOnce(t *testing.T) {
	root := evidenceFixture(t)
	d := declarationWithRefs("alpha:TestAbsent", "alpha:TestPresent", "nowhere:TestPresent")
	d.Claims[1] = capability.Claim{
		Capability: capability.Projection,
		Status:     capability.Partial,
		Tests:      []string{"beta:TestMissing"},
		Detail:     "limited",
	}

	err := capability.CheckEvidence(root, d)
	if err == nil {
		t.Fatal("CheckEvidence() = nil, want three failures")
	}
	for _, want := range []string{
		`target claude, capability installation: test "alpha:TestAbsent" not found`,
		`target claude, capability installation: test "nowhere:TestPresent"`,
		`target claude, capability projection: test "beta:TestMissing" not found`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not report %q; got:\n%s", want, err.Error())
		}
	}
	if strings.Contains(err.Error(), "alpha:TestPresent") {
		t.Errorf("error names a test that exists:\n%s", err.Error())
	}
}

// TestDeclaredEvidenceExists is the guard the design promises: every test a
// shipped declaration names must exist in the real engine tree, so a renamed
// or deleted test cannot leave a supported claim standing on nothing. The
// engine root is the parent of this package's directory.
func TestDeclaredEvidenceExists(t *testing.T) {
	for _, d := range capability.All() {
		if err := capability.CheckEvidence("..", d); err != nil {
			t.Errorf("declared evidence for %s is missing:\n%v", d.Target, err)
		}
	}
}

func TestCheckEvidenceRefusesAMissingEngineRoot(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-engine")
	err := capability.CheckEvidence(missing, declarationWithRefs("alpha:TestPresent"))
	if err == nil || !strings.Contains(err.Error(), `directory "alpha" not found under the engine root`) {
		t.Fatalf("CheckEvidence() = %v, want the directory-not-found error", err)
	}
}
