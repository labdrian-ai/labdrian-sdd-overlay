package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
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

// saysOK reports whether output holds the line that a valid record prints first. The usage text, which a usage error
// prints, describes that line without the path of the schema it carries, so the path is part of what is looked for.
func saysOK(t *testing.T, output string) bool {
	t.Helper()
	return strings.Contains(output, "ok: actuals record satisfies "+schemaPath(t)+" and every semantic invariant")
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

// validateRecord validates record in a repository that holds no stored review result.
func validateRecord(t *testing.T, record map[string]any) (int, string) {
	t.Helper()
	return validateRecordIn(t, record, newRepo(t))
}

// validateRecordIn validates record against the stored review results of the repository at repo.
func validateRecordIn(t *testing.T, record map[string]any, repo string) (int, string) {
	t.Helper()
	return runValidator(t, "--schema", schemaPath(t), "--instance", writeRecord(t, record), "--repo", repo)
}

// newRepo makes the root of a repository as far as the validator can tell: a directory that holds a .git entry. It is
// the --repo of every test that has no stored result to give it, because a directory that is none is refused.
func newRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("make the .git directory: %v", err)
	}
	return repo
}

// storeResults stores in repo, as `labdrian review record` does, one result of the change for each launch given: a
// launch is the rows it was sent. It returns repo.
func storeResults(t *testing.T, repo, change string, launches ...[]string) string {
	t.Helper()
	dir := filepath.Join(repo, filepath.FromSlash(review.ResultsDir), change)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("make the results directory: %v", err)
	}
	for i, rows := range launches {
		head := fmt.Sprintf("%012x", i+2) + strings.Repeat("0", 28) // 40 hex characters, and a first 12 of its own per launch
		c, err := review.NewCandidate(strings.Repeat("1", 40), head, []byte("diff --git a/guard.go b/guard.go\n"), []string{"guard.go"})
		if err != nil {
			t.Fatalf("bind the candidate of launch %d: %v", i+1, err)
		}
		reply := fmt.Sprintf(`{"candidate":{"base":%q,"head":%q,"diff_sha256":%q},"inspection":{"status":"completed","paths":["guard.go"],"reason":""},"findings":[],"evidence":["read every changed path"]}`,
			c.Base, c.Head, c.DiffSHA256)
		file, err := review.EncodeResult(c, rows, []byte(reply))
		if err != nil {
			t.Fatalf("store the result of launch %d: %v", i+1, err)
		}
		if err := os.WriteFile(filepath.Join(dir, review.ResultName(c, 1)), file, 0o644); err != nil {
			t.Fatalf("write the result of launch %d: %v", i+1, err)
		}
	}
	return repo
}

func TestValidRecordExitsZero(t *testing.T) {
	code, output := validateRecord(t, validRecord())
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; output=%q", code, exitOK, output)
	}
}

