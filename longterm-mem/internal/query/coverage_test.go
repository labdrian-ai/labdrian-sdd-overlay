package query

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestResponseCarriesEmbeddingCoverageWhenSourceRequested (design:
// "coverage is a response field, not a diagnostic"): naming engram-embed
// gets exactly one Coverage entry for it, and only for it -- FTS has no
// entry, since this module does not own observations_fts and reporting a
// count for it would report an assumption in the shape of a measurement.
func TestResponseCarriesEmbeddingCoverageWhenSourceRequested(t *testing.T) {
	store, _, _, index := newEmbedArmFixture(t, []embedArmFixture{
		{title: "one", content: "alpha", project: "proj-cov", vec: []float32{1, 0, 0}},
	})

	deps := Deps{Memory: store, ResolveLink: NoLinkResolver, Index: index, Embed: fakeEmbed([]float32{1, 0, 0}, nil)}
	result, err := Run(context.Background(), deps, Request{Project: "proj-cov", Query: "alpha", Sources: []string{SourceEngramEmbed}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Coverage) != 1 {
		t.Fatalf("Coverage = %+v, want exactly one entry", result.Coverage)
	}
	if result.Coverage[0].Source != SourceEngramEmbed {
		t.Fatalf("Coverage[0].Source = %q, want %q", result.Coverage[0].Source, SourceEngramEmbed)
	}
}

// TestCoverageIsPresentEvenWhenIndexIsComplete guards an `omitempty`
// regression on Result.Coverage: an index whose Unindexed is 0 must still
// carry a Coverage entry with that value, never omit the field entirely --
// an absent field and "the index is complete" must never look alike.
func TestCoverageIsPresentEvenWhenIndexIsComplete(t *testing.T) {
	store, _, _, index := newEmbedArmFixture(t, []embedArmFixture{
		{title: "one", content: "alpha", project: "proj-cov", vec: []float32{1, 0, 0}},
	})

	deps := Deps{Memory: store, ResolveLink: NoLinkResolver, Index: index, Embed: fakeEmbed([]float32{1, 0, 0}, nil)}
	result, err := Run(context.Background(), deps, Request{Project: "proj-cov", Query: "alpha", Sources: []string{SourceEngramEmbed}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Coverage) != 1 || result.Coverage[0].Unindexed != 0 {
		t.Fatalf("Coverage = %+v, want one complete (Unindexed=0) entry", result.Coverage)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"coverage"`) {
		t.Fatalf("encoded result is missing the \"coverage\" field entirely: %s", encoded)
	}
}

// TestIncompleteCoverageDetailNamesTheRebuildCommand: a caller reading a
// thin or empty paraphrase result must be told the exact command that
// fixes it, not left to infer one from a bare count (design's own table).
func TestIncompleteCoverageDetailNamesTheRebuildCommand(t *testing.T) {
	store, dbPath, ids, index := newEmbedArmFixture(t, []embedArmFixture{
		{title: "one", content: "alpha", project: "proj-cov", vec: []float32{1, 0, 0}},
		{title: "two", content: "beta", project: "proj-cov", vec: []float32{0, 1, 0}},
	})
	// Remove the second row's manifest entry from the on-disk index by
	// soft-deleting it, so Indexed(1) < Live(2) without touching the
	// fixture builder: the manifest still names it, but it no longer
	// resolves live, so it counts as Unindexed the same way a row added
	// after the last build would.
	_ = ids
	_ = dbPath

	deps := Deps{Memory: store, ResolveLink: NoLinkResolver, Index: index, Embed: fakeEmbed([]float32{1, 0, 0}, nil)}
	// Drop the second entry from the index directly to simulate a row
	// added to the corpus after the index was last built (Unindexed>0).
	held, err := index.LoadEmbeddingIndex("proj-cov")
	if err != nil {
		t.Fatalf("load fixture index: %v", err)
	}
	held.Entries = held.Entries[:1]
	held.Vectors = held.Vectors[:1]

	result, err := Run(context.Background(), deps, Request{Project: "proj-cov", Query: "alpha", Sources: []string{SourceEngramEmbed}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	found := false
	for _, d := range result.Diagnostics {
		if d.Code == DiagnosticEmbeddingIndexIncomplete && strings.Contains(d.Detail, "longterm-mem index --embeddings") {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %+v, want one %q naming the rebuild command verbatim", result.Diagnostics, DiagnosticEmbeddingIndexIncomplete)
	}
}
