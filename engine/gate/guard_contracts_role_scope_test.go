package gate_test

// Tests for what the gate does with the oo-quality contract and the anti-generic-design guard the
// repository really ships, for the sub-agent types of the role agents. PR 5 of the role agents
// (per-role design v3, section 6) deleted the sdd-* agents these two named, so they are scoped to
// the roles that took over that work, as the minimalism contract was in PR 1: the oo-quality
// contract guides the shaper and the builder, the anti-generic-design guard guides the builder,
// and neither reaches the reviewer or the verifier. Their phase lists keep the sdd-* names.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gate"
)

const antiGenericPath = "skills/_shared/anti-generic-design.md"

// repoContract is the gate configuration for the contract at path as the repository ships it,
// with work metadata that satisfies a context-aware contract: trusted, TypeScript, OO domain
// design, application code.
func repoContract(t *testing.T, path string) gate.Config {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", path))
	if err != nil {
		t.Fatalf("reading %s of the repository: %v", path, err)
	}
	return gate.Config{
		Contracts: []gate.ContractConfig{{Path: path, Content: string(data)}},
		WorkContext: &gate.WorkContext{
			Trusted:     true,
			Languages:   []string{"typescript"},
			Activations: []string{"oo-domain-design"},
			WorkKinds:   []string{"application-code"},
		},
	}
}

func TestTheGuardContractsAreInjectedIntoTheRolesTheyGuide(t *testing.T) {
	for path, roles := range map[string][]string{
		ooContractPath:  {"shaper", "builder"},
		antiGenericPath: {"builder"},
	} {
		for _, role := range roles {
			t.Run(path+"/"+role, func(t *testing.T) {
				got, changed := gate.Rewrite(gate.Call{SubagentType: role, Prompt: "Do the work."}, repoContract(t, path))
				if !changed || !hasLine(got, path) {
					t.Fatalf("the gate did not inject %s into the prompt of %s (changed=%v); got:\n%s", path, role, changed, got)
				}
			})
		}
	}
}

func TestTheGuardContractsAreStrippedFromTheReviewRoles(t *testing.T) {
	for _, path := range []string{ooContractPath, antiGenericPath} {
		for _, role := range []string{"reviewer", "verifier"} {
			t.Run(path+"/"+role, func(t *testing.T) {
				listed := "Do the work.\n\n" + skillsHeader + "\n" + path + "\n"
				got, changed := gate.Rewrite(gate.Call{SubagentType: role, Prompt: listed}, repoContract(t, path))
				if !changed || hasLine(got, path) {
					t.Fatalf("the gate left %s in the prompt of %s (changed=%v); got:\n%s", path, role, changed, got)
				}
			})
		}
	}
}

// The sdd-* agents the contracts named left with PR 5 (the owner's decision kept them "until their
// agents are removed"), so no contract of the repository is injected into a prompt launched under
// one of those names any more.
func TestNoShippedContractIsInjectedIntoTheRemovedSddAgents(t *testing.T) {
	for _, path := range []string{"skills/_shared/minimalism-contract.md", ooContractPath, antiGenericPath} {
		for _, name := range []string{"sdd-design", "sdd-tasks", "sdd-apply"} {
			t.Run(path+"/"+name, func(t *testing.T) {
				if got, changed := gate.Rewrite(gate.Call{SubagentType: name, Prompt: "Do the work."}, repoContract(t, path)); changed {
					t.Errorf("the gate injected into the prompt of %s; got:\n%s", name, got)
				}
			})
		}
	}
}

// The shaper also designs, and the anti-generic-design guard has always stayed out of design: it
// is not injected into the shaper's prompt.
func TestTheAntiGenericGuardIsNotInjectedIntoTheShaper(t *testing.T) {
	if got, changed := gate.Rewrite(gate.Call{SubagentType: "shaper", Prompt: "Do the work."}, repoContract(t, antiGenericPath)); changed {
		t.Errorf("the gate changed the shaper's prompt; got:\n%s", got)
	}
}
