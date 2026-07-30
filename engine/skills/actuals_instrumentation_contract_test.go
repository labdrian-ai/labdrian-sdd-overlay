package skills

import (
	"encoding/json"
	"errors"
	"fmt"
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
		// closed. Absent, null, non-boolean and inverted are distinct regressions, so they
		// fail distinctly; the raw-message decode is what makes absence detectable at all
		// (a plain bool field would read as a passing `false` when the flag is simply gone).
		// The four-way classification itself is pure and lives in classifyClosedRecordFlag,
		// which TestActualsInstrumentationGateHelpers exercises across every input shape —
		// the failure branches below are unreachable from committed content by design.
		assertClosedRecordFlag(t, schema.AdditionalProperties)

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

// closedRecordFlagClassification enumerates every distinguishable outcome of reading the
// schema's additionalProperties flag. Only closedRecordFlagClosed proves the record closed;
// the other four are separate regressions with separate causes, so they must stay separately
// detectable rather than collapsing into one generic "flag is wrong".
type closedRecordFlagClassification int

const (
	closedRecordFlagClosed closedRecordFlagClassification = iota
	closedRecordFlagAbsent
	closedRecordFlagNull
	closedRecordFlagNotBoolean
	closedRecordFlagInverted
)

func (c closedRecordFlagClassification) String() string {
	switch c {
	case closedRecordFlagClosed:
		return "closed"
	case closedRecordFlagAbsent:
		return "absent"
	case closedRecordFlagNull:
		return "null"
	case closedRecordFlagNotBoolean:
		return "non-boolean"
	case closedRecordFlagInverted:
		return "inverted"
	default:
		return fmt.Sprintf("closedRecordFlagClassification(%d)", int(c))
	}
}

// closedRecordFlagError carries the classification as a typed value so callers — including
// the table test — branch on Classification instead of matching failure-message substrings.
type closedRecordFlagError struct {
	Classification closedRecordFlagClassification
	Cause          error
}

func (e *closedRecordFlagError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("additionalProperties is %s: %v", e.Classification, e.Cause)
	}
	return fmt.Sprintf("additionalProperties is %s", e.Classification)
}

func (e *closedRecordFlagError) Unwrap() error { return e.Cause }

// classifyClosedRecordFlag is the pure core of the closed-record assertion: it returns nil
// only for the literal boolean false and otherwise a *closedRecordFlagError naming which
// regression occurred. Being pure is the point — the t.Fatalf wrapper below cannot be
// exercised without failing its own test, so the four failure branches would otherwise be
// dead code that a revert could silently delete.
//
// Decoding through *bool rather than bool is what keeps a JSON null distinguishable:
// unmarshalling null into a plain bool is a no-op that returns no error, so it would read as
// a passing `false` and leave the record unprovably closed.
func classifyClosedRecordFlag(raw json.RawMessage) error {
	if len(raw) == 0 {
		return &closedRecordFlagError{Classification: closedRecordFlagAbsent}
	}
	var closedRecord *bool
	if err := json.Unmarshal(raw, &closedRecord); err != nil {
		return &closedRecordFlagError{Classification: closedRecordFlagNotBoolean, Cause: err}
	}
	if closedRecord == nil {
		return &closedRecordFlagError{Classification: closedRecordFlagNull}
	}
	if *closedRecord {
		return &closedRecordFlagError{Classification: closedRecordFlagInverted}
	}
	return nil
}

