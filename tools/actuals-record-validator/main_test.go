package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}

func schemaPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "skills", "_shared", "actuals-record.schema.json")
}

// buildValidator compiles the tool once per test binary. The tests exercise the
// real process, not an in-process function, because the contract this tool
// carries is its EXIT CODE: a closure step that reads "non-zero is a hard stop"
// is only meaningful if a non-zero code actually reaches the caller.
var validatorBinary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "actuals-record-validator")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	validatorBinary = filepath.Join(dir, "actuals-record-validator")
	build := exec.Command("go", "build", "-o", validatorBinary, ".")
	if out, buildErr := build.CombinedOutput(); buildErr != nil {
		panic(string(out))
	}
	os.Exit(m.Run())
}

func runValidator(t *testing.T, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command(validatorBinary, args...)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(output)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("running validator: %v", err)
	}
	return exitErr.ExitCode(), string(output)
}

// validRecord is a minimal record that satisfies every rule. Each test mutates
// exactly one thing, so a failure names one cause.
func validRecord() map[string]any {
	return map[string]any{
		"change_name":       "sample-change",
		"project":           "labdrian-sdd-overlay",
		"approval_decision": "approved",
		"scope_drift_notes": "No scope drift: every task in tasks.md landed as planned, and nothing outside the declared change footprint was edited.",
		"variance_vs_plan":  "Effort landed inside the planned range. total_wall_clock_hours is measured, not reconstructed. checkpoint_count is 1: the tiering go-ahead, durably observed via pipeline-state; zero further checkpoints were reconstructed from the narrative.",
	}
}

func writeRecord(t *testing.T, record map[string]any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "actuals.json")
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatalf("encode record: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write record: %v", err)
	}
	return path
}

func validateRecord(t *testing.T, record map[string]any) (int, string) {
	t.Helper()
	return runValidator(t, "--schema", schemaPath(t), "--instance", writeRecord(t, record))
}

func TestValidRecordExitsZero(t *testing.T) {
	code, output := validateRecord(t, validRecord())
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; output=%q", code, exitOK, output)
	}
}

// TestPointerTextInRequiredNarrativeIsRejected is the acceptance criterion for
// this tool. A required field is satisfied, as far as JSON Schema can tell, by
// ANY string — including one whose entire content is a pointer at some other
// place where the content supposedly lives. That is exactly how the real defect
// looked, and schema shape alone can never catch it: actuals-record.schema.json
// declares approval_decision, scope_drift_notes and variance_vs_plan as bare
// strings with no constraint at all.
func TestPointerTextInRequiredNarrativeIsRejected(t *testing.T) {
	const pointer = "See the narrative below — this field is summarised there rather than duplicated"

	for _, field := range []string{"scope_drift_notes", "variance_vs_plan"} {
		t.Run(field, func(t *testing.T) {
			record := validRecord()
			record[field] = pointer
			code, output := validateRecord(t, record)
			if code != exitSemanticValidation {
				t.Fatalf("pointer text in %s: exit code = %d, want %d; output=%q", field, code, exitSemanticValidation, output)
			}
			if !strings.Contains(output, field) {
				t.Errorf("failure %q does not name the offending field %q", output, field)
			}
		})
	}

	t.Run("approval_decision", func(t *testing.T) {
		record := validRecord()
		record["approval_decision"] = pointer
		code, output := validateRecord(t, record)
		if code != exitSemanticValidation {
			t.Fatalf("pointer text in approval_decision: exit code = %d, want %d; output=%q", code, exitSemanticValidation, output)
		}
		if !strings.Contains(output, "approval_decision") {
			t.Errorf("failure %q does not name approval_decision", output)
		}
	})
}

func TestEmptyNarrativeIsRejected(t *testing.T) {
	for _, value := range []string{"", "   ", "N/A", "TBD", "-"} {
		record := validRecord()
		record["scope_drift_notes"] = value
		code, output := validateRecord(t, record)
		if code != exitSemanticValidation {
			t.Errorf("scope_drift_notes=%q: exit code = %d, want %d; output=%q", value, code, exitSemanticValidation, output)
		}
	}
}