// TestASemanticRefusalComesBeforeTheStoredResultsAreRead pins the order the README states: the record is held to its
// own rules first, so a record that breaks one is refused for it (exit 6) even when --repo could not be read (exit 7).
func TestASemanticRefusalComesBeforeTheStoredResultsAreRead(t *testing.T) {
	record := validRecord()
	record["approval_decision"] = "maybe"
	for name, repo := range map[string]string{
		"a --repo that does not exist":           filepath.Join(t.TempDir(), "missing"),
		"a --repo that is not a repository root": t.TempDir(),
	} {
		t.Run(name, func(t *testing.T) {
			code, output := validateRecordIn(t, record, repo)
			if code != exitSemanticValidation {
				t.Fatalf("exit code = %d, want %d (the semantic refusal, before the results are read); output=%q", code, exitSemanticValidation, output)
			}
			if !strings.Contains(output, "approval_decision") {
				t.Errorf("the refusal must name approval_decision, got %q", output)
			}
		})
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
	code, output := runValidator(t, "--schema", schemaPath(t), "--instance", path, "--repo", newRepo(t))
	if code != exitInstance {
		t.Fatalf("exit code = %d, want %d; output=%q", code, exitInstance, output)
	}
}

func TestMissingSchemaExitsThree(t *testing.T) {
	code, output := runValidator(t, "--schema", filepath.Join(t.TempDir(), "absent.json"), "--instance", writeRecord(t, validRecord()), "--repo", newRepo(t))
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
	code, output := runValidator(t, "--schema", path, "--instance", writeRecord(t, validRecord()), "--repo", newRepo(t))
	if code != exitSchema {
		t.Fatalf("exit code = %d, want %d; output=%q", code, exitSchema, output)
	}
	if !strings.Contains(output, "9.9.9") {
		t.Errorf("failure %q does not name the rejected version", output)
	}
}

func TestUsageErrorExitsTwo(t *testing.T) {
	if code, output := runValidator(t, "--schema", schemaPath(t), "--repo", newRepo(t)); code != exitUsage {
		t.Fatalf("missing --instance: exit code = %d, want %d; output=%q", code, exitUsage, output)
	}
}

// TestCommittedActualsFixturesValidate is the load-bearing regression guard:
// the two real records committed to this repository are the only actuals the
// project has, and a rule that rejects them is a rule that would have blocked
// the closures that produced them. It also proves the semantic rules are not
// so aggressive that honest prose trips them.
//
// One of the two, skills-validate-ondisk-gate, was written before the lens rows
// were stored (own-rdd S8): its review_lens_count of 4 is the length of
// selected_lenses in the receipt of another tool, and no stored result backs it.
// S13 refuses a count nothing backs, so that record is held to two outcomes
// instead of one: refused for that member, naming both numbers, and valid once
// the member is taken out, which shows that no other rule rejects what the
// project already wrote. The fixture is a mirror of the live record and is not
// edited.
func TestCommittedActualsFixturesValidate(t *testing.T) {
	refusedForItsCount := 0
	for _, rel := range []string{
		"engine/skills/testdata/corrected-actuals-skills-validate-ondisk-gate.json",
		"engine/skills/testdata/corrected-actuals-sync-check-repo-behind-origin.json",
	} {
		path := filepath.Join(repoRoot(t), filepath.FromSlash(rel))
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("committed fixture %s is unavailable: %v", rel, err)
		}
		var record map[string]any
		if err := json.Unmarshal(raw, &record); err != nil {
			t.Fatalf("committed fixture %s is not JSON: %v", rel, err)
		}
		claimed, countsLenses := record["review_lens_count"]
		if !countsLenses {
			code, output := runValidator(t, "--schema", schemaPath(t), "--instance", path, "--repo", newRepo(t))
			if code != exitOK {
				t.Errorf("%s: exit code = %d, want %d; output=%q", rel, code, exitOK, output)
			}
			continue
		}
		refusedForItsCount++
		code, output := runValidator(t, "--schema", schemaPath(t), "--instance", path, "--repo", newRepo(t))
		if want := fmt.Sprintf("review_lens_count is %v but", claimed); code != exitSemanticValidation || !strings.Contains(output, want) {
			t.Errorf("%s: exit code = %d, output=%q; want exit %d and %q: the count was never stored", rel, code, output, exitSemanticValidation, want)
		}
		delete(record, "review_lens_count")
		if code, output := validateRecord(t, record); code != exitOK {
			t.Errorf("%s without its review_lens_count: exit code = %d, want %d; output=%q", rel, code, exitOK, output)
		}
	}
	if refusedForItsCount != 1 {
		t.Errorf("%d committed fixtures carry a review_lens_count, want exactly the one written before the rows were stored", refusedForItsCount)
	}
}

// lensRecord is validRecord for the change the stored results of these tests are of, with the count it claims, or
// none when claimed is nil.
func lensRecord(claimed any) map[string]any {
	record := validRecord()
	if claimed != nil {
		record["review_lens_count"] = claimed
	}
	return record
}

