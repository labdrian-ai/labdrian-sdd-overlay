// Command actuals-record-validator validates a closure actuals record against
// skills/_shared/actuals-record.schema.json and then checks the deterministic
// invariants the schema cannot express.
//
// Why a second validator rather than reusing entry-contract-validator: that
// tool's schema handshake demands `properties.contract_version.enum`, which the
// actuals schema does not have (its version lives in `$id`), and its semantic
// pass hardcodes entry-contract invariants — review slices, delivery strategy,
// size exceptions — none of which exist in an actuals record. Reusing it would
// have meant either weakening its handshake or teaching it a second document
// shape; both cost more than a sibling that owns one contract.
//
// The rule that matters most is the narrative one. `approval_decision`,
// `scope_drift_notes` and `variance_vs_plan` are declared in the schema as bare
// strings with no constraint, so JSON Schema considers a required field
// satisfied by ANY string — including one whose whole content is a pointer at
// wherever the content supposedly lives ("See the narrative below — this field
// is summarised there rather than duplicated"). A record like that passes every
// structural check while carrying none of the information the field exists to
// carry, and it was written by the same agent that then attested to it. Shape
// cannot catch it; these rules can.
//
// The other rule that reads outside the record is the lens count (own-rdd S13).
// `review_lens_count` is the number of lens rows the stored review results of
// the change record, one for each row of each launch, and it is checked, not
// trusted: the validator reads `odd/review-results/<change_name>/` under --repo
// through the engine's one function for that number (review/app CountLenses,
// never a count of its own) and refuses a record whose number differs, one that
// claims a count no stored result backs (a change reviewed before the rows were
// stored), one that omits it while a stored result records a row, and a check
// that cannot read the results. A check that cannot run refuses; it never
// passes.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/fsstore"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	exitOK                 = 0
	exitUsage              = 2
	exitSchema             = 3
	exitInstance           = 4
	exitSchemaValidation   = 5
	exitSemanticValidation = 6
	exitReviewResults      = 7
)

// currentRecordVersion is the actuals-record bundle version this validator
// enforces. It is read from the schema's `$id` trailing segment, because the
// actuals record — unlike the entry contract — carries no version property of
// its own: the record is written once at closure and never re-negotiated, so
// there is nothing on the instance to version.
const currentRecordVersion = "2.1.0"

// supportedRecordVersions lists every schema version this validator accepts,
// oldest first with currentRecordVersion last. Membership, not equality: an
// older but still-supported schema must keep validating the records written
// under it, exactly as the entry-contract bundle handles its own versions.
var supportedRecordVersions = []string{currentRecordVersion}

const schemaIDPrefix = "https://labdrian.ai/schemas/actuals-record/"

// approvalDecisions is the closed outcome vocabulary the schema's own
// `approval_decision` description already documents ("e.g. approved,
// approved-with-notes, rejected"). Promoting it from an example to a rule is
// what makes the field an OUTCOME rather than a free-text slot: a decision
// nobody can classify is not a decision, and pointer text lands outside the set
// automatically.
var approvalDecisions = []string{"approved", "approved-with-notes", "rejected"}

// deferralMarkers are phrases that assert the field's content lives somewhere
// ELSE INSTEAD OF HERE. They are deliberately not a general "mentions another
// field" probe: an honest narrative routinely cross-references its siblings
// while still carrying its own content, and forbidding that would push writers
// toward vaguer prose rather than fuller prose. Every marker below claims
// absence, not company.
var deferralMarkers = []string{
	"rather than duplicated",
	"not duplicated here",
	"summarised there",
	"summarized there",
	"summarised elsewhere",
	"summarized elsewhere",
	"see the narrative",
	"see the other field",
	"described elsewhere",
	"documented elsewhere",
	"recorded elsewhere",
	"refer to the narrative",
}

// placeholderNarratives are the values that spell "nothing recorded" while
// technically being strings.
var placeholderNarratives = []string{"", "-", "--", "n/a", "na", "none", "tbd", "todo", "pending", "?"}

