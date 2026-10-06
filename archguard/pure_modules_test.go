package archguard

import (
	"strings"
	"testing"
)

// A module outside the one being judged is third-party code to the rule, unless the caller names
// it a pure module: one that holds itself to the standard library alone (its own test says so),
// which domain and application packages of this module may then import.

const pureModule = "example.test/identity"

func importsPureModule(path string) map[string]string {
	return map[string]string{"dom/a.go": "package dom\nimport \"" + path + "\"\nvar _ = identity.Rule\n"}
}

func checkWithPure(t *testing.T, files map[string]string, pure ...string) []string {
	t.Helper()
	root := writeFixtureModule(t, files)
	problems, err := CheckWith(root, map[string]Ring{"dom": Domain}, nil, Options{PureModules: pure})
	if err != nil {
		t.Fatalf("CheckWith: %v", err)
	}
	return problems
}

func TestAPureModuleMayBeImportedByADomainPackage(t *testing.T) {
	if problems := checkWithPure(t, importsPureModule(pureModule), pureModule); len(problems) != 0 {
		t.Errorf("problems = %q, want none for an import of a module the caller names pure", problems)
	}
}

func TestAPureModuleCoversItsOwnPackagesAndOnlyThose(t *testing.T) {
	if problems := checkWithPure(t, importsPureModule(pureModule+"/sub"), pureModule); len(problems) != 0 {
		t.Errorf("problems = %q, want none for a package of the pure module", problems)
	}
	problems := checkWithPure(t, importsPureModule(pureModule+"x"), pureModule)
	got := strings.Join(problems, "\n")
	for _, want := range []string{"dom -> " + pureModule + "x", "not a third-party module", "is not imported by any package"} {
		if !strings.Contains(got, want) {
			t.Errorf("problems do not mention %q (a module whose path only starts like the pure one is not it):\n%s", want, got)
		}
	}
}

func TestAModuleNobodyNamedPureIsStillThirdParty(t *testing.T) {
	problems := checkWithPure(t, importsPureModule(pureModule))
	if len(problems) != 1 || !strings.Contains(problems[0], "dom -> "+pureModule) || !strings.Contains(problems[0], "not a third-party module") {
		t.Errorf("problems = %q, want the import flagged as third-party", problems)
	}
}

// Like debt, the list only says what exists: a pure module no package imports is a permission
// with nothing behind it.
func TestAPureModuleNobodyImportsIsReportedAsStale(t *testing.T) {
	problems := checkWithPure(t, cleanDomain(), pureModule)
	if len(problems) != 1 || !strings.Contains(problems[0], pureModule) || !strings.Contains(problems[0], "is not imported by any package") {
		t.Errorf("problems = %q, want the stale pure module named", problems)
	}
}

func TestAPureModuleIsNotAnAdapterPermission(t *testing.T) {
	files := map[string]string{"dom/a.go": "package dom\nimport \"os\"\nimport \"" + pureModule + "\"\nvar _, _ = os.Args, identity.Rule\n"}
	problems := checkWithPure(t, files, pureModule)
	if len(problems) != 1 || !strings.Contains(problems[0], "dom -> os") {
		t.Errorf("problems = %q, want only the os import flagged", problems)
	}
}

func TestCheckIsCheckWithNoOptions(t *testing.T) {
	root := writeFixtureModule(t, importsPureModule(pureModule))
	rings := map[string]Ring{"dom": Domain}
	a, errA := Check(root, rings, nil)
	b, errB := CheckWith(root, rings, nil, Options{})
	if errA != nil || errB != nil || strings.Join(a, "\n") != strings.Join(b, "\n") {
		t.Errorf("Check = %q, %v and CheckWith(Options{}) = %q, %v: they must agree", a, errA, b, errB)
	}
}
