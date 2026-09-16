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
}