// minNarrativeRunes is the floor below which a required narrative cannot have
// said anything a reader could act on. It is intentionally low: the rule exists
// to catch empties and stubs, not to legislate prose length.
const minNarrativeRunes = 40

var kebabCase = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// durableStem and reconstructedStem are the STEMS the provenance rules match,
// not whole words. The writer instruction prescribes "durably observed ...
// versus reconstructed from the closure narrative", and an exact-substring
// check for "durable" rejects "durably": a rule that refuses the wording its
// own instruction mandates fails the record, not the writer.
const (
	durableStem       = "durabl"
	reconstructedStem = "reconstruct"
)

const usageText = `Usage: actuals-record-validator --schema PATH --instance PATH --repo PATH

Validates a closure actuals record with the Draft 2020-12 actuals-record schema,
then checks the deterministic invariants the schema cannot express.

Options:
  --schema PATH     Actuals-record schema file
  --instance PATH   Actuals-record JSON instance
  --repo PATH       Repository root (an absolute path, as go run -C changes the
                    working directory) that holds the stored review results of
                    the change, odd/review-results/<change_name>/ (required)
  --version         Print validator/record version
  --help            Show this help

Semantic invariants (beyond schema shape):
  * change_name is kebab-case and project is non-empty.
  * approval_decision is one of: approved, approved-with-notes, rejected.
  * Required narratives (scope_drift_notes, variance_vs_plan) are SELF-CONTAINED:
    not empty, not a placeholder, and not a pointer deferring their content to
    another field or section.
  * Every hour and count field is non-negative.
  * A recorded checkpoint_count discloses, in variance_vs_plan, which units were
    durably observed and which were reconstructed from the closure narrative.
  * A recorded total_wall_clock_hours states in variance_vs_plan whether it was
    measured or reconstructed.
  * review_lens_count is the number of lens rows the stored review results of the
    change record, one for each row of each launch, counted by the engine
    (review/app CountLenses) from odd/review-results/<change_name>/ under --repo.
    A different count is refused naming both numbers; so is a count for a change
    with no stored result (a change reviewed before the rows were stored, whose
    count came from another tool's receipt) and an omitted count while a stored
    result records a row. A record that omits it for a change with no stored
    result passes and says that nothing was compared.

Exit codes:
  0  valid record, help, or version
  2  invalid command-line usage (--repo is required)
  3  schema unavailable, malformed, incompatible, or uncompilable
  4  instance unavailable or malformed
  5  JSON Schema validation failed
  6  semantic invariant validation failed
  7  the stored review results could not be read (--repo is not a directory, or a
     file named like a stored result is not one), so review_lens_count was not
     checked
`

type recordError struct {
	code int
	err  error
}

func (e *recordError) Error() string { return e.err.Error() }
func (e *recordError) Unwrap() error { return e.err }

