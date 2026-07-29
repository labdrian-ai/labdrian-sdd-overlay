package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const (
	actualsSchemaRelPath           = "skills/_shared/actuals-record.schema.json"
	timeEstimationSkillRelPath     = "skills/sdd-time-estimation/SKILL.md"
	inceptionPipelineSkillRelPath  = "skills/inception-pipeline/SKILL.md"
	correctedActualsFixtureRelPath = "engine/skills/testdata/corrected-actuals-sync-check-repo-behind-origin.json"
)

// TestActualsInstrumentationContract is the committed content gate (design D3) for the
// actuals calendar-time / checkpoint-count instrumentation fix. It asserts canonical marker
// strings and schema structure directly against repository content, so `go test ./...`
// is genuinely RED before the fix and GREEN after — see design.md's Content-Verification
// Gate table for the requirement mapping behind each t.Run group.
func TestActualsInstrumentationContract(t *testing.T) {
	repoRoot := actualsInstrumentationRepoRoot(t)

	t.Run("R004_R005_schema_temporal_boundary_and_interruption", func(t *testing.T) {
		schema := readRepoFile(t, repoRoot, actualsSchemaRelPath)
		for _, want := range []string{"from the tiering go-ahead checkpoint to archive", "interruption"} {
			if !strings.Contains(schema, want) {
				t.Fatalf("schema must contain %q", want)
			}
		}
		if strings.Contains(schema, "from first apply to archive") {
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

	t.Run("D2_D5_closed_schema_invariant", func(t *testing.T) {
		// Deliberately not part of the RED gate (design.md Content-Verification Gate,
		// group 7): this schema-shape invariant already holds today. It stays GREEN
		// before and after the fix and starts guarding the fixture (once it exists)
		// against future regressions such as a re-added sub-count field.
		schemaRaw := readRepoFile(t, repoRoot, actualsSchemaRelPath)
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		}
		if err := json.Unmarshal([]byte(schemaRaw), &schema); err != nil {
			t.Fatalf("schema must be valid JSON: %v", err)
		}

		wantProperties := []string{
			"change_name", "project", "implementation_hours", "review_gate_hours",
			"total_wall_clock_hours", "post_review_fix_hours", "approval_decision",
			"scope_drift_notes", "variance_vs_plan", "requirement_count",
			"changed_lines", "review_lens_count", "checkpoint_count",
		}
		assertStringList(t, actualsMapKeys(schema.Properties), wantProperties)

		fixturePath := filepath.Join(repoRoot, correctedActualsFixtureRelPath)
		fixtureBytes, err := os.ReadFile(fixturePath)
		if err != nil {
			return // fixture not created yet — schema-only invariant above already holds
		}
		var fixture map[string]json.RawMessage
		if err := json.Unmarshal(fixtureBytes, &fixture); err != nil {
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
