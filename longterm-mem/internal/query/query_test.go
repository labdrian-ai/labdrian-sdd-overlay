package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

// typedObservation is a fixture row that also carries its Engram type,
// which the plain fixtureObservation fixes to "discovery".
type typedObservation struct{ title, content, project, obsType string }

// newFixtureEngramStoreTyped is newFixtureEngramStore with a
// caller-controlled observation type, for the type filter's tests.
func newFixtureEngramStoreTyped(t *testing.T, rows []typedObservation) *engram.Store {
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
			"sess-1", r.obsType, r.title, r.content, r.project,
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

	got, err := Run(context.Background(), deps, Request{Project: "proj-a", Query: "zephyr keyword", Top: 10, Sources: []string{SourceVault, SourceEngramFTS}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.VaultStatus != VaultStatusOK {
		t.Fatalf("VaultStatus = %q, want %q", got.VaultStatus, VaultStatusOK)
	}
	if len(got.Results) != 4 {
		t.Fatalf("len(Results) = %d, want 4; got %+v", len(got.Results), got.Results)
	}
	wantSources := []string{SourceVault, SourceVault, SourceEngramFTS, SourceEngramFTS}
	for i, row := range got.Results {
		if !hasSource(row, wantSources[i]) {
			t.Errorf("Results[%d].Sources = %v, want to include %q", i, row.Sources, wantSources[i])
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

	got, err := Run(context.Background(), deps, Request{Project: "proj-a", Query: "shared topic", Top: 10, Sources: []string{SourceVault, SourceEngramFTS}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1 (linked pair collapsed); got %+v", len(got.Results), got.Results)
	}
	row := got.Results[0]
	if !hasSource(row, SourceLinked) {
		t.Fatalf("Sources = %v, want to include %q", row.Sources, SourceLinked)
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

	got, err := Run(context.Background(), deps, Request{Project: "proj-a", Query: "keyword", Top: 10, Sources: []string{SourceVault, SourceEngramFTS}})
	if err != nil {
		t.Fatalf("Run returned an error; want nil (not_provisioned must degrade, not fail): %v", err)
	}
	if got.VaultStatus != VaultStatusNotProvisioned {
		t.Fatalf("VaultStatus = %q, want %q", got.VaultStatus, VaultStatusNotProvisioned)
	}
	if len(got.Results) != 1 || !hasSource(got.Results[0], SourceEngramFTS) {
		t.Fatalf("Results = %+v, want exactly one engram-fts-sourced row", got.Results)
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

// hasSource reports whether row names source among the sources that found
// it, without assuming it is the only one -- a row may now be named by
// more than one source (R-006's amended merge).
func hasSource(row ResultRow, source string) bool {
	for _, s := range row.Sources {
		if s == source {
			return true
		}
	}
	return false
}

// TestQuery_EngramRowShipsAnExtractNotTheWholeBody is the payload fix.
// mergeResults assigned Snippet: er.Content, so a query put every matched
// observation body on the wire in full. Measured on the live corpus for
// "canonical identity": 30,075 of 33,877 response bytes -- 88.8% -- were
// Engram bodies, and the largest single body was 42,757 bytes.
func TestQuery_EngramRowShipsAnExtractNotTheWholeBody(t *testing.T) {
	body := strings.Repeat("padding ", 900) + " the zephyr decision " + strings.Repeat("padding ", 900)
	store := newFixtureEngramStore(t, []fixtureObservation{
		{title: "buried", content: body, project: "proj-a"},
	})
	deps := Deps{Engram: store, RetrieveVault: fakeRetrieveVault(vault.Result{Status: vault.StatusOK}, nil), ResolveLink: NoLinkResolver}

	got, err := Run(context.Background(), deps, Request{Project: "proj-a", Query: "zephyr", Top: 10})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(got.Results))
	}
	row := got.Results[0]
	if len(row.Snippet) >= len(body) {
		t.Fatalf("Snippet is %d bytes against a %d-byte body: the whole observation is still on the wire", len(row.Snippet), len(body))
	}
	if !strings.Contains(row.Snippet, "zephyr") {
		t.Fatalf("Snippet is not centred on the match:\n%q", row.Snippet)
	}
	if !row.SnippetTruncated {
		t.Fatalf("SnippetTruncated = false: a caller cannot tell this preview from the whole memory")
	}
	if row.FullLength != len(body) {
		t.Fatalf("FullLength = %d, want %d: truncation is only honest if the caller is told how much is missing", row.FullLength, len(body))
	}
}

// TestQuery_VaultRowIsNotMarkedTruncated keeps the truncation fields
// meaning one thing. A vault snippet is cut by the vault's own retriever
// before longterm-mem ever sees it, so this module has no full body to
// compare against and must not claim to know one.
func TestQuery_VaultRowIsNotMarkedTruncated(t *testing.T) {
	store := newFixtureEngramStore(t, nil)
	vaultResult := vault.Result{
		Status:     vault.StatusOK,
		Candidates: []vault.Candidate{{PageAddress: "c-000001", AbsolutePath: "/v/c-000001.md", Snippet: "vault side snippet"}},
	}
	deps := Deps{Engram: store, RetrieveVault: fakeRetrieveVault(vaultResult, nil), ResolveLink: NoLinkResolver}

	got, err := Run(context.Background(), deps, Request{Project: "proj-a", Query: "zephyr", Top: 10, Sources: []string{SourceVault, SourceEngramFTS}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Results[0].SnippetTruncated || got.Results[0].FullLength != 0 {
		t.Fatalf("vault row claims a truncation it cannot know about: %+v", got.Results[0])
	}
}

// bigFixture builds n observations whose bodies all match the query and
// are each far larger than one row's share of the response ceiling.
func bigFixture(n int) []fixtureObservation {
	rows := make([]fixtureObservation, 0, n)
	for i := 0; i < n; i++ {
		rows = append(rows, fixtureObservation{
			title:   "row " + string(rune('a'+i)),
			content: strings.Repeat("padding ", 1200) + " the zephyr decision " + strings.Repeat("padding ", 1200),
			project: "proj-a",
		})
	}
	return rows
}

// TestQuery_ResponseNeverExceedsTheCeiling is the hard bound. Capping each
// row is not enough on its own: rows vary by orders of magnitude and a
// caller can ask for fifty of them, so a per-row budget multiplied by an
// unbounded row count is not a bound at all. The ceiling is on the whole
// assembled response, measured in the bytes that actually go on the wire.
func TestQuery_ResponseNeverExceedsTheCeiling(t *testing.T) {
	store := newFixtureEngramStore(t, bigFixture(20))
	candidates := make([]vault.Candidate, 0, 20)
	for i := 0; i < 20; i++ {
		candidates = append(candidates, vault.Candidate{
			PageAddress:  "c-00000" + string(rune('a'+i)),
			AbsolutePath: "/vault/very/long/path/to/a/page/" + strings.Repeat("segment/", 8) + "page.md",
			Snippet:      strings.Repeat("vault snippet text ", 20),
		})
	}
	deps := Deps{
		Engram:        store,
		RetrieveVault: fakeRetrieveVault(vault.Result{Status: vault.StatusOK, Candidates: candidates}, nil),
		ResolveLink:   NoLinkResolver,
	}

	got, err := Run(context.Background(), deps, Request{Project: "proj-a", Query: "zephyr", Top: 20, Sources: []string{SourceVault, SourceEngramFTS}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if len(encoded) > ResponseByteCeiling {
		t.Fatalf("response is %d bytes, over the %d-byte (%d-token) ceiling", len(encoded), ResponseByteCeiling, ResponseTokenCeiling)
	}
	if !hasDiagnostic(got, DiagnosticResponseCapped) {
		t.Fatalf("the response was cut to fit and did not say so; diagnostics = %+v", got.Diagnostics)
	}
}

// TestQuery_CeilingKeepsTheMergeOrder guards the one thing the ceiling
// must not do. Dropping the tail of an over-budget response preserves
// which rows outrank which; reordering or re-scoring to fit more in would
// be re-ranking, which this module is forbidden to do (R-006, D8).
func TestQuery_CeilingKeepsTheMergeOrder(t *testing.T) {
	store := newFixtureEngramStore(t, bigFixture(20))
	deps := Deps{
		Engram: store,
		RetrieveVault: fakeRetrieveVault(vault.Result{
			Status:     vault.StatusOK,
			Candidates: []vault.Candidate{{PageAddress: "c-first", AbsolutePath: "/v/first.md", Snippet: "first"}},
		}, nil),
		ResolveLink: NoLinkResolver,
	}

	got, err := Run(context.Background(), deps, Request{Project: "proj-a", Query: "zephyr", Top: 20, Sources: []string{SourceVault, SourceEngramFTS}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got.Results) == 0 {
		t.Fatalf("the ceiling emptied the response entirely")
	}
	if !hasSource(got.Results[0], SourceVault) || got.Results[0].PageAddress != "c-first" {
		t.Fatalf("Results[0] = %+v, want the vault row still first", got.Results[0])
	}
	for i, row := range got.Results {
		if row.Rank != i+1 {
			t.Fatalf("Results[%d].Rank = %d, want %d: the ceiling must not renumber what it kept", i, row.Rank, i+1)
		}
	}
}

// TestQuery_SmallResponseIsUntouched keeps the ceiling from being a second
// cap on ordinary calls. A response that already fits must come back whole
// and must not claim it was cut.
func TestQuery_SmallResponseIsUntouched(t *testing.T) {
	store := newFixtureEngramStore(t, []fixtureObservation{
		{title: "small", content: "a short note about zephyr", project: "proj-a"},
	})
	deps := Deps{Engram: store, RetrieveVault: fakeRetrieveVault(vault.Result{Status: vault.StatusOK}, nil), ResolveLink: NoLinkResolver}

	got, err := Run(context.Background(), deps, Request{Project: "proj-a", Query: "zephyr", Top: 10})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got.Results) != 1 || got.Results[0].Snippet != "a short note about zephyr" {
		t.Fatalf("a response well under the ceiling was altered: %+v", got.Results)
	}
	if hasDiagnostic(got, DiagnosticResponseCapped) {
		t.Fatalf("unexpected %s on a response that already fits", DiagnosticResponseCapped)
	}
}

// TestQuery_ExcludeTypesFiltersAndSaysSo. Filtering is not re-ranking:
// D8 forbids fusing scores across sources, not declining to return a row,
// and the rows that survive keep their merge order. But a corpus quietly
// narrowed is the same failure as a corpus quietly empty, so an applied
// filter is always reported.
func TestQuery_ExcludeTypesFiltersAndSaysSo(t *testing.T) {
	store := newFixtureEngramStoreTyped(t, []typedObservation{
		{title: "the summary", content: "zephyr came up in this session", project: "proj-a", obsType: "session_summary"},
		{title: "the decision", content: "zephyr was chosen deliberately", project: "proj-a", obsType: "decision"},
	})
	deps := Deps{Engram: store, RetrieveVault: fakeRetrieveVault(vault.Result{Status: vault.StatusOK}, nil), ResolveLink: NoLinkResolver}

	got, err := Run(context.Background(), deps, Request{Project: "proj-a", Query: "zephyr", Top: 10, ExcludeTypes: []string{"session_summary"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got.Results) != 1 || got.Results[0].Title != "the decision" {
		t.Fatalf("Results = %+v, want only the non-excluded type", got.Results)
	}
	if !hasDiagnostic(got, DiagnosticTypesExcluded) {
		t.Fatalf("the corpus was narrowed and did not say so; diagnostics = %+v", got.Diagnostics)
	}
}

// TestQuery_NoFilterByDefault. The default excludes nothing, and that is
// a measured choice rather than caution: on the live corpus
// session_summary is 71 of 581 rows (12%) and took 5 of 36 top-5 slots
// across eight real queries (14%). bm25 is already ranking it at about
// its share, so a default exclusion would remove real answers to buy a
// relevance gain the measurement does not show.
func TestQuery_NoFilterByDefault(t *testing.T) {
	store := newFixtureEngramStoreTyped(t, []typedObservation{
		{title: "the summary", content: "zephyr came up in this session", project: "proj-a", obsType: "session_summary"},
	})
	deps := Deps{Engram: store, RetrieveVault: fakeRetrieveVault(vault.Result{Status: vault.StatusOK}, nil), ResolveLink: NoLinkResolver}

	got, err := Run(context.Background(), deps, Request{Project: "proj-a", Query: "zephyr", Top: 10})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got.Results) != 1 {
		t.Fatalf("a query with no filter lost a row: %+v", got.Results)
	}
	if hasDiagnostic(got, DiagnosticTypesExcluded) {
		t.Fatalf("unexpected %s on an unfiltered query", DiagnosticTypesExcluded)
	}
}

// TestUnknownSourceIsRefusedNotIgnored guards R-060: naming a source this
// function does not recognise must be refused, not silently dropped from
// the set actually queried. Silently narrowing the corpus to the sources
// that happen to be spelled correctly is exactly the failure this change
// exists to remove.
func TestUnknownSourceIsRefusedNotIgnored(t *testing.T) {
	store := newFixtureEngramStore(t, nil)
	deps := Deps{Engram: store, RetrieveVault: fakeRetrieveVault(vault.Result{Status: vault.StatusOK}, nil), ResolveLink: NoLinkResolver}

	_, err := Run(context.Background(), deps, Request{Project: "proj-a", Query: "zephyr", Sources: []string{"nonsense"}})
	if !errors.Is(err, ErrUnknownSource) {
		t.Fatalf("err = %v, want ErrUnknownSource", err)
	}
}

// TestOmittedSourcesQueriesBothEngramArmsNotVault is R-060's default-set
// scenario, narrowed to what this PR actually ships: engram-embed's own
// retrieval pipeline (internal/embed, internal/vecindex, the embedding
// arm) does not exist until a later PR of this same change, so the
// default this PR ships is engram-fts alone, not "both Engram arms" --
// shipping a default that silently queried a source with nothing behind
// it would be the same failure R-060 forbids for an unknown name, in a
// different costume. What both this PR and R-060's eventual full shape
// share, and what this test actually proves, is the other half: an
// omitted `sources` must never invoke the vault.
func TestOmittedSourcesQueriesBothEngramArmsNotVault(t *testing.T) {
	store := newFixtureEngramStore(t, []fixtureObservation{
		{title: "engram row", content: "zephyr keyword", project: "proj-a"},
	})
	vaultInvoked := false
	deps := Deps{
		Engram: store,
		RetrieveVault: func(context.Context, string, string, int) (vault.Result, error) {
			vaultInvoked = true
			return vault.Result{Status: vault.StatusOK}, nil
		},
		ResolveLink: NoLinkResolver,
	}

	got, err := Run(context.Background(), deps, Request{Project: "proj-a", Query: "zephyr"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if vaultInvoked {
		t.Fatalf("omitted sources invoked the vault; R-060 requires vault only when named explicitly")
	}
	if got.VaultStatus != VaultStatusNotRequested {
		t.Fatalf("VaultStatus = %q, want %q", got.VaultStatus, VaultStatusNotRequested)
	}
	if len(got.Results) != 1 || !hasSource(got.Results[0], SourceEngramFTS) {
		t.Fatalf("Results = %+v, want the one engram-fts row", got.Results)
	}
}

// TestNamingVaultInvokesIt is R-060's other half: a caller that explicitly
// asks for the vault gets it.
func TestNamingVaultInvokesIt(t *testing.T) {
	store := newFixtureEngramStore(t, nil)
	vaultInvoked := false
	deps := Deps{
		Engram: store,
		RetrieveVault: func(context.Context, string, string, int) (vault.Result, error) {
			vaultInvoked = true
			return vault.Result{Status: vault.StatusOK, Candidates: []vault.Candidate{
				{PageAddress: "c-000001", AbsolutePath: "/v/c-000001.md", Snippet: "vault snippet"},
			}}, nil
		},
		ResolveLink: NoLinkResolver,
	}

	got, err := Run(context.Background(), deps, Request{Project: "proj-a", Query: "zephyr", Sources: []string{SourceVault}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !vaultInvoked {
		t.Fatalf("naming the vault did not invoke it")
	}
	if len(got.Results) != 1 || !hasSource(got.Results[0], SourceVault) {
		t.Fatalf("Results = %+v, want the one vault row", got.Results)
	}
}

// TestRowFoundByBothEngramSourcesEmittedOnceAtEarliestRank exercises
// interleaveEngramSources directly: PR-1 only ever gives mergeResults one
// Engram source (engram-fts; Phase 4 adds engram-embed), so this proves
// the round-robin dedup property the merge will rely on once a second
// source exists, rather than waiting for that source to be built to prove
// it at all.
func TestRowFoundByBothEngramSourcesEmittedOnceAtEarliestRank(t *testing.T) {
	shared := ResultRow{EngramID: 42, Title: "shared row"}
	a := engramSourceRows{name: SourceEngramFTS, rows: []ResultRow{
		{EngramID: 1, Title: "fts only"},
		shared,
	}}
	b := engramSourceRows{name: SourceEngramEmbed, rows: []ResultRow{
		shared,
		{EngramID: 2, Title: "embed only"},
	}}

	merged := interleaveEngramSources([]engramSourceRows{a, b})

	var sharedRow *ResultRow
	for i := range merged {
		if merged[i].EngramID == 42 {
			if sharedRow != nil {
				t.Fatalf("EngramID 42 appears more than once: %+v", merged)
			}
			sharedRow = &merged[i]
		}
	}
	if sharedRow == nil {
		t.Fatalf("the shared row is missing entirely: %+v", merged)
	}
	if !hasSource(*sharedRow, SourceEngramFTS) || !hasSource(*sharedRow, SourceEngramEmbed) {
		t.Fatalf("Sources = %v, want both engram-fts and engram-embed named", sharedRow.Sources)
	}
	// fts's own top row (EngramID 1) precedes the shared row in fts's
	// subsequence, so the earliest position the shared row could take is
	// index 1 (round-robin: fts[0], embed[0]=shared -- embed's own first
	// row IS the shared row, so it surfaces at the earliest slot either
	// source offered it).
	if merged[0].EngramID != 1 {
		t.Fatalf("merged[0] = %+v, want fts's own top row first (its native order preserved)", merged[0])
	}
}

// TestLinkedPairEmittedOnceViaMerge is mergeResults's own version of the
// property above (TestQuery_LinkedPairEmittedOnce already covers the
// vault<->Engram promotion-link case end to end); this direct-merge test
// guards mergeResults not double-counting a row consumed by the vault
// section when it also appears in the (later, round-robin-merged) Engram
// section.
func TestLinkedPairEmittedOnceViaMerge(t *testing.T) {
	engramRows := []engram.Row{{ID: 7, Title: "linked observation"}}
	vaultRows := []vault.Candidate{{PageAddress: "c-000042", AbsolutePath: "/v/c-000042.md", Snippet: "vault side"}}
	resolveLink := func(pageAddress string) (int64, bool) {
		if pageAddress == "c-000042" {
			return 7, true
		}
		return 0, false
	}

	merged := mergeResults(true, true, false, vaultRows, engramRows, nil, resolveLink, "linked observation", engram.MatchAll)

	if len(merged) != 1 {
		t.Fatalf("len(merged) = %d, want 1 (linked pair collapsed); got %+v", len(merged), merged)
	}
	if !hasSource(merged[0], SourceLinked) {
		t.Fatalf("Sources = %v, want %q", merged[0].Sources, SourceLinked)
	}
}