func fail(code int, format string, args ...any) error {
	return &recordError{code: code, err: fmt.Errorf(format, args...)}
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("actuals-record-validator", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	schemaPath := flags.String("schema", "", "actuals-record schema file")
	instancePath := flags.String("instance", "", "actuals-record JSON instance")
	repoPath := flags.String("repo", "", "repository root that holds the stored review results of the change")
	showVersion := flags.Bool("version", false, "print validator/record version")
	showHelp := flags.Bool("help", false, "show help")

	if err := flags.Parse(args); err != nil {
		fmt.Fprintf(stderr, "error: %v\n\n%s", err, usageText)
		return exitUsage
	}
	if *showHelp {
		fmt.Fprint(stdout, usageText)
		return exitOK
	}
	if *showVersion {
		fmt.Fprintf(stdout, "actuals-record-validator record-version %s (supported: %s)\n",
			currentRecordVersion, strings.Join(supportedRecordVersions, ", "))
		return exitOK
	}
	if *schemaPath == "" || *instancePath == "" || *repoPath == "" {
		fmt.Fprintf(stderr, "error: --schema, --instance and --repo are all required\n\n%s", usageText)
		return exitUsage
	}

	lensOutcome, err := validate(*schemaPath, *instancePath, *repoPath)
	if err != nil {
		var typed *recordError
		if errors.As(err, &typed) {
			fmt.Fprintf(stderr, "error: %v\n", typed.err)
			return typed.code
		}
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitSemanticValidation
	}
	fmt.Fprintf(stdout, "ok: actuals record satisfies %s and every semantic invariant\n", *schemaPath)
	fmt.Fprintln(stdout, lensOutcome)
	return exitOK
}

// validate runs every check and, for a record that passes them all, returns the sentence that says what the lens
// count check compared, so that a pass is never silent about it.
func validate(schemaPath, instancePath, repoPath string) (string, error) {
	schemaDoc, err := readJSON(schemaPath)
	if err != nil {
		return "", fail(exitSchema, "reading schema %s: %v", schemaPath, err)
	}
	if err := validateSchemaVersion(schemaDoc); err != nil {
		return "", fail(exitSchema, "%v", err)
	}

	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("actuals-record.schema.json", schemaDoc); err != nil {
		return "", fail(exitSchema, "loading schema %s: %v", schemaPath, err)
	}
	compiled, err := compiler.Compile("actuals-record.schema.json")
	if err != nil {
		return "", fail(exitSchema, "compiling schema %s: %v", schemaPath, err)
	}

	instance, err := readJSON(instancePath)
	if err != nil {
		return "", fail(exitInstance, "reading instance %s: %v", instancePath, err)
	}
	if err := compiled.Validate(instance); err != nil {
		return "", fail(exitSchemaValidation, "schema validation failed: %v", err)
	}
	if err := validateSemantics(instance); err != nil {
		return "", fail(exitSemanticValidation, "%v", err)
	}
	return checkLensCount(instance, repoPath)
}

// checkLensCount holds review_lens_count to the rows the stored review results of the change record (own-rdd S13). The
// number is counted by the engine (app.CountLenses) from odd/review-results/<change_name>/ under repo, and the
// validator compares it with the record and never makes a count of its own. It returns the sentence that says what was
// compared, or the refusal: a different count, a count nothing backs, an omitted count while a stored result records a
// row (exit 6), and a check that cannot read the results, which is no pass either (exit 7).
func checkLensCount(instance any, repo string) (string, error) {
	record, err := decodeRecord(instance)
	if err != nil {
		return "", err
	}
	change := record.ChangeName
	info, err := os.Stat(repo)
	if err == nil && !info.IsDir() {
		err = errors.New("it is not a directory")
	}
	if err != nil {
		return "", fail(exitReviewResults, "--repo %s cannot be read as the repository root (%v), so the stored review results of %q are unavailable and review_lens_count cannot be checked", repo, err, change)
	}
	counted, err := app.CountLenses(fsstore.New(), repo, change)
	if err != nil {
		return "", fail(exitReviewResults, "the stored review results of %q under %s cannot be counted, so review_lens_count cannot be checked: %v", change, repo, err)
	}
	dir := review.ResultsDir + "/" + change
	claimed := record.ReviewLensCount
	switch {
	case claimed == nil && counted.Lenses == 0:
		return fmt.Sprintf("review_lens_count: omitted, and no stored review result of %q records a row: there was nothing to compare", change), nil
	case claimed == nil:
		return "", fail(exitSemanticValidation, "review_lens_count is omitted but the stored review results of %q record %d rows (%d stored results in %s): write review_lens_count %d, the number of rows they record, one for each row of each launch",
			change, counted.Lenses, counted.Results, dir, counted.Lenses)
	case *claimed != counted.Lenses && counted.Results == 0:
		return "", fail(exitSemanticValidation, "review_lens_count is %d but the stored review results of %q record %d rows (no stored review result in %s): nothing backs a count for this change, as for one reviewed before the rows were stored, whose count came from another tool's receipt; omit review_lens_count",
			*claimed, change, counted.Lenses, dir)
	case *claimed != counted.Lenses:
		return "", fail(exitSemanticValidation, "review_lens_count is %d but the stored review results of %q record %d rows (%d stored results in %s): it is the number of rows they record, one for each row of each launch; write %d",
			*claimed, change, counted.Lenses, counted.Results, dir, counted.Lenses)
	}
	return fmt.Sprintf("review_lens_count: %d equals the %d rows recorded in %d stored review results of %q", *claimed, counted.Lenses, counted.Results, change), nil
}

func readJSON(path string) (any, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	var document any
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, err
	}
	return document, nil
}

