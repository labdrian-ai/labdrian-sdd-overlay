package shelltest

// What the Classify step of the role chain hands to evidence.sh and reads back (T4c of
// odd/tasks/merge-playbook-into-sdd-overlay.md, L55, L90): the empty page it creates, and the lines a failed check prints,
// which the step passes to the classifier as `<failed>`. Both run the script as it is deployed (newEvidenceWorld).

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// R4: the empty page that classify.md tells the parent to create passes `check`, and passes again with the two example rows
// of the classifier's body appended as step 8 says (the Kinds row right under its header, the Evidence row at the end).
func TestTheEmptyPageOfTheClassifyStepPassesCheck(t *testing.T) {
	root := overlaytest.RepoRoot(t)
	step, err := os.ReadFile(filepath.Join(root, "skills", "role-chain", "references", "classify.md"))
	if err != nil {
		t.Fatal(err)
	}
	pages := markdownFencesAfter(string(step), "The empty page:")
	if len(pages) != 1 {
		t.Fatalf("classify.md holds %d markdown fences after %q, want the one empty page", len(pages), "The empty page:")
	}
	empty := strings.Join(pages[0], "\n") + "\n"
	body, err := os.ReadFile(filepath.Join(root, "roles", "classifier", "body.md"))
	if err != nil {
		t.Fatal(err)
	}
	examples, err := formatFences(string(body))
	if err != nil {
		t.Fatal(err)
	}

	w := newEvidenceWorld(t, empty)
	wantResult(t, "check of the empty page of classify.md", w.inRepo("check"), 0, "evidence check passed: docs/wiki/evidence.md\n", "")

	kindsEnd := "|------|-------|------|--------------|--------|-------|\n"
	withKinds := strings.Replace(empty, kindsEnd, kindsEnd+examples[1][0]+"\n", 1)
	if withKinds == empty {
		t.Fatalf("the empty page of classify.md does not hold the Kinds header line %q", kindsEnd)
	}
	writeEvidenceFile(t, filepath.Join(w.repo, "docs", "wiki", "evidence.md"), withKinds+examples[1][1]+"\n")
	wantResult(t, "check of the page with the example rows", w.inRepo("check"), 0, "evidence check passed: docs/wiki/evidence.md\n", "")

	// Step 8 appends, for a review whose Evidence fence is empty, a section of the table of reviews classified without
	// evidence at the page's end (L98 choice 3). The lines it writes are held in step 8 as code spans, and the page they make
	// passes check and answers reviewed for the review (the strings are checked against R11's of engine/skills).
	eight := classifyStepText(string(step), 8)
	for _, line := range []string{"## Classified without evidence", "| Review | Date |", "|--------|------|"} {
		if !strings.Contains(eight, "`"+line+"`") {
			t.Fatalf("step 8 of classify.md does not write the line `%s` of the section", line)
		}
	}
	appended := empty + "\n## Classified without evidence\n\n| Review | Date |\n|--------|------|\n| f/aaaa-1 | 2026-11-19 |\n"
	w = newEvidenceWorld(t, appended)
	wantResult(t, "check of the page with the section of step 8", w.inRepo("check"), 0, "evidence check passed: docs/wiki/evidence.md\n", "")
	wantResult(t, "reviewed for the row of the section", w.inRepo("reviewed", "f/aaaa-1"), 0, "", "")
	wantResult(t, "reviewed for another review", w.inRepo("reviewed", "f/bbbb-1"), 1, "", "")
}

// classifyStepText is step n of classify.md: its text from "<n>. " to the next numbered step, or to the end.
func classifyStepText(text string, n int) string {
	_, after, _ := strings.Cut(text, fmt.Sprintf("\n%d. ", n))
	step, _, _ := strings.Cut(after, fmt.Sprintf("\n%d. ", n+1))
	return step
}

// R10 (`8ee5ac95b287-2` F1, L98 choice 4): a FAIL line of the general rules file is labelled `FAIL evidence-rules.md: `, and a
// line of the page `FAIL evidence.md: `, so step 10 of classify.md tells the failures that rows appended to the page can fix
// from the ones they cannot. A page that passes beside a rules file that fails prints no line of the page.
func TestACheckLabelsTheFailLinesOfTheRulesFileApartFromThoseOfThePage(t *testing.T) {
	brokenRules := kindsHead + "| Bad_Kind | 1 | | | | general |\n"
	rulesPath := func(w evidenceWorld) string {
		return filepath.Join(w.tmp, "skills", "_shared", "evidence", "evidence-rules.md")
	}

	w := newEvidenceWorld(t, evidencePage(stdKinds, stdEvidence))
	writeEvidenceFile(t, rulesPath(w), brokenRules)
	wantResult(t, "a rules file that fails beside a page that passes", w.inRepo("check"), 1,
		"FAIL evidence-rules.md: kind \"Bad_Kind\" is not kebab-case\n", "")

	w = newEvidenceWorld(t, evidencePage(stdKinds, "| ghost-kind | 1 | f/aaaa-1 | 2026-01-01 | a |\n"))
	writeEvidenceFile(t, rulesPath(w), brokenRules)
	wantResult(t, "a rules file and a page that both fail", w.inRepo("check"), 1,
		"FAIL evidence-rules.md: kind \"Bad_Kind\" is not kebab-case\nFAIL evidence.md: evidence of kind \"ghost-kind\" has no row in Kinds\n", "")
}

// R7: every line a failed check prints on stdout is a line `FAIL evidence.md: <problem>`, so the lines the step hands back as
// `<failed>` are exactly the lines that start with FAIL. The page has an Evidence row of a kind it lacks and a date that is
// not YYYY-MM-DD, two problems.
func TestEveryStdoutLineOfAFailedCheckIsAFailedLine(t *testing.T) {
	w := newEvidenceWorld(t, evidencePage("", "| unknown-kind | 1 | f/aaaa-1 | 2026/11/19 | x |\n"))
	got := w.inRepo("check")
	if got.Exit != 1 {
		t.Fatalf("check exited %d, want 1: stdout %q, stderr %q", got.Exit, got.Stdout, got.Stderr)
	}
	lines := strings.Split(strings.TrimSuffix(got.Stdout, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("check printed %d lines, want two or more: %q", len(lines), got.Stdout)
	}
	failed := regexp.MustCompile(`^FAIL evidence\.md: .+$`)
	for _, line := range lines {
		if !failed.MatchString(line) {
			t.Errorf("the stdout line %q of a failed check is not a line FAIL evidence.md: <problem>", line)
		}
	}
}
