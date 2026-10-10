package promote

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// portObservation is an eligible observation: pinned, so Eligible answers true.
func portObservation(id int64) memory.Observation {
	return memory.Observation{ID: id, Type: "decision", Title: "Port Fixture", Content: "Body.", Project: "labdrian-sdd-overlay", RevisionCount: 1, Pinned: true}
}

// vaultIsUntouched fails the test unless nothing was written under root.
func vaultIsUntouched(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("the vault holds %d entries after a refused call, want none", len(entries))
	}
}

// A Writer handed no repository for its precedence store is refused before it writes anything: a promotion
// that published a page and then found it could not record who wrote it would leave a page of unknown
// provenance, which promotion refuses from then on.
func TestPromote_RefusesAWriterWithoutAPrecedenceRepository(t *testing.T) {
	vaultRoot := t.TempDir()
	w := &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}, Clock: &fakeClock{at: testInstant}, Addresses: staticAddress(testAddress)}

	_, err := w.Promote(t.Context(), portObservation(901), false)
	if !errors.Is(err, errNoPrecedenceRepository) {
		t.Fatalf("Promote = %v, want errNoPrecedenceRepository", err)
	}
	vaultIsUntouched(t, vaultRoot)
}

// A repository that is a typed nil (a nil pointer in the interface) is as missing as an absent one: calling
// it would panic after the page was written.
func TestPromote_RefusesATypedNilPrecedenceRepository(t *testing.T) {
	var repo *memPrecedence
	vaultRoot := t.TempDir()
	w := &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}, Precedence: repo, Clock: &fakeClock{at: testInstant}, Addresses: staticAddress(testAddress)}

	if _, err := w.Promote(t.Context(), portObservation(902), false); !errors.Is(err, errNoPrecedenceRepository) {
		t.Fatalf("Promote = %v, want errNoPrecedenceRepository", err)
	}
	vaultIsUntouched(t, vaultRoot)
}

// Sync is refused the same way, before it lists a thing.
func TestSync_RefusesAWriterWithoutAPrecedenceRepository(t *testing.T) {
	mem := &fakeMemory{}
	deps := Deps{Memory: mem, Writer: &Writer{VaultRoot: t.TempDir(), Store: PrecedenceStore{}, Clock: &fakeClock{at: testInstant}}}

	if _, err := Sync(t.Context(), deps, "labdrian-sdd-overlay"); !errors.Is(err, errNoPrecedenceRepository) {
		t.Fatalf("Sync = %v, want errNoPrecedenceRepository", err)
	}
}

// Propagate asks for no clock, but it persists the precedence store after patching, so it needs the
// repository and refuses without it before it patches a page: a patched page whose new fingerprint was
// never recorded reads as a human's edit from then on.
func TestPropagate_RefusesAWriterWithoutAPrecedenceRepository(t *testing.T) {
	deps := Deps{Memory: &fakeMemory{}, Writer: &Writer{VaultRoot: t.TempDir(), Store: PrecedenceStore{}}}

	if _, err := Propagate(t.Context(), deps, "labdrian-sdd-overlay"); !errors.Is(err, errNoPrecedenceRepository) {
		t.Fatalf("Propagate = %v, want errNoPrecedenceRepository", err)
	}
}

// Reconcile is refused without the repository before it reads a page, absent or typed nil.
func TestReconcile_RefusesWithoutAPrecedenceRepository(t *testing.T) {
	var typedNil *memPrecedence
	for name, repo := range map[string]PrecedenceRepository{"absent": nil, "typed nil": typedNil} {
		t.Run(name, func(t *testing.T) {
			if _, err := Reconcile(t.TempDir(), "labdrian-sdd-overlay", "c-000001", repo); !errors.Is(err, errNoPrecedenceRepository) {
				t.Fatalf("Reconcile = %v, want errNoPrecedenceRepository", err)
			}
		})
	}
}

// A repository that cannot persist the store after an update is an error the promotion reports, and the
// cause stays reachable: the page was already rewritten, and the operator needs to know the record of it
// was not kept.
func TestPromote_ReportsAnUpdateWhosePrecedenceCannotBePersisted(t *testing.T) {
	vaultRoot := t.TempDir()
	clock := &fakeClock{at: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)}
	repo := &memPrecedence{}
	w := &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}, Precedence: repo, Clock: clock, Addresses: staticAddress(testAddress)}
	obs := portObservation(903)
	if _, err := w.Promote(t.Context(), obs, false); err != nil {
		t.Fatalf("Promote (create): %v", err)
	}

	clock.Set(time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC))
	obs.RevisionCount = 2
	obs.Content = "Revised."
	repo.saveErr = errTestSave
	if _, err := w.Promote(t.Context(), obs, false); !errors.Is(err, errTestSave) {
		t.Fatalf("Promote (update) = %v, want the persistence failure", err)
	}
}

// Propagate reports a store it could not persist, naming what it was doing and keeping the cause.
func TestPropagate_ReportsAPrecedenceStoreThatCannotBePersisted(t *testing.T) {
	vaultRoot := t.TempDir()
	mem, ids := newFixtureEngramStore(t, []fixtureObs{
		{title: "Old Decision", content: "Old body.", project: "labdrian-sdd-overlay", obsType: "decision", revisionCount: 1, syncID: "sync-old", createdAt: "2026-08-01 00:00:00", topicKey: "longterm-mem/old-decision"},
		{title: "New Decision", content: "New body.", project: "labdrian-sdd-overlay", obsType: "decision", revisionCount: 1, syncID: "sync-new", createdAt: "2026-08-15 00:00:00", topicKey: "longterm-mem/new-decision"},
	}, []fixtureRelation{
		{syncID: "rel-1", sourceSyncID: "sync-new", targetSyncID: "sync-old", relation: "supersedes"},
	})
	precedence := PrecedenceStore{}
	seedPromotedPage(t, vaultRoot, precedence, memory.Observation{ID: ids[0], Type: "decision", Title: "Old Decision", Content: "Old body.", Project: "labdrian-sdd-overlay", RevisionCount: 1}, "c-000001")
	seedPromotedPage(t, vaultRoot, precedence, memory.Observation{ID: ids[1], Type: "decision", Title: "New Decision", Content: "New body.", Project: "labdrian-sdd-overlay", RevisionCount: 1}, "c-000002")
	w := &Writer{VaultRoot: vaultRoot, Store: precedence, Precedence: &memPrecedence{saveErr: errTestSave}}

	_, err := Propagate(t.Context(), Deps{Memory: mem, Writer: w}, "labdrian-sdd-overlay")
	if !errors.Is(err, errTestSave) {
		t.Fatalf("Propagate = %v, want the persistence failure", err)
	}
}
