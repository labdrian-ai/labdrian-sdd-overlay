package propagator_test

import (
	"strings"
	"testing"

	propagator "github.com/labdrian-ai/labdrian-sdd-overlay/tools/registry-propagator"
)

// ---- helpers ---------------------------------------------------------------

const contractFrontmatter = `---
applies_to_phases: [sdd-tasks, sdd-apply]
excluded_phases: [sdd-propose, sdd-spec, sdd-design, sdd-verify, sdd-archive]
injection_point: "## Skills to load before work"
---
# Minimalism Contract
`

const contractFrontmatterMissingPhases = `---
injection_point: "## Skills to load before work"
---
# Minimalism Contract
`

// A minimal registry that is entirely missing the contract row and the marker block.
const registryMissingRow = `# Skill Registry — test-project

## Skills Index

### Shared Contracts

| Artifact | Path | Description |
|----------|------|-------------|
| pre-sdd-contracts | skills/_shared/pre-sdd-contracts.md | Shared contracts |
`

// A registry that has the contract row but WITHOUT scope (unscoped — wrong).
const registryUnscopedRow = `# Skill Registry — test-project

## Skills Index

### Shared Contracts

| Artifact | Path | Description |
|----------|------|-------------|
| pre-sdd-contracts | skills/_shared/pre-sdd-contracts.md | Shared contracts |
| minimalism-contract | skills/_shared/minimalism-contract.md | Response-length and minimalism rules. |
`

// A registry that has the contract row with the correct scoped description
// (description must match exactly what buildScopedRow produces from contractFrontmatter).
const registryAlreadyScoped = `# Skill Registry — test-project

## Skills Index

### Shared Contracts

| Artifact | Path | Description |
|----------|------|-------------|
| pre-sdd-contracts | skills/_shared/pre-sdd-contracts.md | Shared contracts |
<!-- BEGIN: minimalism-contract-scope (auto-generated) -->
| minimalism-contract | skills/_shared/minimalism-contract.md | Inject ONLY into sdd-tasks and sdd-apply sub-agent prompts under '## Skills to load before work'. Do NOT inject into sdd-propose/sdd-spec/sdd-design/sdd-verify/sdd-archive. |
<!-- END: minimalism-contract-scope -->
`

// A registry that has the contract row unscoped AND the marker block (simulates
// regeneration that wrote the block but the table row outside was also added
// by someone manually — the regeneration-safety test).
const registryWithMarkerBlock = registryAlreadyScoped

// ---- tests -----------------------------------------------------------------

// TC-A: registry MISSING the contract row → inserts a SCOPED row.
func TestInsertsMissingRow(t *testing.T) {
	cfg := propagator.Config{
		ContractPath: "skills/_shared/minimalism-contract.md",
	}
	phases, err := propagator.ParseFrontmatter(contractFrontmatter)
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}

	out, changed, err := propagator.Propagate(registryMissingRow, cfg, phases)
	if err != nil {
		t.Fatalf("Propagate: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true when row is missing")
	}
	if !strings.Contains(out, "<!-- BEGIN: minimalism-contract-scope (auto-generated) -->") {
		t.Error("output missing BEGIN marker")
	}
	if !strings.Contains(out, "<!-- END: minimalism-contract-scope -->") {
		t.Error("output missing END marker")
	}
	if !strings.Contains(out, "sdd-tasks") {
		t.Error("output missing sdd-tasks in scoped description")
	}
	if !strings.Contains(out, "sdd-apply") {
		t.Error("output missing sdd-apply in scoped description")
	}
}

// TC-B: registry has the row UNSCOPED → corrects it to scoped.
func TestCorrectsUnscopedRow(t *testing.T) {
	cfg := propagator.Config{
		ContractPath: "skills/_shared/minimalism-contract.md",
	}
	phases, err := propagator.ParseFrontmatter(contractFrontmatter)
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}

	out, changed, err := propagator.Propagate(registryUnscopedRow, cfg, phases)
	if err != nil {
		t.Fatalf("Propagate: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true when row is unscoped")
	}
	// The old unscoped row must be replaced (not duplicated).
	if strings.Count(out, "minimalism-contract") > 4 {
		t.Errorf("unscoped row was duplicated instead of replaced; occurrences: %d",
			strings.Count(out, "minimalism-contract"))
	}
	if !strings.Contains(out, "<!-- BEGIN: minimalism-contract-scope (auto-generated) -->") {
		t.Error("output missing BEGIN marker after correction")
	}
	// The plain unscoped row must not remain outside the marker block.
	lines := strings.Split(out, "\n")
	inBlock := false
	for _, line := range lines {
		if strings.Contains(line, "<!-- BEGIN: minimalism-contract-scope") {
			inBlock = true
		}
		if strings.Contains(line, "<!-- END: minimalism-contract-scope -->") {
			inBlock = false
		}
		if !inBlock && strings.Contains(line, "minimalism-contract") &&
			strings.HasPrefix(strings.TrimSpace(line), "|") &&
			!strings.Contains(line, "BEGIN") && !strings.Contains(line, "END") &&
			!strings.Contains(line, "<!-- ") {
			// Check it's inside a marker block or is a marker line
			t.Errorf("unscoped row still present outside marker block: %q", line)
		}
	}
}

