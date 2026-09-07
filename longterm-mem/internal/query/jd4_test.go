package query

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
)

// TestMergeResults_LinkedPairCollapsesAgainstTheEmbeddingArm (JD-4): a
// vault page whose linked observation was found ONLY by the embedding arm
// must still collapse into one linked row. Before this, the matcher
// consulted the FTS rows alone, so the same observation shipped twice --
// once as a vault row and once as an engram_embed row -- which is exactly
// the duplication R-006's linked-pair rule exists to prevent, and it got
// worse the better paraphrase retrieval worked.
func TestMergeResults_LinkedPairCollapsesAgainstTheEmbeddingArm(t *testing.T) {
	const addr, linkedID = "c-000042", int64(42)

	vaultRows := []vault.Candidate{{PageAddress: addr, AbsolutePath: "/v/c-000042.md", Snippet: "page snippet"}}
	// The FTS arm found nothing; only the embedding arm surfaced the
	// observation this page was promoted from.
	embedRows := []ResultRow{{EngramID: linkedID, Title: "linked observation", Snippet: "embed snippet", Content: "embed body", FullLength: 10}}
	resolveLink := func(a string) (int64, bool) { return linkedID, a == addr }

	merged := mergeResults(
		[]string{SourceVault, SourceEngramFTS, SourceEngramEmbed},
		vaultRows, nil, embedRows, resolveLink, "some query", engram.MatchAll,
	)

	if len(merged) != 1 {
		t.Fatalf("len(merged) = %d, want 1 (the linked pair collapsed); got %+v", len(merged), merged)
	}
	row := merged[0]
	if len(row.Sources) != 1 || row.Sources[0] != SourceLinked {
		t.Fatalf("Sources = %v, want [%s]", row.Sources, SourceLinked)
	}
	if row.EngramID != linkedID {
		t.Fatalf("EngramID = %d, want %d", row.EngramID, linkedID)
	}
	if row.PageAddress != addr {
		t.Fatalf("PageAddress = %q, want %q", row.PageAddress, addr)
	}
}

// TestMergeResults_EmbedOnlyLinkDoesNotStealAnFTSRow (JD-4) guards the
// collapse's direction: when both arms hold the linked observation, the
// FTS row is the one consumed, and the pair still collapses to exactly one
// row rather than leaving the embed copy behind.
func TestMergeResults_EmbedOnlyLinkDoesNotStealAnFTSRow(t *testing.T) {
	const addr, linkedID = "c-000042", int64(42)

	vaultRows := []vault.Candidate{{PageAddress: addr, AbsolutePath: "/v/c-000042.md", Snippet: "page snippet"}}
	// The two arms carry DIFFERENT titles on purpose. With the same title
	// this test passes whichever arm is consulted first, which makes it
	// unable to fail for the ordering it is named after -- proven by
	// swapping matchLinkedObservation's two loops and watching the whole
	// package stay green.
	ftsRows := []engram.Row{{ID: linkedID, Title: "title from the fts arm", Snippet: "fts snippet", Content: "fts body", ContentLength: 8}}
	embedRows := []ResultRow{{EngramID: linkedID, Title: "title from the embed arm", Snippet: "embed snippet", Content: "embed body", FullLength: 10}}
	resolveLink := func(a string) (int64, bool) { return linkedID, a == addr }

	merged := mergeResults(
		[]string{SourceVault, SourceEngramFTS, SourceEngramEmbed},
		vaultRows, ftsRows, embedRows, resolveLink, "some query", engram.MatchAll,
	)

	if len(merged) != 1 {
		t.Fatalf("len(merged) = %d, want 1; the embed copy of the consumed observation was left behind: %+v", len(merged), merged)
	}
	if merged[0].Sources[0] != SourceLinked {
		t.Fatalf("Sources = %v, want [%s]", merged[0].Sources, SourceLinked)
	}
	if merged[0].Title != "title from the fts arm" {
		t.Fatalf("Title = %q, want the FTS arm's title: the embed arm was consulted first, changing which row a caller sees for every already-working query", merged[0].Title)
	}
}
