package propagator_test

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator"
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
// (description must match exactly what BuildScopedRow produces from contractFrontmatter).
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

// A registry that has a STALE marker block (wrong phases) — exercises replaceBlock.
const registryStaleScopedBlock = `# Skill Registry — test-project

## Skills Index

### Shared Contracts

| Artifact | Path | Description |
|----------|------|-------------|
| pre-sdd-contracts | skills/_shared/pre-sdd-contracts.md | Shared contracts |
<!-- BEGIN: minimalism-contract-scope (auto-generated) -->
| minimalism-contract | skills/_shared/minimalism-contract.md | Inject ONLY into sdd-spec and sdd-design sub-agent prompts under '## Skills to load before work'. Do NOT inject into sdd-tasks/sdd-apply. |
<!-- END: minimalism-contract-scope -->
`

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

// TC-F: description is derived dynamically from frontmatter phases, not
// hardcoded. Assert that the DYNAMIC phases appear AND that the hardcoded
// production phases (sdd-tasks/sdd-apply) are ABSENT when a different
// frontmatter is used. This makes the test non-tautological.
func TestScopedDescriptionDerivedFromFrontmatter(t *testing.T) {
	// Use a completely different set of phases to verify it's not hardcoded.
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

	// Dynamic phases MUST appear in the inject description.
	if !strings.Contains(out, "sdd-spec") {
		t.Error("description should contain sdd-spec when derived from frontmatter")
	}
	if !strings.Contains(out, "sdd-design") {
		t.Error("description should contain sdd-design when derived from frontmatter")
	}

	// Production phases must NOT appear in the inject (applies_to) part of
	// the description when they were excluded. They may appear in the
	// "Do NOT inject into" clause, but not as injection targets.
	// Parse only the row line to check the "Inject ONLY into" portion.
	var injectLine string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Inject ONLY into") {
			injectLine = line
			break
		}
	}
	if injectLine == "" {
		t.Fatal("could not find the 'Inject ONLY into' line in output")
	}
	// Extract just the "Inject ONLY into X" portion before "Do NOT inject".
	injectPart := injectLine
	if idx := strings.Index(injectLine, " Do NOT inject into"); idx != -1 {
		injectPart = injectLine[:idx]
	}
	if strings.Contains(injectPart, "sdd-tasks") {
		t.Errorf("inject target part should not contain 'sdd-tasks' when not in applies_to_phases; inject part: %q", injectPart)
	}
	if strings.Contains(injectPart, "sdd-apply") {
		t.Errorf("inject target part should not contain 'sdd-apply' when not in applies_to_phases; inject part: %q", injectPart)
	}
}

// TC-G: replaceBlock coverage — stale marker block (wrong phases) is corrected.
// This directly exercises the replaceBlock path (Case 2 in Propagate).
func TestReplaceStaleBlock(t *testing.T) {
	cfg := propagator.Config{
		ContractPath: "skills/_shared/minimalism-contract.md",
	}
	phases, err := propagator.ParseFrontmatter(contractFrontmatter)
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}

	out, changed, err := propagator.Propagate(registryStaleScopedBlock, cfg, phases)
	if err != nil {
		t.Fatalf("Propagate: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true when block has stale content")
	}
	// The stale phases should be gone from the injection description.
	if strings.Contains(out, "Inject ONLY into sdd-spec") {
		t.Error("stale 'Inject ONLY into sdd-spec' text still present after correction")
	}
	// The correct phases must now be present.
	if !strings.Contains(out, "sdd-tasks") {
		t.Error("corrected block should reference sdd-tasks")
	}
	if !strings.Contains(out, "sdd-apply") {
		t.Error("corrected block should reference sdd-apply")
	}
	// Exactly one BEGIN marker.
	if strings.Count(out, propagator.BeginMarker) != 1 {
		t.Errorf("expected exactly 1 BEGIN marker, got %d", strings.Count(out, propagator.BeginMarker))
	}
	// Exactly one END marker.
	if strings.Count(out, propagator.EndMarker) != 1 {
		t.Errorf("expected exactly 1 END marker, got %d", strings.Count(out, propagator.EndMarker))
	}
}

