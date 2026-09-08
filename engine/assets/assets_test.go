package assets_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/assets"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator"
)

// repoRoot returns the overlay repo root, two levels above engine/assets/.
func repoRoot(t *testing.T) string {
	t.Helper()
	// engine/assets/assets_test.go → engine/ → repo root
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Join(wd, "..", "..")
}

// TestAntiGenericDesignFrontmatterParses asserts the embedded
// anti-generic-design asset carries valid, parseable frontmatter scoping it to
// exactly the sdd-tasks and sdd-apply phases (R-104). A parse error here means
// gate-task/propagate would fail loud (propagate exits 1; gate-task logs a
// stderr warning and passes through) instead of injecting the contract.
func TestAntiGenericDesignFrontmatterParses(t *testing.T) {
	phases, err := propagator.ParseFrontmatter(assets.AntiGenericDesign)
	if err != nil {
		t.Fatalf("ParseFrontmatter(assets.AntiGenericDesign): unexpected error: %v", err)
	}

	want := []string{"sdd-tasks", "sdd-apply"}
	if len(phases.AppliesTo) != len(want) {
		t.Fatalf("AppliesTo = %v, want %v", phases.AppliesTo, want)
	}
	for i, phase := range want {
		if phases.AppliesTo[i] != phase {
			t.Errorf("AppliesTo[%d] = %q, want %q", i, phases.AppliesTo[i], phase)
		}
	}
}

// TestAntiGenericDesignStandaloneCopyMatchesEmbeddedAsset is the Phase 5
// (drift guard) test for the two-copy design risk documented in design.md:
// the deployed standalone file skills/_shared/anti-generic-design.md MUST
// stay byte-identical to the compiled-in engine/assets/anti-generic-design.md
// asset. Any drift here means gate-task/propagate (embedded) and a user
// invoking the standalone skill by name would see DIFFERENT guidance — this
// guard fails loud instead of letting that happen silently.
func TestAntiGenericDesignStandaloneCopyMatchesEmbeddedAsset(t *testing.T) {
	standalonePath := filepath.Join(repoRoot(t), "skills", "_shared", "anti-generic-design.md")

	standaloneBytes, err := os.ReadFile(standalonePath)
	if err != nil {
		t.Fatalf("read standalone copy %s: %v", standalonePath, err)
	}

	if string(standaloneBytes) != assets.AntiGenericDesign {
		t.Errorf(
			"skills/_shared/anti-generic-design.md has drifted from the embedded engine/assets/anti-generic-design.md asset — they must be byte-identical.\nstandalone length=%d, embedded length=%d",
			len(standaloneBytes), len(assets.AntiGenericDesign),
		)
	}
}

// antiGenericDesignPaths are the three documents that restate the forbidden-pattern
// guidance, each for a different consumer: the injected contract, the invokable
// skill, and the reviewer checklist.
const (
	sharedContractRel   = "skills/_shared/anti-generic-design.md"
	standaloneSkillRel  = "skills/anti-generic-design/SKILL.md"
	critiqueTemplateRel = "skills/anti-generic-design/references/critique-template.md"
)

// readRepoFile reads a repo-relative, slash-separated path.
func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	p := filepath.Join(repoRoot(t), filepath.FromSlash(rel))
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

// markdownSection returns the body of the '## <heading>' section of doc, stopping
// at the next '## ' heading, with all runs of whitespace collapsed to single
// spaces. Collapsing makes the comparison indifferent to line wrapping, which
// legitimately differs between the two copies, while staying sensitive to every
// word.
func markdownSection(doc, heading string) string {
	marker := "## " + heading
	idx := strings.Index(doc, marker)
	if idx < 0 {
		return ""
	}
	body := doc[idx+len(marker):]
	if end := strings.Index(body, "\n## "); end >= 0 {
		body = body[:end]
	}
	return strings.Join(strings.Fields(body), " ")
}

// TestAntiGenericDesignForbiddenPatternsAgreeAcrossCopies closes the drift gap
// that TestAntiGenericDesignStandaloneCopyMatchesEmbeddedAsset leaves open. That
// test guards the embedded asset against its deployed _shared copy; nothing
// guarded either against skills/anti-generic-design/SKILL.md, which now also
// deploys to every runtime. Both documents are consumed as authoritative — the
// contract is injected into CLAUDE.md, the skill is invoked by name — so editing
// the forbidden-pattern list in one and not the other would hand two different
// answers to the same question with no failing test.
//
// Only the forbidden-pattern section is compared. The 'Steer toward' sections and
// the self-critique checklists differ on purpose: SKILL.md steers per dimension
// via references/palette-typography.md and carries two extra checklist lines that
// the distilled contract deliberately omits.
func TestAntiGenericDesignForbiddenPatternsAgreeAcrossCopies(t *testing.T) {
	const heading = "Forbidden patterns (the gap)"

	shared := markdownSection(readRepoFile(t, sharedContractRel), heading)
	standalone := markdownSection(readRepoFile(t, standaloneSkillRel), heading)

	// Guard against a vacuous pass: if the heading is ever renamed, both sides
	// would extract "" and compare equal while checking nothing.
	if shared == "" {
		t.Fatalf("%s: section %q not found — the heading moved and this guard stopped guarding", sharedContractRel, heading)
	}
	if standalone == "" {
		t.Fatalf("%s: section %q not found — the heading moved and this guard stopped guarding", standaloneSkillRel, heading)
	}

	if shared != standalone {
		t.Errorf(
			"the %q section has drifted between the two deployed copies.\n%s (%d chars):\n%s\n\n%s (%d chars):\n%s",
			heading,
			sharedContractRel, len(shared), shared,
			standaloneSkillRel, len(standalone), standalone,
		)
	}
}

// TestAntiGenericDesignAntiMimicryLinePresent pins the obligation stated in
// openspec/specs/anti-generic-design/spec.md: the critique-template checklist and
// the SKILL.md inline checklist must each carry the anti-mimicry line. It is the
// one checklist entry that catches copying a single anchor's whole kit, so losing
// it from either document silently weakens the review.
//
// The distilled _shared contract is intentionally excluded: it carries the
// shorter v1 checklist and never contained this line.
func TestAntiGenericDesignAntiMimicryLinePresent(t *testing.T) {
	const antiMimicry = "token set is NOT >80% traceable to a single cited anchor"

	for _, rel := range []string{standaloneSkillRel, critiqueTemplateRel} {
		if !strings.Contains(readRepoFile(t, rel), antiMimicry) {
			t.Errorf("%s is missing the anti-mimicry checklist line %q", rel, antiMimicry)
		}
	}
}
