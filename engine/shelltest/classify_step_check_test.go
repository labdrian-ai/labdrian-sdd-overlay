package shelltest

// What the Classify step of the role chain hands to evidence.sh and reads back (T4c of
// odd/tasks/merge-playbook-into-sdd-overlay.md, L55, L90): the empty page it creates, and the lines a failed check prints,
// which the step passes to the classifier as `<failed>`. Both run the script as it is deployed (newEvidenceWorld).

import (
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
