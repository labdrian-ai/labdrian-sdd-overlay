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

// TC-K: replaceBlock guard — malformed marker blocks (lone BEGIN with no END,
// and BEGIN/END out of order) must leave the registry unchanged.
// Exercises the start==-1 || end==-1 || end<start guard in replaceBlock.
func TestReplaceBlock_MalformedMarkers(t *testing.T) {
	cfg := propagator.Config{
		ContractPath: "skills/_shared/minimalism-contract.md",
	}
	phases, err := propagator.ParseFrontmatter(contractFrontmatter)
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}

	t.Run("lone BEGIN no END", func(t *testing.T) {
		// Registry has the BEGIN marker but no END marker.
		// replaceBlock must return the registry unchanged.
		registryLoneBegin := `# Skill Registry
### Shared Contracts
| Artifact | Path | Description |
|----------|------|-------------|
` + propagator.BeginMarker + `
| minimalism-contract | skills/_shared/minimalism-contract.md | stale row |
`
		out, changed, err := propagator.Propagate(registryLoneBegin, cfg, phases)
		if err != nil {
			t.Fatalf("Propagate: %v", err)
		}
		// replaceBlock finds BEGIN but no END → returns registry unchanged → changed=false.
		if changed {
			t.Errorf("lone BEGIN with no END: expected changed=false (replaceBlock guard), got changed=true; out:\n%s", out)
		}
		if out != registryLoneBegin {
			t.Errorf("lone BEGIN with no END: registry content should be unchanged;\ngot:\n%s\nwant:\n%s", out, registryLoneBegin)
		}
	})

	t.Run("BEGIN and END out of order", func(t *testing.T) {
		// Registry has END before BEGIN — out of order.
		// replaceBlock must return the registry unchanged.
		registryOutOfOrder := `# Skill Registry
### Shared Contracts
| Artifact | Path | Description |
|----------|------|-------------|
` + propagator.EndMarker + `
| minimalism-contract | skills/_shared/minimalism-contract.md | stale row |
` + propagator.BeginMarker + `
`
		out, changed, err := propagator.Propagate(registryOutOfOrder, cfg, phases)
		if err != nil {
			t.Fatalf("Propagate: %v", err)
		}
		// replaceBlock finds end < start → returns registry unchanged → changed=false.
		if changed {
			t.Errorf("BEGIN/END out of order: expected changed=false (replaceBlock guard), got changed=true; out:\n%s", out)
		}
		if out != registryOutOfOrder {
			t.Errorf("BEGIN/END out of order: registry content should be unchanged;\ngot:\n%s\nwant:\n%s", out, registryOutOfOrder)
		}
	})
}

// TC-L: replaceUnscopedRow inAnyBlock branches — an UNSCOPED minimalism-contract
// row OUTSIDE any block PLUS a foreign BEGIN/END marker block in the same registry.
// The unscoped row (outside any block) must be corrected; the foreign block untouched.
// Exercises both the block-entry (<!-- BEGIN:) and block-exit/END (<!-- END:) branches.
func TestReplaceUnscopedRow_InAnyBlockBranches(t *testing.T) {
	// Registry contains:
	// 1. A foreign BEGIN/END marker block (must survive untouched).
	// 2. An UNSCOPED minimalism-contract row OUTSIDE any block (must be corrected).
	const foreignBlockContent = `<!-- BEGIN: foreign-tool (auto-generated) -->
| foreign-skill | skills/foreign/SKILL.md | Foreign skill description. |
<!-- END: foreign-tool -->`

	registryWithBothBlocks := `# Skill Registry
### Shared Contracts
| Artifact | Path | Description |
|----------|------|-------------|
| pre-sdd-contracts | skills/_shared/pre-sdd-contracts.md | Shared contracts |
` + foreignBlockContent + `
| minimalism-contract | skills/_shared/minimalism-contract.md | Response-length and minimalism rules. |
`

	cfg := propagator.Config{
		ContractPath: "skills/_shared/minimalism-contract.md",
	}
	phases, err := propagator.ParseFrontmatter(contractFrontmatter)
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}

	out, changed, err := propagator.Propagate(registryWithBothBlocks, cfg, phases)
	if err != nil {
		t.Fatalf("Propagate: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true: unscoped row outside any block must be corrected")
	}

	// The unscoped row must be replaced with the scoped BEGIN/END block.
	if !strings.Contains(out, propagator.BeginMarker) {
		t.Error("minimalism BEGIN marker must be present after correcting unscoped row")
	}
	if !strings.Contains(out, propagator.EndMarker) {
		t.Error("minimalism END marker must be present after correcting unscoped row")
	}
	if !strings.Contains(out, "sdd-tasks") {
		t.Error("corrected block must reference sdd-tasks in description")
	}

	// The foreign block must survive verbatim (block-entry and block-exit branches
	// protect rows inside any marker block from replacement).
	if !strings.Contains(out, foreignBlockContent) {
		t.Error("foreign marker block was removed or modified — must survive untouched")
	}
	if !strings.Contains(out, "foreign-skill") {
		t.Error("foreign-skill row inside the foreign block must not be touched")
	}

	// No unscoped minimalism-contract row should remain outside the marker block.
	lines := strings.Split(out, "\n")
	inBlock := false
	for _, line := range lines {
		if strings.Contains(line, "<!-- BEGIN:") {
			inBlock = true
		}
		if strings.Contains(line, "<!-- END:") {
			inBlock = false
		}
		trimmed := strings.TrimSpace(line)
		if !inBlock && strings.HasPrefix(trimmed, "|") &&
			strings.Contains(trimmed, "minimalism-contract") &&
			!strings.Contains(trimmed, "BEGIN") && !strings.Contains(trimmed, "END") &&
			!strings.Contains(trimmed, "<!--") &&
			!strings.Contains(trimmed, "Inject ONLY into") { // scoped row is OK
			t.Errorf("unscoped minimalism-contract row still present outside any block: %q", line)
		}
	}
}

