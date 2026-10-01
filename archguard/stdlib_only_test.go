package archguard

import (
	"os"
	"strings"
	"testing"
)

// archguard is imported by the tests of the modules it guards, and it must never
// pull anything into them: no third-party module, no other module of the
// repository. Its go.mod therefore requires nothing, and its code imports the
// standard library only.
func TestTheModuleDependsOnNothingButTheStandardLibrary(t *testing.T) {
	gomod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(gomod), "\n") {
		if directive := strings.Fields(line); len(directive) > 0 && (directive[0] == "require" || directive[0] == "replace") {
			t.Errorf("go.mod has a %q directive (%q); archguard depends on the standard library only", directive[0], line)
		}
	}

	packages, err := loadPackages(".")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, pkg := range packages {
		for _, ref := range pkg.refs {
			if ref.kind == refImport {
				checked++
				if !isStandard(ref.pkg) {
					t.Errorf("%s imports %q, which is not in the standard library", ref.file, ref.pkg)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no imports were examined; the walk is broken")
	}
}
