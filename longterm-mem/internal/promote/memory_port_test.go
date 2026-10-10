package promote

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// fakeMemory is the Memory port promote owns, implemented without a database: promotion and
// propagation depend on the memory model and on that port, not on the store that serves them.
type fakeMemory struct {
	live       []memory.Observation
	all        []memory.Observation
	edges      map[int64][]memory.Edge
	listErr    error
	historyErr error
	edgesErr   error

	listed   []string
	history  []string
	edgesFor []int64
}

func (f *fakeMemory) ListObservations(project string) ([]memory.Observation, error) {
	f.listed = append(f.listed, project)
	return f.live, f.listErr
}

func (f *fakeMemory) ObservationsIncludingDeleted(project string) ([]memory.Observation, error) {
	f.history = append(f.history, project)
	return f.all, f.historyErr
}

func (f *fakeMemory) RelatedEdges(observationID int64) ([]memory.Edge, error) {
	f.edgesFor = append(f.edgesFor, observationID)
	return f.edges[observationID], f.edgesErr
}

func TestSync_ReadsTheLiveObservationsThroughItsMemoryPort(t *testing.T) {
	vaultRoot := t.TempDir()
	clock := &fakeClock{at: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)}

	curated := memory.Observation{ID: 11, Type: "decision", Title: "Curated", Content: "Curated body.", Project: "labdrian-sdd-overlay", RevisionCount: 1, TopicKey: "longterm-mem/curated"}
	process := memory.Observation{ID: 12, Type: "discovery", Title: "Process note", Content: "Not curated.", Project: "labdrian-sdd-overlay", RevisionCount: 1, TopicKey: "sdd/some-change/progress"}
	mem := &fakeMemory{live: []memory.Observation{curated, process}}

	report, err := Sync(context.Background(), Deps{Memory: mem, Writer: &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}, Precedence: &memPrecedence{}, Clock: clock, AddressMap: &memAddressMap{}, Addresses: staticAddress(testAddress)}}, "labdrian-sdd-overlay")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(report.Promoted) != 1 || report.Skipped != 1 {
		t.Fatalf("report = %+v, want the curated observation promoted and the process note skipped", report)
	}
	if len(mem.listed) != 1 || mem.listed[0] != "labdrian-sdd-overlay" {
		t.Fatalf("the port was asked for %v, want exactly the project once", mem.listed)
	}
	page, err := os.ReadFile(filepath.Join(vaultRoot, report.Promoted[0].Page.Path))
	if err != nil {
		t.Fatalf("read the promoted page: %v", err)
	}
	if !strings.Contains(string(page), "Curated body.") {
		t.Fatalf("the page does not carry the body of the observation the port returned:\n%s", page)
	}
}

func TestSync_AFailingMemoryPortIsTheRunsError(t *testing.T) {
	broken := errors.New("memory store offline")
	deps := Deps{Memory: &fakeMemory{listErr: broken}, Writer: &Writer{VaultRoot: t.TempDir(), Store: PrecedenceStore{}, Precedence: &memPrecedence{}, Clock: &fakeClock{at: testInstant}}}

	if _, err := Sync(context.Background(), deps, "labdrian-sdd-overlay"); !errors.Is(err, broken) {
		t.Fatalf("Sync = %v, want an error that wraps the port's own", err)
	}
	if _, err := Plan(context.Background(), deps, "labdrian-sdd-overlay"); !errors.Is(err, broken) {
		t.Fatalf("Plan = %v, want an error that wraps the port's own", err)
	}
}

