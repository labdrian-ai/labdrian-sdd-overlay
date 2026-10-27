package skills

import (
	"strings"
	"testing"
)

// The convention of the review results also says what an exemption of a head from review is (own-rdd T4, S5, L16):
// who may make one and on what evidence, where it is stored and what the stored file is, what the record of the chain
// that closes it is held to, and what clears a head by it, so that a head is never delivered on an exemption that
// the engine did not check, and an exemption is never taken for a review.
func TestReviewResultsConventionSaysWhatAnExemptionIsAndWhatClearsAHeadByIt(t *testing.T) {
	readme := readRepoFile(t, skillsRepoRoot(t), "odd/review-results/README.md")
	for _, fragment := range []string{
		"`labdrian review exempt`", "## The exemption of a head from review", "`<head12>-<n>.exempt.json`",
		"`odd/review-results/<feature>/<head12>-<n>.exempt.json`", "a verdict and an exemption count",
		"The report is read from stdin and from nowhere else",
		"`--risk`", "a head that names any item is never exempt", "`review.ErrHighRisk`", "`review.ErrInvalidRisk`",
		"`reviewverdict.ParseReport`", "`ErrNotClear`", "a check that failed or did not run never passes",
		"`binding`", "`risk`", "`report`",
		"Exit 0 stored, 2 report refused (the error names the rule, and nothing was written), 1 usage error, a risk that is refused or any other failure",
		"`review-exemption`", "`labdrian roles append` holds a completed `review-exemption` record to the exemption",
		"an exemption that was edited after it was made is refused for what it now says",
		"at least one result binds it and every one is clear, or when an exemption binds it and no result does",
		"A result that blocks wins over any exemption",
		"`exemptions`", "`review.CheckExemption`",
		"An exemption grants nothing by itself", "never review authority",
	} {
		if !strings.Contains(readme, fragment) {
			t.Errorf("odd/review-results/README.md must carry %q", fragment)
		}
	}
}
