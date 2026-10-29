package main

// The user documents of what a commit needs before its review (own-rdd T6b7b, S6 d, e, f and h): the README entries of
// `review check` and `review sweep`, the records README that lists the three role records, and the CHANGELOG clauses that
// point at the help and the README section. A fragment is written on one line with no code marks, because the texts are
// compared after normalizeDocText.

import (
	"strings"
	"testing"
)

// The README entry of `review check` says what a commit needs before its review and that the first thing it lacks
// refuses it, the passive commit and the paths that make one.
func TestReadmeStatesWhatReviewCheckAsksOfACommit(t *testing.T) {
	section := docSection(t, readRepositoryFile(t, "README.md"), "    check  --feature", "    exempt --feature")

	for _, want := range []string{
		"the first thing it lacks refuses it",
		"a sweeper record that answers it",
		"A passive commit",
		"odd/tasks/, odd/review-results/ or odd/review-receipts/",
	} {
		if !strings.Contains(section, want) {
			t.Errorf("the README entry of review check does not say %q", want)
		}
	}
}

// The README entry of `review sweep` says what stops the sweep and so the delivery: the tools it needs on the PATH, and
// a tree it cannot copy (S6 h).
func TestReadmeStatesTheLimitsOfReviewSweep(t *testing.T) {
	section := docSection(t, readRepositoryFile(t, "README.md"), "    sweep  --feature", "    record --role sweeper|polisher")

	for _, want := range []string{
		"it needs deadcode, staticcheck and shellcheck on PATH",
		"256 MiB",
		"cannot be swept",
		"delivery stops",
	} {
		if !strings.Contains(section, want) {
			t.Errorf("the README entry of review sweep does not say %q", want)
		}
	}
	// No task makes the copy stream (L63), so the README promises no streaming. It is compared after
	// normalizeDocText because the phrase wraps across lines.
	if text := normalizeDocText(readRepositoryFile(t, "README.md")); strings.Contains(text, "until the copy streams") {
		t.Errorf("the README still says %q", "until the copy streams")
	}
}

// The records README names the three role records, the verbs that write them and that `review check` holds them.
func TestRecordsReadmeNamesTheRoleRecordsAndTheirVerbs(t *testing.T) {
	text := normalizeDocText(readRepositoryFile(t, "odd/review-results/README.md"))

	for _, want := range []string{
		"<head12>-<n>.sweep.json",
		"<head12>-<n>.sweeper.json",
		"<head12>-<n>.polisher.json",
		"review sweep",
		"review record --role sweeper",
		"review record --role polisher",
		"review check holds them",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("odd/review-results/README.md does not say %q", want)
		}
	}
}

// The CHANGELOG entries of T6b5 and T6b6 point at the help and at the README section that the later tasks wrote.
func TestChangelogNamesTheHelpAndTheReadmeOfWhatACommitNeeds(t *testing.T) {
	changelog := readRepositoryFile(t, "CHANGELOG.md")
	for _, tc := range []struct{ start, want string }{
		{"- `review check` asks for what a commit needs", "The help of review check and review exempt states these rules (T6b5a)"},
		{"- The `labdrian review stop-hook` asks for what a commit needs", "the README sections overlay review <verb> and gentle-ai-overlay review stop-hook describe it (T6b7b)"},
	} {
		entry := docSection(t, changelog, tc.start, "\n")
		if !strings.Contains(entry, tc.want) {
			t.Errorf("the CHANGELOG entry %q does not say %q", tc.start, tc.want)
		}
	}
}
