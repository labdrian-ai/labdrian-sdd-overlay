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
	search, err := store.Search("proj-a", "shared", 10)
	if err != nil || len(search.Rows) != 1 {
		t.Fatalf("fixture setup: Search = %+v, %v", search, err)
	}
	linkedID := search.Rows[0].ID
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

// newDegradedFixtureEngramStore builds the same schema fixture
// newFixtureEngramStore does, then forces engram.Open down its immutable=1
// fallback: a WAL database with no -wal/-shm on disk inside a directory
// that cannot be written means the primary mode=ro connection cannot
// create the shared-memory index, so Open retries immutable and marks the
// Store degraded. It mirrors internal/engram's own
// TestOpen_FallsBackToImmutableWhenPrimaryReadOnlyOpenFails fixture.
func newDegradedFixtureEngramStore(t *testing.T, rows []fixtureObservation) *engram.Store {
	t.Helper()

	schema, err := os.ReadFile(filepath.Join("..", "engram", "testdata", "schema.sql"))
	if err != nil {
		t.Fatalf("read engram schema fixture: %v", err)
	}
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "engram.db")
	setup, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture setup connection: %v", err)
	}
	if _, err := setup.Exec("PRAGMA journal_mode=WAL"); err != nil {
		setup.Close()
		t.Fatalf("set fixture journal_mode=WAL: %v", err)
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

	for _, suffix := range []string{"-wal", "-shm"} {
		_ = os.Remove(dbPath + suffix)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod %s 0o555: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	store, err := engram.Open(dbPath)
	if err != nil {
		t.Fatalf("engram.Open(%q): %v", dbPath, err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if degraded, _ := store.Degraded(); !degraded {
		t.Fatal("fixture store is not degraded; the immutable=1 fallback was not exercised")
	}
	return store
}

// TestQuery_DegradedEngramSnapshotIsReportedAsADiagnostic: a Store opened
// through the immutable=1 fallback answers from a point-in-time snapshot
// taken when the connection was opened, not the live database. The MCP
// server opens that connection once for a whole session (cmd_mcp.go), so a
// degraded fallback there serves a frozen corpus for as long as the client
// stays connected -- silently, because Store.Degraded had exactly one
// production reader (cmd_status.go) and query results carried no trace of
// it. Every query surface must be able to tell a live corpus from a frozen
// snapshot.
//
// The diagnostic code is asserted as a literal, not through the constant:
// it is a wire value clients match on, so the test must fail if the
// constant is ever repointed.
func TestQuery_DegradedEngramSnapshotIsReportedAsADiagnostic(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root ignores directory permissions; the degraded fallback cannot be forced")
	}

	store := newDegradedFixtureEngramStore(t, []fixtureObservation{{title: "snapshot row", content: "keyword content", project: "proj-a"}})
	deps := Deps{
		Engram:        store,
		RetrieveVault: fakeRetrieveVault(vault.Result{Status: vault.StatusOK}, nil),
		ResolveLink:   NoLinkResolver,
	}

	got, err := Run(context.Background(), deps, Request{Project: "proj-a", Query: "keyword", Top: 10})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var found *Diagnostic
	for i := range got.Diagnostics {
		if got.Diagnostics[i].Code == "engram_degraded_snapshot" {
			found = &got.Diagnostics[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("Diagnostics = %+v, want one with code \"engram_degraded_snapshot\": a caller cannot otherwise tell a live corpus from a session-long frozen snapshot", got.Diagnostics)
	}
	if found.Detail == "" {
		t.Error("the degraded-snapshot diagnostic carries no detail; the primary open error is the only clue to why the corpus is frozen")
	}
}

// TestQuery_HealthyEngramReportsNoDegradedDiagnostic is the other half:
// the diagnostic must mean something, so an ordinary live connection must
// never emit it.
func TestQuery_HealthyEngramReportsNoDegradedDiagnostic(t *testing.T) {
	store := newFixtureEngramStore(t, []fixtureObservation{{title: "live row", content: "keyword content", project: "proj-a"}})
	deps := Deps{
		Engram:        store,
		RetrieveVault: fakeRetrieveVault(vault.Result{Status: vault.StatusOK}, nil),
		ResolveLink:   NoLinkResolver,
	}

	got, err := Run(context.Background(), deps, Request{Project: "proj-a", Query: "keyword", Top: 10})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, d := range got.Diagnostics {
		if d.Code == "engram_degraded_snapshot" {
			t.Fatalf("a healthy connection reported %q: %s", d.Code, d.Detail)
		}
	}
}

// A memory that was explicitly replaced comes back from Engram's own search
// looking exactly like one that was not: verified on a copy of a real
// database, inserting "B supersedes A" left A's search results
// byte-identical and still ranked first. That is how an abandoned decision
// gets read as current and reintroduced. longterm-mem cannot change
// Engram's search -- its connection is read-only (R-002) -- but repeating
// the omission in its own answers is a choice, and this is it being made
// the other way.
func TestQuery_ResultsCarryWhatTheRelationLedgerSays(t *testing.T) {
	store, oldID, newID := newRelatedFixtureStore(t)
	deps := Deps{Engram: store, RetrieveVault: fakeRetrieveVault(vault.Result{Status: vault.StatusNotProvisioned}, nil), ResolveLink: NoLinkResolver}

	got, err := Run(context.Background(), deps, Request{Project: "proj-a", Query: "zephyr", Top: 10})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var oldRow, newRow *ResultRow
	for i := range got.Results {
		switch got.Results[i].EngramID {
		case oldID:
			oldRow = &got.Results[i]
		case newID:
			newRow = &got.Results[i]
		}
	}
	if oldRow == nil || newRow == nil {
		t.Fatalf("fixture did not return both observations: %+v", got.Results)
	}

	if oldRow.Standing == nil || len(oldRow.Standing.SupersededBy) != 1 {
		t.Fatalf("the replaced memory must come back saying so: %+v", oldRow.Standing)
	}
	if oldRow.Standing.SupersededBy[0].ID != newID {
		t.Errorf("it must name what replaced it: %+v", oldRow.Standing.SupersededBy[0])
	}
	if newRow.Standing != nil {
		t.Errorf("the replacement carries no warning of its own: %+v", newRow.Standing)
	}
}

// newRelatedFixtureStore builds a store holding two observations, the
// second judged to supersede the first.
func newRelatedFixtureStore(t *testing.T) (*engram.Store, int64, int64) {
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
	defer setup.Close()
	if _, err := setup.Exec(string(schema)); err != nil {
		t.Fatalf("apply engram schema fixture: %v", err)
	}

	ids := make([]int64, 0, 2)
	for i, sync := range []string{"sync-old", "sync-new"} {
		res, err := setup.Exec(
			`INSERT INTO observations (session_id, sync_id, type, title, content, project) VALUES (?, ?, ?, ?, ?, ?)`,
			"sess-1", sync, "decision", "zephyr approach", "zephyr keyword body", "proj-a",
		)
		if err != nil {
			t.Fatalf("insert fixture observation %d: %v", i, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatalf("LastInsertId: %v", err)
		}
		ids = append(ids, id)
	}
	if _, err := setup.Exec(
		`INSERT INTO memory_relations (sync_id, source_id, target_id, relation, judgment_status) VALUES (?, ?, ?, ?, ?)`,
		"rel-1", "sync-new", "sync-old", "supersedes", "judged",
	); err != nil {
		t.Fatalf("insert fixture relation: %v", err)
	}

	store, err := engram.Open(dbPath)
	if err != nil {
		t.Fatalf("engram.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, ids[0], ids[1]
}

// TestQuery_ReportsAWidenedSearch keeps the AND->OR fallback from being a
// silent rewrite. Broadening the query is the right answer to an empty
// precise one, but the caller is then reading results that satisfy one of
// its words rather than all of them, and nothing in the rows themselves
// says so.
func TestQuery_ReportsAWidenedSearch(t *testing.T) {
	store := newFixtureEngramStore(t, []fixtureObservation{
		{title: "writer", content: "the register writer sorts its keys", project: "proj-a"},
	})
	deps := Deps{Engram: store, RetrieveVault: fakeRetrieveVault(vault.Result{Status: vault.StatusOK}, nil), ResolveLink: NoLinkResolver}

	got, err := Run(context.Background(), deps, Request{Project: "proj-a", Query: "what conventions apply when editing the register writer", Top: 10})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1: the widened search must find the row the precise one missed", len(got.Results))
	}
	if !hasDiagnostic(got, DiagnosticSearchWidened) {
		t.Fatalf("no %s diagnostic; diagnostics = %+v", DiagnosticSearchWidened, got.Diagnostics)
	}
}

// TestQuery_SaysNothingAboutWideningWhenItDidNotWiden guards the other
// direction: a diagnostic on every call is a diagnostic nobody reads.
func TestQuery_SaysNothingAboutWideningWhenItDidNotWiden(t *testing.T) {
	store := newFixtureEngramStore(t, []fixtureObservation{
		{title: "both", content: "canonical identity resolution", project: "proj-a"},
	})
	deps := Deps{Engram: store, RetrieveVault: fakeRetrieveVault(vault.Result{Status: vault.StatusOK}, nil), ResolveLink: NoLinkResolver}

	got, err := Run(context.Background(), deps, Request{Project: "proj-a", Query: "canonical identity", Top: 10})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if hasDiagnostic(got, DiagnosticSearchWidened) {
		t.Fatalf("unexpected %s diagnostic on a search that matched every token: %+v", DiagnosticSearchWidened, got.Diagnostics)
	}
}

func hasDiagnostic(r Result, code string) bool {
	for _, d := range r.Diagnostics {
		if d.Code == code {
			return true
		}
	}
	return false
}
