package skills

import (
	"strings"
	"testing"
)

func TestIngestedObservationContractArtifact(t *testing.T) {
	repoRoot := ooQualityContractRepoRoot(t)
	contract := readRepoFile(t, repoRoot, "skills/_shared/ingested-observation-contract.md")

	t.Run("R002_provenance_block_fields_present_verbatim", func(t *testing.T) {
		for _, field := range []string{
			"Ingested",
			"Source-Kind",
			"Source-URI",
			"Source-Id",
			"Source-Title",
			"Source-SHA256",
			"Content-SHA256",
			"Ingested-At",
			"Ingested-By",
			"Chunk",
			"Chunk-Span",
			"Chunk-Path",
			"Split",
			"Status",
		} {
			marker := "**" + field + "**"
			if !strings.Contains(contract, marker) {
				t.Fatalf("contract must contain provenance field %q verbatim", marker)
			}
		}
	})

	t.Run("R002_manifest_only_fields_present_verbatim", func(t *testing.T) {
		for _, field := range []string{
			"Chunks-Expected",
			"Chunks-Saved",
			"Chunk-Status",
		} {
			marker := "**" + field + "**"
			if !strings.Contains(contract, marker) {
				t.Fatalf("contract must contain manifest-only field %q verbatim", marker)
			}
		}
	})

	t.Run("R004_topic_key_shapes_present_verbatim", func(t *testing.T) {
		for _, required := range []string{
			"ingested/{source-kind}/{source-id}",
			"ingested/{source-kind}/{source-id}/c{NNNN}",
		} {
			if !strings.Contains(contract, required) {
				t.Fatalf("contract must contain topic-key shape %q verbatim", required)
			}
		}
	})

	t.Run("SourceId_derivation_rule_table_present", func(t *testing.T) {
		for _, required := range []string{
			"url",
			"file",
			"directory",
			"pasted",
			"Canonical origin",
			"Human part",
		} {
			if !strings.Contains(contract, required) {
				t.Fatalf("contract must contain Source-Id derivation table token %q", required)
			}
		}
	})

	t.Run("OQ5_reingestion_decision_table_present", func(t *testing.T) {
		for _, required := range []string{
			"none",
			"equal",
			"differ",
			"new N < old N",
			"new N > old N",
			"origin changed",
		} {
			if !strings.Contains(contract, required) {
				t.Fatalf("contract must contain OQ-5 decision table row token %q", required)
			}
		}
	})
}
