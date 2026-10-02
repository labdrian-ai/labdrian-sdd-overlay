package capability_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
)

// fakeCatalog is an in-memory capability.TestCatalog: the runnable tests of each
// directory, a directory that cannot be read, and a record of every question it was
// asked. No file system is involved, which is the point of the port.
type fakeCatalog struct {
	tests  map[string][]string // directory -> the runnable tests it declares
	broken map[string]error    // directory -> why it cannot be answered for
	asked  []string            // "<dir>:<name>", in the order asked
}

func (f *fakeCatalog) HasTest(dir, name string) (bool, error) {
	f.asked = append(f.asked, dir+":"+name)
	if err := f.broken[dir]; err != nil {
		return false, err
	}
	for _, have := range f.tests[dir] {
		if have == name {
			return true, nil
		}
	}
	return false, nil
}

// declarationWithRefs returns a declaration whose installation claim names refs; its
// other claim (projection) is unsupported and names none. CheckEvidence reads the
// references and nothing else, so no more of a declaration is needed.
func declarationWithRefs(refs ...string) capability.Declaration {
	return capability.Declaration{
		Target: capability.TargetClaude,
		Claims: []capability.Claim{
			{Capability: capability.Installation, Status: capability.Supported, Tests: refs},
			{Capability: capability.Projection, Status: capability.Unsupported, Detail: "not implemented"},
		},
	}
}

func TestCheckEvidenceAcceptsReferencesTheCatalogHas(t *testing.T) {
	catalog := &fakeCatalog{tests: map[string][]string{
		"alpha": {"TestPresent", "TestAlsoPresent"},
		"beta":  {"TestAliased"},
	}}
	err := capability.CheckEvidence(catalog, declarationWithRefs("alpha:TestAlsoPresent", "alpha:TestPresent", "beta:TestAliased"))
	if err != nil {
		t.Fatalf("CheckEvidence() = %v, want nil", err)
	}
	want := "alpha:TestAlsoPresent alpha:TestPresent beta:TestAliased"
	if got := strings.Join(catalog.asked, " "); got != want {
		t.Errorf("the catalog was asked %q, want %q: one question per reference, with the directory and name the reference carries", got, want)
	}
}

func TestCheckEvidenceAsksNothingOfADeclarationThatNamesNoTests(t *testing.T) {
	catalog := &fakeCatalog{}
	if err := capability.CheckEvidence(catalog, declarationWithRefs()); err != nil {
		t.Fatalf("CheckEvidence() = %v, want nil: a declaration that names no tests has nothing to prove", err)
	}
	if len(catalog.asked) != 0 {
		t.Errorf("the catalog was asked %v, want no question", catalog.asked)
	}
}

func TestCheckEvidenceRefusesATestTheCatalogDoesNotHave(t *testing.T) {
	catalog := &fakeCatalog{tests: map[string][]string{"alpha": {"TestPresent"}}}
	err := capability.CheckEvidence(catalog, declarationWithRefs("alpha:TestAbsent"))
	want := `target claude, capability installation: test "alpha:TestAbsent" not found: ` +
		`no top-level func TestAbsent(t *testing.T) in a _test.go file of "alpha"`
	if err == nil || err.Error() != want {
		t.Fatalf("CheckEvidence() = %v, want %q", err, want)
	}
}

// Every failing reference is reported at once, in claim order, each naming its target and
// capability, and a reference the catalog has is not among them.
func TestCheckEvidenceReportsEveryFailureAtOnce(t *testing.T) {
	catalog := &fakeCatalog{tests: map[string][]string{"alpha": {"TestPresent"}}}
	d := declarationWithRefs("alpha:TestAbsent", "alpha:TestPresent", "nowhere:TestPresent")
	d.Claims[1] = capability.Claim{
		Capability: capability.Projection,
		Status:     capability.Partial,
		Tests:      []string{"beta:TestMissing"},
		Detail:     "limited",
	}

	err := capability.CheckEvidence(catalog, d)
	if err == nil {
		t.Fatal("CheckEvidence() = nil, want three failures")
	}
	wants := []string{
		`target claude, capability installation: test "alpha:TestAbsent" not found`,
		`target claude, capability installation: test "nowhere:TestPresent" not found`,
		`target claude, capability projection: test "beta:TestMissing" not found`,
	}
	prev := -1
	for _, want := range wants {
		at := strings.Index(err.Error(), want)
		if at < 0 {
			t.Errorf("error does not report %q; got:\n%s", want, err.Error())
		}
		if at < prev {
			t.Errorf("error reports %q out of claim order; got:\n%s", want, err.Error())
		}
		prev = at
	}
	if strings.Contains(err.Error(), "alpha:TestPresent") {
		t.Errorf("error names a test the catalog has:\n%s", err.Error())
	}
}

// A reference is checked for format before the catalog is asked anything, so one that
// could name a path outside the engine root never reaches an adapter that reads files.
func TestCheckEvidenceRefusesAMalformedReferenceWithoutAskingTheCatalog(t *testing.T) {
	for _, ref := range []string{
		"../escape:TestEscaped",
		"/etc:TestAbsolutePath",
		"alpha//beta:TestDoubledSlash",
		"alpha:NotATest",
		"alpha",
		"",
	} {
		t.Run(ref, func(t *testing.T) {
			catalog := &fakeCatalog{tests: map[string][]string{"alpha": {"NotATest"}}}
			err := capability.CheckEvidence(catalog, declarationWithRefs(ref))
			if err == nil || !strings.Contains(err.Error(), "target claude, capability installation: test reference "+`"`+ref+`"`) {
				t.Fatalf("CheckEvidence(%q) = %v, want a format refusal naming the target, the capability and the reference", ref, err)
			}
			if len(catalog.asked) != 0 {
				t.Errorf("the catalog was asked %v for a malformed reference, want no question", catalog.asked)
			}
		})
	}
}

// A catalog that cannot answer for a directory says why, and the reason is reported with
// the reference; the references of other directories are still checked.
func TestCheckEvidenceReportsWhyTheCatalogCouldNotAnswer(t *testing.T) {
	reason := errors.New(`directory "gone" not found under the engine root`)
	catalog := &fakeCatalog{
		tests:  map[string][]string{"alpha": {"TestPresent"}},
		broken: map[string]error{"gone": reason},
	}
	err := capability.CheckEvidence(catalog, declarationWithRefs("alpha:TestAbsent", "gone:TestPresent"))
	if err == nil {
		t.Fatal("CheckEvidence() = nil, want two failures")
	}
	for _, want := range []string{
		`target claude, capability installation: test "gone:TestPresent": directory "gone" not found under the engine root`,
		`target claude, capability installation: test "alpha:TestAbsent" not found`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not report %q; got:\n%s", want, err.Error())
		}
	}
	if !errors.Is(err, reason) {
		t.Errorf("the catalog's reason is not reachable with errors.Is: %v", err)
	}
}
