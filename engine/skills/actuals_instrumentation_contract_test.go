package skills

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const (
	actualsSchemaRelPath           = "skills/_shared/actuals-record.schema.json"
	timeEstimationSkillRelPath     = "skills/sdd-time-estimation/SKILL.md"
	inceptionPipelineSkillRelPath  = "skills/inception-pipeline/SKILL.md"
	roadmapMakerSkillRelPath       = "skills/roadmap-maker/SKILL.md"
	correctedActualsFixtureRelPath = "engine/skills/testdata/corrected-actuals-sync-check-repo-behind-origin.json"

	// Section anchors for the marker pins below. A pinned marker only counts when it
	// lives in the section that owns it, so relocating it into an unrelated section
	// fails the gate instead of passing on a whole-file substring hit.
	timeEstimationHardRulesHeading      = "## Hard Rules"
	timeEstimationOutputContractHeading = "## Output Contract"
)

// TestActualsInstrumentationContract is the committed content gate (design D3) for the
// actuals calendar-time / checkpoint-count instrumentation fix. It asserts canonical marker
// strings and schema structure directly against repository content, so `go test ./...`
// is genuinely RED before the fix and GREEN after — see design.md's Content-Verification
// Gate table for the requirement mapping behind each t.Run group.
func TestActualsInstrumentationContract(t *testing.T) {
	repoRoot := actualsInstrumentationRepoRoot(t)

	t.Run("R004_R005_schema_temporal_boundary_and_interruption", func(t *testing.T) {
		schemaRaw := readRepoFile(t, repoRoot, actualsSchemaRelPath)
		schema := decodeActualsSchema(t, schemaRaw)

		// R-004/R-005 constrain total_wall_clock_hours' OWN description, so the pins are
		// scoped to that description: the word "interruption" appearing on some unrelated
		// property must not satisfy this gate.
		wallClock := actualsSchemaPropertyDescription(t, schema, "total_wall_clock_hours")
		for _, want := range []string{
			"from the tiering go-ahead checkpoint to archive",
			"including interruption gaps",
		} {
			if !strings.Contains(wallClock, want) {
				t.Fatalf("total_wall_clock_hours description must contain %q, got %q", want, wallClock)
			}
		}

		// The FORBID stays whole-file on purpose. For a negative assertion, whole-file
		// scope is strictly stronger than description scope: the retired boundary phrase
		// must not reappear anywhere, including on checkpoint_count, which now shares the
		// tiering-go-ahead-to-archive boundary and could regress the same way.
		if strings.Contains(schemaRaw, "from first apply to archive") {
			t.Fatal("schema must not retain the old temporal boundary phrase")
		}
	})

	t.Run("R015_R006_R007_R008_checkpoint_count_description", func(t *testing.T) {
		schema := readRepoFile(t, repoRoot, actualsSchemaRelPath)
		if !strings.Contains(schema, "round-trip") {
			t.Fatal("checkpoint_count description must describe round-trip counting (R-015)")
		}
		if strings.Contains(schema, "LOWER BOUND") {
			t.Fatal("checkpoint_count description must drop the structural LOWER BOUND framing")
		}
	})

	t.Run("R009_calibration_excludes_calendar_time", func(t *testing.T) {
		skill := readRepoFile(t, repoRoot, timeEstimationSkillRelPath)
		for _, want := range []string{
			"NEVER an input to the agent-compute-time baseline",
			"interruption-clean residual samples with positive `checkpoint_count`",
			"`(total_wall_clock_hours − sum(phase hours)) / checkpoint_count`",
			"Exclude any residual known or narrated to include a session, rate-limit, or provider interruption",
			"never subtract a guessed interruption amount",
		} {
			if !strings.Contains(skill, want) {
				t.Fatalf("CALIBRATION rule must contain %q", want)
			}
		}
		if strings.Contains(skill, "and `total_wall_clock_hours`, build a per-phase agent-compute-time baseline") {
			t.Fatal("CALIBRATION rule must not blend total_wall_clock_hours into the compute baseline")
		}
	})

	t.Run("R010_R011_delivery_window_formula_shape", func(t *testing.T) {
		skill := readRepoFile(t, repoRoot, timeEstimationSkillRelPath)
		for _, want := range []string{
			"expected checkpoints × round-trip latency + interruption allowance",
			"does not scale with checkpoint count",
			"no interruption-clean residual sample exists",
			"fixed, explicitly-disclosed buffer marked \"uncalibrated\"",
		} {
			if !strings.Contains(skill, want) {
				t.Fatalf("Output item 6 must contain %q", want)
			}
		}
	})

	t.Run("R006_R007_R015_inception_round_trip_and_explicit_zero", func(t *testing.T) {
		skill := readRepoFile(t, repoRoot, inceptionPipelineSkillRelPath)
		for _, want := range []string{
			"one unit per distinct human round-trip reply",
			"explicitly zero",
		} {
			if !strings.Contains(skill, want) {
				t.Fatalf("closure-feedback must contain %q", want)
			}
		}
		if strings.Contains(skill, "is a structural lower bound, not a complete count") {
			t.Fatal("closure-feedback must drop the structural-lower-bound framing")
		}
	})

	t.Run("R014_amendedR007_fixture_pins", func(t *testing.T) {
		fixtureRaw := readRepoFile(t, repoRoot, correctedActualsFixtureRelPath)
		var fixture map[string]any
		if err := json.Unmarshal([]byte(fixtureRaw), &fixture); err != nil {
			t.Fatalf("fixture must be valid JSON: %v", err)
		}

		checkpointCount, ok := fixture["checkpoint_count"].(float64)
		if !ok || checkpointCount != 12 {
			t.Fatalf("fixture checkpoint_count = %v, want 12", fixture["checkpoint_count"])
		}

		wallClock, ok := fixture["total_wall_clock_hours"].(float64)
		if !ok || wallClock == 1.4 || wallClock < 24 {
			t.Fatalf("fixture total_wall_clock_hours = %v, want != 1.4 and >= 24", fixture["total_wall_clock_hours"])
		}

		variance, ok := fixture["variance_vs_plan"].(string)
		if !ok {
			t.Fatal("fixture variance_vs_plan must be a string")
		}
		for _, want := range []string{
			"RECONSTRUCTED FROM THE CLOSURE NARRATIVE, NOT MEASURED",
			"durably observed",
			"reconstructed from narrative",
			"= 12",
			"Durable floor: 2 of 12",
			"AMB-001",
		} {
			if !strings.Contains(variance, want) {
				t.Fatalf("fixture variance_vs_plan must contain %q", want)
			}
		}
		if strings.Contains(variance, "the only checkpoint inception-pipeline itself durably records") {
			t.Fatal("fixture variance_vs_plan must not claim tiering go-ahead is the only durably recorded checkpoint")
		}
	})

	t.Run("R001_R002_three_units_never_blended", func(t *testing.T) {
		skill := readRepoFile(t, repoRoot, timeEstimationSkillRelPath)
		hardRules := markdownSectionBody(t, skill, timeEstimationHardRulesHeading)
		// Scoped to the R-001/R-002 list item itself: "agent-compute-time" also appears in
		// other Hard Rules bullets, so a section-wide substring search could not detect the
		// three-unit naming being dropped from this rule.
		rule := markdownListItemContaining(t, hardRules, timeEstimationHardRulesHeading, "R-001/R-002")
		for _, want := range []string{
			"R-001/R-002",
			// All three units must be named explicitly, not merely alluded to.
			"agent-compute-time",
			"elapsed-calendar-time",
			"human-confirmation-checkpoint-count",
			"never summed or averaged into one figure",
		} {
			if !strings.Contains(rule, want) {
				t.Fatalf("%s R-001/R-002 rule must contain %q, got %q", timeEstimationHardRulesHeading, want, rule)
			}
		}
	})

	t.Run("R012_actuals_output_separate_labels", func(t *testing.T) {
		skill := readRepoFile(t, repoRoot, timeEstimationSkillRelPath)
		outputContract := markdownSectionBody(t, skill, timeEstimationOutputContractHeading)
		item := markdownListItemContaining(t, outputContract, timeEstimationOutputContractHeading, "Actuals and Calibration")
		want := "elapsed-calendar-time, and checkpoint count under separate labels, never blended"
		if !strings.Contains(item, want) {
			t.Fatalf("%s Actuals and Calibration item must contain %q, got %q", timeEstimationOutputContractHeading, want, item)
		}
	})

	t.Run("R013_roadmap_maker_no_compute_time_source_invariant", func(t *testing.T) {
		// Invariant, same precedent as D2_D5_closed_schema_invariant: roadmap-maker/SKILL.md
		// is unchanged versus main (design D6 forbids editing it), so this FORBID-list
		// regression guard is GREEN-by-construction on both main and HEAD. It is NOT RED
		// coverage for R-013 — it only guards against a future regression that sources a
		// tracking-line figure from one of the compute-time/calendar-time fields.
		skill := readRepoFile(t, repoRoot, roadmapMakerSkillRelPath)
		for _, forbidden := range []string{
			"total_wall_clock_hours", "checkpoint_count", "implementation_hours",
			"review_gate_hours", "post_review_fix_hours",
		} {
			if strings.Contains(skill, forbidden) {
				t.Fatalf("roadmap-maker must not source tracking-line data from %q", forbidden)
			}
		}
	})

	t.Run("D2_D5_closed_schema_invariant", func(t *testing.T) {
		// Deliberately not part of the RED gate (design.md Content-Verification Gate,
		// group 7): every assertion in this group — the closed-schema flag, the declared
		// property set, and the schema/fixture agreement — already holds today. The group
		// stays GREEN before and after the fix, so it is NOT RED coverage; it exists to
		// guard against future regressions such as a re-added sub-count field, an opened
		// schema, or a fixture that drifts from the declared shape.
		schemaRaw := readRepoFile(t, repoRoot, actualsSchemaRelPath)
		schema := decodeActualsSchema(t, schemaRaw)

		// "No schema shape change anywhere in the record" also requires the record to stay
		// closed. Absent and inverted are distinct regressions, so they fail distinctly;
		// the raw-message decode is what makes absence detectable at all (a plain bool
		// field would read as a passing `false` when the flag is simply gone).
		if schema.AdditionalProperties == nil {
			t.Fatal("schema must declare additionalProperties: the closed-record flag is absent")
		}
		var closedRecord bool
		if err := json.Unmarshal(schema.AdditionalProperties, &closedRecord); err != nil {
			t.Fatalf("schema additionalProperties must be the boolean false, got %s: %v", schema.AdditionalProperties, err)
		}
		if closedRecord {
			t.Fatal("schema additionalProperties must be false, got true: the record is no longer closed")
		}

		wantProperties := []string{
			"change_name", "project", "implementation_hours", "review_gate_hours",
			"total_wall_clock_hours", "post_review_fix_hours", "approval_decision",
			"scope_drift_notes", "variance_vs_plan", "requirement_count",
			"changed_lines", "review_lens_count", "checkpoint_count",
		}
		assertStringList(t, actualsMapKeys(schema.Properties), wantProperties)

		// The fixture is committed, so an unreadable path is a real failure — read it
		// through readRepoFile, which fails loudly, instead of skipping the two
		// schema/fixture agreement checks below on any read error.
		fixtureRaw := readRepoFile(t, repoRoot, correctedActualsFixtureRelPath)
		var fixture map[string]json.RawMessage
		if err := json.Unmarshal([]byte(fixtureRaw), &fixture); err != nil {
			t.Fatalf("fixture must be valid JSON: %v", err)
		}
		for key := range fixture {
			if _, ok := schema.Properties[key]; !ok {
				t.Fatalf("fixture key %q is not declared in schema properties", key)
			}
		}
		for _, req := range schema.Required {
			if _, ok := fixture[req]; !ok {
				t.Fatalf("schema required field %q missing from fixture", req)
			}
		}
	})
}