// validateSchemaVersion is this bundle's handshake. The actuals schema carries
// no `properties.contract_version.enum` for the entry-contract validator's
// handshake to read — that absence is exactly why that tool could not be reused
// here — so the version is taken from the schema's own `$id`.
func validateSchemaVersion(document any) error {
	schema, ok := document.(map[string]any)
	if !ok {
		return errors.New("schema root must be an object")
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		return errors.New("$schema must select Draft 2020-12")
	}
	id, ok := schema["$id"].(string)
	if !ok {
		return fmt.Errorf("schema must declare a string $id of the form %s<version>", schemaIDPrefix)
	}
	if !strings.HasPrefix(id, schemaIDPrefix) {
		return fmt.Errorf("schema $id %q is not an actuals-record schema (expected prefix %s)", id, schemaIDPrefix)
	}
	version := strings.TrimPrefix(id, schemaIDPrefix)
	for _, supported := range supportedRecordVersions {
		if version == supported {
			return nil
		}
	}
	return fmt.Errorf("schema declares actuals-record version %q, which this validator does not support (supported: %s)",
		version, strings.Join(supportedRecordVersions, ", "))
}

type actualsRecord struct {
	ChangeName          string   `json:"change_name"`
	Project             string   `json:"project"`
	ImplementationHours *float64 `json:"implementation_hours"`
	ReviewGateHours     *float64 `json:"review_gate_hours"`
	TotalWallClockHours *float64 `json:"total_wall_clock_hours"`
	PostReviewFixHours  *float64 `json:"post_review_fix_hours"`
	ApprovalDecision    string   `json:"approval_decision"`
	ScopeDriftNotes     string   `json:"scope_drift_notes"`
	VarianceVsPlan      string   `json:"variance_vs_plan"`
	RequirementCount    *int     `json:"requirement_count"`
	ChangedLines        *int     `json:"changed_lines"`
	ReviewLensCount     *int     `json:"review_lens_count"`
	CheckpointCount     *int     `json:"checkpoint_count"`
}

// decodeRecord reads the fields the semantic checks use out of a record that already passed the schema.
func decodeRecord(instance any) (actualsRecord, error) {
	data, err := json.Marshal(instance)
	if err != nil {
		return actualsRecord{}, err
	}
	var record actualsRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return actualsRecord{}, err
	}
	return record, nil
}

