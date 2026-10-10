package promote

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// testAddress is the address the tests that do not care which one they get are handed.
const testAddress = "c-000042"

// staticAddress is an AddressAllocator that hands out the same address every time.
type staticAddress string

func (a staticAddress) NextAddress(context.Context) (string, error) { return string(a), nil }

// sequentialAddresses is an AddressAllocator that hands out c-000001, c-000002, ...: a run that promotes
// several observations needs one address for each, not a collision.
type sequentialAddresses struct{ taken int }

func (a *sequentialAddresses) NextAddress(context.Context) (string, error) {
	a.taken++
	return fmt.Sprintf("c-%06d", a.taken), nil
}

// countingAddresses wraps an allocator and records how it was asked: how many times, and with which
// context. It is not synchronized: a test reads it after the promotion it drives has returned, from the
// goroutine that ran it.
type countingAddresses struct {
	AddressAllocator
	asked int
	// lastAskContext is the context the allocator was last asked with: what the test inspects, not
	// state shared with the promotion.
	lastAskContext context.Context
}

func (a *countingAddresses) NextAddress(ctx context.Context) (string, error) {
	a.asked++
	a.lastAskContext = ctx
	return a.AddressAllocator.NextAddress(ctx)
}

// failingAddresses is an AddressAllocator that answers err, or the address it is given with it.
type failingAddresses struct {
	address string
	err     error
}

func (a failingAddresses) NextAddress(context.Context) (string, error) { return a.address, a.err }

// addressFunc is an AddressAllocator that is a function, with a value receiver: a nil one is a port that
// is not there although no pointer is nil.
type addressFunc func(context.Context) (string, error)

func (f addressFunc) NextAddress(ctx context.Context) (string, error) { return f(ctx) }

// TestAllocateAddress_RePromotionReusesExistingAddress: R-028 scenario 2. The
// allocator is counted, and reuse must never ask it: a re-promotion that
// spent an address would burn the vault's counter on every sync.
func TestAllocateAddress_RePromotionReusesExistingAddress(t *testing.T) {
	vaultRoot := t.TempDir()

	memoryDir := filepath.Join(vaultRoot, "wiki", "memory")
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", memoryDir, err)
	}
	obs := memory.Observation{ID: 101, Type: "decision", Title: "Already Promoted", Content: "Body.", Project: "labdrian-sdd-overlay"}
	page, err := EmitPage(obs, "c-000099", nil, testInstant)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vaultRoot, page.Path), []byte(page.Frontmatter+page.Body), 0o644); err != nil {
		t.Fatalf("write pre-promoted page: %v", err)
	}

	allocator := &countingAddresses{AddressAllocator: staticAddress(testAddress)}
	address, err := allocateAddress(t.Context(), vaultRoot, "labdrian-sdd-overlay", 101, allocator, &memAddressMap{}, testInstant)
	if err != nil {
		t.Fatalf("allocateAddress: %v", err)
	}
	if address != "c-000099" {
		t.Fatalf("address = %q, want the reused %q (the allocator must not have been asked)", address, "c-000099")
	}
	if allocator.asked != 0 {
		t.Fatalf("the allocator was asked %d time(s) for a page that already has an address", allocator.asked)
	}
}

// The context a promotion is called with is the one the allocator is asked with, so a caller that cancels
// abandons the allocation instead of waiting out the adapter's own timeout.
func TestPromote_AsksTheAllocatorWithTheCallersContext(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(t.Context(), key{}, "the caller's")
	allocator := &countingAddresses{AddressAllocator: staticAddress(testAddress)}
	w := &Writer{VaultRoot: t.TempDir(), Store: PrecedenceStore{}, Precedence: &memPrecedence{}, AddressMap: &memAddressMap{}, Addresses: allocator, Clock: &fakeClock{at: testInstant}}
	obs := memory.Observation{ID: 704, Type: "decision", Title: "Context", Content: "Body.", Project: "labdrian-sdd-overlay", RevisionCount: 1, Pinned: true}

	if _, err := w.Promote(ctx, obs, false); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if allocator.lastAskContext == nil || allocator.lastAskContext.Value(key{}) != "the caller's" {
		t.Errorf("the allocator was asked with %v, want the context Promote was called with", allocator.lastAskContext)
	}
}

