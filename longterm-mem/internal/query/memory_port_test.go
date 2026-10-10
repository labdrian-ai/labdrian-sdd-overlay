package query

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vecindex"
)

// fakeMemory is the Memory port query owns, implemented without a database or an index: the use case
// depends on the memory model and on these ports, not on the store that serves them.
type fakeMemory struct {
	search       memory.SearchResult
	searchErr    error
	standings    map[int64]memory.Standing
	standingsErr error
	degraded     bool
	cause        string
	live         int
	liveByID     map[int64]memory.Observation
	snapshotErr  error

	searchedProject string
	searchedQuery   string
	searchedLimit   int
	searchedExclude []string
	standingsFor    []int64
	snapshotProject string
	snapshotIDs     []int64
}

func (f *fakeMemory) Search(project, query string, limit int, excludeTypes ...string) (memory.SearchResult, error) {
	f.searchedProject, f.searchedQuery, f.searchedLimit, f.searchedExclude = project, query, limit, excludeTypes
	return f.search, f.searchErr
}

func (f *fakeMemory) Standings(ids []int64) (map[int64]memory.Standing, error) {
	f.standingsFor = ids
	return f.standings, f.standingsErr
}

func (f *fakeMemory) Degraded() (bool, string) { return f.degraded, f.cause }

func (f *fakeMemory) CoverageSnapshot(project string, indexedIDs []int64) (int, map[int64]memory.Observation, error) {
	f.snapshotProject, f.snapshotIDs = project, indexedIDs
	return f.live, f.liveByID, f.snapshotErr
}

func ftsRows() []memory.Row {
	return []memory.Row{
		{ID: 7, Title: "first", Content: "first body", Project: "p", Snippet: "first body", ContentLength: 10},
		{ID: 9, Title: "second", Content: "second body", Project: "p", Snippet: "second body", ContentLength: 11},
	}
}