func validateSemantics(instance any) error {
	record, err := decodeRecord(instance)
	if err != nil {
		return err
	}

	if !kebabCase.MatchString(record.ChangeName) {
		return fmt.Errorf("change_name %q is not kebab-case: the slug is derived once upstream and inherited verbatim, so a reshaped one no longer joins this record to its change", record.ChangeName)
	}
	if strings.TrimSpace(record.Project) == "" {
		return errors.New("project must name the project this record calibrates")
	}
	if err := validateApprovalDecision(record.ApprovalDecision); err != nil {
		return err
	}
	for _, narrative := range []struct {
		field string
		value string
	}{
		{"scope_drift_notes", record.ScopeDriftNotes},
		{"variance_vs_plan", record.VarianceVsPlan},
	} {
		if err := validateSelfContained(narrative.field, narrative.value); err != nil {
			return err
		}
	}
	for _, hours := range []struct {
		field string
		value *float64
	}{
		{"implementation_hours", record.ImplementationHours},
		{"review_gate_hours", record.ReviewGateHours},
		{"total_wall_clock_hours", record.TotalWallClockHours},
		{"post_review_fix_hours", record.PostReviewFixHours},
	} {
		if hours.value != nil && *hours.value < 0 {
			return fmt.Errorf("%s is %g: a duration cannot be negative, and a negative one silently deflates every rate derived from it", hours.field, *hours.value)
		}
	}
	for _, count := range []struct {
		field string
		value *int
	}{
		{"requirement_count", record.RequirementCount},
		{"changed_lines", record.ChangedLines},
		{"review_lens_count", record.ReviewLensCount},
		{"checkpoint_count", record.CheckpointCount},
	} {
		if count.value != nil && *count.value < 0 {
			return fmt.Errorf("%s is %d: a count cannot be negative", count.field, *count.value)
		}
	}

	variance := strings.ToLower(record.VarianceVsPlan)
	if record.CheckpointCount != nil {
		// R-006/R-007: the record carries one checkpoint total and discloses its
		// provenance split in prose, because no structured field distinguishes a
		// durably-observed checkpoint from one reconstructed after the fact. A
		// total with no split is a number a reader cannot weigh.
		//
		// Both halves are matched on their STEM, because a rule may not reject
		// the wording the writer instruction prescribes. skills/inception-
		// pipeline/SKILL.md tells closure-feedback to say which checkpoints were
		// "durably observed (via pipeline-state) versus reconstructed from the
		// closure narrative" — and "durably" does not contain "durable", so the
		// exact-substring form rejected records written to the letter of the
		// instruction it exists to enforce. `durabl` covers durable/durably and
		// `reconstruct` covers reconstructed/reconstruction/reconstructing.
		if !strings.Contains(variance, durableStem) || !strings.Contains(variance, reconstructedStem) {
			return errors.New("checkpoint_count is recorded but variance_vs_plan does not disclose the durable-vs-reconstructed split (R-006/R-007): state which units were durably observed via pipeline-state and which were reconstructed from the closure narrative, explicitly stating zero when none were reconstructed")
		}
	}
	if record.TotalWallClockHours != nil {
		// R-014: a reconstructed elapsed-time figure presented as if measured is
		// the exact fabrication this instrument exists to remove from its own
		// numbers, so the record must say which it is.
		if !strings.Contains(variance, "measured") && !strings.Contains(variance, reconstructedStem) {
			return errors.New("total_wall_clock_hours is recorded but variance_vs_plan does not state whether it was measured or reconstructed (R-014): an unlabelled elapsed-time figure reads as measured even when it is a best-estimate reconstruction")
		}
	}
	return nil
}

func validateApprovalDecision(value string) error {
	normalized := strings.ToLower(strings.TrimSpace(value))
	for _, accepted := range approvalDecisions {
		if normalized == accepted {
			return nil
		}
	}
	return fmt.Errorf("approval_decision %q is outside the closed outcome vocabulary (%s): a free-text sentence here is not an outcome a calibration reader can classify, and pointer text is not an outcome at all",
		value, strings.Join(approvalDecisions, ", "))
}

// validateSelfContained enforces that a required narrative carries its own
// content. Two distinct failures, reported distinctly: nothing was written, and
// something was written that points at where the content actually lives.
func validateSelfContained(field, value string) error {
	trimmed := strings.TrimSpace(value)
	normalized := strings.ToLower(trimmed)
	for _, placeholder := range placeholderNarratives {
		if normalized == placeholder {
			return fmt.Errorf("%s is %q: a required narrative field with a placeholder value records nothing — write what happened, or state plainly that nothing did and why that is the whole story", field, value)
		}
	}
	if len([]rune(trimmed)) < minNarrativeRunes {
		return fmt.Errorf("%s is %d characters (%q): below the %d-character floor for a required narrative, which is the point at which the field has stopped carrying information a later reader can act on",
			field, len([]rune(trimmed)), trimmed, minNarrativeRunes)
	}
	for _, marker := range deferralMarkers {
		if strings.Contains(normalized, marker) {
			return fmt.Errorf("%s defers its own content elsewhere (%q): JSON Schema counts a pointer as a satisfied required field, but a reader who follows the pointer to a section that was itself never written finds nothing. Write the content here; cross-reference in ADDITION to it, never INSTEAD of it",
				field, marker)
		}
	}
	return nil
}