// TC-C: registry already has the correct SCOPED row → no-op (IDEMPOTENT).
func TestIdempotentWhenAlreadyScoped(t *testing.T) {
	cfg := propagator.Config{
		ContractPath: "skills/_shared/minimalism-contract.md",
	}
	phases, err := propagator.ParseFrontmatter(contractFrontmatter)
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}

	out, changed, err := propagator.Propagate(registryAlreadyScoped, cfg, phases)
	if err != nil {
		t.Fatalf("Propagate: %v", err)
	}
	if changed {
		t.Fatal("expected changed=false when registry is already correct")
	}
	if out != registryAlreadyScoped {
		t.Errorf("output differs from input on no-op run:\ngot:\n%s\nwant:\n%s", out, registryAlreadyScoped)
	}
}

// TC-D: contract frontmatter missing applies_to_phases → fails LOUDLY.
func TestFailsLoudlyOnMissingFrontmatter(t *testing.T) {
	_, err := propagator.ParseFrontmatter(contractFrontmatterMissingPhases)
	if err == nil {
		t.Fatal("expected error when applies_to_phases is missing, got nil")
	}
	if !strings.Contains(err.Error(), "applies_to_phases") {
		t.Errorf("error should mention 'applies_to_phases', got: %v", err)
	}
}

// TC-E: regeneration-safety — after a simulated regeneration (marker block
// already present with correct content), Propagate is a no-op and the marker
// block content is preserved verbatim.
func TestRegenerationSafety(t *testing.T) {
	cfg := propagator.Config{
		ContractPath: "skills/_shared/minimalism-contract.md",
	}
	phases, err := propagator.ParseFrontmatter(contractFrontmatter)
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}

	// First pass: insert.
	out1, changed1, err := propagator.Propagate(registryMissingRow, cfg, phases)
	if err != nil {
		t.Fatalf("first Propagate: %v", err)
	}
	if !changed1 {
		t.Fatal("first pass should change registry")
	}

	// Simulate regeneration: Propagate again on the output.
	out2, changed2, err := propagator.Propagate(out1, cfg, phases)
	if err != nil {
		t.Fatalf("second Propagate: %v", err)
	}
	if changed2 {
		t.Fatal("second pass (after regeneration) should be a no-op")
	}
	if out2 != out1 {
		t.Errorf("registry content changed after simulated regeneration:\nfirst pass:\n%s\nsecond pass:\n%s",
			out1, out2)
	}
	// Verify the scoped row is still present.
	if !strings.Contains(out2, "<!-- BEGIN: minimalism-contract-scope (auto-generated) -->") {
		t.Error("scoped row missing after simulated regeneration")
	}
}

// TC-F (bonus): description is derived from frontmatter phases, not hardcoded.
func TestScopedDescriptionDerivedFromFrontmatter(t *testing.T) {
	// Use a different set of phases to verify it's not hardcoded.
	altFrontmatter := `---
applies_to_phases: [sdd-spec, sdd-design]
excluded_phases: [sdd-tasks, sdd-apply]
injection_point: "## Skills to load before work"
---
# Alt Contract
`
	cfg := propagator.Config{
		ContractPath: "skills/_shared/minimalism-contract.md",
	}
	phases, err := propagator.ParseFrontmatter(altFrontmatter)
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}

	out, _, err := propagator.Propagate(registryMissingRow, cfg, phases)
	if err != nil {
		t.Fatalf("Propagate: %v", err)
	}
	if !strings.Contains(out, "sdd-spec") {
		t.Error("description should contain sdd-spec when derived from frontmatter")
	}
	if !strings.Contains(out, "sdd-design") {
		t.Error("description should contain sdd-design when derived from frontmatter")
	}
	if strings.Contains(out, "sdd-tasks") && !strings.Contains(out, "sdd-spec") {
		t.Error("description should not hardcode sdd-tasks")
	}
}
