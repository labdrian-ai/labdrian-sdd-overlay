package propagator_test

// This file pins the registry row that Propagate writes for the minimalism contract the repository
// really ships (Phase 9, per-role design v3, section 6, PR 1): the contract is scoped to the roles
// that write (shaper and builder; sdd-tasks and sdd-apply left with their agents in PR 5) and
// kept out of the roles that judge and verify (reviewer and verifier). The other tests of this package give Propagate a contract written in the test; this one
// reads skills/_shared/minimalism-contract.md, so the row a project's registry is refreshed to
// follows the frontmatter of the file.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/contract"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator"
)

// roleScopedRow is the row the registry of a project holds once `engine propagate` has run against
// the contract as it ships: the phases are named in the order of the contract, joined with " and ",
// and the excluded ones with "/".
const roleScopedRow = "| minimalism-contract | skills/_shared/minimalism-contract.md | Inject ONLY into shaper and builder sub-agent prompts under '## Skills to load before work'. Do NOT inject into sdd-propose/sdd-spec/sdd-design/sdd-verify/sdd-archive/reviewer/verifier. |"

// realMinimalismContract is the contract of the repository, parsed the way `engine propagate` parses it.
func realMinimalismContract(t *testing.T) contract.Contract {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "skills", "_shared", "minimalism-contract.md"))
	if err != nil {
		t.Fatalf("reading the minimalism contract of the repository: %v", err)
	}
	c, err := contract.Parse(string(data))
	if err != nil {
		t.Fatalf("contract.Parse of the minimalism contract of the repository: %v", err)
	}
	return c
}

// A registry that holds the row written for the phases the contract used to name is refreshed to the
// row for the roles it names now, and nothing else in it changes.
func TestPropagateRefreshesTheRowToTheRolesOfTheRealContract(t *testing.T) {
	before := reg(pinHeader, pinHead, pinRule, pinFirst, minimalismBegin, minimalismRow, minimalismEnd)
	want := reg(pinHeader, pinHead, pinRule, pinFirst, minimalismBegin, roleScopedRow, minimalismEnd)

	got, changed, err := propagator.Propagate(before, minimalismConfig(), realMinimalismContract(t))
	if err != nil {
		t.Fatalf("Propagate: %v", err)
	}
	if !changed {
		t.Fatal("changed = false, want the row of the registry refreshed to the roles of the contract")
	}
	if got != want {
		t.Errorf("registry after Propagate:\n%s\nwant:\n%s", got, want)
	}
}

// A registry that holds the row of the contract as it ships is left alone, byte for byte, on every run
// of the hook that propagates it, which is every prompt a person sends.
func TestPropagateLeavesTheRowOfTheRealContractAlone(t *testing.T) {
	current := reg(pinHeader, pinHead, pinRule, pinFirst, minimalismBegin, roleScopedRow, minimalismEnd)

	got, changed, err := propagator.Propagate(current, minimalismConfig(), realMinimalismContract(t))
	if err != nil {
		t.Fatalf("Propagate: %v", err)
	}
	if changed || got != current {
		t.Errorf("Propagate rewrote a registry that holds the current row (changed=%v):\n%s", changed, got)
	}
}