// TC-M: ParseFrontmatter missing-delimiter — input with no YAML frontmatter
// (len(parts) < 3) must return an error with the expected struct shape.
func TestParseFrontmatter_MissingDelimiter(t *testing.T) {
	inputs := []string{
		"no frontmatter at all",
		"just some text without any dashes",
		"",
		"---\nonly one delimiter\n",
	}

	for _, input := range inputs {
		phases, err := propagator.ParseFrontmatter(input)
		if err == nil {
			t.Errorf("ParseFrontmatter(%q): expected error for missing frontmatter delimiters, got nil", input)
			continue
		}
		// Error message should be informative.
		if !strings.Contains(err.Error(), "frontmatter") && !strings.Contains(err.Error(), "---") &&
			!strings.Contains(err.Error(), "applies_to_phases") {
			t.Errorf("ParseFrontmatter(%q): error message should mention frontmatter/delimiters/applies_to_phases; got: %v", input, err)
		}
		// The returned struct must be the zero value (no partial data).
		if len(phases.AppliesTo) != 0 {
			t.Errorf("ParseFrontmatter(%q): AppliesTo should be empty on error, got: %v", input, phases.AppliesTo)
		}
		if len(phases.Excluded) != 0 {
			t.Errorf("ParseFrontmatter(%q): Excluded should be empty on error, got: %v", input, phases.Excluded)
		}
	}
}

// TC-N: appendToSharedContracts break-on-next-heading — a registry whose
// '### Shared Contracts' section is followed by another heading before EOF.
// The block must be inserted at the correct position (inside the Shared
// Contracts table, NOT after the next heading).
func TestAppendToSharedContracts_NextHeadingBreak(t *testing.T) {
	// Registry whose Shared Contracts section has a table row and is followed
	// by another '## Other Section' heading before EOF.
	const registryWithNextHeading = `# Skill Registry

## Skills Index

### Shared Contracts

| Artifact | Path | Description |
|----------|------|-------------|
| pre-sdd-contracts | skills/_shared/pre-sdd-contracts.md | Shared contracts |

## Other Section

| some-other | path | desc |
`

	cfg := propagator.Config{
		ContractPath: "skills/_shared/minimalism-contract.md",
	}
	phases, err := propagator.ParseFrontmatter(contractFrontmatter)
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}

	out, changed, err := propagator.Propagate(registryWithNextHeading, cfg, phases)
	if err != nil {
		t.Fatalf("Propagate: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true: minimalism block was missing")
	}

	// The minimalism block must be present.
	if !strings.Contains(out, propagator.BeginMarker) {
		t.Error("BEGIN marker missing from output")
	}

	// The block must be inserted BEFORE '## Other Section' (inside Shared Contracts),
	// not after it. Verify correct insert position.
	beginIdx := strings.Index(out, propagator.BeginMarker)
	otherSectionIdx := strings.Index(out, "## Other Section")
	if otherSectionIdx != -1 && beginIdx > otherSectionIdx {
		t.Errorf("minimalism block was inserted AFTER '## Other Section' — wrong position;\nbeginIdx=%d, otherSectionIdx=%d\nout:\n%s",
			beginIdx, otherSectionIdx, out)
	}

	// The '## Other Section' content must survive unchanged.
	if !strings.Contains(out, "## Other Section") {
		t.Error("'## Other Section' heading was removed or modified")
	}
	if !strings.Contains(out, "some-other") {
		t.Error("content under '## Other Section' was removed or modified")
	}
}