// The count is the number of rows the stored results of the change record, one for each row of each launch: two
// launches that were sent two rows and one row make 3, and a record that claims 3 is accepted, with the comparison
// said on stdout so that a pass is never silent.
func TestLensCountEqualToTheRowsOfTheStoredResultsIsAccepted(t *testing.T) {
	repo := storeResults(t, newRepo(t), "sample-change", []string{"default", "readability"}, []string{"default"})

	code, output := validateRecordIn(t, lensRecord(3), repo)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; output=%q", code, exitOK, output)
	}
	if want := `review_lens_count: 3 equals the 3 rows recorded in 2 stored review results of "sample-change"`; !strings.Contains(output, want) {
		t.Errorf("output %q does not say what was compared, want it to contain %q", output, want)
	}
}

// A count that differs from the rows the stored results record is refused whichever way it differs, and the refusal
// names both numbers: the one the record claims and the one counted.
func TestLensCountThatDiffersFromTheRowsOfTheStoredResultsIsRefusedNamingBothNumbers(t *testing.T) {
	repo := storeResults(t, newRepo(t), "sample-change", []string{"default", "readability"}, []string{"default"})
	for _, claimed := range []int{2, 5} {
		code, output := validateRecordIn(t, lensRecord(claimed), repo)

		if code != exitSemanticValidation {
			t.Errorf("review_lens_count %d: exit code = %d, want %d; output=%q", claimed, code, exitSemanticValidation, output)
		}
		for _, want := range []string{fmt.Sprintf("review_lens_count is %d but", claimed), `of "sample-change" record 3 rows`} {
			if !strings.Contains(output, want) {
				t.Errorf("review_lens_count %d: output %q does not contain %q", claimed, output, want)
			}
		}
	}
}

// A change reviewed before the rows were stored has no stored result, and the count it claims has nothing behind it:
// it is refused, whoever else's results the repository holds, and the refusal says the change has none.
func TestLensCountOfAChangeWithNoStoredResultIsRefused(t *testing.T) {
	anotherChangeOnly := storeResults(t, newRepo(t), "another-change", []string{"default", "readability", "risk-line"})
	cases := map[string]string{
		"an empty repository":                newRepo(t),
		"the results of another change only": anotherChangeOnly,
	}
	for name, repo := range cases {
		t.Run(name, func(t *testing.T) {
			code, output := validateRecordIn(t, lensRecord(4), repo)

			if code != exitSemanticValidation {
				t.Fatalf("exit code = %d, want %d; output=%q", code, exitSemanticValidation, output)
			}
			for _, want := range []string{"review_lens_count is 4 but", `of "sample-change" record 0 rows`, "no stored review result"} {
				if !strings.Contains(output, want) {
					t.Errorf("output %q does not contain %q", output, want)
				}
			}
		})
	}
}

// A count of 0 is a count like any other: no stored result backs it either, so it is refused as 4 is, and the refusal
// says to omit it. Only omitting the count is the explicit outcome of a change without stored results; a 0 that passed
// because it does not differ from 0 rows would be the claim of a count nothing backs.
func TestLensCountOfZeroForAChangeWithNoStoredResultIsRefused(t *testing.T) {
	anotherChangeOnly := storeResults(t, newRepo(t), "another-change", []string{"default"})
	cases := map[string]string{
		"an empty repository":                newRepo(t),
		"the results of another change only": anotherChangeOnly,
	}
	for name, repo := range cases {
		t.Run(name, func(t *testing.T) {
			code, output := validateRecordIn(t, lensRecord(0), repo)

			if code != exitSemanticValidation {
				t.Fatalf("exit code = %d, want %d; output=%q", code, exitSemanticValidation, output)
			}
			for _, want := range []string{"review_lens_count is 0 but", `of "sample-change" record 0 rows`, "no stored review result", "omit review_lens_count"} {
				if !strings.Contains(output, want) {
					t.Errorf("output %q does not contain %q", output, want)
				}
			}
			if saysOK(t, output) {
				t.Errorf("output %q must not say ok", output)
			}
		})
	}
}

// A record that omits the count says that no stored result records a row, and with none stored there is nothing to
// compare: it passes, and says so instead of passing in silence.
func TestOmittedLensCountOfAChangeWithNoStoredResultPassesAndSaysNothingWasCompared(t *testing.T) {
	code, output := validateRecord(t, lensRecord(nil))

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; output=%q", code, exitOK, output)
	}
	if want := `review_lens_count: omitted, and no stored review result of "sample-change" records a row: there was nothing to compare`; !strings.Contains(output, want) {
		t.Errorf("output %q does not say that nothing was compared, want it to contain %q", output, want)
	}
}

