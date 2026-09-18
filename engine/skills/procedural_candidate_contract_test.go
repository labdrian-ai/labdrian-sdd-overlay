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
}
