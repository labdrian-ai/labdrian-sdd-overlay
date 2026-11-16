package skills

// Checks of the wording that names the foreman, the parent of the labdrian role chain (odd/tasks/
// labdrian-orchestrator-opencode.md, S2 and S4, T18): the `foreman` row of the Decision Gates of
// odd-feature-doc, the parent the role-chain skill names and the delivery -> foreman hand-off that
// role-chain and inception-pipeline state. The hand-off is the delivery agent's envelope returning to its
// parent: engine/roles keeps delivery terminal, so no `delivery -> foreman` record exists.

import (
	"strings"
	"testing"
)

// odd-feature-doc gives the foreman its part (S2, derived from L2.4): it writes the feature document and
// its memorycard mirror, and never source, tests or another role's segment.
func TestOddFeatureDocSkillGivesTheForemanItsPart(t *testing.T) {
	_, _, body := oddRoleSkillFile(t, "odd-feature-doc")
	_, under := roleChainSections(body)
	_, rows := oddSkillTable(under["Decision Gates"])
	var row []string
	for _, r := range rows {
		if r[0] == "foreman" {
			row = r
		}
	}
	if row == nil {
		t.Fatalf("the Decision Gates table has no foreman row; rows = %v", rows)
	}
	for _, fragment := range []string{"`odd/tasks/<feature>.md`", "memorycard mirror `odd/<feature>/tasks`", "the owner's words verbatim", "evidence it observed"} {
		if !strings.Contains(row[1], fragment) {
			t.Errorf("what the foreman writes must say %q; got %q", fragment, row[1])
		}
	}
	for _, fragment := range []string{"write source or tests", "any path outside `odd/tasks/` and the mirror", "a `shaped:` segment"} {
		if !strings.Contains(row[2], fragment) {
			t.Errorf("what the foreman never does must say %q; got %q", fragment, row[2])
		}
	}
}

// The role-chain skill names the foreman as its parent, in the description that the registry matches and
// in the line before the first heading, and states the hand-off: delivery ends the chain and returns to
// the foreman, not to the owner and not as a new record.
func TestRoleChainSkillNamesTheForemanAsParentAndTheDeliveryHandOff(t *testing.T) {
	_, frontmatter, body := roleChainSkill(t)
	if description := parseYAMLScalar(t, frontmatter, "description"); !strings.Contains(description, "Trigger: the foreman runs the role chain;") {
		t.Errorf("the description = %q, want it to name the foreman as the trigger", description)
	}
	const parent = "Parent: the `foreman`, frontier, medium effort. Tiers: `references/models.md`."
	headings, under := roleChainSections(body)
	if !strings.Contains(under[""], parent+"\n") {
		t.Errorf("the text before the first heading must name the foreman as the parent, exactly:\n%s", parent)
	}
	if description := parseYAMLScalar(t, frontmatter, "description"); !strings.Contains(description, "`delivery` returns to the foreman, not the owner.") {
		t.Errorf("the description = %q, want it to state the delivery -> foreman hand-off", description)
	}
	if strings.Contains(strings.Join(headings, "\n")+body, "orchestrator") {
		t.Error("the role-chain skill must not call the parent an orchestrator")
	}
}

// The inception-pipeline skill names the foreman as the parent that enters the chain and, when delivery
// returns to it, runs the closure and reports to the owner.
func TestInceptionPipelineNamesTheForemanAndTheDeliveryHandOff(t *testing.T) {
	repoRoot := actualsInstrumentationRepoRoot(t)
	skill := readRepoFile(t, repoRoot, inceptionPipelineSkillRelPath)
	section := markdownSectionBody(t, skill, "## Chain entry")
	const want = "delivery adds its final Log entry in place and returns to the parent, the `foreman`, not the owner;"
	if !strings.Contains(section, want) {
		t.Errorf("the Chain entry section must carry %q, got:\n%s", want, section)
	}
}
