package piguard_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/piguard"
)

// recordingT collects the failures CheckTestSources reports, so a test can read them.
type recordingT struct {
	testing.TB
	errors []string
}

func (r *recordingT) Errorf(format string, args ...any) {
	r.errors = append(r.errors, strings.TrimSpace(sprintf(format, args...)))
}
func (r *recordingT) Helper() {}
func (r *recordingT) Fatalf(format string, args ...any) {
	r.errors = append(r.errors, "FATAL "+sprintf(format, args...))
}

func sprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }

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
