package skills

import (
	"strings"
	"testing"
)

func TestProceduralCandidateContractArtifact(t *testing.T) {
	repoRoot := ooQualityContractRepoRoot(t)
	contract := readRepoFile(t, repoRoot, "skills/_shared/procedural-candidate-detection.md")

	t.Run("R001_topic_key_shapes_present_verbatim", func(t *testing.T) {
		for _, required := range []string{
			"procedural/candidates/repeated-success/{approach-slug}",
			"procedural/candidates/failure-recovery/{failure-slug}/{recovery-slug}",
		} {
			if !strings.Contains(contract, required) {
				t.Fatalf("contract must contain topic-key shape %q verbatim", required)
			}
		}
	})

	t.Run("R001_record_field_block_present", func(t *testing.T) {
		for _, field := range []string{
			"Kind",
			"Candidate",
			"Aliases",
			"Status",
			"RejectionReason",
			"MatchedSkillPath",
			"Threshold",
			"OccurrenceCount",
			"FirstObserved",
			"LastObserved",
			"Occurrences",
			"Summary",
		} {
			marker := "**" + field + "**"
			if !strings.Contains(contract, marker) {
				t.Fatalf("contract must contain record field %q verbatim", marker)
			}
		}
	})

	t.Run("R002_R003_emission_decision_table_present", func(t *testing.T) {
		for _, row := range []string{
			"| — (no record) | — | 1 | not called | `observing` | No |",
			"| `observing` | yes | unchanged | not called | `observing` | No |",
			"| `observing` | no | < N | not called | `observing` | No |",
			"| `observing` | no | = N | no match | `emitted` | **Yes, once** |",
			"| `observing` | no | = N | match | `rejected` | No |",
			"| `emitted` | no | > N | not called | `emitted` | No |",
			"| `rejected` | no | > N | not called | `rejected` | No |",
		} {
			if !strings.Contains(contract, row) {
				t.Fatalf("contract must contain emission decision table row %q verbatim", row)
			}
		}
	})

	t.Run("R002_R003_threshold_default_present", func(t *testing.T) {
		if !strings.Contains(contract, "Threshold: 3") {
			t.Fatalf("contract must contain %q verbatim", "Threshold: 3")
		}
	})

	t.Run("R004_rejection_record_fields_present", func(t *testing.T) {
		for _, required := range []string{
			"**RejectionReason**",
			"**MatchedSkillPath**",
			"present only when Status=rejected",
			"present only when RejectionReason=duplicate",
		} {
			if !strings.Contains(contract, required) {
				t.Fatalf("contract must contain rejection field marker %q verbatim", required)
			}
		}
	})

	t.Run("R004_match_candidate_call_site_present", func(t *testing.T) {
		if !strings.Contains(contract, "MatchCandidate(registry, slug)") {
			t.Fatalf("contract must contain the MatchCandidate call-site description %q verbatim", "MatchCandidate(registry, slug)")
		}
	})

	t.Run("R004_untruncated_comparison_rule_present", func(t *testing.T) {
		required := "Comparison always uses the untruncated normalized form, never the truncated form `NormalizeSlug` returns."
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must state the untruncated-comparison rule verbatim: %q", required)
		}
	})

	t.Run("R004_sweep_memo_key_untruncated_present", func(t *testing.T) {
		required := "Any per-sweep reuse or memoization of a `MatchCandidate` answer MUST be keyed by the untruncated normalized form, never by the truncated `NormalizeSlug` topic-key slug, because two long candidates sharing a truncated prefix would otherwise share one cached answer and reintroduce the false duplicate this section exists to prevent."
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must state the sweep-memoization key rule verbatim: %q", required)
		}
	})

	t.Run("Section7_DoNotCaptureCategoriesPresentVerbatim", func(t *testing.T) {
		for _, required := range []string{
			"**Environment-dependent failure**",
			"**Negative claim about a tool with no independent verification**",
			"**Transient error**",
			"**One-off narrative**",
			"**Unresolved failure presented as workflow**",
		} {
			if !strings.Contains(contract, required) {
				t.Fatalf("contract must contain do-not-capture category %q verbatim", required)
			}
		}
	})

	t.Run("Section7_DoNotCaptureRejectionReasonFormatPresent", func(t *testing.T) {
		required := "RejectionReason: do-not-capture:<class>"
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must state the do-not-capture rejection reason format verbatim: %q", required)
		}
	})

	t.Run("Section7_DoNotCaptureSingleListEnforcedTwice", func(t *testing.T) {
		required := "This is the single do-not-capture list for this contract. It is defined exactly once, here, and enforced at two independent points in the lifecycle: at emission (this document's own section 4, before a candidate that reaches `Threshold` is allowed to advance past `emitted`) and at draft time (`procedural-skill-drafting`, before any draft record is saved). Neither enforcement point supersedes the other: a candidate that slips past emission-time enforcement is still refused at draft time."
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must state the single-list-enforced-twice rule verbatim: %q", required)
		}
	})

	t.Run("Section8_LessonShapeRulePresentVerbatim", func(t *testing.T) {
		required := "the summary MUST be phrased as an imperative rule followed by exactly one clause explaining why, and MUST NOT include observation ids, dates, or PR/issue numbers."
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must state the lesson-shape rule verbatim: %q", required)
		}
	})

	t.Run("Section8_DispositionRulePresentVerbatim", func(t *testing.T) {
		required := "Every candidate MUST have a `Disposition` of exactly `new` or `extend:<skill-id>`, computed before drafting."
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must state the Disposition rule verbatim: %q", required)
		}
	})

	t.Run("Section8_DraftTopicKeyShapePresent", func(t *testing.T) {
		required := "procedural/drafts/{kind}/{slug}"
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must contain the draft topic-key shape %q verbatim", required)
		}
	})

	t.Run("Section8_DraftNoFileUnderSkillsPresent", func(t *testing.T) {
		required := "No draft file is ever written under `skills/` at any point in the drafting lifecycle"
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must state the no-draft-file-under-skills rule verbatim: %q", required)
		}
	})

	t.Run("Section8_DraftRecordFieldBlockPresent", func(t *testing.T) {
		for _, field := range []string{
			"**Candidate**",
			"**Disposition**",
			"**Status**",
			"**ForRevision**",
			"**Lint**",
			"**History**",
			"**Body**",
		} {
			if !strings.Contains(contract, field) {
				t.Fatalf("contract must contain draft record field %q verbatim", field)
			}
		}
	})

	t.Run("Section9_StatusVocabularyPresentVerbatim", func(t *testing.T) {
		required := "observing | emitted | rejected | drafted | registered | promoted | retired"
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must contain the extended Status vocabulary %q verbatim", required)
		}
	})

	t.Run("Section9_TransitionTableRowsPresentVerbatim", func(t *testing.T) {
		for _, row := range []string{
			"| none | `observing` | agent | none | first occurrence (item 30) | `engram:<id>` |",
			"| `observing` | `emitted` | agent | none | count reaches `Threshold`, no `MatchCandidate` match (item 30) | count |",
			"| `observing` | `rejected` | agent | none | `MatchCandidate` match (item 30) | `MatchedSkillPath` |",
			"| `observing` or `emitted` | `rejected` | agent | none | do-not-capture class matched at emission or at draft time | `RejectionReason: do-not-capture:<class>` |",
			"| `emitted` | `drafted` | agent | project | draft record written, `LintSkill` hard = 0 (new) or diff applies cleanly (extend) | draft topic key, `Disposition`, lint counts |",
			"| `drafted` | `registered` | agent | project | `project-register` and commit (new), or `project-revise` of an agent-owned target (extend) | skill id, `sha256`, commit, rev |",
			"| `registered` | `registered` | agent | project | revision (ownership OK, trigger reached) | `sha256`, commit, rev |",
			"| `drafted` or `registered` | `promoted` | human | global | `engine skills add` merged in the overlay | overlay commit, `sha256` |",
			"| `registered` | `retired` | agent | project | `project-retire` and commit | `RetirementReason`, commit |",
			"| `promoted` | `retired` | human | global | `engine skills remove` merged | `RetirementReason`, overlay commit |",
		} {
			if !strings.Contains(contract, row) {
				t.Fatalf("contract must contain transition table row %q verbatim", row)
			}
		}
	})

	t.Run("Section9_PromotedRetirePreservesStatusPresent", func(t *testing.T) {
		required := "it is NOT the `registered -> retired` transition in the table above — `Status` stays `promoted`, and only a `promoted -> promoted` `History` line records the removal."
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must state the promoted-retire status-preservation rule verbatim: %q", required)
		}
	})

	t.Run("Section9_HistoryFormatLinesPresentVerbatim", func(t *testing.T) {
		for _, line := range []string{
			"- 2026-09-20T10:00:00Z | observing -> emitted | agent | count 3/3, no registry match",
			"- 2026-09-20T10:05:00Z | emitted -> drafted | agent | draft procedural/drafts/repeated-success/probe-engram-with-home-override; Disposition new; lint hard 0 warnings 1 (body-recommended)",
			"- 2026-09-20T10:09:00Z | drafted -> registered | agent | skill probe-engram-with-home-override rev 1 sha256:3f2a...c1 commit a1b2c3d",
		} {
			if !strings.Contains(contract, line) {
				t.Fatalf("contract must contain History example line %q verbatim", line)
			}
		}
	})

	t.Run("Section9_HistoryWriteRulePrefixInvariantPresent", func(t *testing.T) {
		required := "After the write, the old lines MUST be a prefix of the new ones"
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must state the History prefix-invariant write rule verbatim: %q", required)
		}
	})

	t.Run("Section9_NewCandidateRecordFieldsPresentVerbatim", func(t *testing.T) {
		for _, field := range []string{
			"`**Disposition**: new | extend:<skill-id>`",
			"`**Draft**: procedural/drafts/...`",
			"`**Registered**: <id> rev:<n> sha256:<hex> commit:<sha> at:<RFC3339>`",
			"`**Promoted**: <id> path:skills/<id>/SKILL.md sha256:<hex> commit:<sha> at:<RFC3339>`",
			"`**OccurrencesSincePromotion**: <n>`",
			"`**RetirementReason**: stale-reference | superseded | absorbed | promoted | quiet | human-request`",
			"`**AbsorbedInto**: <skill-id>`",
		} {
			if !strings.Contains(contract, field) {
				t.Fatalf("contract must contain new candidate-record field %q verbatim", field)
			}
		}
	})

	t.Run("Section10_RuntimeTargetsTableRowsPresent", func(t *testing.T) {
		for _, required := range []string{
			"| `.claude/skills/` | Claude Code | verified |",
			"| `.agents/skills/` | Pi (always, after project trust); Codex (discovery only, on this host) | verified |",
		} {
			if !strings.Contains(contract, required) {
				t.Fatalf("contract must contain runtime targets table row starting %q verbatim", required)
			}
		}
	})

	t.Run("Section10_CodexStatusCellDiscoveryScopedVerbatim", func(t *testing.T) {
		required := "recorded verdict `PASS`, scoped to discovery of `.agents/skills/<id>/` (name and description exposed in Codex's skill list); loading the skill body is unverified on this host"
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must state the scoped Codex verdict verbatim: %q", required)
		}
	})

	t.Run("Section10_PiTrustNotePresent", func(t *testing.T) {
		required := "**Pi trust note**:"
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must contain the Pi trust note marker %q verbatim", required)
		}
	})

	t.Run("Section10_NeverWritesPiSkillsDirectory", func(t *testing.T) {
		if strings.Contains(contract, "| `.pi/skills") {
			t.Fatalf("contract must never name .pi/skills/ as a runtime targets table row")
		}
		required := "`.pi/skills/` is never written by this capability."
		if !strings.Contains(contract, required) {
			t.Fatalf("contract must state the never-write-.pi/skills rule verbatim: %q", required)
		}
	})
}