// TestAntiGenericDesignMarkersAreDistinct: the third managed contract
// (anti-generic-design) MUST own its own BEGIN/END marker pair, distinct from
// both the minimalism-contract defaults and the skill-discovery-safety pair.
// Reusing any existing marker pair would make Propagate overwrite a foreign
// contract's block instead of coexisting with it (see package doc).
func TestAntiGenericDesignMarkersAreDistinct(t *testing.T) {
	if propagator.AntiGenericDesignBeginMarker == "" {
		t.Fatal("AntiGenericDesignBeginMarker must not be empty")
	}
	if propagator.AntiGenericDesignEndMarker == "" {
		t.Fatal("AntiGenericDesignEndMarker must not be empty")
	}

	pairs := map[string]string{
		"minimalism-contract (Begin)":    propagator.BeginMarker,
		"minimalism-contract (End)":      propagator.EndMarker,
		"skill-discovery-safety (Begin)": propagator.DiscoverySafetyBeginMarker,
		"skill-discovery-safety (End)":   propagator.DiscoverySafetyEndMarker,
	}

	for label, marker := range pairs {
		if propagator.AntiGenericDesignBeginMarker == marker {
			t.Errorf("AntiGenericDesignBeginMarker collides with %s marker %q", label, marker)
		}
		if propagator.AntiGenericDesignEndMarker == marker {
			t.Errorf("AntiGenericDesignEndMarker collides with %s marker %q", label, marker)
		}
	}

	if propagator.AntiGenericDesignBeginMarker == propagator.AntiGenericDesignEndMarker {
		t.Error("AntiGenericDesignBeginMarker and AntiGenericDesignEndMarker must not be equal")
	}
}

// TestAntiGenericDesignPropagate_ThreeBlockIsolationAndIdempotency is the
// Phase 5 (R-101) isolation/lock test: propagating anti-generic-design into a
// registry that ALREADY contains correctly-scoped minimalism-contract and
// skill-discovery-safety blocks must leave both pre-existing blocks
// byte-identical, add its own third block, and be idempotent on re-run.
func TestAntiGenericDesignPropagate_ThreeBlockIsolationAndIdempotency(t *testing.T) {
	const minimalismBlock = `<!-- BEGIN: minimalism-contract-scope (auto-generated) -->
| minimalism-contract | skills/_shared/minimalism-contract.md | Inject ONLY into sdd-tasks and sdd-apply sub-agent prompts under '## Skills to load before work'. Do NOT inject into sdd-propose/sdd-spec/sdd-design/sdd-verify/sdd-archive. |
<!-- END: minimalism-contract-scope -->`

	const safetyBlock = `<!-- BEGIN: skill-discovery-safety-scope (auto-generated) -->
| skill-discovery-safety | skills/_shared/skill-discovery-safety.md | Inject ONLY into sdd-tasks and sdd-apply sub-agent prompts under '## Skills to load before work'. Do NOT inject into sdd-propose/sdd-spec/sdd-design/sdd-verify/sdd-archive. |
<!-- END: skill-discovery-safety-scope -->`

	registryWithTwoBlocks := `# Skill Registry — test-project

## Skills Index

### Shared Contracts

| Artifact | Path | Description |
|----------|------|-------------|
| pre-sdd-contracts | skills/_shared/pre-sdd-contracts.md | Shared contracts |
` + minimalismBlock + `
` + safetyBlock + `
`

	designCfg := propagator.Config{
		ContractPath: "skills/_shared/anti-generic-design.md",
		BeginMarker:  propagator.AntiGenericDesignBeginMarker,
		EndMarker:    propagator.AntiGenericDesignEndMarker,
		RowLabel:     "anti-generic-design",
	}
	phases, err := propagator.ParseFrontmatter(contractFrontmatter)
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}

	firstOut, changed, err := propagator.Propagate(registryWithTwoBlocks, designCfg, phases)
	if err != nil {
		t.Fatalf("Propagate (first run): %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true: anti-generic-design block was not yet present")
	}

	// The two pre-existing blocks must survive BYTE-IDENTICAL.
	if !strings.Contains(firstOut, minimalismBlock) {
		t.Error("minimalism-contract block was not left byte-identical after propagating anti-generic-design")
	}
	if !strings.Contains(firstOut, safetyBlock) {
		t.Error("skill-discovery-safety block was not left byte-identical after propagating anti-generic-design")
	}

	// The new anti-generic-design block must be present with its own markers.
	if !strings.Contains(firstOut, propagator.AntiGenericDesignBeginMarker) {
		t.Error("anti-generic-design BEGIN marker missing after Propagate")
	}
	if !strings.Contains(firstOut, propagator.AntiGenericDesignEndMarker) {
		t.Error("anti-generic-design END marker missing after Propagate")
	}

	// All three BEGIN markers must coexist — no block overwrote another.
	if n := strings.Count(firstOut, "BEGIN:"); n != 3 {
		t.Errorf("expected 3 BEGIN: markers (minimalism + safety + anti-generic-design), got %d:\n%s", n, firstOut)
	}

	// Idempotency: re-propagating the same design block on its own output is a no-op.
	secondOut, changedAgain, err := propagator.Propagate(firstOut, designCfg, phases)
	if err != nil {
		t.Fatalf("Propagate (second run): %v", err)
	}
	if changedAgain {
		t.Error("expected changed=false on re-propagate: anti-generic-design block is already correctly scoped")
	}
	if secondOut != firstOut {
		t.Error("re-propagate must be idempotent: output must be byte-identical to the first run's output")
	}
}
