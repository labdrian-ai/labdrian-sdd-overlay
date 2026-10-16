package propagator_test

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/contract"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator"
)

// This file pins what Propagate does to a registry, whole: the registry that comes out for the
// registry that goes in, for the shapes of registry the three scans over its lines tell apart. The
// older tests of this package look for a marker or a phrase in the output; a scan that moved a block
// one line, or kept an indentation, passed them. Every line a block and a row take, and the lines
// around them, are compared here.
//
// The scans are the three questions Propagate asks of the lines: is there an unscoped row of this
// contract (outside every marker block), which line is it on, and where does the table of
// "### Shared Contracts" end.

const (
	minimalismBegin = "<!-- BEGIN: minimalism-contract-scope (auto-generated) -->"
	minimalismEnd   = "<!-- END: minimalism-contract-scope -->"
	minimalismRow   = "| minimalism-contract | skills/_shared/minimalism-contract.md | Inject ONLY into sdd-tasks and sdd-apply sub-agent prompts under '## Skills to load before work'. Do NOT inject into sdd-propose/sdd-spec/sdd-design/sdd-verify/sdd-archive. |"
	minimalismBlock = minimalismBegin + "\n" + minimalismRow + "\n" + minimalismEnd
	designBegin     = "<!-- BEGIN: anti-generic-design-scope (auto-generated) -->"
	designEnd       = "<!-- END: anti-generic-design-scope -->"
	designRow       = "| anti-generic-design | skills/_shared/anti-generic-design.md | Inject ONLY into sdd-tasks and sdd-apply sub-agent prompts under '## Skills to load before work'. Do NOT inject into sdd-propose/sdd-spec/sdd-design/sdd-verify/sdd-archive. |"
	designBlock     = designBegin + "\n" + designRow + "\n" + designEnd

	pinHeader = "### Shared Contracts"
	pinHead   = "| Artifact | Path | Description |"
	pinRule   = "|----------|------|-------------|"
	pinFirst  = "| pre-sdd-contracts | skills/_shared/pre-sdd-contracts.md | Shared contracts |"
	pinStale  = "| minimalism-contract | old/path.md | old text |"
	pinOther  = "<!-- BEGIN: other -->"
	pinOtherE = "<!-- END: other -->"
)

// reg is a registry of these lines, each ended by a newline.
func reg(lines ...string) string {
	out := ""
	for _, l := range lines {
		out += l + "\n"
	}
	return out
}

func minimalismConfig() propagator.Config {
	return propagator.Config{
		ContractPath: "skills/_shared/minimalism-contract.md",
		BeginMarker:  minimalismBegin,
		EndMarker:    minimalismEnd,
		RowLabel:     "minimalism-contract",
	}
}

func designConfig() propagator.Config {
	return propagator.Config{
		ContractPath: "skills/_shared/anti-generic-design.md",
		BeginMarker:  designBegin,
		EndMarker:    designEnd,
		RowLabel:     "anti-generic-design",
	}
}

