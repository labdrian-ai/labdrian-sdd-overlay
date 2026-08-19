package skills

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
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
			"from the tiering go-ahead checkpoint to merge",
			"including interruption gaps",
		} {
			if !strings.Contains(wallClock, want) {
				t.Fatalf("total_wall_clock_hours description must contain %q, got %q", want, wallClock)
			}
		}

		// checkpoint_count shares total_wall_clock_hours' boundary (R-003/R-004/R-005), so the
		// merge anchor must move on both descriptions together, not only on the field whose
		// name mentions "clock".
		checkpointDesc := actualsSchemaPropertyDescription(t, schema, "checkpoint_count")
		if !strings.Contains(checkpointDesc, "tiering-go-ahead-to-merge boundary") {
			t.Fatalf("checkpoint_count description must contain %q, got %q", "tiering-go-ahead-to-merge boundary", checkpointDesc)
		}

		// The FORBID stays whole-file on purpose. For a negative assertion, whole-file
		// scope is strictly stronger than description scope: the retired boundary phrase
		// must not reappear anywhere, including on checkpoint_count, which now shares the
		// tiering-go-ahead-to-merge boundary and could regress the same way.
		for _, forbidden := range []string{
			"from first apply to archive",
			"checkpoint to archive",
			"tiering-go-ahead-to-archive boundary",
		} {
			if strings.Contains(schemaRaw, forbidden) {
				t.Fatalf("schema must not retain the old temporal boundary phrase %q", forbidden)
			}
		}
	})

	t.Run("R021_required_drops_wall_clock", func(t *testing.T) {
		schemaRaw := readRepoFile(t, repoRoot, actualsSchemaRelPath)
		schema := decodeActualsSchema(t, schemaRaw)

		if slices.Contains(schema.Required, "total_wall_clock_hours") {
			t.Fatalf("schema required list must not contain %q (R-021), got %v", "total_wall_clock_hours", schema.Required)
		}
		// R-021 relaxes which fields are mandatory, not which fields exist: the property
		// itself must still be declared, only demoted out of `required`.
		if _, ok := schema.Properties["total_wall_clock_hours"]; !ok {
			t.Fatal("schema must still declare property total_wall_clock_hours (R-021 removes it from required only, not from properties)")
		}

		// The schema relaxation alone does not prove closure-feedback actually WRITES the
		// record when total_wall_clock_hours is unresolved (spec.md scenarios "A cycle
		// missing t0 still produces a usable record" and "Neither anchor resolves": "every
		// other resolved field is still written"). This shipped behavioral clause in
		// SKILL.md's Validate step was entirely unpinned before this batch: deleting it, or
		// inverting it to the all-or-nothing rule R-021 exists to abolish, left the whole
		// suite green (verify round 4, W1). Pinned at its unique site — both substrings
		// occur exactly once in the file.
		skill := readRepoFile(t, repoRoot, inceptionPipelineSkillRelPath)
		for _, want := range []string{
			"`total_wall_clock_hours` is validly absent when its anchors do not resolve (R-021)",
			"every other resolved field is still required",
		} {
			if !strings.Contains(skill, want) {
				t.Fatalf("closure-feedback Validate step must contain %q (R-021 write-anyway clause)", want)
			}
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
		// The REQUIRE pins are scoped to the CALIBRATION list item itself, matching
		// R019_calibration_absent_numerators_fallback below. A positive pin checked
		// whole-file is satisfied by the same wording appearing anywhere in the
		// document, so relocating a clause out of the CALIBRATION rule into an
		// unrelated section would keep this gate green while the rule it guards no
		// longer says it.
		hardRules := markdownSectionBody(t, skill, timeEstimationHardRulesHeading)
		rule := markdownListItemContaining(t, hardRules, timeEstimationHardRulesHeading, "CALIBRATION")
		for _, want := range []string{
			"NEVER an input to the agent-compute-time baseline",
			"interruption-clean residual samples with positive `checkpoint_count`",
			"`(total_wall_clock_hours − sum(phase hours)) / checkpoint_count`",
			"Exclude any residual known or narrated to include a session, rate-limit, or provider interruption",
			"never subtract a guessed interruption amount",
			// D4: boundary co-move, archive → merge (task 4.1). Previously
			// unpinned entirely — reverting "merge" to "archive" on this line
			// left the whole suite green (verify round 3, W4).
			"tiering-go-ahead to merge, including interruption gaps",
		} {
			if !strings.Contains(rule, want) {
				t.Fatalf("%s CALIBRATION rule must contain %q, got %q", timeEstimationHardRulesHeading, want, rule)
			}
		}
		// The FORBID stays whole-file on purpose, for the same reason the schema
		// FORBID above does: for a negative assertion, whole-file scope is strictly
		// stronger than list-item scope. The retired boundary phrase must not
		// reappear ANYWHERE in the skill, not merely outside this one rule.
		for _, forbidden := range []string{
			"and `total_wall_clock_hours`, build a per-phase agent-compute-time baseline",
			"tiering-go-ahead to archive",
		} {
			if strings.Contains(skill, forbidden) {
				t.Fatalf("CALIBRATION rule must not retain %q", forbidden)
			}
		}
	})

	t.Run("R019_calibration_absent_numerators_fallback", func(t *testing.T) {
		// R-019 mandates implementation_hours/review_gate_hours/post_review_fix_hours stay
		// unpopulated until a durable source exists, which starves this skill's own compute-time
		// numerators — the CALIBRATION rule builds its baseline from exactly these three fields
		// "ONLY" (R009 above). Scoped to the CALIBRATION list item itself, not the whole file, so
		// a substring hit elsewhere in Hard Rules cannot satisfy this gate.
		skill := readRepoFile(t, repoRoot, timeEstimationSkillRelPath)
		hardRules := markdownSectionBody(t, skill, timeEstimationHardRulesHeading)
		rule := markdownListItemContaining(t, hardRules, timeEstimationHardRulesHeading, "CALIBRATION")
		for _, want := range []string{
			"WHEN one or more of these three fields is absent under R-019",
			"the record supplies NO compute-time numerator for the affected phase",
			"never substitute `total_wall_clock_hours` or any other elapsed-time figure in its place",
			"the per-unit compute-time rate cannot be computed, so confidence falls back to the disclosed bootstrap defaults / qualitative scaling regardless of calibration `n`",
		} {
			if !strings.Contains(rule, want) {
				t.Fatalf("%s CALIBRATION rule must contain %q, got %q", timeEstimationHardRulesHeading, want, rule)
			}
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

	t.Run("R016_R017_R018_closure_feedback_anchor_prose", func(t *testing.T) {
		skill := readRepoFile(t, repoRoot, inceptionPipelineSkillRelPath)
		for _, want := range []string{
			// D4: boundary co-move, archive → merge.
			"through merge",
			// D2 REVISION 3: the anchor is recorded in a versioned artifact at
			// delivery — landing_commit + approved_tree — and is verifiable
			// rather than merely asserted.
			"landing_commit",
			"approved_tree",
			"verifiable rather than merely asserted",
			// D2 REVISION 3: a mis-recorded anchor is rejected, not trusted
			// (R-017 scenario "A mis-recorded anchor is rejected, not trusted").
			// Pinned as the operative rejection clause rather than a bare word,
			// because a bare pin cannot distinguish the RULE from any sentence
			// that merely names the outcome. Note the case-collision rationale
			// this comment used to give was wrong: strings.Contains is
			// case-sensitive, uppercase "REJECTED" occurs exactly once in the
			// shipped prose, and the "verified, self-asserted, rejected, or
			// absent" sentence spells it lowercase, so it could never have
			// satisfied a bare uppercase pin. The operative-clause pin is still
			// the right shape — it binds the rule's own wording, not the
			// vocabulary — only the stated reason needed correcting.
			"the anchor is REJECTED, t1 is omitted, and the mismatch is disclosed in `variance_vs_plan`",
			// D2 REVISION 3: the exact verification command, previously unpinned
			// entirely — the design's Threat Matrix claimed RED coverage for the
			// shipped git command shape that did not exist (verify round 3, W3).
			"`git show -s --format=%T <landing_commit>` MUST equal the recorded `approved_tree`",
			// D2 REVISION 3: a tree verifies a commit, it never discovers one.
			// Pin the operative clause, not the bare word "earliest": that word
			// survives whether the rule is stated or negated, so pinning it alone
			// is vacuous — it passed unchanged while the rule was inverted.
			"**verify** a commit, never to **discover** one",
			// A shared tree cannot identify a landing commit; it is omitted, not guessed.
			"the anchor is ambiguous",
			// Phase 13 (orchestrator finding, verify round 6 W2): `approved_tree`
			// was previously synthesized from `landing_commit`'s own tree when no
			// review ran, making the mandated check compare a value against
			// itself — structurally unfailable, so reporting it as "verified" was
			// a fabricated assurance. `approved_tree` is now recorded ONLY when
			// an independent receipt exists. Pinned as the operative rule clause,
			// not a bare word, so the pin cannot survive the tautology's return.
			"`approved_tree` MUST NOT be recorded at all",
			// Phase 13: a no-review anchor still measures — it is USED, just
			// never independently checked — recorded as self-asserted, not
			// verified. This is the third outcome state this batch adds.
			"the anchor is recorded as **self-asserted**: used, but with no independent authority to check it against",
			// Phase 13: the resolution-outcome vocabulary itself, so a reader can
			// never mistake a self-asserted anchor for a verified one. Pinned as
			// the full four-way enumeration, not the bare word "self-asserted",
			// so a rewrite that drops one of the other three states also fails.
			"verified, self-asserted, rejected, or absent",
			// D3: the archive-report section both stores must carry. Pinned as the
			// operative Execute-item clause, not the bare heading name: "Cycle
			// Timestamps" alone occurs at two sites (this Execute item and the
			// Gotchas carve-out reference), so deleting the Execute item in full
			// left the bare pin green — mutation-proven non-covering.
			"Append a `## Cycle Timestamps` section to the archived",
			// D1: t0 is read from the typed field, never the rendered footer.
			"typed `created_at` field",
			// D1: engram export selected by topic_key, never a rendered footer parse.
			"engram export",
			// D1: a pipeline-state observation with no topic_key is unresolvable.
			"no `topic_key`",
			// D1b: t0 falls back to the earliest created_at among the change's own
			// observations when no pipeline-state observation exists — a change
			// entered directly at SDD never runs inception-pipeline, and this is the
			// exact path this very change takes at its own closure. Previously
			// unpinned entirely.
			"t0 falls back to the earliest `created_at` among the change",
			// D4: post-merge archive-authorization lag is excluded from
			// checkpoint_count's boundary (R-003/4/5 scenario "Post-merge
			// bookkeeping replies do not count"). Pinned as the operative
			// checkpoint_count clause, not the bare word "bookkeeping": that word
			// also appears on the unrelated total_wall_clock_hours sentence, so the
			// bare pin survived deletion of this clause — mutation-proven
			// non-covering.
			"excluded from this count as post-boundary bookkeeping",
			// D3: the append-only carve-out to closure-feedback's own "do not modify
			// engine outputs" rule. Pinned as the operative Execute-item-3 clause,
			// not the bare phrase "append-only carve-out": that phrase also
			// appears in the unrelated Gotchas cross-reference sentence, so the
			// bare pin survived deletion of the Execute item itself —
			// mutation-proven non-covering (verify round 3, W2).
			"a named, delimited, append-only carve-out to closure-feedback's own \"do NOT modify engine outputs\" rule",
		} {
			if !strings.Contains(skill, want) {
				t.Fatalf("closure-feedback must contain %q", want)
			}
		}
		for _, forbidden := range []string{
			"through archive",
			// D2 REVISION 3 removes the receipt-bound/path-scan two-rule
			// resolution outright — neither name may survive anywhere in the
			// prose, and "first-parent" scanning specifically must not
			// reappear as a t1 resolution mechanism (design.md D2 revision 3,
			// "No heuristic fallback").
			"receipt-bound",
			"path-scan",
			"first-parent",
			// C1 (round 2): the abolished positional/earliest-carrier resolution
			// rule — "the earliest commit on the default branch carrying that
			// tree MUST be chosen" — inverted the ratified "verify, never
			// discover" rule. This is the structural guard work unit 4 adds so
			// the rule cannot silently re-invert: nothing previously bound this
			// FORBID list to positional-resolution language at all.
			"carrying that tree MUST be chosen",
			// Phase 13: the abolished tautological definition of `approved_tree`
			// — "the receipt's final_candidate_tree when review ran... otherwise
			// landing_commit's own tree" — made the mandated verification check
			// compare a value against itself, so it could never fail. Pinned by
			// its exact retired substring so this specific defect class cannot
			// silently return under a differently-worded FORBID list.
			"otherwise `landing_commit`'s own tree",
		} {
			if strings.Contains(skill, forbidden) {
				t.Fatalf("closure-feedback must not retain the abolished D2 phrase %q", forbidden)
			}
		}
	})

	t.Run("R019_compute_time_deferral_rationale", func(t *testing.T) {
		// R-019: the three compute-time fields' deferral reason must be stated
		// in shipped documentation, not only in this change's own proposal/spec
		// (verify's C3 finding: the rationale existed nowhere else).
		skill := readRepoFile(t, repoRoot, inceptionPipelineSkillRelPath)
		for _, want := range []string{
			"implementation_hours",
			"review_gate_hours",
			"post_review_fix_hours",
			"session-transient",
			"no timestamp fields",
			"no structured subagent durations",
		} {
			if !strings.Contains(skill, want) {
				t.Fatalf("closure-feedback must state the R-019 compute-time deferral rationale, missing %q", want)
			}
		}
	})

	t.Run("R019_required_drops_compute_time_fields", func(t *testing.T) {
		// Orchestrator finding (phase 12, human diff review after verify round 6 passed):
		// R-019 says the three compute-time fields "MUST stay unpopulated until a durable
		// source exists" and its own scenario opens "GIVEN a closed record with the three
		// compute-time fields empty" — but all six verify rounds pinned only the rationale
		// prose (R019_compute_time_deferral_rationale above), never that a record actually
		// CONFORMING to R-019 validates against the shipped schema. Before this fix all three
		// fields were still in the schema's `required` list, so an R-019-conforming record was
		// REJECTED by the very schema this change ships, and the Validate rule's own "report
		// and STOP" clause fired on the one thing exercising the instrument end to end (task
		// 6.3, this change's own closure).
		schemaRaw := readRepoFile(t, repoRoot, actualsSchemaRelPath)
		schema := decodeActualsSchema(t, schemaRaw)

		for _, field := range []string{"implementation_hours", "review_gate_hours", "post_review_fix_hours"} {
			if slices.Contains(schema.Required, field) {
				t.Fatalf("schema required list must not contain %q (R-019: no durable source exists yet), got %v", field, schema.Required)
			}
			// Same non-destructive treatment total_wall_clock_hours already received under
			// R-021: `required` is a floor, not a prohibition. The property itself, and any
			// historical record that DOES carry a narrative value (obs #2789, obs #2096, and
			// their committed fixture mirrors), stay valid and untouched.
			if _, ok := schema.Properties[field]; !ok {
				t.Fatalf("schema must still declare property %q (R-019 removes it from required only, not from properties)", field)
			}
		}

		// Binds the rule to the artifact: constructs the exact record R-019's own scenario
		// describes and validates it against the SHIPPED schema read above, not a private
		// copy — so a future re-add to `required` fails THIS assertion, not only the negative
		// pins above (which would keep passing against a schema nobody re-validated a record
		// with — that gap is exactly what six verify rounds missed).
		record := r019ConformingRecordFixture()
		if err := validateAgainstActualsSchema(schema, record); err != nil {
			t.Fatalf("an R-019-conforming record (three compute-time fields absent) must validate against the shipped schema: %v", err)
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

		// Positive exact-list assertion on schema.Required, mirroring the assertStringList
		// guard above for schema.Properties. Verify round 7 (CRITICAL-1) found this list was
		// pinned only by NEGATIVE membership checks (R021_required_drops_wall_clock and
		// R019_required_drops_compute_time_fields each assert one or three specific names are
		// ABSENT); a negative pin cannot detect any OTHER name leaving the list, which is
		// exactly how spec.md's R-021 scenario ("total_wall_clock_hours is the only name
		// removed from required") went silently false when Phase 12 additionally dropped three
		// more names for R-019's separate reason — the suite stayed green throughout. This
		// exact-list assertion closes the class: any name entering OR leaving `required`, for
		// any reason and named by any pin or none, now fails here independently.
		wantRequired := []string{
			"change_name", "project", "approval_decision", "scope_drift_notes", "variance_vs_plan",
		}
		assertStringList(t, schema.Required, wantRequired)

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

// validateAgainstActualsSchema is a minimal closed-schema validator over the decoded actuals
// schema: every name in schema.Required MUST be present in record, and — because the schema
// declares additionalProperties: false — every key in record MUST be declared in
// schema.Properties. It exists so a test can prove a candidate record actually VALIDATES,
// not merely that a name is absent from the required list; the two are not the same claim
// (a record can omit a field the schema still requires and just never get tested against
// it). Pure and t-free so it can be table-tested directly, in the same style as
// classifyClosedRecordFlag above.
func validateAgainstActualsSchema(schema actualsSchemaDocument, record map[string]any) error {
	for _, required := range schema.Required {
		if _, ok := record[required]; !ok {
			return fmt.Errorf("required property %q is missing from record", required)
		}
	}
	for key := range record {
		if _, ok := schema.Properties[key]; !ok {
			return fmt.Errorf("record property %q is not declared in schema (additionalProperties: false)", key)
		}
	}
	return nil
}

// r019ConformingRecordFixture returns a record satisfying R-019's own scenario ("a closed
// record with the three compute-time fields empty"): every field the schema still requires
// after this fix is present, and implementation_hours/review_gate_hours/post_review_fix_hours
// — the three fields R-019 says MUST stay unpopulated until a durable source exists — are
// absent, exactly as the requirement mandates.
func r019ConformingRecordFixture() map[string]any {
	return map[string]any{
		"change_name":       "example-change",
		"project":           "example-project",
		"approval_decision": "approved",
		"scope_drift_notes": "none",
		"variance_vs_plan":  "compute-time fields deferred per R-019: no durable source exists yet",
	}
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

// markdownHeadingLineMatches reports whether line is the heading line for heading, ignoring
// trailing spaces, tabs and carriage returns.
//
// "\r" is in the cutset as defence in depth for its only production caller: by the time
// sliceMarkdownSection reaches this predicate, normaliseLineEndings has already removed every
// carriage return, so no document shape can drive that byte of the cutset from there. It is a
// separate function precisely so the cutset stops being unpinned — the heading_line_matching
// table calls it directly, bypassing normalisation, so deleting "\r" from the cutset now turns
// a case red instead of leaving the suite green.
func markdownHeadingLineMatches(line, heading string) bool {
	return strings.TrimRight(line, " \t\r") == heading
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
		if !inFence && markdownHeadingLineMatches(line, heading) {
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

// lineEndingFixtureTerminators names the terminator of every lineEndingFixture line
// explicitly and in order, so the mixed-endings encoding is a property of the line rather
// than of the line's position. The list must stay parallel to the fixture above; adding a
// fixture line without adding its terminator here is a fixture error that mixLineEndings
// reports as such.
//
// The rotation this replaced derived each terminator from the line's index, which made a
// benign fixture edit dangerous. Inserting a single line shifted the blank separator to an
// index that took a "\n" terminator while the line above it took a lone "\r". The separator
// carries no text of its own, so those two bytes landed adjacent as a "\r\n" pair, which
// collapses to one newline in normaliseLineEndings' first pass. The separator vanished, the
// multi-line list item was truncated where it used to end, and only the mixed-endings case
// went red — with a message blaming the section slicer for a fixture defect.
//
// All three conventions must stay represented, otherwise the mixed case stops being mixed.
var lineEndingFixtureTerminators = []string{
	"\n",   // # Doc
	"\r\n", // ## Hard Rules
	"\r",   // - R-001/R-002 keep the three units separate
	"\n",   //   never summed or averaged into one figure
	"\r\n", // - unrelated rule
	"\r",   // blank separator — one of the two lone-CR entries; mixLineEndings enforces the rest
	"\n",   // ## Output Contract
	"\r\n", // - trailing item
}

// mixLineEndings re-terminates each line of doc with its own named terminator, so a single
// document exercises LF, CRLF and lone CR simultaneously. It fails the test rather than
// panicking or silently truncating when the two lists have drifted apart, so a fixture edit
// that forgets a terminator surfaces as a fixture error instead of as a mysterious slicing
// failure.
func mixLineEndings(t *testing.T, doc string, terminators []string) string {
	t.Helper()
	lines := strings.Split(doc, "\n")
	if len(lines)-1 != len(terminators) {
		t.Fatalf("fixture error: document has %d terminated lines but %d terminators were named — "+
			"name the new line's terminator in lineEndingFixtureTerminators", len(lines)-1, len(terminators))
	}
	// The rotation this replaced guaranteed all three conventions structurally, because it
	// cycled a three-element list. An explicit list cannot promise that on its own: flattening
	// every entry to "\n" would keep the count guard happy and silently turn the mixed case
	// into a byte-for-byte duplicate of the LF case, which is a decorative test. Assert the
	// property directly instead of trusting the list to be written correctly.
	for _, convention := range []string{"\n", "\r\n", "\r"} {
		if !slices.Contains(terminators, convention) {
			t.Fatalf("fixture error: terminators must exercise LF, CRLF and lone CR, but %q is absent — "+
				"the mixed-endings case stops being mixed without it", convention)
		}
	}

	var b strings.Builder
	for i, line := range lines[:len(lines)-1] {
		b.WriteString(line)
		b.WriteString(terminators[i])
	}
	b.WriteString(lines[len(lines)-1])
	mixed := b.String()

	// The count guard above catches a missing terminator but not a misplaced one. Naming a
	// terminator at the wrong index — or reordering the list — can still put a lone "\r"
	// immediately before a "\n", where the two collapse into a single newline during
	// normalisation, swallowing the blank separator and truncating the multi-line list item.
	// Round-tripping closes that whole class: whatever terminators are named, normalising the
	// mixed document must reproduce the LF fixture exactly, or the fixture is wrong rather
	// than the slicer.
	if got := normaliseLineEndings(mixed); got != doc {
		t.Fatalf("fixture error: mixed document does not normalise back to the LF fixture — "+
			"a terminator is named at the wrong index or out of order.\n got %q\nwant %q", got, doc)
	}
	return mixed
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

	t.Run("validate_against_actuals_schema", func(t *testing.T) {
		schema := actualsSchemaDocument{
			Required: []string{"change_name", "approval_decision"},
			Properties: map[string]json.RawMessage{
				"change_name":          json.RawMessage(`{"type":"string"}`),
				"approval_decision":    json.RawMessage(`{"type":"string"}`),
				"implementation_hours": json.RawMessage(`{"type":"number"}`),
			},
		}

		tests := []struct {
			name    string
			record  map[string]any
			wantErr bool
		}{
			{
				name:   "every required name present, no undeclared keys — valid",
				record: map[string]any{"change_name": "x", "approval_decision": "approved"},
			},
			{
				name:   "an optional property may be present without becoming required",
				record: map[string]any{"change_name": "x", "approval_decision": "approved", "implementation_hours": 1.2},
			},
			{
				name:    "a missing required name is rejected — this is the R-019/R-021 mutation this gate exists to catch",
				record:  map[string]any{"change_name": "x"},
				wantErr: true,
			},
			{
				name:    "a key not declared in properties is rejected (additionalProperties: false)",
				record:  map[string]any{"change_name": "x", "approval_decision": "approved", "undeclared_field": true},
				wantErr: true,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := validateAgainstActualsSchema(schema, tt.record)
				if tt.wantErr && err == nil {
					t.Fatalf("validateAgainstActualsSchema(%v) error = nil, want an error", tt.record)
				}
				if !tt.wantErr && err != nil {
					t.Fatalf("validateAgainstActualsSchema(%v) error = %v, want nil", tt.record, err)
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
			{name: "mixed endings", doc: mixLineEndings(t, lineEndingFixture, lineEndingFixtureTerminators)},
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

	t.Run("heading_line_matching", func(t *testing.T) {
		// markdownHeadingLineMatches is called directly here, bypassing normaliseLineEndings,
		// because that is the only way to reach the "\r" byte of its trim cutset: every path
		// through sliceMarkdownSection normalises first, so the cutset would otherwise be
		// unpinned and a revert of that byte would leave the whole suite green.
		tests := []struct {
			name    string
			line    string
			heading string
			want    bool
		}{
			{name: "exact match", line: "## Hard Rules", heading: timeEstimationHardRulesHeading, want: true},
			{name: "trailing spaces are trimmed", line: "## Hard Rules   ", heading: timeEstimationHardRulesHeading, want: true},
			{name: "trailing tabs are trimmed", line: "## Hard Rules\t\t", heading: timeEstimationHardRulesHeading, want: true},
			{name: "trailing carriage return is trimmed", line: "## Hard Rules\r", heading: timeEstimationHardRulesHeading, want: true},
			{name: "trailing carriage return before spaces is trimmed", line: "## Hard Rules\r  ", heading: timeEstimationHardRulesHeading, want: true},
			{name: "trailing carriage return after spaces is trimmed", line: "## Hard Rules  \r", heading: timeEstimationHardRulesHeading, want: true},
			{name: "a different heading does not match", line: "## Output Contract", heading: timeEstimationHardRulesHeading, want: false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if got := markdownHeadingLineMatches(tt.line, tt.heading); got != tt.want {
					t.Fatalf("markdownHeadingLineMatches(%q, %q) = %t, want %t", tt.line, tt.heading, got, tt.want)
				}
			})
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
				// The one input shape that returns an empty body with a nil error, pinned
				// because sliceMarkdownSection's own doc comment calls the empty-versus-error
				// distinction load-bearing. Allowing it is deliberate, not an oversight: the
				// gate's REQUIRE assertions all fail on an empty section, and its FORBID
				// assertions are deliberately whole-file rather than section-scoped, so an
				// empty body can never silently satisfy a prohibition. Promoting this shape to
				// an error would change what the ten content-gate groups assert.
				name:    "heading as the final line yields an empty body and no error",
				doc:     "# Doc\nbody\n## Hard Rules\n",
				heading: timeEstimationHardRulesHeading,
				want:    "",
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