// The allocator is the one place an address comes from. A failure of the allocator is promotion's failure,
// in promotion's words (that it records nothing is TestAllocateAddress_NothingAllocatedRecordsNothing).
func TestAllocateAddress_AnAllocatorFailureIsPromotionsAndRecordsNothing(t *testing.T) {
	boom := errors.New("the counter is locked")

	address, err := allocateAddress(t.Context(), t.TempDir(), "labdrian-sdd-overlay", 101, failingAddresses{err: boom}, &memAddressMap{}, testInstant)
	if !errors.Is(err, boom) {
		t.Fatalf("allocateAddress = (%q, %v), want the allocator's own error", address, err)
	}
	if !strings.HasPrefix(err.Error(), "promote: ") {
		t.Errorf("error %q does not say it is promotion's", err)
	}
	if address != "" {
		t.Errorf("address = %q alongside an error, want none", address)
	}
}

// A promotion whose address cannot be had writes nothing at all: no page, no sidecar entry, no catalog or
// log line. This is the writer's half of the port, the half the vault adapter cannot see.
func TestPromote_NoAddressMeansNothingIsWritten(t *testing.T) {
	boom := errors.New("the counter is locked")
	for name, addresses := range map[string]AddressAllocator{
		"the allocator fails":      failingAddresses{err: boom},
		"the allocator is missing": nil,
	} {
		t.Run(name, func(t *testing.T) {
			vaultRoot := t.TempDir()
			store := PrecedenceStore{}
			w := &Writer{VaultRoot: vaultRoot, Store: store, Precedence: &memPrecedence{}, AddressMap: &memAddressMap{}, Addresses: addresses, Clock: &fakeClock{at: testInstant}}
			obs := memory.Observation{ID: 704, Type: "decision", Title: "No Address", Content: "Body.", Project: "labdrian-sdd-overlay", RevisionCount: 1, Pinned: true}

			result, err := w.Promote(t.Context(), obs, false)
			if err == nil {
				t.Fatalf("Promote = %+v, nil error, want the failure to get an address", result)
			}
			if result.Action.Kind != ActionNone {
				t.Errorf("a failed promotion reports Action %v, want ActionNone", result.Action.Kind)
			}
			if entries, readErr := os.ReadDir(vaultRoot); readErr != nil || len(entries) != 0 {
				t.Errorf("the vault holds %d entries after a promotion with no address (read err = %v), want none", len(entries), readErr)
			}
			if len(store) != 0 {
				t.Errorf("the precedence store holds %d entries after a promotion with no address", len(store))
			}
		})
	}
}

// The allocator is a port the caller wires. One that is missing is named, but only when an address is
// actually needed: a page that already has one is re-promoted without it.
func TestAllocateAddress_AMissingAllocatorIsRefusedOnlyWhenOneIsNeeded(t *testing.T) {
	t.Run("a new page needs one", func(t *testing.T) {
		vaultRoot := t.TempDir()
		for name, allocator := range map[string]AddressAllocator{"absent": nil, "a nil pointer": (*sequentialAddresses)(nil), "a nil function": addressFunc(nil)} {
			if address, err := allocateAddress(t.Context(), vaultRoot, "labdrian-sdd-overlay", 101, allocator, &memAddressMap{}, testInstant); !errors.Is(err, errNoAddressAllocator) {
				t.Errorf("%s: allocateAddress = (%q, %v), want errNoAddressAllocator", name, address, err)
			}
		}
	})
	t.Run("a promoted page does not", func(t *testing.T) {
		vaultRoot := t.TempDir()
		page := "---\nengram_id: 101\nproject: labdrian-sdd-overlay\naddress: c-000099\n---\n\nBody.\n"
		memoryDir := filepath.Join(vaultRoot, "wiki", "memory")
		if err := os.MkdirAll(memoryDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", memoryDir, err)
		}
		if err := os.WriteFile(filepath.Join(memoryDir, "c-000099.md"), []byte(page), 0o644); err != nil {
			t.Fatalf("write the promoted page: %v", err)
		}
		address, err := allocateAddress(t.Context(), vaultRoot, "labdrian-sdd-overlay", 101, nil, nil, testInstant)
		if err != nil || address != "c-000099" {
			t.Fatalf("allocateAddress = (%q, %v), want the page's own address without an allocator", address, err)
		}
	})
}