func TestApprovalDecisionOutsideTheClosedVocabularyIsRejected(t *testing.T) {
	record := validRecord()
	record["approval_decision"] = "looks fine to me"
	code, output := validateRecord(t, record)
	if code != exitSemanticValidation {
		t.Fatalf("exit code = %d, want %d; output=%q", code, exitSemanticValidation, output)
	}
	if !strings.Contains(output, "approved") {
		t.Errorf("failure %q does not name the accepted vocabulary", output)
	}
}

func TestApprovalDecisionVocabularyIsCaseAndSpaceInsensitive(t *testing.T) {
	for _, value := range []string{"Approved", " approved-with-notes ", "REJECTED"} {
		record := validRecord()
		record["approval_decision"] = value
		if code, output := validateRecord(t, record); code != exitOK {
			t.Errorf("approval_decision=%q: exit code = %d, want %d; output=%q", value, code, exitOK, output)
		}
	}
}

func TestNegativeHoursAreRejected(t *testing.T) {
	// The schema types these as plain numbers with no minimum, so a negative
	// duration passes schema validation and lands in the calibration baseline.
	for _, field := range []string{
		"implementation_hours", "review_gate_hours", "total_wall_clock_hours",
		"post_review_fix_hours",
	} {
		record := validRecord()
		record[field] = -1.5
		record["variance_vs_plan"] = validRecord()["variance_vs_plan"]
		code, output := validateRecord(t, record)
		if code != exitSemanticValidation {
			t.Errorf("%s=-1.5: exit code = %d, want %d; output=%q", field, code, exitSemanticValidation, output)
		}
	}
	for _, field := range []string{"requirement_count", "changed_lines", "review_lens_count", "checkpoint_count"} {
		record := validRecord()
		record[field] = -1
		code, output := validateRecord(t, record)
		if code != exitSemanticValidation {
			t.Errorf("%s=-1: exit code = %d, want %d; output=%q", field, code, exitSemanticValidation, output)
		}
	}
}

// TestCheckpointCountRequiresDurableProvenanceDisclosure is the cross-field
// rule R-007 already states in prose and nothing enforced: a checkpoint_count
// that does not say which of its units were durably observed and which were
// reconstructed from the narrative is a number with no provenance.
func TestCheckpointCountRequiresDurableProvenanceDisclosure(t *testing.T) {
	record := validRecord()
	record["checkpoint_count"] = 7
	record["variance_vs_plan"] = "Effort landed inside the planned range and nothing else is worth noting about this cycle."
	code, output := validateRecord(t, record)
	if code != exitSemanticValidation {
		t.Fatalf("exit code = %d, want %d; output=%q", code, exitSemanticValidation, output)
	}
	if !strings.Contains(output, "checkpoint_count") {
		t.Errorf("failure %q does not name checkpoint_count", output)
	}
}

// TestCheckpointDisclosureAcceptsTheDocumentedWording runs the rule against the
// words the writer instruction actually prescribes.
//
// skills/inception-pipeline/SKILL.md tells closure-feedback to "itemize in
// variance_vs_plan free text which checkpoints were durably observed (via
// pipeline-state) versus reconstructed from the closure narrative". The rule
// required the exact substring "durable", and "durably" does not contain it —
// so following the documented instruction to the letter produced a record the
// validator REJECTED, and the fail-closed rule turns that into a hard stop that
// blocks the write. A rule and the instruction it enforces cannot disagree
// about which words satisfy it.
func TestCheckpointDisclosureAcceptsTheDocumentedWording(t *testing.T) {
	spellings := map[string]string{
		"durably_observed": "checkpoint_count is 7. Exactly 1 was durably observed via sdd/{change}/pipeline-state: the tiering go-ahead. The remaining 6 were reconstructed from the closure narrative.",
		"durable_floor":    "checkpoint_count is 7. The durable floor read from pipeline-state is 1; the other 6 were reconstructed from the closure narrative.",
		"documented_zero":  "checkpoint_count is 1: the tiering go-ahead, durably observed via pipeline-state; zero further checkpoints were reconstructed from the narrative.",
		"reconstruction":   "checkpoint_count is 7: 1 durably observed via pipeline-state, 6 by reconstruction from the closure narrative.",
	}
	for name, variance := range spellings {
		t.Run(name, func(t *testing.T) {
			record := validRecord()
			record["checkpoint_count"] = 7
			record["variance_vs_plan"] = variance
			if code, output := validateRecord(t, record); code != exitOK {
				t.Fatalf("exit code = %d, want %d for the documented wording; output=%q", code, exitOK, output)
			}
		})
	}
}

