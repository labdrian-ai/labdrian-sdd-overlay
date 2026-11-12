package main

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// The tests of S3 of the store-agnostic promotion for the Engram source: an observation whose memorycard lineage
// memory already owns a page is bridged, as a memorycard memory bridged to an observation that has a page is, so
// it is not promoted a second time (the defect: "would promote 0 observation(s)" before the memory was re-keyed to
// memorycard, "would promote 10 observation(s)" after). They run through the command on the world of the promote
// tests: memorycardFixture's memory 2 (pinned) was imported from Engram observation 1 (pinned), and observation 2
// carries a curated topic key and has no lineage.

func promoteMemorycardMemory2(t *testing.T) *promoteGoldenWorld {
	t.Helper()
	w, _ := memorycardWorld(t)
	code, stdout, stderr := runPromoteGolden(t, []string{"promote", "--project", promoteGoldenProject, "--source", "memorycard", "--id", "2"})
	if code != exitOK || !strings.Contains(stdout, "promoted c-000901 (created)") {
		t.Fatalf("promote memory 2: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	return w
}

// An explicit promote of such an observation is refused as the explicit promote of a bridged memory is: exit 6 (the
// code of every refusal that leaves a page of the vault alone), the existing page named, nothing written. The MCP
// promote tool refuses it with the page named too.
func TestEngramPromoteRefusesAnObservationWhoseLineageMemoryOwnsAPage(t *testing.T) {
	w := promoteMemorycardMemory2(t)
	treeBefore := renderPromoteGoldenTree(t, w.vaultRoot)

	code, stdout, stderr := runPromoteGolden(t, []string{"promote", "--project", promoteGoldenProject, "--id", "1"})
	if code != exitRegistrationConflict {
		t.Fatalf("exit %d, want %d\nstdout: %s\nstderr: %s", code, exitRegistrationConflict, stdout, stderr)
	}
	if !strings.Contains(stderr, "c-000901") || !strings.Contains(stderr, "Engram observation 1 is bridged to memorycard memory 2") {
		t.Errorf("stderr = %q, want the refusal to name the page c-000901 and the memory it is bridged to", stderr)
	}
	if renderPromoteGoldenTree(t, w.vaultRoot) != treeBefore {
		t.Errorf("the refused promote changed the vault")
	}

	store, err := engram.Open(w.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	tool := commands{newEmbedClient: testEmbedClient, clock: promoteGoldenClock}.mcpPromote(store)
	if _, err := tool(context.Background(), promoteGoldenProject, "engram", 1); err == nil || !strings.Contains(err.Error(), "c-000901") {
		t.Errorf("bridged observation over MCP: err = %v, want a refusal naming c-000901", err)
	}
}

func TestEngramSyncDryRunDoesNotPlanAnObservationWhoseLineageMemoryOwnsAPage(t *testing.T) {
	w := promoteMemorycardMemory2(t)
	treeBefore := renderPromoteGoldenTree(t, w.vaultRoot)

	code, stdout, stderr := runSync(t, "--dry-run")
	if code != exitOK {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	// Observation 2 only: observation 1 is bridged and is counted among the skipped, with the two it is not
	// eligible for.
	if !strings.Contains(stdout, "would promote 1 observation(s), patch 0 page(s), skip 3, into ") {
		t.Errorf("stdout = %q, want one observation to promote and three skipped (two ineligible, one bridged)", stdout)
	}
	if !strings.Contains(stdout, "would promote: Curated Note") || strings.Contains(stdout, "would promote: Pinned Decision") {
		t.Errorf("stdout = %q, want observation 2 named and observation 1 not", stdout)
	}
	if after := renderPromoteGoldenTree(t, w.vaultRoot); after != treeBefore {
		t.Errorf("the dry run changed the vault")
	}
}

// When memorycard cannot say the lineage, the Engram sync fails (for now) with the failure named as memorycard's:
// not with the exit code of an Engram that can no longer be listed, which it is not.
func TestEngramSyncNamesMemorycardWhenItsLineageCannotBeRead(t *testing.T) {
	_, cardDB := memorycardWorld(t)
	db, err := sql.Open("sqlite", cardDB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`drop table import_map`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := runSync(t)

	if code != exitInternal {
		t.Errorf("exit %d, want %d: the store of the memories answers, so it is not %d (Engram unavailable)\nstdout: %s\nstderr: %s", code, exitInternal, exitEngramUnavailable, stdout, stderr)
	}
	if !strings.Contains(stderr, "memorycard: read the lineage of the memories") {
		t.Errorf("stderr = %q, want the failure to name memorycard", stderr)
	}
}

func TestEngramSyncDoesNotPromoteAnObservationWhoseLineageMemoryOwnsAPage(t *testing.T) {
	w := promoteMemorycardMemory2(t)
	cardPage := readVaultFile(t, w, "wiki/memory/c-000901.md")

	code, stdout, stderr := runSync(t)
	if code != exitOK {
		t.Fatalf("exit %d, want 0: a bridged observation is skipped and is no failure\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "sync promoted 1 observation(s), patched 0 page(s)") {
		t.Errorf("stdout = %q, want the summary to keep its words and count one promotion", stdout)
	}
	if names := pageNames(t, w); len(names) != 2 {
		t.Errorf("pages = %v, want the memorycard page and observation 2's page, and none for observation 1", names)
	}
	if got := readVaultFile(t, w, "wiki/memory/c-000901.md"); got != cardPage {
		t.Errorf("the bridged page changed:\n%s", got)
	}
	if strings.Contains(stderr, "bridged") {
		t.Errorf("stderr = %q, want a bridged observation named nowhere", stderr)
	}
}
