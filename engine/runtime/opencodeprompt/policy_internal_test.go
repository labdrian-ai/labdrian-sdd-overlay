package opencodeprompt

import (
	"errors"
	"testing"
)

// onMalformed is the one place the owner's decision of 2026-10-09 lives (H26,
// R3-opencode-secondary-contract-silent-drop): what the loader does with a contract whose
// frontmatter does not parse. The outcome of Derive follows from it alone, so a different decision
// is a change to this function and to this test.
func TestOnMalformedAbortsAndNamesTheContract(t *testing.T) {
	parseErr := errors.New("no frontmatter")

	got := onMalformed("skills/_shared/oo-quality-contract.md", parseErr)

	if got == nil {
		t.Fatal("onMalformed = nil, want the contract to abort the whole prompt config")
	}
	var malformed *MalformedContractError
	if !errors.As(got, &malformed) || malformed.Path != "skills/_shared/oo-quality-contract.md" || !errors.Is(got, parseErr) {
		t.Errorf("onMalformed = %v, want a *MalformedContractError that names the file and wraps the parse error", got)
	}
	if want := "skills/_shared/oo-quality-contract.md: no frontmatter"; got.Error() != want {
		t.Errorf("message = %q, want %q", got.Error(), want)
	}
}