// TestCheckpointDisclosureStillRejectsAHalfDisclosure is the guard against
// fixing the wording by loosening the rule: naming one side of the split and
// not the other still records a number a reader cannot weigh.
func TestCheckpointDisclosureStillRejectsAHalfDisclosure(t *testing.T) {
	halves := map[string]string{
		"durable_only":       "checkpoint_count is 7, and 1 of those was durably observed via pipeline-state.",
		"reconstructed_only": "checkpoint_count is 7, all of them reconstructed from the closure narrative.",
	}
	for name, variance := range halves {
		t.Run(name, func(t *testing.T) {
			record := validRecord()
			record["checkpoint_count"] = 7
			record["variance_vs_plan"] = variance
			if code, output := validateRecord(t, record); code != exitSemanticValidation {
				t.Fatalf("exit code = %d, want %d for a half disclosure; output=%q", code, exitSemanticValidation, output)
			}
		})
	}
}

// TestTotalWallClockHoursRequiresMeasurementProvenance mirrors R-014's
// mandatory disclaimer: a reconstructed figure presented as if measured is the
// defect class this whole instrument exists to remove.
func TestTotalWallClockHoursRequiresMeasurementProvenance(t *testing.T) {
	record := validRecord()
	record["total_wall_clock_hours"] = 36
	record["variance_vs_plan"] = "The pre-start estimate was 14-24h and the change took roughly a day and a half of calendar time."
	code, output := validateRecord(t, record)
	if code != exitSemanticValidation {
		t.Fatalf("exit code = %d, want %d; output=%q", code, exitSemanticValidation, output)
	}
	if !strings.Contains(output, "total_wall_clock_hours") {
		t.Errorf("failure %q does not name total_wall_clock_hours", output)
	}
}

func TestChangeNameMustBeKebabCase(t *testing.T) {
	record := validRecord()
	record["change_name"] = "Sample Change"
	if code, output := validateRecord(t, record); code != exitSemanticValidation {
		t.Fatalf("exit code = %d, want %d; output=%q", code, exitSemanticValidation, output)
	}
}

func TestUnknownPropertyFailsSchemaValidation(t *testing.T) {
	record := validRecord()
	record["invented_field"] = "anything"
	code, output := validateRecord(t, record)
	if code != exitSchemaValidation {
		t.Fatalf("exit code = %d, want %d; output=%q", code, exitSchemaValidation, output)
	}
}

func TestMissingRequiredFieldFailsSchemaValidation(t *testing.T) {
	record := validRecord()
	delete(record, "variance_vs_plan")
	code, output := validateRecord(t, record)
	if code != exitSchemaValidation {
		t.Fatalf("exit code = %d, want %d; output=%q", code, exitSchemaValidation, output)
	}
}

