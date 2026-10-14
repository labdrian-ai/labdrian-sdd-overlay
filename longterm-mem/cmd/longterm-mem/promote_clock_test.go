package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
)

// The clock the commands are composed with is the one that dates what they write. The golden files hold the
// promote and sync commands to the instant of promoteGoldenClock; these tests give the commands an instant of
// their own, that no golden file and no fixture uses, and look for it in everything the three surfaces that
// promote (promote, sync and the MCP promote tool) wrote: a call site that dropped the clock it was handed and
// read another would write the date of the golden clock, or the day the test runs, and not this one.
var clockE2EInstant = time.Date(2031, 2, 3, 4, 5, 6, 0, time.UTC)

const (
	clockE2EDate        = "2031-02-03"
	clockE2EInstant8601 = "2031-02-03T04:05:06Z"
)

// enterClockWorld builds the world of the golden tests (a vault whose scripts are stubs, an Engram database with
// four observations, a HOME of the test's own) and makes it the one the process sees.
func enterClockWorld(t *testing.T) *promoteGoldenWorld {
	t.Helper()
	if !vault.PrerequisitePresent("python3") {
		t.Skip("python3 is not on PATH: the vault's index rebuild runs its scripts under it")
	}
	w := newPromoteGoldenWorld(t)
	t.Setenv("HOME", w.home)
	t.Setenv("LONGTERM_MEM_ENGRAM_DB", w.dbPath)
	t.Setenv("LONGTERM_MEM_VAULT", w.vaultRoot)
	t.Setenv("LONGTERM_MEM_VAULTS_FILE", "")
	t.Chdir(t.TempDir())
	return w
}

func readVaultFile(t *testing.T, w *promoteGoldenWorld, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(w.vaultRoot, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

// requireOnlyTheInjectedDate fails unless the files the promotion wrote say the injected date, and no other
// day: not the day the golden files were recorded on.
func requireOnlyTheInjectedDate(t *testing.T, w *promoteGoldenWorld, files map[string]string) {
	t.Helper()
	for rel, want := range files {
		got := readVaultFile(t, w, rel)
		if !strings.Contains(got, want) {
			t.Errorf("%s does not say %q:\n%s", rel, want, got)
		}
		if goldenDay := promoteGoldenClock.at.Format("2006-01-02"); strings.Contains(got, goldenDay) {
			t.Errorf("%s says %s, a day the clock the commands were given did not tell:\n%s", rel, goldenDay, got)
		}
	}
}

func TestPromoteDatesWhatItWritesWithTheClockItWasGiven(t *testing.T) {
	w := enterClockWorld(t)
	var code int
	captureStreams(t, func() {
		code = commands{newEmbedClient: testEmbedClient, clock: fixedClock{at: clockE2EInstant}}.run([]string{"promote", "--project", promoteGoldenProject, "--id", "1"})
	})
	if code != exitOK {
		t.Fatalf("promote exited %d", code)
	}

	requireOnlyTheInjectedDate(t, w, map[string]string{
		"wiki/memory/c-000901.md": "created: " + clockE2EDate,
		"wiki/log.md":             "## [" + clockE2EDate + "] promote | Pinned Decision",
		".raw/.manifest.json":     `"created": "` + clockE2EDate + `"`,
	})
}

func TestSyncDatesWhatItWritesWithTheClockItWasGiven(t *testing.T) {
	w := enterClockWorld(t)
	var code int
	captureStreams(t, func() {
		code = commands{newEmbedClient: testEmbedClient, clock: fixedClock{at: clockE2EInstant}}.run([]string{"sync", "--project", promoteGoldenProject})
	})
	if code != exitOK {
		t.Fatalf("sync exited %d", code)
	}

	requireOnlyTheInjectedDate(t, w, map[string]string{
		"wiki/memory/c-000901.md":                  "updated: " + clockE2EDate,
		"wiki/log.md":                              "## [" + clockE2EDate + "] promote | ",
		".raw/.manifest.json":                      `"created": "` + clockE2EDate + `"`,
		".vault-meta/longterm-mem-sync-state.json": `"last_sync_completed_at": "` + clockE2EInstant8601 + `"`,
		// The sync rebuilt the index of a vault nobody had provisioned, and the vault dated that with the clock.
		".vault-meta/.longterm-mem-provisioned": clockE2EInstant8601,
	})
}

func TestTheMCPPromoteToolDatesWhatItWritesWithTheClockItWasGiven(t *testing.T) {
	w := enterClockWorld(t)
	store, err := engram.Open(w.dbPath)
	if err != nil {
		t.Fatalf("open the Engram database: %v", err)
	}
	defer store.Close()

	outcome, err := commands{newEmbedClient: testEmbedClient, clock: fixedClock{at: clockE2EInstant}}.mcpPromote(store)(context.Background(), promoteGoldenProject, 1)
	if err != nil {
		t.Fatalf("the promote tool: %v", err)
	}
	if outcome.IndexRebuildErr != nil {
		t.Fatalf("the index rebuild of the promote tool: %v", outcome.IndexRebuildErr)
	}

	requireOnlyTheInjectedDate(t, w, map[string]string{
		"wiki/memory/c-000901.md": "created: " + clockE2EDate,
		"wiki/log.md":             "## [" + clockE2EDate + "] promote | Pinned Decision",
		".raw/.manifest.json":     `"created": "` + clockE2EDate + `"`,
		// The tool rebuilt the index of a vault nobody had provisioned, and the vault dated that with the clock.
		".vault-meta/.longterm-mem-provisioned": clockE2EInstant8601,
	})
}

func TestIndexDatesTheProvisionedVaultWithTheClockItWasGiven(t *testing.T) {
	w := enterClockWorld(t)
	var code int
	captureStreams(t, func() {
		code = commands{newEmbedClient: testEmbedClient, clock: fixedClock{at: clockE2EInstant}}.run([]string{"index", "--project", promoteGoldenProject})
	})
	if code != exitOK {
		t.Fatalf("index exited %d", code)
	}

	requireOnlyTheInjectedDate(t, w, map[string]string{".vault-meta/.longterm-mem-provisioned": clockE2EInstant8601})
}
