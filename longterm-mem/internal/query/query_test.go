package query

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"

	_ "modernc.org/sqlite"
)

// fixtureObservation is one row newFixtureEngramStore inserts before
// opening the (read-only) production Store.
type fixtureObservation struct{ title, content, project string }

// newFixtureEngramStore builds a temp SQLite DB from internal/engram's own
// schema.sql fixture (shared, not duplicated) and opens it through the real
// production engram.Open path, so Run's Engram search runs against a real
// database. Only the vault side is faked (design-notes #3133 Testing
// Strategy: "Fake retrieve + temp DB").
func newFixtureEngramStore(t *testing.T, rows []fixtureObservation) *engram.Store {
	t.Helper()

	schema, err := os.ReadFile(filepath.Join("..", "engram", "testdata", "schema.sql"))
	if err != nil {
		t.Fatalf("read engram schema fixture: %v", err)
	}
	dbPath := filepath.Join(t.TempDir(), "engram.db")
	setup, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture setup connection: %v", err)
	}
	if _, err := setup.Exec(string(schema)); err != nil {
		setup.Close()
		t.Fatalf("apply engram schema fixture: %v", err)
	}
	for _, r := range rows {
		if _, err := setup.Exec(
			`INSERT INTO observations (session_id, type, title, content, project) VALUES (?, ?, ?, ?, ?)`,
			"sess-1", "discovery", r.title, r.content, r.project,
		); err != nil {
			setup.Close()
			t.Fatalf("insert fixture observation %q: %v", r.title, err)
		}
	}
	setup.Close()

	store, err := engram.Open(dbPath)
	if err != nil {
		t.Fatalf("engram.Open(%q): %v", dbPath, err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// fakeRetrieveVault returns a Deps.RetrieveVault stand-in that ignores its
// arguments and always answers with result/err.
func fakeRetrieveVault(result vault.Result, err error) func(context.Context, string, string, int) (vault.Result, error) {
	return func(context.Context, string, string, int) (vault.Result, error) { return result, err }
}

func TestQuery_GroupedBySourceInNativeRankOrder(t *testing.T) {
	store := newFixtureEngramStore(t, []fixtureObservation{
		{title: "strong engram match", content: "zephyr zephyr zephyr keyword", project: "proj-a"},
		{title: "weak engram match", content: "zephyr keyword extra padding text", project: "proj-a"},
	})
	vaultResult := vault.Result{
		Status: vault.StatusOK,
		Candidates: []vault.Candidate{
			{PageAddress: "c-000001", AbsolutePath: "/vault/c-000001.md", Snippet: "vault snippet one"},
			{PageAddress: "c-000002", AbsolutePath: "/vault/c-000002.md", Snippet: "vault snippet two"},
		},
	}
	deps := Deps{Engram: store, RetrieveVault: fakeRetrieveVault(vaultResult, nil), ResolveLink: NoLinkResolver}

	got, err := Run(context.Background(), deps, Request{Project: "proj-a", Query: "zephyr keyword", Top: 10})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.VaultStatus != VaultStatusOK {
		t.Fatalf("VaultStatus = %q, want %q", got.VaultStatus, VaultStatusOK)
	}
	if len(got.Results) != 4 {
		t.Fatalf("len(Results) = %d, want 4; got %+v", len(got.Results), got.Results)
	}
	wantSources := []string{SourceVault, SourceVault, SourceEngram, SourceEngram}
	for i, row := range got.Results {
		if row.Source != wantSources[i] {
			t.Errorf("Results[%d].Source = %q, want %q", i, row.Source, wantSources[i])
		}
		if row.Rank != i+1 {
			t.Errorf("Results[%d].Rank = %d, want %d", i, row.Rank, i+1)
		}
	}
	if got.Results[0].PageAddress != "c-000001" || got.Results[1].PageAddress != "c-000002" {
		t.Fatalf("vault rows out of vault order: %+v", got.Results[:2])
	}
	if got.Results[2].Title != "strong engram match" || got.Results[3].Title != "weak engram match" {
		t.Fatalf("engram rows out of Engram's own rank order: %+v", got.Results[2:])
	}
}

func TestQuery_LinkedPairEmittedOnce(t *testing.T) {
	store := newFixtureEngramStore(t, []fixtureObservation{
		{title: "linked observation", content: "shared topic notes", project: "proj-a"},
	})
	rows, err := store.Search("proj-a", "shared", 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("fixture setup: Search = %+v, %v", rows, err)
	}
	linkedID := rows[0].ID
	vaultResult := vault.Result{
		Status:     vault.StatusOK,
		Candidates: []vault.Candidate{{PageAddress: "c-000042", AbsolutePath: "/vault/c-000042.md", Snippet: "vault side snippet"}},
	}
	deps := Deps{
		Engram:        store,
		RetrieveVault: fakeRetrieveVault(vaultResult, nil),
		ResolveLink: func(pageAddress string) (int64, bool) {
			if pageAddress == "c-000042" {
				return linkedID, true
			}
			return 0, false
		},
	}

	got, err := Run(context.Background(), deps, Request{Project: "proj-a", Query: "shared topic", Top: 10})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1 (linked pair collapsed); got %+v", len(got.Results), got.Results)
	}
	row := got.Results[0]
	if row.Source != SourceLinked {
		t.Fatalf("Source = %q, want %q", row.Source, SourceLinked)
	}
	if row.PageAddress != "c-000042" {
		t.Errorf("PageAddress = %q, want c-000042 (vault reference)", row.PageAddress)
	}
	if row.EngramID != linkedID {
		t.Errorf("EngramID = %d, want %d (engram reference)", row.EngramID, linkedID)
	}
}

func TestQuery_MissingProjectRejected(t *testing.T) {
	_, err := Run(context.Background(), Deps{}, Request{Project: "", Query: "anything"})
	if !errors.Is(err, ErrMissingProject) {
		t.Fatalf("err = %v, want ErrMissingProject", err)
	}
}

func TestQuery_NotProvisionedDegradesToEngramOnly(t *testing.T) {
	store := newFixtureEngramStore(t, []fixtureObservation{{title: "engram only result", content: "keyword content", project: "proj-a"}})
	deps := Deps{
		Engram:        store,
		RetrieveVault: fakeRetrieveVault(vault.Result{Status: vault.StatusNotProvisioned}, nil),
		ResolveLink:   NoLinkResolver,
	}

	got, err := Run(context.Background(), deps, Request{Project: "proj-a", Query: "keyword", Top: 10})
	if err != nil {
		t.Fatalf("Run returned an error; want nil (not_provisioned must degrade, not fail): %v", err)
	}
	if got.VaultStatus != VaultStatusNotProvisioned {
		t.Fatalf("VaultStatus = %q, want %q", got.VaultStatus, VaultStatusNotProvisioned)
	}
	if len(got.Results) != 1 || got.Results[0].Source != SourceEngram {
		t.Fatalf("Results = %+v, want exactly one engram-sourced row", got.Results)
	}
}
