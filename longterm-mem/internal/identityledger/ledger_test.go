package identityledger_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/identityledger"
)

func at(day int) time.Time {
	return time.Date(2026, 9, day, 12, 0, 0, 0, time.UTC)
}

// The ledger's whole reason to exist: a name this repository used before is
// still ITS name later, even once nothing derives it any more.
func TestRecordThenNames_RemembersWhatIsNoLongerDerivable(t *testing.T) {
	dir := t.TempDir()

	if err := identityledger.Record(dir, []identityledger.Name{
		{Name: "/old/path/.git", Rule: "common_dir", Adoptable: true},
	}, at(1)); err != nil {
		t.Fatalf("Record: %v", err)
	}

	got, err := identityledger.Names(dir)
	if err != nil {
		t.Fatalf("Names: %v", err)
	}
	if len(got) != 1 || got[0].Name != "/old/path/.git" {
		t.Fatalf("the ledger did not remember the name: %+v", got)
	}
}

// Recording the same name again is the common case -- every command does it.
// It must update when the name was last seen, never grow a second row: a
// ledger that duplicates is a ledger nobody can read.
func TestRecord_IsIdempotentPerNameAndKeepsFirstSeen(t *testing.T) {
	dir := t.TempDir()
	n := []identityledger.Name{{Name: "widgets", Rule: "declared", Adoptable: true}}

	if err := identityledger.Record(dir, n, at(1)); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := identityledger.Record(dir, n, at(5)); err != nil {
		t.Fatalf("Record again: %v", err)
	}

	got, err := identityledger.Names(dir)
	if err != nil {
		t.Fatalf("Names: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("re-recording a known name duplicated it: %+v", got)
	}
	if !got[0].FirstSeen.Equal(at(1)) {
		t.Errorf("FirstSeen moved: got %v, want %v", got[0].FirstSeen, at(1))
	}
	if !got[0].LastSeen.Equal(at(5)) {
		t.Errorf("LastSeen did not move: got %v, want %v", got[0].LastSeen, at(5))
	}
}

// Newest first, because the most recently used name is the likeliest one the
// memory actually lives under.
func TestNames_NewestLastSeenFirst(t *testing.T) {
	dir := t.TempDir()
	if err := identityledger.Record(dir, []identityledger.Name{{Name: "old", Rule: "declared", Adoptable: true}}, at(1)); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := identityledger.Record(dir, []identityledger.Name{{Name: "new", Rule: "declared", Adoptable: true}}, at(9)); err != nil {
		t.Fatalf("Record: %v", err)
	}

	got, err := identityledger.Names(dir)
	if err != nil {
		t.Fatalf("Names: %v", err)
	}
	if len(got) != 2 || got[0].Name != "new" || got[1].Name != "old" {
		t.Fatalf("names are not ordered newest-first: %+v", got)
	}
}

// A record that can no longer prove it describes what it recorded is the
// defect family this whole module keeps meeting. The ledger carries a digest
// of its own entries; a tampered file is REPORTED, never quietly trusted --
// trusting it would adopt a name nobody wrote.
func TestNames_TamperedLedgerIsReportedNotTrusted(t *testing.T) {
	dir := t.TempDir()
	if err := identityledger.Record(dir, []identityledger.Name{{Name: "widgets", Rule: "declared", Adoptable: true}}, at(1)); err != nil {
		t.Fatalf("Record: %v", err)
	}

	path := identityledger.Path(dir)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(raw), "widgets", "stolen!", 1)), 0o600); err != nil {
		t.Fatalf("tamper: %v", err)
	}

	if _, err := identityledger.Names(dir); !errors.Is(err, identityledger.ErrCorrupt) {
		t.Fatalf("a tampered ledger must be reported: got %v", err)
	}
}

// No ledger is the ordinary state of every repository that has not been
// asked about yet. It is emptiness, not breakage.
func TestNames_AbsentLedgerIsEmptyNotAnError(t *testing.T) {
	got, err := identityledger.Names(t.TempDir())
	if err != nil {
		t.Fatalf("an absent ledger must read as empty: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %+v, want nothing", got)
	}
}

// The ledger belongs to the repository, so it is written under the git
// common directory -- the one directory a main checkout and every linked
// worktree share. Writing it anywhere per-worktree would fragment the very
// record that exists to prevent fragmentation.
func TestPath_LivesUnderTheGivenCommonDir(t *testing.T) {
	dir := t.TempDir()
	got := identityledger.Path(dir)
	if !strings.HasPrefix(got, dir+string(filepath.Separator)) {
		t.Fatalf("ledger path %q is not inside the common dir %q", got, dir)
	}
}
