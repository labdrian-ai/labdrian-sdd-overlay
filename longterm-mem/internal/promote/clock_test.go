package promote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// fakeClock is the Clock of a test: it says the time it was given, and a test moves it to say a later
// one. It replaces the nowFunc package variable the tests used to swap, so no test changes state that
// another can see.
type fakeClock struct{ at time.Time }

func (c *fakeClock) Now() time.Time { return c.at }

// Set moves the clock to a later (or earlier) instant, as a test of a promotion that spans days does.
func (c *fakeClock) Set(at time.Time) { c.at = at }

// TestEmitPage_DatesThePageInUTCWhateverZoneTheClockSpeaks (Phase 9, L2): a page is dated by the day it
// is in UTC. The clock is a port, so the zone it answers in is the adapter's business; the date rule is
// promote's, and it must not depend on whether the adapter converted.
func TestEmitPage_DatesThePageInUTCWhateverZoneTheClockSpeaks(t *testing.T) {
	// 23:30 on 1 August in UTC-5 is 04:30 on 2 August in UTC.
	west := time.FixedZone("UTC-5", -5*60*60)
	at := time.Date(2026, 8, 1, 23, 30, 0, 0, west)
	obs := memory.Observation{ID: 701, Type: "decision", Title: "Zoned", Content: "Body.", Project: "labdrian-sdd-overlay", RevisionCount: 1}

	page, err := EmitPage(obs, "c-000701", nil, at)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	for _, want := range []string{"created: 2026-08-02\n", "updated: 2026-08-02\n"} {
		if !strings.Contains(page.Frontmatter, want) {
			t.Errorf("frontmatter lacks %q (the UTC day), got:\n%s", want, page.Frontmatter)
		}
	}
}

// TestPromote_DatesTheNewPageAndTheLogWithTheWritersClock: the day on a page and in the promotion log is
// the day the Writer's own clock gave, and a clock that moves moves the next promotion with it.
func TestPromote_DatesTheNewPageAndTheLogWithTheWritersClock(t *testing.T) {
	vaultRoot := t.TempDir()
	clock := &fakeClock{at: time.Date(2026, 9, 3, 22, 0, 0, 0, time.UTC)}
	w := &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}, Addresses: staticAddress(testAddress), Clock: clock}
	obs := memory.Observation{ID: 703, Type: "decision", Title: "Dated", Content: "Body.", Project: "labdrian-sdd-overlay", RevisionCount: 1, Pinned: true}

	result, err := w.Promote(obs, false)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	for _, want := range []string{"created: 2026-09-03\n", "updated: 2026-09-03\n"} {
		if !strings.Contains(result.Page.Frontmatter, want) {
			t.Errorf("page frontmatter lacks %q, got:\n%s", want, result.Page.Frontmatter)
		}
	}
	logData, err := os.ReadFile(filepath.Join(vaultRoot, "wiki", "log.md"))
	if err != nil {
		t.Fatalf("read wiki/log.md: %v", err)
	}
	if !strings.Contains(string(logData), "## [2026-09-03] promote | Dated") {
		t.Errorf("wiki/log.md lacks the entry dated by the writer's clock, got:\n%s", logData)
	}
}

// TestPromote_RefusesAWriterWithoutAClockAndWritesNothing: the clock is a port the writer is handed. A
// writer built without one says so, in its own words, before it allocates an address or writes a page,
// instead of failing inside the first call that needs the time.
func TestPromote_RefusesAWriterWithoutAClockAndWritesNothing(t *testing.T) {
	vaultRoot := t.TempDir()
	w := &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}}
	obs := memory.Observation{ID: 702, Type: "decision", Title: "No Clock", Content: "Body.", Project: "labdrian-sdd-overlay", RevisionCount: 1, Pinned: true}

	result, err := w.Promote(obs, false)
	if err == nil {
		t.Fatalf("Promote = %+v, nil error, want a writer without a clock refused", result)
	}
	if result.Action.Kind != ActionNone {
		t.Errorf("a refused promotion reports Action %v, want ActionNone", result.Action.Kind)
	}
	entries, readErr := os.ReadDir(vaultRoot)
	if readErr != nil {
		t.Fatalf("read the vault: %v", readErr)
	}
	if len(entries) != 0 {
		t.Errorf("a refused promotion wrote %d entries into the vault", len(entries))
	}
}

// TestSync_RefusesAWriterWithoutAClockBeforeTouchingAnything: Sync stamps the completion of the run with
// the writer's clock, so a writer without one is refused up front rather than after every page was
// promoted.
func TestSync_RefusesAWriterWithoutAClockBeforeTouchingAnything(t *testing.T) {
	vaultRoot := t.TempDir()
	deps := Deps{Memory: &fakeMemory{}, Writer: &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}}}

	if _, err := Sync(t.Context(), deps, "labdrian-sdd-overlay"); err == nil {
		t.Fatal("Sync = nil error, want a writer without a clock refused")
	}
	if _, err := os.Stat(filepath.Join(vaultRoot, syncStateRelPath)); !os.IsNotExist(err) {
		t.Errorf("the sync-state record exists after a refused run (stat err = %v)", err)
	}
}

// testInstant is the instant of a test that has no reason to care which one it is.
var testInstant = time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
