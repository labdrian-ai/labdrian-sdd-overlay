package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The README of the overlay documents the exemption of a head from review where it documents the other review verbs
// (own-rdd T4, S5, L16): the verb with its flags, where it reads the report of the verifier from, the grammar of the
// declared risk, what it stores and prints, what `review check` makes of an exempted head, and the evidence that
// `roles append` holds the record to. The sections are read with the line breaks and the code marks taken out, so
// that it is the words that are compared.
func TestReadmeDocumentsReviewExemptWhereItDocumentsTheOtherReviewVerbs(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	sections := map[string]struct {
		text string
		want []string
	}{
		"overlay review <verb>": {
			docSection(t, string(readme), "overlay review <verb>\n", "overlay memory <verb>\n"),
			[]string{
				"exempt --feature <name> --base <rev> --head <rev> --risk <none|items> [--cwd <repo>]",
				"The verifier's report is read from stdin and from nowhere else",
				`{"checks": [{"command": ..., "result": pass, fail or not-run, "output": ...}]}`,
				"--risk is none or the numbers 1 to 6 of the high-risk items",
				"a head that names any item is never exempt",
				"<head12>-<n>.exempt.json",
				"numbered with the results and the verdicts of the head and never over an existing file",
				"payload_kind review-exemption",
				"check clears an exempted head unless a stored result blocks it",
				"Exit 0 stored, 2 report refused, 1 usage error, a risk that is refused or any other failure",
				"or when an exemption binds it and no result does",
				"whatever exemption exists",
				"stdout is one JSON object (head, outcome, results, and exemptions when one of them cleared the head)",
			},
		},
		"overlay roles <verb>": {
			docSection(t, string(readme), "overlay roles <verb>\n", "overlay review <verb>\n"),
			[]string{
				"a completed review-exemption whose review-diff or review-exemption evidence is missing or differs from the file review exempt stored",
			},
		},
	}
	for name, section := range sections {
		for _, want := range section.want {
			if !strings.Contains(section.text, want) {
				t.Errorf("the README section %q does not say %q", name, want)
			}
		}
	}
}
