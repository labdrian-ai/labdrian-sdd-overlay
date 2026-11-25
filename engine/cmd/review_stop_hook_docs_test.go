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
	section := docSection(t, readRepositoryFile(t, "README.md"), "gentle-ai-overlay review stop-hook\n", "gentle-ai-overlay roles builder-guard\n")

	for _, want := range []string{
		"review stop-hook",
		"Stop",
		"blocks the stop",
		"once for each session and commit",
		"labdrian review bind --base",
		"the first commit that misses anything",
		"a passive commit is never asked",
		"labdrian review sweep --feature",
		"labdrian review record --role sweeper",
		"labdrian review record --role polisher",
		"asks for each thing a commit misses once for each session and commit",
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
	// These are compared after normalizeDocText: the first wraps across lines and the second holds code marks.
	text := normalizeDocText(section)
	for _, want := range []string{
		"the first commit with unfinished review steps",
		"(`git commit --amend --trailer` when the commit is HEAD)",
		"when the branch tracks a local branch of another name (its upstream",
		"the commits of that branch up to the fork point",
		"never the tip",
		"a remote upstream of any name",
		"leaves the merged ones out",
	} {
		if !strings.Contains(text, normalizeDocText(want)) {
			t.Errorf("the README section of the review stop-hook does not say %q", want)
		}
	}
	if strings.Contains(text, "not ready for delivery") {
		t.Errorf("the README section of the review stop-hook still says %q", "not ready for delivery")
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

// The other texts that describe the hook say what it asks for as the README does (own-rdd T6b7b, S6 e, L70): the help of
// the wrapper, the README entry of the verb, the install-hooks summary, the records README and the claim of the Claude
// installation say that
// it asks for each thing the first commit with unfinished review steps misses, once for each session and commit, and none
// says any more that it asks for the next commit no review covers, nor that the commit is not ready for delivery.
func TestOtherTextsOfTheReviewStopHookSayWhatItAsksFor(t *testing.T) {
	readme := readRepositoryFile(t, "README.md")
	for _, tc := range []struct{ name, text, want string }{
		{"bin/lib/usage.sh", readRepositoryFile(t, "bin/lib/usage.sh"),
			"not run by a person: the Claude Code Stop hook that install-hooks wires (the review stop-hook). On a branch other than the default one, " +
				"it blocks the stop for the first commit that misses anything review check asks of it, with the command for each thing it misses, " +
				"asking for each thing once for each session and commit, and never for the whole branch; " +
				"it lets the stop happen on anything it cannot tell (exit 0 always, 1 usage error)"},
		{"the stop-hook entry of README.md", docSection(t, readme, "    stop-hook\n", "overlay memory <verb>"),
			"It asks the agent for what the first commit with unfinished review steps misses, each thing once for each session and commit."},
		{"bin/lib/hooks.sh", readRepositoryFile(t, "bin/lib/hooks.sh"),
			"(on Stop, asks the agent for each thing the first commit with unfinished review steps misses, once for each session and commit),"},
		{"odd/review-results/README.md", readRepositoryFile(t, "odd/review-results/README.md"),
			"when the agent is about to stop on a branch that has commits with unfinished review steps, the hook blocks the stop and asks for what the first of them misses"},
	} {
		text := normalizeDocText(tc.text)
		if !strings.Contains(text, normalizeDocText(tc.want)) {
			t.Errorf("%s does not say %q", tc.name, tc.want)
		}
		if strings.Contains(text, "no review covers") || strings.Contains(text, "no stored result or exemption binds") {
			t.Errorf("%s still says that the hook asks for the next commit no review covers", tc.name)
		}
		if strings.Contains(text, "not ready for delivery") {
			t.Errorf("%s still says %q", tc.name, "not ready for delivery")
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

// The bound of the evidence run is told as the runner has it (T4y, `36d5bc62df38-4` F2): the stop can wait while a process
// the script started holds its output, and that process can outlive the stop. No text says that the stop takes at most 2
// seconds or that it stops every process the script started. The seconds themselves are held by evidencesh's
// TestTheTextsStateTheBoundOfTheRunner, against evidencesh.DefaultTimeout and execrunner.KillGrace (T4z).
func TestTextsOfTheReviewStopHookStateTheBoundOfTheEvidenceRun(t *testing.T) {
	readme := readRepositoryFile(t, "README.md")
	for _, tc := range []struct{ name, text string }{
		{"the README section of the review stop-hook", docSection(t, readme, "gentle-ai-overlay review stop-hook\n", "gentle-ai-overlay roles builder-guard\n")},
		{"CHANGELOG.md", docSection(t, readRepositoryFile(t, "CHANGELOG.md"), "- The Stop hook (`labdrian review stop-hook`)", "\n- ")},
	} {
		text := normalizeDocText(tc.text)
		for _, want := range []string{"can outlive the stop"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s does not say %q", tc.name, want)
			}
		}
		for _, refused := range []string{"with every process it started", "at most 2 seconds"} {
			if strings.Contains(text, refused) {
				t.Errorf("%s still says %q", tc.name, refused)
			}
		}
	}
}

// What the evidence run cannot tell is told as it is (T4x, `4b9c3789c2e0-3` F2): a store that is not deployed is named, and
// the line repeats at each stop because nothing of it is remembered. The refusals made before a run are held by evidencesh's
// TestTheTextsNameEveryRefusalBeforeARun, against the errors of the adapter (T4b-e); this test keeps the wording that is
// not a refusal.
func TestTextsOfTheReviewStopHookSayTheLineOfAnUndeployedStoreRepeats(t *testing.T) {
	readme := readRepositoryFile(t, "README.md")
	for _, tc := range []struct{ name, text string }{
		{"the README section of the review stop-hook", docSection(t, readme, "gentle-ai-overlay review stop-hook\n", "gentle-ai-overlay roles builder-guard\n")},
		{"CHANGELOG.md", docSection(t, readRepositoryFile(t, "CHANGELOG.md"), "- The Stop hook (`labdrian review stop-hook`)", "\n- ")},
	} {
		text := normalizeDocText(tc.text)
		for _, want := range []string{"not deployed", "repeats at each stop", "of it is remembered"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s does not say %q", tc.name, want)
			}
		}
	}
}

// A result held for an unverified severe finding waits for the checks it needs, not for a verdict (T4e, L102, L106): a
// stored not-run holds it for good, only a new review result runs the check again, and review check refuses the head,
// naming the findings not checked.
func TestTextsOfTheReviewStopHookSayAHeldResultWaitsForItsCheck(t *testing.T) {
	readme := readRepositoryFile(t, "README.md")
	for _, tc := range []struct{ name, text string }{
		{"the stop-hook entry of README.md", docSection(t, readme, "    stop-hook\n", "overlay memory <verb>")},
		{"the README section of the review stop-hook", docSection(t, readme, "gentle-ai-overlay review stop-hook\n", "gentle-ai-overlay roles builder-guard\n")},
		{"bin/lib/usage.sh", docSection(t, readRepositoryFile(t, "bin/lib/usage.sh"), "                                   stop-hook\n", "Only launch starts an agent")},
		{"CHANGELOG.md", docSection(t, readRepositoryFile(t, "CHANGELOG.md"), "- The Stop hook (`labdrian review stop-hook`)", "\n- ")},
	} {
		text := normalizeDocText(tc.text)
		for _, want := range []string{"until its check has run", "each check it needs", "not-run", "for good", "a new review result", "findings not checked"} {
			if !strings.Contains(text, normalizeDocText(want)) {
				t.Errorf("%s does not say %q", tc.name, want)
			}
		}
		for _, stale := range []string{"until its verdict is stored", "meanwhile", "not-reproduced for it"} {
			if strings.Contains(text, normalizeDocText(stale)) {
				t.Errorf("%s still says %q", tc.name, stale)
			}
		}
	}
}

// The texts that say what the hook asks for also say that it asks for the classification of results (S6, T4w, L84): the
// README entry and section, the help of the wrapper and the CHANGELOG, each naming the page and the script it asks through.
func TestTextsOfTheReviewStopHookSayItAsksForClassification(t *testing.T) {
	readme := readRepositoryFile(t, "README.md")
	for _, tc := range []struct{ name, text string }{
		{"the stop-hook entry of README.md", docSection(t, readme, "    stop-hook\n", "overlay memory <verb>")},
		{"the README section of the review stop-hook", docSection(t, readme, "gentle-ai-overlay review stop-hook\n", "gentle-ai-overlay roles builder-guard\n")},
		{"bin/lib/usage.sh", docSection(t, readRepositoryFile(t, "bin/lib/usage.sh"), "                                   stop-hook\n", "Only launch starts an agent")},
		{"CHANGELOG.md", docSection(t, readRepositoryFile(t, "CHANGELOG.md"), "- The Stop hook (`labdrian review stop-hook`)", "\n- ")},
	} {
		text := normalizeDocText(tc.text)
		for _, want := range []string{"classification", "docs/wiki/evidence.md", "evidence.sh reviewed-each"} {
			if !strings.Contains(text, normalizeDocText(want)) {
				t.Errorf("%s does not say %q", tc.name, want)
			}
		}
	}
}