// Omitting the count is not a way around the rows: when a stored result of the change records them, a record without
// the count is refused and the refusal names the number to write.
func TestOmittedLensCountWhileTheStoredResultsRecordRowsIsRefused(t *testing.T) {
	repo := storeResults(t, newRepo(t), "sample-change", []string{"default", "readability"}, []string{"default"})

	code, output := validateRecordIn(t, lensRecord(nil), repo)

	if code != exitSemanticValidation {
		t.Fatalf("exit code = %d, want %d; output=%q", code, exitSemanticValidation, output)
	}
	for _, want := range []string{"review_lens_count is omitted but", `of "sample-change" record 3 rows`, "write review_lens_count 3"} {
		if !strings.Contains(output, want) {
			t.Errorf("output %q does not contain %q", output, want)
		}
	}
}

// A pass prints two lines on stdout, not one: the ok line, then the line that says what the review_lens_count check
// compared (S13 asks for the explicit outcome, and a pass is never silent about it). The usage text says so, so that a
// caller that reads the output does not take the first line for the whole of it.
func TestAPassPrintsTwoLinesAndTheUsageTextSaysSo(t *testing.T) {
	code, output := validateRecord(t, lensRecord(nil))

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; output=%q", code, exitOK, output)
	}
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "ok: actuals record satisfies ") || !strings.HasPrefix(lines[1], "review_lens_count: ") {
		t.Errorf("a pass printed %q, want two lines: the ok line, then the review_lens_count line", output)
	}
	if !saysOK(t, output) {
		t.Errorf("the check that a refusal does not say ok does not find the ok line of a pass %q: it would pass for any output", output)
	}

	code, usage := runValidator(t, "--help")

	if code != exitOK {
		t.Fatalf("--help: exit code = %d, want %d; output=%q", code, exitOK, usage)
	}
	if want := "prints two lines on stdout"; !strings.Contains(usage, want) {
		t.Errorf("the usage text %q must say that a valid record %s", usage, want)
	}
}

// A check that cannot run refuses and never passes. Without --repo the stored results cannot be read, so the command
// line is refused as a usage error, whatever the record says.
func TestMissingRepoExitsTwoAndChecksNothing(t *testing.T) {
	code, output := runValidator(t, "--schema", schemaPath(t), "--instance", writeRecord(t, lensRecord(nil)))

	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d; output=%q", code, exitUsage, output)
	}
	if !strings.Contains(output, "--repo") || saysOK(t, output) {
		t.Errorf("output %q must name --repo as the missing option and must not say ok", output)
	}
}

// exitUnreadableResults is the number the usage text and the skills document for "the stored review results could not
// be read". It is written out here, not taken from exitReviewResults, so that renumbering the code fails a test: the
// callers read the number.
const exitUnreadableResults = 7

// A --repo that is not a directory is not "a repository with no results": the stored results cannot be read, which is
// exit 7 and not a pass. A file says so in the validator's own words, because what the store makes of a root that is
// a file is the store's to say.
func TestRepoThatIsNotADirectoryExitsSeven(t *testing.T) {
	file := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(file, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cases := []struct{ name, repo, reason string }{
		{"a path that is not there", filepath.Join(t.TempDir(), "absent"), ""},
		{"a file", file, "it is not a directory"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			code, output := validateRecordIn(t, lensRecord(nil), tt.repo)

			if code != exitUnreadableResults {
				t.Fatalf("exit code = %d, want %d; output=%q", code, exitUnreadableResults, output)
			}
			if !strings.Contains(output, tt.repo) || !strings.Contains(output, tt.reason) || saysOK(t, output) {
				t.Errorf("output %q must name %s as the repository that cannot be read, say %q, and must not say ok", output, tt.repo, tt.reason)
			}
		})
	}
}

