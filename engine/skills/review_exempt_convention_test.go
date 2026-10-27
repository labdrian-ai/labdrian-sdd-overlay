package skills

import (
	"strings"
	"testing"
)

// The convention of the review results also says what an exemption of a head from review is (own-rdd T4, S5, L16), so
// that a head is never delivered on an exemption that the engine did not check, and an exemption is never taken for a
// review. Each rule below is held by the fragments listed under it, so a failure says which rule the README no longer
// states; a fragment is an identifier or a short phrase of the rule, not the sentence that carries it.
func TestReviewResultsConventionSaysWhatAnExemptionIsAndWhatClearsAHeadByIt(t *testing.T) {
	readme := readRepoFile(t, skillsRepoRoot(t), "odd/review-results/README.md")
	for _, rule := range []struct {
		name      string
		fragments []string
	}{
		{"where an exemption is stored and what the stored file holds", []string{
			"## The exemption of a head from review", "`labdrian review exempt`", "`odd/review-results/<feature>/<head12>-<n>.exempt.json`",
			"a verdict and an exemption count", "`binding`", "`risk`", "`report`",
		}},
		{"who may exempt a head and on what evidence", []string{
			"The report is read from stdin and from nowhere else", "`--risk`", "a head that names any item is never exempt",
			"`review.ErrHighRisk`", "`review.ErrInvalidRisk`", "`reviewverdict.ParseReport`", "`ErrNotClear`",
			"a check that failed or did not run never passes", "2 report refused", "a risk that is refused or any other failure",
		}},
		{"what the record of the chain that closes an exemption is held to", []string{
			"`review-exemption`", "`labdrian roles append` holds a completed `review-exemption` record to the exemption",
			"`review.CheckExemption`", "an exemption that was edited after it was made is refused for what it now says",
		}},
		{"what clears a head by an exemption", []string{
			"or when an exemption binds it and no result does", "A result that blocks wins over any exemption", "`exemptions`",
		}},
		{"that an exemption is never a review", []string{
			"An exemption grants nothing by itself", "never review authority",
		}},
	} {
		for _, fragment := range rule.fragments {
			if !strings.Contains(readme, fragment) {
				t.Errorf("odd/review-results/README.md must state %s: it lacks %q", rule.name, fragment)
			}
		}
	}
}