// TestAllocateAddress_ReuseWithoutAddressFails: a page matching this engram_id +
// project whose frontmatter carries no address must fail the promotion,
// mirroring the fresh path's "produced no address" guard, never succeed
// with an empty address (findings R2/R3/R4 reuse-empty-address). No
// allocator fixture exists: silently falling through to a fresh
// allocation for an already-promoted page would error too.
func TestAllocateAddress_ReuseWithoutAddressFails(t *testing.T) {
	vaultRoot := t.TempDir()
	memoryDir := filepath.Join(vaultRoot, "wiki", "memory")
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", memoryDir, err)
	}
	page := "---\nengram_id: 101\nproject: labdrian-sdd-overlay\n---\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(memoryDir, "c-000099.md"), []byte(page), 0o644); err != nil {
		t.Fatalf("write pre-promoted page: %v", err)
	}

	address, err := allocateAddress(t.Context(), vaultRoot, "labdrian-sdd-overlay", 101, staticAddress(testAddress), &memAddressMap{}, testInstant)
	if err == nil {
		t.Fatalf("allocateAddress = (%q, nil), want an error for a matched page without an address", address)
	}
	if !strings.Contains(err.Error(), "c-000099.md") {
		t.Fatalf("error %q does not name the offending page c-000099.md", err)
	}
}

// TestFindPromotedPage_RevisionRoundTrips: task 7.10 Gap 2 coverage. The
// 7a REFACTOR widened findPromotedPage's return from a bare address to
// promotedPage{Address, Revision}, but address_test.go was never touched
// to exercise Revision directly -- Sync's own tests only ever see it
// indirectly through re-promotion decisions.
func TestFindPromotedPage_RevisionRoundTrips(t *testing.T) {
	vaultRoot := t.TempDir()
	obs := memory.Observation{ID: 201, Type: "decision", Title: "Revisioned", Content: "Body.", Project: "labdrian-sdd-overlay", RevisionCount: 5}
	page, err := EmitPage(obs, "c-000201", nil, testInstant)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	full := filepath.Join(vaultRoot, page.Path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(page.Frontmatter+page.Body), 0o644); err != nil {
		t.Fatalf("write page: %v", err)
	}

	promoted, ok, err := findPromotedPage(vaultRoot, "labdrian-sdd-overlay", 201)
	if err != nil {
		t.Fatalf("findPromotedPage: %v", err)
	}
	if !ok {
		t.Fatalf("findPromotedPage ok = false, want true")
	}
	if promoted.Address != "c-000201" {
		t.Fatalf("Address = %q, want c-000201", promoted.Address)
	}
	if promoted.Revision != 5 {
		t.Fatalf("Revision = %d, want 5 (the promoted page's own engram_revision)", promoted.Revision)
	}
}

// TestFindPromotedPage_MissingRevisionDefaultsToZero: task 7.10 Gap 2
// coverage. A page with no engram_revision field at all (a page promoted
// before D7's engram_revision field existed, or a hand-authored one)
// must report Revision 0, not error -- distinguishing "never recorded" a
// revision from "recorded an unparseable one".
func TestFindPromotedPage_MissingRevisionDefaultsToZero(t *testing.T) {
	vaultRoot := t.TempDir()
	memoryDir := filepath.Join(vaultRoot, "wiki", "memory")
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", memoryDir, err)
	}
	page := "---\nengram_id: 202\nproject: labdrian-sdd-overlay\naddress: c-000202\n---\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(memoryDir, "c-000202.md"), []byte(page), 0o644); err != nil {
		t.Fatalf("write pre-promoted page: %v", err)
	}

	promoted, ok, err := findPromotedPage(vaultRoot, "labdrian-sdd-overlay", 202)
	if err != nil {
		t.Fatalf("findPromotedPage: %v", err)
	}
	if !ok {
		t.Fatalf("findPromotedPage ok = false, want true")
	}
	if promoted.Revision != 0 {
		t.Fatalf("Revision = %d, want 0 for a page with no engram_revision field (not an error)", promoted.Revision)
	}
}

// TestFindPromotedPage_UnparseableRevisionErrors: task 7.10 Gap 2
// coverage. A page whose engram_revision cannot be parsed as an integer
// is corrupted promotion state and must error, naming the offending
// page -- never silently treated as revision 0, which could make Sync
// (R-009) skip re-promoting content that is not actually current.
func TestFindPromotedPage_UnparseableRevisionErrors(t *testing.T) {
	vaultRoot := t.TempDir()
	memoryDir := filepath.Join(vaultRoot, "wiki", "memory")
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", memoryDir, err)
	}
	page := "---\nengram_id: 203\nproject: labdrian-sdd-overlay\naddress: c-000203\nengram_revision: not-a-number\n---\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(memoryDir, "c-000203.md"), []byte(page), 0o644); err != nil {
		t.Fatalf("write pre-promoted page: %v", err)
	}

	_, _, err := findPromotedPage(vaultRoot, "labdrian-sdd-overlay", 203)
	if err == nil {
		t.Fatalf("findPromotedPage = nil error, want an error for an unparseable engram_revision")
	}
	if !strings.Contains(err.Error(), "c-000203.md") {
		t.Fatalf("error %q does not name the offending page c-000203.md", err)
	}
}