func TestRun_SearchesThroughItsMemoryPortAndForwardsTheRequest(t *testing.T) {
	mem := &fakeMemory{search: memory.SearchResult{Rows: ftsRows(), MatchMode: memory.MatchAny, DroppedTokens: []string{"the"}}}
	deps := Deps{Memory: mem, ResolveLink: NoLinkResolver}

	result, err := Run(context.Background(), deps, Request{
		Project: "p", Query: "the alpha beta", Top: 3, ExcludeTypes: []string{"session_summary"}, Sources: []string{SourceEngramFTS},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if mem.searchedProject != "p" || mem.searchedQuery != "the alpha beta" || mem.searchedLimit != 3 || !reflect.DeepEqual(mem.searchedExclude, []string{"session_summary"}) {
		t.Errorf("the port was asked for (%q, %q, %d, %v), want the request forwarded as it was made", mem.searchedProject, mem.searchedQuery, mem.searchedLimit, mem.searchedExclude)
	}
	if len(result.Results) != 2 || result.Results[0].EngramID != 7 || result.Results[1].EngramID != 9 {
		t.Fatalf("Results = %+v, want the two rows of the port in its own order", result.Results)
	}
	for _, code := range []string{DiagnosticSearchWidened, DiagnosticTypesExcluded, DiagnosticSearchStopwordsDropped} {
		if !hasDiagnostic(result, code) {
			t.Errorf("diagnostics %+v lack %q, which the widened, filtered, stopword-dropping search the port reported calls for", result.Diagnostics, code)
		}
	}
	if hasDiagnostic(result, DiagnosticEngramDegradedSnapshot) {
		t.Errorf("a store that is not degraded produced a degraded-snapshot diagnostic: %+v", result.Diagnostics)
	}
}

func TestRun_ASearchTheMemoryPortCannotAnswerIsTheCallsError(t *testing.T) {
	broken := errors.New("memory store offline")
	deps := Deps{Memory: &fakeMemory{searchErr: broken}, ResolveLink: NoLinkResolver}

	_, err := Run(context.Background(), deps, Request{Project: "p", Query: "alpha", Sources: []string{SourceEngramFTS}})
	if !errors.Is(err, broken) {
		t.Fatalf("Run = %v, want an error that wraps the port's own", err)
	}
}

func TestRun_NamesADegradedMemoryAndItsCause(t *testing.T) {
	mem := &fakeMemory{search: memory.SearchResult{Rows: ftsRows(), MatchMode: memory.MatchAll}, degraded: true, cause: "the writer is offline"}

	result, err := Run(context.Background(), Deps{Memory: mem, ResolveLink: NoLinkResolver}, Request{Project: "p", Query: "alpha", Sources: []string{SourceEngramFTS}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var found bool
	for _, d := range result.Diagnostics {
		if d.Code == DiagnosticEngramDegradedSnapshot {
			found = true
			if !strings.Contains(d.Detail, "the writer is offline") {
				t.Errorf("detail = %q, want it to carry the cause the port reported", d.Detail)
			}
		}
	}
	if !found {
		t.Fatalf("diagnostics %+v lack the degraded-snapshot one", result.Diagnostics)
	}
}

func TestRun_AttachesTheStandingsThePortReportsToTheRowsItAskedAbout(t *testing.T) {
	mem := &fakeMemory{
		search: memory.SearchResult{Rows: ftsRows(), MatchMode: memory.MatchAll},
		standings: map[int64]memory.Standing{
			7: {SupersededBy: []memory.Neighbour{{ID: 9, Title: "second"}}},
			9: {},
		},
	}

	result, err := Run(context.Background(), Deps{Memory: mem, ResolveLink: NoLinkResolver}, Request{Project: "p", Query: "alpha", Sources: []string{SourceEngramFTS}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !reflect.DeepEqual(mem.standingsFor, []int64{7, 9}) {
		t.Errorf("standings were asked for %v, want the ids of the rows in rank order", mem.standingsFor)
	}
	if result.Results[0].Standing == nil || !reflect.DeepEqual(result.Results[0].Standing.SupersededBy, []memory.Neighbour{{ID: 9, Title: "second"}}) {
		t.Errorf("row 7 standing = %+v, want superseded by 9", result.Results[0].Standing)
	}
	if result.Results[1].Standing != nil {
		t.Errorf("row 9 standing = %+v, want none: the port reported an empty standing, which is nothing to tell a reader", result.Results[1].Standing)
	}
}

func TestRun_AnUnreadableLedgerCostsTheAnnotationNotTheAnswer(t *testing.T) {
	broken := errors.New("ledger offline")
	mem := &fakeMemory{search: memory.SearchResult{Rows: ftsRows(), MatchMode: memory.MatchAll}, standingsErr: broken}

	result, err := Run(context.Background(), Deps{Memory: mem, ResolveLink: NoLinkResolver}, Request{Project: "p", Query: "alpha", Sources: []string{SourceEngramFTS}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Results) != 2 {
		t.Fatalf("Results = %+v, want both rows although the ledger could not be read", result.Results)
	}
	if !hasDiagnostic(result, DiagnosticRelationsUnreadable) {
		t.Fatalf("diagnostics %+v lack %q: silence would read as nothing being superseded", result.Diagnostics, DiagnosticRelationsUnreadable)
	}
}

func TestRun_TheEmbeddingArmCountsCoverageThroughTheMemoryPort(t *testing.T) {
	mem := &fakeMemory{search: memory.SearchResult{MatchMode: memory.MatchAll}, live: 7}
	deps := Deps{
		Memory:      mem,
		ResolveLink: NoLinkResolver,
		StateDir:    t.TempDir(),
		LoadIndex:   func(string) (*vecindex.Index, error) { return nil, vecindex.ErrNoIndex },
	}

	result, err := Run(context.Background(), deps, Request{Project: "p", Query: "alpha", Sources: []string{SourceEngramEmbed}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if mem.snapshotProject != "p" || len(mem.snapshotIDs) != 0 {
		t.Errorf("the snapshot was asked for (%q, %v), want the project and no indexed id: there is no index", mem.snapshotProject, mem.snapshotIDs)
	}
	want := []Coverage{{Source: SourceEngramEmbed, Live: 7, Indexed: 0, Unindexed: 7, BuiltAt: embeddingIndexNeverBuilt}}
	if !reflect.DeepEqual(result.Coverage, want) {
		t.Fatalf("Coverage = %+v, want %+v", result.Coverage, want)
	}
}

func TestRun_AnUnreadableSnapshotIsReportedAsZerosThatAreNotMeasurements(t *testing.T) {
	broken := errors.New("snapshot offline")
	mem := &fakeMemory{search: memory.SearchResult{MatchMode: memory.MatchAll}, snapshotErr: broken}
	deps := Deps{
		Memory:      mem,
		ResolveLink: NoLinkResolver,
		StateDir:    t.TempDir(),
		LoadIndex:   func(string) (*vecindex.Index, error) { return nil, vecindex.ErrNoIndex },
	}

	result, err := Run(context.Background(), deps, Request{Project: "p", Query: "alpha", Sources: []string{SourceEngramEmbed}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !hasDiagnostic(result, DiagnosticCoverageUnreadable) {
		t.Fatalf("diagnostics %+v lack %q", result.Diagnostics, DiagnosticCoverageUnreadable)
	}
	if len(result.Coverage) != 1 || result.Coverage[0].Live != 0 || result.Coverage[0].Unindexed != 0 {
		t.Fatalf("Coverage = %+v, want one entry of zeros: an unreadable snapshot is not a measurement", result.Coverage)
	}
}

// A query that needs the memory and was not given one fails in its own words, whether the port is absent
// or holds a nil pointer (a store that failed to open and was assigned anyway), instead of panicking
// inside the store.
func TestRun_RefusesToSearchWithoutAMemory(t *testing.T) {
	var notThere *fakeMemory
	for name, mem := range map[string]Memory{"no port": nil, "a nil pointer": notThere} {
		for _, source := range []string{SourceEngramFTS, SourceEngramEmbed} {
			_, err := Run(context.Background(), Deps{Memory: mem, ResolveLink: NoLinkResolver, StateDir: t.TempDir()}, Request{Project: "p", Query: "alpha", Sources: []string{source}})
			if !errors.Is(err, ErrNoMemory) {
				t.Errorf("Run over %s for %s = %v, want ErrNoMemory", name, source, err)
			}
		}
	}
}

// A vault-only query reads nothing from the memory, so it needs none: a link from a vault page to an
// observation is only followed against the observations a memory source returned, and there are none. The
// refusal above is for the sources that read the memory, not for the call as a whole.
func TestRun_AVaultOnlyQueryNeedsNoMemory(t *testing.T) {
	deps := Deps{
		RetrieveVault: fakeRetrieveVault(vault.Result{Status: vault.StatusOK, Candidates: []vault.Candidate{{PageAddress: "c-000042", Snippet: "vault side"}}}, nil),
		ResolveLink:   func(string) (int64, bool) { return 7, true },
	}

	result, err := Run(context.Background(), deps, Request{Project: "p", Query: "alpha", Sources: []string{SourceVault}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Results) != 1 || result.Results[0].PageAddress != "c-000042" {
		t.Fatalf("Results = %+v, want the vault row", result.Results)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %+v, want none: nothing was asked of the memory", result.Diagnostics)
	}
}