func TestPropagateWritesTheRegistryThisWayForEveryShapeOfItsLines(t *testing.T) {
	tests := []struct {
		name        string
		cfg         propagator.Config
		in, want    string
		wantChanged bool
	}{
		// Where the block goes when there is no row to correct: the end of the table under
		// "### Shared Contracts", or the end of the file.
		{"an empty registry gets the block after a blank line", minimalismConfig(),
			"", "\n" + minimalismBlock + "\n", true},
		{"no Shared Contracts section: the block goes at the end of the file", minimalismConfig(),
			reg("# R", "## Skills", "| a | b |"),
			reg("# R", "## Skills", "| a | b |") + "\n" + minimalismBlock + "\n", true},
		{"the header with no table under it: the block goes at the end of the file", minimalismConfig(),
			reg("# R", pinHeader, "text only"),
			reg("# R", pinHeader, "text only") + "\n" + minimalismBlock + "\n", true},
		{"after the last row of the table, before the next heading", minimalismConfig(),
			reg("# R", pinHeader, pinHead, pinRule, pinFirst, "", "## Other", "| x | y |"),
			reg("# R", pinHeader, pinHead, pinRule, pinFirst, minimalismBlock, "", "## Other", "| x | y |"), true},
		{"a table that ends the file", minimalismConfig(),
			reg("# R", pinHeader, pinHead, pinRule, pinFirst),
			reg("# R", pinHeader, pinHead, pinRule, pinFirst, minimalismBlock), true},
		{"rows inside a foreign block do not count: the closing line ends the table", minimalismConfig(),
			reg("# R", pinHeader, pinHead, pinRule, pinFirst, pinOther, "| other | p | d |", pinOtherE, "", "text"),
			reg("# R", pinHeader, pinHead, pinRule, pinFirst, pinOther, "| other | p | d |", pinOtherE, minimalismBlock, "", "text"), true},
		{"a foreign block that is never closed: the rows after its opening line are out of the table", minimalismConfig(),
			reg("# R", pinHeader, pinHead, pinRule, pinFirst, pinOther, "| other | p | d |"),
			reg("# R", pinHeader, pinHead, pinRule, pinFirst, minimalismBlock, pinOther, "| other | p | d |"), true},
		{"a closing marker with no opening one, in the section, ends the table", minimalismConfig(),
			reg("# R", pinHeader, pinHead, pinRule, pinFirst, "| r2 | p | d |", "<!-- END: stray -->", "text"),
			reg("# R", pinHeader, pinHead, pinRule, pinFirst, "| r2 | p | d |", "<!-- END: stray -->", minimalismBlock, "text"), true},
		{"a closing marker with no opening one, before the section, is not the table", minimalismConfig(),
			reg("<!-- END: stray -->", "# R", pinHeader, pinHead),
			reg("<!-- END: stray -->", "# R", pinHeader, pinHead, minimalismBlock), true},
		{"an indented header and indented rows", minimalismConfig(),
			reg("# R", "  "+pinHeader, "  | a | b |"),
			reg("# R", "  "+pinHeader, "  | a | b |", minimalismBlock), true},
		{"a second header that starts with the first is the same section", minimalismConfig(),
			reg("# R", pinHeader, "| a | b |", pinHeader+" (more)", "| c | d |", "## Next"),
			reg("# R", pinHeader, "| a | b |", pinHeader+" (more)", "| c | d |", minimalismBlock, "## Next"), true},
		{"the header on the first line, a heading of the top level ends the section", minimalismConfig(),
			reg(pinHeader, "| a | b |", "# Title", "| c | d |"),
			reg(pinHeader, "| a | b |", minimalismBlock, "# Title", "| c | d |"), true},
		{"lines ended by CR LF keep their CR and the block is ended by LF", minimalismConfig(),
			"# R\r\n" + pinHeader + "\r\n| a | b |\r\n",
			"# R\r\n" + pinHeader + "\r\n| a | b |\r\n" + minimalismBlock + "\n", true},

		// An unscoped row of this contract: outside every marker block, its first cell exactly the
		// label of the contract.
		{"an unscoped row is replaced where it stands", minimalismConfig(),
			reg("# R", pinHeader, pinHead, pinRule, pinFirst, pinStale, "| z | p | d |"),
			reg("# R", pinHeader, pinHead, pinRule, pinFirst, minimalismBlock, "| z | p | d |"), true},
		{"every unscoped row is replaced, each by a whole block", minimalismConfig(),
			reg("# R", pinHeader, pinStale, "| z | p | d |", pinStale),
			reg("# R", pinHeader, minimalismBlock, "| z | p | d |", minimalismBlock), true},
		{"an indented unscoped row is replaced and its indentation goes with it", minimalismConfig(),
			reg("# R", pinHeader, "   "+pinStale),
			reg("# R", pinHeader, minimalismBlock), true},
		{"the lines around a replaced row keep their indentation and their CR", minimalismConfig(),
			"# R\r\n   " + pinHeader + "  \r\n" + pinStale + "\r\n\t| z | p | d |\r\n",
			"# R\r\n   " + pinHeader + "  \r\n" + minimalismBlock + "\n\t| z | p | d |\r\n", true},
		{"a first cell that only contains the label is not the contract's row", minimalismConfig(),
			reg("# R", pinHeader, pinFirst, "| minimalism-contract-x | p | d |", "| other | minimalism-contract | d |"),
			reg("# R", pinHeader, pinFirst, "| minimalism-contract-x | p | d |", "| other | minimalism-contract | d |", minimalismBlock), true},
		{"a row inside a foreign block is not unscoped", minimalismConfig(),
			reg("# R", pinHeader, pinHead, pinRule, pinFirst, pinOther, pinStale, pinOtherE),
			reg("# R", pinHeader, pinHead, pinRule, pinFirst, pinOther, pinStale, pinOtherE, minimalismBlock), true},
		{"a row of the label inside a foreign block stays while the unscoped one outside it is replaced", minimalismConfig(),
			reg(pinHeader, pinOther, pinStale, pinOtherE, pinStale),
			reg(pinHeader, pinOther, pinStale, pinOtherE, minimalismBlock), true},
		{"a row after a foreign block closes is unscoped again", minimalismConfig(),
			reg(pinHeader, pinOther, "| a | b |", pinOtherE, pinStale),
			reg(pinHeader, pinOther, "| a | b |", pinOtherE, minimalismBlock), true},
		{"an indented foreign marker still opens a block", minimalismConfig(),
			reg(pinHeader, "   "+pinOther, pinStale, "   "+pinOtherE),
			reg(pinHeader, "   "+pinOther, pinStale, "   "+pinOtherE, minimalismBlock), true},
		{"a foreign block that is never closed protects every row after it", minimalismConfig(),
			reg(pinHeader, "| a | b |", pinOther, pinStale),
			reg(pinHeader, "| a | b |", minimalismBlock, pinOther, pinStale), true},

		// The block of this contract: replaced between its own markers, left alone when it is right
		// or cannot be told, and never mistaken for another contract's.
		{"a stale block is replaced between its own markers", minimalismConfig(),
			reg(pinHeader, pinHead, minimalismBegin, pinStale, minimalismEnd, "| tail | p | d |"),
			reg(pinHeader, pinHead, minimalismBlock, "| tail | p | d |"), true},
		{"the block as it should be anywhere in the registry is left alone", minimalismConfig(),
			reg("# R", pinHeader, minimalismBlock, "x"), reg("# R", pinHeader, minimalismBlock, "x"), false},
		{"an opening marker with no closing one is left as it is", minimalismConfig(),
			reg(pinHeader, minimalismBegin, "| x | y | z |"), reg(pinHeader, minimalismBegin, "| x | y | z |"), false},
		{"a closing marker before the opening one is left as it is", minimalismConfig(),
			reg(pinHeader, minimalismEnd, minimalismBegin), reg(pinHeader, minimalismEnd, minimalismBegin), false},
		{"a second contract's block goes beside the first, in the same table", designConfig(),
			reg("# R", pinHeader, pinHead, pinRule, minimalismBlock),
			reg("# R", pinHeader, pinHead, pinRule, minimalismBlock, designBlock), true},
		{"a stale block of the second contract is replaced and the first's is not touched", designConfig(),
			reg(pinHeader, minimalismBlock, designBegin, "| anti-generic-design | old | old |", designEnd, "tail"),
			reg(pinHeader, minimalismBlock, designBlock, "tail"), true},
		{"an unscoped row of another contract is not this contract's row", designConfig(),
			reg(pinHeader, pinHead, pinStale),
			reg(pinHeader, pinHead, pinStale, designBlock), true},
	}

	doc, err := contract.Parse(contractFrontmatter)
	if err != nil {
		t.Fatalf("contract.Parse: %v", err)
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, changed, err := propagator.Propagate(tc.in, tc.cfg, doc)
			if err != nil {
				t.Fatalf("Propagate: %v", err)
			}
			if got != tc.want {
				t.Errorf("Propagate wrote\n%q\nwant\n%q", got, tc.want)
			}
			if changed != tc.wantChanged {
				t.Errorf("changed = %v, want %v", changed, tc.wantChanged)
			}
			again, changedAgain, err := propagator.Propagate(got, tc.cfg, doc)
			if err != nil {
				t.Fatalf("Propagate (again): %v", err)
			}
			if tc.wantChanged && (again != got || changedAgain) {
				t.Errorf("a second Propagate changed the registry again (changed=%v):\n%q", changedAgain, again)
			}
		})
	}
}