func TestPropagate_ReadsTheHistoryAndTheEdgesThroughItsMemoryPort(t *testing.T) {
	vaultRoot := t.TempDir()
	clock := &fakeClock{at: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)}

	older := memory.Observation{ID: 21, SyncID: "sync-older", Type: "decision", Title: "Older", Content: "Older body.", Project: "labdrian-sdd-overlay", RevisionCount: 1, CreatedAt: "2026-08-01 00:00:00"}
	newer := memory.Observation{ID: 22, SyncID: "sync-newer", Type: "decision", Title: "Newer", Content: "Newer body.", Project: "labdrian-sdd-overlay", RevisionCount: 1, CreatedAt: "2026-08-15 00:00:00"}
	retired := memory.Observation{ID: 23, SyncID: "sync-retired", Type: "decision", Title: "Retired", Content: "Retired body.", Project: "labdrian-sdd-overlay", RevisionCount: 1, DeletedAt: "2026-08-20 00:00:00"}
	mem := &fakeMemory{
		all: []memory.Observation{older, newer, retired},
		edges: map[int64][]memory.Edge{
			21: {{Relation: "supersedes", SourceSyncID: "sync-newer", TargetSyncID: "sync-older"}},
			22: {{Relation: "supersedes", SourceSyncID: "sync-newer", TargetSyncID: "sync-older"}},
		},
	}
	precedence := PrecedenceStore{}
	olderPage := seedPromotedPage(t, vaultRoot, precedence, older, "c-000001")
	seedPromotedPage(t, vaultRoot, precedence, newer, "c-000002")
	retiredPage := seedPromotedPage(t, vaultRoot, precedence, retired, "c-000003")

	report, err := Propagate(context.Background(), Deps{Memory: mem, Writer: &Writer{VaultRoot: vaultRoot, Store: precedence, Precedence: &memPrecedence{}, Clock: clock}}, "labdrian-sdd-overlay")
	if err != nil {
		t.Fatalf("Propagate: %v", err)
	}
	if len(report.Patched) != 2 || report.Patched[0] != "c-000001" || report.Patched[1] != "c-000003" {
		t.Fatalf("Patched = %v, want the superseded page and the retired one, and not the survivor", report.Patched)
	}
	if len(mem.history) != 1 || mem.history[0] != "labdrian-sdd-overlay" {
		t.Fatalf("the history was asked for %v, want exactly the project once", mem.history)
	}

	superseded, err := os.ReadFile(filepath.Join(vaultRoot, olderPage.Path))
	if err != nil {
		t.Fatalf("read the superseded page: %v", err)
	}
	if !strings.Contains(string(superseded), "status: superseded") || !strings.Contains(string(superseded), "[[c-000002|Newer]]") {
		t.Fatalf("the superseded page was not patched from the edge the port returned:\n%s", superseded)
	}
	archived, err := os.ReadFile(filepath.Join(vaultRoot, retiredPage.Path))
	if err != nil {
		t.Fatalf("read the retired page: %v", err)
	}
	if !strings.Contains(string(archived), "status: archived") {
		t.Fatalf("the retired page was not archived:\n%s", archived)
	}
}

func TestPropagate_AFailingMemoryPortIsReportedNotSwallowed(t *testing.T) {
	broken := errors.New("memory store offline")
	vaultRoot := t.TempDir()
	clock := &fakeClock{at: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)}
	writer := &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}, Precedence: &memPrecedence{}, Clock: clock}

	t.Run("the history cannot be read", func(t *testing.T) {
		_, err := Propagate(context.Background(), Deps{Memory: &fakeMemory{historyErr: broken}, Writer: writer}, "labdrian-sdd-overlay")
		if !errors.Is(err, broken) {
			t.Fatalf("Propagate = %v, want an error that wraps the port's own", err)
		}
	})

	t.Run("the edges of one observation cannot be read", func(t *testing.T) {
		obs := memory.Observation{ID: 31, SyncID: "sync-31", Type: "decision", Title: "Promoted", Content: "Body.", Project: "labdrian-sdd-overlay", RevisionCount: 1}
		precedence := PrecedenceStore{}
		seedPromotedPage(t, vaultRoot, precedence, obs, "c-000031")
		writer := &Writer{VaultRoot: vaultRoot, Store: precedence, Precedence: &memPrecedence{}, Clock: clock}

		report, err := Propagate(context.Background(), Deps{Memory: &fakeMemory{all: []memory.Observation{obs}, edgesErr: broken}, Writer: writer}, "labdrian-sdd-overlay")
		if err == nil {
			t.Fatal("Propagate returned no error although an observation could not be decided")
		}
		if len(report.Failed) != 1 || report.Failed[0].ObservationID != 31 || !errors.Is(report.Failed[0].Err, broken) {
			t.Fatalf("Failed = %+v, want observation 31 with the port's error", report.Failed)
		}
	})
}