// --repo is an absolute path, as the usage text says: go run -C changes the working directory, so a relative path names
// whatever the caller's directory was not. It is refused as a usage error before anything is read, even when it names a
// repository root whose results agree with the count, so that the refusal is about the form and not about the content.
func TestRelativeRepoExitsTwoAndChecksNothing(t *testing.T) {
	repo := storeResults(t, newRepo(t), "sample-change", []string{"default"})
	here, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	toRepo, err := filepath.Rel(here, repo)
	if err != nil {
		t.Fatalf("path from %s to %s: %v", here, repo, err)
	}
	cases := []struct{ name, repo string }{
		{"the working directory", "."},
		{"a repository root written relative to the working directory", toRepo},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			code, output := validateRecordIn(t, lensRecord(1), tt.repo)

			if code != exitUsage {
				t.Fatalf("exit code = %d, want %d; output=%q", code, exitUsage, output)
			}
			if want := "--repo " + tt.repo + " is not an absolute path"; !strings.Contains(output, want) || saysOK(t, output) {
				t.Errorf("output %q must say %q and must not say ok", output, want)
			}
		})
	}
}

// --repo is the root of the repository that holds the stored results, and a directory that is not one holds none of
// them: with any directory, or a subdirectory of the repository, an omitted count would pass as "nothing to compare"
// while the repository records rows. It is exit 7 naming the path, as a --repo that is not a directory is.
func TestRepoThatIsNotTheRootOfARepositoryExitsSeven(t *testing.T) {
	repo := storeResults(t, newRepo(t), "sample-change", []string{"default", "readability"}, []string{"default"})
	cases := []struct{ name, dir string }{
		{"a directory that holds no repository", t.TempDir()},
		{"a subdirectory of the repository that holds the results", filepath.Join(repo, "odd")},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			code, output := validateRecordIn(t, lensRecord(nil), tt.dir)

			if code != exitUnreadableResults {
				t.Fatalf("exit code = %d, want %d; output=%q", code, exitUnreadableResults, output)
			}
			if !strings.Contains(output, tt.dir) || !strings.Contains(output, ".git") || saysOK(t, output) {
				t.Errorf("output %q must name %s as the directory that is not a repository root, say that it holds no .git, and must not say ok", output, tt.dir)
			}
		})
	}
}

// A linked worktree is a repository root too, and its .git is a file and not a directory: the root a closing agent in
// a worktree computes with git rev-parse --show-toplevel is still accepted, and its results are counted.
func TestRepoThatIsALinkedWorktreeRootIsAccepted(t *testing.T) {
	repo := storeResults(t, t.TempDir(), "sample-change", []string{"default", "readability"})
	if err := os.WriteFile(filepath.Join(repo, ".git"), []byte("gitdir: /elsewhere/.git/worktrees/w\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	code, output := validateRecordIn(t, lensRecord(2), repo)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; output=%q", code, exitOK, output)
	}
	if want := `review_lens_count: 2 equals the 2 rows recorded in 1 stored review results of "sample-change"`; !strings.Contains(output, want) {
		t.Errorf("output %q does not say what was compared, want it to contain %q", output, want)
	}
}

// A file named like a stored result that is not one cannot be counted, and the count is refused whether the record
// claims a number or omits it: the check runs either way.
func TestStoredResultThatIsNotAResultExitsSevenNamingTheFile(t *testing.T) {
	repo := storeResults(t, newRepo(t), "sample-change", []string{"default"})
	broken := filepath.Join(repo, "odd", "review-results", "sample-change", "aaaaaaaaaaaa-1.json")
	if err := os.WriteFile(broken, []byte(`{"version": 1}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	for name, claimed := range map[string]any{"a count claimed": 1, "a count omitted": nil} {
		t.Run(name, func(t *testing.T) {
			code, output := validateRecordIn(t, lensRecord(claimed), repo)

			if code != exitUnreadableResults {
				t.Fatalf("exit code = %d, want %d; output=%q", code, exitUnreadableResults, output)
			}
			if want := "odd/review-results/sample-change/aaaaaaaaaaaa-1.json"; !strings.Contains(output, want) {
				t.Errorf("output %q does not name the file %q that is not a stored result", output, want)
			}
		})
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
