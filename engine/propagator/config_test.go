package propagator_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/contract"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator"
)

// The package names no contract: which markers delimit a contract's block and what its row is
// called are the caller's, and a Config that leaves one out is refused rather than given the
// minimalism contract's, which is what the package once did and what made a second contract the
// exception that had to remember to override them.
func TestPropagateRefusesAConfigThatDoesNotScopeABlock(t *testing.T) {
	doc, err := contract.Parse(contractFrontmatter)
	if err != nil {
		t.Fatalf("contract.Parse: %v", err)
	}
	registry := reg("# R", pinHeader, pinHead, pinRule, pinFirst)
	for _, tc := range []struct {
		name string
		edit func(*propagator.Config)
		says string
	}{
		{"no begin marker", func(c *propagator.Config) { c.BeginMarker = "" }, "no begin marker"},
		{"no end marker", func(c *propagator.Config) { c.EndMarker = "" }, "no end marker"},
		{"no row label", func(c *propagator.Config) { c.RowLabel = "" }, "no row label"},
		{"a begin marker that does not open a block", func(c *propagator.Config) { c.BeginMarker = "minimalism-contract-scope" }, "begin marker \"minimalism-contract-scope\" does not start with"},
		{"an end marker that does not close a block", func(c *propagator.Config) { c.EndMarker = "minimalism-contract-scope" }, "end marker \"minimalism-contract-scope\" does not start with"},
		{"a begin marker that closes one", func(c *propagator.Config) { c.BeginMarker = minimalismEnd }, "does not start with"},
		{"an end marker that opens one", func(c *propagator.Config) { c.EndMarker = minimalismBegin }, "does not start with"},
		{"a begin marker with space before it", func(c *propagator.Config) { c.BeginMarker = " " + minimalismBegin }, "does not start with"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := minimalismConfig()
			tc.edit(&cfg)

			got, changed, err := propagator.Propagate(registry, cfg, doc)
			if !errors.Is(err, propagator.ErrInvalidConfig) {
				t.Fatalf("err = %v, want one that is propagator.ErrInvalidConfig", err)
			}
			if !strings.Contains(err.Error(), tc.says) {
				t.Errorf("err = %q, want it to say %q", err, tc.says)
			}
			if got != registry || changed {
				t.Errorf("a refused Config returned (%q, %v), want the registry as it was and false", got, changed)
			}
		})
	}
}

// The path of the contract is what the row says; it does not scope the block, and an empty one is
// written as it always was.
func TestPropagateTakesAConfigWhoseContractPathIsEmpty(t *testing.T) {
	doc, err := contract.Parse(contractFrontmatter)
	if err != nil {
		t.Fatalf("contract.Parse: %v", err)
	}
	cfg := minimalismConfig()
	cfg.ContractPath = ""

	got, changed, err := propagator.Propagate(reg("# R", pinHeader, pinHead, pinRule, pinFirst), cfg, doc)
	if err != nil || !changed {
		t.Fatalf("Propagate = (%v, %v), want a written registry", changed, err)
	}
	want := reg("# R", pinHeader, pinHead, pinRule, pinFirst, minimalismBegin,
		"| minimalism-contract |  | Inject ONLY into sdd-tasks and sdd-apply sub-agent prompts under '## Skills to load before work'. Do NOT inject into sdd-propose/sdd-spec/sdd-design/sdd-verify/sdd-archive. |",
		minimalismEnd)
	if got != want {
		t.Errorf("Propagate wrote\n%q\nwant\n%q", got, want)
	}
}
