package piguard_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/piguard"
)

// recordingT collects the failures CheckTestSources reports, so a test can read them. It
// implements exactly the two methods of piguard.Reporter, so the scan cannot reach a method it
// leaves out: there is none to reach.
type recordingT struct {
	errors []string
}

func (r *recordingT) Errorf(format string, args ...any) {
	r.errors = append(r.errors, strings.TrimSpace(fmt.Sprintf(format, args...)))
}

func (r *recordingT) Helper() {}

func writeTests(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestCheckTestSourcesAcceptsTestsThatStartNothingReal(t *testing.T) {
	dir := writeTests(t, map[string]string{"a_test.go": `package a
import ("os/exec"; "testing")
func TestA(t *testing.T) { _, _ = exec.LookPath("pi"); _ = exec.Command("node", "x"); _ = exec.CommandContext(nil, "node") }
`})
	r := &recordingT{}
	piguard.CheckTestSources(r, dir)
	if len(r.errors) != 0 {
		t.Fatalf("clean tests reported %v", r.errors)
	}
}

func TestCheckTestSourcesNamesWhatItRejects(t *testing.T) {
	dir := writeTests(t, map[string]string{
		"imports_test.go": `package a
import _ "github.com/labdrian-ai/labdrian-sdd-overlay/engine/execrunner"
`,
		"command_test.go": `package a
import "os/exec"
var _ = exec.Command("pi", "remove", "x")
`,
		"context_test.go": `package a
import "os/exec"
var _ = exec.CommandContext(nil, "pi")
`,
	})
	r := &recordingT{}
	piguard.CheckTestSources(r, dir)
	all := strings.Join(r.errors, "\n")
	for _, want := range []string{"imports_test.go imports", "command_test.go:3: exec.Command(\"pi\")", "context_test.go:3: exec.CommandContext(\"pi\")"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in:\n%s", want, all)
		}
	}
}

func TestCheckTestSourcesFailsWhenThereIsNothingToScan(t *testing.T) {
	r := &recordingT{}
	piguard.CheckTestSources(r, t.TempDir())
	if len(r.errors) == 0 {
		t.Fatal("an empty directory passed the scan; a wrong path would pass it for good")
	}
}

func TestCheckTestSourcesFollowsAnAliasedImportAndANamedConstant(t *testing.T) {
	dir := writeTests(t, map[string]string{
		"alias_test.go": `package a
import run "os/exec"
var _ = run.Command("pi", "remove", "x")
`,
		"constant_test.go": `package a
import "os/exec"
var _ = exec.CommandContext(nil, piBin)
`,
		"decl_test.go": `package a
const piBin = "pi"
`,
	})
	r := &recordingT{}
	piguard.CheckTestSources(r, dir)
	all := strings.Join(r.errors, "\n")
	for _, want := range []string{"alias_test.go:3: exec.Command(\"pi\")", "constant_test.go:3: exec.CommandContext(\"pi\")"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in:\n%s", want, all)
		}
	}
}

func TestCheckTestSourcesIgnoresACallOfAnotherPackageNamedExec(t *testing.T) {
	dir := writeTests(t, map[string]string{"a_test.go": `package a
import "example.com/exec"
var _ = exec.Command("pi")
`})
	r := &recordingT{}
	piguard.CheckTestSources(r, dir)
	if len(r.errors) != 0 {
		t.Fatalf("a package that is not os/exec was reported: %v", r.errors)
	}
}

// TestCheckTestSourcesStatesItsLimits pins what the scan does not see (see the doc of
// CheckTestSources). If one of these starts to be reported, the scan got stronger: move the case
// to the tests above and update the doc.
func TestCheckTestSourcesStatesItsLimits(t *testing.T) {
	cases := map[string]string{
		"a variable":               "func f() { bin := \"pi\"; _ = exec.Command(bin) }",
		"a function result":        "func name() string { return \"pi\" }\nvar _ = exec.Command(name())",
		"a constant in a function": "func f() { const piBin = \"pi\"; _ = exec.Command(piBin) }",
		"joined strings":           "var _ = exec.Command(\"p\" + \"i\")",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			dir := writeTests(t, map[string]string{"a_test.go": "package a\nimport \"os/exec\"\n" + body + "\n"})
			r := &recordingT{}
			piguard.CheckTestSources(r, dir)
			if len(r.errors) != 0 {
				t.Fatalf("%s is now seen by the scan (%v): update the limits in its doc and this test", name, r.errors)
			}
		})
	}
}

// A constant declared inside a function is not read, and a name is looked up in the constants of
// the package whatever the scope, so a local constant that shadows a package one with another
// value is read as the package one (see the doc of CheckTestSources). Both directions are
// pinned, so a scan that learns scopes shows up as a test to rewrite.
func TestCheckTestSourcesReadsAShadowedConstantAsThePackageOne(t *testing.T) {
	cases := []struct {
		name, packageValue, localValue string
		reported                       bool
	}{
		{"the local one is pi and the package one is not: missed", "git", "pi", false},
		{"the package one is pi and the local one is not: reported", "pi", "git", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := "const bin = \"" + c.packageValue + "\"\nfunc f() { const bin = \"" + c.localValue + "\"; _ = exec.Command(bin) }\n"
			dir := writeTests(t, map[string]string{"a_test.go": "package a\nimport \"os/exec\"\n" + body})
			r := &recordingT{}
			piguard.CheckTestSources(r, dir)
			if got := len(r.errors) != 0; got != c.reported {
				t.Fatalf("reported = %v (%v), want %v: the scan reads the package constant whatever the scope", got, r.errors, c.reported)
			}
		})
	}
}
