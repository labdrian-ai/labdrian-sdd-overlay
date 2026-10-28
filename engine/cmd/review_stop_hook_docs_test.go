package main

// The documentation of the review stop-hook (own-rdd T6a, S6): README, the wrapper's install-hooks and uninstall-hooks
// summaries and help, the table of verbs, and the CHANGELOG. The usage text of the engine has its own test beside the
// verb (TestUsageDocumentsReviewStopHook). Like the documentation tests of the approve guard, they pin what a user must
// be told: what the hook asks for and when, what counts as a review, that install-hooks must be re-run and Claude Code
// restarted, and that no hook family is left out of the summaries of the verbs that manage them.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

func readRepositoryFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

// The hook is installed by install-hooks and run by Claude Code on every Stop, so the README says what it asks for,
// once for what, what counts as reviewed, that it never blocks on an error, and what a user must do to get it.
func TestDocsDescribeTheReviewStopHook(t *testing.T) {
	section := docSection(t, readRepositoryFile(t, "README.md"), "gentle-ai-overlay review stop-hook\n", "overlay --help\n")

	for _, want := range []string{
		"review stop-hook",
		"Stop",
		"blocks the stop",
		"once for each session and commit",
		"labdrian review bind --base",
		"the next commit",
		"not the whole branch",
		"a stored result or an exemption that binds a commit",
		"whatever its outcome",
		"review check",
		"review exempt",
		"never blocks on an error",
		"re-run install-hooks",
		"restart Claude Code",
	} {
		if !strings.Contains(section, want) {
			t.Errorf("the README section of the review stop-hook does not say %q", want)
		}
	}
}

// Every summary of the verbs that manage the hook families names the review stop-hook, with the other families.
func TestInstallHooksSummariesNameTheReviewStopHook(t *testing.T) {
	readme := readRepositoryFile(t, "README.md")
	overlay := overlaytest.ScriptText(t)
	sections := map[string]string{
		"README install-hooks":          docSection(t, readme, "overlay install-hooks\n", "overlay uninstall-hooks\n"),
		"README uninstall-hooks":        docSection(t, readme, "overlay uninstall-hooks\n", "overlay status-hooks\n"),
		"README status-hooks":           docSection(t, readme, "overlay status-hooks\n", "overlay doctor"),
		"README table install-hooks":    docSection(t, readme, "| `install-hooks` |", "| `uninstall-hooks` |"),
		"README table uninstall-hooks":  docSection(t, readme, "| `uninstall-hooks` |", "| `status-hooks` |"),
		"README table status-hooks":     docSection(t, readme, "| `status-hooks` |", "| `doctor [--fix]` |"),
		"install-hooks summary":         docSection(t, overlay, `echo "install-hooks complete."`, "Run 'labdrian doctor'"),
		"uninstall-hooks summary":       docSection(t, overlay, `echo "uninstall-hooks complete."`, "Run 'labdrian doctor'"),
		"--help for install-hooks":      docSection(t, overlay, "  install-hooks                    Build + deploy", "  uninstall-hooks "),
		"--help for uninstall-hooks":    docSection(t, overlay, "  uninstall-hooks                  Remove our hook entries", "  status-hooks "),
		"--help for status-hooks":       docSection(t, overlay, "  status-hooks                     Read-only check", "  doctor "),
		"README review verbs":           docSection(t, readme, "overlay review <verb>\n", "overlay memory <verb>"),
		"--help for the review command": docSection(t, overlay, "  review <verb>                    Forward review", "  memory <verb> "),
	}
	for name, text := range sections {
		if !strings.Contains(text, "review stop-hook") {
			t.Errorf("%s does not mention the review stop-hook", name)
		}
	}
	for _, name := range []string{"README install-hooks", "install-hooks summary", "--help for install-hooks"} {
		if text := sections[name]; !strings.Contains(text, "Stop") {
			t.Errorf("%s does not say the hook is wired into Stop", name)
		}
	}
}

// The CHANGELOG entry says what changed for a user of an existing install: the new Stop entry, the verb, and that
// status-hooks warns until install-hooks is run again.
func TestChangelogRecordsTheReviewStopHook(t *testing.T) {
	entry := docSection(t, readRepositoryFile(t, "CHANGELOG.md"), "- `labdrian review stop-hook`", "\n- ")

	for _, want := range []string{
		"own-rdd T6a",
		"Stop",
		"once for each session and commit",
		"labdrian review bind --base",
		"install-hooks",
		"uninstall-hooks",
		"status-hooks",
		"restart Claude Code",
		"a stored result or an exemption that binds a commit",
		"whatever its outcome",
	} {
		if !strings.Contains(entry, want) {
			t.Errorf("the CHANGELOG entry of the review stop-hook does not say %q", want)
		}
	}
}