func actualsInstrumentationRepoRoot(t *testing.T) string {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolving repo root: %v", err)
	}
	return repoRoot
}

func actualsMapKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// actualsSchemaDocument is the decoded slice of the actuals record schema this gate pins.
// AdditionalProperties is a raw message so an absent flag stays distinguishable from a
// flag that is present and set to true.
type actualsSchemaDocument struct {
	AdditionalProperties json.RawMessage            `json:"additionalProperties"`
	Properties           map[string]json.RawMessage `json:"properties"`
	Required             []string                   `json:"required"`
}

func decodeActualsSchema(t *testing.T, schemaRaw string) actualsSchemaDocument {
	t.Helper()
	var schema actualsSchemaDocument
	if err := json.Unmarshal([]byte(schemaRaw), &schema); err != nil {
		t.Fatalf("schema must be valid JSON: %v", err)
	}
	return schema
}

// actualsSchemaPropertyDescription returns one property's own description, so a
// description pin cannot be satisfied by text living on a different property.
func actualsSchemaPropertyDescription(t *testing.T, schema actualsSchemaDocument, property string) string {
	t.Helper()
	raw, ok := schema.Properties[property]
	if !ok {
		t.Fatalf("schema must declare property %q", property)
	}
	var decoded struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("schema property %q must be a JSON object: %v", property, err)
	}
	if decoded.Description == "" {
		t.Fatalf("schema property %q must carry a description", property)
	}
	return decoded.Description
}

