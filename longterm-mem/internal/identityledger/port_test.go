package identityledger_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/adoption"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/identityledger"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/projectid"
)

// The ledger, as the adoption use case sees it: the names a repository derives go in with their rule and with the
// strictness that decides whether they may later be adopted on the ledger's word alone; the names a repository was
// known by come out most recently seen first.

func TestLedger_RecordsWhatTheRepositoryDerivesWithItsRuleAndItsStrictness(t *testing.T) {
	common := t.TempDir()
	first := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	later := first.Add(24 * time.Hour)
	ledger := identityledger.Ledger{}

	if err := ledger.Record(common, []projectid.DerivedName{
		{Name: "github.com/acme/widgets", Rule: projectid.RuleRemote, Strict: true},
		{Name: "widgets", Rule: projectid.RuleRemote, Strict: false},
	}, first); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := ledger.Record(common, []projectid.DerivedName{
		{Name: "chosen-name", Rule: projectid.RuleDeclared, Strict: true},
		{Name: "widgets", Rule: projectid.RuleRemote, Strict: false},
	}, later); err != nil {
		t.Fatalf("Record: %v", err)
	}

	entries, err := identityledger.Names(common)
	if err != nil {
		t.Fatalf("Names: %v", err)
	}
	want := []identityledger.Name{
		{Name: "chosen-name", Rule: "declared", Adoptable: true, FirstSeen: later, LastSeen: later},
		{Name: "widgets", Rule: "remote", Adoptable: false, FirstSeen: first, LastSeen: later},
		{Name: "github.com/acme/widgets", Rule: "remote", Adoptable: true, FirstSeen: first, LastSeen: first},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("the ledger holds %+v, want %+v", entries, want)
	}
}

func TestLedger_KnownIsWhatWasRecordedMostRecentlySeenFirst(t *testing.T) {
	common := t.TempDir()
	at := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	ledger := identityledger.Ledger{}
	if err := ledger.Record(common, []projectid.DerivedName{{Name: "old", Rule: projectid.RuleCommonDir, Strict: true}}, at); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Record(common, []projectid.DerivedName{{Name: "new", Rule: projectid.RuleRemote, Strict: false}}, at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	got, err := ledger.Known(common)

	want := []adoption.KnownName{{Name: "new", Adoptable: false}, {Name: "old", Adoptable: true}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Known = %+v, %v; want %+v", got, err, want)
	}
}

// A repository with no ledger has been known by nothing yet: that is emptiness, not an error.
func TestLedger_AbsentLedgerKnowsNothing(t *testing.T) {
	got, err := identityledger.Ledger{}.Known(t.TempDir())

	if err != nil || len(got) != 0 {
		t.Errorf("Known = %+v, %v; want nothing and no error", got, err)
	}
}

// A ledger that cannot be trusted is reported, never read as empty.
func TestLedger_ACorruptLedgerIsReported(t *testing.T) {
	common := t.TempDir()
	path := identityledger.Path(common)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := identityledger.Ledger{}.Known(common)

	if !errors.Is(err, identityledger.ErrCorrupt) || got != nil {
		t.Errorf("Known = %+v, %v; want ErrCorrupt and no names", got, err)
	}
	if err := (identityledger.Ledger{}).Record(common, []projectid.DerivedName{{Name: "x", Rule: projectid.RuleDeclared, Strict: true}}, time.Unix(0, 0)); !errors.Is(err, identityledger.ErrCorrupt) {
		t.Errorf("Record over a corrupt ledger = %v, want ErrCorrupt: it must not be overwritten", err)
	}
}
