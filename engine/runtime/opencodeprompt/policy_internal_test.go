package opencodeprompt

import (
	"errors"
	"testing"
)

// onMalformed is the one place the owner's pending decision lives (H26,
// R3-opencode-secondary-contract-silent-drop): what the loader does with a contract whose
// frontmatter does not parse. The outcome of Derive follows from it alone, so a different
// decision is a change to this function and to these two cases.
func TestOnMalformedAbortsForARequiredContractAndDropsAnOptionalOne(t *testing.T) {
	parseErr := errors.New("no frontmatter")

	if got := onMalformed(requiredContract, parseErr); !errors.Is(got, parseErr) {
		t.Errorf("onMalformed(required) = %v, want the parse error: the whole prompt config is abandoned", got)
	}
	if got := onMalformed(optionalContract, parseErr); got != nil {
		t.Errorf("onMalformed(optional) = %v, want nil: the contract is dropped in silence", got)
	}
}
