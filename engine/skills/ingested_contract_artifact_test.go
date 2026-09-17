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
		if !strings.Contains(contract, "| Source-Kind | Canonical origin | Human part |") {
			t.Fatal("contract must contain the Source-Id derivation table header verbatim")
		}
		for _, row := range []string{
			"| `url` | lowercase scheme+host",
			"| `file` | cleaned absolute path",
			"| `directory` | as `file`, applied per contained file",
			"| `pasted` | `pasted:` + the operator-supplied label",
		} {
			if !strings.Contains(contract, row) {
				t.Fatalf("contract must contain Source-Id derivation row %q verbatim", row)
			}
		}
	})

	t.Run("OQ5_reingestion_decision_table_present", func(t *testing.T) {
		if !strings.Contains(contract, "| Prior state at the key | `Source-SHA256` vs stored | Action |") {
			t.Fatal("contract must contain the OQ-5 decision table header verbatim")
		}
		for _, row := range []string{
			"| none | — | Create every record, promote each",
			"| exists | **equal** | **Hard no-op.**",
			"| exists | **differ** | Upsert every chunk whose `Content-SHA256` changed",
			"| exists, **new N < old N**, new N ≥ 1 | — | Surplus keys `c{N+1}…` upsert to a retired tombstone",
			"| exists, **new N == 1** (was > 1) | — | The record moves to the manifest key",
			"| exists, **new N > old N** | — | New chunk keys are created and promoted |",
			"| **origin changed** (new URI ⇒ new `Source-Id`) | — | A different topic key.",
		} {
			if !strings.Contains(contract, row) {
				t.Fatalf("contract must contain OQ-5 decision table row %q verbatim", row)
			}
		}
	})

	t.Run("hard_no_op_and_tombstone_rules_present", func(t *testing.T) {
		for _, required := range []string{
			"**Hard no-op.** No `mem_save`, no `promote`.",
			"**Never hand-deleted.**",
		} {
			if !strings.Contains(contract, required) {
				t.Fatalf("contract must state rule %q verbatim", required)
			}
		}
	})

	t.Run("SkipCode_vocabulary_table_present", func(t *testing.T) {
		if !strings.Contains(contract, "| `Skip.Code` | Meaning |") {
			t.Fatal("contract must contain the Skip.Code table header verbatim")
		}
		for _, code := range []string{
			"unreadable", "binary", "unsupported_extension",
			"too_large", "empty", "symlink", "outside_root",
		} {
			row := "| `" + code + "` |"
			if !strings.Contains(contract, row) {
				t.Fatalf("contract must contain Skip.Code row %q verbatim", row)
			}
		}
	})
}
