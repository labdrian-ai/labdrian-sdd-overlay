package evidencesh_test

// The seconds that the texts of the evidence run give are the ones the code has (T4z, `c9440b24b20f-6` F4): the timeout
// is evidencesh.DefaultTimeout and the wait past it is execrunner.KillGrace. A text that states a bound the code does not
// have is the class T4w found; this test builds both phrases from the constants, so a change of either constant fails
// here until the three texts are changed with it. It lives in this package because the package doc is one of the texts and
// the tests of this package already import execrunner; the docs test of engine/cmd holds the rest of the wording.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/execrunner"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/evidencesh"
)

// textBetween is the text of whole from the first start to the next end, with backticks dropped and each run of white
// space made one space.
func textBetween(t *testing.T, name, whole, start, end string) string {
	t.Helper()
	i := strings.Index(whole, start)
	if i < 0 {
		t.Fatalf("%s: marker %q not found", name, start)
	}
	j := strings.Index(whole[i+len(start):], end)
	if j < 0 {
		t.Fatalf("%s: marker %q not found after %q", name, end, start)
	}
	return strings.Join(strings.Fields(strings.ReplaceAll(whole[i:i+len(start)+j], "`", "")), " ")
}

func TestTheTextsStateTheBoundOfTheRunner(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for name, d := range map[string]time.Duration{"evidencesh.DefaultTimeout": evidencesh.DefaultTimeout, "execrunner.KillGrace": execrunner.KillGrace} {
		if d <= 0 || d%time.Second != 0 {
			t.Fatalf("%s is %v: the texts give whole seconds, so reword them before it is not", name, d)
		}
	}
	stopped := fmt.Sprintf("stopped at %d seconds", evidencesh.DefaultTimeout/time.Second)
	more := fmt.Sprintf("up to %d seconds more", execrunner.KillGrace/time.Second)
	root := overlaytest.RepoRoot(t)
	read := func(parts ...string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(append([]string{root}, parts...)...))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	for _, tc := range []struct{ name, text string }{
		{"the README section of the review stop-hook", textBetween(t, "README.md", read("README.md"), "gentle-ai-overlay review stop-hook\n", "overlay --help\n")},
		{"CHANGELOG.md", textBetween(t, "CHANGELOG.md", read("CHANGELOG.md"), "- The Stop hook (`labdrian review stop-hook`)", "\n- ")},
	} {
		for _, want := range []string{stopped, more} {
			if !strings.Contains(tc.text, want) {
				t.Errorf("%s does not say %q", tc.name, want)
			}
		}
	}
	doc := strings.Join(strings.Fields(strings.ReplaceAll(read("engine", "review", "evidencesh", "evidencesh.go"), "\n//", " ")), " ")
	if want := fmt.Sprintf("KillGrace, %d seconds", execrunner.KillGrace/time.Second); !strings.Contains(doc, want) {
		t.Errorf("the package doc of evidencesh does not say %q", want)
	}
}
