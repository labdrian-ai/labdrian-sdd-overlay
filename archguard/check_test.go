package archguard

import (
	"strings"
	"testing"
)

// checkFixture runs Check on a throwaway module.
func checkFixture(t *testing.T, rings map[string]Ring, debt Debt, files map[string]string) []string {
	t.Helper()
	root := writeFixtureModule(t, files)
	problems, err := Check(root, rings, debt)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	return problems
}

func cleanDomain() map[string]string {
	return map[string]string{"dom/a.go": "package dom\nimport \"strings\"\nvar _ = strings.ToUpper\n"}
}

func TestCheckPassesACleanModule(t *testing.T) {
	if problems := checkFixture(t, map[string]Ring{"dom": Domain}, nil, cleanDomain()); len(problems) != 0 {
		t.Errorf("problems = %q, want none", problems)
	}
}

func TestCheckNamesANewViolationItsRuleAndItsFile(t *testing.T) {
	files := map[string]string{"dom/a.go": "package dom\nimport \"os\"\nvar _ = os.Args\n"}
	problems := checkFixture(t, map[string]Ring{"dom": Domain}, nil, files)

	if len(problems) != 1 {
		t.Fatalf("problems = %q, want exactly one", problems)
	}
	for _, want := range []string{"dom -> os", "breaks the dependency rule", "pure standard library", "dom/a.go", "knownDebt", "unit"} {
		if !strings.Contains(problems[0], want) {
			t.Errorf("problem %q does not mention %q", problems[0], want)
		}
	}
}

func TestCheckAcceptsAViolationThatIsKnownDebt(t *testing.T) {
	files := map[string]string{"dom/a.go": "package dom\nimport \"os\"\nvar _ = os.Args\n"}
	debt := Debt{"dom": {"os": "H6"}}
	if problems := checkFixture(t, map[string]Ring{"dom": Domain}, debt, files); len(problems) != 0 {
		t.Errorf("problems = %q, want none for listed debt", problems)
	}
}

// Debt only shrinks: when the violation is gone its line must go with it, or the
// next one to reintroduce the import would find the permission still there.
func TestCheckFailsOnDebtWhoseViolationIsGone(t *testing.T) {
	debt := Debt{"dom": {"os": "H6"}}
	problems := checkFixture(t, map[string]Ring{"dom": Domain}, debt, cleanDomain())

	if len(problems) != 1 || !strings.Contains(problems[0], "dom -> os") || !strings.Contains(problems[0], "H6") || !strings.Contains(problems[0], "no longer a violation") {
		t.Errorf("problems = %q, want the stale line named with its unit", problems)
	}
}

func TestCheckReportsDeclarationDriftAndBadDebtTogether(t *testing.T) {
	files := cleanDomain()
	files["newcomer/b.go"] = "package newcomer\n"
	rings := map[string]Ring{"dom": Domain, "gone": Domain}
	debt := Debt{"dom": {"os": "later"}}

	got := strings.Join(checkFixture(t, rings, debt, files), "\n")
	for _, want := range []string{
		"package newcomer has no ring",
		"declares gone but no such package exists",
		`unit "later" is not a work unit id`,
		"dom -> os",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("problems do not mention %q:\n%s", want, got)
		}
	}
}

func TestCheckFailsWhenThereIsNoGoCodeToJudge(t *testing.T) {
	root := writeFixtureModule(t, map[string]string{})
	if _, err := Check(root, map[string]Ring{}, nil); err == nil || !strings.Contains(err.Error(), "no Go packages") {
		t.Errorf("Check on an empty module = %v, want an error saying no Go packages were found", err)
	}
}

func TestCheckFailsWhenTheRootIsNotAModule(t *testing.T) {
	if _, err := Check(t.TempDir(), map[string]Ring{}, nil); err == nil || !strings.Contains(err.Error(), "go.mod") {
		t.Errorf("Check on a directory without go.mod = %v, want an error naming go.mod", err)
	}
}