// TC-H: FOREIGN-SURVIVAL — a different writer's marker block is already present.
// Running Propagate must leave the foreign block UNTOUCHED and the minimalism
// block must be correctly inserted alongside it.
func TestForeignBlockSurvival(t *testing.T) {
	const foreignBlock = `<!-- BEGIN: project-manifest-rules (auto-generated) -->
| project-manifest | skills/project-manifest/SKILL.md | Core project rules — always inject. |
<!-- END: project-manifest-rules -->`

	registryWithForeign := `# Skill Registry — test-project

## Skills Index

### Shared Contracts

| Artifact | Path | Description |
|----------|------|-------------|
| pre-sdd-contracts | skills/_shared/pre-sdd-contracts.md | Shared contracts |
` + foreignBlock + `
`

	cfg := propagator.Config{
		ContractPath: "skills/_shared/minimalism-contract.md",
	}
	phases, err := propagator.ParseFrontmatter(contractFrontmatter)
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}

	out, changed, err := propagator.Propagate(registryWithForeign, cfg, phases)
	if err != nil {
		t.Fatalf("Propagate: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true: minimalism block was not yet present")
	}

	// The foreign block must survive verbatim.
	if !strings.Contains(out, foreignBlock) {
		t.Error("foreign marker block was removed or modified — foreign blocks must survive")
	}

	// The minimalism block must be present.
	if !strings.Contains(out, propagator.BeginMarker) {
		t.Error("minimalism BEGIN marker missing after Propagate with foreign block present")
	}
	if !strings.Contains(out, propagator.EndMarker) {
		t.Error("minimalism END marker missing after Propagate with foreign block present")
	}
	if !strings.Contains(out, "sdd-tasks") {
		t.Error("minimalism block should reference sdd-tasks")
	}

	// Both blocks must coexist: the foreign BEGIN/END and the minimalism BEGIN/END.
	if strings.Count(out, "BEGIN:") != 2 {
		t.Errorf("expected 2 BEGIN: markers (foreign + minimalism), got %d",
			strings.Count(out, "BEGIN:"))
	}
}

// TC-I: appendToSharedContracts fallback — registry has NO '### Shared Contracts'
// section at all. The block must still be appended (at the end of the file).
// This exercises the lastTableRowIdx == -1 fallback branch in appendToSharedContracts.
func TestAppendFallback_NoSharedContractsSection(t *testing.T) {
	// A registry that has NO Shared Contracts section.
	registryNoSection := `# Skill Registry — test-project

## Skills Index

| Artifact | Path | Description |
|----------|------|-------------|
| some-skill | skills/some/SKILL.md | Does something. |
`
	cfg := propagator.Config{
		ContractPath: "skills/_shared/minimalism-contract.md",
	}
	phases, err := propagator.ParseFrontmatter(contractFrontmatter)
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}

	out, changed, err := propagator.Propagate(registryNoSection, cfg, phases)
	if err != nil {
		t.Fatalf("Propagate: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true when block is missing")
	}
	// The block must appear somewhere in the output.
	if !strings.Contains(out, propagator.BeginMarker) {
		t.Error("BEGIN marker missing when no Shared Contracts section present")
	}
	if !strings.Contains(out, propagator.EndMarker) {
		t.Error("END marker missing when no Shared Contracts section present")
	}
	if !strings.Contains(out, "sdd-tasks") {
		t.Error("scoped row must reference sdd-tasks")
	}
	// The original content must be preserved.
	if !strings.Contains(out, "some-skill") {
		t.Error("original registry content must be preserved in output")
	}
}

// TC-J: replaceUnscopedRow inBlock guard — a minimalism-contract table row that
// sits INSIDE a foreign marker block must NOT be wrongly replaced. Only rows
// outside any marker block should be replaced.
func TestReplaceUnscopedRow_ForeignBlockProtectsRow(t *testing.T) {
	// A registry where the minimalism-contract row is INSIDE a foreign block.
	// This row must not be replaced by replaceUnscopedRow.
	registryWithContractInsideForeign := `# Skill Registry — test-project

## Skills Index

### Shared Contracts

| Artifact | Path | Description |
|----------|------|-------------|
| pre-sdd-contracts | skills/_shared/pre-sdd-contracts.md | Shared contracts |
<!-- BEGIN: some-other-tool (auto-generated) -->
| minimalism-contract | skills/_shared/minimalism-contract.md | Unscoped row inside foreign block. |
<!-- END: some-other-tool -->
`
	cfg := propagator.Config{
		ContractPath: "skills/_shared/minimalism-contract.md",
	}
	phases, err := propagator.ParseFrontmatter(contractFrontmatter)
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}

	out, changed, err := propagator.Propagate(registryWithContractInsideForeign, cfg, phases)
	if err != nil {
		t.Fatalf("Propagate: %v", err)
	}

	// Propagate must still add the minimalism-contract block (the row inside the
	// foreign block is protected and does not count as the "real" scoped row).
	if !changed {
		t.Fatal("expected changed=true: the row inside the foreign block is not the scoped block")
	}
	// The minimalism BEGIN/END marker must now be present.
	if !strings.Contains(out, propagator.BeginMarker) {
		t.Error("minimalism BEGIN marker missing after Propagate on registry with foreign-protected row")
	}
	// The foreign block must be preserved verbatim.
	if !strings.Contains(out, "<!-- BEGIN: some-other-tool (auto-generated) -->") {
		t.Error("foreign block BEGIN marker was removed or modified")
	}
	if !strings.Contains(out, "<!-- END: some-other-tool -->") {
		t.Error("foreign block END marker was removed or modified")
	}
	// The minimalism-contract row inside the foreign block must still be there.
	if !strings.Contains(out, "Unscoped row inside foreign block.") {
		t.Error("minimalism-contract row inside foreign block was wrongly removed or modified")
	}
}
