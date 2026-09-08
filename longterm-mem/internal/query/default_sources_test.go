package query

import (
	"context"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
)

// TestDefaultSources_UnionWhenIndexExists is this issue's core behavior: an
// omitted Sources must widen to engram-fts + engram-embed the moment a
// project's embedding index actually exists, rather than requiring a
// caller to name engram-embed by hand forever.
//
// Coverage is populated only on the wantEmbed branch of Run (query.go), so
// a non-nil Coverage entry for engram-embed is proof the embedding arm
// actually ran -- not just that the default sources slice happens to
// mention it.
func TestDefaultSources_UnionWhenIndexExists(t *testing.T) {
	store, _, _, stateDir := newEmbedArmFixture(t, []embedArmFixture{
		{title: "indexed row", content: "zephyr content", project: "proj-default", vec: []float32{1, 0, 0}},
	})
	deps := Deps{
		Engram:        store,
		RetrieveVault: fakeRetrieveVault(vault.Result{Status: vault.StatusOK}, nil),
		ResolveLink:   NoLinkResolver,
		StateDir:      stateDir,
		Embed:         fakeEmbed([]float32{1, 0, 0}, nil),
	}

	got, err := Run(context.Background(), deps, Request{Project: "proj-default", Query: "zephyr"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got.Coverage) != 1 || got.Coverage[0].Source != SourceEngramEmbed {
		t.Fatalf("Coverage = %+v, want one engram-embed entry proving the embedding arm ran by default when an index exists", got.Coverage)
	}
}

// TestDefaultSources_FTSOnlyWhenNoIndex is the "otherwise" half: a project
// with no embedding index yet must not silently ask the embedding arm to
// run every query, since it has nothing to search and nothing to learn
// from doing so on every call.
func TestDefaultSources_FTSOnlyWhenNoIndex(t *testing.T) {
	store := newFixtureEngramStore(t, []fixtureObservation{
		{title: "engram row", content: "zephyr keyword", project: "proj-no-index"},
	})
	deps := Deps{
		Engram:        store,
		RetrieveVault: fakeRetrieveVault(vault.Result{Status: vault.StatusOK}, nil),
		ResolveLink:   NoLinkResolver,
		StateDir:      t.TempDir(),
	}

	got, err := Run(context.Background(), deps, Request{Project: "proj-no-index", Query: "zephyr"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got.Coverage) != 0 {
		t.Fatalf("Coverage = %+v, want none: no index exists, so the embedding arm must not be defaulted on", got.Coverage)
	}
	if len(got.Results) != 1 || !hasSource(got.Results[0], SourceEngramFTS) {
		t.Fatalf("Results = %+v, want the one engram-fts row", got.Results)
	}
}
