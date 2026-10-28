package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The README of the overlay documents the exemption of a head from review where it documents the other review verbs
// (own-rdd T4, S5, L16). The fragments are grouped by the rule of the verb they guard, so that a failure says which
// rule the README no longer states. A section is what docSection returns, normalized by normalizeDocText (white space
// collapsed to single spaces, backticks dropped), so a fragment is written on one line with no code marks: a re-wrap of
// the README does not break it, and a reworded sentence does.
func TestReadmeDocumentsReviewExemptWhereItDocumentsTheOtherReviewVerbs(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	const reviewVerbs, rolesVerbs = "overlay review <verb>", "overlay roles <verb>"
	sections := map[string]string{
		reviewVerbs: docSection(t, string(readme), reviewVerbs+"\n", "overlay memory <verb>\n"),
		rolesVerbs:  docSection(t, string(readme), rolesVerbs+"\n", reviewVerbs+"\n"),
	}
	for _, rule := range []struct {
		section, rule string
		fragments     []string
	}{
		{reviewVerbs, "the verb and the report it reads", []string{
			"exempt --feature <name> --base <rev> --head <rev> --risk <none|items> [--cwd <repo>]",
			"The verifier's report is read from stdin and from nowhere else",
			`{"checks": [{"command": ..., "result": pass, fail or not-run, "output": ...}]}`,
		}},
		{reviewVerbs, "the risk the agent declares", []string{
			"--risk is none or the numbers 1 to 6 of the high-risk items",
			"a head that names any item is never exempt",
		}},
		{reviewVerbs, "the head's own Risk trailer (T6b5, S6 f)", []string{
			"a head whose own Risk trailer is missing, malformed or names an item is refused",
		}},
		{reviewVerbs, "an exemption is for one commit", []string{
			"is for one commit: --base must be the first parent of --head",
			"a range whose base is its head covers no commit and is refused",
		}},
		{reviewVerbs, "what it stores and prints, and how it exits", []string{
			"<head12>-<n>.exempt.json",
			"numbered with the results and the verdicts of the head and never over an existing file",
			"payload_kind review-exemption",
			"Exit 0 stored, 2 report refused, 1 usage error, a risk that is refused or any other failure",
		}},
		{reviewVerbs, "what check makes of an exempted head", []string{
			"check clears an exempted head unless a stored result blocks it",
			"or when an exemption binds it and no result does",
			"whatever exemption exists",
			"stdout is one JSON object (head, outcome, results, and exemptions when one of them cleared the head)",
		}},
		{rolesVerbs, "what roles append holds a review-exemption to", []string{
			"a completed review-exemption whose review-diff or review-exemption evidence is missing or differs from the file review exempt stored",
		}},
	} {
		for _, want := range rule.fragments {
			if !strings.Contains(sections[rule.section], want) {
				t.Errorf("the README section %q must state %s: it lacks %q (sections are compared after normalizeDocText, with no backticks and one space between words)", rule.section, rule.rule, want)
			}
		}
	}
}