// markdownSectionBody returns the body of the section introduced by the exact heading
// line, ending at the next heading of the same or higher level. A missing heading is a
// hard failure so a renamed section surfaces instead of silently passing an empty scope.
func markdownSectionBody(t *testing.T, doc, heading string) string {
	t.Helper()
	level := markdownHeadingLevel(heading)
	if level == 0 {
		t.Fatalf("heading anchor %q must be '#'-prefixed markdown heading text", heading)
	}

	lines := strings.Split(doc, "\n")
	start := -1
	inFence := false
	for i, line := range lines {
		if isMarkdownFence(line) {
			inFence = !inFence
			continue
		}
		if !inFence && strings.TrimRight(line, " \t") == heading {
			start = i + 1
			break
		}
	}
	if start < 0 {
		t.Fatalf("section heading %q not found — was the section renamed or removed?", heading)
	}

	inFence = false
	for i := start; i < len(lines); i++ {
		if isMarkdownFence(lines[i]) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if next := markdownHeadingLevel(lines[i]); next > 0 && next <= level {
			return strings.Join(lines[start:i], "\n")
		}
	}
	return strings.Join(lines[start:], "\n")
}

// markdownListItemContaining returns the single top-level list item of section (including
// its indented continuation lines) whose text contains marker. sectionName only labels
// failure messages.
func markdownListItemContaining(t *testing.T, section, sectionName, marker string) string {
	t.Helper()
	lines := strings.Split(section, "\n")
	hit := -1
	for i, line := range lines {
		if strings.Contains(line, marker) {
			hit = i
			break
		}
	}
	if hit < 0 {
		t.Fatalf("section %s contains no line with marker %q", sectionName, marker)
	}

	start := hit
	for start >= 0 && !isMarkdownListItemStart(lines[start]) {
		start--
	}
	if start < 0 {
		t.Fatalf("marker %q in section %s is not inside a top-level list item", marker, sectionName)
	}

	end := start + 1
	for end < len(lines) && !isMarkdownListItemStart(lines[end]) && strings.TrimSpace(lines[end]) != "" {
		end++
	}
	return strings.Join(lines[start:end], "\n")
}

func markdownHeadingLevel(line string) int {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level >= len(line) || line[level] != ' ' {
		return 0
	}
	return level
}

func isMarkdownFence(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "```")
}

// isMarkdownListItemStart reports whether line opens a top-level bullet ("- ", "* ") or
// ordered ("14. ") list item.
func isMarkdownListItemStart(line string) bool {
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
		return true
	}
	digits := 0
	for digits < len(line) && line[digits] >= '0' && line[digits] <= '9' {
		digits++
	}
	return digits > 0 && strings.HasPrefix(line[digits:], ". ")
}