func TestMalformedInstanceExitsFour(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actuals.json")
	if err := os.WriteFile(path, []byte("{ not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	code, output := runValidator(t, "--schema", schemaPath(t), "--instance", path)
	if code != exitInstance {
		t.Fatalf("exit code = %d, want %d; output=%q", code, exitInstance, output)
	}
}

func TestMissingSchemaExitsThree(t *testing.T) {
	code, output := runValidator(t, "--schema", filepath.Join(t.TempDir(), "absent.json"), "--instance", writeRecord(t, validRecord()))
	if code != exitSchema {
		t.Fatalf("exit code = %d, want %d; output=%q", code, exitSchema, output)
	}
}

// TestIncompatibleSchemaVersionExitsThree is the bundle handshake. The actuals
// record has no contract_version property to pin — that is precisely why the
// entry-contract validator could not be reused here — so the version lives in
// the schema's own $id and is pinned there instead.
func TestIncompatibleSchemaVersionExitsThree(t *testing.T) {
	raw, err := os.ReadFile(schemaPath(t))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	schema["$id"] = "https://labdrian.ai/schemas/actuals-record/9.9.9"
	path := filepath.Join(t.TempDir(), "schema.json")
	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("encode schema: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write schema: %v", err)
	}
	code, output := runValidator(t, "--schema", path, "--instance", writeRecord(t, validRecord()))
	if code != exitSchema {
		t.Fatalf("exit code = %d, want %d; output=%q", code, exitSchema, output)
	}
	if !strings.Contains(output, "9.9.9") {
		t.Errorf("failure %q does not name the rejected version", output)
	}
}

func TestUsageErrorExitsTwo(t *testing.T) {
	if code, output := runValidator(t, "--schema", schemaPath(t)); code != exitUsage {
		t.Fatalf("missing --instance: exit code = %d, want %d; output=%q", code, exitUsage, output)
	}
}

// TestCommittedActualsFixturesValidate is the load-bearing regression guard:
// the two real records committed to this repository are the only actuals the
// project has, and a rule that rejects them is a rule that would have blocked
// the closures that produced them. It also proves the semantic rules are not
// so aggressive that honest prose trips them.
func TestCommittedActualsFixturesValidate(t *testing.T) {
	for _, rel := range []string{
		"engine/skills/testdata/corrected-actuals-skills-validate-ondisk-gate.json",
		"engine/skills/testdata/corrected-actuals-sync-check-repo-behind-origin.json",
	} {
		path := filepath.Join(repoRoot(t), filepath.FromSlash(rel))
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("committed fixture %s is unavailable: %v", rel, err)
		}
		code, output := runValidator(t, "--schema", schemaPath(t), "--instance", path)
		if code != exitOK {
			t.Errorf("%s: exit code = %d, want %d; output=%q", rel, code, exitOK, output)
		}
	}
}

// TestSpecZeroCaseScenarioWordingIsAccepted pins the rule to the SCENARIO that
// documents it. The zero case is the one where a writer has nothing to itemize,
// and openspec/specs/actuals-instrumentation/spec.md spells out what the record
// must then say. The half-disclosure guard requires BOTH stems, so a scenario
// whose own words name only the durable half describes a record the validator
// hard-stops at exit 6 — the spec and the rule disagreeing about the same
// sentence, which is the defect class this instrument exists to remove.
// Feeding the scenario's own words through the validator is the only check that
// cannot drift from it.
func TestSpecZeroCaseScenarioWordingIsAccepted(t *testing.T) {
	specPath := filepath.Join(repoRoot(t), filepath.FromSlash("openspec/specs/actuals-instrumentation/spec.md"))
	body, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	const marker = "explicitly states that all counted checkpoints"
	var scenario string
	for _, line := range strings.Split(string(body), "\n") {
		if strings.Contains(line, marker) {
			scenario = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- THEN"))
			break
		}
	}
	if scenario == "" {
		t.Fatalf("spec.md no longer carries the zero-case scenario (%q)", marker)
	}
	record := validRecord()
	record["checkpoint_count"] = 1
	record["variance_vs_plan"] = scenario
	if code, output := validateRecord(t, record); code != exitOK {
		t.Fatalf("the spec's own zero-case wording %q is rejected: exit code = %d, want %d; output=%q",
			scenario, code, exitOK, output)
	}
}