// assertClosedRecordFlag is the thin t-taking wrapper around classifyClosedRecordFlag. It
// reproduces exactly the four failure messages the D2_D5_closed_schema_invariant group has
// always emitted, so extracting the classifier changed no observable gate behaviour.
func assertClosedRecordFlag(t *testing.T, raw json.RawMessage) {
	t.Helper()
	err := classifyClosedRecordFlag(raw)
	if err == nil {
		return
	}
	// The errors.As guard and the switch default are exhaustiveness guards, not branches any
	// input can reach: classifyClosedRecordFlag returns nil or *closedRecordFlagError, and the
	// four cases below cover every classification it attaches to an error. They exist so that
	// adding a fifth classification later still fails loudly instead of passing silently.
	var flagErr *closedRecordFlagError
	if !errors.As(err, &flagErr) {
		t.Fatalf("schema additionalProperties must be the boolean false, got %s: %v", raw, err)
	}
	switch flagErr.Classification {
	case closedRecordFlagAbsent:
		t.Fatal("schema must declare additionalProperties: the closed-record flag is absent")
	case closedRecordFlagNotBoolean:
		t.Fatalf("schema additionalProperties must be the boolean false, got %s: %v", raw, flagErr.Cause)
	case closedRecordFlagNull:
		t.Fatalf("schema additionalProperties must be the boolean false, got %s: a null flag does not close the record", raw)
	case closedRecordFlagInverted:
		t.Fatal("schema additionalProperties must be false, got true: the record is no longer closed")
	default:
		t.Fatalf("schema additionalProperties must be the boolean false, got %s: %v", raw, err)
	}
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

// Sentinel errors returned by the pure markdown-slicing cores. The t-taking wrappers map
// each one back to the exact t.Fatalf message the gate has always emitted, and the table
// tests assert on these sentinels rather than on message text.
var (
	errMarkdownHeadingAnchorInvalid = errors.New("heading anchor is not '#'-prefixed markdown heading text")
	errMarkdownSectionNotFound      = errors.New("section heading not found")
	errMarkdownMarkerNotFound       = errors.New("no line contains the marker")
	errMarkdownMarkerNotInListItem  = errors.New("marker is not inside a top-level list item")
)

// normaliseLineEndings rewrites all three line-ending conventions to LF, so an encoding-only
// re-save cannot break line splitting or heading matching and take the whole gate down on a
// change that altered no visible character.
//
// CRLF is collapsed first and lone CR second: doing lone CR first would rewrite the CR of
// every CRLF pair and turn each one into a blank line. As a consequence a "\r\r\n" sequence
// becomes two newlines — the residual CR is gone, which is what matters, but the pathological
// double-CR encoding legitimately gains a blank line.
//
// Normalisation is applied for line splitting and matching only. Section bodies and list
// items are re-joined with "\n", so a carriage return that was genuine content comes back as
// a newline in the returned slice. That is harmless here because every downstream use is
// substring matching on marker text, never byte-exact reproduction of the source.
func normaliseLineEndings(doc string) string {
	return strings.ReplaceAll(strings.ReplaceAll(doc, "\r\n", "\n"), "\r", "\n")
}

// sliceMarkdownSection is the pure core of markdownSectionBody: it returns the body of the
// section introduced by the exact heading line, ending at the next heading of the same or
// higher level. It returns an error instead of failing a test so the not-found path is
// reachable from an executable test; returning errMarkdownSectionNotFound rather than "" is
// load-bearing, because an empty body would make every assertion scoped to the section pass
// vacuously.
func sliceMarkdownSection(doc, heading string) (string, error) {
	level := markdownHeadingLevel(heading)
	if level == 0 {
		return "", errMarkdownHeadingAnchorInvalid
	}

	lines := strings.Split(normaliseLineEndings(doc), "\n")
	start := -1
	inFence := false
	for i, line := range lines {
		if isMarkdownFence(line) {
			inFence = !inFence
			continue
		}
		// "\r" belongs in the cutset as defence in depth, not because it is reachable from
		// here: normaliseLineEndings above already removed every carriage return. It matters
		// only if a future caller reaches this comparison with un-normalised lines.
		if !inFence && strings.TrimRight(line, " \t\r") == heading {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return "", errMarkdownSectionNotFound
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
			return strings.Join(lines[start:i], "\n"), nil
		}
	}
	return strings.Join(lines[start:], "\n"), nil
}

// markdownSectionBody is the thin t-taking wrapper around sliceMarkdownSection. A missing
// heading is a hard failure so a renamed section surfaces instead of silently passing an
// empty scope.
func markdownSectionBody(t *testing.T, doc, heading string) string {
	t.Helper()
	body, err := sliceMarkdownSection(doc, heading)
	switch {
	case err == nil:
		return body
	case errors.Is(err, errMarkdownHeadingAnchorInvalid):
		t.Fatalf("heading anchor %q must be '#'-prefixed markdown heading text", heading)
	case errors.Is(err, errMarkdownSectionNotFound):
		t.Fatalf("section heading %q not found — was the section renamed or removed?", heading)
	default:
		t.Fatalf("slicing section %q: %v", heading, err)
	}
	return ""
}

// sliceMarkdownListItem is the pure core of markdownListItemContaining: it returns the single
// top-level list item of section (including its indented continuation lines) whose text
// contains marker, or an error naming which lookup failed.
func sliceMarkdownListItem(section, marker string) (string, error) {
	lines := strings.Split(normaliseLineEndings(section), "\n")
	hit := -1
	for i, line := range lines {
		if strings.Contains(line, marker) {
			hit = i
			break
		}
	}
	if hit < 0 {
		return "", errMarkdownMarkerNotFound
	}

	start := hit
	for start >= 0 && !isMarkdownListItemStart(lines[start]) {
		start--
	}
	if start < 0 {
		return "", errMarkdownMarkerNotInListItem
	}

	end := start + 1
	for end < len(lines) && !isMarkdownListItemStart(lines[end]) && strings.TrimSpace(lines[end]) != "" {
		end++
	}
	return strings.Join(lines[start:end], "\n"), nil
}

// markdownListItemContaining is the thin t-taking wrapper around sliceMarkdownListItem.
// sectionName only labels failure messages.
func markdownListItemContaining(t *testing.T, section, sectionName, marker string) string {
	t.Helper()
	item, err := sliceMarkdownListItem(section, marker)
	switch {
	case err == nil:
		return item
	case errors.Is(err, errMarkdownMarkerNotFound):
		t.Fatalf("section %s contains no line with marker %q", sectionName, marker)
	case errors.Is(err, errMarkdownMarkerNotInListItem):
		t.Fatalf("marker %q in section %s is not inside a top-level list item", marker, sectionName)
	default:
		t.Fatalf("slicing list item %q in section %s: %v", marker, sectionName, err)
	}
	return ""
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

// lineEndingFixture is one logical document used to prove the gate's markdown helpers are
// insensitive to line-ending convention. Written with LF; the table re-encodes it.
const lineEndingFixture = "# Doc\n" +
	"## Hard Rules\n" +
	"- R-001/R-002 keep the three units separate\n" +
	"  never summed or averaged into one figure\n" +
	"- unrelated rule\n" +
	"\n" +
	"## Output Contract\n" +
	"- trailing item\n"

// mixLineEndings re-terminates each line with a different convention in rotation, so a
// single document exercises LF, CRLF and lone CR simultaneously.
func mixLineEndings(doc string) string {
	terminators := []string{"\n", "\r\n", "\r"}
	lines := strings.Split(doc, "\n")
	var b strings.Builder
	for i, line := range lines[:len(lines)-1] {
		b.WriteString(line)
		b.WriteString(terminators[i%len(terminators)])
	}
	b.WriteString(lines[len(lines)-1])
	return b.String()
}

// TestActualsInstrumentationGateHelpers exercises the pure cores behind
// TestActualsInstrumentationContract. Those cores' failure and normalisation branches are
// unreachable from committed repository content — the schema declares additionalProperties
// false and no pinned document contains a carriage-return byte — so without this function
// reverting the pointer decode or the line-ending normalisation would leave the suite green.
// It is a separate top-level function on purpose: external evidence records cite the ten
// t.Run group names of TestActualsInstrumentationContract, so that list must not grow.
func TestActualsInstrumentationGateHelpers(t *testing.T) {
	t.Run("closed_record_flag_classification", func(t *testing.T) {
		tests := []struct {
			name string
			raw  json.RawMessage
			want closedRecordFlagClassification
		}{
			{name: "boolean false is the only closed record", raw: json.RawMessage("false"), want: closedRecordFlagClosed},
			{name: "absent flag leaves the record unprovably closed", raw: nil, want: closedRecordFlagAbsent},
			{name: "null flag does not close the record", raw: json.RawMessage("null"), want: closedRecordFlagNull},
			{name: "boolean true inverts the flag", raw: json.RawMessage("true"), want: closedRecordFlagInverted},
			{name: "object value is not a boolean", raw: json.RawMessage(`{"type":"string"}`), want: closedRecordFlagNotBoolean},
			{name: "quoted string value is not a boolean", raw: json.RawMessage(`"false"`), want: closedRecordFlagNotBoolean},
			{name: "number value is not a boolean", raw: json.RawMessage("0"), want: closedRecordFlagNotBoolean},
			{name: "array value is not a boolean", raw: json.RawMessage("[false]"), want: closedRecordFlagNotBoolean},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := classifyClosedRecordFlag(tt.raw)
				if tt.want == closedRecordFlagClosed {
					if err != nil {
						t.Fatalf("classifyClosedRecordFlag(%s) error = %v, want nil", tt.raw, err)
					}
					return
				}
				var flagErr *closedRecordFlagError
				if !errors.As(err, &flagErr) {
					t.Fatalf("classifyClosedRecordFlag(%s) error = %v, want *closedRecordFlagError with classification %s", tt.raw, err, tt.want)
				}
				if flagErr.Classification != tt.want {
					t.Fatalf("classifyClosedRecordFlag(%s) classification = %s, want %s", tt.raw, flagErr.Classification, tt.want)
				}
			})
		}
	})

	t.Run("line_ending_conventions_are_equivalent", func(t *testing.T) {
		// The expectations are written out literally rather than derived from an LF run, so a
		// break in the LF path cannot quietly redefine what the other encodings are compared to.
		const wantBody = "- R-001/R-002 keep the three units separate\n" +
			"  never summed or averaged into one figure\n" +
			"- unrelated rule\n"
		const wantItem = "- R-001/R-002 keep the three units separate\n" +
			"  never summed or averaged into one figure"

		tests := []struct {
			name string
			doc  string
		}{
			{name: "lf endings", doc: lineEndingFixture},
			{name: "crlf endings", doc: strings.ReplaceAll(lineEndingFixture, "\n", "\r\n")},
			{name: "lone cr endings", doc: strings.ReplaceAll(lineEndingFixture, "\n", "\r")},
			{name: "mixed endings", doc: mixLineEndings(lineEndingFixture)},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				body, err := sliceMarkdownSection(tt.doc, timeEstimationHardRulesHeading)
				if err != nil {
					t.Fatalf("sliceMarkdownSection() error = %v, want nil", err)
				}
				if body != wantBody {
					t.Fatalf("sliceMarkdownSection() = %q, want %q", body, wantBody)
				}
				item, err := sliceMarkdownListItem(body, "R-001/R-002")
				if err != nil {
					t.Fatalf("sliceMarkdownListItem() error = %v, want nil", err)
				}
				if item != wantItem {
					t.Fatalf("sliceMarkdownListItem() = %q, want %q", item, wantItem)
				}
			})
		}
	})

	t.Run("double_carriage_return_leaves_no_residual_cr", func(t *testing.T) {
		// "\r\r\n" is the second half of the review finding: a single CRLF replacement pass
		// leaves a residual "\r" on the line, which breaks the heading comparison. Full
		// normalisation removes it — but because CRLF collapses first, the surviving lone CR
		// becomes its own newline, so this encoding legitimately gains a blank line between
		// every pair of source lines. Byte equality with the LF baseline is therefore not the
		// property to assert; the properties that matter are that no carriage return survives
		// and that the heading and marker are still located instead of the gate dying on a
		// not-found fatal.
		doc := strings.ReplaceAll(lineEndingFixture, "\n", "\r\r\n")

		body, err := sliceMarkdownSection(doc, timeEstimationHardRulesHeading)
		if err != nil {
			t.Fatalf("sliceMarkdownSection() error = %v, want nil", err)
		}
		if strings.Contains(body, "\r") {
			t.Fatalf("sliceMarkdownSection() = %q, want no residual carriage return", body)
		}
		if !strings.Contains(body, "- R-001/R-002 keep the three units separate") {
			t.Fatalf("sliceMarkdownSection() = %q, want it to contain the R-001/R-002 rule line", body)
		}

		item, err := sliceMarkdownListItem(body, "R-001/R-002")
		if err != nil {
			t.Fatalf("sliceMarkdownListItem() error = %v, want nil", err)
		}
		if item != "- R-001/R-002 keep the three units separate" {
			t.Fatalf("sliceMarkdownListItem() = %q, want the marker line", item)
		}
	})

	t.Run("section_slicing", func(t *testing.T) {
		tests := []struct {
			name    string
			doc     string
			heading string
			want    string
			wantErr error
		}{
			{
				name:    "missing heading is an error not an empty section",
				doc:     "# Doc\n## Other Section\nbody\n",
				heading: timeEstimationHardRulesHeading,
				wantErr: errMarkdownSectionNotFound,
			},
			{
				name:    "heading anchor without a level is rejected",
				doc:     "# Doc\nHard Rules\nbody\n",
				heading: "Hard Rules",
				wantErr: errMarkdownHeadingAnchorInvalid,
			},
			{
				name:    "fenced heading-like line does not start a section",
				doc:     "# Doc\n```\n## Hard Rules\nfenced body\n```\n## Hard Rules\nreal body\n",
				heading: timeEstimationHardRulesHeading,
				want:    "real body\n",
			},
			{
				name:    "fenced heading-like line does not terminate a section",
				doc:     "## Hard Rules\nbody\n```\n## Output Contract\n```\ntail\n## Output Contract\nafter\n",
				heading: timeEstimationHardRulesHeading,
				want:    "body\n```\n## Output Contract\n```\ntail",
			},
			{
				name:    "last section in the document runs to end of file",
				doc:     "# Doc\n## Hard Rules\nbody\nmore body\n",
				heading: timeEstimationHardRulesHeading,
				want:    "body\nmore body\n",
			},
			{
				name:    "terminates at the next heading of the same level",
				doc:     "## Hard Rules\nbody\n## Output Contract\nafter\n",
				heading: timeEstimationHardRulesHeading,
				want:    "body",
			},
			{
				name:    "terminates at a higher-level heading",
				doc:     "## Hard Rules\nbody\n# Title\nafter\n",
				heading: timeEstimationHardRulesHeading,
				want:    "body",
			},
			{
				name:    "does not terminate at a deeper heading",
				doc:     "## Hard Rules\nbody\n### Subsection\nsub body\n## Output Contract\nafter\n",
				heading: timeEstimationHardRulesHeading,
				want:    "body\n### Subsection\nsub body",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := sliceMarkdownSection(tt.doc, tt.heading)
				if tt.wantErr != nil {
					if !errors.Is(err, tt.wantErr) {
						t.Fatalf("sliceMarkdownSection() error = %v, want %v", err, tt.wantErr)
					}
					if got != "" {
						t.Fatalf("sliceMarkdownSection() = %q on error, want empty", got)
					}
					return
				}
				if err != nil {
					t.Fatalf("sliceMarkdownSection() error = %v, want nil", err)
				}
				if got != tt.want {
					t.Fatalf("sliceMarkdownSection() = %q, want %q", got, tt.want)
				}
			})
		}
	})

	t.Run("list_item_extraction", func(t *testing.T) {
		tests := []struct {
			name    string
			section string
			marker  string
			want    string
			wantErr error
		}{
			{
				name:    "item with indented continuation lines is returned whole",
				section: "- first item\n- R-001/R-002 rule\n  continuation one\n  continuation two\n- third item\n",
				marker:  "R-001/R-002",
				want:    "- R-001/R-002 rule\n  continuation one\n  continuation two",
			},
			{
				name:    "ordered list item is recognised",
				section: "1. first\n6. Actuals and Calibration line\n   continuation\n7. next\n",
				marker:  "Actuals and Calibration",
				want:    "6. Actuals and Calibration line\n   continuation",
			},
			{
				name:    "marker present in no item is an error not an empty item",
				section: "- first item\n- second item\n",
				marker:  "R-001/R-002",
				wantErr: errMarkdownMarkerNotFound,
			},
			{
				name:    "marker outside any list item is an error",
				section: "prose mentioning R-001/R-002\n- first item\n",
				marker:  "R-001/R-002",
				wantErr: errMarkdownMarkerNotInListItem,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := sliceMarkdownListItem(tt.section, tt.marker)
				if tt.wantErr != nil {
					if !errors.Is(err, tt.wantErr) {
						t.Fatalf("sliceMarkdownListItem() error = %v, want %v", err, tt.wantErr)
					}
					if got != "" {
						t.Fatalf("sliceMarkdownListItem() = %q on error, want empty", got)
					}
					return
				}
				if err != nil {
					t.Fatalf("sliceMarkdownListItem() error = %v, want nil", err)
				}
				if got != tt.want {
					t.Fatalf("sliceMarkdownListItem() = %q, want %q", got, tt.want)
				}
			})
		}
	})
}
