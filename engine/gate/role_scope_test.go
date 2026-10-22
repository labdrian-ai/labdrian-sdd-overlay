package gate_test

// Tests for what the gate does with the minimalism contract the repository really ships, for the
// sub-agent types of the role agents (Phase 9, per-role design v3, section 6, PR 1): the contract
// is injected into the prompts of the shaper and the builder, kept out of the prompts of the
// reviewer and the verifier, and the other roles are left alone. The tests of gate_test.go give
// the gate a contract written in the test; these read skills/_shared/minimalism-contract.md, so a
// change to its frontmatter is checked against the sub-agent types it is meant for.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gate"
)

// realContract is the gate configuration for the minimalism contract as the repository ships it,
// listed under the same path the other tests of this package use.
func realContract(t *testing.T) gate.Config {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "skills", "_shared", "minimalism-contract.md"))
	if err != nil {
		t.Fatalf("reading the minimalism contract of the repository: %v", err)
	}
	return singleContract(contractPath, string(data))
}

const (
	skillsHeader = "## Skills to load before work"
	listedPrompt = "Do the work.\n\n" + skillsHeader + "\n" + contractPath + "\n"
)

// The roles the contract guides get it injected, once, under the header the contract names, and
// their prompt is kept. sdd-tasks and sdd-apply keep it until their agents are removed.
func TestRealContractIsInjectedIntoTheRolesItGuides(t *testing.T) {
	for _, role := range []string{"shaper", "builder", "sdd-tasks", "sdd-apply"} {
		t.Run(role, func(t *testing.T) {
			got, changed := rewrite(realContract(t), role, "Do the work.")
			if !changed {
				t.Fatalf("the gate left the prompt of %s alone, want the contract injected", role)
			}
			if !hasLine(got, contractPath) {
				t.Errorf("the contract path should be a line of the prompt of %s; got:\n%s", role, got)
			}
			if strings.Count(got, skillsHeader) != 1 {
				t.Errorf("the header %q should appear once; got:\n%s", skillsHeader, got)
			}
			if !strings.HasPrefix(got, "Do the work.") {
				t.Errorf("the prompt must be kept; got:\n%s", got)
			}
		})
	}
}

// The roles that judge and verify never get the contract: a prompt that lists it has it taken out.
func TestRealContractIsStrippedFromTheRolesItExcludes(t *testing.T) {
	for _, role := range []string{"reviewer", "verifier"} {
		t.Run(role, func(t *testing.T) {
			got, changed := rewrite(realContract(t), role, listedPrompt)
			if !changed {
				t.Fatalf("the gate left the prompt of %s alone, want the contract stripped", role)
			}
			assertNoCanonicalEntry(t, got)
		})
	}
}

// The roles the contract names in neither list are left alone, whether or not their prompt lists
// the contract: the polisher judges, the prototyper measures, the sweeper only removes, and the
// delivery moves files.
func TestRealContractLeavesTheOtherRolesAlone(t *testing.T) {
	for _, role := range []string{"polisher", "prototyper", "sweeper", "delivery"} {
		t.Run(role, func(t *testing.T) {
			for name, prompt := range map[string]string{"without the contract": "Do the work.", "with the contract listed": listedPrompt} {
				if got, changed := rewrite(realContract(t), role, prompt); changed || got != prompt {
					t.Errorf("%s, %s: the gate changed the prompt (changed=%v); got:\n%s", role, name, changed, got)
				}
			}
		})
	}
}

// The phases the contract has always excluded stay excluded: a prompt that lists the contract has
// it taken out, whatever else the lists of the contract say.
func TestRealContractStillStripsTheLegacyExcludedPhases(t *testing.T) {
	for _, phase := range []string{"sdd-propose", "sdd-spec", "sdd-design", "sdd-verify", "sdd-archive"} {
		t.Run(phase, func(t *testing.T) {
			got, changed := rewrite(realContract(t), phase, listedPrompt)
			if !changed {
				t.Fatalf("the gate left the prompt of %s alone, want the contract stripped", phase)
			}
			assertNoCanonicalEntry(t, got)
		})
	}
}
