package query

import (
	"context"
	"errors"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vecindex"
)

// TestDeps_LoadIndexIsConsultedInsteadOfPackageLoad proves Deps.LoadIndex is
// the actual seam Run and the embedding arm read the index through, not
// merely a field that exists beside an unconditional vecindex.Load call.
//
// It sets Deps.LoadIndex to a fake that always reports ErrNoIndex, even
// though a real, loadable index sits on disk at deps.StateDir -- if Run
// ever fell back to vecindex.Load directly, this query would default to
// the union and Coverage would show a real index. It must not: the
// injected loader is what decides, so the default stays engram-fts alone
// and no Coverage entry is produced.
func TestDeps_LoadIndexIsConsultedInsteadOfPackageLoad(t *testing.T) {
	store, _, _, stateDir := newEmbedArmFixture(t, []embedArmFixture{
		{title: "indexed row", content: "zephyr content", project: "proj-loadindex", vec: []float32{1, 0, 0}},
	})

	loadCalls := 0
	deps := Deps{
		Engram:        store,
		RetrieveVault: fakeRetrieveVault(vault.Result{Status: vault.StatusOK}, nil),
		ResolveLink:   NoLinkResolver,
		StateDir:      stateDir,
		Embed:         fakeEmbed([]float32{1, 0, 0}, nil),
		LoadIndex: func(dir string) (*vecindex.Index, error) {
			loadCalls++
			return nil, vecindex.ErrNoIndex
		},
	}

	got, err := Run(context.Background(), deps, Request{Project: "proj-loadindex", Query: "zephyr"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if loadCalls == 0 {
		t.Fatalf("Deps.LoadIndex was never called; Run must consult it rather than vecindex.Load directly")
	}
	if len(got.Coverage) != 0 {
		t.Fatalf("Coverage = %+v, want none: the injected loader reported no index, so the union must not be defaulted in even though a real index exists on disk", got.Coverage)
	}
}

// TestDeps_LoadIndexErrorIsNotSwallowedAsSuccess is the same seam's other
// direction: an injected loader that reports success for a project with no
// real index on disk must be trusted, proving Run reads the *Index it
// returns rather than re-deriving one from StateDir.
func TestDeps_LoadIndexErrorIsNotSwallowedAsSuccess(t *testing.T) {
	store := newFixtureEngramStore(t, []fixtureObservation{
		{title: "engram row", content: "zephyr keyword", project: "proj-loadindex-2"},
	})

	deps := Deps{
		Engram:        store,
		RetrieveVault: fakeRetrieveVault(vault.Result{Status: vault.StatusOK}, nil),
		ResolveLink:   NoLinkResolver,
		StateDir:      t.TempDir(), // no real index here
		LoadIndex: func(dir string) (*vecindex.Index, error) {
			return nil, errors.New("boom")
		},
	}

	got, err := Run(context.Background(), deps, Request{Project: "proj-loadindex-2", Query: "zephyr", Sources: []string{SourceEngramFTS, SourceEngramEmbed}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got.Coverage) != 1 || got.Coverage[0].BuiltAt != embeddingIndexNeverBuilt {
		t.Fatalf("Coverage = %+v, want one entry degrading exactly like ErrNoIndex", got.Coverage)
	}
}
