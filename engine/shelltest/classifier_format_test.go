package shelltest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// markdownFencesAfter are the contents of the markdown fences of the body after its heading, in order,
// each without its fence lines.
func markdownFencesAfter(body, heading string) [][]string {
	_, rest, found := strings.Cut(body, "\n"+heading+"\n")
	if !found {
		return nil
	}
	var fences [][]string
	var open []string
	in := false
	for _, line := range strings.Split(rest, "\n") {
		switch {
		case !in && line == "```markdown":
			in = true
			open = nil
		case in && line == "```":
			in = false
			fences = append(fences, open)
		case in:
			open = append(open, line)
		}
	}
	return fences
}

// K4 (R2-F1, L47): the Format of the body of the classifier is what evidence.sh check accepts. The empty
// page of the first fence passes, and passes again with the two example rows of the second fence under
// their tables, so a renamed header or a malformed example fails here and not in a deployed run.
func TestTheFormatOfTheClassifierBodyPassesEvidenceCheck(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(overlaytest.RepoRoot(t), "roles", "classifier", "body.md"))
	if err != nil {
		t.Fatal(err)
	}
	fences := markdownFencesAfter(string(body), "## Format")
	if len(fences) != 2 || len(fences[1]) != 2 {
		t.Fatalf("the Format of roles/classifier/body.md holds %d markdown fences (the second with %d lines), want the empty page and two example rows", len(fences), len(fences[len(fences)-1]))
	}
	empty := strings.Join(fences[0], "\n") + "\n"

	w := newEvidenceWorld(t, empty)
	wantResult(t, "check of the empty page", w.inRepo("check"), 0, "evidence check passed: docs/wiki/evidence.md\n", "")

	// The Kinds example goes right under the header of Kinds, the Evidence example at the end of the page.
	kindsEnd := "|------|-------|------|--------------|--------|-------|\n"
	withKinds := strings.Replace(empty, kindsEnd, kindsEnd+fences[1][0]+"\n", 1)
	if withKinds == empty {
		t.Fatalf("the empty page does not hold the Kinds header line %q", kindsEnd)
	}
	writeEvidenceFile(t, filepath.Join(w.repo, "docs", "wiki", "evidence.md"), withKinds+fences[1][1]+"\n")
	wantResult(t, "check of the page with the example rows", w.inRepo("check"), 0, "evidence check passed: docs/wiki/evidence.md\n", "")
}
