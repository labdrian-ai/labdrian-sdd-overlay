package review

import "github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"

// IsCorrection reports whether the candidate current is a correction of the candidate earlier: the same base
// and another head. When a review blocks, the builder fixes the findings with commits on top of the head that
// was reviewed, and the fix is reviewed against the same base, so a candidate is named by its base and each
// of its heads is one try. A second launch on the same head is no correction, and a range from another base
// is another candidate (a review of the commits one by one has a base for each).
//
// Which candidate a review corrects is all the rule "at most one correction round per candidate" needs to
// know. The limit is that a fix reviewed alone, as the range from the reviewed head to the fix, is another
// base and so another candidate.
func IsCorrection(earlier, current reviewresult.Binding) bool {
	return earlier.Base == current.Base && earlier.Head != current.Head
}
